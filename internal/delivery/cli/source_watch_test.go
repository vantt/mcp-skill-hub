package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceWatchRemoteValidationAndJSON(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)

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
	code, _, stderr = runCLI(t, "source", "watch", locator, "--workspace", root, "--cadence", "daily")
	if code != 2 {
		t.Fatalf("expected exit 2 on nonexistent repo, got %d", code)
	}
	if !strings.Contains(stderr, "Repository not found") && !strings.Contains(stderr, "operation failed") {
		t.Errorf("expected repository error in stderr: %s", stderr)
	}

	// 3. JSON error output is a valid envelope without prose on stdout
	var stdout, jsonErr bytes.Buffer
	code = Run([]string{"source", "watch", locator, "--workspace", root, "--json"}, &stdout, &jsonErr)
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

func TestSourceCheckAliasParity(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)

	// Compare `check --all` and `source check --all`
	var stdoutTop, stderrTop bytes.Buffer
	codeTop := Run([]string{"check", "--workspace", root, "--all"}, &stdoutTop, &stderrTop)

	var stdoutSub, stderrSub bytes.Buffer
	codeSub := Run([]string{"source", "check", "--workspace", root, "--all"}, &stdoutSub, &stderrSub)

	if codeTop != codeSub {
		t.Errorf("exit codes differ: top=%d, sub=%d", codeTop, codeSub)
	}
	if stdoutTop.String() != stdoutSub.String() {
		t.Errorf("stdout differs:\ntop=%s\nsub=%s", stdoutTop.String(), stdoutSub.String())
	}
	if stderrTop.String() != stderrSub.String() {
		t.Errorf("stderr differs:\ntop=%s\nsub=%s", stderrTop.String(), stderrSub.String())
	}
}
