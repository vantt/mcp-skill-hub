# Routing Calibration Report

- **Date:** 2026-10-05
- **Commit:** `2e79658b4566c74d3209ca4ba80a1337c76a917e`
- **Command:** `SKILLHUB_CALIBRATE=1 go test -v -count=1 -run RoutingCalibration ./internal/app/`

## Objective & Rules
From Phase 12 requirements:
1. Grid over policy parameters:
   - `applicability_floor`: {0.12, 0.14, 0.16, 0.18, 0.20, 0.22}
   - `minimum_margin`: {0.02, 0.04, 0.06}
   - `weights.trigger`: {0.32, 0.38, 0.44}
   - `weights.not_for_penalty`: {0.36, 0.42, 0.48}
2. Selection criteria:
   - Golden-v1 held-out gates must pass (Accuracy >= 0.82, Precision >= 0.85, No-Skill Recall >= 0.85, Ambiguity Recall >= 0.85).
   - Maximize routing Precision@1 subject to No-Skill Recall >= baseline (1.0000) and False Positive Rate <= baseline (0.1125).
   - Tie-break by smallest change from defaults.
3. Decision threshold:
   - Apply a change only if the selected point beats defaults on the routing corpus by >= 0.02 in Precision@1 or Recall without regressing any gated metric.

## Baseline (Default Policy)
- `applicability_floor`: 0.16
- `minimum_margin`: 0.04
- `weights.trigger`: 0.38
- `weights.not_for_penalty`: 0.42
- **Routing Precision@1:** `0.5976`
- **Routing Recall:** `0.5833`
- **No-Skill Recall:** `1.0000`
- **No-Skill Precision:** `0.8947`
- **False Positive Rate:** `0.1125`
- **Golden-v1 Held-Out:** Accuracy 0.971, Precision 0.957, No-Skill Recall 1.000, Ambiguity Recall 0.857 (ALL PASS)

## One-Off Local Edits Evaluated
1. **Example discount constant (`ExampleOverlapWeight`)**:
   - `0.8`: Precision `0.6098`, Recall `0.5952`, No-Skill Recall `1.0000`, FPR `0.1187` (FPR regressed from `0.1125` to `0.1187`).
   - `1.0`: Precision `0.5976`, Recall `0.5833`, No-Skill Recall `1.0000`, FPR `0.1125` (identical to 0.9 baseline).
   - **Conclusion:** Reverted to 0.9; neither 0.8 nor 1.0 improved performance cleanly.
2. **FTS Trigger BM25 Weight (5.0 vs 1.0)**:
   - Measured effect: Precision `0.5976`, Recall `0.5833`, FPR `0.1125` (0.0000 delta). Because FTS ranking only provides candidacy up to `CandidateLimit` 40 and the catalog has 42 skills, trigger candidate sets are already inclusive.
   - **Conclusion:** Reverted to 1.0 default.

## Winning Policy Selection
- `applicability_floor`: **0.16** (unchanged)
- `minimum_margin`: **0.04** (unchanged)
- `weights.trigger`: **0.44** (increased from 0.38)
- `weights.not_for_penalty`: **0.36** (decreased from 0.42)

### Performance Comparison
| Metric | Baseline | Calibrated | Delta |
|---|---|---|---|
| **Routing Precision@1** | 0.5976 | **0.6220** | **+0.0244** (>= +0.02 threshold) |
| **Routing Recall** | 0.5833 | **0.6071** | **+0.0238** (>= +0.02 threshold) |
| **No-Skill Recall** | 1.0000 | **1.0000** | 0.0000 (no regression) |
| **No-Skill Precision** | 0.8947 | **0.8947** | 0.0000 (no regression) |
| **False Positive Rate (FPR)** | 0.1125 | **0.1062** | **-0.0063** (improved) |
| **Failures Count** | 88 | **83** | **-5 failures** |
| **Golden Held-Out Gates** | PASS | **PASS** | Accuracy 0.971, Precision 0.957, No-Skill 1.000, Ambiguity 0.857 |

## Decision
**APPLY CHANGE.**
The selected point beats baseline defaults by +0.0244 in Precision@1 and +0.0238 in Recall (both exceeding the required 0.02 margin), improves FPR by -0.0063 with zero regression in No-Skill Recall (1.0000), and passes all golden-v1 held-out gates.

### Derived Gate Thresholds (`gate-v1.json`)
Using the margin rule (flooring gated metrics to two decimals minus 0.02, ceiling FPR plus 0.02):
- `min_precision`: `0.60` (floor(0.6220) - 0.02)
- `min_recall`: `0.58` (floor(0.6071) - 0.02)
- `min_no_skill_recall`: `0.98` (floor(1.0000) - 0.02)
- `max_fpr`: `0.13` (ceil(0.1062) + 0.02)
