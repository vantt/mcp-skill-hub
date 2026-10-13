---
phase: 4
title: "Scorecard report"
label: agent-only
status: pending
priority: P2
effort: "3h"
dependencies: [1]
---

# Phase 4: Scorecard report (the one table the user reviews)

Inputs: `reports/scorecard-static.json` (Phase 1) and, when Phase 3 ran, `reports/inuse/summary.json`.
Output: `reports/scorecard.md` and `reports/scorecard.json`, rendered by `go run ./cmd/distill-scorecard report`
(never edited by hand, so that a rerun gives the same table).

## Table format

One row per field: `notable`, `contrast`, `score.relevance`, `score.facts`/`impact`, `score.evidence`,
`score.effort`, `score.why`, `final_score` (ordering), and a summary row "R/E/F as a block".

| Field | A: decision association (preliminary) | B: information / ordering | C1 triage Δ vs full | C2 reconstruct agreement | C3 apply Δ vs full | Consumers (file:line) | Recommendation | Confidence | If dropped |
|---|---|---|---|---|---|---|---|---|---|

Cells show a number with its range, for example "−0.6 (−0.9..−0.2), 5/5 same sign". "If dropped" lists the code,
contract and data changes from the [scout notes §5](reports/scout-field-inventory.md) in one line, for example
"strip on `format`; `list` loses a column; Home high-value count needs a new rule (public JSON)". Under the table:
the thresholds used, a "limits" paragraph (n = 42, one skill, one model, `-p` mode, biased gold) and the run
manifest hashes.

## Default decision rule (thresholds are the user's call)

Per field, in order:
1. **Keep (required)** when removing the field lowers the C1 or C3 score by at least **0.3 on the 0-3 judge
   scale** or **10 percentage points** on a rate, with the same sign in **at least 4 of 5** paired repetitions,
   and the drop is larger than the `full` variant's own min-max spread.
2. **Drop** when all three hold: the agent can rebuild the field (C2 kappa **≥ 0.6**; for an ordering-only field,
   Kendall tau-b **≥ 0.9** between the order with and without it); removing it has no effect under rule 1; and every
   code consumer has a stated replacement.
3. **Keep optional** otherwise (the status D7 promised): written when the agent has it, not required by
   validation.

Reasons for these defaults: 0.6 is the lower edge of "substantial" agreement on the usual kappa scale; 0.3 is
10% of the judge scale and above the judge noise expected from the repeated judge runs (if measured judge
disagreement is larger, the gate rises to that value); 4 of 5 is the strongest sign evidence 5 pairs can give; a
tau of 0.9 means fewer than 5% of lesson pairs swap order.

**Confidence.** High: static and in-use evidence agree and the in-use effect clears the gate. Medium: only the
in-use evidence is clear. Low: static evidence only, or the methods disagree. Method A never raises confidence.

## What the static evidence already suggests (preliminary, from the scout notes)

- `notable` carries content that is not in `what` (Jaccard median 0.09; 91% of its content words are new; it adds
  3291 B of 6857 B after gzip). Likely keep, unless C3 shows no effect.
- `contrast` is the only signal that separates the 8 already-covered lessons from the 14 new-but-outside-goal
  ones (all 22 have impact 0), and fact `b` is defined through it. Likely keep, possibly optional.
- R/E/F: 24 of 42 lessons score 0, and the order among them falls back to layer and key. `final_score` follows
  `impact` almost exactly (Spearman 0.98); `effort` (−0.12) and `evidence` (0.28) barely move it. Candidates for
  "drop a component" are `effort` and `evidence` if ordering sensitivity confirms it. `relevance` decides the
  "outside the goal" call, so C1 must show whether the agent can do without it.

These are hypotheses for the table, not conclusions.

## Steps

1. Implement the Phase 2 `report` rendering for both cases (with and without in-use data).
2. Run it, then re-read every number in the table against the JSON inputs (a test asserts the rendering of a
   fixture table).
3. Hand the table to the lead with a list of the decisions for Phase 5.

## Validation

```bash
go test -count=1 ./cmd/distill-scorecard/...
go run ./cmd/distill-scorecard report --stats .../scorecard-static.json [--runs ...] --thresholds cmd/distill-scorecard/thresholds.json --out .../reports/
```

## Risks and rollback

Risk: the table reads as more certain than it is. The confidence cap and the "limits" paragraph are part of the
test-asserted output. Rollback: delete `reports/scorecard.*`.
