package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestCurationTelemetryServiceDerivesPinsAndNeverMutatesCanonicalState(t *testing.T) {
	root := newResolverWorkspace(t)
	before, err := (WorkspaceService{}).GetCurationDiff(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	turns, confirmations, prompts, batch, duration := int64(1), int64(0), int64(1), int64(3), int64(900)
	finalized, recovered, noise := true, true, int64(0)
	input := CurationSessionInput{
		SchemaVersion: "1", EventID: "evt_app_curation", Status: "completed", Basis: telemetry.CurationBasisHostReported,
		TurnsToNextAction: &turns, UnnecessaryConfirmations: &confirmations,
		PromptsPerBatch: &prompts, BatchSize: &batch, AutoFinalized: &finalized,
		RecoveryCompleted: &recovered, RoutineGitNoise: &noise, DurationMS: &duration,
	}
	service := CurationTelemetryService{}
	first, err := service.RecordSession(t.Context(), root, input)
	if err != nil || first.Deduplicated || first.CanonicalMutated || first.PolicyMutated {
		t.Fatalf("first record = %+v, %v", first, err)
	}
	second, err := service.RecordSession(t.Context(), root, input)
	if err != nil || !second.Deduplicated || second.CanonicalMutated || second.PolicyMutated {
		t.Fatalf("duplicate record = %+v, %v", second, err)
	}
	after, err := (WorkspaceService{}).GetCurationDiff(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatalf("telemetry changed canonical Git state\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}

	exportPath := filepath.Join(t.TempDir(), "curation.jsonl")
	exported, err := (TelemetryService{}).Export(t.Context(), root, exportPath)
	if err != nil || exported.Events != 1 || exported.Skipped != 0 {
		t.Fatalf("export = %+v, %v", exported, err)
	}
	contents, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{contents, "task", "conversation"} {
		if bytes.Contains(data, []byte(prohibited)) {
			t.Fatalf("export leaked %q: %s", prohibited, data)
		}
	}
	for _, expected := range []string{`"event_type":"curation.session_completed"`, `"catalog_snapshot":"sha256:`, `"policy_revision":"sha256:`, `"client":{"name":"skillhub-mcp","version":"1"}`} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatalf("export omitted %q: %s", expected, data)
		}
	}

	changed := input
	changed.BatchSize = int64Pointer(4)
	if _, err := service.RecordSession(t.Context(), root, changed); !errors.Is(err, telemetry.ErrCurationSessionConflict) {
		t.Fatalf("measurement conflict = %v", err)
	}
}

func TestCurationTelemetryServiceRequiresExplicitObservedValues(t *testing.T) {
	root := newResolverWorkspace(t)
	service := CurationTelemetryService{}
	for name, input := range map[string]CurationSessionInput{
		"missing status": {SchemaVersion: "1", EventID: "evt_missing_status", Basis: telemetry.CurationBasisHostReported},
		"missing basis":  {SchemaVersion: "1", EventID: "evt_missing_basis", Status: "completed"},
		"wrong schema":   {SchemaVersion: "2", EventID: "evt_schema", Status: "completed", Basis: telemetry.CurationBasisHostReported},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.RecordSession(t.Context(), root, input); err == nil {
				t.Fatal("invalid observation was accepted")
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }
