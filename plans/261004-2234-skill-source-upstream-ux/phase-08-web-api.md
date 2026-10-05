---
title: "Phase 8: Web API"
status: todo
---

# Phase 8: Web API

<!-- Updated: Validation Session 1 - WebUI and CLI are the only update paths -->

## Context

- Plan: [plan.md](./plan.md). Depends on phases 3–5 (services) and on the runtime plan's WebUI parity phase being complete (it also edits `routes_read_test.go`).
- Read first: `internal/delivery/web/server.go` (Server fields, `registerRoutes`), `routes_read.go`, `routes_skill_write.go` (`decodeJSON`, `confirmationRequest`, `handleSkillConfirm` at line 227), `errors.go` (`statusByCode`, `writeError`, `writeAppError`), `locator.go` (`validateGitHubLocator`), `security.go`, `fixtures_test.go` (`newWebWorkspace`, `newTestServer`), `routes_read_test.go` (golden mechanism, `-update`), `internal/app/skill_list.go`.

## Overview

Expose the upstream and source services to the WebUI with the same envelopes, error mapping, and preview/confirm pins as the existing skill routes. Upstream review responses include diffs: the WebUI is, with the CLI, one of the only two places an upstream update can be reviewed and applied (agents cannot, Validation Session 1 decision 5); nothing here weakens the agent-facing rules.

## Requirements

1. Server gains `upstream app.UpstreamService` and `sourceImport app.SourceImportService` fields (zero values use default adapters; tests inject file-protocol adapters). No in-process lock map: concurrent checks are serialized by the adapter's per-mirror file lock (phase 1), which also covers CLI and MCP processes.
2. Routes (new `routes_sources.go`, registered with `registerRoutes`):

   | Method + path | Service call | Notes |
   |---|---|---|
   | `GET /api/v1/skills/{id}/sources` | `SourceService.SkillSources` (new, see 3) | 404 for unknown skill |
   | `POST /api/v1/skills/{id}/upstream/check` | `CheckSources([sourceID])` then `GetSkillUpstream` | 400 `invalid_request` when not tracked |
   | `POST /api/v1/skills/{id}/upstream/review` | `UpstreamService.PreviewUpdate` | body `{target_commit?, resolutions?[{path, action, content?}], idempotency_key?}`; full preview with diffs |
   | `POST /api/v1/upstream/proposals/{proposal_id}/confirm` | `UpstreamService.ConfirmUpdate` | body `{proposal_digest, base_version}`; all pins required. Not under `/api/v1/skills/`: a `/skills/{id}/upstream/confirm` pattern conflicts with the existing `POST /api/v1/skills/proposals/{proposal_id}/confirm` and makes `http.ServeMux` panic at registration |
   | `POST /api/v1/skills/{id}/sources/attach/preview` | `PreviewAttach` | body `{source_id?, locator?, ref?, path?, cadence?}`; `locator` passes `validateGitHubLocator` |
   | `POST /api/v1/skills/{id}/sources/{source_id}/detach/preview` | `PreviewDetach` | |
   | `GET /api/v1/sources` | `ListSourceGroups(ctx, ws)` | returns `groups`, `candidates`, `sources` |
   | `POST /api/v1/sources/check` | `CheckSources` | body `{source_ids?: [], all?: bool}`; exactly one of them |
   | `POST /api/v1/sources/{id}/unwatch/preview` | `PreviewUnwatch` | |
   | `POST /api/v1/sources/proposals/{proposal_id}/confirm` | `LoadSourceProposal` + `ConfirmSourceProposal` | body `{proposal_digest, base_version}` |
   | `POST /api/v1/sources/{id}/import/preview` | `PreviewSourceImport` | body `{skills?: [], path?}` |
   | `POST /api/v1/sources/import/confirm` | `LoadSourceImportProposal` + `ConfirmSourceImport` | body = `confirmationRequest` |

   Every body is decoded with `decodeJSON` (unknown fields rejected). Service `(result with Error, nil)` responses go through `writeAppError`; returned errors through `writeError`.
3. `SourceService.SkillSources(ctx, path, skillID) (SkillSourcesResult, error)` in new `internal/app/skill_sources.go`: `{Result; SkillID; Upstream *SkillUpstream (nil when the skill has no github/git origin); Learning []LearningReference{SourceID, Locator (repository or URL), Ref, Path, Role, Monitoring, LastCheckedAt, Availability, PendingInsights int}}`. Pending insights = `SELECT count(*) FROM insights WHERE skill_id=? AND status='pending'` grouped by source when the insight records one, else attributed to the skill only (`PendingInsights` on the first reference and a top-level `PendingInsights` total).
4. `SkillListEntry` gains `UpstreamStatus string \`json:"upstream_status,omitempty"\``, filled only by a new `SkillService.ListSkillsWithUpstream(ctx, path, state)` that wraps `ListSkills` and merges `ListSkillUpstream`. Only the web `GET /api/v1/skills` handler calls it; `ListSkills`, MCP `skill_list`, and CLI `skill list` stay unchanged (field omitted). A failure to compute statuses leaves the field empty and never fails the list.

## Related code files

Create: `internal/delivery/web/routes_sources.go`, `internal/delivery/web/routes_sources_test.go`, `internal/app/skill_sources.go`, `internal/app/skill_sources_test.go`, goldens `internal/delivery/web/testdata/golden/sources.json` and `skill-sources.json` (generated).

Modify: `internal/delivery/web/server.go` (fields), `internal/delivery/web/routes_read.go` (`handleSkills` calls `ListSkillsWithUpstream`), `internal/delivery/web/routes_read_test.go` (two golden cases), `internal/app/skill_list.go`.

Do not modify any other file.

## Implementation steps

### Task 8.1 — Read model and list field
- Steps: implement Requirements 3–4 with a test: local skill → `Upstream == nil`, empty `Learning`; skill with a learning link → one reference with role `learning-source`.
- Verify: `go test ./internal/app/ -run 'TestSkillSources|TestSkillList' -count=1` exits 0 and prints `ok`.

### Task 8.2 — Routes
- Steps: implement Requirements 1–2. Tests in `routes_sources_test.go` (authenticated requests as in existing route tests): `POST /api/v1/sources/check` with both or neither field → 400; unknown skill on `/sources` → 404; attach preview with a non-GitHub locator → 400; attach (by `source_id` of a seeded source) preview → confirm through `/api/v1/sources/proposals/{id}/confirm` → 200 and the link file exists; upstream review → confirm (`/api/v1/upstream/proposals/{id}/confirm`) with a file-protocol adapter injected into `srv.upstream` and `srv.sources` (repository and state prepared as in phase 4 tests) → 200 and summary contains `skillhub skill review`; the server constructs without panicking with all routes registered; upstream confirm with a wrong digest → 409 (`statusByCode[app.ErrorStaleProposal]`, `errors.go:25`).
- Verify: `go test ./internal/delivery/web/ -run 'TestSourceRoutes|TestUpstreamRoutes' -count=1` exits 0 and prints `ok`.

### Task 8.3 — Goldens
- Steps: add cases `{name: "sources", path: "/api/v1/sources"}` and `{name: "skill-sources", path: "/api/v1/skills/review-skill/sources"}` to `TestReadEndpointsGolden`; run `go test ./internal/delivery/web/ -run TestReadEndpointsGolden -update -count=1`; inspect: existing goldens unchanged (the fixture skill is local, so `skills.json` gains no field).
- Verify: `go test ./internal/delivery/web/ -count=1 && git status --porcelain internal/delivery/web/testdata/golden/` — tests print `ok`; status lists only the two new golden files.

### Task 8.4 — Gate
- Verify: `make check` exits 0.

## Todo

- [ ] Task 8.1 read model + list field
- [ ] Task 8.2 routes
- [ ] Task 8.3 goldens
- [ ] Task 8.4 `make check`

## Success criteria

Every WebUI action in phase 9 has exactly one endpoint, every mutation is preview/confirm with pins, and goldens pin the read shapes.

## UX acceptance

`GET /api/v1/skills/review-skill/sources` for a local skill returns `{"status":"ok", "skill_id":"review-skill", "upstream":null, "learning":[], ...}` (schema_version and summary per the result envelope).

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Long-running check blocks an HTTP request | M×L | Request context cancels the check; the UI shows a pending state; the per-mirror lock serializes duplicate checks and fails with `ErrMirrorBusy` after 30 s. |
| List endpoint slows down by hashing repository skills | L×M | Same helper as `status`; failures leave `upstream_status` empty. |

## Security considerations

All routes sit behind the existing token and host/origin checks in `security.go`. Attach locators are validated as public GitHub URLs. Manual resolution content is size-limited by the existing 4 MiB request body cap (`internal/delivery/web/security.go:145`, `http.MaxBytesReader`).

## Rollback

Revert the phase commit and delete the two goldens.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
