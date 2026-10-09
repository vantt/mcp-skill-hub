package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCasesFlagAndExportSuccess verifies Item 1:
// with the case-journal flag on, export still succeeds and every event is content_mode none.
func TestCasesFlagAndExportSuccess(t *testing.T) {
	temp := t.TempDir()
	config := Config{
		Path:               filepath.Join(temp, "telemetry.db"),
		WorkspaceRoot:      temp,
		ContentMode:        ContentModeNone,
		CaseJournalEnabled: true,
	}
	recorder, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	// Record a normal telemetry event
	recorder.Record(Event{
		ID:              "evt-test-1",
		Type:            EventSkillLoaded,
		OccurredAt:      time.Now().UTC(),
		Client:          Client{Name: "claude-code"},
		CatalogSnapshot: "sha256:cat",
		PolicyRevision:  "sha256:pol",
		Payload: map[string]any{
			"skill_id":      "demo-skill",
			"resource_kind": "entrypoint",
			"surface":       "skills_get",
			"basis":         LoadBasisServerObserved,
		},
	})

	// Record a case with task description
	err = recorder.RecordCase(t.Context(), CaseRecord{
		OccurredAt:      time.Now().UTC(),
		Kind:            "override",
		Client:          Client{Name: "claude-code"},
		CatalogSnapshot: "sha256:abc",
		Task:            map[string]any{"description": "investigate production outage with secret-token-xyz"},
		Chosen:          "demo-skill",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Flush to ensure event is written
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Verify export still succeeds
	exportPath := filepath.Join(temp, "export.json")
	result, err := recorder.Export(t.Context(), exportPath)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if result.Events != 1 {
		t.Fatalf("expected 1 exported event, got %d", result.Events)
	}

	// Read exported JSONL to confirm content_mode is "none"
	exportBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(exportBytes)), "\n")
	if len(lines) == 0 {
		t.Fatal("empty export")
	}
	var env struct {
		Privacy struct {
			ContentMode string `json:"content_mode"`
		} `json:"privacy"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatal(err)
	}
	if env.Privacy.ContentMode != ContentModeNone {
		t.Fatalf("expected content_mode none, got %q", env.Privacy.ContentMode)
	}
}

// TestTwoSessionsSameTaskTwoCases verifies Item 2 & 3:
// Case key: random case_id primary key, plus session_hash, resolution_id, event_id.
// Two sessions with the same task fingerprint (same resolution_id) both produce cases.
// Store and return case_id, kind, occurred_at, session_hash and resolution_id in JSON.
func TestTwoSessionsSameTaskTwoCases(t *testing.T) {
	temp := t.TempDir()
	config := Config{
		Path:               filepath.Join(temp, "telemetry.db"),
		WorkspaceRoot:      temp,
		ContentMode:        ContentModeNone,
		CaseJournalEnabled: true,
	}
	recorder, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	now := time.Now().UTC()
	sameResID := "res_fingerprint_same_task"

	// Session 1 case
	err = recorder.RecordCase(t.Context(), CaseRecord{
		ResolutionID: sameResID,
		SessionHash:  "session_hash_1",
		EventID:      "evt_1",
		OccurredAt:   now.Add(-time.Minute),
		Kind:         "override",
		Client:       Client{Name: "client_1"},
		Task:         map[string]any{"description": "same task description"},
		Chosen:       "skill_a",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Session 2 case with same resolution_id (same task)
	err = recorder.RecordCase(t.Context(), CaseRecord{
		ResolutionID: sameResID,
		SessionHash:  "session_hash_2",
		EventID:      "evt_2",
		OccurredAt:   now,
		Kind:         "after_no_skill",
		Client:       Client{Name: "client_2"},
		Task:         map[string]any{"description": "same task description"},
		Chosen:       "skill_b",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify both cases exist!
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases for two sessions with same task, got %d", len(cases))
	}
	if cases[0].CaseID == "" || cases[1].CaseID == "" {
		t.Fatal("case_id should not be empty")
	}
	if cases[0].CaseID == cases[1].CaseID {
		t.Fatalf("expected distinct case_ids, got %q and %q", cases[0].CaseID, cases[1].CaseID)
	}

	// Test Item 3: JSON serialization contains case_id, kind, occurred_at, session_hash, resolution_id
	b, err := json.Marshal(cases[0])
	if err != nil {
		t.Fatal(err)
	}
	var jsonMap map[string]any
	if err := json.Unmarshal(b, &jsonMap); err != nil {
		t.Fatal(err)
	}
	for _, requiredField := range []string{"case_id", "kind", "occurred_at", "session_hash", "resolution_id"} {
		if _, ok := jsonMap[requiredField]; !ok {
			t.Errorf("JSON output missing required field %q in %s", requiredField, string(b))
		}
	}
}

// TestRetentionAndPruning verifies Item 4:
// Enforce 90-day retention: PruneCases removes cases older than 90 days.
func TestRetentionAndPruning(t *testing.T) {
	temp := t.TempDir()
	config := Config{
		Path:               filepath.Join(temp, "telemetry.db"),
		WorkspaceRoot:      temp,
		ContentMode:        ContentModeNone,
		CaseJournalEnabled: true,
	}
	recorder, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -95) // 95 days old

	// Insert an old case directly
	err = recorder.RecordCase(t.Context(), CaseRecord{
		CaseID:       "case_old_95d",
		OccurredAt:   oldDate,
		Kind:         "override",
		SessionHash:  "session_old",
		ResolutionID: "res_old",
		Client:       Client{Name: "client"},
		Task:         map[string]any{"description": "old task"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify it was inserted (since RecordCase checks retention cutoff before inserting the new record, it's there)
	// Now insert a recent case: RecordCase will prune rows older than 90 days
	err = recorder.RecordCase(t.Context(), CaseRecord{
		CaseID:       "case_recent",
		OccurredAt:   now,
		Kind:         "override",
		SessionHash:  "session_recent",
		ResolutionID: "res_recent",
		Client:       Client{Name: "client"},
		Task:         map[string]any{"description": "recent task"},
	})
	if err != nil {
		t.Fatal(err)
	}

	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The 95-day-old case must have been pruned!
	for _, c := range cases {
		if c.CaseID == "case_old_95d" {
			t.Fatalf("case_old_95d should have been pruned by 90-day retention")
		}
	}
	if len(cases) != 1 || cases[0].CaseID != "case_recent" {
		t.Fatalf("expected only case_recent, got %v", cases)
	}

	// Test PurgeCases
	if err := recorder.PurgeCases(t.Context()); err != nil {
		t.Fatal(err)
	}
	casesAfterPurge, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(casesAfterPurge) != 0 {
		t.Fatalf("expected 0 cases after purge, got %d", len(casesAfterPurge))
	}
}

// TestSetCaseJournalEnabled verifies CLI enable/disable state persistence.
func TestSetCaseJournalEnabled(t *testing.T) {
	temp := t.TempDir()

	if IsCaseJournalEnabled(temp) {
		t.Fatal("expected case journal to be disabled by default")
	}

	if err := SetCaseJournalEnabled(temp, true); err != nil {
		t.Fatal(err)
	}
	if !IsCaseJournalEnabled(temp) {
		t.Fatal("expected case journal to be enabled after SetCaseJournalEnabled(true)")
	}

	if err := SetCaseJournalEnabled(temp, false); err != nil {
		t.Fatal(err)
	}
	if !IsCaseJournalEnabled(temp) {
		// disabled
	} else {
		t.Fatal("expected case journal to be disabled after SetCaseJournalEnabled(false)")
	}
}
