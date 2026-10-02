package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestDistillCLIRecordsSanitizedPrepareAndSubmitEvents(t *testing.T) {
	t.Parallel()
	root := onboardCLIFilesystemSource(t)
	code, stdout, stderr := runCLIForTest([]string{"distill", "prepare", "source-a", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("prepare=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var batch app.DistillBatchResult
	if err := json.Unmarshal([]byte(stdout), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Prepared != 1 || len(batch.Results) != 1 || batch.Results[0].Run == nil {
		t.Fatalf("prepare result: %#v", batch)
	}
	runID := batch.Results[0].Run.ID
	code, stdout, stderr = runCLIForTest([]string{"distill", "start", runID, "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("start=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var started app.DistillRunResult
	if err := json.Unmarshal([]byte(stdout), &started); err != nil {
		t.Fatal(err)
	}
	coverage := make([]distillpkg.CoverageEntry, 0, len(started.Run.ChangedResources))
	for _, resource := range started.Run.ChangedResources {
		coverage = append(coverage, distillpkg.CoverageEntry{Resource: resource.Path, Status: "analyzed", Reason: "private coverage explanation"})
	}
	submission := app.DistillSubmission{Coverage: coverage, Findings: []app.FindingSubmission{}}
	encoded, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	submissionPath := filepath.Join(t.TempDir(), "submission.json")
	if err := os.WriteFile(submissionPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runCLIForTest([]string{"distill", "submit", runID, "--submission", submissionPath, "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("submit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	preview, err := (app.TelemetryService{}).Preview(t.Context(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	jsonl := string(preview.JSONL)
	for _, eventType := range []string{telemetry.EventDistillRunPrepared, telemetry.EventDistillRunSubmitted, telemetry.EventDistillRunFinalized} {
		if !strings.Contains(jsonl, `"event_type":"`+eventType+`"`) {
			t.Fatalf("missing %s event: %s", eventType, jsonl)
		}
	}
	for _, sensitive := range []string{"private coverage explanation", "notes.md", "upstream content"} {
		if strings.Contains(jsonl, sensitive) {
			t.Fatalf("telemetry leaked %q: %s", sensitive, jsonl)
		}
	}
}

func TestDistillCLIBlockedTelemetryPreservesSuccessAndErrorContracts(t *testing.T) {
	t.Parallel()
	normal := newSourceCLIWorkspace(t)
	blocked := newSourceCLIWorkspace(t)
	if err := os.WriteFile(filepath.Join(blocked, "runtime", "telemetry.db"), []byte("blocked telemetry"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"distill", "prepare", "missing-source", "--workspace", "WORKSPACE", "--json"},
		{"distill", "start", "RUN-missing", "--workspace", "WORKSPACE", "--json"},
	} {
		normalArgs := replaceWorkspaceArgument(args, normal)
		blockedArgs := replaceWorkspaceArgument(args, blocked)
		normalCode, normalOut, normalErr := runCLIForTest(normalArgs)
		blockedCode, blockedOut, blockedErr := runCLIForTest(blockedArgs)
		if normalCode != blockedCode || normalOut != blockedOut || normalErr != blockedErr {
			t.Fatalf("telemetry changed command contract\nnormal=(%d,%q,%q)\nblocked=(%d,%q,%q)", normalCode, normalOut, normalErr, blockedCode, blockedOut, blockedErr)
		}
	}
}

func replaceWorkspaceArgument(args []string, workspace string) []string {
	result := append([]string(nil), args...)
	for index, value := range result {
		if value == "WORKSPACE" {
			result[index] = workspace
		}
	}
	return result
}
