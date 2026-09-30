package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func FuzzMCPFrameAndInputHelpers(f *testing.F) {
	for _, frame := range [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skill_resolve","arguments":{"schema_version":"1","request_id":"seed","task":{"description":"review code","scope":"multi_step"},"operation":"review"}}}`),
		[]byte(`{"jsonrpc":"2.0","id":"list","method":"tools/list","params":{}}`),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`),
		[]byte(`{"jsonrpc":"2.0","method":""}`),
		[]byte(`{}`),
		[]byte(`{malformed}`),
	} {
		f.Add(frame)
	}

	f.Fuzz(func(t *testing.T, frame []byte) {
		if len(frame) == 0 || len(frame) > MaxFrameBytes {
			t.Skip()
		}
		var object map[string]json.RawMessage
		isJSONObject := json.Unmarshal(frame, &object) == nil && object != nil
		message, decodeErr := jsonrpc.DecodeMessage(frame)
		if request, ok := message.(*jsonrpc.Request); isJSONObject && decodeErr == nil && ok && request.Method != "" {
			assertRequestRoundTrip(t, request)
		}

		sample := frame
		if len(sample) > 128 {
			sample = sample[:128]
		}
		generatedInput := resolveInput(resolver.Request{
			SchemaVersion: SchemaVersion,
			RequestID:     fmt.Sprintf("fuzz-%d", len(frame)),
			Task: resolver.Task{
				Description: fmt.Sprintf("fuzz input %x", sample),
				Scope:       "multi_step",
			},
			Operation: "review",
		})
		if _, err := resolver.NormalizeRequest(resolver.Request(generatedInput)); err != nil {
			t.Fatalf("normalize generated request: %v", err)
		}
		params, err := json.Marshal(struct {
			Name      string       `json:"name"`
			Arguments resolveInput `json:"arguments"`
		}{Name: "skill_resolve", Arguments: generatedInput})
		if err != nil {
			t.Fatalf("marshal generated request params: %v", err)
		}
		id, err := jsonrpc.MakeID(float64(len(frame)))
		if err != nil {
			t.Fatalf("create generated request ID: %v", err)
		}
		assertRequestRoundTrip(t, &jsonrpc.Request{ID: id, Method: "tools/call", Params: params})

		var call struct {
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(frame, &call) == nil && len(call.Params.Arguments) > 0 {
			var page pageInput
			if json.Unmarshal(call.Params.Arguments, &page) == nil {
				_, _ = normalizeLimit(page.Limit)
				_, _ = decodeCursor(page.Cursor, "fuzz-owner", "fuzz-filter")
			}
			var request resolveInput
			if json.Unmarshal(call.Params.Arguments, &request) == nil {
				_, _ = resolver.NormalizeRequest(resolver.Request(request))
			}
			var feedback feedbackInput
			_ = json.Unmarshal(call.Params.Arguments, &feedback)
		}

		schema, err := jsonschema.For[resolveInput](&jsonschema.ForOptions{})
		if err != nil {
			t.Fatal(err)
		}
		closeObjectSchemas(schema)
		applyCommonConstraints(schema)
		normalizeMultiTypeSchemas(schema)
		if schema.AdditionalProperties == nil {
			t.Fatal("resolve input schema permits unspecified object properties")
		}
		version := schema.Properties["schema_version"]
		if version == nil || len(version.Enum) != 1 || version.Enum[0] != SchemaVersion {
			t.Fatalf("resolve input schema version constraint = %#v", version)
		}
	})
}

func assertRequestRoundTrip(t *testing.T, request *jsonrpc.Request) {
	t.Helper()
	encoded, err := jsonrpc.EncodeMessage(request)
	if err != nil {
		t.Fatalf("encode protocol-valid request: %v", err)
	}
	message, err := jsonrpc.DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("decode re-encoded protocol-valid request: %v", err)
	}
	roundTripped, ok := message.(*jsonrpc.Request)
	if !ok {
		t.Fatalf("re-encoded protocol-valid request decoded as %T", message)
	}
	if roundTripped.Method != request.Method || roundTripped.ID.Raw() != request.ID.Raw() {
		t.Fatalf("request identity changed: method=%q id=%v", roundTripped.Method, roundTripped.ID.Raw())
	}
	if len(request.Params) == 0 || len(roundTripped.Params) == 0 {
		if len(request.Params) != len(roundTripped.Params) {
			t.Fatalf("request params presence changed: got %s, want %s", roundTripped.Params, request.Params)
		}
		return
	}
	var wantParams, gotParams bytes.Buffer
	if err := json.Compact(&wantParams, request.Params); err != nil {
		t.Fatalf("compact request params: %v", err)
	}
	if err := json.Compact(&gotParams, roundTripped.Params); err != nil {
		t.Fatalf("compact round-tripped params: %v", err)
	}
	if !bytes.Equal(gotParams.Bytes(), wantParams.Bytes()) {
		t.Fatalf("request params changed: got %s, want %s", gotParams.Bytes(), wantParams.Bytes())
	}
}

func TestMultipleStdioProcessesConcurrentFramesAreBounded(t *testing.T) {
	binary := buildSkillHub(t)
	const processes = 4
	roots := make([]string, processes)
	for process := range processes {
		roots[process] = newMCPWorkspace(t)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, processes)
	var wait sync.WaitGroup
	for process := 0; process < processes; process++ {
		process := process
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var diagnostics bytes.Buffer
			command := exec.CommandContext(ctx, binary, "mcp", "serve", "--workspace", roots[process])
			command.Stderr = &diagnostics
			client := mcp.NewClient(&mcp.Implementation{Name: fmt.Sprintf("hardening-%d", process), Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
			session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
			if err != nil {
				errs <- fmt.Errorf("process %d connect: %w; stderr=%s", process, err, diagnostics.String())
				return
			}

			var processErr error
			for iteration := 0; iteration < 8; iteration++ {
				listed, err := session.ListTools(ctx, nil)
				if err != nil {
					processErr = fmt.Errorf("process %d list %d: %w", process, iteration, err)
					break
				}
				if len(listed.Tools) != 35 {
					processErr = fmt.Errorf("process %d list %d: tools=%d", process, iteration, len(listed.Tools))
					break
				}
				status, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hub_status", Arguments: map[string]any{}})
				if err != nil || status == nil || status.IsError {
					processErr = fmt.Errorf("process %d status %d: result=%#v err=%v", process, iteration, status, err)
					break
				}
			}
			if err := session.Close(); err != nil && processErr == nil {
				processErr = fmt.Errorf("process %d close: %w", process, err)
			}
			if processErr != nil {
				errs <- fmt.Errorf("%w; stderr=%s", processErr, diagnostics.String())
			}
		}()
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		wait.Wait()
		t.Fatalf("stdio processes deadlocked or exceeded bound: %v", ctx.Err())
	}
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func BenchmarkMCPFrameParsing(b *testing.B) {
	frame := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"skill_resolve","arguments":{"schema_version":"1","request_id":"benchmark","task":{"description":"review changed code","scope":"multi_step"},"operation":"review"}}}`)
	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(len(frame)), "frame_bytes")
	for range b.N {
		message, err := jsonrpc.DecodeMessage(frame)
		if err != nil || message == nil {
			b.Fatalf("decode = %#v, %v", message, err)
		}
	}
}

func BenchmarkMCPToolList(b *testing.B) {
	root := newMCPBenchmarkWorkspace(b)
	workspaceBytes := mcpWorkspaceBytes(b, root)
	_, server, err := New(root, nil)
	if err != nil {
		b.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "benchmark", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer serverSession.Close()
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer session.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(workspaceBytes), "workspace_bytes")
	for range b.N {
		listed, err := session.ListTools(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(listed.Tools) != 33 {
			b.Fatalf("tool list = %d", len(listed.Tools))
		}
	}
}

func newMCPBenchmarkWorkspace(b *testing.B) string {
	b.Helper()
	root := filepath.Join(b.TempDir(), "workspace")
	if _, err := (app.WorkspaceService{}).Init(root, true); err != nil {
		b.Fatal(err)
	}
	service := app.SkillService{}
	content := []byte("---\nname: benchmark-review\ndescription: Review benchmark changes safely.\n---\n\n# Review\n\nReview carefully.\n")
	created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID: "benchmark-review", Collection: "benchmark", Name: "Benchmark Review", Description: "Review benchmark changes safely.", Content: content,
		Routing: skill.RoutingInput{Operations: []string{"review"}, Triggers: []string{"review benchmark changes"}, NotFor: []string{"write prose"}, MinScope: "multi_step"},
	}, false)
	if err != nil {
		b.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		b.Fatalf("confirm create = %#v, %v", result, err)
	}
	activated, err := service.PreviewActivate(context.Background(), root, "benchmark-review", false)
	if err != nil {
		b.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, activated, activated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		b.Fatalf("confirm activate = %#v, %v", result, err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		b.Fatal(err)
	}
	return root
}

func mcpWorkspaceBytes(tb testing.TB, root string) int64 {
	tb.Helper()
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		tb.Fatal(err)
	}
	return total
}
