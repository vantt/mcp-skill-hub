package main

import (
	"math"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/distill"
)

// Result maps a field name to its metrics. encoding/json sorts the keys, so output is deterministic.
type Result map[string]map[string]any

func finalOf(l distill.Lesson) float64 {
	if l.FinalScore != nil {
		return *l.FinalScore
	}
	return distill.FinalScore(l.Score)
}

// column returns one numeric value per lesson.
func column(lessons []distill.Lesson, f func(distill.Lesson) float64) []float64 {
	out := make([]float64, len(lessons))
	for i, l := range lessons {
		out[i] = f(l)
	}
	return out
}

func labels(lessons []distill.Lesson, f func(distill.Lesson) string) []string {
	out := make([]string, len(lessons))
	for i, l := range lessons {
		out[i] = f(l)
	}
	return out
}

func factsKey(l distill.Lesson) string {
	f := append([]string(nil), l.Score.Facts...)
	sort.Strings(f)
	return strings.Join(f, "")
}

// computeStats runs every method over the lessons of one document.
func computeStats(lessons []distill.Lesson) Result {
	res := Result{}
	res["_dataset"] = datasetBlock(lessons)
	for name, block := range associationBlocks(lessons) {
		res["association."+name] = block
	}
	res["association.contrast_by_state"] = contrastByState(lessons)
	res["notable"] = notableBlock(lessons)
	res["contrast"] = contrastBlock(lessons)
	res["score.components"] = componentBlock(lessons)
	res["score.ordering_sensitivity"] = orderingBlock(lessons)
	res["score.why"] = whyBlock(lessons)
	return res
}

func datasetBlock(lessons []distill.Lesson) map[string]any {
	states := map[string]int{}
	for _, l := range lessons {
		states[l.Decision.State]++
	}
	return map[string]any{
		"lessons":         len(lessons),
		"decision_states": states,
		"decided":         decidedCount(lessons),
		"weaknesses": []string{
			"few lessons have a decision, so any association rests on a handful of cases",
			"candidate means undecided, not rejected: there are no negatives unless a lesson is rejected",
			"decisions were made with the scores visible, so a high rank is partly circular",
			"this method can never support a decision alone",
		},
	}
}

func decidedCount(lessons []distill.Lesson) int {
	n := 0
	for _, l := range lessons {
		if l.Decision.State != "candidate" {
			n++
		}
	}
	return n
}

// associationBlocks ranks lessons that carry a decision against every scored field (method A).
// The probability is exact: the chance that k lessons drawn at random all rank at least as high
// as the lowest-ranked decided lesson.
func associationBlocks(lessons []distill.Lesson) map[string]map[string]any {
	fields := map[string]func(distill.Lesson) float64{
		"relevance":   func(l distill.Lesson) float64 { return float64(l.Score.Relevance) },
		"impact":      func(l distill.Lesson) float64 { return float64(distill.ImpactOf(l.Score.Facts)) },
		"evidence":    func(l distill.Lesson) float64 { return float64(l.Score.Evidence) },
		"effort":      func(l distill.Lesson) float64 { return -float64(l.Score.Effort) }, // lower effort is better
		"final_score": finalOf,
	}
	out := map[string]map[string]any{}
	for name, f := range fields {
		out[name] = associationBlock(lessons, column(lessons, f))
	}
	return out
}

func associationBlock(lessons []distill.Lesson, values []float64) map[string]any {
	block := map[string]any{"status": "preliminary", "higher_is_better": true}
	var decided []int
	for i, l := range lessons {
		if l.Decision.State != "candidate" {
			decided = append(decided, i)
		}
	}
	k, n := len(decided), len(lessons)
	block["decided"] = k
	if k == 0 {
		block["note"] = "no lesson has a decision yet"
		return block
	}
	ranks := midRanks(values, true)
	minVal := math.Inf(1)
	ranksOut := make([]float64, 0, k)
	for _, i := range decided {
		minVal = math.Min(minVal, values[i])
		ranksOut = append(ranksOut, ranks[i])
	}
	sort.Float64s(ranksOut)
	atLeast := 0
	for _, v := range values {
		if v >= minVal {
			atLeast++
		}
	}
	block["decided_midranks"] = ranksOut
	block["lessons_ranking_at_least_as_high_as_the_lowest_decided"] = atLeast
	block["chance_k_random_lessons_rank_that_high"] = round4(choose(atLeast, k) / choose(n, k))
	return block
}

func contrastByState(lessons []distill.Lesson) map[string]any {
	table := map[string]map[string]int{}
	for _, l := range lessons {
		if table[l.Contrast] == nil {
			table[l.Contrast] = map[string]int{}
		}
		table[l.Contrast][l.Decision.State]++
	}
	return map[string]any{"status": "preliminary", "table": table}
}

func notableBlock(lessons []distill.Lesson) map[string]any {
	var jac, newShare, notableWords []float64
	var allWhat, allBoth strings.Builder
	var notableTexts []string
	for _, l := range lessons {
		w, nt := contentWordSet(l.What), contentWordSet(l.Notable)
		jac = append(jac, jaccard(wordSet(l.What), wordSet(l.Notable)))
		missing := 0
		for x := range nt {
			if !w[x] {
				missing++
			}
		}
		if len(nt) > 0 {
			newShare = append(newShare, float64(missing)/float64(len(nt)))
		}
		notableWords = append(notableWords, float64(len(words(l.Notable))))
		allWhat.WriteString(l.What + "\n")
		allBoth.WriteString(l.What + "\n" + l.Notable + "\n")
		notableTexts = append(notableTexts, l.Notable)
	}
	gzWhat, gzBoth := gzipSize(allWhat.String()), gzipSize(allBoth.String())
	return map[string]any{
		"jaccard_with_what_median":                  optional(median(jac), len(jac) > 0),
		"jaccard_with_what_max":                     optional(maxOf(jac), len(jac) > 0),
		"share_of_content_words_not_in_what_median": optional(median(newShare), len(newShare) > 0),
		"words_median":                              optional(median(notableWords), len(notableWords) > 0),
		"gzip_bytes_what":                           gzWhat,
		"gzip_bytes_what_and_notable":               gzBoth,
		"gzip_marginal_bytes_of_notable":            gzBoth - gzWhat,
		"trigrams_shared_by_3_or_more":              sharedTrigrams(notableTexts, 3),
		"weakness":                                  "word overlap misses paraphrase, and gzip is a crude entropy proxy on short texts",
	}
}

func maxOf(v []float64) float64 {
	m := math.Inf(-1)
	for _, x := range v {
		m = math.Max(m, x)
	}
	return m
}

func contrastBlock(lessons []distill.Lesson) map[string]any {
	counts := map[string]int{}
	for _, l := range lessons {
		counts[l.Contrast]++
	}
	contrast := labels(lessons, func(l distill.Lesson) string { return l.Contrast })
	zeroImpact := labels(lessons, func(l distill.Lesson) string {
		if distill.ImpactOf(l.Score.Facts) == 0 {
			return "impact0"
		}
		return "impact>0"
	})
	hasB := labels(lessons, func(l distill.Lesson) string {
		for _, f := range l.Score.Facts {
			if f == "b" {
				return "b"
			}
		}
		return "no-b"
	})
	facts := labels(lessons, factsKey)
	return map[string]any{
		"counts":                       counts,
		"entropy_bits":                 round4(entropy(counts)),
		"entropy_bits_given_impact0":   round4(conditionalEntropy(contrast, zeroImpact)),
		"entropy_bits_given_fact_b":    round4(conditionalEntropy(contrast, hasB)),
		"entropy_bits_given_all_facts": round4(conditionalEntropy(contrast, facts)),
		"cross_table_with_impact0":     crossTable(contrast, zeroImpact),
		"cross_table_with_fact_b":      crossTable(contrast, hasB),
		"weakness":                     "entropy says how much contrast varies and how much the facts imply, not whether it helps an agent",
	}
}

func crossTable(a, b []string) map[string]map[string]int {
	t := map[string]map[string]int{}
	for i := range a {
		if t[a[i]] == nil {
			t[a[i]] = map[string]int{}
		}
		t[a[i]][b[i]]++
	}
	return t
}

func componentBlock(lessons []distill.Lesson) map[string]any {
	dist := func(f func(distill.Lesson) int) map[string]int {
		m := map[string]int{}
		for _, l := range lessons {
			m[itoa(f(l))]++
		}
		return m
	}
	rel := column(lessons, func(l distill.Lesson) float64 { return float64(l.Score.Relevance) })
	imp := column(lessons, func(l distill.Lesson) float64 { return float64(distill.ImpactOf(l.Score.Facts)) })
	evi := column(lessons, func(l distill.Lesson) float64 { return float64(l.Score.Evidence) })
	eff := column(lessons, func(l distill.Lesson) float64 { return float64(l.Score.Effort) })
	fin := column(lessons, finalOf)
	zero := 0
	for _, v := range fin {
		if v == 0 {
			zero++
		}
	}
	sp := func(x, y []float64) any { v, ok := spearman(x, y); return optional(v, ok) }
	return map[string]any{
		"relevance_distribution":    dist(func(l distill.Lesson) int { return l.Score.Relevance }),
		"impact_distribution":       dist(func(l distill.Lesson) int { return distill.ImpactOf(l.Score.Facts) }),
		"evidence_distribution":     dist(func(l distill.Lesson) int { return l.Score.Evidence }),
		"effort_distribution":       dist(func(l distill.Lesson) int { return l.Score.Effort }),
		"final_score_zero":          zero,
		"spearman_impact_final":     sp(imp, fin),
		"spearman_relevance_final":  sp(rel, fin),
		"spearman_evidence_final":   sp(evi, fin),
		"spearman_effort_final":     sp(eff, fin),
		"spearman_relevance_impact": sp(rel, imp),
		"weakness":                  "correlations describe this one file; ties at zero make the order among zero-score lessons arbitrary",
	}
}

// orderingBlock measures how much each component moves the ranking: Kendall tau-b between the
// stored score and the score recomputed with one component held at 1 (or impact alone).
func orderingBlock(lessons []distill.Lesson) map[string]any {
	stored := column(lessons, finalOf)
	recompute := func(rel, imp, evi, eff bool) []float64 {
		return column(lessons, func(l distill.Lesson) float64 {
			r, i, e, f := float64(l.Score.Relevance), float64(distill.ImpactOf(l.Score.Facts)), float64(l.Score.Evidence), float64(l.Score.Effort)
			if rel {
				r = 1
			}
			if imp {
				i = 1
			}
			if evi {
				e = 1
			}
			if eff {
				f = 1
			}
			if f <= 0 {
				return 0
			}
			return r * i * e / f
		})
	}
	tau := func(y []float64) any { v, ok := kendallTauB(stored, y); return optional(v, ok) }
	impactOnly := column(lessons, func(l distill.Lesson) float64 { return float64(distill.ImpactOf(l.Score.Facts)) })
	return map[string]any{
		"tau_b_relevance_held_constant": tau(recompute(true, false, false, false)),
		"tau_b_impact_held_constant":    tau(recompute(false, true, false, false)),
		"tau_b_evidence_held_constant":  tau(recompute(false, false, true, false)),
		"tau_b_effort_held_constant":    tau(recompute(false, false, false, true)),
		"tau_b_impact_alone":            tau(impactOnly),
		"weakness":                      "this shows how far a component moves the ranking, not whether the ranking is right",
	}
}

func whyBlock(lessons []distill.Lesson) map[string]any {
	var counts []float64
	var texts []string
	for _, l := range lessons {
		counts = append(counts, float64(len(words(l.Score.Why))))
		texts = append(texts, l.Score.Why)
	}
	return map[string]any{
		"words_median":                 optional(median(counts), len(counts) > 0),
		"trigrams_shared_by_3_or_more": sharedTrigrams(texts, 3),
	}
}
