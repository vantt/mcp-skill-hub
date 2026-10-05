---
phase: 12
title: "Calibration with recorded evidence"
status: done
priority: P2
effort: 4h
dependencies: [10]
---

# Phase 12: Calibration with recorded evidence

## Goal

Decide, from a recorded grid search, whether any routing policy parameter should change. Change nothing without a report that justifies it.

## Context (read these first)

- `plan.md` → "Executor notes". Rule: change parameters only with recorded evidence (`.claude/rules/review-audit-self-decision.md`).
- Defaults in `DefaultPolicy()` (`internal/resolver/policy.go:32-38`): `CandidateLimit 40`, `ApplicabilityFloor 0.16`, `HighConfidence 0.68`, `MinimumMargin 0.04`, `AmbiguityWindow 0.04`, `SupportingFloor 0.08`, weights `Lexical .28, Trigger .38, Artifact .08, Fact .10, Operation .15, Quality .03, NotFor .42, Constraint .48, Scope .25`. `policy.Revision` is a fingerprint of these values (`policy.go:39-44`), so any change alters `policy_revision` in responses, telemetry, and evaluation manifests.
- Policy file field names: `weights.trigger`, `weights.not_for_penalty` (`Weights` JSON tags, `policy.go:20-30`); thresholds `applicability_floor`, `minimum_margin`, … (parsed in `LoadPolicy`, `internal/resolver/sqlite_catalog.go:29`, which phase 10 split into `ParsePolicy`).
- Existing calibration grid and held-out gates: `testdata/resolver/evaluation-policy-v1.json` (`calibration_grid`, `selected`, `held_out_gates`), enforced by `internal/resolver/resolver_test.go` and `internal/evaluation/evaluation_test.go`.
- FTS trigger weight is 1.0 and the example discount is 0.9 (phase 9 constants).
- Tooling: `skillhub eval routing --policy <file>` and `app` routing eval service (phase 10).

## Requirements

1. Grid over policy-file parameters only: `applicability_floor` {0.12, 0.14, 0.16, 0.18, 0.20, 0.22}, `minimum_margin` {0.02, 0.04, 0.06}, `weights.trigger` {0.32, 0.38, 0.44}, `weights.not_for_penalty` {0.36, 0.42, 0.48}. Each point is evaluated on (a) the routing corpus (phase 10 gate workspace, explicit policy) and (b) the golden-v1 calibration split. The example discount (0.8/1.0) and the FTS trigger weight (5.0) are evaluated by one-off local edits whose results are recorded in the report and reverted unless selected. The FTS weight only affects candidacy (capped at `CandidateLimit` 40), so on the 42-skill corpus its effect is expected to be small. [UNVERIFIED: expectation; the report states the measured effect.]
2. Selection, in order: golden-v1 held-out gates must pass; maximize routing precision@1 subject to no-skill recall ≥ baseline and FPR ≤ baseline; tie-break by smallest change from defaults.
3. Apply a change only if the selected point beats defaults on the routing corpus by ≥ 0.02 in precision@1 or recall without regressing any gated metric; otherwise record "no change, evidence attached".
4. If applied: update `DefaultPolicy` (and the discount constant or bm25 weight if selected), `testdata/resolver/evaluation-policy-v1.json` `selected`, every test pinning the default policy revision (enumerate with `grep -rn "Revision" internal --include='*_test.go'` before editing), and re-derive `testdata/routing/gate-v1.json` with the phase 10 margin rule.
5. Report `reports/routing-calibration-report.md`: commit hash, commands, full grid table (point → precision@1, recall, no-skill P/R, FPR, golden-v1 gate results), selection, decision.

## Files

Create:
- `plans/261004-1547-skill-execution-measurement-routing/reports/routing-calibration-report.md`
- `internal/app/routing_calibration_test.go` (grid runner behind `SKILLHUB_CALIBRATE=1`, skipped otherwise; prints the report table)

Modify (only if a change is applied):
- `internal/resolver/policy.go`, `internal/resolver/evidence.go` (discount constant), `internal/resolver/sqlite_catalog.go` (bm25 weight)
- `testdata/resolver/evaluation-policy-v1.json`, `testdata/routing/gate-v1.json`
- Tests pinning `DefaultPolicy().Revision`

## Steps

- [x] **1. Grid runner.**
  Pass: `SKILLHUB_CALIBRATE=1 go test -count=1 -run RoutingCalibration -v ./internal/app/` prints the full table; `go test -count=1 -run RoutingCalibration ./internal/app/` (without the variable) reports the test as skipped.
- [x] **2. Report** with the full table and the decision.
  Pass: `reports/routing-calibration-report.md` exists with the commit hash and every grid row.
- [x] **3. Apply or decline** per the rule.
  Pass: `go test -count=1 ./internal/resolver/ ./internal/evaluation/ ./internal/app/` → `ok` (golden-v1 gates and routing gate pass).
- [x] **4. Gate.** Pass: `make check` exits 0.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Overfitting to a synthetic corpus | Medium × Medium | Held-out golden-v1 gates; minimum improvement margin; smallest-change tie-break. |
| Policy revision churn breaks pinned tests and stored manifests | Medium × Low | Enumerate pins first; manifests regenerate with `skillhub eval manifest`. |

## Rollback

Revert the constants commit; the report stays as a record.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol". Never apply a change whose evidence is not in the report. Write `reports/<agent>-<YYMMDD-HHMM>-calibration.md` if blocked, set `status: blocked`, report the blocker.

## Implementation Note

Implemented policy calibration with recorded evidence: created the grid runner in `internal/app/routing_calibration_test.go` behind `SKILLHUB_CALIBRATE=1`; evaluated 162 parameter combinations over policy-file thresholds and weights along with one-off discount constant and FTS weight variants; selected the optimal policy (`applicability_floor: 0.16`, `minimum_margin: 0.04`, `weights.trigger: 0.44`, `weights.not_for_penalty: 0.36`) which improved Precision@1 to 0.6220 (+0.0244) and Recall to 0.6071 (+0.0238) while improving FPR to 0.1062 and passing all golden-v1 held-out gates; updated `DefaultPolicy`, regenerated `cli-manifest-v1.json`, updated `gate-v1.json` thresholds, and recorded full evidence in `reports/routing-calibration-report.md`.
