package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func createSkillWithExamples(t *testing.T, root, id, name string, triggers, examples, counterExamples []string) {
	t.Helper()
	service := SkillService{}
	ctx := context.Background()
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          id,
		Collection:  "software",
		Name:        name,
		Description: "Description for " + name,
		Content:     []byte("# " + name + "\n"),
		Routing: skill.RoutingInput{
			Operations:      []string{"review"},
			Triggers:        triggers,
			NotFor:          []string{"unrelated"},
			Examples:        examples,
			CounterExamples: counterExamples,
			MinScope:        "multi_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	previewAct, err := service.PreviewActivate(ctx, root, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, previewAct, previewAct.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingEvalService(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	createSkillWithExamples(t, root, "skill-alpha", "Skill Alpha",
		[]string{"alpha trigger"},
		[]string{"run alpha task", "execute alpha workflow"},
		[]string{"do beta thing"},
	)
	createSkillWithExamples(t, root, "skill-beta", "Skill Beta",
		[]string{"beta trigger"},
		[]string{"run beta task"},
		[]string{"do alpha thing"},
	)

	if _, err := (CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	// Create no-skill suite file
	noSkillPath := filepath.Join(t.TempDir(), "no-skill.yaml")
	noSkillYAML := `schema_version: 1
id: test-no-skill
cases:
  - id: ns-1
    partition: held_out
    split: held_out
    request:
      schema_version: "1"
      request_id: ns-req-1
      task:
        description: order a pizza with extra cheese
        scope: multi_step
      operation: ""
    expected:
      status: no_skill
  - id: ns-2
    partition: held_out
    split: held_out
    request:
      schema_version: "1"
      request_id: ns-req-2
      task:
        description: tell me a bedtime story
        scope: multi_step
      operation: ""
    expected:
      status: no_skill
`
	if err := os.WriteFile(noSkillPath, []byte(noSkillYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	service := RoutingEvalService{}
	report, err := service.Run(context.Background(), RoutingEvalQuery{
		WorkspacePath: root,
		NoSkillPath:   noSkillPath,
	})
	if err != nil {
		t.Fatalf("RoutingEvalService.Run: %v", err)
	}

	// Positives: 2 for alpha + 1 for beta = 3
	if report.TotalPositives != 3 {
		t.Errorf("TotalPositives = %d, want 3", report.TotalPositives)
	}
	// Counters: 1 for alpha + 1 for beta = 2
	if report.TotalCounters != 2 {
		t.Errorf("TotalCounters = %d, want 2", report.TotalCounters)
	}
	// NoSkill: 2
	if report.TotalNoSkill != 2 {
		t.Errorf("TotalNoSkill = %d, want 2", report.TotalNoSkill)
	}

	// Metrics should be non-nil
	if report.Precision1 == nil {
		t.Errorf("Precision1 is nil")
	}
	if report.Recall == nil {
		t.Errorf("Recall is nil")
	}
	if report.NoSkillRecall == nil {
		t.Errorf("NoSkillRecall is nil")
	}
	if report.FalsePositiveRate == nil {
		t.Errorf("FalsePositiveRate is nil")
	}

	// Per-skill recall
	if len(report.PerSkillRecall) != 2 {
		t.Errorf("PerSkillRecall length = %d, want 2", len(report.PerSkillRecall))
	}

	// Telemetry isolation check: telemetry.db must have NO events
	rec, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Close(context.Background()) }()

	preview, err := rec.Preview(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != 0 || len(preview.JSONL) != 0 {
		t.Fatalf("telemetry.db leaked events during eval: events=%d jsonl=%s", preview.Events, string(preview.JSONL))
	}
}
