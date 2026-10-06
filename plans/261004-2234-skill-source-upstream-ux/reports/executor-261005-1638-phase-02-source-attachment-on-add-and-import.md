# Phase 2: Source attachment on add and import — Completion Report

**Date:** 2026-10-05 16:38 (Asia/Saigon)  
**Branch:** `feat/skill-source-upstream`  
**Status:** Completed  

## What Changed

1. **Task 2.1 (Source matching helper & Record Purpose):**
   - Added `Purpose` field (`purpose,omitempty`) to `sourcepkg.Record` in `internal/source/records.go` and enforced validation: only `""` or `"upstream"` permitted.
   - Updated `schemas/source-record.schema.json` to allow `"purpose": {"enum": ["upstream"]}`.
   - Created `internal/app/upstream_source.go` implementing:
     - `UpstreamSourceRef` struct (`SourceID`, `Created`).
     - `sameRepository` comparing repository URLs with scheme/host lowercasing, GitHub-specific path lowercasing, and trailing `/` and `.git` trimming.
     - `upstreamSourceID` with ref-suffix and numeric collision resolution (`-2`, `-3`).
     - `ensureUpstreamSource` creating or matching source records with `purpose: upstream`, weekly cadence, and depth-1 `CurrentRevision`.
   - Created `internal/app/upstream_source_test.go` with table tests for `sameRepository`, ID derivation, and `purpose` validation.

2. **Task 2.2 (`skill add` attaches the source):**
   - Added `UpstreamSource` to `SkillAddProposal` and `SkillAddResult`.
   - In `PreviewSkillAdd`, invoked `ensureUpstreamSource` for non-local origins, passing the created source record and change to `buildSkillAddChanges`.
   - In `buildSkillAddChanges`, appended `sourceChange` to `changes` and wrote `provenance.source_id`.
   - Implemented `deriveUpstreamSourceFromChanges` helper to satisfy Requirement 6 for `ConfirmSkillAdd`, `ConfirmSkillAddProposal`, and `LoadSkillAddProposal`.
   - Updated `internal/canonical/canonical.go` (line 301) with maintainer approval to allow `file://` repositories when `adapter == "git"` in `validateSourcePolicy`, unblocking local git test harnesses.
   - Extended `TestSkillAddRemoteGitRealAdapter` in `internal/app/skill_add_test.go` verifying dual-skill addition, single catalog upstream record, metadata `source_id`, proposal dispatch confirmation, in-memory confirmation, and clean canonical validation.

3. **Task 2.3 (`source import` writes origin, drops links):**
   - In `PreviewSourceImport` (`internal/app/source_import.go`), dynamically queried `adapter.CurrentRevision` for git adapters.
   - Extracted `buildImportItemChanges` helper. For git sources, generated structured `provenance.origin` (`kind`, `repository`, `ref`, `commit`, `path`, `folder_digest`, `files_digest`, `content_digest`) and `provenance.source_id`.
   - Completely dropped creation of legacy `sources/skills/LINK-*` files for all adapters.
   - Updated `internal/app/source_import_test.go` to assert absence of `LINK-*` files and presence of `source_id` and `origin:` with stub current revisions.

4. **Task 2.4 (CLI output):**
   - In `writeSkillAddPreview` (`internal/delivery/cli/skill_add.go`), dropped `Watching: off.` and added upstream tracking lines (`Upstream: tracked by new source <id>...` or `Upstream: tracked by source <id>.`).
   - In `writeSkillAddResult`, replaced `Watching: off.` with `Upstream: <id>.` when attached, or dropped it for local adds.
   - Added `TestSkillAddPreviewAndResultRenderUpstream` in `internal/delivery/cli/skill_add_test.go`.

5. **Task 2.5 (Quality Gate):**
   - Ran `make check`: 0 lint issues, all tests passing across all 24 packages.
   - Marked Phase 2 `status: done` and plan row `Done`.

## Every Command Run with Its Result

1. `git branch --show-current` -> `feat/skill-source-upstream` (exit 0)
2. `go test ./internal/app/ -run TestUpstreamSource -count=1` -> ok (exit 0)
3. `go test ./internal/source/ ./schemas/ -count=1` -> ok (exit 0)
4. `go test ./internal/app/ -run 'TestSkillAdd' -count=1` (first run) -> failed due to `file://` rejection in `canonical.go` (exit 1)
5. `go test ./internal/app/ -run 'TestSkillAdd' -count=1` (after canonical fix) -> ok (exit 0)
6. `go test ./internal/app/ -run 'TestSourceImport' -count=1` -> ok (exit 0)
7. `go test ./internal/delivery/cli/ -run 'TestSkillAdd' -count=1` -> ok (exit 0)
8. `make check` -> 0 issues, all 24 packages passed (exit 0)

## Deviations and Why

- With maintainer approval, updated `internal/canonical/canonical.go` line 301 in `validateSourcePolicy` to use `sourcepkg.ValidateRemoteURLWithOptions(value, sourcepkg.URLValidationOptions{AllowFile: adapter == "git"})`. This harmonized canonical validation with `records.go`'s `ValidateLocator` and unblocked `file://` local git test harnesses without altering production constraints.

## Open Questions

None.
