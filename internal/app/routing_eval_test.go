package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestRoutingEvalTwoSkillsAndTelemetryIsolation(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	skillADir := filepath.Join(root, "skills", "core", "code-review")
	if err := os.MkdirAll(skillADir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillAMeta := `schema_version: 1
id: code-review
name: Code Review
status: active
description: Review code and inspect pull requests.
routing:
  triggers: [review code, inspect diff]
  not_for: [write prose]
  min_scope: single_step
  examples: [review pull request changes for issues, inspect git diff for security bugs]
  counter_examples: [write database migration]
`
	if err := os.WriteFile(filepath.Join(skillADir, "skill.meta.yaml"), []byte(skillAMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte("---\nname: code-review\ndescription: Review code and inspect pull requests.\n---\n# Code Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillBDir := filepath.Join(root, "skills", "core", "db-migrate")
	if err := os.MkdirAll(skillBDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillBMeta := `schema_version: 1
id: db-migrate
name: Database Migration
status: active
description: Execute and write schema migrations.
routing:
  triggers: [run database migration, update schema]
  not_for: [write prose]
  min_scope: single_step
  examples: [write database migration script, execute schema migration on postgres]
  counter_examples: [review pull request]
`
	if err := os.WriteFile(filepath.Join(skillBDir, "skill.meta.yaml"), []byte(skillBMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillBDir, "SKILL.md"), []byte("---\nname: db-migrate\ndescription: Execute and write schema migrations.\n---\n# Database Migration\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}

	// Create a no-skill suite file
	noSkillPath := filepath.Join(t.TempDir(), "no-skill.yaml")
	noSkillContent := `schema_version: 1
id: no-skill-test
cases:
  - schema_version: 1
    id: weather-query
    partition: development
    request:
      schema_version: "1"
      request_id: req-no-skill-1
      task:
        description: what is the weather in Tokyo today
        scope: single_step
    expected:
      status: no_skill
      no_skill: true
`
	if err := os.WriteFile(noSkillPath, []byte(noSkillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := EvaluateRouting(t.Context(), RoutingEvalOptions{
		WorkspacePath: root,
		NoSkillFile:   noSkillPath,
	})
	if err != nil {
		t.Fatalf("EvaluateRouting failed: %v", err)
	}

	if report.PositiveCases != 4 {
		t.Fatalf("expected 4 positive cases, got %d", report.PositiveCases)
	}
	if report.CounterCases != 2 {
		t.Fatalf("expected 2 counter cases, got %d", report.CounterCases)
	}
	if report.NoSkillCases != 1 {
		t.Fatalf("expected 1 no-skill case, got %d", report.NoSkillCases)
	}
	if report.TotalCases != 7 {
		t.Fatalf("expected 7 total cases, got %d", report.TotalCases)
	}

	if report.Recall == nil || *report.Recall < 1.0 {
		t.Fatalf("expected 1.0 recall, got %f, failures: %#v", *report.Recall, report.Failures)
	}
	if report.Precision == nil || *report.Precision < 1.0 {
		t.Fatalf("expected 1.0 precision, got %v", report.Precision)
	}
	if report.NoSkillRecall == nil || *report.NoSkillRecall < 1.0 {
		t.Fatalf("expected 1.0 no-skill recall, got %v", report.NoSkillRecall)
	}
	if report.FalsePositiveRate == nil || *report.FalsePositiveRate > 0.0 {
		t.Fatalf("expected 0.0 false positive rate, got %v", report.FalsePositiveRate)
	}

	// Verify Telemetry Isolation (Requirement 6)
	telemetryDBPath := filepath.Join(root, "runtime", "telemetry.db")
	db, err := sql.Open("sqlite", telemetryDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var resolutionEventCount int
	err = db.QueryRow(`SELECT count(*) FROM telemetry_events WHERE kind LIKE 'resolution.%'`).Scan(&resolutionEventCount)
	if err != nil {
		t.Fatalf("query resolution events: %v", err)
	}
	if resolutionEventCount != 0 {
		t.Fatalf("expected 0 resolution telemetry events after routing eval, got %d", resolutionEventCount)
	}
}
