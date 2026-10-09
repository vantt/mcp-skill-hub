package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestTelemetryCLICases(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)

	var stdout, stderr bytes.Buffer

	// 1. Initial status: disabled
	if code := Run([]string{"telemetry", "cases", "status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal: disabled") {
		t.Fatalf("expected disabled, got %s", stdout.String())
	}

	// 2. Enable case journal
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "enable", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("enable code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal enabled.") {
		t.Fatalf("expected enabled confirmation, got %s", stdout.String())
	}

	// 3. Status after enable
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal: enabled") {
		t.Fatalf("expected enabled, got %s", stdout.String())
	}

	// 4. Record a case and list it
	telService := app.TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	err = recorder.RecordCase(t.Context(), telemetry.CaseRecord{
		ResolutionID:    "res_cli_case_1",
		SessionHash:     "sess_cli_case",
		OccurredAt:      time.Now().UTC(),
		Kind:            "override",
		Client:          telemetry.Client{Name: "claude-code"},
		CatalogSnapshot: "sha256:cat",
		Task:            map[string]any{"description": "my cli test task description"},
		Chosen:          "review-skill",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Close(t.Context())

	// 5. Query cases via CLI with --json
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("cases --json code=%d: %s", code, stderr.String())
	}
	var cases []telemetry.CaseRecord
	if err := json.Unmarshal(stdout.Bytes(), &cases); err != nil {
		t.Fatalf("unmarshal cases json: %v\nOutput: %s", err, stdout.String())
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].CaseID == "" {
		t.Fatal("expected non-empty case_id in json output")
	}
	if cases[0].Kind != "override" {
		t.Fatalf("expected kind override, got %q", cases[0].Kind)
	}
	if task, _ := cases[0].Task["description"].(string); task != "my cli test task description" {
		t.Fatalf("expected task description, got %q", task)
	}

	// 6. Disable case journal
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "disable", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("disable code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal disabled.") {
		t.Fatalf("expected disabled confirmation, got %s", stdout.String())
	}
}
