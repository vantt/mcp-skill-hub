package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed thresholds.json
var defaultThresholds []byte

// Thresholds are the user's decision gates; the defaults are proposals.
type Thresholds struct {
	JudgeScaleGate   float64 `json:"judge_scale_gate"`
	RateGate         float64 `json:"rate_gate"`
	MinSameSignPairs int     `json:"min_same_sign_pairs"`
	KappaMin         float64 `json:"kappa_min"`
	TauMin           float64 `json:"tau_min"`
}

// Row is one line of the scorecard.
type Row struct {
	Field          string `json:"field"`
	Association    string `json:"a_decision_association"`
	Information    string `json:"b_information_or_ordering"`
	Triage         string `json:"c1_triage_vs_full"`
	Reconstruct    string `json:"c2_reconstruct_agreement"`
	Apply          string `json:"c3_apply_vs_full"`
	Consumers      string `json:"consumers"`
	Recommendation string `json:"recommendation"`
	Confidence     string `json:"confidence"`
	IfDropped      string `json:"if_dropped"`
}

type fieldSpec struct {
	name        string
	variant     string // variant that removes the field; empty when none tests it
	reconstruct string // F2 metric, empty when the field cannot be reconstructed
	orderingTau string // ordering-sensitivity metric used as drop evidence
	consumers   string
	ifDropped   string
	static      func(Result) (text string, leanDrop bool)
}

const consumersScore = "distill.mjs:665-700,778-795; distill_model.go:228-253; curation_home.go:375; distill.schema.json:125; DistillTab.tsx:212-235"

func fieldSpecs() []fieldSpec {
	tauOf := func(key string) func(Result) (string, bool) {
		return func(r Result) (string, bool) {
			v, ok := r["score.ordering_sensitivity"][key].(float64)
			if !ok {
				return "tau-b not defined", false
			}
			return fmt.Sprintf("tau-b %.2f when held constant", v), v >= 0.9
		}
	}
	return []fieldSpec{
		{name: "notable", variant: variantNoNotable,
			consumers: "distill.mjs:765-767; distill_model.go:195-197; distill.schema.json:94; DistillTab.tsx:212",
			ifDropped: "strip on `format`; remove from writer, Go struct, schema, tab and SKILL.md step 5; loses the only why-it-matters prose",
			static: func(r Result) (string, bool) {
				share, _ := r["notable"]["share_of_content_words_not_in_what_median"].(float64)
				return fmt.Sprintf("%.0f%% of its content words are not in `what`; adds %v gzip bytes", share*100, r["notable"]["gzip_marginal_bytes_of_notable"]), share < 0.5
			}},
		{name: "contrast", variant: variantNoContrast, reconstruct: "contrast_kappa",
			consumers: "distill.mjs:37,775-777,2020; distill_model.go:31,224-226; v4.go:343; list goldens",
			ifDropped: "strip on `format`; `list` loses a column and its goldens; fact b needs a new definition; separates 8 already-covered lessons from 14 new-but-outside-goal ones",
			static: func(r Result) (string, bool) {
				h, _ := r["contrast"]["entropy_bits_given_all_facts"].(float64)
				return fmt.Sprintf("%.2f bits left after the facts (of %.2f)", h, r["contrast"]["entropy_bits"]), h < 0.3
			}},
		{name: "score.relevance", variant: variantNoScore, reconstruct: "relevance_kappa", orderingTau: "tau_b_relevance_held_constant",
			consumers: consumersScore, ifDropped: "formula and `final_score` change for every lesson; Home high-value count needs a new rule (public JSON)",
			static: tauOf("tau_b_relevance_held_constant")},
		{name: "score.facts / impact", variant: variantNoScore, reconstruct: "impact_exact_agreement", orderingTau: "tau_b_impact_held_constant",
			consumers: consumersScore, ifDropped: "impact decides the ranking (alone it reproduces most of the order); dropping it removes the ordering",
			static: tauOf("tau_b_impact_held_constant")},
		{name: "score.evidence", variant: variantNoScore, reconstruct: "evidence_kappa", orderingTau: "tau_b_evidence_held_constant",
			consumers: consumersScore, ifDropped: "formula and `final_score` change for every lesson; one reorder on `format`",
			static: tauOf("tau_b_evidence_held_constant")},
		{name: "score.effort", variant: variantNoScore, reconstruct: "effort_kappa", orderingTau: "tau_b_effort_held_constant",
			consumers: consumersScore, ifDropped: "formula and `final_score` change for every lesson; one reorder on `format`",
			static: tauOf("tau_b_effort_held_constant")},
		{name: "final_score (ordering)", variant: variantNoScore, orderingTau: "tau_b_impact_alone",
			consumers: consumersScore, ifDropped: "strip `score` and `final_score` on `format`; `list` order becomes layer then key; `pending_high_value_insights` needs a new rule",
			static: tauOf("tau_b_impact_alone")},
		{name: "R/E/F as a block", variant: variantNoScore,
			consumers: consumersScore, ifDropped: "remove impactOf, finalScore, sort, Go Score type, schema block, tab row and about 60 lines of SKILL.md",
			static: func(r Result) (string, bool) {
				v, ok := r["score.components"]["spearman_impact_final"].(float64)
				if !ok {
					return "", false
				}
				return fmt.Sprintf("final_score tracks impact (Spearman %.2f); %v of %v lessons score 0", v, r["score.components"]["final_score_zero"], r["_dataset"]["lessons"]), false
			}},
	}
}

func fmtPaired(p *Paired) string {
	return fmt.Sprintf("%+.2f (%+.2f..%+.2f), %d of %d lower", p.Mean, p.Min, p.Max, p.Negative, p.N)
}

// clears applies the keep rule to one paired metric: a drop of at least gate, in at least
// MinSameSignPairs pairs, larger than the spread of the full variant on its own.
func clears(sum *Summary, family, metric, variant string, gate float64, th Thresholds) (bool, string, bool) {
	p := sum.Paired[family][metric][variant]
	if p == nil {
		return false, "", false
	}
	spread := 0.0
	if full := sum.Metrics[family][metric][variantFull]; full != nil {
		spread = full.Max - full.Min
	}
	ok := -p.Mean >= gate && p.Negative >= th.MinSameSignPairs && -p.Mean > spread
	return ok, fmt.Sprintf("%s %s", metric, fmtPaired(p)), true
}

func triageEffect(sum *Summary, variant string, th Thresholds) (clear bool, text string, ran bool) {
	var parts []string
	for _, m := range []string{"recall_at_5_planned", "already_covered_agreement", "outside_goal_agreement"} {
		c, t, ok := clears(sum, "F1", m, variant, th.RateGate, th)
		if !ok {
			continue
		}
		ran = true
		clear = clear || c
		parts = append(parts, t)
	}
	return clear, strings.Join(parts, "; "), ran
}

func applyEffect(sum *Summary, variant string, th Thresholds) (bool, string, bool) {
	gate := th.JudgeScaleGate
	if spread, ok := sum.JudgeNoise["spread"].(float64); ok && spread > gate {
		gate = spread
	}
	return clears(sum, "F3", "judge_mean", variant, gate, th)
}

func reconstructText(sum *Summary, metric, variant string, th Thresholds) (string, bool, bool) {
	if metric == "" {
		return "not testable", false, false
	}
	c := sum.Metrics["F2"][metric][variant]
	if c == nil {
		return "not run", false, false
	}
	return fmt.Sprintf("%s %.2f (%.2f..%.2f)", metric, c.Mean, c.Min, c.Max), c.Mean >= th.KappaMin, true
}

// buildRows assembles the scorecard. Missing in-use evidence renders as "not run" and caps
// confidence at low.
func buildRows(stats Result, sum *Summary, th Thresholds) []Row {
	var rows []Row
	for _, f := range fieldSpecs() {
		row := Row{Field: f.name, Consumers: f.consumers, IfDropped: f.ifDropped,
			Triage: "not run", Reconstruct: "not run", Apply: "not run"}
		row.Association = associationText(stats, f.name)
		staticText, leanDrop := f.static(stats)
		row.Information = staticText
		keep, ran := false, false
		rebuild := false
		if sum != nil {
			if c, t, ok := triageEffect(sum, f.variant, th); ok {
				row.Triage, keep, ran = t, keep || c, true
			}
			if c, t, ok := applyEffect(sum, f.variant, th); ok {
				row.Apply, keep, ran = t, keep || c, true
			}
			text, rb, _ := reconstructText(sum, f.reconstruct, f.variant, th)
			row.Reconstruct, rebuild = text, rb
		} else if f.reconstruct == "" {
			row.Reconstruct = "not testable"
		}
		if f.orderingTau != "" && f.reconstruct == "" {
			if tau, ok := stats["score.ordering_sensitivity"][f.orderingTau].(float64); ok && tau >= th.TauMin {
				rebuild = true
				row.Reconstruct = fmt.Sprintf("ordering tau-b %.2f", tau)
			}
		}
		row.Recommendation, row.Confidence = recommend(ran, keep, rebuild, leanDrop)
		rows = append(rows, row)
	}
	return rows
}

func recommend(ran, keep, rebuild, leanDrop bool) (string, string) {
	if !ran {
		lean := "lean keep"
		if leanDrop {
			lean = "lean drop"
		}
		return "keep optional (" + lean + ", static evidence only)", "low"
	}
	switch {
	case keep && !leanDrop:
		return "keep (required)", "high"
	case keep:
		return "keep (required)", "low"
	case rebuild && leanDrop:
		return "drop", "high"
	case rebuild:
		return "drop", "low"
	}
	return "keep optional", "medium"
}

func associationText(stats Result, field string) string {
	key := map[string]string{"score.relevance": "relevance", "score.facts / impact": "impact", "score.evidence": "evidence",
		"score.effort": "effort", "final_score (ordering)": "final_score", "R/E/F as a block": "final_score"}[field]
	if key == "" {
		return "n/a (text field)"
	}
	b := stats["association."+key]
	chance, ok := b["chance_k_random_lessons_rank_that_high"].(float64)
	if !ok {
		return "no decided lessons"
	}
	return fmt.Sprintf("preliminary: chance %.3f by luck, %v decided", chance, b["decided"])
}

const limits = "Limits: one skill (test-audit), 42 lessons, one model, `claude -p` mode, 5 paired repetitions (a sign test cannot reach p < 0.05 with 5 pairs, so the gate is an effect size), a biased gold (3 planned lessons chosen with the scores visible), a judge that is itself a model, and lexical information measures. Static evidence alone never gives more than low confidence."

func renderMarkdown(rows []Row, th Thresholds, manifest map[string]string) string {
	var b strings.Builder
	b.WriteString("# distill-lab scorecard\n\n")
	b.WriteString("| Field | A: decision association (preliminary) | B: information / ordering | C1 triage vs full | C2 reconstruct | C3 apply vs full | Consumers | Recommendation | Confidence | If dropped |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", r.Field, cell(r.Association), cell(r.Information),
			cell(r.Triage), cell(r.Reconstruct), cell(r.Apply), cell(r.Consumers), cell(r.Recommendation), r.Confidence, cell(r.IfDropped))
	}
	fmt.Fprintf(&b, "\nThresholds (your decision): a drop of at least %.2f on the 0-3 judge scale (or the measured judge noise, if larger) or %.0f percentage points on a rate, with the same sign in at least %d paired repetitions and larger than the spread of the full variant; reconstruct agreement (kappa) at least %.2f; ordering tau-b at least %.2f.\n\n",
		th.JudgeScaleGate, th.RateGate*100, th.MinSameSignPairs, th.KappaMin, th.TauMin)
	b.WriteString(limits + "\n\n")
	if len(manifest) > 0 {
		b.WriteString("## Run manifest (sha256)\n\n")
		names := make([]string, 0, len(manifest))
		for n := range manifest {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(&b, "- `%s` %s\n", n, manifest[n])
		}
	}
	return b.String()
}

func cell(s string) string { return strings.ReplaceAll(s, "|", "/") }

func loadThresholds(path string) (Thresholds, error) {
	data := defaultThresholds
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Thresholds{}, err
		}
		data = b
	}
	var th Thresholds
	return th, json.Unmarshal(data, &th)
}

func runReport(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	statsPath := fs.String("stats", "", "stats JSON made by `stats --json`")
	sandbox := fs.String("runs", "", "sandbox with finished runs (omit when no in-use run was made)")
	thPath := fs.String("thresholds", "", "thresholds file (default: the embedded proposal)")
	scPath := fs.String("scenarios", "", "scenario file (default: the embedded one)")
	out := fs.String("out", "", "folder to write scorecard.md and scorecard.json into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *statsPath == "" || *out == "" {
		return fmt.Errorf("--stats and --out are required")
	}
	if err := checkOutsideHub(*out); err != nil {
		return err
	}
	raw, err := os.ReadFile(*statsPath)
	if err != nil {
		return err
	}
	var stats Result
	if err := json.Unmarshal(raw, &stats); err != nil {
		return fmt.Errorf("stats: %w", err)
	}
	normalizeStats(stats)
	th, err := loadThresholds(*thPath)
	if err != nil {
		return err
	}
	var sum *Summary
	manifest := map[string]string{}
	if *sandbox != "" {
		sc, err := loadScenarios(*scPath)
		if err != nil {
			return err
		}
		if sum, err = summarize(*sandbox, sc); err != nil {
			return err
		}
		manifest = readManifest(*sandbox)
		sb, _ := json.MarshalIndent(sum, "", "  ")
		if err := os.MkdirAll(filepath.Join(*out, "inuse"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, "inuse", "summary.json"), append(sb, '\n'), 0o644); err != nil {
			return err
		}
	}
	rows := buildRows(stats, sum, th)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	jb, _ := json.MarshalIndent(map[string]any{"rows": rows, "thresholds": th, "limits": limits, "manifest": manifest}, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "scorecard.json"), append(jb, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "scorecard.md"), []byte(renderMarkdown(rows, th, manifest)), 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote %s and %s\n", filepath.Join(*out, "scorecard.md"), filepath.Join(*out, "scorecard.json"))
	return err
}

func readManifest(sandbox string) map[string]string {
	b, err := os.ReadFile(filepath.Join(sandbox, "manifest.json"))
	if err != nil {
		return map[string]string{}
	}
	var m struct {
		Sha map[string]string `json:"sha256"`
	}
	_ = json.Unmarshal(b, &m)
	return m.Sha
}

// normalizeStats makes a stats file read back from JSON look like a freshly computed one:
// numbers are float64 either way.
func normalizeStats(r Result) {
	for _, block := range r {
		for k, v := range block {
			if i, ok := v.(int); ok {
				block[k] = float64(i)
			}
		}
	}
}
