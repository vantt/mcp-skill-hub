package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/distill"
)

type f1Out struct {
	Top5   []string `json:"top5"`
	Labels []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	} `json:"labels"`
}

type f2Out struct {
	Lessons []struct {
		Key       string   `json:"key"`
		Contrast  *string  `json:"contrast"`
		Relevance *int     `json:"relevance"`
		Facts     []string `json:"facts"`
		Evidence  *int     `json:"evidence"`
		Effort    *int     `json:"effort"`
	} `json:"lessons"`
}

type f3Out struct {
	Answers []struct {
		Key            string `json:"key"`
		NoChangeNeeded bool   `json:"no_change_needed"`
		Change         string `json:"change"`
	} `json:"answers"`
}

type judgeOut struct {
	Scores []struct {
		Key        string `json:"key"`
		Mechanism  *int   `json:"mechanism"`
		InGoal     *int   `json:"in_goal"`
		NotInSkill *int   `json:"not_in_skill"`
		Placement  *int   `json:"placement"`
	} `json:"scores"`
}

// validateOutput checks an agent's answer against the run it belongs to, beyond what the schema
// guarantees: every expected lesson is covered, keys are real, and the fields asked for are present.
func validateOutput(r plannedRun, payload json.RawMessage) error {
	expect := map[string]bool{}
	for _, k := range r.Expected {
		expect[k] = true
	}
	covered := func(keys []string) error {
		seen := map[string]bool{}
		for _, k := range keys {
			if !expect[k] {
				return fmt.Errorf("unknown lesson key %q", k)
			}
			seen[k] = true
		}
		if len(seen) != len(expect) {
			return fmt.Errorf("answer covers %d of %d lessons", len(seen), len(expect))
		}
		return nil
	}
	switch {
	case r.Judge:
		var o judgeOut
		if err := json.Unmarshal(payload, &o); err != nil {
			return err
		}
		keys := make([]string, 0, len(o.Scores))
		for _, s := range o.Scores {
			keys = append(keys, s.Key)
			for _, p := range []*int{s.Mechanism, s.InGoal, s.NotInSkill, s.Placement} {
				if p == nil || *p < 0 || *p > 3 {
					return fmt.Errorf("score for %q is missing or outside 0-3", s.Key)
				}
			}
		}
		return covered(keys)
	case r.Family == "F1":
		var o f1Out
		if err := json.Unmarshal(payload, &o); err != nil {
			return err
		}
		if len(o.Top5) != 5 {
			return fmt.Errorf("top5 has %d entries, want 5", len(o.Top5))
		}
		keys := make([]string, 0, len(o.Labels))
		for _, l := range o.Labels {
			keys = append(keys, l.Key)
		}
		for _, k := range o.Top5 {
			if !expect[k] {
				return fmt.Errorf("top5 holds unknown key %q", k)
			}
		}
		return covered(keys)
	case r.Family == "F2":
		var o f2Out
		if err := json.Unmarshal(payload, &o); err != nil {
			return err
		}
		keys := make([]string, 0, len(o.Lessons))
		for _, l := range o.Lessons {
			keys = append(keys, l.Key)
			if r.Variant == variantNoContrast && l.Contrast == nil {
				return fmt.Errorf("lesson %q has no contrast", l.Key)
			}
			if r.Variant == variantNoScore && (l.Relevance == nil || l.Evidence == nil || l.Effort == nil) {
				return fmt.Errorf("lesson %q is missing a score field", l.Key)
			}
		}
		return covered(keys)
	case r.Family == "F3":
		var o f3Out
		if err := json.Unmarshal(payload, &o); err != nil {
			return err
		}
		keys := make([]string, 0, len(o.Answers))
		for _, a := range o.Answers {
			keys = append(keys, a.Key)
		}
		return covered(keys)
	}
	return fmt.Errorf("unknown family %q", r.Family)
}

// cohenKappa is the unweighted kappa of two label lists.
func cohenKappa(a, b []string) (float64, bool) {
	n := float64(len(a))
	if n == 0 {
		return 0, false
	}
	ca, cb, agree := map[string]float64{}, map[string]float64{}, 0.0
	for i := range a {
		ca[a[i]]++
		cb[b[i]]++
		if a[i] == b[i] {
			agree++
		}
	}
	po, pe := agree/n, 0.0
	for k, v := range ca {
		pe += (v / n) * (cb[k] / n)
	}
	if pe == 1 {
		return 0, false
	}
	return (po - pe) / (1 - pe), true
}

// weightedKappa is the quadratic-weighted kappa of two integer ratings on min..max.
func weightedKappa(a, b []int, lo, hi int) (float64, bool) {
	n := float64(len(a))
	k := hi - lo + 1
	if n == 0 || k < 2 {
		return 0, false
	}
	obs := make([][]float64, k)
	for i := range obs {
		obs[i] = make([]float64, k)
	}
	ra, rb := make([]float64, k), make([]float64, k)
	for i := range a {
		x, y := a[i]-lo, b[i]-lo
		obs[x][y]++
		ra[x]++
		rb[y]++
	}
	var num, den float64
	for i := 0; i < k; i++ {
		for j := 0; j < k; j++ {
			w := float64((i-j)*(i-j)) / float64((k-1)*(k-1))
			num += w * obs[i][j] / n
			den += w * (ra[i] / n) * (rb[j] / n)
		}
	}
	if den == 0 {
		return 0, false
	}
	return 1 - num/den, true
}

func recallAtK(top []string, gold map[string]bool) float64 {
	if len(gold) == 0 {
		return 0
	}
	hit := 0
	for _, k := range top {
		if gold[k] {
			hit++
		}
	}
	return float64(hit) / float64(len(gold))
}

func pairwiseAgreement(runs []map[string]string) (float64, bool) {
	var sum float64
	var pairs int
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			same := 0
			for k, v := range runs[i] {
				if runs[j][k] == v {
					same++
				}
			}
			if len(runs[i]) > 0 {
				sum += float64(same) / float64(len(runs[i]))
				pairs++
			}
		}
	}
	if pairs == 0 {
		return 0, false
	}
	return sum / float64(pairs), true
}

// Cell holds one metric of one variant over the repetitions.
type Cell struct {
	ByRep map[int]float64 `json:"by_rep"`
	Mean  float64         `json:"mean"`
	Min   float64         `json:"min"`
	Max   float64         `json:"max"`
}

// Paired is the difference variant minus full over repetitions that both have.
type Paired struct {
	Mean     float64 `json:"mean"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Negative int     `json:"negative"`
	Positive int     `json:"positive"`
	N        int     `json:"n"`
}

// Summary is the in-use evidence; metrics are family -> metric -> variant.
type Summary struct {
	Metrics    map[string]map[string]map[string]*Cell   `json:"metrics"`
	Paired     map[string]map[string]map[string]*Paired `json:"paired_vs_full"`
	JudgeNoise map[string]any                           `json:"judge_noise"`
	Runs       map[string]map[string]int                `json:"runs"`
}

func (s *Summary) add(family, metric, variant string, rep int, v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	if s.Metrics[family] == nil {
		s.Metrics[family] = map[string]map[string]*Cell{}
	}
	if s.Metrics[family][metric] == nil {
		s.Metrics[family][metric] = map[string]*Cell{}
	}
	c := s.Metrics[family][metric][variant]
	if c == nil {
		c = &Cell{ByRep: map[int]float64{}}
		s.Metrics[family][metric][variant] = c
	}
	c.ByRep[rep] = round4(v)
}

func (s *Summary) finish() {
	for fam, metrics := range s.Metrics {
		for metric, variants := range metrics {
			for _, c := range variants {
				vals := make([]float64, 0, len(c.ByRep))
				for _, v := range c.ByRep {
					vals = append(vals, v)
				}
				c.Mean, c.Min, c.Max = round4(mean(vals)), round4(minOf(vals)), round4(maxOf(vals))
			}
			full := variants[variantFull]
			if full == nil {
				continue
			}
			for name, c := range variants {
				if name == variantFull || len(c.ByRep) < 2 && len(full.ByRep) < 2 {
					continue
				}
				p := &Paired{}
				var diffs []float64
				for rep, v := range c.ByRep {
					if fv, ok := full.ByRep[rep]; ok {
						diffs = append(diffs, v-fv)
					}
				}
				if len(diffs) == 0 {
					continue
				}
				for _, d := range diffs {
					switch {
					case d < 0:
						p.Negative++
					case d > 0:
						p.Positive++
					}
				}
				p.N, p.Mean, p.Min, p.Max = len(diffs), round4(mean(diffs)), round4(minOf(diffs)), round4(maxOf(diffs))
				if s.Paired[fam] == nil {
					s.Paired[fam] = map[string]map[string]*Paired{}
				}
				if s.Paired[fam][metric] == nil {
					s.Paired[fam][metric] = map[string]*Paired{}
				}
				s.Paired[fam][metric][name] = p
			}
		}
	}
}

func minOf(v []float64) float64 {
	m := math.Inf(1)
	for _, x := range v {
		m = math.Min(m, x)
	}
	return m
}

func loadOutput[T any](sandbox, id string) (T, bool) {
	var out T
	b, err := os.ReadFile(filepath.Join(sandbox, "runs", id, "output.json"))
	if err != nil {
		return out, false
	}
	return out, json.Unmarshal(b, &out) == nil
}

func judgeMean(o judgeOut) float64 {
	var sum float64
	for _, s := range o.Scores {
		sum += float64(*s.Mechanism+*s.InGoal+*s.NotInSkill+*s.Placement) / 4
	}
	if len(o.Scores) == 0 {
		return math.NaN()
	}
	return sum / float64(len(o.Scores))
}

// summarize scores every finished run of a sandbox.
func summarize(sandbox string, sc Scenarios) (*Summary, error) {
	in, err := readInputs(sandbox)
	if err != nil {
		return nil, err
	}
	gold := map[string]goldLesson{}
	planned := map[string]bool{}
	for _, g := range in.gold {
		gold[g.Key] = g
		if g.State == "planned" {
			planned[g.Key] = true
		}
	}
	s := &Summary{Metrics: map[string]map[string]map[string]*Cell{}, Paired: map[string]map[string]map[string]*Paired{},
		Runs: map[string]map[string]int{}}
	f1Labels := map[string][]map[string]string{}
	for _, f := range sc.Families {
		s.Runs[f.ID] = map[string]int{"ok": 0, "failed": 0, "planned": len(f.Variants) * f.Repetitions}
		for _, v := range f.Variants {
			for rep := 1; rep <= f.Repetitions; rep++ {
				id := fmt.Sprintf("%s-%s-r%d", strings.ToLower(f.ID), v, rep)
				if m, ok := readMeta(filepath.Join(sandbox, "runs", id)); ok {
					s.Runs[f.ID][m.Status]++
				}
				switch f.ID {
				case "F1":
					if o, ok := loadOutput[f1Out](sandbox, id); ok {
						labels := map[string]string{}
						for _, l := range o.Labels {
							labels[l.Key] = l.Label
						}
						f1Labels[v] = append(f1Labels[v], labels)
						scoreF1(s, v, rep, o, in.gold, planned)
					}
				case "F2":
					if o, ok := loadOutput[f2Out](sandbox, id); ok {
						scoreF2(s, v, rep, o, gold)
					}
				case "F3":
					if j, ok := loadOutput[judgeOut](sandbox, "judge-"+id); ok {
						s.add("F3", "judge_mean", v, rep, judgeMean(j))
					}
				}
			}
		}
	}
	for v, runs := range f1Labels {
		if a, ok := pairwiseAgreement(runs); ok {
			s.add("F1", "label_stability", v, 0, a)
		}
	}
	s.JudgeNoise = judgeNoise(sandbox)
	s.finish()
	return s, nil
}

func scoreF1(s *Summary, v string, rep int, o f1Out, gold []goldLesson, planned map[string]bool) {
	s.add("F1", "recall_at_5_planned", v, rep, recallAtK(o.Top5, planned))
	got := map[string]string{}
	for _, l := range o.Labels {
		got[l.Key] = l.Label
	}
	var ac, og, n float64
	for _, g := range gold {
		n++
		if (got[g.Key] == "already-covered") == (g.Contrast == "already-covered") {
			ac++
		}
		if (got[g.Key] == "outside-goal") == (g.Relevance == 0) {
			og++
		}
	}
	if n > 0 {
		s.add("F1", "already_covered_agreement", v, rep, ac/n)
		s.add("F1", "outside_goal_agreement", v, rep, og/n)
	}
}

func scoreF2(s *Summary, v string, rep int, o f2Out, gold map[string]goldLesson) {
	var contrastA, contrastB []string
	var rel, relG, evi, eviG, eff, effG []int
	var factsSame, impactSame, n float64
	var recomputed, stored []float64
	for _, l := range o.Lessons {
		g, ok := gold[l.Key]
		if !ok {
			continue
		}
		n++
		if v == variantNoContrast && l.Contrast != nil {
			contrastA, contrastB = append(contrastA, *l.Contrast), append(contrastB, g.Contrast)
			continue
		}
		if v != variantNoScore || l.Relevance == nil || l.Evidence == nil || l.Effort == nil {
			continue
		}
		rel, relG = append(rel, *l.Relevance), append(relG, g.Relevance)
		evi, eviG = append(evi, *l.Evidence), append(eviG, g.Evidence)
		eff, effG = append(eff, *l.Effort), append(effG, g.Effort)
		if sameSet(l.Facts, g.Facts) {
			factsSame++
		}
		impact := distill.ImpactOf(l.Facts)
		if impact == g.Impact {
			impactSame++
		}
		sc := distill.Score{Relevance: *l.Relevance, Facts: l.Facts, Evidence: *l.Evidence, Effort: *l.Effort}
		recomputed, stored = append(recomputed, distill.FinalScore(sc)), append(stored, g.Final)
	}
	if k, ok := cohenKappa(contrastA, contrastB); ok {
		s.add("F2", "contrast_kappa", v, rep, k)
	}
	for name, p := range map[string][3]any{"relevance_kappa": {rel, relG, [2]int{0, 3}}, "evidence_kappa": {evi, eviG, [2]int{1, 3}},
		"effort_kappa": {eff, effG, [2]int{1, 3}}} {
		a, b, r := p[0].([]int), p[1].([]int), p[2].([2]int)
		if k, ok := weightedKappa(a, b, r[0], r[1]); ok {
			s.add("F2", name, v, rep, k)
		}
	}
	if len(recomputed) > 0 && n > 0 {
		s.add("F2", "facts_exact_agreement", v, rep, factsSame/float64(len(recomputed)))
		s.add("F2", "impact_exact_agreement", v, rep, impactSame/float64(len(recomputed)))
		if rho, ok := spearman(recomputed, stored); ok {
			s.add("F2", "final_score_spearman", v, rep, rho)
		}
	}
}

func sameSet(a, b []string) bool {
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, ",") == strings.Join(y, ",")
}

// judgeNoise compares the repeated judgements of one answer set.
func judgeNoise(sandbox string) map[string]any {
	var scores []float64
	for _, id := range []string{"judge-f3-full-r1"} {
		if j, ok := loadOutput[judgeOut](sandbox, id); ok {
			scores = append(scores, judgeMean(j))
		}
	}
	for n := 1; n <= 20; n++ {
		if j, ok := loadOutput[judgeOut](sandbox, fmt.Sprintf("judge-noise-%d", n)); ok {
			scores = append(scores, judgeMean(j))
		}
	}
	if len(scores) < 2 {
		return map[string]any{"judgements": len(scores)}
	}
	return map[string]any{"judgements": len(scores), "mean": round4(mean(scores)), "spread": round4(maxOf(scores) - minOf(scores))}
}
