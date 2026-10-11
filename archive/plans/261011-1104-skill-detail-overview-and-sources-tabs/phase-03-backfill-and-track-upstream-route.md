---
phase: 3
title: "Backfill keeps data, and the track-upstream preview route"
status: completed
priority: P1
effort: "4h"
dependencies: []
---

# Phase 3: Backfill keeps data, and the track-upstream preview route

Two parts, done in order inside this phase: (A) make backfill update only the upstream entry, (B) expose
it to the web as a pinned preview. Part B's minimal-diff test depends on Part A.

## Context

`skillhub source backfill --skill <id>` (`internal/delivery/cli/source.go:260-318`) calls
`SourceService.PreviewBackfill` then `ApplyBackfill` (`internal/app/source_backfill.go:66-72`).

- Kind A, upstream without source id (`processCandidateA`, `:257`): optionally creates
  `sources/catalog/<id>.yaml` (`:293-335`), then for schema >= 3 sets `doc["sources"] = []any{src}` and
  `delete(doc, "provenance")` (`:337-353`). That drops every other `sources:` entry (for example learning
  references), the whole `provenance` block (`created_by`, ...), and fields of the old upstream entry it does
  not recompute (`folder_digest`, `files_digest`, `transformations`, `added_at`). The schema < 3 branch
  (`:354-361`) edits `provenance` in place and keeps other keys.
- Kind B, imported source without origin (`processCandidateB`, `:396`): the same replace and delete at
  `:426-442`.
- Both re-marshal the whole document from `map[string]any` with `yaml.Marshal` (`:363`, `:459`), which sorts
  keys and re-indents, so the diff covers the whole file.

The repo already edits `.meta/skill.yaml` in place with `yaml.Node`: `updateSkillMetaYAML`,
`findMappingValueNode`, `setMappingKey` (`internal/app/upstream_update.go:108-200`, same package). Reuse
those helpers (call them, do not edit that file).

For Part B, the source proposal machinery already pins and stores write sets: `SourceProposal`
(`internal/app/source.go:79-88`), `storeSourceProposal` (`source.go:1297`), and the generic confirm route
`POST /api/v1/sources/proposals/{proposal_id}/confirm` (`routes_sources.go:27,295`) -> `LoadSourceProposal`
-> `ConfirmSourceProposal` (`source.go:641,653`), which verifies pins, calls `confirmAndPublish` for any
command and picks a summary by command (`source.go:684-730`). The diff renderer `renderFullDiff` is in
`internal/skill/lifecycle.go:1011` (one caller, `:478`).

Data flow: browser `POST /api/v1/skills/{id}/upstream/track/preview` -> `SourceService.PreviewTrackUpstream`
-> `PreviewBackfill{SkillID}` -> pinned, stored `SourceProposal` + `full_diff` -> JSON. Then
`POST /api/v1/sources/proposals/{id}/confirm` -> files written through the mutation journal -> the next
`GET .../sources` shows a tracked upstream.

## Intentional contract change (CLI and app)

`skillhub source backfill` and `SourceService.PreviewBackfill`/`ApplyBackfill` no longer replace the
`sources:` list or remove `provenance`. They fill in the upstream entry only and leave everything else as it
was. Record this in the commit body and in `docs/curating-skills.md` (`:347-357`). `internal/delivery/cli/help.go:157`
("Backfill source records and provenance for legacy skills") does not describe the replace behavior; keep it.

## Requirements

### Part A: backfill keeps data

- Rewrite the meta update of both kinds with `yaml.Node`:
  - Find the first `sources[]` mapping with role `upstream`. If found, set `id` and set `kind`, the repository key (keep the existing key name, `repo` or `repository`), `ref`, `path`, `commit`, `synced` only when the entry lacks them or they differ from the backfill value. Leave every other key of that entry (`folder_digest`, `files_digest`, `transformations`, `added_at`, ...) unchanged.
  - If no upstream entry exists (kind A from `provenance.origin`, or kind B), append one upstream entry to `sources:` (create the sequence if absent) and keep all existing entries.
  - Never delete `provenance`. In the schema < 3 branch keep today's in-place edit; only make sure `sources:` is untouched.
  - Encode with the file's own indentation (detect it from the original bytes; default 2) so unchanged lines stay byte-identical where `yaml.v3` allows. Do not change `updateSkillMetaYAML`.
- Keep the source-record creation and matching logic as it is.

### Part B: track-upstream preview route

- New file `internal/app/source_backfill_proposal.go`:

  ```go
  type UpstreamTrackProposal struct {
      SourceProposal
      FullDiff  string                 `json:"full_diff,omitempty"`
      Candidate *BackfillCandidateItem `json:"candidate,omitempty"`
  }
  func (service SourceService) PreviewTrackUpstream(ctx context.Context, path, skillID string) (UpstreamTrackProposal, error)
  ```
  - Calls `service.PreviewBackfill(ctx, path, BackfillInput{SkillID: skillID})`; passes its error result through.
  - No candidate: `StatusOK`, summary "Nothing to track: the skill has no untracked upstream.", no pins.
  - One candidate: `Source` = the existing record or `{ID: candidate.SourceID}`, `Diff` from the backfill diff, `planned`, `expiresAt` = now + 24h, `Confirmation` shaped like `PreviewAttach` (`source_links.go:211-224`), `FullDiff` = `skill.RenderFullDiff(changes, current bytes of each path)`; store with `storeSourceProposal`.
- `internal/skill/lifecycle.go`: rename `renderFullDiff` to `RenderFullDiff` and update its caller.
- `internal/app/source.go`: add `case "source_backfill"` to the summary switch of `ConfirmSourceProposal` ("Now tracking upstream <source> for <skill>."). Nothing else there; the telemetry event stays as it is (decided).
- New `internal/delivery/web/routes_upstream_track.go`: `POST /api/v1/skills/{id}/upstream/track/preview`, registered through `registerRoutes` in its own `init`, error handling as in `handleSkillSourceDetachPreview` (`routes_sources.go:185-198`), unknown skill -> 404.
- `docs/contracts/web-api.md`: add the route row; update the `GET /api/v1/skills/{id}/sources` row to say it includes an untracked upstream origin (Phase 2's behavior).

## Files

- Modify: `internal/app/source_backfill.go`, `internal/app/source_backfill_test.go`, `internal/delivery/cli/source_test.go`, `internal/app/source.go` (summary case only), `internal/skill/lifecycle.go` (export only), `docs/contracts/web-api.md`, `docs/curating-skills.md` (backfill section only)
- Create: `internal/app/source_backfill_proposal.go`, `internal/app/source_backfill_proposal_test.go`, `internal/delivery/web/routes_upstream_track.go`, `internal/delivery/web/routes_upstream_track_test.go`
- Read only: `internal/app/upstream_update.go` (helpers), `routes_sources.go`, `skill_sources.go`, `internal/delivery/cli/source.go`

## Steps

1. Part A tests first, in `source_backfill_test.go` (keep `TestBackfill` green):
   - Kind A with an id-less upstream entry plus a `learning` entry, `provenance.created_by`, and `folder_digest` on the upstream entry: after apply the upstream entry has the new `id` and still has `folder_digest`; the other entry and `provenance` are unchanged; the planned change for `.meta/skill.yaml` differs from the original only in the upstream entry (assert on the rendered diff lines).
   - Kind A with only the id-less upstream entry (the `herdr-cook-plan` shape, 4-space indent): the id is filled and the diff touches only that entry.
   - Kind A from `provenance.origin` only: an upstream entry is appended and `provenance` is kept.
   - Kind B: `provenance` (`created_by: source_import`, `source_id`) is kept and other entries survive.
2. Implement Part A.
3. CLI test in `internal/delivery/cli/source_test.go` (in-process `runCLIForTest`): `source backfill --skill <id> --yes` on a skill with extra sources and provenance keeps them and fills the upstream id; a skill with only the id-less upstream gets its id.
4. Export `RenderFullDiff`.
5. Part B app tests: preview for an untracked skill with no matching record adds `sources/catalog/<id>.yaml`, modifies `.meta/skill.yaml`, has pins, writes nothing yet, and its `full_diff` is minimal (only the upstream entry lines change); with a matching record no catalog file is added; tracked or local skill has no pins; unknown skill is an error result; confirming through `ConfirmSourceProposal` writes the files and `GetSkillUpstream` is then no longer untracked.
6. Implement Part B; add the summary case.
7. Route tests with `newTestServer` and `postJSON` (`fixtures_test.go:203`, `routes_skill_write_test.go:54`): preview 200 with `full_diff`, `candidate.source_id`, `confirmation.confirmation.pins` (the names Phase 1 put in `types.ts`); confirm via `/api/v1/sources/proposals/{id}/confirm` -> 200; `GET /api/v1/skills/{id}/sources` shows a tracked upstream; unknown skill -> 404; replayed confirm -> stale error.
8. Update `web-api.md` and the backfill section of `curating-skills.md` (what it fills in, that other entries and provenance stay).

## Validation

- `go test ./internal/skill ./internal/app ./internal/delivery/cli ./internal/delivery/web -count=1`
- `make check` (no binary build).

## Risks

- `yaml.v3` cannot keep every formatting detail (for example comment spacing or flow style). Mitigation: detect indentation; tests use the real `herdr-cook-plan` shape; the preview shows the diff before anything is written.
- Workspaces that relied on backfill wiping stale entries now keep them. Intended; documented as a contract change.
- `PlanMutation` returns `ErrRecoveryRequired` when a journal is pending; surface it like the other preview routes.

## Rollback

Revert the commit. Part B without Part A is not supported, so revert them together; the front end's Track
upstream call then gets a 404 and the command hint remains.
