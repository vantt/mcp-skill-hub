package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type RoutingGateThresholds struct {
	MinPrecision     float64 `json:"min_precision"`
	MinRecall        float64 `json:"min_recall"`
	MinNoSkillRecall float64 `json:"min_no_skill_recall"`
	MaxFPR           float64 `json:"max_fpr"`
}

type RoutingGateConfig struct {
	SchemaVersion int                   `json:"schema_version"`
	Thresholds    RoutingGateThresholds `json:"thresholds"`
}

type goldenCorpusInput struct {
	Skills []resolverpkg.Skill `json:"skills"`
}

type examplesOverlayInput struct {
	Skills map[string]struct {
		Examples        []string `json:"examples"`
		CounterExamples []string `json:"counter_examples"`
	} `json:"skills"`
}

func materializeRoutingWorkspace(t *testing.T, repoRoot string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "routing-workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatalf("apply workspace: %v", err)
	}

	goldenPath := filepath.Join(repoRoot, "testdata", "resolver", "golden-v1.json")
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden-v1: %v", err)
	}
	var golden goldenCorpusInput
	if err := json.Unmarshal(goldenBytes, &golden); err != nil {
		t.Fatalf("unmarshal golden-v1: %v", err)
	}

	examplesPath := filepath.Join(repoRoot, "testdata", "routing", "examples-v1.json")
	examplesBytes, err := os.ReadFile(examplesPath)
	if err != nil {
		t.Fatalf("read examples-v1: %v", err)
	}
	var overlay examplesOverlayInput
	if err := json.Unmarshal(examplesBytes, &overlay); err != nil {
		t.Fatalf("unmarshal examples-v1: %v", err)
	}

	for _, s := range golden.Skills {
		collection := s.CollectionID
		if collection == "" {
			collection = "core"
		}
		skillDir := filepath.Join(root, "skills", collection, s.ID)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill %s: %v", s.ID, err)
		}

		exData := overlay.Skills[s.ID]
		examples := exData.Examples
		counterExamples := exData.CounterExamples

		minScope := s.MinScope
		if minScope == "" {
			minScope = "single_step"
		}
		notFor := s.NotFor
		if len(notFor) == 0 {
			notFor = []string{"unrelated tasks"}
		}

		manifestMap := map[string]any{
			"schema_version": 1,
			"id":             s.ID,
			"name":           s.Name,
			"status":         "active",
			"description":    s.Description,
			"routing": map[string]any{
				"triggers":         s.Triggers,
				"not_for":          notFor,
				"min_scope":        minScope,
				"examples":         examples,
				"counter_examples": counterExamples,
			},
			"quality": map[string]any{
				"reviewed": true,
			},
		}
		if len(s.Operations) > 0 {
			manifestMap["routing"].(map[string]any)["operations"] = s.Operations
		}
		if len(s.Aliases) > 0 {
			manifestMap["aliases"] = s.Aliases
		}

		manifestBytes, err := yaml.Marshal(manifestMap)
		if err != nil {
			t.Fatalf("marshal manifest for %s: %v", s.ID, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), manifestBytes, 0o644); err != nil {
			t.Fatalf("write skill.meta.yaml for %s: %v", s.ID, err)
		}

		desc := s.Description
		if desc == "" {
			desc = s.Name
		}
		skillMD := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\n\n%s\n", s.ID, desc, s.Name, desc)
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
			t.Fatalf("write SKILL.md for %s: %v", s.ID, err)
		}
	}

	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatalf("build catalog generation: %v", err)
	}

	return root
}

func TestRoutingEvalGate(t *testing.T) {
	t.Parallel()
	start := time.Now()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	root := materializeRoutingWorkspace(t, repoRoot)
	noSkillPath := filepath.Join(repoRoot, "testdata", "routing", "no-skill-v1.yaml")

	report, err := EvaluateRouting(t.Context(), RoutingEvalOptions{
		WorkspacePath: root,
		NoSkillFile:   noSkillPath,
	})
	if err != nil {
		t.Fatalf("EvaluateRouting failed: %v", err)
	}

	duration := time.Since(start)
	t.Logf("RoutingEvalGate duration: %v", duration)
	t.Logf("Total: %d, P: %d, N: %d, Z: %d", report.TotalCases, report.PositiveCases, report.CounterCases, report.NoSkillCases)
	if report.Precision != nil {
		t.Logf("Precision: %.4f", *report.Precision)
	}
	if report.Recall != nil {
		t.Logf("Recall: %.4f", *report.Recall)
	}
	if report.NoSkillRecall != nil {
		t.Logf("NoSkillRecall: %.4f", *report.NoSkillRecall)
	}
	if report.NoSkillPrecision != nil {
		t.Logf("NoSkillPrecision: %.4f", *report.NoSkillPrecision)
	}
	if report.FalsePositiveRate != nil {
		t.Logf("FPR: %.4f", *report.FalsePositiveRate)
	}
	if len(report.Failures) > 0 {
		t.Logf("Failures count: %d", len(report.Failures))
		for _, f := range report.Failures {
			t.Logf("  Failure: [%s] %s #%d phrase=%q status=%s gotPrimary=%s", f.Kind, f.SkillID, f.Index, f.Phrase, f.GotStatus, f.GotPrimary)
		}
	}

	gatePath := filepath.Join(repoRoot, "testdata", "routing", "gate-v1.json")
	gateBytes, err := os.ReadFile(gatePath)
	if err != nil {
		t.Logf("gate-v1.json not yet written; skipping threshold check")
		return
	}
	var gateConfig RoutingGateConfig
	if err := json.Unmarshal(gateBytes, &gateConfig); err != nil {
		t.Fatalf("unmarshal gate-v1: %v", err)
	}

	thresholds := gateConfig.Thresholds
	if report.Precision == nil || *report.Precision < thresholds.MinPrecision {
		t.Errorf("Precision %.4f < gate %.4f", *report.Precision, thresholds.MinPrecision)
	}
	if report.Recall == nil || *report.Recall < thresholds.MinRecall {
		t.Errorf("Recall %.4f < gate %.4f", *report.Recall, thresholds.MinRecall)
	}
	if report.NoSkillRecall == nil || *report.NoSkillRecall < thresholds.MinNoSkillRecall {
		t.Errorf("NoSkillRecall %.4f < gate %.4f", *report.NoSkillRecall, thresholds.MinNoSkillRecall)
	}
	if report.FalsePositiveRate == nil || *report.FalsePositiveRate > thresholds.MaxFPR {
		t.Errorf("FPR %.4f > gate %.4f", *report.FalsePositiveRate, thresholds.MaxFPR)
	}
}
