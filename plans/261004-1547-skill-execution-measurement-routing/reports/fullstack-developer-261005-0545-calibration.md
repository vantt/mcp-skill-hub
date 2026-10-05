# Phase 12: Calibration with recorded evidence report

## Summary
- Implemented calibration grid runner in `internal/app/routing_calibration_test.go`:
  - Guarded behind `SKILLHUB_CALIBRATE=1` environment variable; skipped during standard CI test runs.
  - Explores 162 parameter combinations over:
    - `applicability_floor`: {0.12, 0.14, 0.16, 0.18, 0.20, 0.22}
    - `minimum_margin`: {0.02, 0.04, 0.06}
    - `weights.trigger`: {0.32, 0.38, 0.44}
    - `weights.not_for_penalty`: {0.36, 0.42, 0.48}
  - Evaluates both the leave-one-out routing corpus (via `RoutingEvalService.Run`) and the golden-v1 held-out split with in-memory SQLite catalog and full relationship assertions.
- Evaluated one-off local edits:
  - Example discount factor: 0.8 regressed recall and worsened FPR; 1.0 regressed precision; 0.9 confirmed optimal and retained.
  - FTS trigger BM25 weight: 5.0 vs 1.0 yielded identical precision (0.6281) and recall (0.5891) on the 42-skill corpus due to candidacy capping; 1.0 retained.
- Recorded full grid table and calibration evidence in `reports/routing-calibration-report.md`.
- Decision: **No change, evidence attached.** Defaults in `DefaultPolicy()` (`ApplicabilityFloor = 0.16, MinimumMargin = 0.04, Weights.Trigger = 0.38, Weights.NotFor = 0.42`) satisfy all golden-v1 calibration and held-out gates, avoiding policy revision fingerprint churn.

## Verification
- Unit & integration tests:
  - `go test -count=1 -run RoutingCalibration ./internal/app/` (PASS: skipped without env var)
  - `SKILLHUB_CALIBRATE=1 go test -count=1 -run RoutingCalibration ./internal/app/` (PASS: evaluates all 162 points)
  - `go test -count=1 ./internal/resolver/ ./internal/evaluation/ ./internal/app/` (PASS: golden-v1 gates and routing gate pass)
  - `make check` (go vet, golangci-lint 0 issues, full test suite across all 24 packages) (PASS)
