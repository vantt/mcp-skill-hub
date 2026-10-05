package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestEvaluationRoutingCLIExitCodesAndReporting(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate evaluation test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	fixtureRoot := filepath.Join(repositoryRoot, "testdata", "evaluation")
	workspace := filepath.Join(t.TempDir(), "workspace")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", workspace, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	copyEvaluationFixtureTree(t, filepath.Join(fixtureRoot, "workspace-overlay"), workspace)
	if err := os.RemoveAll(filepath.Join(workspace, "runtime", "catalog")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", workspace, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	// 1. Success exit code 0 without thresholds
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"eval", "routing", "--workspace", workspace, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stdout=%s, stderr=%s)", code, stdout.String(), stderr.String())
	}
	var report app.RoutingEvalReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report JSON: %v", err)
	}
	if report.PositiveCases != 9 { // 3 skills * 3 examples each = 9
		t.Fatalf("expected 9 positive cases, got %d", report.PositiveCases)
	}
	if report.CounterCases != 6 { // 3 skills * 2 counter examples each = 6
		t.Fatalf("expected 6 counter cases, got %d", report.CounterCases)
	}

	// 2. Failed threshold exit code 1
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"eval", "routing", "--workspace", workspace, "--min-recall", "1.0"}, &stdout, &stderr)
	if report.Recall != nil && *report.Recall < 1.0 {
		if code != 1 {
			t.Fatalf("expected code 1 on failed threshold, got %d", code)
		}
	}

	// 3. Invalid argument exit code 2
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"eval", "routing", "--workspace", workspace, "--min-precision", "invalid-float"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2 for invalid float, got %d", code)
	}
}

func TestEvaluationRoutingCLIHelpListsRouting(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "eval"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0 for help eval, got %d", code)
	}
	if !strings.Contains(stdout.String(), "routing") {
		t.Fatalf("help eval should list routing subcommand, got:\n%s", stdout.String())
	}
}
