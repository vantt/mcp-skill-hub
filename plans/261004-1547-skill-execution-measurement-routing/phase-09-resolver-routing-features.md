---
phase: 9
title: "Resolver routing features"
status: pending
priority: P1
effort: 8h
dependencies: [1]
---

# Phase 9: Resolver routing features

## Goal

The resolver uses `routing.examples`, `routing.counter_examples`, `topics`, and `technologies` (already accepted by the manifest schema since phase 1) for candidacy and scoring, without changing results for skills that do not use them.

## Context (read these first)

- `plan.md` → "Executor notes", D7; design `docs/design/03-resolver-design.md` §4 (index), §7 (feature model).
- Manifest schema already allows top-level `topics`, `technologies`, `domain` (`schemas/skill-metadata.schema.json:17-19`) and `routing.examples` / `routing.counter_examples` (`:41-42`, max 10 items, 300 chars).
- FTS: `CREATE VIRTUAL TABLE skill_fts USING fts5(skill_id UNINDEXED, name, aliases, description, triggers)` (`internal/catalog/schema.go:131`); filled at `internal/catalog/project.go:78`; `projectRouting` (`project.go:254`) writes `routing_metadata` rows for categories `operation, trigger, exclusion, requirement, relationship` and returns the triggers; `routing_metadata.category` is free text (`schema.go:48-54`). Skill FTS smoke query: `internal/catalog/build.go:369` (matches a name/description token; unaffected by new columns). `DerivedSchemaVersion = 2` (`internal/catalog/catalog.go:38`).
- Search: `bm25(skill_fts,0.0,8.0,5.0,7.0)` (`internal/resolver/sqlite_catalog.go:99`) — triggers currently get bm25's default weight 1.0.
- Projection: `(*SQLiteCatalog).Skills` (`sqlite_catalog.go:115`), `type skillDocument` (`:139`), `normalizeSkillRouting` (`:183`), `decodeRouting` (`:206`). `resolver.Skill` (`internal/resolver/types.go:207`).
- Scoring: `scoreSkill` (`internal/resolver/evidence.go:256`; lexical metadata at `:258`, trigger at `:259`, artifact at `:261`); rule-channel trigger rank (`internal/resolver/resolver.go:171`); helpers `tokenize` (`evidence.go:152`), `overlap` (`:170`), `bestOverlap` (`:188`), `positiveQuery` (`:360`), `equalEvidence` (`:371`).
- Evaluation pins: `EvaluationIndexVersion = "sqlite-fts5-v2"` (`internal/app/evaluation.go:25`), also in `testdata/evaluation/cli-manifest-v1.json:9` (`index_version`).
- Request evidence for technology match: `Request.ActiveArtifact *Artifact` and `Request.Facts []Fact` (`internal/resolver/types.go:47-48`).

## Requirements

1. **Index.** `skill_fts` gains `examples` (all `routing.examples` joined by space) and `keywords` (`topics` + `technologies` joined). `DerivedSchemaVersion = 3`; `EvaluationIndexVersion = "sqlite-fts5-v3"`. Counter-examples are **not** indexed (they must not create candidacy). `projectRouting` also writes `routing_metadata` categories `example` and `counter_example` (diagnostics only).
2. **Search weights** written explicitly for every column: `bm25(skill_fts, 0.0, 8.0, 5.0, 7.0, 1.0, 3.0, 2.0)` for (skill_id, name, aliases, description, triggers, examples, keywords). Triggers stay at 1.0, so existing ranking does not change; phase 12 decides any change from evidence.
3. **Projection.** `resolver.Skill` gains `Examples`, `CounterExamples`, `Topics`, `Technologies []string` with `json:"...,omitempty"`; `skillDocument` reads `routing.examples`, `routing.counter_examples`, top-level `topics`, `technologies`; `normalizeSkillRouting` sorts them. `domain` stays unused.
4. **Scoring** (no new policy weights):
   - Trigger feature: `max(bestOverlap(query, Triggers), 0.9 × bestOverlap(query, Examples))`; reason code `example_match` when the example term wins and is ≥ 0.3. Define the 0.9 as a named constant (phase 12 may tune it).
   - Rule-channel trigger rank uses the same combined value.
   - Not-for feature: `max(bestOverlap(query, NotFor), bestOverlap(query, CounterExamples))`, so counter-examples share the existing soft penalty (≥ .35 → unknown exclusion) and hard exclusion (≥ .72).
   - Lexical metadata tokens: name + description + aliases + topics + technologies.
   - Technology match: `techEvidence` = `ActiveArtifact.Language` plus values of facts whose key is one of `language, framework, library, dependency, technology, runtime, platform` (any basis; soft positive signal). If any `Technologies` entry `equalEvidence` a techEvidence value, the artifact feature becomes `max(current, 1.0)` and reason code `technology_match` is added. No penalty on mismatch.
5. Determinism unchanged: same catalog snapshot and request → same response.

## Files

Modify:
- `internal/catalog/schema.go`, `internal/catalog/project.go`, `internal/catalog/catalog.go`, `internal/catalog/catalog_test.go`
- `internal/resolver/types.go`, `internal/resolver/sqlite_catalog.go`, `internal/resolver/evidence.go`, `internal/resolver/resolver.go`
- `internal/resolver/resolver_test.go`, `internal/resolver/sqlite_catalog_test.go`
- `internal/app/evaluation.go` (`EvaluationIndexVersion`), `testdata/evaluation/cli-manifest-v1.json` (`index_version`)
- Any test that pins `DerivedSchemaVersion` 2 or `sqlite-fts5-v2` (enumerate first: `grep -rn 'DerivedSchemaVersion\|sqlite-fts5-v2' --include='*.go' --include='*.json' internal testdata`)

## Steps

- [ ] **1. Catalog schema, projection, version bump.**
  Pass: `go test -count=1 ./internal/catalog/` → `ok`; a test asserts `SELECT examples, keywords FROM skill_fts WHERE skill_id=?` returns the joined values for a fixture skill.
- [ ] **2. Projection and explicit bm25 weights.** Existing golden-v1 resolver and evaluation tests pass unchanged (behavior-preserving proof).
  Pass: `go test -count=1 ./internal/resolver/ ./internal/evaluation/` → `ok` with no edits to golden-v1 expectations.
- [ ] **3. Scoring changes** with unit tests: an example phrase with no trigger overlap resolves to its skill (`example_match`); a counter-example phrase is penalized/excluded for that skill; `technologies: [go]` + `active_artifact.language: go` adds `technology_match` and lifts the score; for every golden-v1 skill (none has the new fields) the feature vector is identical before and after (compare `scoreSkill` output).
  Pass: `go test -count=1 -run 'Example|Counter|Technology|FeatureVector' ./internal/resolver/` → `ok`.
- [ ] **4. Evaluation index version** bump and fixture update.
  Pass: `go test -count=1 ./internal/app/ ./internal/delivery/cli/ -run 'Eval|Resolve|Catalog'` → `ok`.
- [ ] **5. Performance budget.**
  Pass: `SKILLHUB_PERF=1 go test -p 1 -count=1 -run Performance ./internal/resolver/ ./internal/catalog/` → `ok`.
- [ ] **6. Gate.** Pass: `make check` exits 0.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Counter-examples hard-exclude a skill for legitimate requests | Medium × Medium | Counter-examples are full phrasings; ≥ .72 overlap means a near-identical request; phase 10 counter cases and phase 12 measure it. |
| Catalog rebuild on upgrade | High × Low | Automatic via `StateIncompatible` → `EnsureCatalog`; documented in phase 13. |
| Stored evaluation manifests pinned to `sqlite-fts5-v2` fail validation | Medium × Low | Intentional (index shape changed); regenerate with `skillhub eval manifest`. |

## Rollback

Revert; the catalog rebuilds at version 2.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol". In particular, if a golden-v1 assertion changes in step 2, stop: that means the change is not behavior-preserving. Write `reports/<agent>-<YYMMDD-HHMM>-resolver-routing-features.md`, set `status: blocked`, report the blocker.
