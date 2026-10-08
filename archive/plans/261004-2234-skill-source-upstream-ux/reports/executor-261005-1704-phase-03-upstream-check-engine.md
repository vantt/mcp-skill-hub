# Phase 3: Upstream check engine — Completion Report

**Date:** 2026-10-05 17:04 (Asia/Saigon)  
**Branch:** `feat/skill-source-upstream`  
**Status:** Completed  

## What Changed

1. **Task 3.1 (Operational Table & Store Methods):**
   - Added DDL for table `skill_upstream_state` with `STRICT` enforcement to `openOnce` in `internal/source/operational_store.go`.
   - Defined `UpstreamState` struct in `internal/source/operational_store.go`.
   - Implemented `RecordUpstream(ctx, states)` with transaction and timestamp-guarded upsert (`ON CONFLICT(skill_id) DO UPDATE ... WHERE excluded.checked_at >= skill_upstream_state.checked_at`).
   - Implemented `ListUpstream(ctx)` reading states ordered by `skill_id ASC`, correctly parsing RFC3339Nano timestamps and `ChangedFiles` (`null` to `nil`).
   - Implemented `DeleteUpstreamExcept(ctx, keep)` deleting rows for untracked skills or truncating table when `keep` is empty.
   - Added unit test `TestOperationalUpstreamState` in `internal/source/operational_store_test.go`.

2. **Task 3.2 (Pure Status Derivation & Local Digest):**
   - Created `internal/app/upstream.go` implementing:
     - `is40Hex` validating 40-character lowercase hexadecimal ref pins.
     - `deriveLocalStatus` comparing working-tree digest to origin `files_digest` (`unknown`, `clean`, `modified`).
     - `deriveUpstreamStatus` pure function implementing all first-match conditions of the Requirement 2 state table.
     - `workingTreeSkillFilesDigest` using `inventorySkillResources` and `skillruntime.ContentDigest`.
   - Created `internal/app/upstream_test.go` with table test `TestDeriveUpstreamStatus` covering all 10 condition rows and empty `files_digest`.

3. **Task 3.3 (Per-Source Check & CheckSources Integration):**
   - Implemented `CommitTime` on `GitRepositoryAdapter` in `internal/source/git_repository.go` reading the committer timestamp under shared mirror lock.
   - Created `internal/app/source_links.go` with `readSkillSourceLinks` and `learningSourceIDs`.
   - In `internal/app/upstream.go`:
     - Defined `SkillUpstreamFile`, `SkillUpstream`, and `TrackedSkill`.
     - Implemented `deriveNextAction`, `loadTrackedSkills`, `loadTrackedSkillsBySource`.
     - Implemented `reconstructSkillFilesAtCommit` and `diffReconstructedFiles`.
     - Implemented `checkSourceUpstream` and `checkSkillUpstreamState` supporting pinned bypass, missing adapter checks, remote ref probing, mirror sync once, file reconstruction, and diffs.
     - Implemented `ListSkillUpstream` and `GetSkillUpstream` (and exposed on `SkillService`).
   - In `internal/app/source.go`:
     - Added `Skills []SkillUpstream` to `SourceCheckItem`.
     - Integrated upstream checks into `CheckSources`: upstream-only sources branch to `checkUpstreamOnlySource`, skip canonical revision update, count `updates_available` toward `Changed`, and record operational state. Non-upstream-only sources run the standard pipeline and attach skill models.
     - Invoked `store.DeleteUpstreamExcept` at the end of `CheckSources`.
   - Added integration test `TestUpstreamCheck` in `internal/app/upstream_test.go` verifying `updates_available`, `ChangedFiles`, `LatestCommittedAt`, unchanged catalog records, `diverged` on local edits, `upstream_removed` on upstream deletion, and outside-commit isolation.

4. **Task 3.4 (Status Integration):**
   - Added `UpstreamUpdates int` to `CurationSummary` in `internal/app/curation_home.go`.
   - Updated `ChangedSources` query in `internal/app/curation_home.go` using `COALESCE(json_extract(..., '$.purpose'), '')` to exclude upstream-only sources while retaining sources with learning links.
   - Counted `update_available` and `diverged` skills in `readHomeCounts` into `Summary.UpstreamUpdates`.
   - Generated `review_upstream_updates` action item (Priority 75), recommendation label, and summary text.
   - Added `upstream_updates` to `schemas/curation-home-v1.schema.json`.
   - Added `TestCurationHomeUpstreamUpdatesAndExcludesUpstreamOnlySources` in `internal/app/curation_home_test.go`.
   - Regenerated web golden files (`home.json`).

5. **Task 3.5 (Quality Gate):**
   - Refactored `checkSourceUpstream` to satisfy `funlen` (< 120 lines) and dropped unused `root` parameter.
   - Fixed De Morgan's law warning in `is40Hex` for `staticcheck`.
   - Fixed `gofmt` whitespace.
   - Ran `make check`: 0 lint issues, all tests passing across all 24 packages.
   - Marked Phase 3 `status: done` and plan row `Done`.

## Every Command Run with Its Result

1. `go test ./internal/source/ -run TestOperationalUpstreamState -count=1` -> ok (exit 0)
2. `go test ./internal/app/ -run TestDeriveUpstreamStatus -count=1` -> ok (exit 0)
3. `go test ./internal/app/ -run 'TestUpstreamCheck|TestSourceChecks' -count=1` -> ok (exit 0)
4. `go test ./internal/delivery/web/ -run TestReadEndpointsGolden -update -count=1` -> ok (exit 0)
5. `go test ./internal/app/ -run 'TestCurationHome|TestGetCurationHome' -count=1 && go test ./internal/delivery/cli/ -run TestStatusJSON -count=1 && git diff --stat internal/delivery/web/testdata/golden/` -> ok (exit 0)
6. `make check` -> 0 issues, all 24 packages passed (exit 0)

## Deviations and Why

- Wrapped `json_extract(s.content_json, '$.purpose')` with `COALESCE(..., '')` in the `ChangedSources` SQL query (`curation_home.go:216`) because SQL ternary logic evaluates `NOT (NULL = 'upstream')` to `NULL` (falsy in `WHERE`), which would incorrectly exclude normal sources without a purpose attribute.

## Open Questions

None.
