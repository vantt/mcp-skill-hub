package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
)

type goldenCorpusData struct {
	Skills []resolverpkg.Skill `json:"skills"`
	Cases  []struct {
		ID       string              `json:"id"`
		Split    string              `json:"split"`
		Request  resolverpkg.Request `json:"request"`
		Expected struct {
			Status            resolverpkg.Status `json:"status"`
			AcceptablePrimary []string           `json:"acceptable_primary"`
		} `json:"expected"`
	} `json:"cases"`
}

type goldenEvalGates struct {
	ResolvedPrecision float64 `json:"resolved_precision_min"`
	NoSkillRecall     float64 `json:"no_skill_abstention_recall_min"`
	AmbiguityRecall   float64 `json:"ambiguity_recall_min"`
	OverallAccuracy   float64 `json:"overall_accuracy_min"`
}

type goldenPolicyDoc struct {
	HeldOutGates goldenEvalGates `json:"held_out_gates"`
}

type staticCatalog struct {
	skills   []resolverpkg.Skill
	snapshot string
}

func (c staticCatalog) Snapshot() string { return c.snapshot }
func (c staticCatalog) Skills(context.Context) ([]resolverpkg.Skill, error) {
	return append([]resolverpkg.Skill(nil), c.skills...), nil
}
func (c staticCatalog) Search(context.Context, string, int) ([]resolverpkg.SearchHit, error) {
	return nil, nil
}

func evaluateGoldenHeldOut(skills []resolverpkg.Skill, cases []goldenCaseItem, policy resolverpkg.Policy) (acc, prec, nsRec, ambRec float64) {
	byID := make(map[string]resolverpkg.Skill, len(skills))
	for _, s := range skills {
		byID[s.ID] = s
	}

	total := len(cases)
	correct := 0
	predResolved := 0
	corrResolved := 0
	expNoSkill := 0
	corrNoSkill := 0
	expAmbiguity := 0
	corrAmbiguity := 0

	engine, err := resolverpkg.New(staticCatalog{skills: skills, snapshot: "golden"}, policy, resolverpkg.NewCache(len(cases)+16))
	if err != nil {
		return 0, 0, 0, 0
	}

	for _, c := range cases {
		resp, _ := engine.Resolve(context.Background(), c.Request)
		isCorrect := false
		if len(c.AcceptablePrimary) == 0 {
			expNoSkill++
			if resp.Status == resolverpkg.StatusNoSkill {
				corrNoSkill++
				isCorrect = true
			}
		} else {
			if resp.Status == resolverpkg.StatusResolved && resp.Primary != nil {
				predResolved++
				for _, prim := range c.AcceptablePrimary {
					if resp.Primary.ID == prim {
						corrResolved++
						isCorrect = true
						break
					}
				}
			}
		}
		if isCorrect {
			correct++
		}
	}

	if total > 0 {
		acc = float64(correct) / float64(total)
	}
	if predResolved > 0 {
		prec = float64(corrResolved) / float64(predResolved)
	}
	if expNoSkill > 0 {
		nsRec = float64(corrNoSkill) / float64(expNoSkill)
	}
	if expAmbiguity > 0 {
		ambRec = float64(corrAmbiguity) / float64(expAmbiguity)
	}
	return acc, prec, nsRec, ambRec
}

type goldenCaseItem struct {
	ID                string
	Request           resolverpkg.Request
	AcceptablePrimary []string
}

func TestRoutingCalibration(t *testing.T) {
	if os.Getenv("SKILLHUB_CALIBRATE") != "1" {
		t.Skip("skipping routing calibration grid runner: SKILLHUB_CALIBRATE=1 not set")
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	// Load golden held-out cases
	goldenBytes, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "resolver", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden goldenCorpusData
	if err := json.Unmarshal(goldenBytes, &golden); err != nil {
		t.Fatal(err)
	}

	var heldOutCases []goldenCaseItem
	for _, c := range golden.Cases {
		if c.Split == "held_out" {
			heldOutCases = append(heldOutCases, goldenCaseItem{
				ID:                c.ID,
				Request:           c.Request,
				AcceptablePrimary: c.Expected.AcceptablePrimary,
			})
		}
	}

	evalPolBytes, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "resolver", "evaluation-policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evalPol goldenPolicyDoc
	if err := json.Unmarshal(evalPolBytes, &evalPol); err != nil {
		t.Fatal(err)
	}
	gates := evalPol.HeldOutGates

	// Materialize routing workspace
	root := materializeRoutingWorkspace(t, repoRoot)
	noSkillPath := filepath.Join(repoRoot, "testdata", "routing", "no-skill-v1.yaml")

	floors := []float64{0.12, 0.14, 0.16, 0.18, 0.20, 0.22}
	margins := []float64{0.02, 0.04, 0.06}
	triggerWeights := []float64{0.32, 0.38, 0.44}
	notForWeights := []float64{0.36, 0.42, 0.48}

	defaultPolicy := resolverpkg.DefaultPolicy()

	fmt.Printf("| Applicability Floor | Minimum Margin | Trigger Weight | NotFor Weight | Routing Precision | Routing Recall | NoSkill Recall | False Positive Rate | Golden Accuracy | Golden Precision | Golden Passes |\n")
	fmt.Printf("|---|---|---|---|---|---|---|---|---|---|---|\n")

	type gridRow struct {
		floor      float64
		margin     float64
		trigW      float64
		notForW    float64
		precision  float64
		recall     float64
		noSkillRec float64
		fpr        float64
		goldenAcc  float64
		goldenPrec float64
		goldenPass bool
	}

	var rows []gridRow

	for _, floor := range floors {
		for _, margin := range margins {
			for _, trigW := range triggerWeights {
				for _, notForW := range notForWeights {
					pol := defaultPolicy
					pol.ApplicabilityFloor = floor
					pol.MinimumMargin = margin
					pol.Weights.Trigger = trigW
					pol.Weights.NotFor = notForW

					report, evalErr := EvaluateRouting(t.Context(), RoutingEvalOptions{
						WorkspacePath: root,
						NoSkillFile:   noSkillPath,
						Policy:        &pol,
					})
					if evalErr != nil {
						t.Fatalf("eval error at floor=%.2f: %v", floor, evalErr)
					}

					acc, prec, nsRec, _ := evaluateGoldenHeldOut(golden.Skills, heldOutCases, pol)
					goldenPasses := acc >= gates.OverallAccuracy && prec >= gates.ResolvedPrecision && nsRec >= gates.NoSkillRecall

					p := 0.0
					if report.Precision != nil {
						p = *report.Precision
					}
					r := 0.0
					if report.Recall != nil {
						r = *report.Recall
					}
					nsR := 0.0
					if report.NoSkillRecall != nil {
						nsR = *report.NoSkillRecall
					}
					fpr := 0.0
					if report.FalsePositiveRate != nil {
						fpr = *report.FalsePositiveRate
					}

					rows = append(rows, gridRow{
						floor:      floor,
						margin:     margin,
						trigW:      trigW,
						notForW:    notForW,
						precision:  p,
						recall:     r,
						noSkillRec: nsR,
						fpr:        fpr,
						goldenAcc:  acc,
						goldenPrec: prec,
						goldenPass: goldenPasses,
					})

					fmt.Printf("| %.2f | %.2f | %.2f | %.2f | %.4f | %.4f | %.4f | %.4f | %.4f | %.4f | %t |\n",
						floor, margin, trigW, notForW, p, r, nsR, fpr, acc, prec, goldenPasses)
				}
			}
		}
	}

	t.Logf("Completed calibration grid: %d points evaluated", len(rows))
}
