package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/resolver"
)

type goldenV1Corpus struct {
	Skills []resolver.Skill `json:"skills"`
}

type examplesOverlay map[string]struct {
	Examples        []string `json:"examples"`
	CounterExamples []string `json:"counter_examples"`
}

type routingGateThresholds struct {
	MinPrecision     float64 `json:"precision_at_1_min"`
	MinRecall        float64 `json:"recall_min"`
	MinNoSkillRecall float64 `json:"no_skill_recall_min"`
	MaxFPR           float64 `json:"false_positive_rate_max"`
}

func materializeRoutingWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "routing-workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	goldenPath, err := filepath.Abs("../../testdata/resolver/golden-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus goldenV1Corpus
	if err := json.Unmarshal(goldenBytes, &corpus); err != nil {
		t.Fatal(err)
	}

	overlayPath, err := filepath.Abs("../../testdata/routing/examples-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	overlayBytes, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatal(err)
	}
	var overlay examplesOverlay
	if err := json.Unmarshal(overlayBytes, &overlay); err != nil {
		t.Fatal(err)
	}

	for _, s := range corpus.Skills {
		col := s.CollectionID
		if col == "" {
			col = "core"
		}
		skillDir := filepath.Join(root, "skills", col, s.ID)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}

		skillMD := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n", s.ID, s.Description, s.Name)
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
			t.Fatal(err)
		}

		meta := map[string]any{
			"schema_version": 1,
			"id":             s.ID,
			"name":           s.Name,
			"status":         "active",
			"description":    s.Description,
			"aliases":        s.Aliases,
			"topics":         s.Topics,
			"technologies":   s.Technologies,
			"routing": map[string]any{
				"operations":       s.Operations,
				"triggers":         s.Triggers,
				"not_for":          s.NotFor,
				"min_scope":        s.MinScope,
				"examples":         overlay[s.ID].Examples,
				"counter_examples": overlay[s.ID].CounterExamples,
			},
			"quality": map[string]any{
				"reviewed": true,
			},
			"provenance": map[string]any{
				"created_by": "skillhub",
			},
		}
		metaBytes, err := yaml.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), metaBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := (CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	return root
}

func TestRoutingEvalGate(t *testing.T) {
	t.Parallel()
	start := time.Now()
	root := materializeRoutingWorkspace(t)

	noSkillPath, err := filepath.Abs("../../testdata/routing/no-skill-v1.yaml")
	if err != nil {
		t.Fatal(err)
	}

	gatePath, err := filepath.Abs("../../testdata/routing/gate-v1.json")
	if err != nil {
		t.Fatal(err)
	}

	gateBytes, err := os.ReadFile(gatePath)
	if err != nil {
		t.Fatalf("read gate file: %v", err)
	}
	var gate routingGateThresholds
	if err := json.Unmarshal(gateBytes, &gate); err != nil {
		t.Fatalf("unmarshal gate file: %v", err)
	}

	service := RoutingEvalService{}
	report, err := service.Run(context.Background(), RoutingEvalQuery{
		WorkspacePath:    root,
		NoSkillPath:      noSkillPath,
		MinPrecision:     &gate.MinPrecision,
		MinRecall:        &gate.MinRecall,
		MinNoSkillRecall: &gate.MinNoSkillRecall,
		MaxFPR:           &gate.MaxFPR,
	})
	if err != nil {
		t.Fatalf("RoutingEvalService.Run: %v", err)
	}

	duration := time.Since(start)
	t.Logf("RoutingEvalGate duration: %s", duration)

	if report.Precision1 != nil {
		t.Logf("Precision@1: %.4f (threshold >= %.4f)", *report.Precision1, gate.MinPrecision)
	}
	if report.Recall != nil {
		t.Logf("Recall: %.4f (threshold >= %.4f)", *report.Recall, gate.MinRecall)
	}
	if report.NoSkillRecall != nil {
		t.Logf("NoSkillRecall: %.4f (threshold >= %.4f)", *report.NoSkillRecall, gate.MinNoSkillRecall)
	}
	if report.NoSkillPrecision != nil {
		t.Logf("NoSkillPrecision: %.4f", *report.NoSkillPrecision)
	}
	if report.FalsePositiveRate != nil {
		t.Logf("FalsePositiveRate: %.4f (threshold <= %.4f)", *report.FalsePositiveRate, gate.MaxFPR)
	}
	t.Logf("Failed cases count: %d", len(report.FailedCases))
	for _, fc := range report.FailedCases {
		t.Logf("  [%s] %s #%d %q -> status=%s got=%s", fc.Kind, fc.SkillID, fc.Index, fc.Phrase, fc.GotStatus, fc.GotPrimary)
	}
	if !report.PassedGate {
		t.Fatalf("RoutingEvalGate FAILED: %v (failing cases: %d)", report.GateFailures, len(report.FailedCases))
	}
}
