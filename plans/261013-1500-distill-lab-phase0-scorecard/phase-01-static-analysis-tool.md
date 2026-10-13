---
phase: 1
title: "Static analysis tool"
label: agent-only
status: done
priority: P2
effort: "5h"
dependencies: []
---

# Phase 1: Static analysis tool (methods A and B)

## Context

- Field inventory, real-data numbers and code sites: [reports/scout-field-inventory.md](reports/scout-field-inventory.md).
- The live file and both fixtures are byte-identical (`sha256 048ab876…7675`), so tests can use
  `internal/migration/testdata/test-audit-distill.yaml` and expect the same numbers as the live file.

## Decision: language and location

Go, as `cmd/distill-scorecard` (package `main`) in this module. Reasons:
- It reuses `internal/distill.LoadDocument` (`internal/distill/distill_model.go`), so the tool reads exactly what
  the product validates. Rewriting a loader would duplicate it.
- `make test` runs its tests without Node. CI builds only `./cmd/skillhub` (`.github/workflows/ci.yml:202`,
  `release.yml:339`), so the tool never ships.
- distill-lab is not touched: its bundle is a shipped, digest-tracked artifact.

## Requirements

- `stats --in <distill.yaml> [--json] [--out <file>]`. Reads one file and prints metrics. Opens the input
  read-only and uses no network. `--out` must not resolve inside a folder that contains
  `.skillhub/schema-version`, and must not equal `--in`.
- **Method A, association with `decision.state`.** Inputs: `decision.state` and every scored field. Computation:
  - the counts per state, reported as they are (the live file has candidate 39, planned 3, no rejected);
  - for each numeric field and for `final_score`, the mid-ranks of the decided lessons and the exact
    probability that k random lessons all rank at least that high, computed exactly from binomial
    coefficients (no sampling);
  - a cross-table of `contrast` × state.
  Output: one block per field, labelled "preliminary". Known weaknesses, printed in the output: 3 positives out of
  42; `candidate` means "undecided", not "rejected", so there are no negatives; the 3 decisions were made with the
  scores visible (`12c7925`), so a high rank is partly circular. This method can never support a decision alone.
- **Method B, information content.** Inputs: the text and enum fields. Computation:
  - `notable` against `what`: word-set Jaccard, the share of `notable` content words missing from `what`
    (fixed stop-word list in the code), words per lesson, gzip marginal bytes `gz(what+notable) − gz(what)`, and
    trigrams shared by 3 or more lessons (template wording);
  - `contrast`: Shannon entropy, cross-tables against `impact == 0` and against fact `b`, and the conditional
    entropy H(contrast | facts), which shows how much of `contrast` the score already implies;
  - R/E/F: the distribution of each component, ties at `final_score == 0`, Spearman between components, and
    **ordering sensitivity**: Kendall tau-b between the stored priority order and the order recomputed with one
    component held constant (relevance, impact, evidence, effort), and with `impact` alone;
  - `score.why`: words and trigram reuse.
  Output: JSON `{"field": {"metric": value}}` plus a text summary. Weaknesses: lexical measures miss paraphrase;
  gzip is a crude entropy proxy on 42 short texts; ordering sensitivity says how much a component moves the
  ranking, not whether that ranking is right.
- Ties use mid-ranks everywhere. The output is deterministic: sorted keys and fixed float formatting.

## Related code files

Create:
- `cmd/distill-scorecard/main.go`: subcommand dispatch and flags (`stats` only in this phase).
- `cmd/distill-scorecard/stats.go`: methods A and B.
- `cmd/distill-scorecard/output_guard.go`: refusal to write inside a hub or onto the input.
- `cmd/distill-scorecard/stats_test.go`, `output_guard_test.go`.
- `cmd/distill-scorecard/testdata/`: small synthetic files: all `already-covered`, a field with zero entropy, a
  tie-heavy score set, a file with a rejected lesson.
Modify: none. Delete: none.

## Implementation steps

1. Write `stats` on top of `distill.LoadDocument`. If the live file fails Go validation, stop and report; do not
   work around the validation.
2. Implement methods A and B as pure functions over `[]distill.Lesson` and keep them under the lint limit of 120
   lines per function.
3. Tests: one table test over the 42-lesson fixture asserts the scout-note numbers (counts, entropy 1.762,
   Jaccard median 0.09, 24 zero scores, Spearman impact~final 0.98 ± 0.005); synthetic fixtures cover zero
   entropy, all-tied scores and a rejected lesson; the guard test creates a temp hub marker and expects a refusal.
   Test names describe behaviour (for example `TestStatsReportsNoNegativesWhenNothingIsRejected`), with no plan or
   phase labels.
4. Run on the live file, reading it only: record its sha256 and
   `git -C /home/vantt/skill-hub status --porcelain` before, run
   `go run ./cmd/distill-scorecard stats --in /home/vantt/skill-hub/skills/default/test-audit/.meta/distill.yaml --json --out plans/261013-1500-distill-lab-phase0-scorecard/reports/scorecard-static.json`,
   then compare both checks after the run.

## Validation

```bash
go test -count=1 ./cmd/distill-scorecard/...
go vet ./cmd/distill-scorecard/...
make check
sha256sum /home/vantt/skill-hub/skills/default/test-audit/.meta/distill.yaml   # equal before/after
git -C /home/vantt/skill-hub status --porcelain                               # equal before/after
```

## Success criteria

- `reports/scorecard-static.json` exists and matches the fixture numbers asserted by the tests.
- The live hub is unchanged (sha256 and git status identical).

## Risks

| Risk | L×I | Mitigation |
|---|---|---|
| Go loader rejects the live file (header names `distill.py`) | Low×Med | Tests already load the identical fixture; if it fails, stop and report |
| Metrics over-read as proof | Med×High | Every block printed with its weakness; Phase 4 caps static-only confidence at "low" |
| Lint debt in a new package | Low×Low | Small functions; `make check` before handing back |

## Rollback

Delete `cmd/distill-scorecard/` and `reports/scorecard-static.json`. Nothing else changed.
