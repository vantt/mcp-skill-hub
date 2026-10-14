---
phase: 2
title: "Share the self-resolution loop and add the overlay catalog"
status: pending
effort: 5h
depends_on: []
---

# Phase 2: Share the self-resolution loop and add the overlay catalog

## Context

`evaluatePositiveCases` and `evaluateCounterCases` (internal/app/routing_eval.go) each:

- build the same request (phrase, `min_scope`, `operations[0]`, `buildPreconditionFacts`,
  `buildPreconditionCapabilities`);
- wrap the catalog in `leaveOneOutCatalog`;
- resolve through `resolveWithin` with a fresh cache;
- classify the result.

The lint needs the same loop, with two differences:

- **It resolves through the engine directly.** `resolveWithin` builds the winner's distributed
  manifest, which errors for a draft winner, and the lint does not need servability.
- **It resolves against an overlay of the served generation**, where some skills are replaced, added
  or removed. Examples: a draft under review, working-tree versions, HEAD versions, index versions.

Measured on the golden corpus: the engine over a plain `SQLiteCatalog` costs 2.14 ms per case, because
`Skills()` decodes every row on each resolve. With skills decoded once it costs 0.90 ms.

## Requirements

- There is one per-case request builder and one per-skill case loop, used by `EvaluateRouting` and by
  the lint.
- `skillhub eval routing --json` on the golden corpus stays byte-identical. The eval keeps
  `resolveWithin` and its telemetry.
- `overlayCatalog{Catalog, replace map[id]Skill, remove set}` represents the served library with
  changed skills swapped in:
  - `Skills()`: the base skills, decoded once per run. Removed ids are dropped, replaced ids are
    swapped, and added skills get `Status = "active"`. The result is sorted by id.
  - `Search()`: the base hits with replaced and removed ids filtered out. Each overlay skill is added
    when any query token (resolver `tokenize`, split on non-alphanumerics as FTS5 `unicode61` does)
    appears in its indexed fields. The indexed fields mirror the `skill_fts` columns: name, aliases,
    description, triggers, examples, and topics plus technologies. The result is deduplicated and
    stable-ordered.
  - It composes under `leaveOneOutCatalog`.
- `engineResolver(catalog, policy)` creates a fresh `NewCache(1)` per case. It never writes telemetry
  and never builds manifests. It returns the `Response`, including the existing unexported-JSON
  `TopKSkillIDs`, which phase 3 reads to see which skills are in the top two.
- The work bound is benchmarked, not guessed.

## Files

- Create `internal/app/self_resolution.go`:
  - type `selfResolutionCase {SkillID, Kind, Index, Phrase}`;
  - `selfResolutionRequest(case, owner)` with the same `RequestID` format as the eval;
  - `ownCases(skill)`: examples first, then counter-examples, in index order;
  - `overlayCatalog`, `engineResolver`, and the bound constant `maxSelfResolutionWork`.
- Modify `internal/app/routing_eval.go`: the eval loops use `ownCases` and `selfResolutionRequest`
  and keep their accumulator logic. Move `leaveOneOutCatalog` and the precondition builders, or
  leave them in place.
- Create `internal/app/self_resolution_test.go`.
- Create a benchmark gated like `TestResolverPerformanceBudget` (`SKILLHUB_PERF=1`).

## Steps

1. Snapshot first: a test that runs `EvaluateRouting` on `materializeRoutingWorkspace` and compares
   the full JSON report to a checked-in expected file under `testdata/routing/`. Today's values are
   328/168/126/34 cases and 83 failures. It must pass before the refactor.
2. Extract the request builder and the case loop, and rewire the eval. The snapshot must still pass.
3. Test-first `overlayCatalog`:
   - an added draft;
   - a replaced active skill, where the base FTS hit is dropped and the overlay match is used;
   - a removed skill;
   - leave-one-out composed on top;
   - stable ordering.
4. Benchmark resolves at 42 skills (golden) and 256 synthetic skills. Fix `maxSelfResolutionWork` so
   one run stays at or under about 2 s on the dev machine, starting from 60,000 resolve x skill units.
   Record the measured numbers in the constant's comment.

## Validation

```bash
go test ./internal/app -run 'SelfResolution|RoutingEval|Overlay' -count=1
go test -race ./internal/app -run 'SelfResolution|Overlay' -count=1
SKILLHUB_PERF=1 go test ./internal/app -run 'SelfResolution.*Performance' -count=1 -v
make check
```

## Risks

- **Eval drift.** Mitigation: the snapshot test from step 1, written before the refactor.
- **Overlay search over-approximates FTS** for overlay skills: no BM25 ordering, no truncation. This
  is accepted and documented in plan.md.
- **The resolver rejects an incomplete draft** (`validateProjectedSkill`). The error is returned, and
  phase 4 maps it to `self_resolution_skipped`.

## Rollback

Revert the commit. The eval goes back to its own loops. No contract changes.
