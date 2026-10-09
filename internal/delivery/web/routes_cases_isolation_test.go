package web

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestCasesIsolationWebAndExport(t *testing.T) {
	t.Parallel()

	root := newWebWorkspace(t)

	// 1. Turn on case journal flag
	if err := telemetry.SetCaseJournalEnabled(root, true); err != nil {
		t.Fatal(err)
	}

	telService := app.TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	// 2. Record a case whose task contains a unique marker string
	marker := "MARKER_ISOLATION_TASK_LEAK_SECRET_48291"
	err = recorder.RecordCase(t.Context(), telemetry.CaseRecord{
		ResolutionID:    "res_isolation_test",
		SessionHash:     "sess_isolation_test",
		OccurredAt:      time.Now().UTC(),
		Kind:            "override",
		Client:          telemetry.Client{Name: "web-test"},
		CatalogSnapshot: "sha256:cat",
		Task:            map[string]any{"description": "critical task containing " + marker + " data"},
		Chosen:          "review-skill",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Assert that the marker appears in telemetry cases
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least 1 case recorded")
	}
	foundInCases := false
	for _, c := range cases {
		taskDesc, _ := c.Task["description"].(string)
		if strings.Contains(taskDesc, marker) {
			foundInCases = true
			break
		}
	}
	if !foundInCases {
		t.Fatalf("expected marker %q to appear in telemetry cases", marker)
	}

	// 4. Assert that the marker does NOT appear in telemetry export output
	exportPath := filepath.Join(root, "runtime", "export.json")
	if _, err := recorder.Export(t.Context(), exportPath); err != nil {
		t.Fatal(err)
	}
	exportBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exportBytes), marker) {
		t.Fatalf("isolation violation: marker %q leaked into telemetry export!", marker)
	}

	// 5. Assert that the marker does NOT appear in any web JSON route
	srv := newTestServer(t, root)
	webRoutes := []string{
		"/api/v1/session",
		"/api/v1/home",
		"/api/v1/skills",
		"/api/v1/skills/review-skill",
		"/api/v1/skills/review-skill/review",
		"/api/v1/skills/review-skill/usage",
		"/api/v1/skills/review-skill/usage?since=30d",
		"/api/v1/skills/review-skill/runtime",
		"/api/v1/sources",
		"/api/v1/skills/review-skill/sources",
	}

	for _, path := range webRoutes {
		rec := get(t, srv, path)
		body := rec.Body.String()
		if strings.Contains(body, marker) {
			t.Fatalf("isolation violation: marker %q leaked into web route %s! Body:\n%s", marker, path, body)
		}
	}
}
