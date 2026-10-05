# Phase 9: Resolver routing features report

## Summary
- Updated catalog schema and projection in `internal/catalog/`:
  - Added `examples` and `keywords` columns to `skill_fts` virtual table.
  - Bumped `DerivedSchemaVersion` from 2 to 3.
  - Updated `projectRouting` to insert `example` and `counter_example` categories into `routing_metadata` table and compute expected row counts.
  - Updated `skill_fts` insertion in `projectSkills` to index `examples` and combined `keywords` (`topics` + `technologies`).
- Updated resolver projection and search ranking in `internal/resolver/`:
  - Updated `skill_fts` BM25 query with explicit weights for all 7 columns: `bm25(skill_fts, 0.0, 8.0, 5.0, 7.0, 1.0, 3.0, 2.0)`, keeping triggers at 1.0.
  - Added `Examples`, `CounterExamples`, `Topics`, and `Technologies` to `resolver.Skill` and `skillDocument` with sorting in `normalizeSkillRouting`.
- Implemented updated scoring in `internal/resolver/evidence.go` and `internal/resolver/resolver.go`:
  - Trigger feature combines triggers and examples: `max(bestOverlap(query, Triggers), 0.9 * bestOverlap(query, Examples))`, adding reason code `example_match` when example overlap wins and is >= 0.3. Constant `ExampleDiscount = 0.9`.
  - Rule-channel trigger rank uses the same combined trigger/example feature value.
  - Not-for feature combines not_for and counter_examples: `max(bestOverlap(query, NotFor), bestOverlap(query, CounterExamples))`.
  - Lexical metadata tokens include `name + description + aliases + topics + technologies`.
  - Technology matching: checks `ActiveArtifact.Language` and tech-related facts (`language, framework, library, dependency, technology, runtime, platform`) against `skill.Technologies`. If matched, artifact feature becomes `max(current, 1.0)` and reason code `technology_match` is added.
- Updated evaluation pins:
  - Bumped `EvaluationIndexVersion` to `"sqlite-fts5-v3"` in `internal/app/evaluation.go`.
  - Updated `index_version` in `testdata/evaluation/cli-manifest-v1.json`.

## Verification
- Unit & regression tests:
  - `go test -count=1 ./internal/catalog/` (PASS, asserted `SELECT examples, keywords FROM skill_fts`)
  - `go test -count=1 ./internal/resolver/ ./internal/evaluation/` (PASS, behavior-preserving proof across golden-v1 cases)
  - `go test -count=1 -run 'Example|Counter|Technology|FeatureVector' ./internal/resolver/` (PASS: verified example_match, counter_example exclusion, technology_match score lift, and identical feature vectors for all golden-v1 skills)
  - `go test -count=1 ./internal/app/ ./internal/delivery/cli/ -run 'Eval|Resolve|Catalog'` (PASS)
  - `SKILLHUB_PERF=1 go test -p 1 -count=1 -run Performance ./internal/resolver/ ./internal/catalog/` (PASS)
  - `make check` (go vet, golangci-lint 0 issues, full test suite across all 24 packages) (PASS)
