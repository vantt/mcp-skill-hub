# Executor Report: Phase 8 - Web API

**Phase:** Phase 8: Web API
**Branch:** `feat/skill-source-upstream`
**Timestamp:** 2026-10-05 21:42 Asia/Saigon

## 1. What Changed

1. **Read Models & Skills List Field (`internal/app/skill_sources.go`, `internal/app/skill_list.go`)**:
   - Implemented `SourceService.SkillSources(ctx, path, skillID)` in `internal/app/skill_sources.go`: returns `SkillSourcesResult` containing `Upstream` (nil when local or untracked), `Learning` references, and total/per-source `PendingInsights`.
   - Added `UpstreamStatus` field to `SkillListEntry` in `internal/app/skill_list.go` and implemented `SkillService.ListSkillsWithUpstream` which merges `ListSkillUpstream` results into skill list entries for the Web API.

2. **Server Struct & Route Definitions (`internal/delivery/web/server.go`, `internal/delivery/web/routes_sources.go`, `internal/delivery/web/routes_read.go`)**:
   - Extended `Server` with `upstream app.UpstreamService` and `sourceImport app.SourceImportService`.
   - Updated `handleSkills` in `routes_read.go` to call `ListSkillsWithUpstream`.
   - Implemented all 12 source and upstream endpoints in `internal/delivery/web/routes_sources.go`:
     - `GET /api/v1/skills/{id}/sources`
     - `POST /api/v1/skills/{id}/upstream/check`
     - `POST /api/v1/skills/{id}/upstream/review`
     - `POST /api/v1/upstream/proposals/{proposal_id}/confirm`
     - `POST /api/v1/skills/{id}/sources/attach/preview`
     - `POST /api/v1/skills/{id}/sources/{source_id}/detach/preview`
     - `GET /api/v1/sources`
     - `POST /api/v1/sources/check`
     - `POST /api/v1/sources/{id}/unwatch/preview`
     - `POST /api/v1/sources/proposals/{proposal_id}/confirm`
     - `POST /api/v1/sources/{id}/import/preview`
     - `POST /api/v1/sources/import/confirm`
   - Added validation: `POST /api/v1/sources/check` validates that exactly one of `source_ids`, `all`, or `due` is provided; locator validation enforces public GitHub URLs via `validateGitHubLocator`.

3. **Golden Tests & Test Coverage (`internal/delivery/web/routes_sources_test.go`, `internal/delivery/web/routes_read_test.go`, `internal/app/skill_sources_test.go`)**:
   - Added comprehensive HTTP route tests in `routes_sources_test.go` covering check parameter validation (400), `all` vs `due` execution, unknown skill on `/sources` (404), attach non-GitHub rejection (400), attach preview -> confirm via `/api/v1/sources/proposals/{id}/confirm` (200), upstream review -> check -> confirm (200) with injected file-protocol adapter, and server constructor non-panic check.
   - Added `sources` and `skill-sources` golden test cases in `routes_read_test.go` and generated corresponding golden files.

## 2. Commands Executed and Results

| Command | Purpose | Result |
|---|---|---|
| `git branch --show-current` | Verify branch | `feat/skill-source-upstream` |
| `go test ./internal/app/ -run 'TestSkillSources\|TestSkillList' -count=1` | Task 8.1 verification | Pass (exit 0) |
| `go test ./internal/delivery/web/ -run 'TestSourceRoutes\|TestUpstreamRoutes' -count=1` | Task 8.2 verification | Pass (exit 0) |
| `go test ./internal/delivery/web/ -count=1 && git status --porcelain internal/delivery/web/testdata/golden/` | Task 8.3 verification | Pass (exit 0) |
| `make check` | Task 8.4 gate verification (vet, golangci-lint, full test suite) | Pass (exit 0) |

## 3. Deviations and Why

None. All implementations and constraints strictly adhered to the Phase 8 plan and architecture decisions.

## 4. Open Questions

None.
