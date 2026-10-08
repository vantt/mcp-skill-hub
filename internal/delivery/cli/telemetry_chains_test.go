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

func TestTelemetryCLIChainsAndFunnelCuts(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)

	var stdout, stderr bytes.Buffer

	// 1. Initial chains on empty workspace: should exit 0 and report none found
	if code := Run([]string{"telemetry", "chains", "--workspace", root, "--since", "7d"}, &stdout, &stderr); code != 0 {
		t.Fatalf("chains 7d code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "No disagreement chains found") {
		t.Fatalf("expected none found message, got: %s", stdout.String())
	}

	// 2. Initial chains with --json: should exit 0 with empty JSON list
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "chains", "--workspace", root, "--since", "7d", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("chains json code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var emptyChains []app.DisagreementChain
	if err := json.Unmarshal(stdout.Bytes(), &emptyChains); err != nil {
		t.Fatalf("unmarshal empty chains json: %v", err)
	}

	// 3. Inject an override chain into telemetry.db
	telService := app.TelemetryService{}
	rec, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rec.Record(telemetry.Event{
		ID:              "evt_override_cli",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-10 * time.Minute),
		SessionIDHash:   "sess_cli_disagree",
		ResolutionID:    "res_cli_1",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "claude-code", Version: "1.2"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
			"operation":             "review",
		},
	})
	rec.Record(telemetry.Event{
		ID:              "evt_override_cli_load",
		Type:            telemetry.EventSkillLoaded,
		OccurredAt:      now.Add(-5 * time.Minute),
		SessionIDHash:   "sess_cli_disagree",
		ResolutionID:    "res_cli_1",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "claude-code", Version: "1.2"},
		Payload: map[string]any{
			"skill_id": "other-tool", "surface": "skill_get", "resource_kind": "entrypoint",
			"basis": telemetry.LoadBasisServerObserved, "attribution": "override",
		},
	})
	rec.Flush(t.Context())
	_ = rec.Close(t.Context())

	// 4. Chains with --kind override
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "chains", "--workspace", root, "--since", "7d", "--kind", "override", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("chains override code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var disagreementChains []app.DisagreementChain
	if err := json.Unmarshal(stdout.Bytes(), &disagreementChains); err != nil {
		t.Fatalf("unmarshal disagreement chains json: %v", err)
	}
	if len(disagreementChains) != 1 {
		t.Fatalf("expected 1 override chain, got %d", len(disagreementChains))
	}
	if disagreementChains[0].Kind != "override" || disagreementChains[0].LoadedSkill != "other-tool" {
		t.Fatalf("unexpected chain data: %+v", disagreementChains[0])
	}

	// 5. Funnel with --by client
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--by", "client", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel by client code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var funnelReport app.FunnelReport
	if err := json.Unmarshal(stdout.Bytes(), &funnelReport); err != nil {
		t.Fatalf("unmarshal funnel report json: %v", err)
	}
	if len(funnelReport.Cuts) == 0 {
		t.Fatal("expected non-empty cuts by client")
	}
	if funnelReport.Cuts[0].Key != "claude-code" {
		t.Fatalf("cut key = %q, want claude-code", funnelReport.Cuts[0].Key)
	}
	if funnelReport.RawRetentionDays != 30 {
		t.Fatalf("raw_retention_days = %d, want 30", funnelReport.RawRetentionDays)
	}

	// 6. Funnel with --by operation
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--by", "operation", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("funnel by operation code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &funnelReport); err != nil {
		t.Fatalf("unmarshal funnel report json: %v", err)
	}
	if len(funnelReport.Cuts) == 0 {
		t.Fatal("expected non-empty cuts by operation")
	}

	// 7. Funnel with invalid --by returns 2
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "funnel", "--workspace", root, "--by", "invalid"}, &stdout, &stderr); code != 2 {
		t.Fatalf("expected code 2 on invalid --by, got %d", code)
	}

	// 8. Chains with invalid --kind returns 2
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "chains", "--workspace", root, "--kind", "invalid"}, &stdout, &stderr); code != 2 {
		t.Fatalf("expected code 2 on invalid --kind, got %d", code)
	}
}
