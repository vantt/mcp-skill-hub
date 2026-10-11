package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceWatchRemoteValidationAndJSON(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	runCLI(t, "skill", "create", "--workspace", root, "--id", "test-skill", "--collection", "default", "--name", "Test Skill", "--description", "Test skill description", "--yes")
	// 1. Missing locator fails
	code, _, stderr := runCLI(t, "source", "watch", "--workspace", root)
	if code != 2 {
		t.Fatalf("expected exit 2 on missing locator, got %d", code)
	}
	if !strings.Contains(stderr, "watch requires one locator") {
		t.Errorf("expected missing locator error, got: %s", stderr)
	}

	// 2. Non-existent remote repo returns exit 2 with actionable error
	locator := "https://github.com/example/nonexistent-skillhub-test-repo"
	code, _, stderr = runCLI(t, "source", "watch", locator, "--workspace", root, "--cadence", "daily", "--skill-id", "test-skill")
	if code != 2 {
		t.Fatalf("expected exit 2 on nonexistent repo, got %d", code)
	}
	if !strings.Contains(stderr, "Repository not found") && !strings.Contains(stderr, "operation failed") {
		t.Errorf("expected repository error in stderr: %s", stderr)
	}

	// 3. JSON error output is a valid envelope without prose on stdout
	var stdout, jsonErr bytes.Buffer
	code = Run([]string{"source", "watch", locator, "--workspace", root, "--json", "--skill-id", "test-skill"}, &stdout, &jsonErr)
	if code != 2 {
		t.Fatalf("expected exit 2 on JSON error, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"status":"error"`) || !strings.Contains(stdout.String(), `"schema_version":"1"`) {
		t.Errorf("expected valid JSON error envelope on stdout, got: %s", stdout.String())
	}
}
func TestSourceWatchRejectsLocalFolders(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, _, stderr := runCLI(t, "source", "watch", "./local-folder", "--workspace", root)
	if code != 2 {
		t.Fatalf("expected exit 2 on local watch, got %d", code)
	}
	if !strings.Contains(stderr, "local folders are machine-specific") && !strings.Contains(stderr, "local_watch_unsupported") {
		t.Errorf("expected local watch error in stderr: %s", stderr)
	}
}

func TestSourceCheckIsTheOnlyCheckCommand(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)

	var stdoutTop, stderrTop bytes.Buffer
	if code := Run([]string{"check", "--workspace", root, "--all"}, &stdoutTop, &stderrTop); code != 2 || !strings.Contains(stderrTop.String(), `The command "check" is not supported.`) {
		t.Errorf("top-level check: exit %d, stderr %q", code, stderrTop.String())
	}
	var stdoutSub, stderrSub bytes.Buffer
	if code := Run([]string{"source", "check", "--workspace", root, "--all"}, &stdoutSub, &stderrSub); code != 0 || !strings.Contains(stdoutSub.String(), "Source checks completed") {
		t.Errorf("source check: exit %d, stdout %q, stderr %q", code, stdoutSub.String(), stderrSub.String())
	}
}
