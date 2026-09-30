package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestCLICreateActivateThenMCPListGetRead(t *testing.T) {
	binary := buildSkillHub(t)
	root := filepath.Join(t.TempDir(), "workspace")
	run := func(args ...string) []byte {
		command := exec.Command(binary, args...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("skillhub %s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return output
	}
	run("init", root, "--yes")
	run("skill", "create", "--workspace", root, "--id", "created-cli", "--collection", "core", "--name", "Created CLI", "--description", "Created through the canonical CLI lifecycle.", "--operation", "review", "--trigger", "review a created CLI skill", "--not-for", "write prose", "--min-scope", "multi_step", "--idempotency-key", "create-e2e", "--yes")
	run("skill", "activate", "--workspace", root, "created-cli", "--idempotency-key", "activate-e2e", "--yes")

	contents, err := os.ReadFile(filepath.Join(root, "skills", "core", "created-cli", "SKILL.md"))
	if err != nil || !bytes.HasPrefix(contents, []byte("---\nname: created-cli\ndescription:")) {
		t.Fatalf("created SEP-2640 frontmatter = %q, %v", contents, err)
	}
	var stderr bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "create-e2e", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, stderr.String())
	}
	defer session.Close()
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatalf("skills/list = %#v, %v", listed, err)
	}
	entry := findSkillEntryByName(t, listed.Skills, "created-cli")
	got, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: entry.URI})
	if err != nil || got.Skill.URI != entry.URI {
		t.Fatalf("workspace skills/get = %#v, %v", got, err)
	}
	resource, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: entry.URI})
	if err != nil || len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "# Created CLI") {
		t.Fatalf("workspace resources/read = %#v, %v", resource, err)
	}
	curator := findSkillEntryByName(t, listed.Skills, systemskills.CuratorSkillID)
	if curator.Frontmatter["activation-policy"] != systemskills.CuratorActivationPolicy || curator.Frontmatter["contract-version"] != systemskills.CuratorContractVersion {
		t.Fatalf("bundled curator entry = %#v", curator)
	}
	gotCurator, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: curator.URI})
	if err != nil || gotCurator.Skill.URI != curator.URI {
		t.Fatalf("bundled curator skills/get = %#v, %v", gotCurator, err)
	}
	curatorResource, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: curator.URI})
	if err != nil || len(curatorResource.Contents) != 1 || curatorResource.Contents[0].Text != systemskills.CuratorSkill {
		t.Fatalf("bundled curator resources/read = %#v, %v", curatorResource, err)
	}
}

func TestMCPSourceUsesServeTelemetryRecorderWithoutPersistingRawInput(t *testing.T) {
	root := newMCPWorkspace(t)
	binary := buildSkillHub(t)
	var diagnostics bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &diagnostics
	client := mcp.NewClient(&mcp.Implementation{Name: "source-telemetry-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, diagnostics.String())
	}
	const locator = "https://github.com/private/source.git"
	const reason = "private source rationale"
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "source_intake_add", Arguments: map[string]any{
		"locator": locator, "reason": reason, "idempotency_key": "mcp-source-telemetry",
	}})
	if err != nil || result.IsError {
		_ = session.Close()
		t.Fatalf("source_intake_add = %#v, %v; stderr=%s", result, err, diagnostics.String())
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close MCP session: %v; stderr=%s", err, diagnostics.String())
	}
	exportPath := filepath.Join(t.TempDir(), "source-telemetry.jsonl")
	if _, err := (app.TelemetryService{}).Export(t.Context(), root, exportPath); err != nil {
		t.Fatal(err)
	}
	exported, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(exported, []byte(`"event_type":"`+telemetry.EventSourceCandidateCaptured+`"`)) {
		t.Fatalf("source telemetry was not recorded by Serve: %s", exported)
	}
	for _, raw := range []string{locator, reason} {
		if bytes.Contains(exported, []byte(raw)) {
			t.Fatalf("source telemetry leaked %q: %s", raw, exported)
		}
	}
}

func TestMCPCurationSessionRecordExportsObservedEvidence(t *testing.T) {
	root := newMCPWorkspace(t)
	before, err := (app.WorkspaceService{}).GetCurationDiff(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	binary := buildSkillHub(t)
	var diagnostics bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &diagnostics
	client := mcp.NewClient(&mcp.Implementation{Name: "curation-telemetry-e2e", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, diagnostics.String())
	}
	arguments := map[string]any{
		"schema_version": "1", "event_id": "evt_curation_e2e", "status": "completed", "basis": "host-reported",
		"turns_to_next_action": 2, "unnecessary_confirmations": 0, "prompts_per_batch": 1,
		"batch_size": 5, "auto_finalized": true, "recovery_completed": true,
		"routine_git_noise": 0, "duration_ms": 1400,
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "curation_session_record", Arguments: arguments})
		if callErr != nil || result.IsError {
			_ = session.Close()
			t.Fatalf("curation_session_record attempt %d = %#v, %v; stderr=%s", attempt, result, callErr, diagnostics.String())
		}
		var outcome toolOutcome[app.CurationSessionResult]
		decodeStructuredContent(t, result, &outcome)
		if outcome.Result == nil || outcome.Result.Deduplicated != (attempt == 1) || outcome.Result.CanonicalMutated || outcome.Result.PolicyMutated {
			t.Fatalf("curation_session_record outcome %d = %#v", attempt, outcome)
		}
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close MCP session: %v; stderr=%s", err, diagnostics.String())
	}

	after, err := (app.WorkspaceService{}).GetCurationDiff(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatalf("telemetry-only tool changed canonical Git state\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
	exportPath := filepath.Join(t.TempDir(), "curation-telemetry.jsonl")
	exported, err := (app.TelemetryService{}).Export(t.Context(), root, exportPath)
	if err != nil || exported.Events != 1 || exported.Skipped != 0 {
		t.Fatalf("export = %+v, %v", exported, err)
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"event_type":"curation.session_completed"`, `"event_id":"evt_curation_e2e"`,
		`"basis":"host-reported"`, `"turns_to_next_action":2`, `"routine_git_noise":0`,
		`"catalog_snapshot":"sha256:`, `"policy_revision":"sha256:`, `"content_mode":"none"`,
	} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatalf("export omitted %q: %s", expected, data)
		}
	}
	for _, prohibited := range []string{root, "task", "conversation", "source_path"} {
		if bytes.Contains(data, []byte(prohibited)) {
			t.Fatalf("export leaked %q: %s", prohibited, data)
		}
	}
}

func TestMCPResolveTelemetryLifecycle(t *testing.T) {
	root := newMCPWorkspace(t)
	binary := buildSkillHub(t)
	privateDescription := "review changed code for private-task-description"
	arguments := map[string]any{
		"schema_version": "1", "request_id": "REQ-mcp-telemetry",
		"task":      map[string]any{"description": privateDescription, "scope": "multi_step"},
		"operation": "review",
	}

	healthy, healthyDiagnostics := callMCPResolve(t, binary, root, arguments, true)
	if healthy.Result == nil || healthy.Result.Resolution.Primary == nil {
		t.Fatalf("healthy resolution = %#v; diagnostics=%s", healthy, healthyDiagnostics)
	}

	exportPath := filepath.Join(t.TempDir(), "mcp-telemetry.jsonl")
	exported, err := (app.TelemetryService{}).Export(t.Context(), root, exportPath)
	if err != nil {
		t.Fatalf("export MCP telemetry: %v", err)
	}
	if exported.Events < 3 || exported.Skipped != 0 {
		t.Fatalf("export result = %#v, want at least three valid resolution events", exported)
	}
	exportBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{privateDescription} {
		if bytes.Contains(exportBytes, []byte(secret)) {
			t.Fatalf("telemetry export contains private request content %q: %s", secret, exportBytes)
		}
	}
	foundRecommendation, foundFeedback := false, false
	for _, line := range bytes.Split(bytes.TrimSpace(exportBytes), []byte{'\n'}) {
		var envelope struct {
			EventVersion string `json:"event_version"`
			EventType    string `json:"event_type"`
			RequestID    string `json:"request_id"`
			Privacy      struct {
				ContentMode      string `json:"content_mode"`
				RedactionVersion string `json:"redaction_version"`
			} `json:"privacy"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.Fatalf("decode telemetry envelope: %v; line=%s", err, line)
		}
		if envelope.EventVersion != telemetry.EventVersion || (envelope.RequestID != "" && envelope.RequestID != "REQ-mcp-telemetry") || envelope.Privacy.ContentMode != telemetry.ContentModeNone || envelope.Privacy.RedactionVersion != telemetry.RedactionVersion {
			t.Fatalf("unsanitized or unversioned telemetry envelope: %s", line)
		}
		if _, exists := envelope.Payload["task"]; exists {
			t.Fatalf("telemetry payload persisted task content: %s", line)
		}
		if envelope.EventType == "skill.feedback" {
			t.Fatalf("legacy feedback row was exported: %s", line)
		}
		foundRecommendation = foundRecommendation || envelope.EventType == telemetry.EventResolutionRecommended
		foundFeedback = foundFeedback || envelope.EventType == telemetry.EventSkillUsed
	}
	if !foundRecommendation || !foundFeedback {
		t.Fatalf("telemetry export omitted recommendation or funnel feedback: %s", exportBytes)
	}

	databasePath := filepath.Join(root, "runtime", "telemetry.db")
	if err := os.RemoveAll(databasePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(databasePath, 0o700); err != nil {
		t.Fatal(err)
	}
	degraded, degradedDiagnostics := callMCPResolve(t, binary, root, arguments, false)
	if !reflect.DeepEqual(degraded.Result, healthy.Result) || !reflect.DeepEqual(degraded.Error, healthy.Error) {
		t.Fatalf("telemetry path failure changed MCP response\nhealthy:  %#v\ndegraded: %#v", healthy, degraded)
	}
	if !strings.Contains(degradedDiagnostics, "MCP telemetry recorder") {
		t.Fatalf("telemetry path failure was not logged as a warning: %q", degradedDiagnostics)
	}
}

func callMCPResolve(t *testing.T, binary, root string, arguments map[string]any, wantFeedbackSuccess bool) (toolOutcome[resolveResult], string) {
	t.Helper()
	var diagnostics bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &diagnostics
	client := mcp.NewClient(&mcp.Implementation{Name: "telemetry-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, diagnostics.String())
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: arguments})
	if err != nil || result.IsError {
		_ = session.Close()
		t.Fatalf("skill_resolve = %#v, %v; stderr=%s", result, err, diagnostics.String())
	}
	var outcome toolOutcome[resolveResult]
	decodeStructuredContent(t, result, &outcome)
	feedback, feedbackErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_feedback", Arguments: map[string]any{
		"schema_version": "1", "resolution_id": outcome.Result.Resolution.ResolutionID,
		"event_id": "evt_mcp_feedback", "outcome": "used", "selected_skill": outcome.Result.Resolution.Primary.ID,
	}})
	if feedbackErr != nil || feedback.IsError == wantFeedbackSuccess {
		_ = session.Close()
		t.Fatalf("skill_feedback success=%v result=%#v err=%v; stderr=%s", wantFeedbackSuccess, feedback, feedbackErr, diagnostics.String())
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close MCP session: %v; stderr=%s", err, diagnostics.String())
	}
	return outcome, diagnostics.String()
}

func TestBuiltStdioLifecycleAndSemanticParity(t *testing.T) {
	root := newMCPWorkspace(t)
	binary := buildSkillHub(t)
	var stderr bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "subprocess-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, stderr.String())
	}
	status, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "hub_status", Arguments: map[string]any{}})
	if err != nil || status.IsError {
		t.Fatalf("hub_status = %#v, %v; stderr=%s", status, err, stderr.String())
	}
	var wire toolOutcome[app.CurationHome]
	data, _ := json.Marshal(status.StructuredContent)
	if err := json.Unmarshal(data, &wire); err != nil || wire.Result == nil {
		t.Fatalf("decode status: %v, %s", err, data)
	}
	cliCommand := exec.CommandContext(t.Context(), binary, "status", "--workspace", root, "--json")
	cliOutput, err := cliCommand.Output()
	if err != nil {
		t.Fatalf("CLI status: %v", err)
	}
	var cliResult app.CurationHome
	if err := json.Unmarshal(cliOutput, &cliResult); err != nil {
		t.Fatalf("decode CLI status: %v; payload=%s", err, cliOutput)
	}
	if wire.Result.Status != cliResult.Status || wire.Result.HomeSummary != cliResult.HomeSummary {
		t.Fatalf("MCP/CLI semantic parity mismatch: MCP=%#v CLI=%#v", wire.Result, cliResult)
	}
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatalf("skills/list = %#v, %v", listed, err)
	}
	findSkillEntryByName(t, listed.Skills, "review-skill")
	findSkillEntryByName(t, listed.Skills, systemskills.CuratorSkillID)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), root) {
		t.Fatalf("stderr leaked an absolute workspace path: %s", stderr.String())
	}
}

func TestStdioEOFMalformedAndFrameLimit(t *testing.T) {
	root := newMCPWorkspace(t)
	binary := buildSkillHub(t)

	t.Run("clean EOF", func(t *testing.T) {
		command := exec.Command(binary, "mcp", "serve", "--workspace", root)
		stdin, _ := command.StdinPipe()
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		_ = stdin.Close()
		if err := command.Wait(); err != nil {
			t.Fatalf("EOF exit: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("unexpected non-protocol stdout: %q", stdout.String())
		}
	})

	t.Run("clean signal", func(t *testing.T) {
		command := exec.Command(binary, "mcp", "serve", "--workspace", root)
		stdin, _ := command.StdinPipe()
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
		if err := command.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		_ = stdin.Close()
		if err := command.Wait(); err != nil {
			t.Fatalf("signal exit: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("signal emitted non-protocol stdout: %q", stdout.String())
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "mcp", "serve", "--workspace", root)
		stdin, _ := command.StdinPipe()
		stdout, _ := command.StdoutPipe()
		var stderr bytes.Buffer
		command.Stderr = &stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		if _, err := stdin.Write([]byte("{malformed}\n")); err != nil {
			t.Fatal(err)
		}
		line, readErr := bufio.NewReader(stdout).ReadBytes('\n')
		if readErr == nil {
			var response map[string]any
			if json.Unmarshal(line, &response) != nil || response["error"] == nil {
				t.Fatalf("malformed frame response is not JSON-RPC: %q", line)
			}
		} else if !errors.Is(readErr, io.EOF) || len(line) != 0 {
			t.Fatalf("malformed frame read = %q, %v; expected SDK transport close with no partial stdout", line, readErr)
		}
		_ = stdin.Close()
		if err := command.Wait(); err == nil {
			t.Fatal("malformed frame unexpectedly exited successfully")
		}
		if readErr != nil && !strings.Contains(stderr.String(), "server session ended with error") {
			t.Fatalf("SDK transport close lacked stderr evidence: %q", stderr.String())
		}
		if ctx.Err() != nil {
			t.Fatal("malformed frame left a long-lived subprocess")
		}
	})

	t.Run("bounded frame", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "mcp", "serve", "--workspace", root)
		stdin, _ := command.StdinPipe()
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		_, _ = stdin.Write(bytes.Repeat([]byte{'x'}, MaxFrameBytes+1))
		_ = stdin.Close()
		err := command.Wait()
		if err == nil {
			t.Fatal("oversized frame unexpectedly exited successfully")
		}
		if ctx.Err() != nil {
			t.Fatal("oversized frame left a long-lived subprocess")
		}
	})
}

func TestMultipleStdioProcessesReadAcrossApply(t *testing.T) {
	root := newMCPWorkspace(t)
	binary := buildSkillHub(t)
	entries, _, err := (app.DistributionService{}).ListSkills(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	workspaceEntry := findDistributedSkillByID(t, entries, "review-skill")
	oldURI := workspaceEntry.URI
	wantDigest := workspaceEntry.Resources[0].Digest

	sessions := make([]*mcp.ClientSession, 0, 2)
	for range 2 {
		client := mcp.NewClient(&mcp.Implementation{Name: "concurrent-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
		var stderr bytes.Buffer
		command := exec.Command(binary, "mcp", "serve", "--workspace", root)
		command.Stderr = &stderr
		session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
		if err != nil {
			t.Fatalf("connect: %v; stderr=%s", err, stderr.String())
		}
		sessions = append(sessions, session)
		t.Cleanup(func() { _ = session.Close() })
	}

	var wait sync.WaitGroup
	errorsFound := make(chan error, 64)
	for _, session := range sessions {
		wait.Add(1)
		go func(session *mcp.ClientSession) {
			defer wait.Done()
			for range 12 {
				result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: oldURI})
				if err != nil {
					var rpcErr *jsonrpc.Error
					if errors.As(err, &rpcErr) && rpcErr.Code == jsonrpc.CodeInvalidParams && strings.Contains(string(rpcErr.Data), "snapshot_expired") {
						continue
					}
					errorsFound <- err
					return
				}
				contents := []byte(result.Contents[0].Text)
				sum := sha256.Sum256(contents)
				if "sha256:"+hex.EncodeToString(sum[:]) != wantDigest {
					errorsFound <- errors.New("mixed resource bytes observed")
					return
				}
			}
		}(session)
	}

	service := app.SkillService{}
	updated := "---\nname: review-skill\ndescription: Review changed code safely.\nlicense: Apache-2.0\n---\n\n# Review\n\nUpdated under the workspace lock.\n"
	preview, err := service.PreviewSkillUpdate(t.Context(), root, "review-skill", skill.UpdateInput{Content: []byte(updated), SetContent: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(t.Context(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("apply = %#v, %v", result, err)
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	// Existing clients may validly serve their digest-verified immutable URI from
	// the advertised cache. A fresh process must observe deterministic expiry.
	client := mcp.NewClient(&mcp.Implementation{Name: "expiry-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	var stderr bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &stderr
	fresh, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("fresh connect: %v; stderr=%s", err, stderr.String())
	}
	defer fresh.Close()
	_, err = fresh.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: oldURI})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "snapshot_expired") {
		t.Fatalf("old read after apply = %v", err)
	}
}

func buildSkillHub(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "skillhub")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-o", binary, "./cmd/skillhub")
	command.Dir = filepath.Clean(filepath.Join("..", "..", ".."))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build skillhub: %v\n%s", err, output)
	}
	return binary
}
