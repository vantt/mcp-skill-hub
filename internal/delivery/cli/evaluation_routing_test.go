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
	report := assertRoutingEvalJSONEnvelope(t, stdout.Bytes(), app.StatusOK)
	if stderr.Len() != 0 {
		t.Fatalf("JSON success wrote stderr: %s", stderr.String())
	}
	if report.PositiveCases != 9 { // 3 skills * 3 examples each = 9
		t.Fatalf("expected 9 positive cases, got %d", report.PositiveCases)
	}
	if report.CounterCases != 6 { // 3 skills * 2 counter examples each = 6
		t.Fatalf("expected 6 counter cases, got %d", report.CounterCases)
	}

	// Passing thresholds retain a successful envelope and exit code.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"eval", "routing", "--workspace", workspace, "--min-recall", "0.0", "--max-fpr", "1.0", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0 on passing thresholds, got %d (stdout=%s, stderr=%s)", code, stdout.String(), stderr.String())
	}
	assertRoutingEvalJSONEnvelope(t, stdout.Bytes(), app.StatusOK)

	// An undefined no-skill recall deterministically fails its threshold.
	// JSON must expose that outcome without discarding the measured report.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"eval", "routing", "--workspace", workspace, "--min-no-skill-recall", "1.0", "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1 on failed threshold, got %d (stdout=%s, stderr=%s)", code, stdout.String(), stderr.String())
	}
	failedReport := assertRoutingEvalJSONEnvelope(t, stdout.Bytes(), app.StatusActionRequired)
	if failedReport.PositiveCases != report.PositiveCases || failedReport.CounterCases != report.CounterCases || failedReport.NoSkillRecall != nil {
		t.Fatalf("threshold failure lost metrics: %+v", failedReport)
	}
	var failedResult app.Result
	if err := json.Unmarshal(stdout.Bytes(), &failedResult); err != nil {
		t.Fatal(err)
	}
	if len(failedResult.Warnings) != 1 || failedResult.Warnings[0].Code != "routing_threshold_failed" {
		t.Fatalf("threshold failure warnings=%+v", failedResult.Warnings)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON threshold failure wrote stderr: %s", stderr.String())
	}

	// Human output and its threshold diagnostics remain unchanged.
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"eval", "routing", "--workspace", workspace, "--min-no-skill-recall", "1.0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1 on human threshold failure, got %d", code)
	}

	// 3. Invalid argument exit code 2
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"eval", "routing", "--workspace", workspace, "--min-precision", "invalid-float", "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2 for invalid float, got %d", code)
	}
	var invalidResult app.Result
	if err := json.Unmarshal(stdout.Bytes(), &invalidResult); err != nil {
		t.Fatalf("unmarshal invalid request JSON: %v", err)
	}
	if invalidResult.SchemaVersion != app.ResultSchemaVersion || invalidResult.Status != app.StatusError || invalidResult.Error == nil {
		t.Fatalf("invalid request envelope=%+v", invalidResult)
	}
}

func assertRoutingEvalJSONEnvelope(t *testing.T, data []byte, wantStatus app.Status) app.RoutingEvalReport {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal routing JSON: %v", err)
	}
	for _, key := range []string{"schema_version", "status", "summary", "items", "suggested_actions", "warnings", "error", "metrics"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("routing envelope missing %q: %s", key, data)
		}
	}
	for _, key := range []string{"total_cases", "positive_cases", "counter_cases", "no_skill_cases", "precision", "recall", "no_skill_recall", "no_skill_precision", "false_positive_rate", "skill_metrics", "failures"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("routing metric %q leaked outside metrics: %s", key, data)
		}
	}
	var result app.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != app.ResultSchemaVersion || result.Status != wantStatus || result.Summary == "" || result.Error != nil {
		t.Fatalf("routing envelope=%+v, want status=%s with no operational error", result, wantStatus)
	}
	if result.Items == nil || result.SuggestedActions == nil || result.Warnings == nil {
		t.Fatalf("routing envelope arrays must not be null: %s", data)
	}
	if wantStatus == app.StatusOK && len(result.Warnings) != 0 {
		t.Fatalf("successful routing warnings=%+v", result.Warnings)
	}
	var report app.RoutingEvalReport
	if err := json.Unmarshal(fields["metrics"], &report); err != nil {
		t.Fatalf("unmarshal nested metrics: %v", err)
	}
	if report.TotalCases != report.PositiveCases+report.CounterCases+report.NoSkillCases || len(report.SkillMetrics) != 3 {
		t.Fatalf("incomplete routing metrics: %+v", report)
	}
	return report
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
