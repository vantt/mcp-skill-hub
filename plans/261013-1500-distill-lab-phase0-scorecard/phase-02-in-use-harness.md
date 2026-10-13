---
phase: 2
title: "In-use harness, dry run"
label: agent-only
status: done
priority: P2
effort: "7h"
dependencies: [1]
---

# Phase 2: In-use harness (method C), proven against a stub

## Context

- Precedent: `plans/reports/measure-261012-curator-compliance.md`. It used repeated `claude -p` runs per scenario,
  a pinned model, an isolated HOME and XDG dirs and a throwaway hub, and it reported counts per scenario. That
  harness was never committed; this one is, so that the numbers can be reproduced.
- The question method C answers: does an agent that uses test-audit's lessons do better with or without each
  field? Two sub-questions: does the field help the agent decide or act (triage, apply), and can the agent
  rebuild the field from the rest (reconstruct)? A field the agent can rebuild, and whose absence does not hurt,
  is cheap to drop.
- `claude` 2.1.296 offers `-p`, `--model`, `--tools ""`, `--strict-mcp-config`, `--setting-sources`,
  `--no-session-persistence`, `--output-format json`, `--json-schema` and `--max-budget-usd` (checked with
  `claude --help` on 2026-10-13). Re-check them in the Phase 3 preflight.

## Design

**Inputs.** The test-audit content (`SKILL.md` and `CAMPAIGN.md` from the hub clone), the `goal` block, and one
lesson-file variant. Everything goes inline into the prompt and the agent gets no tools (`--tools ""`). The run
then cannot read or write any file, and its input is fixed and hashed.

**Variants** (`variants` subcommand). Each variant is built from the input file and written outside any hub:
- `full`: unchanged;
- `no-notable`: `notable` removed;
- `no-contrast`: `contrast` removed;
- `no-score`: `score` and `final_score` removed;
- `full-sorted`: unchanged and kept in priority order (used only in F1).
For every variant except `full-sorted`, lessons are shuffled with seed = repetition number, and the same seed is
used for all variants. The field content is then the only difference, apart from the order effect that
`full-sorted` measures. `manifest.json` records the sha256 of each variant and prompt.

**Scenario families** (`cmd/distill-scorecard/scenarios/test-audit.json`; each defines a prompt template and a
JSON output schema passed with `--json-schema`):
- **F1, triage** (all 5 variants). "Pick the 5 lessons to port first. Label every lesson `port`,
  `already-covered` or `outside-goal`." Metrics: recall@5 of the 3 planned lessons (a weak, biased gold, reported
  as such); agreement of `already-covered` labels with the 8 stored `already-covered` lessons; agreement of
  `outside-goal` labels with the 8 lessons of relevance 0; label stability across repetitions (mean pairwise
  agreement).
- **F2, reconstruct** (`no-contrast`, `no-score`). "Fill in the missing field for every lesson with the rubric
  from distill-lab." The rubric text is copied verbatim from `internal/systemskills/distill-lab/SKILL.md`
  (contrast values at line 337, weights at 228-289). Metrics: Cohen's kappa for `contrast`; quadratic-weighted
  kappa for `relevance`, `evidence` and `effort`; exact agreement of the `facts` set and of `impact`; Spearman of
  the recomputed against the stored `final_score`. The gold labels are themselves one agent's judgement, so the
  numbers measure reproducibility, not truth.
- **F3, apply** (`full`, `no-notable`, `no-contrast`, `no-score`). One run handles 6 fixed lessons and drafts, for
  each, the change to test-audit's SKILL.md in at most 150 words. The 6 lessons: the 3 planned
  (`ratchet-over-doctrine`, `mock-isolation-must-be-declared`, `ban-ships-with-replacement`), `description-trap`
  (contradicts), `no-source-text-assertions` (contradicts) and `prefer-real-boundary-over-broad-mocks`
  (already-covered, where the right answer is "no change needed"). Scoring: a blinded judge run per F3 run sees
  the goal, SKILL.md, each lesson's `what` and `where` (never `notable`, so that copying notable earns nothing),
  and the 6 answers under shuffled labels. It scores each answer 0-3 on a fixed rubric: right mechanism, inside the
  goal, not already in the skill, concrete placement. To measure judge noise, 4 judge runs are repeated and their
  agreement is reported.

**Repetitions and variance.** 5 repetitions per variant and family. For each metric the report shows the mean
and the min-max over repetitions. For paired differences (variant minus `full`, same seed) it shows the mean, the
min-max and how many of the 5 pairs point the same way. With 5 pairs a sign test cannot reach p < 0.05
(5/5 gives p = 0.0625 one-sided), so the threshold is an effect-size gate, not a significance test.

**Run budget (estimate).** F1 25 runs, F2 6 runs (2 variants × 3 repetitions; long outputs), F3 20 runs, judge
20 + 4 runs: 75 runs, each capped by `--max-budget-usd` (proposal: 0.50, so at most 37.5 USD; the user sets it in
Phase 3).

**Runner** (`run` subcommand). For each planned run it creates an empty working dir under the sandbox and
executes:

```bash
claude -p "<prompt>" --model <pinned id> --tools "" --strict-mcp-config --setting-sources "" \
  --no-session-persistence --output-format json --json-schema '<schema>' --max-budget-usd <cap>
```

The environment is limited to `HOME`, `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME` and `XDG_DATA_HOME`
inside the sandbox, `PATH`, and only the credential variable the user approves in Phase 3. `SKILLHUB_WORKSPACE` is
unset. The runner stores stdout, stderr, the exit code and the wall time per run, and is resumable: a run whose
output file exists is skipped. With `--claude <path>` any executable can stand in for `claude`; tests use a stub.
With `--dry-run` it prints the commands and writes the prompts without executing.

**Scorer and report** (`report` subcommand). Reads the stats JSON, the runs folder and the thresholds file
(`cmd/distill-scorecard/thresholds.json`, defaults from Phase 4), and writes `scorecard.md` and `scorecard.json`.

## Related code files

Create:
- `cmd/distill-scorecard/variants.go`, `run.go`, `score.go`, `report.go`, each with a `_test.go`.
- `cmd/distill-scorecard/scenarios/test-audit.json`, `cmd/distill-scorecard/thresholds.json`.
- `cmd/distill-scorecard/testdata/stub-claude.sh` (or a Go test helper compiled into a temp dir, so the tests also
  run on Windows) that returns canned JSON per family.
Modify: `cmd/distill-scorecard/main.go` (subcommands `variants`, `run`, `report`). Phase 1 owns it until Phase 1
is done; nothing runs in parallel.

## Implementation steps

1. `variants --in <distill.yaml> --hub-content <skill dir> --out <dir>`: remove fields, apply the seeded shuffle,
   copy `SKILL.md` and `CAMPAIGN.md` from the skill dir (read-only) next to the variants, and write the manifest
   with every hash. Refuse an output dir inside a hub (reuse `output_guard.go`).
2. Scenario file: the prompts quote the rubric verbatim, and each output schema has fixed enums.
3. `run`: build the command line exactly as above with the restricted environment, add resume and `--dry-run`,
   and fail fast when a run exits non-zero or its output does not match the schema. Count failures and stop when
   more than 20% of runs in a family fail.
4. `score`: kappa (unweighted and quadratic-weighted), Spearman, recall@k, pairwise agreement and paired
   differences, all unit-tested on hand-computed tables.
5. `report`: render the Phase 4 table from stats, the in-use summary and thresholds; any missing in-use input is
   rendered as "not run" and confidence is capped at "low".
6. End-to-end test: fixture → `variants` → `run --claude <stub>` → `report`, producing a table whose cells match
   the stub's canned answers.

## Validation

```bash
go test -count=1 ./cmd/distill-scorecard/...
make check
# dry run on a sandbox copy, no tokens:
go run ./cmd/distill-scorecard run --sandbox "$S" --dry-run --scenarios cmd/distill-scorecard/scenarios/test-audit.json
```

## Success criteria

- The end-to-end stub test passes and a dry run on the real fixture prints 75 well-formed commands with the
  restricted environment.
- No code path writes outside the sandbox or the plan's `reports/` folder (guard tested).

## Risks

| Risk | L×I | Mitigation |
|---|---|---|
| Field removal also removes the order signal and confounds the result | Med×High | Seeded shuffle in every variant; a separate `full-sorted` variant measures order |
| Judge rewards wording copied from `notable` | Med×High | The judge never sees `notable`; the rubric scores the mechanism and its placement |
| The planned-lesson gold is biased | High×Med | Reported as weak; the decision rule in Phase 4 does not rely on it alone |
| Prompt too long (SKILL.md + CAMPAIGN.md + 42 lessons) | Low×Med | Measure prompt size in the dry run; if over budget, drop CAMPAIGN.md and say so |

## Rollback

Delete the files this phase created and revert `main.go` to its Phase 1 state.
