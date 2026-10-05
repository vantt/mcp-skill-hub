# Routing Evaluation Baseline Report

## Metadata
- **Commit:** `a4e184ba126c127a3de0f8483e2c872acd9975e2`
- **Command:** `go test -v -count=1 -run RoutingEvalGate ./internal/app/`
- **Duration:** 2.18s (meets target under 10s)
- **Date:** 2026-10-05

## Dataset Summary
- **Skills Evaluated:** 42 golden-v1 skills
- **Total Positive Cases:** 129 (3–5 natural task phrasings per skill)
- **Total Counter Cases:** 84 (1–3 near-misses per skill)
- **Total No-Skill Cases:** 32 (unrelated domains, chit-chat, trivial edits)
- **Evaluation Strategy:** Leave-one-out cross-validation per skill example/counter-example

## Measured Metrics & Gate Thresholds
Thresholds are derived by flooring rates to two decimal places minus 0.02 (and ceiling plus 0.02 for FPR):

| Metric | Measured Baseline | Gate Threshold | Margin | Status |
|---|---|---|---|---|
| Precision@1 | 0.5714 | 0.5500 | +0.0214 | PASS |
| Recall | 0.5891 | 0.5600 | +0.0291 | PASS |
| No-Skill Recall | 0.8438 | 0.8200 | +0.0238 | PASS |
| No-Skill Precision | 0.9643 | — | — | PASS |
| False Positive Rate | 0.1379 | 0.1600 | -0.0221 | PASS |

## Observations & Failing Cases
- **Positives resolving to sibling or generic skills:**
  - When positive examples use common action verbs such as `review`, `implement`, `design`, or `audit` without specific technical domain vocabulary, they sometimes collide with broad skills like `code-review` or `authentication` because of trigger overlap on those generic verbs.
  - Sibling pairs (`code-review` vs `pull-request-review`, `unit-testing` vs `test-design`) compete closely when phrasing touches both domains.
- **Counter-examples:**
  - 11 counter cases resolved to their own skill due to lexical token similarity (e.g. `api-design` counter-example mentioning database schema).
- **No-skill cases:**
  - 27 of 32 correctly abstained with `no_skill`. 5 non-software tasks (e.g. plumbing, soldering) matched vocabulary like `design` or `operate` on `event-architecture-review` or `kubernetes-operations`.
- **Calibration Opportunity (Phase 12):**
  - These baseline metrics establish the uncalibrated floor. Phase 12 will execute grid search across policy parameters (applicability floor, minimum margin, trigger weight, not-for penalty) to further optimize Precision@1 and Recall.
