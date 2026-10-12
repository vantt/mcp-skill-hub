package mcpserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// loadBenchSkillID names a skill shaped like a real multi-file skill: an
// entrypoint plus twelve reference files, about 186 KB in all.
const loadBenchSkillID = "load-bench"

func newLoadBenchWorkspace(tb testing.TB) string {
	tb.Helper()
	root := filepath.Join(tb.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		tb.Fatal(err)
	}
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	for _, id := range []string{loadBenchSkillID, "load-bench-small"} {
		base := "skills/benchmark/" + id
		write(base+"/skill.meta.yaml", fmt.Sprintf("schema_version: 1\nid: %s\nname: Load Bench\nstatus: active\ndescription: Review benchmark load paths.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review load paths]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id))
		write(base+"/SKILL.md", fmt.Sprintf("---\nname: %s\ndescription: Review benchmark load paths.\n---\n\n# Load bench\n\n%s\n", id, strings.Repeat("Read the reference files before answering.\n", 150)))
	}
	for index := range 12 {
		write(fmt.Sprintf("skills/benchmark/%s/references/part-%02d.md", loadBenchSkillID, index), "# Part\n\n"+strings.Repeat("Reference text for the benchmark skill.\n", 400))
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		tb.Fatal(err)
	}
	// A hub nobody edited in the last minute: the integrity result of the
	// published catalog is cached once its file is older than a few seconds.
	past := time.Now().Add(-time.Minute)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		return os.Chtimes(path, past, past)
	})
	if err != nil {
		tb.Fatal(err)
	}
	return root
}

func connectLoadBenchSession(tb testing.TB, root string) (*Server, *mcp.ClientSession) {
	tb.Helper()
	adapter, server, err := New(root, nil)
	if err != nil {
		tb.Fatal(err)
	}
	recorder, err := (app.TelemetryService{}).Open(root)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = recorder.Close(context.Background()) })
	adapter.telemetry = recorder
	adapter.resolver.Telemetry = recorder
	adapter.feedback.Recorder = recorder
	capabilities := &mcp.ClientCapabilities{}
	capabilities.AddExtension("io.modelcontextprotocol/skills", map[string]any{})
	client := mcp.NewClient(&mcp.Implementation{Name: "benchmark", Version: "1"}, &mcp.ClientOptions{Capabilities: capabilities})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		tb.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
		tb.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = session.Close() })
	return adapter, session
}

// BenchmarkLoadPaths measures the requests an agent makes to load a skill,
// warm over an in-memory MCP session, plus the first skill_get of a session.
func BenchmarkLoadPaths(b *testing.B) {
	root := newLoadBenchWorkspace(b)
	ctx := context.Background()
	adapter, session := connectLoadBenchSession(b, root)
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](ctx, session, "skills/list", &listSkillsParams{})
	if err != nil {
		b.Fatal(err)
	}
	var entryURI string
	for _, entry := range listed.Skills {
		if remoteEntryID(entry.URI) == loadBenchSkillID {
			entryURI = entry.URI
		}
	}
	if entryURI == "" {
		b.Fatalf("skills/list has no %s: %#v", loadBenchSkillID, listed.Skills)
	}
	b.Run("skill_get_warm", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "skill_get", Arguments: map[string]any{"skill_id": loadBenchSkillID}})
			if err != nil || result.IsError {
				b.Fatalf("skill_get = %#v, %v", result, err)
			}
		}
	})
	b.Run("skill_get_first_call", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			b.StopTimer()
			_, fresh := connectLoadBenchSession(b, root)
			b.StartTimer()
			result, err := fresh.CallTool(ctx, &mcp.CallToolParams{Name: "skill_get", Arguments: map[string]any{"skill_id": loadBenchSkillID}})
			if err != nil || result.IsError {
				b.Fatalf("skill_get = %#v, %v", result, err)
			}
		}
	})
	b.Run("resources_read", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			// The handler is called directly: the client caches a read for its ttl.
			if _, err := adapter.readResource(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: entryURI}}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("skills_get", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](ctx, session, "skills/get", &getSkillParams{URI: entryURI}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("skill_resolve", func(b *testing.B) {
		b.ReportAllocs()
		args := map[string]any{"schema_version": "1", "request_id": "bench", "task": map[string]any{"description": "review load paths", "scope": "multi_step"}, "operation": "review"}
		for range b.N {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "skill_resolve", Arguments: args})
			if err != nil || result.IsError {
				b.Fatalf("skill_resolve = %#v, %v", result, err)
			}
		}
	})
}
