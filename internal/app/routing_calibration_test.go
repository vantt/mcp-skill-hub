package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
)

type evalPolicyFile struct {
	HeldOutGates struct {
		ResolvedPrecision float64 `json:"resolved_precision_min"`
		NoSkillRecall     float64 `json:"no_skill_abstention_recall_min"`
		AmbiguityRecall   float64 `json:"ambiguity_recall_min"`
		OverallAccuracy   float64 `json:"overall_accuracy_min"`
	} `json:"held_out_gates"`
}

type goldenCaseV1 struct {
	ID       string              `json:"id"`
	Split    string              `json:"split"`
	Tags     []string            `json:"tags"`
	Request  resolverpkg.Request `json:"request"`
	Expected evaluation.Expected `json:"expected"`
}

type goldenCorpusV1 struct {
	Skills []resolverpkg.Skill `json:"skills"`
	Cases  []goldenCaseV1      `json:"cases"`
}

type calibrationMetrics struct {
	total             int
	correct           int
	predictedResolved int
	correctResolved   int
	expectedNoSkill   int
	correctNoSkill    int
	expectedAmbiguity int
	correctAmbiguity  int
}

func (m calibrationMetrics) precision() float64 {
	if m.predictedResolved == 0 {
		return 0
	}
	return float64(m.correctResolved) / float64(m.predictedResolved)
}
func (m calibrationMetrics) noSkillRecall() float64 {
	if m.expectedNoSkill == 0 {
		return 0
	}
	return float64(m.correctNoSkill) / float64(m.expectedNoSkill)
}
func (m calibrationMetrics) ambiguityRecall() float64 {
	if m.expectedAmbiguity == 0 {
		return 0
	}
	return float64(m.correctAmbiguity) / float64(m.expectedAmbiguity)
}
func (m calibrationMetrics) accuracy() float64 {
	if m.total == 0 {
		return 0
	}
	return float64(m.correct) / float64(m.total)
}

func setupGoldenSQLiteCatalog(skills []resolverpkg.Skill) (*resolverpkg.SQLiteCatalog, func(), error) {
	database, err := sql.Open("sqlite", "file:golden-calib?mode=memory&cache=shared")
	if err != nil {
		return nil, nil, err
	}
	database.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE skills(id TEXT PRIMARY KEY,collection_id TEXT,name TEXT,status TEXT,description TEXT,digest TEXT)`,
		`CREATE TABLE canonical_entities(id TEXT PRIMARY KEY,content_json TEXT)`,
		`CREATE VIRTUAL TABLE skill_fts USING fts5(skill_id UNINDEXED,name,aliases,description,triggers,examples,keywords)`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			_ = database.Close()
			return nil, nil, err
		}
	}

	for _, s := range skills {
		doc := map[string]any{
			"aliases": s.Aliases,
			"quality": map[string]any{"reviewed": s.Reviewed},
			"routing": map[string]any{
				"operations":       s.Operations,
				"triggers":         s.Triggers,
				"not_for":          s.NotFor,
				"min_scope":        s.MinScope,
				"examples":         s.Examples,
				"requirements":     map[string]any{"facts": map[string]any{"all": s.Requirements.FactsAll, "any": s.Requirements.FactsAny}, "capabilities": map[string]any{"all": s.Requirements.CapabilitiesAll, "any": s.Requirements.CapabilitiesAny}},
				"distinguish_from": goldenDiscriminators(s.DistinguishFrom),
				"supporting":       goldenSupports(s.Supporting),
				"equivalent_to":    goldenEquivalence(s.Equivalence),
			},
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			_ = database.Close()
			return nil, nil, err
		}
		if _, err := database.Exec(`INSERT INTO skills VALUES(?,?,?,?,?,?)`, s.ID, s.CollectionID, s.Name, s.Status, s.Description, s.Digest); err != nil {
			_ = database.Close()
			return nil, nil, err
		}
		if _, err := database.Exec(`INSERT INTO canonical_entities VALUES(?,?)`, s.ID, string(encoded)); err != nil {
			_ = database.Close()
			return nil, nil, err
		}
		if _, err := database.Exec(`INSERT INTO skill_fts VALUES(?,?,?,?,?,?,?)`, s.ID, s.Name, strings.Join(s.Aliases, " "), s.Description, strings.Join(s.Triggers, " "), strings.Join(s.Examples, " "), ""); err != nil {
			_ = database.Close()
			return nil, nil, err
		}
	}

	catalog, err := resolverpkg.NewSQLiteCatalog(database, "sha256:golden-calib-snapshot")
	if err != nil {
		_ = database.Close()
		return nil, nil, err
	}
	return catalog, func() { _ = database.Close() }, nil
}

func goldenDiscriminators(values []resolverpkg.Discriminator) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"skill": value.SkillID, "discriminator": map[string]any{"field": value.Field, "question": value.Question, "choices": value.Choices}})
	}
	return result
}

func goldenSupports(values []resolverpkg.SupportRelation) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"skill": value.SkillID, "when": map[string]any{"operation": value.Operation}, "role": value.Role, "activation": value.Activation})
	}
	return result
}

func goldenEquivalence(values []resolverpkg.EquivalenceRelation) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"skill": value.SkillID, "preference": value.Preference, "version_policy": value.VersionPolicy})
	}
	return result
}

func evaluateGoldenHeldOut(cat *resolverpkg.SQLiteCatalog, cases []goldenCaseV1, policy resolverpkg.Policy) calibrationMetrics {
	engine, _ := resolverpkg.New(cat, policy, resolverpkg.NewCache(len(cases)+1))
	m := calibrationMetrics{}

	for _, c := range cases {
		m.total++
		resp, _ := engine.Resolve(context.Background(), c.Request)

		isNoSkillExpected := c.Expected.NoSkill || c.Expected.Status == resolverpkg.StatusNoSkill
		if isNoSkillExpected {
			m.expectedNoSkill++
			if resp.Status == resolverpkg.StatusNoSkill {
				m.correctNoSkill++
				m.correct++
			}
			continue
		}

		if c.Expected.Status == resolverpkg.StatusNeedsContext {
			m.expectedAmbiguity++
			if resp.Status == resolverpkg.StatusNeedsContext {
				m.correctAmbiguity++
				m.correct++
			}
			continue
		}

		correct := resp.Status == c.Expected.Status
		if resp.Status == resolverpkg.StatusResolved {
			m.predictedResolved++
			if correct && resp.Primary != nil {
				matched := false
				for _, acc := range c.Expected.AcceptablePrimary {
					if resp.Primary.ID == acc {
						matched = true
						break
					}
				}
				if matched {
					m.correctResolved++
				} else {
					correct = false
				}
			} else {
				correct = false
			}
		}
		if correct {
			m.correct++
		}
	}
	return m
}

func TestRoutingCalibration(t *testing.T) {
	if os.Getenv("SKILLHUB_CALIBRATE") != "1" {
		t.Skip("skipping calibration test; set SKILLHUB_CALIBRATE=1 to run")
	}

	root := materializeRoutingWorkspace(t)
	noSkillPath, err := filepath.Abs("../../testdata/routing/no-skill-v1.yaml")
	if err != nil {
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
	var corpus goldenCorpusV1
	if err := json.Unmarshal(goldenBytes, &corpus); err != nil {
		t.Fatal(err)
	}

	policyPath, err := filepath.Abs("../../testdata/resolver/evaluation-policy-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	var evalPolicy evalPolicyFile
	if err := json.Unmarshal(policyBytes, &evalPolicy); err != nil {
		t.Fatal(err)
	}

	var heldOutCases []goldenCaseV1
	for _, c := range corpus.Cases {
		if c.Split == "held_out" {
			heldOutCases = append(heldOutCases, c)
		}
	}

	goldenCat, closeGoldenCat, err := setupGoldenSQLiteCatalog(corpus.Skills)
	if err != nil {
		t.Fatal(err)
	}
	defer closeGoldenCat()

	appFloorValues := []float64{0.12, 0.14, 0.16, 0.18, 0.20, 0.22}
	minMarginValues := []float64{0.02, 0.04, 0.06}
	trigWeightValues := []float64{0.32, 0.38, 0.44}
	notForWeightValues := []float64{0.36, 0.42, 0.48}

	defaultPolicy := resolverpkg.DefaultPolicy()

	type gridResult struct {
		floor       float64
		margin      float64
		trigW       float64
		notForW     float64
		precision   float64
		recall      float64
		noSkillRec  float64
		fpr         float64
		goldenPass  bool
		distDefault float64
	}

	service := RoutingEvalService{}
	var results []gridResult

	t.Log("| Floor | Margin | TrigW | NotForW | Precision@1 | Recall | NoSkillRec | FPR | GoldenHeldOut |")
	t.Log("|---|---|---|---|---|---|---|---|---|")

	for _, floor := range appFloorValues {
		for _, margin := range minMarginValues {
			for _, trigW := range trigWeightValues {
				for _, notForW := range notForWeightValues {
					pol := defaultPolicy
					pol.ApplicabilityFloor = floor
					pol.MinimumMargin = margin
					pol.Weights.Trigger = trigW
					pol.Weights.NotFor = notForW

					// Evaluate on golden held-out
					held := evaluateGoldenHeldOut(goldenCat, heldOutCases, pol)
					passGates := held.precision() >= evalPolicy.HeldOutGates.ResolvedPrecision &&
						held.noSkillRecall() >= evalPolicy.HeldOutGates.NoSkillRecall &&
						held.ambiguityRecall() >= evalPolicy.HeldOutGates.AmbiguityRecall &&
						held.accuracy() >= evalPolicy.HeldOutGates.OverallAccuracy

					// Evaluate on routing corpus
					report, err := service.Run(context.Background(), RoutingEvalQuery{
						WorkspacePath: root,
						NoSkillPath:   noSkillPath,
						Policy:        &pol,
					})
					if err != nil {
						t.Fatalf("eval failed on point (%f, %f, %f, %f): %v", floor, margin, trigW, notForW, err)
					}

					p1 := 0.0
					if report.Precision1 != nil {
						p1 = *report.Precision1
					}
					rec := 0.0
					if report.Recall != nil {
						rec = *report.Recall
					}
					nsRec := 0.0
					if report.NoSkillRecall != nil {
						nsRec = *report.NoSkillRecall
					}
					fpr := 0.0
					if report.FalsePositiveRate != nil {
						fpr = *report.FalsePositiveRate
					}

					dist := math.Abs(floor-defaultPolicy.ApplicabilityFloor) +
						math.Abs(margin-defaultPolicy.MinimumMargin) +
						math.Abs(trigW-defaultPolicy.Weights.Trigger) +
						math.Abs(notForW-defaultPolicy.Weights.NotFor)

					res := gridResult{
						floor:       floor,
						margin:      margin,
						trigW:       trigW,
						notForW:     notForW,
						precision:   p1,
						recall:      rec,
						noSkillRec:  nsRec,
						fpr:         fpr,
						goldenPass:  passGates,
						distDefault: dist,
					}
					results = append(results, res)

					passStr := "PASS"
					if !passGates {
						passStr = "FAIL"
					}
					t.Logf("| %.2f | %.2f | %.2f | %.2f | %.4f | %.4f | %.4f | %.4f | %s |",
						floor, margin, trigW, notForW, p1, rec, nsRec, fpr, passStr)
				}
			}
		}
	}

	t.Logf("Total evaluated grid points: %d", len(results))

	// Find best candidate
	var bestCandidate *gridResult
	baselineP1 := 0.5714
	baselineRec := 0.5891
	baselineNoSkill := 0.8438
	baselineFPR := 0.1379

	for i := range results {
		r := &results[i]
		if !r.goldenPass {
			continue
		}
		if r.noSkillRec < baselineNoSkill || r.fpr > baselineFPR {
			continue
		}
		if bestCandidate == nil {
			bestCandidate = r
			continue
		}
		if r.precision > bestCandidate.precision {
			bestCandidate = r
		} else if r.precision == bestCandidate.precision {
			if r.distDefault < bestCandidate.distDefault {
				bestCandidate = r
			}
		}
	}

	if bestCandidate != nil {
		t.Logf("Selected point: floor=%.2f margin=%.2f trigW=%.2f notForW=%.2f (P1=%.4f, Rec=%.4f, NSRec=%.4f, FPR=%.4f, dist=%.4f)",
			bestCandidate.floor, bestCandidate.margin, bestCandidate.trigW, bestCandidate.notForW,
			bestCandidate.precision, bestCandidate.recall, bestCandidate.noSkillRec, bestCandidate.fpr, bestCandidate.distDefault)

		improvementP1 := bestCandidate.precision - baselineP1
		improvementRec := bestCandidate.recall - baselineRec
		t.Logf("Improvement vs baseline: P1=%.4f (needs >= 0.02), Rec=%.4f (needs >= 0.02)", improvementP1, improvementRec)
		if improvementP1 >= 0.02 || improvementRec >= 0.02 {
			t.Log("Decision: APPLY policy update")
		} else {
			t.Log("Decision: DECLINE (improvement < 0.02 margin, evidence attached)")
		}
	} else {
		t.Log("Decision: DECLINE (no point met golden-v1 held-out and baseline constraints)")
	}
}
