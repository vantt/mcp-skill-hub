---
phase: 9
title: "Resolver routing features"
status: pending
priority: P1
effort: 8h
dependencies: [1]
---

# Phase 9: Resolver routing features

## Context

- [plan.md](./plan.md) D7; design `docs/design/03-resolver-design.md` §4 (index), §7 (feature model).
- FTS table `skill_fts(skill_id UNINDEXED, name, aliases, description, triggers)` (`internal/catalog/schema.go:131`), filled in `internal/catalog/project.go:78`; `projectRouting` (`project.go:254`); smoke query `internal/catalog/build.go:369`; `DerivedSchemaVersion = 2` (`internal/catalog/catalog.go:38`).
- Search ranking `bm25(skill_fts,0.0,8.0,5.0,7.0)` (`internal/resolver/sqlite_catalog.go:99`): triggers currently get the default weight 1.0.
- Projection into `resolver.Skill`: `skillDocument` / `decodeRouting` (`sqlite_catalog.go:139-237`), `normalizeSkillRouting`.
- Scoring: `scoreSkill` (`internal/resolver/evidence.go:256`); rule channel trigger rank (`internal/resolver/resolver.go:170`); `positiveQuery` (`evidence.go:360`).
- Evaluation pins: `EvaluationIndexVersion = "sqlite-fts5-v2"` (`internal/app/evaluation.go:25`), also pinned in `testdata/evaluation/cli-manifest-v1.json`.
- Request evidence available for technology matching: `context.active_artifact.language`, `context.facts[]{key,value,basis}` (`internal/resolver/types.go:52-66`).

## Requirements

1. **Index.** `skill_fts` gains `examples` (all `routing.examples` joined) and `keywords` (`topics` + `technologies` joined). `DerivedSchemaVersion = 3`; `EvaluationIndexVersion = "sqlite-fts5-v3"`. Counter-examples are **not** indexed (they must not create candidacy). `routing_metadata` gets categories `example` and `counter_example` for diagnostics.
2. **Search weights**, written explicitly for every column: `bm25(skill_fts, 0.0, 8.0, 5.0, 7.0, 1.0, 3.0, 2.0)` (skill_id, name, aliases, description, triggers, examples, keywords). Triggers stay at their current effective weight 1.0 so this phase does not change existing ranking; Phase 12 decides any trigger weight change from evidence.
3. **Projection.** `resolver.Skill` gains `Examples`, `CounterExamples`, `Topics`, `Technologies` (`json:",omitempty"`); `skillDocument` reads `routing.examples`, `routing.counter_examples`, top-level `topics`, `technologies`; `normalizeSkillRouting` sorts them. `domain` stays unused.
4. **Scoring** (no new policy weights):
   - Trigger feature: `max(bestOverlap(query, Triggers), 0.9 × bestOverlap(query, Examples))`. Reason code `example_match` when the example term wins and is ≥ 0.3.
   - Rule channel trigger rank uses the same combined value.
   - Not-for feature: `max(bestOverlap(query, NotFor), bestOverlap(query, CounterExamples))`, so counter-examples share the existing soft penalty (≥ .35 → unknown exclusion) and hard exclusion (≥ .72).
   - Lexical metadata tokens: name + description + aliases + **topics + technologies**.
   - Technology match: let `techEvidence` = `active_artifact.language` plus values of facts whose key is one of `language`, `framework`, `library`, `dependency`, `technology`, `runtime`, `platform` (any basis; this is a soft positive signal). If any `Technologies` entry `equalEvidence` any techEvidence value, the artifact feature becomes `max(current, 1.0)` and reason code `technology_match` is added. No penalty for a mismatch.
5. Resolver determinism is unchanged: same catalog snapshot and request → same response.

## Files

Modify:
- `internal/catalog/schema.go`, `internal/catalog/project.go`, `internal/catalog/catalog.go`, `internal/catalog/build.go` (smoke query unaffected; adjust only if column count is asserted)
- `internal/catalog/catalog_test.go` (projection of new columns)
- `internal/resolver/types.go`, `internal/resolver/sqlite_catalog.go`, `internal/resolver/evidence.go`, `internal/resolver/resolver.go`
- `internal/resolver/resolver_test.go`, `internal/resolver/sqlite_catalog_test.go`
- `internal/app/evaluation.go` (`EvaluationIndexVersion`)
- `testdata/evaluation/cli-manifest-v1.json` (`index_version`)

## Steps

1. Catalog schema, projection, version bump; rebuild tests pass.
2. Projection and explicit bm25 weights; existing golden-v1 resolver tests must pass unchanged (behavior-preserving proof).
3. Scoring changes with unit tests per feature.
4. Bump evaluation index version and fixture.

## Tests and validation

- `go test ./internal/catalog/ ./internal/resolver/ ./internal/evaluation/ ./internal/app/ ./internal/delivery/cli/ -run 'Eval|Resolve|Catalog'`
- New tests: an example phrase with no trigger overlap resolves to its skill; a counter-example phrase is penalized/excluded for that skill; `technologies: [go]` plus `active_artifact.language: go` adds `technology_match` and lifts the score; a skill without new fields scores exactly as before (compare feature vectors on golden-v1 skills).
- All existing golden-v1 assertions (`internal/resolver/resolver_test.go`, `internal/evaluation/evaluation_test.go`) unchanged and passing.
- `SKILLHUB_PERF=1 go test -p 1 -run Performance ./internal/resolver/ ./internal/catalog/` stays within budget.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Counter-examples hard-exclude a skill for legitimate requests | Medium × Medium | Counter-examples are full phrasings, so ≥ .72 overlap means a near-identical request; eval counter-example cases and Phase 12 measure it. |
| Catalog rebuild on upgrade | High × Low | Automatic via `StateIncompatible` → `EnsureCatalog`; documented. |
| Existing evaluation manifests pinned to `sqlite-fts5-v2` fail validation | Medium × Low | Pin change is intentional (index shape changed); fixture updated; manifests are regenerated with `skillhub eval manifest`. |

## Rollback

Revert; the catalog rebuilds at version 2.
