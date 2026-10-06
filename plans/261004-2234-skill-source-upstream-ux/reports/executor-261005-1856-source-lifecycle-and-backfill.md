# Executor Report: Phase 5 - Source Lifecycle and Backfill

**Phase:** Phase 5: Source lifecycle and backfill
**Branch:** `feat/skill-source-upstream`
**Timestamp:** 2026-10-05 18:56 Asia/Saigon

## 1. What Changed

1. **Source Link Operations (`internal/app/source_links.go`, `internal/app/source_links_test.go`)**:
   - Implemented `PreviewAttach`: supports linking existing or new sources to a skill as `learning-source` in `sources/skills/LINK-<skill>--<source>.yaml`, idempotently reusing matching records.
   - Implemented `PreviewDetach`: unlinks learning sources without deleting the source record if referenced elsewhere or if external observations/runs exist.
   - Implemented `PreviewUnwatch`: deletes unreferenced sources from `sources/catalog/<id>.yaml` or warns if still linked to other skills.
   - Extended `ConfirmSourceProposal` command dispatch for `source_attach`, `source_detach`, `source_unwatch`.

2. **Source Watch & Triage Policy Enforcement (`internal/app/source_watch.go`, `internal/app/source.go`)**:
   - `PreviewSourceWatch` enforces that a watched source must belong to a skill (`invalid_request` if `SkillID` is omitted after locator validation) and delegates to `PreviewAttach`.
   - `TriageSourceCandidate` accepts `decision: "import"`, delegating to `PreviewSkillAdd` and staging candidate acceptance.
   - `TriageSourceCandidate` with `decision: "accept"` requires either `--skill-id` or `--new-skill`, scaffolding a new draft skill in one transaction if `--new-skill` is supplied.

3. **Grouped Read Model & Import-More (`internal/app/source.go`, `internal/app/source_import.go`)**:
   - Updated `SourceListResult` with `Sources []SourceListItem` containing `Record`, `Skills`, `Role` (`upstream`, `learning-source`, `both`, `unattached`), and `ImportableCount`.
   - Added `ListSourceGroups` returning repositories with `SourceSummary` items.
   - Added `Imported` field on `DiscoveredSkill` and `commonParentDirForSource` scoping for whole-repo imports without path.

4. **Source Backfill Engine (`internal/app/source_backfill.go`, `internal/app/source_backfill_test.go`)**:
   - Implemented `PreviewBackfill` and `ApplyBackfill` supporting Candidate (A) (`origin.kind` in github/git without source) and Candidate (B) (`created_by: source_import` without origin from a git source; excluding filesystem sources).

5. **Status Actions (`internal/app/upstream.go`, `internal/app/upstream_test.go`)**:
   - Completed `deriveNextAction` covering all 8 upstream statuses (`up_to_date`, `update_available`, `modified`, `diverged`, `upstream_removed`, `unavailable`, `untracked`, `pinned`).

6. **CLI and MCP Delivery Updates (`internal/delivery/cli/`, `internal/delivery/mcpserver/`)**:
   - Added `--new-skill` flag to CLI source triage.
   - Forwarded `SkillID` and `NewSkillID` across CLI and MCP endpoints.
   - Adapted callers to `SourceListItem.Record`.

## 2. Commands Executed and Results

| Command | Purpose | Result |
|---|---|---|
| `git branch --show-current` | Verify active branch | `feat/skill-source-upstream` |
| `go test ./internal/app/ -run TestSourceLinks -count=1` | Task 5.1 verification | Pass (exit 0) |
| `go test ./internal/app/ -run 'Test(SourceWatch\|SourceTriage)' -count=1 && go test ./internal/delivery/cli/ -run TestSourceWatch -count=1` | Task 5.2 verification | Pass (exit 0) |
| `go test ./internal/app/ -run 'Test(SourceList\|SourceImport)' -count=1` | Task 5.3 verification | Pass (exit 0) |
| `go test ./internal/app/ -run TestBackfill -count=1` | Task 5.4 verification | Pass (exit 0) |
| `go test ./internal/app/ -run TestNextAction -count=1` | Task 5.5 verification | Pass (exit 0) |
| `make check` | Task 5.6 full check gate (vet, golangci-lint, full test suite) | Pass (exit 0) |

## 3. Deviations and Why

None. All implementations and constraints strictly adhered to the Phase 5 plan and architecture decisions.

## 4. Open Questions

None.
