package main

import "testing"

func TestCohenKappaMatchesAHandComputedTable(t *testing.T) {
	// po = 3/4, expected agreement = .5*.25 + .5*.75 = .5, so kappa = .5.
	got, ok := cohenKappa([]string{"x", "x", "y", "y"}, []string{"x", "y", "y", "y"})
	if !ok {
		t.Fatal("kappa should be defined")
	}
	near(t, "kappa", got, 0.5, 1e-9)
}

func TestWeightedKappaIsOneForIdenticalAndMinusOneForOpposite(t *testing.T) {
	one, _ := weightedKappa([]int{0, 1, 2, 3}, []int{0, 1, 2, 3}, 0, 3)
	near(t, "identical", one, 1, 1e-9)
	opp, _ := weightedKappa([]int{0, 1}, []int{1, 0}, 0, 1)
	near(t, "opposite", opp, -1, 1e-9)
}

func TestRecallAtKCountsPlannedLessonsFound(t *testing.T) {
	gold := map[string]bool{"a": true, "b": true, "c": true}
	near(t, "recall", recallAtK([]string{"a", "x", "c", "y", "z"}, gold), 2.0/3.0, 1e-9)
}

func TestPairwiseAgreementAveragesOverRepetitionPairs(t *testing.T) {
	runs := []map[string]string{{"a": "p", "b": "q"}, {"a": "p", "b": "r"}, {"a": "p", "b": "q"}}
	// pairs: (1,2)=.5 (1,3)=1 (2,3)=.5
	got, _ := pairwiseAgreement(runs)
	near(t, "agreement", got, 2.0/3.0, 1e-9)
}

func TestPairedDifferencesAreTakenRepetitionByRepetition(t *testing.T) {
	s := &Summary{Metrics: map[string]map[string]map[string]*Cell{}, Paired: map[string]map[string]map[string]*Paired{}}
	for rep, v := range []float64{2, 2, 2, 2, 2} {
		s.add("F3", "judge_mean", "full", rep+1, v)
	}
	for rep, v := range []float64{1, 1.2, 1, 3, 1} {
		s.add("F3", "judge_mean", "no-score", rep+1, v)
	}
	s.finish()
	p := s.Paired["F3"]["judge_mean"]["no-score"]
	if p == nil || p.N != 5 || p.Negative != 4 || p.Positive != 1 {
		t.Fatalf("paired = %#v", p)
	}
	near(t, "mean diff", p.Mean, -0.56, 1e-9)
}

func TestKeepRuleNeedsTheGateTheSignCountAndMoreThanTheFullSpread(t *testing.T) {
	th := Thresholds{JudgeScaleGate: 0.3, RateGate: 0.1, MinSameSignPairs: 4, KappaMin: 0.6, TauMin: 0.9}
	build := func(full, variant []float64) *Summary {
		s := &Summary{Metrics: map[string]map[string]map[string]*Cell{}, Paired: map[string]map[string]map[string]*Paired{}, JudgeNoise: map[string]any{}}
		for i := range full {
			s.add("F3", "judge_mean", "full", i+1, full[i])
			s.add("F3", "judge_mean", "no-notable", i+1, variant[i])
		}
		s.finish()
		return s
	}
	cases := []struct {
		name          string
		full, variant []float64
		want          bool
	}{
		{"clear drop", []float64{2, 2, 2, 2, 2}, []float64{1, 1, 1.2, 1, 1}, true},
		{"too small", []float64{2, 2, 2, 2, 2}, []float64{1.9, 1.9, 1.9, 1.9, 1.9}, false},
		{"only three pairs agree", []float64{2, 2, 2, 2, 2}, []float64{1, 1, 1, 2.5, 2.5}, false},
		{"drop smaller than the spread of full", []float64{1, 3, 1, 3, 1}, []float64{0.6, 2.6, 0.6, 2.6, 0.6}, false},
	}
	for _, c := range cases {
		got, _, ok := applyEffect(build(c.full, c.variant), "no-notable", th)
		if !ok || got != c.want {
			t.Errorf("%s: clears = %v (ran %v), want %v", c.name, got, ok, c.want)
		}
	}
}

func TestStaticEvidenceAloneNeverGivesMoreThanLowConfidence(t *testing.T) {
	stats := computeStats(loadFixture(t))
	b, _ := jsonRoundTrip(stats)
	rows := buildRows(b, nil, Thresholds{JudgeScaleGate: 0.3, RateGate: 0.1, MinSameSignPairs: 4, KappaMin: 0.6, TauMin: 0.9})
	if len(rows) != 8 {
		t.Fatalf("rows = %d, want 8", len(rows))
	}
	for _, r := range rows {
		if r.Confidence != "low" {
			t.Errorf("%s has confidence %q without in-use evidence", r.Field, r.Confidence)
		}
		if r.Triage != "not run" || r.Apply != "not run" {
			t.Errorf("%s should show not run, got %q / %q", r.Field, r.Triage, r.Apply)
		}
	}
}

func TestRecommendationCombinesInUseAndStaticEvidence(t *testing.T) {
	cases := []struct {
		ran, keep, rebuild, leanDrop bool
		rec, conf                    string
	}{
		{false, false, false, false, "keep optional (lean keep, static evidence only)", "low"},
		{true, true, false, false, "keep (required)", "high"},
		{true, true, false, true, "keep (required)", "low"},
		{true, false, true, true, "drop", "high"},
		{true, false, true, false, "drop", "low"},
		{true, false, false, false, "keep optional", "medium"},
	}
	for _, c := range cases {
		rec, conf := recommend(c.ran, c.keep, c.rebuild, c.leanDrop)
		if rec != c.rec || conf != c.conf {
			t.Errorf("recommend(%v) = %q/%q, want %q/%q", c, rec, conf, c.rec, c.conf)
		}
	}
}
