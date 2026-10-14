---
phase: 2
title: "Share the self-resolution loop with routing eval"
status: pending
effort: 4h
depends_on: []
---

# Phase 2: Share the self-resolution loop with routing eval

## Context

`evaluatePositiveCases` and `evaluateCounterCases` (internal/app/routing_eval.go) each:

- build the same request (phrase, `min_scope`, `operations[0]`, `buildPreconditionFacts`,
  `buildPreconditionCapabilities`);
- wrap the catalog in `leaveOneOutCatalog`;
- resolve through `resolveWithin` with a fresh cache;
- classify the result.

Lint needs the same loop, with two differences:

- **It resolves through the engine directly.** `resolveWithin` builds the winner's distributed
  manifest, which errors for a draft winner, and lint does not need servability.
- **It can inject a draft target into the catalog.**

Measured (golden corpus, 42 skills): the engine over a plain `SQLiteCatalog` costs 2.14 ms per case,
because `Skills()` decodes every row on each resolve. With memoized skills it costs 1.17 ms. The lint
resolver must decode skills once.

## Requirements

- There is one per-skill case loop, used by both `EvaluateRouting` and lint.
- `skillhub eval routing --json` on the golden corpus stays byte-identical: same metrics, same failures
  in the same order, same telemetry event. The eval keeps `resolveWithin`.
- There is a target-injection catalog decorator. It represents "the active library as if this skill
  were active now":
  - `Skills()`: the base skills with the target added, or replacing the served version that has the
    same id. The target gets `Status = "active"`, and the result is sorted by id.
  - `Search()`: the base hits plus the target id when any query token (resolver `tokenize`) appears
    in the target's indexed fields. The indexed fields mirror the `skill_fts` columns: name, aliases,
    description, triggers, examples, and topics plus technologies. Add the target id only once.
    Compose it under the leave-one-out decorator, so the left-out example is removed from scoring the
    same way the eval removes it.
- The lint resolver decodes skills once per run and creates a fresh `NewCache(1)` per case. It never
  writes telemetry and never builds manifests.
- The work bound is benchmarked, not guessed.

## Files

- Create `internal/app/self_resolution.go`:
  - type `selfResolutionCase {SkillID, Kind ("example" | "counter_example"), Index, Phrase, Status, Primary}`;
  - `selfResolutionRequest(skill, kind, index, phrase)`, moved from the two eval functions with the
    same `RequestID` format;
  - `resolveSelfCases(ctx, skill, resolve) ([]selfResolutionCase, error)`: examples first, then
    counter-examples, in index order;
  - `withTargetCatalog`;
  - `engineResolver(catalog, policy)`: a closure over a skills-once catalog;
  - `selfResolutionWork(cases, skills int) int` and the bound constant.
- Modify `internal/app/routing_eval.go`: `evaluatePositiveCases` and `evaluateCounterCases` consume
  `resolveSelfCases` and keep their accumulator logic. Move `leaveOneOutCatalog`,
  `buildPreconditionFacts` and `buildPreconditionCapabilities` into the new file, or leave them in place.
- Create `internal/app/self_resolution_test.go`: unit tests for the decorator, the ordering, a draft
  winner, a draft that replaces an active version, and the counter case.
- Create `internal/app/self_resolution_bench_test.go` (or add a case to an existing perf test, gated
  like `TestResolverPerformanceBudget` with `SKILLHUB_PERF=1`): measure cost at 42 and 256 skills.

## Steps

1. Snapshot: add a test that runs `EvaluateRouting` on `materializeRoutingWorkspace` and compares the
   full JSON report to a checked-in expected value under `testdata/routing/`. Alternatively assert the
   current counts (328/168/126/34, 83 failures) plus a digest of the failures. It must pass before the
   refactor.
2. Extract `selfResolutionRequest` and `resolveSelfCases`. Rewire the eval. The snapshot test must
   still pass.
3. Add `withTargetCatalog` and `engineResolver`, test first:
   - draft example whose phrase matches an active skill's trigger: primary is that skill;
   - draft example matching only its own triggers: primary is the draft;
   - edited active skill: the canonical version replaces the served one.
4. Benchmark `cases x skills` at 42 and 256 synthetic skills. Set the bound so validate stays at or
   under about 2 s on the dev machine. 60,000 is the starting value. Record the measured numbers
   in the constant's comment.

## Validation

```bash
go test ./internal/app -run 'SelfResolution|RoutingEval' -count=1
SKILLHUB_PERF=1 go test ./internal/app -run 'SelfResolution.*Performance' -count=1 -v
go test -race ./internal/app -run 'SelfResolution|RoutingEval' -count=1
make check
```

## Risks

- **Eval drift.** A subtle change in request building or cache use changes the metrics. Mitigation:
  the step 1 snapshot test, written before the refactor.
- **Draft retrieval over-approximates FTS.** Any token match in an indexed field makes the draft a
  candidate, even where FTS would rank it past `CandidateLimit`. This is accepted and documented in
  plan.md.
- `validateProjectedSkill` rejects an incomplete draft, for example one with no `min_scope`, so
  `Resolve` returns an error. `resolveSelfCases` returns that error, and phase 3 turns it into
  `self_resolution_skipped`.

## Rollback

Revert the commit. The eval goes back to its own loops. No data or contract changes.
