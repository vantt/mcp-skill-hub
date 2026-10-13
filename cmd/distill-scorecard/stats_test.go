package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/distill"
)

const fixture = "../../internal/migration/testdata/test-audit-distill.yaml"

func loadFixture(t *testing.T) []distill.Lesson {
	t.Helper()
	doc, err := distill.LoadDocument(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return doc.Lessons
}

func num(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("want a number, got %#v", v)
	}
	return f
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v ± %v", name, got, want, tol)
	}
}

func TestStatsReproducesTheFieldInventoryOnTheFixture(t *testing.T) {
	res := computeStats(loadFixture(t))

	if got := res["_dataset"]["lessons"]; got != 42 {
		t.Errorf("lessons = %v, want 42", got)
	}
	if got := res["score.components"]["final_score_zero"]; got != 24 {
		t.Errorf("final_score_zero = %v, want 24", got)
	}
	near(t, "contrast entropy", num(t, res["contrast"]["entropy_bits"]), 1.762, 0.001)
	near(t, "jaccard median", num(t, res["notable"]["jaccard_with_what_median"]), 0.09, 0.005)
	near(t, "new-word share", num(t, res["notable"]["share_of_content_words_not_in_what_median"]), 0.91, 0.02)
	near(t, "spearman impact~final", num(t, res["score.components"]["spearman_impact_final"]), 0.98, 0.005)
	near(t, "spearman effort~final", num(t, res["score.components"]["spearman_effort_final"]), -0.12, 0.005)
	near(t, "spearman evidence~final", num(t, res["score.components"]["spearman_evidence_final"]), 0.28, 0.005)
	marginal, _ := res["notable"]["gzip_marginal_bytes_of_notable"].(int)
	near(t, "gzip marginal", float64(marginal), 3291, 60)
}

func TestStatsCountsDecisionStatesAsTheyAre(t *testing.T) {
	res := computeStats(loadFixture(t))
	states, ok := res["_dataset"]["decision_states"].(map[string]int)
	if !ok || states["candidate"] != 39 || states["planned"] != 3 || states["rejected"] != 0 {
		t.Errorf("decision_states = %#v", res["_dataset"]["decision_states"])
	}
}

func lesson(contrast, state string, rel int, facts []string, evidence, effort int) distill.Lesson {
	s := distill.Score{Relevance: rel, Facts: facts, Evidence: evidence, Effort: effort, Impact: distill.ImpactOf(facts)}
	f := distill.FinalScore(s)
	return distill.Lesson{Contrast: contrast, Score: s, FinalScore: &f, Decision: distill.Decision{State: state}}
}

func TestStatsReportsNoNegativesWhenNothingIsRejected(t *testing.T) {
	lessons := []distill.Lesson{
		lesson("new", "candidate", 3, []string{"a", "b"}, 2, 1),
		lesson("new", "planned", 3, []string{"a", "b", "c"}, 2, 1),
		lesson("extends", "candidate", 1, nil, 1, 1),
	}
	res := computeStats(lessons)
	if res["_dataset"]["decided"] != 1 {
		t.Fatalf("decided = %v", res["_dataset"]["decided"])
	}
	// Only the one planned lesson is decided; two of three lessons rank at least as high.
	block := res["association.final_score"]
	near(t, "chance", num(t, block["chance_k_random_lessons_rank_that_high"]), 1.0/3.0, 0.001)
}

func TestStatsHandlesADocumentWithNoDecisions(t *testing.T) {
	res := computeStats([]distill.Lesson{lesson("new", "candidate", 2, []string{"a"}, 1, 1)})
	if _, ok := res["association.impact"]["note"]; !ok {
		t.Errorf("expected a note when nothing is decided: %#v", res["association.impact"])
	}
}

func TestStatsEntropyIsZeroWhenEveryLessonIsAlreadyCovered(t *testing.T) {
	var lessons []distill.Lesson
	for i := 0; i < 4; i++ {
		lessons = append(lessons, lesson("already-covered", "candidate", 0, nil, 1, 1))
	}
	res := computeStats(lessons)
	if got := num(t, res["contrast"]["entropy_bits"]); got != 0 {
		t.Errorf("entropy = %v, want 0", got)
	}
}

func TestStatsLeavesCorrelationUndefinedWhenEveryScoreIsTied(t *testing.T) {
	var lessons []distill.Lesson
	for i := 0; i < 4; i++ {
		lessons = append(lessons, lesson("new", "candidate", 0, nil, 1, 1))
	}
	res := computeStats(lessons)
	if res["score.components"]["spearman_impact_final"] != nil {
		t.Errorf("want null for a constant column, got %v", res["score.components"]["spearman_impact_final"])
	}
	if res["score.ordering_sensitivity"]["tau_b_impact_alone"] != nil {
		t.Errorf("want null tau for tied scores")
	}
}

func TestStatsOutputIsDeterministic(t *testing.T) {
	var a, b bytes.Buffer
	args := []string{"stats", "--in", fixture, "--json"}
	if err := run(args, &a, &a); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &b, &b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("two runs printed different output")
	}
	var parsed map[string]any
	if err := json.Unmarshal(a.Bytes(), &parsed); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

func TestMidRanksShareTheMeanOfTiedPositions(t *testing.T) {
	got := midRanks([]float64{5, 3, 3, 1}, true)
	want := []float64{1, 2.5, 2.5, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranks = %v, want %v", got, want)
		}
	}
}

func TestKendallTauBMatchesAHandComputedCase(t *testing.T) {
	// Pairs: (1,2)c (1,3)c (2,3)d -> (2-1)/sqrt(3*3) = 1/3.
	got, ok := kendallTauB([]float64{1, 2, 3}, []float64{1, 3, 2})
	if !ok {
		t.Fatal("tau should be defined")
	}
	near(t, "tau", got, 1.0/3.0, 1e-9)
}

func TestChooseMatchesKnownValues(t *testing.T) {
	if choose(9, 3) != 84 || choose(42, 3) != 11480 {
		t.Errorf("choose(9,3)=%v choose(42,3)=%v", choose(9, 3), choose(42, 3))
	}
}

func TestStatsWritesOnlyWhereTheGuardAllows(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "stats.json")
	if err := run([]string{"stats", "--in", fixture, "--json", "--out", out}, os.Stdout, os.Stderr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output not written: %v", err)
	}
}
