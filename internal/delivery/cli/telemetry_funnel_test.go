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
