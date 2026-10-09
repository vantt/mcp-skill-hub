package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCasesIsolationAndLimits(t *testing.T) {
	temp := t.TempDir()
	config := Config{
		Path:          filepath.Join(temp, "telemetry.db"),
		WorkspaceRoot: temp,
		ContentMode:   ContentModeRedacted,
	}
	recorder, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	// Write a case
	err = recorder.RecordCase(t.Context(), CaseRecord{
		ResolutionID: "res-1",
		OccurredAt:   time.Now().UTC(),
		Kind:         "override",
		Client:       Client{Name: "cli"},
		Task:         map[string]any{"description": "super secret case text"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Verify cases API returns it
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil || len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d (err: %v)", len(cases), err)
	}
	if task, _ := cases[0].Task["description"].(string); task != "super secret case text" {
		t.Fatalf("case missing text")
	}

	// 2. Verify export does NOT contain it
	exportPath := filepath.Join(temp, "export.json")
	if _, err := recorder.Export(t.Context(), exportPath); err != nil {
		t.Fatal(err)
	}
	exported, _ := os.ReadFile(exportPath)
	if strings.Contains(string(exported), "super secret case text") {
		t.Fatal("export leaked case text!")
	}

	// 3. Purge removes it
	if err := recorder.PurgeCases(t.Context()); err != nil {
		t.Fatal(err)
	}
	casesAfter, _ := recorder.Cases(t.Context(), time.Time{}, "")
	if len(casesAfter) != 0 {
		t.Fatal("expected 0 cases after purge")
	}
}
