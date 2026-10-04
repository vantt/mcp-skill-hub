---
phase: 12
title: "Calibration with recorded evidence"
status: pending
priority: P2
effort: 4h
dependencies: [10]
---

# Phase 12: Calibration with recorded evidence

## Context

- Current defaults: `ApplicabilityFloor 0.16`, `HighConfidence 0.68`, `MinimumMargin 0.04`, `AmbiguityWindow 0.04`, weights `Lexical .28, Trigger .38, Artifact .08, Fact .10, Operation .15, Quality .03, NotFor .42, Constraint .48, Scope .25` (`internal/resolver/policy.go:32-36`). `DefaultPolicy().Revision` is a fingerprint of these values (`policy.go:37-43`), so any change alters `policy_revision` in responses, telemetry, and evaluation manifests.
- Existing calibration grid and held-out gates: `testdata/resolver/evaluation-policy-v1.json`, enforced by `internal/resolver/resolver_test.go` and `internal/evaluation/evaluation_test.go`.
- FTS trigger weight is effectively 1.0 (plan.md "Verified starting facts"); Phase 9 kept it.
- Tooling: `skillhub eval routing --policy <file>` from Phase 10.
- Rule: change parameters only with recorded evidence (`.claude/rules/review-audit-self-decision.md`, user instruction).

## Requirements

1. Grid search over the policy-file parameters only, so no test-only exports are needed: `applicability_floor` {0.12, 0.14, 0.16, 0.18, 0.20, 0.22}, `minimum_margin` {0.02, 0.04, 0.06}, `weights.trigger` {0.32, 0.38, 0.44}, `weights.not_for_penalty` {0.36, 0.42, 0.48}. Each point is evaluated on (a) the routing corpus (Phase 10 gate workspace, via `RoutingEvalService` with an explicit `Policy`) and (b) the golden-v1 calibration split. The example discount (0.9) and the FTS trigger weight (1.0) are code constants; they are evaluated by one-off local edits (discount 0.8/1.0; trigger weight 5.0) whose results are recorded in the report, and changed only under the same rule. The FTS weight only affects candidacy, which is capped at `CandidateLimit` 40, so on the 42-skill corpus its effect is expected to be small; the report states the measured effect either way.
2. Selection rule, in order: golden-v1 held-out gates must still pass; maximize routing precision@1 subject to no-skill recall ≥ baseline and FPR ≤ baseline; tie-break by smallest change from current defaults.
3. Apply a change only if the selected point beats the current defaults on the routing corpus by at least 0.02 in precision@1 or recall without regressing any gated metric. Otherwise keep the defaults and record "no change, evidence attached".
4. If applied: update `DefaultPolicy` (and the example discount constant or FTS weight if selected), update `testdata/resolver/evaluation-policy-v1.json` `selected` values, update every test that pins the default policy revision, and re-derive `testdata/routing/gate-v1.json` thresholds from the new measurement with the Phase 10 margin rule.
5. Evidence report `reports/routing-calibration-report.md`: commit hash, commands, full grid table (point → precision@1, recall, no-skill P/R, FPR, golden-v1 gate results), the selection, and the decision.

## Files

Create:
- `plans/261004-1547-skill-execution-measurement-routing/reports/routing-calibration-report.md`
- `internal/app/routing_calibration_test.go` (grid runner behind `SKILLHUB_CALIBRATE=1`, skipped by default; prints the table used in the report)

Modify (only if a change is applied):
- `internal/resolver/policy.go`
- `internal/resolver/evidence.go` (example discount constant)
- `internal/resolver/sqlite_catalog.go` (trigger bm25 weight)
- `testdata/resolver/evaluation-policy-v1.json`
- `testdata/routing/gate-v1.json`
- tests that pin `DefaultPolicy().Revision` (enumerate with `grep -rn "Revision" internal --include='*_test.go'` before editing)

## Steps

1. Implement the opt-in grid runner; run `SKILLHUB_CALIBRATE=1 go test -run RoutingCalibration -v ./internal/app/`.
2. Write the report with the full table.
3. Apply or decline per the rule; if applied, update constants, fixtures, pinned revisions, and gate thresholds.

## Tests and validation

- `go test ./internal/resolver/ ./internal/evaluation/ ./internal/app/` (golden-v1 gates and routing gate pass)
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Overfitting to a synthetic corpus | Medium × Medium | Held-out golden-v1 gates must pass; minimum improvement margin; smallest-change tie-break. |
| Policy revision churn breaks pinned tests and stored manifests | Medium × Low | Enumerate pins before editing; manifests are regenerated with `skillhub eval manifest`. |

## Rollback

Revert the constants commit; the report stays as a record.
