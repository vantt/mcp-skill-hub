# Phase 4: Update merge and apply — Completion Report

**Date:** 2026-10-05 17:31 (Asia/Saigon)  
**Branch:** `feat/skill-source-upstream`  
**Status:** Completed  

## What Changed

1. **Task 4.1 (Git-backed merge and diff):**
   - Created `internal/app/upstream_merge.go` implementing:
     - `mergeableText` checking UTF-8, no NUL bytes, max 256 KiB.
     - `createMergeTempDir` and `cleanupOldMergeDirs` creating isolated `0600`/`0700` directories under `runtime/tmp/upstream-merge-*`.
     - `gitMergeFile` executing `git merge-file -p --diff3` with exit-code handling (0 clean, 1..127 conflict count).
     - `gitDiffNoIndex` executing `git diff --no-index` with `/dev/null` absent-side substitution and header path rewriting.
   - Exported `canonical.HasConflictMarker` in `internal/canonical/canonical.go` (and retained unexported alias for compatibility).
   - Created `internal/app/upstream_merge_test.go` verifying non-overlapping edits, same-line conflicts with 3-way markers, 2 conflicts, adjacent edits against direct git, identical edits, missing newlines, CRLF preservation, NUL rejection, and header rewriting.

2. **Task 4.2 (Proposal Kind & Kind Guard):**
   - Added `ProposalKindUpstreamUpdate ProposalKind = "upstream_update"` in `internal/skill/lifecycle.go`.
   - Updated `LoadProposal` in `internal/skill/proposal_store.go` to accept `ProposalKindUpstreamUpdate`.
   - Renamed `storeSkillAddProposal` to `storeProposalArtifact` in `internal/app/skill_add.go`.
   - Implemented kind guard in `SkillService.LoadSkillProposal` (`internal/app/skill_lifecycle.go`), refusing non-lifecycle proposal kinds with `invalid_request`.
   - Added round-trip unit test for `upstream_update` proposal artifacts in `internal/skill/proposal_store_test.go`.

3. **Task 4.3 (Preview, Write Set, Confirm):**
   - Created `internal/app/upstream_update.go` implementing:
     - Data models: `UpstreamResolution`, `UpstreamUpdateInput`, `UpstreamFile`, `TrustImpact`, `UpstreamUpdatePreview`, `UpstreamUpdateResult`.
     - Helper `updateSkillMetaYAML` via `yaml.Node` updating origin commit, folder digest, and files digest while preserving key ordering and comments, leaving `quality.content_reviewed_digest` untouched.
     - Helper `validateAndFilterUpstreamPaths` enforcing printable characters, ignoring case variants of `skill.meta.yaml`, and rejecting case-folding collisions.
     - Helper `readLocalSkillFiles` loading working-tree files except metadata.
     - Helper `classifyUpstreamFile` implementing all rows of the Requirement 3 classification table.
     - Modular helpers `validateUpdatePreconditions`, `reconstructBaseAndUpstreamFiles`, `classifyAllFiles`, `makeUnresolvedPreview`, and `buildAndPlanUpstreamWriteSet`.
     - `UpstreamService.PreviewUpdate` planning write sets when unresolved is empty or returning decision summaries when unresolved.
     - `UpstreamService.ConfirmUpdate` applying write sets via `confirmAndPublish`, updating operational DB state, re-checking content trust, and returning `UpstreamUpdateResult`.
     - Registered confirmer for `ProposalKindUpstreamUpdate` in `init()`.
   - Added MCP guard test `TestSkillTransitionConfirmRefusesUpstreamUpdateProposal` in `internal/delivery/mcpserver/skill_tools_test.go`.
   - Created `internal/app/upstream_update_test.go` with 12 subtests covering clean update, merged, conflict with manual resolution, removed upstream with local edit, stale detection, base mismatch, dispatch confirm, kind guard, shallow SHA base fetch, nested skill exclusion, rejected paths/blocked markers, and planned before-digest checks.

4. **Task 4.4 (Quality Gate):**
   - Refactored `PreviewUpdate` and helpers to satisfy `funlen` (< 120 lines) and `argument-limit` (< 6 parameters).
   - Ran `make check`: 0 lint issues, all tests passing across all 24 packages.
   - Marked Phase 4 `status: done` and plan row `Done`.

## Every Command Run with Its Result

1. `go test ./internal/canonical/... -count=1` -> ok (exit 0)
2. `go test ./internal/app/ -run TestUpstreamGitMerge -count=1` -> ok (exit 0)
3. `go test ./internal/skill/ -count=1` -> ok (exit 0)
4. `go test ./internal/delivery/mcpserver/ -run TestSkillTransitionConfirmRefusesUpstreamUpdateProposal -count=1` -> ok (exit 0)
5. `go test ./internal/app/ -run TestUpstreamUpdate -count=1` -> ok (exit 0)
6. `make check` -> 0 issues, all 24 packages passed (exit 0)

## Deviations and Why

- Decomposed `PreviewUpdate` and `buildAndPlanUpstreamWriteSet` into smaller modular helpers (`reconstructBaseAndUpstreamFiles`, `classifyAllFiles`, `makeUnresolvedPreview`, `buildUpstreamFileChanges`) to satisfy `funlen` (max 120 lines) and `revive` (max 6 parameters) rules enforced by `make check`.

## Open Questions

None.
