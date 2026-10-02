package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetryCLIHealthPreviewExportAndPurge(t *testing.T) {
	t.Parallel()
	root, requestPath := telemetryCLIWorkspace(t)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"resolve", "--workspace", root, "--request", requestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("resolve=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "health", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("health=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var health struct {
		State   string `json:"state"`
		Written uint64 `json:"written"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health.State != "healthy" {
		t.Fatalf("health=%+v", health)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "preview", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("preview=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var preview telemetryPreviewResult
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Events < 2 || !strings.Contains(preview.JSONL, `"content_mode":"none"`) {
		t.Fatalf("preview=%+v", preview)
	}
	if strings.Contains(preview.JSONL, "inspect a patch") {
		t.Fatalf("preview leaked task content: %s", preview.JSONL)
	}

	output := filepath.Join(t.TempDir(), "export.jsonl")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "export", "--workspace", root, "--output", output}, &stdout, &stderr); code != 0 {
		t.Fatalf("export preview=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preview wrote output: %v", err)
	}
	if !strings.Contains(stdout.String(), `"privacy":{"content_mode":"none"`) {
		t.Fatalf("preview was not JSONL: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "export", "--workspace", root, "--output", output, "--yes", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("export=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	exported, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(exported) != preview.JSONL {
		t.Fatalf("export differs from preview\nexport=%s\npreview=%s", exported, preview.JSONL)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "purge", "--workspace", root}, &stdout, &stderr); code != 2 {
		t.Fatalf("purge without yes=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "purge", "--workspace", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("purge=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := Run([]string{"telemetry", "preview", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("preview after purge=%d", code)
	}
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil || preview.Events != 0 || preview.JSONL != "" {
		t.Fatalf("preview after purge=%+v err=%v", preview, err)
	}
}

func TestTelemetryCLIRejectsUnsafeAndUnboundedArguments(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)
	existing := filepath.Join(t.TempDir(), "existing.jsonl")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.jsonl")
	if err := os.Symlink(existing, link); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"telemetry", "export", "--workspace", root, "--output", link, "--yes", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("symlink output code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	contents, err := os.ReadFile(existing)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("symlink target changed: %q %v", contents, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "export", "--workspace", root, "--output", existing, "--yes", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("authorized replacement code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	contents, err = os.ReadFile(existing)
	if err != nil || string(contents) != "" {
		t.Fatalf("authorized replacement failed: %q %v", contents, err)
	}

	if code := Run([]string{"telemetry", "health", "--workspace", root, "--workspace", root, "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("duplicate flag code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "health", "--workspace", strings.Repeat("x", maxDeliveryValueSize+1), "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unbounded value code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestResolveCLIIgnoresTelemetryStorageFailure(t *testing.T) {
	t.Parallel()
	root, requestPath := telemetryCLIWorkspace(t)
	database := filepath.Join(root, "runtime", "telemetry.db")
	if err := os.WriteFile(database, []byte("corrupt telemetry"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"resolve", "--workspace", root, "--request", requestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("telemetry changed resolve result: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status":"resolved"`)) {
		t.Fatalf("unexpected resolver response: %s", stdout.String())
	}
}

func telemetryCLIWorkspace(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	writeResolverFixture(t, root, "skills/core/code-review/skill.meta.yaml", "schema_version: 1\nid: code-review\nname: Code Review\nstatus: active\ndescription: Review code changes for defects.\naliases: [patch inspection]\nrouting:\n  operations: [review]\n  triggers: [review code changes pull requests]\n  not_for: [write marketing prose]\n  min_scope: multi_step\nquality:\n  reviewed: true\n")
	writeResolverFixture(t, root, "skills/core/code-review/SKILL.md", "---\nname: code-review\ndescription: Review code changes for defects.\n---\n\n# Code Review\nInspect evidence.\n")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild=%d: %s %s", code, stdout.String(), stderr.String())
	}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	request := []byte(`{"schema_version":"1","request_id":"req-telemetry","task":{"description":"inspect a patch for code defects","scope":"multi_step"},"operation":"review"}`)
	if err := os.WriteFile(requestPath, request, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, requestPath
}

func TestTelemetryHealthWithCorruptStoreSuggestsPurgeNotWorkspaceRepair(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "runtime", "telemetry.db"), []byte("corrupt telemetry"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"telemetry", "health", "--workspace", root, "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("health=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result struct {
		Error struct {
			Code   string `json:"code"`
			Render struct {
				Fix string `json:"FIX"`
			} `json:"render"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode %s: %v", stdout.String(), err)
	}
	if result.Error.Code == "workspace_invalid" || !strings.Contains(result.Error.Render.Fix, "telemetry purge") || !strings.Contains(result.Error.Render.Fix, root) {
		t.Fatalf("error = %+v", result.Error)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "purge", "--workspace", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("purge=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := Run([]string{"telemetry", "health", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("health after purge=%d stdout=%s", code, stdout.String())
	}
}

func TestTelemetryHealthReportsCountersFromEarlierCommands(t *testing.T) {
	t.Parallel()
	root, requestPath := telemetryCLIWorkspace(t)
	var stdout, stderr bytes.Buffer
	for i := 0; i < 2; i++ {
		stdout.Reset()
		if code := Run([]string{"resolve", "--workspace", root, "--request", requestPath, "--json"}, &stdout, &stderr); code != 0 {
			t.Fatalf("resolve=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
	}
	stdout.Reset()
	if code := Run([]string{"telemetry", "health", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("health=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var health struct {
		Written uint64 `json:"written"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health.Written < 2 {
		t.Fatalf("health.Written = %d, want cumulative count from earlier commands", health.Written)
	}
}
