# Phase 9: Resolver routing features report

## Summary
- Catalog schema: updated `skill_fts` virtual table schema to add `examples` and `keywords` columns. Bumped `DerivedSchemaVersion` 2 -> 3 in `internal/catalog/catalog.go`.
- Catalog projection: updated `projectRouting` to record `example` and `counter_example` categories in `routing_metadata`. Populated `examples` in `skill_fts` with space-joined `routing.examples`, and `keywords` with space-joined `topics` and `technologies`. Updated `expectedRowCounts` in `internal/catalog/project.go` to account for new routing metadata rows.
- Search weights: updated `SQLiteCatalog.Search` in `internal/resolver/sqlite_catalog.go` to explicitly set bm25 weights for all columns: `bm25(skill_fts, 0.0, 8.0, 5.0, 7.0, 1.0, 3.0, 2.0)`.
- Model projection: `resolver.Skill` now projects `Examples`, `CounterExamples`, `Topics`, `Technologies []string` with `omitempty`. Updated `skillDocument`, `decodeRouting`, and `normalizeSkillRouting` in `sqlite_catalog.go`.
- Scoring updates:
  - Trigger feature uses `max(bestOverlap(query, Triggers), 0.9 * bestOverlap(query, Examples))`. Defined named constant `ExampleOverlapWeight = 0.9` in `internal/resolver/evidence.go`. Emits `example_match` when example overlap wins and is >= 0.3.
  - Updated rule-channel trigger ranking in `internal/resolver/resolver.go` to use `triggerFeature`.
  - Not-for feature uses `max(bestOverlap(query, NotFor), bestOverlap(query, CounterExamples))`, sharing existing soft penalties and hard exclusions.
  - Lexical metadata tokenization includes `Topics` and `Technologies`.
  - Technology matching: collects evidence from `ActiveArtifact.Language` and facts with key `language, framework, library, dependency, technology, runtime, platform` (case-insensitive via `normalizeIdentifier`). If matched against `skill.Technologies`, sets `f.Artifact = max(f.Artifact, 1.0)` and appends `technology_match` reason code.
- Evaluation index version: bumped `EvaluationIndexVersion` to `sqlite-fts5-v3` in `internal/app/evaluation.go` and `testdata/evaluation/cli-manifest-v1.json`.

## Verification
- Unit & regression tests:
  - `go test -count=1 ./internal/catalog/` (PASS, includes verification of `SELECT examples, keywords FROM skill_fts`)
  - `go test -count=1 ./internal/resolver/ ./internal/evaluation/` (PASS, golden-v1 resolver & evaluation preserved)
  - `go test -count=1 -run 'Example|Counter|Technology|FeatureVector' ./internal/resolver/` (PASS)
  - `go test -count=1 ./internal/app/ ./internal/delivery/cli/ -run 'Eval|Resolve|Catalog'` (PASS)
  - `SKILLHUB_PERF=1 go test -p 1 -count=1 -run Performance ./internal/resolver/ ./internal/catalog/` (PASS)
- Quality gate:
  - `make check` (vet, golangci-lint with 0 issues, go test across all packages) passes cleanly.
