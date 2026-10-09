package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestTelemetryCLIBaselineEmpty(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"telemetry", "baseline", "--workspace", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "=== Telemetry Baseline") {
		t.Errorf("expected header, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "No telemetry events found") {
		t.Errorf("expected no events line, got %s", stdout.String())
	}

	stdout.Reset()
	code = Run([]string{"telemetry", "baseline", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0 for --json, got %d: %s", code, stderr.String())
	}
	var rep app.BaselineReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if len(rep.Buckets) != 0 {
		t.Errorf("expected 0 buckets, got %d", len(rep.Buckets))
	}
}

func TestTelemetryCLIBaselineSufficiencyAndWrite(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)
	telService := app.TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	snap := "sha256:baseline_snapshot_1"
	client := "claude-code"

	// Record 2 resolved resolutions and 2 no_skill resolutions
	for i := range 2 {
		recorder.Record(telemetry.Event{
			Version:         telemetry.EventVersion,
			ID:              fmt.Sprintf("evt_res_%d", i),
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      now.Add(-time.Duration(10-i) * time.Hour),
			CatalogSnapshot: snap,
			PolicyRevision:  "sha256:policy_rev",
			Client:          telemetry.Client{Name: client},
			SessionIDHash:   fmt.Sprintf("sess_res_%d", i),
			ResolutionID:    fmt.Sprintf("res_ok_%d", i),
			Payload: map[string]any{
				"status":       "resolved",
				"top_skill_id": "skill-1",
			},
		})
	}
	for i := range 2 {
		recorder.Record(telemetry.Event{
			Version:         telemetry.EventVersion,
			ID:              fmt.Sprintf("evt_noskill_%d", i),
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      now.Add(-time.Duration(5-i) * time.Hour),
			CatalogSnapshot: snap,
			PolicyRevision:  "sha256:policy_rev",
			Client:          telemetry.Client{Name: client},
			SessionIDHash:   fmt.Sprintf("sess_noskill_%d", i),
			ResolutionID:    fmt.Sprintf("res_noskill_%d", i),
			Payload: map[string]any{
				"status": "no_skill",
			},
		})
	}
	// Record 1 load with attribution unsolicited
	recorder.Record(telemetry.Event{
		Version:         telemetry.EventVersion,
		ID:              "evt_load_1",
		Type:            telemetry.EventSkillLoaded,
		OccurredAt:      now.Add(-time.Hour),
		CatalogSnapshot: snap,
		PolicyRevision:  "sha256:policy_rev",
		Client:          telemetry.Client{Name: client},
		SessionIDHash:   "sess_other",
		Payload: map[string]any{
			"basis":         telemetry.LoadBasisServerObserved,
			"skill_id":      "skill-1",
			"resource_kind": "entrypoint",
			"surface":       "skill_get",
			"attribution":   "unsolicited",
		},
	})

	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	recorder.Close(t.Context())

	outPath := filepath.Join(t.TempDir(), "reports", "baseline.json")
	var stdout, stderr bytes.Buffer

	// Run baseline with --min-chains 2 --write <outPath>
	code := Run([]string{
		"telemetry", "baseline",
		"--workspace", root,
		"--min-chains", "2",
		"--write", outPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}

	terminalOut := stdout.String()
	if !strings.Contains(terminalOut, "sufficient (resolved: 2/2, no_skill: 2/2)") {
		t.Errorf("expected sufficient verdict in terminal output, got:\n%s", terminalOut)
	}
	if !strings.Contains(terminalOut, "unsolicited_share: 100.0% (1/1)") {
		t.Errorf("expected unsolicited share in terminal output, got:\n%s", terminalOut)
	}
	if !strings.Contains(terminalOut, "Baseline report written to") || !strings.Contains(terminalOut, "baseline.json") {
		t.Errorf("expected written path notice, got:\n%s", terminalOut)
	}

	// Verify file was written
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected file to exist at %s: %v", outPath, err)
	}

	var rep app.BaselineReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("failed to unmarshal written JSON: %v", err)
	}
	if len(rep.Buckets) != 1 {
		t.Fatalf("expected 1 bucket in report, got %d", len(rep.Buckets))
	}
	b := rep.Buckets[0]
	if b.Sufficiency.Verdict != "sufficient" {
		t.Errorf("expected verdict sufficient, got %q", b.Sufficiency.Verdict)
	}
	if b.UnsolicitedShare.Rate == nil || *b.UnsolicitedShare.Rate != 1.0 {
		t.Errorf("expected unsolicited share 1.0, got %v", b.UnsolicitedShare.Rate)
	}

	// PRIVACY VERIFICATION: Confirm JSON contains NO task text, task description, or case content
	fileContent := string(data)
	forbiddenTokens := []string{
		"task", "description", "request", "conversation", "prompt", "case_id", "payload_json",
	}
	for _, tok := range forbiddenTokens {
		if strings.Contains(fileContent, `"`+tok+`"`) {
			t.Fatalf("privacy violation: written baseline JSON contains forbidden key %q:\n%s", tok, fileContent)
		}
	}
}

func TestTelemetryCLIBaselineSnapshotResetAndSuperseded(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)
	telService := app.TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	// Older snapshot
	recorder.Record(telemetry.Event{
		Version:         telemetry.EventVersion,
		ID:              "evt_old",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-48 * time.Hour),
		CatalogSnapshot: "sha256:old_snapshot",
		PolicyRevision:  "sha256:policy_rev",
		Client:          telemetry.Client{Name: "claude-code"},
		ResolutionID:    "res_old",
		Payload:         map[string]any{"status": "resolved", "top_skill_id": "skill-1"},
	})
	// Newer snapshot
	recorder.Record(telemetry.Event{
		Version:         telemetry.EventVersion,
		ID:              "evt_new",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-2 * time.Hour),
		CatalogSnapshot: "sha256:new_snapshot",
		PolicyRevision:  "sha256:policy_rev",
		Client:          telemetry.Client{Name: "claude-code"},
		ResolutionID:    "res_new",
		Payload:         map[string]any{"status": "resolved", "top_skill_id": "skill-2"},
	})

	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	recorder.Close(t.Context())

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"telemetry", "baseline",
		"--workspace", root,
		"--min-chains", "5",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}

	var rep app.BaselineReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if rep.ActiveSnapshot != "sha256:new_snapshot" {
		t.Errorf("expected active snapshot 'sha256:new_snapshot', got %q", rep.ActiveSnapshot)
	}
	if len(rep.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(rep.Buckets))
	}

	// Verify one baseline, one superseded
	var foundBaseline, foundSuperseded bool
	for _, b := range rep.Buckets {
		if b.CatalogSnapshot == "sha256:new_snapshot" && b.IsBaselineWindow && b.Status == "baseline" {
			foundBaseline = true
		}
		if b.CatalogSnapshot == "sha256:old_snapshot" && !b.IsBaselineWindow && b.Status == "superseded" {
			foundSuperseded = true
		}
	}
	if !foundBaseline {
		t.Error("newest snapshot was not marked baseline")
	}
	if !foundSuperseded {
		t.Error("older snapshot was not marked superseded")
	}
}
