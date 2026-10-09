package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestTelemetryCLIFunnel(t *testing.T) {
	t.Parallel()
	root, requestPath := telemetryCLIWorkspace(t)
	var stdout, stderr bytes.Buffer

	// 1. Workspace with no telemetry events: json output, exit 0
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--since", "7d", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel 7d json=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var report app.FunnelReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal funnel report: %v", err)
	}
	if report.Overall == nil || report.Overall.TotalResolutions != 0 || report.Overall.AcceptanceRate != nil {
		t.Fatalf("expected empty overall with nil acceptance rate, got: %+v", report.Overall)
	}
	if report.Window.Days != 8 {
		t.Fatalf("expected 8 days for 7d offset, got %d", report.Window.Days)
	}

	// 2. Human output, exit 0
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--since", "7d"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel 7d human=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	outStr := stdout.String()
	if !strings.Contains(outStr, "Funnel Report") || !strings.Contains(outStr, "Overall Summary") || !strings.Contains(outStr, "Skills") {
		t.Fatalf("unexpected human output: %s", outStr)
	}

	// 3. Trigger a resolution to have data
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"resolve", "--workspace", root, "--request", requestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("resolve=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	// Re-run funnel with json
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel json=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report after resolve: %v", err)
	}
	if report.Overall.TotalResolutions != 1 {
		t.Fatalf("expected 1 resolution, got %d", report.Overall.TotalResolutions)
	}

	// 4. Single skill query: --skill code-review
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--skill", "code-review", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel --skill code-review=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var skillReport app.FunnelReport
	if err := json.Unmarshal(stdout.Bytes(), &skillReport); err != nil {
		t.Fatalf("unmarshal skill report: %v", err)
	}
	if skillReport.Overall != nil {
		t.Fatalf("expected nil Overall for single skill report, got: %+v", skillReport.Overall)
	}
	if skillReport.Skill == nil || skillReport.Skill.SkillID != "code-review" {
		t.Fatalf("expected Skill code-review, got: %+v", skillReport.Skill)
	}

	// Human output for single skill query
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--skill", "code-review"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel single skill human=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "code-review") {
		t.Fatalf("expected code-review in output: %s", stdout.String())
	}

	// 5. Error cases: exit code 2
	// Unknown skill
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--skill", "non-existent-skill", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown skill code=%d, want 2. stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	// Malformed since
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--since", "not-a-date", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("malformed since code=%d, want 2. stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	// Malformed until
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--until", "bad-date", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("malformed until code=%d, want 2. stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	// Since after until
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--since", "2026-10-20", "--until", "2026-10-01", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("since after until code=%d, want 2. stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestTelemetryCLIHelpIncludesFunnel(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"help", "telemetry"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help telemetry code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "funnel") {
		t.Fatalf("help telemetry missing funnel subcommand:\n%s", stdout.String())
	}
}

func TestFunnelAndBaselineRenderUnmeasurableCountersAsUnknown(t *testing.T) {
	t.Parallel()
	root, requestPath := telemetryCLIWorkspace(t)
	var stdout, stderr bytes.Buffer

	// Trigger a resolution so Overall summary renders
	if code := Run([]string{"resolve", "--workspace", root, "--request", requestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("resolve=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--since", "7d"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel human=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	funnelOut := stdout.String()
	if !strings.Contains(funnelOut, "Unlisted Resource Reads:") || !strings.Contains(funnelOut, "unknown (blocked by ReadResource; unverified against pinned manifest)") {
		t.Errorf("funnel expected Unlisted Resource Reads to be unknown, got:\n%s", funnelOut)
	}
	if !strings.Contains(funnelOut, "Unsupported Method Calls:") || !strings.Contains(funnelOut, "unknown (rejected before middleware by go-sdk)") {
		t.Errorf("funnel expected Unsupported Method Calls to be unknown, got:\n%s", funnelOut)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "baseline", "--workspace", root, "--since", "7d"}, &stdout, &stderr); code != 0 {
		t.Fatalf("baseline human=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	baselineOut := stdout.String()
	if !strings.Contains(baselineOut, "unsupported_method_calls:") || !strings.Contains(baselineOut, "unknown (rejected before middleware by go-sdk)") {
		t.Errorf("baseline expected unsupported_method_calls to be unknown, got:\n%s", baselineOut)
	}
	if !strings.Contains(baselineOut, "unlisted_resource_reads:") || !strings.Contains(baselineOut, "unknown (blocked by ReadResource; unverified against pinned manifest)") {
		t.Errorf("baseline expected unlisted_resource_reads to be unknown, got:\n%s", baselineOut)
	}

	// Test --json output matches null and unmeasured status
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--since", "7d", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel json=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var jsonMap map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &jsonMap); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	overall, ok := jsonMap["overall"].(map[string]any)
	if !ok || overall == nil {
		t.Fatalf("expected overall object in json output: %s", stdout.String())
	}
	if overall["unlisted_resource_reads"] != nil {
		t.Errorf("expected null unlisted_resource_reads in JSON, got %v", overall["unlisted_resource_reads"])
	}
	if overall["unlisted_resource_reads_status"] != "unmeasured" {
		t.Errorf("expected unmeasured status, got %v", overall["unlisted_resource_reads_status"])
	}
	if overall["unsupported_method_calls"] != nil {
		t.Errorf("expected null unsupported_method_calls in JSON, got %v", overall["unsupported_method_calls"])
	}
	if overall["unsupported_method_calls_status"] != "unmeasured" {
		t.Errorf("expected unmeasured status, got %v", overall["unsupported_method_calls_status"])
	}
}
