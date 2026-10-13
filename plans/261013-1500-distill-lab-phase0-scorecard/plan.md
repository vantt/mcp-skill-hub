---
title: "distill-lab Phase 0 scorecard: keep or drop notable, contrast and R/E/F"
description: "Measure each experimental lesson field of distill-lab with read-only static analysis and an opt-in agent ablation test, so the user decides from one table instead of scoring 42 lessons by hand."
status: in-progress
priority: P2
effort: 25h
branch: main
tags: [distill, experimental, measurement, system-skills]
blockedBy: []
blocks: []
created: 2026-10-13
---

# distill-lab Phase 0 scorecard (backlog M6)

## Outcome, constraints, non-goals

- **Outcome.** One table (`reports/scorecard.md`) gives, for `notable`, `contrast` and each part of R/E/F, the
  evidence from every method, a keep / keep-optional / drop recommendation, a confidence level and what dropping
  it would change. The user reviews that table, sets thresholds and decides. No hand-scoring of 42 lessons.
- **R/E/F** is the `score` block plus `final_score` of distill-lab. The name comes from the older `distill` skill's
  Reach / Evidence / Effort rubric. Evidence: [scout notes §1](reports/scout-field-inventory.md). The user
  confirms this mapping in Phase 5.
- **Constraints.** Read the live hub, never write to it. `/home/vantt/skill-hub`, `~/.claude`, `~/.codex` and
  `~/.gemini` are never written. Experiments run on a clone under a temp dir with its own HOME and XDG_* dirs, and
  every `skillhub` call there passes `--workspace`. distill-lab's behaviour and the lesson schema stay unchanged
  until the user decides (Phase 5). Analysis tools are read-only on `distill.yaml` and use no network. Code
  comments, test names and commit messages carry no plan IDs, phase numbers or audit labels. Commits use the
  conventional format with no AI references, and only the lead commits.
- **Non-goals.** Scoring `layer`, `also_fits`, `status` or `found_by` (D7 does not list them as experimental;
  see question 4). Improving test-audit itself. Running a new distill pass. Changing telemetry.

## Acceptance criteria

- [ ] `go run ./cmd/distill-scorecard stats --in <file>` reproduces the numbers in the scout notes §2-3 on the
      fixture. `go test ./cmd/distill-scorecard/...` passes offline, and `make check` is green.
- [ ] The tool refuses to write inside any folder that has `.skillhub/schema-version` (tested). The live file's
      sha256 and `git -C /home/vantt/skill-hub status --porcelain` are identical before and after every phase.
- [ ] The in-use harness runs end to end against a stub `claude` (tested) before any paid run.
- [ ] `reports/scorecard.md` has one row per field with evidence per method, recommendation, confidence and
      drop impact. Static-only rows are capped at confidence "low".
- [ ] The user's decisions are recorded (Phase 5) before any change to distill-lab, `internal/distill`, schemas
      or fixtures.

## Phases

| Phase | Name | Label | Depends on | Status |
|---|---|---|---|---|
| 1 | [Static analysis tool](./phase-01-static-analysis-tool.md) | agent-only | none | done |
| 2 | [In-use harness, dry run](./phase-02-in-use-harness.md) | agent-only | 1 | done |
| 3 | [Paid in-use runs](./phase-03-paid-in-use-runs.md) | needs-user-go | 2 | pending |
| 4 | [Scorecard report](./phase-04-scorecard-report.md) | agent-only | 1 (3 optional) | done (static only) |
| 5 | [User decisions](./phase-05-user-decisions.md) | user-decision | 4 | pending |
| 6 | [Apply the decision](./phase-06-apply-the-decision.md) | needs-user-go | 5 | pending |

Phases run one after another, so no two phases edit the same file at the same time. Phase 4 can run right after
Phase 1 when the user declines Phase 3; the table then shows static evidence only.

## Who does what

| Work | Kind | How |
|---|---|---|
| Field inventory, static metrics, ordering sensitivity, decision association | fully automatable | `cmd/distill-scorecard stats`, Go tests on fixtures |
| Variant files, prompts, stub-run pipeline, scoring, report assembly | fully automatable | `variants`, `run --claude <stub>`, `report` subcommands, Go tests |
| `claude -p` runs and judge runs (about 70 runs, capped by `--max-budget-usd`) | automatable, needs the user's go | Phase 3 after preflight and budget approval |
| Credentials for `claude -p` under an isolated HOME | needs the user's go | Phase 3 preflight; never copied without approval |
| Confirm or drop the R/E/F mapping, thresholds, final keep/drop per field | user only | Phase 5 |
| Change distill-lab, schema, Go model, fixtures; migrate the live `distill.yaml` | agent implements after the go; the user runs `format` on the live hub and commits | Phase 6 |

## Must not / failure protocol

- Never write to a real hub's `distill.yaml` or any file under `/home/vantt/skill-hub`; the user runs the final
  `format` there (Phase 6, step 8).
- Never change distill-lab, `internal/distill`, schemas, web types or fixtures before Phase 5 is recorded. Until
  then the lesson schema stays as it is.
- Never spend tokens (`claude -p`) without the Phase 3 go; never copy credentials without approval.
- On any guard mismatch, auth failure, budget hit or more than 20% failed runs: stop, keep what exists, report
  (details in Phase 3). Never retry the same failing setup unchanged.

## Links

- Evidence and field inventory: [reports/scout-field-inventory.md](reports/scout-field-inventory.md)
- Origin: `docs/plans/2026-10-10-observation-backlog.md` row M6; `archive/plans/261008-1433-simplify-hub-model/plan.md` D7, Phase 0, S15
- Harness precedent: `plans/reports/measure-261012-curator-compliance.md`
