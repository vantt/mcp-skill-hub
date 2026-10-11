---
phase: 3
title: "App service and web routes"
status: completed
priority: P2
effort: "2.5h"
dependencies: [2]
---

# Phase 3: App service and web routes

## Context

The SKILL.md edit path is: `handleSkillUpdatePreview` (`internal/delivery/web/routes_skill_write.go:178-206`)
-> `SkillService.PreviewSkillUpdate` (`internal/app/skill_lifecycle.go:106-130`), which maps
`*skill.EditConflictError` to `NewEditConflictError` (`:112-117`), stores the proposal with
`skill.StoreProposal` (`:124`) and returns `makeSkillProposal` (`:397-415`). Confirm is the generic
`handleSkillConfirm` (`routes_skill_write.go:240-277`) -> `LoadSkillProposal` (requires
`ProposalKindLifecycle`, `skill_lifecycle.go:186-207`) -> `ConfirmSkillMutation` (`:210-246`). Phase 2's proposal
uses `Manager.plan`, so it is a lifecycle-kind `skill_edit` proposal and the existing confirm route accepts it
with no change.

Routes register themselves per file through `registerRoutes` in `init()` (`routes_read.go:15-17`,
`routes_usage.go:13`). Errors go through `writeError` / `writePreviewError` (`routes_skill_write.go:28-38`,
`internal/delivery/web/errors.go:51-69`). The JSON contract is in `plan.md` ("Shared contract") and in the
types of Phase 1; field names must match exactly.

## Requirements

New `internal/app/skill_resource.go` on `SkillService`:

- `type SkillResourceItem struct { Path, Kind string; SizeBytes int64; Digest string; Editable bool; Reason string }`
  with JSON tags `path, kind, size_bytes, digest, editable, reason`.
- `type SkillResourceList struct { SkillID string; Items []SkillResourceItem }` (`skill_id`, `items`; empty
  slice, never null).
- `type SkillResourceContent struct` with `skill_id, path, size_bytes, digest, editable, reason, line_ending,
  truncated, content,omitempty` (content as a string; omitted for binary).
- `ListSkillResources(ctx, workspace, id)`, `ReadSkillResource(ctx, workspace, id, path)`,
  `PreviewSkillResourceUpdate(ctx, workspace, id, path, content, expectedDigest, idempotencyKey string) (SkillProposal, error)`:
  resolve the workspace with `skill.ResolveWorkspace`, call Phase 2, map errors:
  `*skill.EditConflictError` -> `NewEditConflictError` with a WHY naming the file (not SKILL.md) and a FIX
  "Reload the file, then apply your edit again."; validation errors -> `NewInvalidRequestError(reason, fix)`;
  `skill.ErrNotFound` passes through. Store the proposal with `skill.StoreProposal` and return
  `makeSkillProposal(proposal, "Resource edit is ready for review.")`.

New `internal/delivery/web/routes_skill_resources.go`:

- `GET /api/v1/skills/{id}/resources` -> `ListSkillResources`.
- `GET /api/v1/skills/{id}/resources/content` with query `path` (required; missing -> 400) -> `ReadSkillResource`.
- `POST /api/v1/skills/{id}/resources/preview` with body
  `{ "path", "content", "expected_digest", "idempotency_key"? }` decoded with `decodeJSON`
  (`DisallowUnknownFields`) -> `PreviewSkillResourceUpdate` -> `writePreviewError(w, err, errors.Is(err, skill.ErrNotFound))`.
- No new confirm route.

`docs/contracts/web-api.md`: add the three rows next to `update/preview` (`:74`), stating that confirm uses the
existing skill proposal route and that only text files up to 1 MiB inside the skill folder are editable.

## Files

- Create: `internal/app/skill_resource.go`, `internal/app/skill_resource_test.go`,
  `internal/delivery/web/routes_skill_resources.go`, `internal/delivery/web/routes_skill_resources_test.go`,
  `internal/delivery/web/testdata/golden/skill-resources.json`, `internal/delivery/web/testdata/golden/skill-resource-content.json`
- Modify: `docs/contracts/web-api.md`
- Read only: `internal/app/skill_lifecycle.go`, `routes_skill_write.go`, `internal/delivery/web/errors.go`,
  `internal/delivery/web/fixtures_test.go`

## Steps

1. Implement the app service with tests (conflict mapping, not found, validation, proposal stored and
   loadable by `LoadSkillProposal`).
2. Implement the routes. Route tests use the existing fixtures (`fixtures_test.go`) and cover:
   - list excludes `SKILL.md` and `.meta/*`, has `editable`/`reason` per row; unknown skill -> 404;
   - content for a text file, a binary file (no `content` key), a too-large file (`truncated: true`);
   - traversal and symlink paths -> 400 or 404, never file bytes;
   - preview -> 200 with `full_diff` and pins, then `POST /api/v1/skills/proposals/{id}/confirm` -> applied,
     and the file on disk has the new bytes;
   - stale `expected_digest` -> 409 `edit_conflict`; unknown body field -> 400; missing Origin or JSON
     content type on preview -> 403/415 (existing middleware, one assertion);
   - write the two goldens and assert their top-level field names equal the Phase 1 TS interfaces.
3. Update the contract doc.

## Validation

- `go test ./internal/app/ -run 'SkillResource' -count=1`
- `go test ./internal/delivery/web/ -count=1`
- `go vet ./internal/app/ ./internal/delivery/web/`

## Risks

- `writePreviewError` turns an unclassified invalid request into a WHY from `err.Error()`
  (`routes_skill_write.go:31-36`); make sure validation messages never include file content.
- Golden drift between Go and TS; the route test checks field names explicitly.

## Rollback

Revert the commit; the UI then shows an error banner on the Resources tab actions and nothing is written.
