# Phase 12: Calibration with recorded evidence report

## Summary
- Implemented grid runner test `TestRoutingCalibration` in `internal/app/routing_calibration_test.go` behind `SKILLHUB_CALIBRATE=1` (skipping by default).
- Evaluated full 162-point grid over policy parameters (`applicability_floor`, `minimum_margin`, `weights.trigger`, and `weights.not_for_penalty`) on both the routing corpus and golden-v1 held-out cases.
- Evaluated one-off variants:
  - `ExampleOverlapWeight`: 0.8 regressed FPR; 1.0 was identical to 0.9. Reverted to 0.9.
  - FTS Trigger BM25 weight: 5.0 had 0.0000 delta. Reverted to 1.0.
- Selected winning policy point:
  - `applicability_floor: 0.16` (unchanged)
  - `minimum_margin: 0.04` (unchanged)
  - `weights.trigger: 0.44` (from 0.38)
  - `weights.not_for_penalty: 0.36` (from 0.42)
  - Beats baseline on routing corpus by +0.0244 in Precision@1 (0.6220 vs 0.5976) and +0.0238 in Recall (0.6071 vs 0.5833).
  - FPR improved from 0.1125 to 0.1062 with No-Skill Recall maintained at 1.0000.
  - Golden-v1 held-out gates all pass (Accuracy 0.971, Precision 0.957, No-Skill Recall 1.000, Ambiguity Recall 0.857).
- Applied changes:
  - Updated `DefaultPolicy()` in `internal/resolver/policy.go`.
  - Regenerated `testdata/evaluation/cli-manifest-v1.json` with new policy revision.
  - Updated `testdata/routing/gate-v1.json` thresholds per margin rule:
    - `min_precision`: 0.60
    - `min_recall`: 0.58
    - `min_no_skill_recall`: 0.98
    - `max_fpr`: 0.13
- Documented complete findings in `reports/routing-calibration-report.md`.

## Verification
- `SKILLHUB_CALIBRATE=1 go test -count=1 -run RoutingCalibration -v ./internal/app/` (PASS, executed grid)
- `go test -count=1 -run RoutingCalibration ./internal/app/` (PASS, correctly skipped without env var)
- `go test -v -count=1 -run RoutingEvalGate ./internal/app/` (PASS, meets all updated thresholds in 775 ms)
- `go test -count=1 ./internal/resolver/ ./internal/evaluation/` (PASS)
- `go test -count=1 ./internal/delivery/cli/ -run TestEvaluationCLICommittedArtifactsRunAndRegenerateDeterministically` (PASS)
