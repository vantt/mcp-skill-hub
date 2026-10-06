# Phase 1: Upstream provenance model — Completion Report

**Date:** 2026-10-05 15:54 (Asia/Saigon)  
**Branch:** `feat/skill-source-upstream`  
**Status:** Completed  

## What Changed

1. **Task 1.0 (Runtime Plan Overlap Check):**
   - Verified that `plans/261004-1547-skill-execution-measurement-routing/plan.md` has all phases marked as `Done`. No file overlaps.

2. **Task 1.1 (Canonical files_digest):**
   - Added `files_digest` to allowed `provenance.origin` keys in `internal/canonical/skill.go` and verified format (`sha256:<64 hex>`).
   - Updated `schemas/skill-metadata.schema.json` with `files_digest` validation.
   - Added valid and invalid test cases in `internal/canonical/canonical_test.go` and `schemas/embed_test.go`.

3. **Task 1.2 (RevisionAt, RemoteRefCommit, Mirror Lock):**
   - Added `ErrMirrorBusy` and `ErrPathNotFound` sentinels in `internal/source/git_repository.go`.
   - Implemented `withMirrorLock` helper using `github.com/gofrs/flock` with a 30-second timeout.
   - Split `syncMirror` into public locked entry and `syncMirrorLocked`.
   - Added `openMirrorLocked` and `openOrCloneLocked`.
   - Wrapped `Diff`, `Read`, `List`, and `CommitHasSkill` in shared mirror locks around mirror access.
   - Updated `scopedObjectHash` to return `ErrPathNotFound` when an entry/directory is missing.
   - Updated `resolveCommit` to explicitly reject references under `refs/skillhub/commits/`.
   - Added `RemoteRefCommit` to resolve ref/tag/branch commits cheaply via `ListAdvertisedRefs` without modifying mirror.
   - Added `RevisionAt` to fetch missing commits at depth 1 into `refs/skillhub/commits/<sha>` with safe transport and map unavailable commits to `ErrHistoryUnavailable`.
   - Added comprehensive tests in `internal/source/git_revision_at_test.go` covering shallow clone, SHA fetching, missing paths, unknown commits, ref isolation, tag peeling, and mirror lock contention.

4. **Task 1.3 (Origin Helper & Skill Add):**
   - Created `internal/app/upstream_origin.go` with `repoRelativeSkillPath`, `filesDigestOf`, `importedSkillFiles`, `revisionAtAdapter`, `makeRevisionAtCallback`, and `buildGitOrigin`.
   - Updated `SkillOrigin` in `internal/app/skill_add.go` with `FilesDigest` and serialized it in `skillOriginToMap`.
   - Updated `skillAddCapture` with `scopePath` and `revisionAt` callback.
   - Updated `captureRemoteSkillAddSource` to set `Commit: rev.Value`, retain `scopePath: resolved.Path`, and supply the `revisionAt` callback.
   - Updated `buildSkillAddChanges` to use `importedSkillFiles` and `buildGitOrigin`.
   - Extended `TestSkillAddRemoteGitRealAdapter` in `internal/app/skill_add_test.go` to test multi-skill additions (`skills/a`, `skills/b`), asserting `origin.path`, `origin.commit`, differing `folder_digest`, and equality of `files_digest` to `ContentTrustFor().ContentDigest`.

5. **Task 1.4 (Quality Gate):**
   - Fixed minor lint warnings (double blank lines, lowercased error strings in git repository adapter).
   - Ran `make check`: 0 lint issues, all tests passing across all 24 packages.
   - Marked Phase 1 `status: done` and plan row `Done`.

## Every Command Run with Its Result

1. `git branch --show-current` -> `feat/skill-source-upstream` (exit 0)
2. `TZ=Asia/Saigon date +"%Y-%m-%d %H:%M %z"` -> `2026-10-05 15:39 +0700` (exit 0)
3. `grep -nE "\| (Pending|In progress) \|" plans/261004-1547-skill-execution-measurement-routing/plan.md` -> no output (exit 1, runtime plan complete)
4. `go test ./internal/canonical/ ./schemas/ -count=1` -> ok (exit 0)
5. `go test ./internal/source/ -run 'TestGitRevisionAt' -count=1` -> ok (exit 0)
6. `go test ./internal/source/... -count=1` -> ok (exit 0)
7. `go vet ./internal/app/...` -> ok (exit 0)
8. `go test ./internal/app/ -run 'TestSkillAdd' -count=1` -> ok (exit 0)
9. `make check` (first run) -> gofmt & staticcheck errors on `internal/source/git_repository.go` (exit 2)
10. `make check` (after formatting/lint fixes) -> 0 issues, all 24 packages passed (exit 0)
11. `git log -n 5 --oneline` -> confirmed 5 conventional commits (exit 0)

## Deviations and Why

- Lowercased internal error strings in `internal/source/git_repository.go` (`"git source ref ..."` and `"git HTTPS source operation failed"`) to satisfy `staticcheck` ST1005 required by `make check`. No callers assert on the capitalized error text.

## Open Questions

None.
