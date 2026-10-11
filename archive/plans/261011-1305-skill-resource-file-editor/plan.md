---
title: "Skill Resources tab: view and edit companion files"
description: "Let a person open every companion file of a skill from the Resources tab, read it, and edit text files through the same preview, diff and confirm flow as SKILL.md, with no new storage."
status: completed
priority: P2
effort: 13.5h
branch: main
tags: [web, skill-detail, resources, editor, security]
blockedBy: []
blocks: []
created: 2026-10-11
---

# Skill Resources tab: view and edit companion files

## Why this is missing today (evidence)

- The Resources tab only lists catalog rows and says "Resource content is not viewable in v1"
  (`web/src/screens/skill-detail/ResourcesTab.tsx:9-10`). Its rows come from `SkillDetail.resources`, which is
  the catalog manifest (`internal/app/skill_detail.go:117`, `internal/skill/resource.go:69`), already without hub
  metadata under `.meta/` (`internal/catalog/project.go:111`).
- The only write path for skill files is `POST /api/v1/skills/{id}/update/preview`
  (`internal/delivery/web/routes_skill_write.go:21,178-206`) -> `SkillService.PreviewSkillUpdate`
  (`internal/app/skill_lifecycle.go:106`) -> `Manager.PreviewUpdate` (`internal/skill/lifecycle.go:257`). It can
  change only `.meta/skill.yaml` and `SKILL.md`, and it always rewrites the metadata file (`:296-300`).
- Everything needed for a safe file edit already exists: `mutation.Change{Path, BeforeDigest, Contents}`
  (`internal/mutation/mutation.go:33`), `Manager.plan` (stores a pinned proposal, renders the diff through
  `RenderFullDiff`, `internal/skill/lifecycle.go:430-492`), `Manager.Confirm` (applies and republishes the
  catalog, `:494-520`), the generic confirm route (`routes_skill_write.go:23,240-277`), and the browser pieces
  `ProposalPreview`, `ConflictDrawer`, `ActionBar`, `Markdown` and `state/drafts.ts`.

So the feature is: a domain function that plans a one-file `skill_edit` write set, two read routes and one
preview route, and a file view/editor in the Resources tab. Confirm reuses the existing route. No new storage:
the file in the skill folder is the record and Git keeps history.

## Outcome and acceptance criteria

- [x] Each companion file row in the Resources tab has an **Edit** button when the file is editable, and a
      **View** button otherwise. SKILL.md is not listed here (it stays in the Editor tab).
- [x] Edit opens a simple editor for that file: the shared `ActionBar` at the top (status, **Preview changes**,
      **Back to resources**), a monospace text area, and for `.md` files a Markdown preview whose frontmatter
      shows as the existing YAML block (`web/src/components/Markdown.tsx:41-71`).
- [x] Preview changes shows `ProposalPreview` with the exact unified diff of that one file; nothing is written
      until the person confirms; confirm uses `POST /api/v1/skills/proposals/{proposal_id}/confirm`.
- [x] Editable means: a regular file inside the skill folder, listed in the skill's catalog manifest, not
      `SKILL.md`, not under `.meta/`, not a symlink, valid UTF-8 without NUL bytes, at most 1 MiB.
- [x] View shows a read-only text view for text files (truncated at 1 MiB with a notice) and a short
      explanation with size and digest for binary files; it never shows raw bytes of a binary file.
- [x] A concurrent change (file changed on disk after the person opened it) opens the existing conflict
      drawer with "keep my edits on the latest version" and "discard my edits"; a stale proposal is refused.
- [x] A draft of an unsaved edit survives a reload (browser storage, same as the SKILL.md editor).
- [x] Line endings, the executable bit and the rest of the skill folder are preserved; the saved bytes are
      exactly the previewed bytes.
- [x] `EditorTab` behavior is unchanged (`editor-tab.test.tsx` and the lifecycle e2e still pass untouched).
- [x] `make check`, `make web-check` and `make web-e2e` pass; a new e2e spec views a binary file, edits a
      script and a reference document, and resolves a conflict, offline.

Non-goals: creating, renaming or deleting files; editing `SKILL.md` or `.meta/*` from this tab; syntax
validation of YAML/JSON/scripts; MCP or CLI surfaces for resource editing; regenerating screenshots.

## Security threat model (decided before design)

What the feature exposes: file bytes of a skill folder (already in Git, already served to agents) to the
local Web UI, which requires the session bearer token on every `/api/` request
(`internal/delivery/web/security.go:150-188`), checks Host (`:99`), and requires a same-origin JSON request for
every write (`:191-214`, body capped at 4 MiB `:136-147`). What it can change: one file inside one skill folder.

| Threat | Mitigation (layer) |
|---|---|
| Path traversal (`../`, absolute, backslash, NUL) | Domain resolves the skill folder with the strict `loadSkill` (`internal/skill/lifecycle.go:773`), requires the requested path to start with that folder plus `/`, rejects `.`/`..`/empty segments, and requires the path to be a row of the skill's catalog manifest (allow-list). `mutation.canonicalPath` re-checks on write (`internal/mutation/mutation.go:353-366`). |
| Symlinks (final file or a parent directory) | `os.OpenRoot` + `Lstat` of every segment from the skill folder down; any symlink or non-regular file is not editable and not readable. The catalog already refuses symlinks (`internal/catalog/input.go:160-163`); the mutation layer re-checks parents and target on write (`internal/mutation/filesystem.go:50,97,309-321`). |
| Files outside the skill folder, or another skill's folder | Prefix check against this skill's folder plus the manifest allow-list for this `skill_id`. |
| `SKILL.md` and hub metadata (`.meta/skill.yaml` holds the runtime spec, provenance and the trust approval digest) | Refused by the domain (`workspace.IsHubMeta`, `internal/workspace/workspace.go:566`; entrypoint path equality). The runtime spec and `content_reviewed_digest` therefore cannot be edited through this route. |
| Binary and huge files | Text means valid UTF-8 and no NUL byte. Edit cap 1 MiB (below the 4 MiB per-file canonical limit `internal/workspace/workspace.go:24` and the 4 MiB request cap). Larger text is view-only and truncated; binary is metadata-only. New content is re-validated on preview (UTF-8, no NUL, cap). Searchable extensions must stay UTF-8 or the catalog build fails (`internal/catalog/project.go:116-118`), which the text rule already guarantees. |
| Secrets in files | No new exposure: the files are canonical Git content already readable by any agent the skill is served to. Content is returned as JSON (`nosniff`, `security.go:127`) and rendered as text, never as HTML or a download of raw bytes. Not logged. |
| XSS through file content | Text area and `<pre>` render text; `.md` preview goes through the existing `Markdown` component with its safe-link handling (`Markdown.tsx:28-38`). |
| Encoding and line endings | The browser text area turns CRLF into LF. The read response reports `line_ending`; on preview, a file whose newlines are all CRLF gets its LF newlines converted back to CRLF, so an edit does not rewrite every line. Mixed files are saved as typed (the diff shows it). A UTF-8 BOM round-trips as U+FEFF. |
| Concurrent edits and stale digests | Read returns the working-tree digest; preview pins it as `BeforeDigest` and returns `edit_conflict` when the file changed (`Manager.plan`, `internal/skill/lifecycle.go:446-468`); confirm re-verifies pins and returns `stale_proposal` (`internal/app/skill_lifecycle.go:210-221`). |
| Executable scripts losing their mode | The mutation layer keeps the existing file mode on replace (`internal/mutation/filesystem.go:101-140`); a domain test asserts it. |
| Trust bypass for third-party skills | Any file change changes the content digest (`internal/skillruntime/trust.go:49-55`, all files except hub metadata), so an approved third-party skill becomes "review stale" and agents get no content until the person re-approves in the terminal (`skill_review.go:388-413`). Approval stays CLI-only. The preview warns about this before confirm. |

## Lifecycle, review and trust impact (investigated)

- A resource edit is a `skill_edit` operation, exactly like a SKILL.md edit today: the lifecycle state is
  unchanged (`skillStateAfter`, `internal/app/skill_lifecycle.go:361-373`), `quality.reviewed` is untouched, the
  catalog generation is rebuilt on confirm (`internal/skill/lifecycle.go:503-509`) so an active skill serves the
  new file at once, and Git is left dirty for the person to commit (`gitMutationSummary`).
- Reusing the `skill_edit` command keeps the confirm route, result summary (`appliedSkillSummary` `:375-389`),
  CLI next-command hints (`internal/delivery/cli/next_commands.go:146`) and journal kind valid without edits.
  A new command name would be misread as a lifecycle target by `skillStateAfter` (`:365`).
- Third-party skills: the content trust approval goes stale (see table). Local skills: always trusted.
- Upstream-tracked skills: the local side becomes `modified` (or `diverged`), through the working-tree digest
  (`internal/app/upstream.go:163-175`), the same as after a SKILL.md edit.
- Default chosen: keep SKILL.md parity (no forced return to draft, no extra re-review). This is recorded as an
  open product question below.

## Phases

| # | Phase | Status | Effort | Depends on |
|---|---|---|---|---|
| 1 | [Shared web contracts](./phase-01-shared-web-contracts.md) | Done | 1.5h | none |
| 2 | [Domain: read, classify and plan a resource edit](./phase-02-domain-resource-edit.md) | Done | 2.5h | none |
| 3 | [App service and web routes](./phase-03-app-service-and-routes.md) | Done | 2.5h | 2 |
| 4 | [Resources tab list with Edit and View](./phase-04-resources-tab-list.md) | Done | 1.5h | 1 |
| 5 | [Resource file view and editor](./phase-05-resource-file-view-and-editor.md) | Done | 3.5h | 1 |
| 6 | [Integration: e2e, docs, build, visual review](./phase-06-integration-e2e-docs.md) | Done | 2h | 3, 4, 5 |

## Dependency graph

```text
t=0:   Phase 1 (web contracts)            Phase 2 (domain)
          |            |                        |
          v            v                        v
       Phase 4      Phase 5                  Phase 3 (app + routes)
          |            |                        |
          +------------+-----------+------------+
                                   v
                                Phase 6
```

Wave 1: Phases 1 and 2. Wave 2: Phases 3, 4 and 5 in parallel. Wave 3: Phase 6.
Critical path: 1 -> 5 -> 6 (7h) and 2 -> 3 -> 6 (7h) are balanced.

## Parallel execution

| Phase | Owner files (exclusive) | Depends on | Can run with |
|---|---|---|---|
| 1 | `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/components/ConflictDrawer.tsx`, `web/src/components/conflict-drawer.test.tsx`, `web/src/screens/skill-detail/SkillDetailScreen.tsx`, `web/src/screens/skill-detail/skill-detail-tabs.test.tsx`, `web/src/screens/skill-detail/ResourcesTab.tsx` (props only), new `web/src/screens/skill-detail/ResourceFileView.tsx` (shell) | none | 2 |
| 2 | new `internal/skill/resource_edit.go`, new `internal/skill/resource_edit_test.go` | none | 1, 4, 5 |
| 3 | new `internal/app/skill_resource.go`, new `internal/app/skill_resource_test.go`, new `internal/delivery/web/routes_skill_resources.go`, new `internal/delivery/web/routes_skill_resources_test.go`, new goldens `internal/delivery/web/testdata/golden/skill-resources.json` and `skill-resource-content.json`, `docs/contracts/web-api.md` | 2 | 4, 5 |
| 4 | `web/src/screens/skill-detail/ResourcesTab.tsx` (takes over from 1), new `web/src/screens/skill-detail/resources-tab.test.tsx` | 1 | 2, 3, 5 |
| 5 | `web/src/screens/skill-detail/ResourceFileView.tsx` (takes over from 1), new `web/src/screens/skill-detail/resource-file-view.test.tsx` | 1 | 2, 3, 4 |
| 6 | new `web/e2e/skill-resources.spec.ts`, `web/e2e/a11y.spec.ts`, `web/e2e/ux/flows.ts`, `docs/user-guide.md` | 3, 4, 5 | none |

Read-only for every phase (import or call, never edit): `internal/skill/lifecycle.go` (another agent is
reworking `RenderFullDiff`; Phase 2 only calls `Manager.plan`, `loadSkill` and `normalizedDigest` from a new file
in the same package), `internal/mutation/*`, `internal/workspace/*`, `internal/app/skill_lifecycle.go`,
`routes_skill_write.go`, `EditorTab.tsx`, `web/src/components/*` except `ConflictDrawer.tsx` in Phase 1,
`web/src/state/drafts.ts`, `web/src/api/proposal-view.ts`, `web/src/api/client.ts`.

## Shared contract (fixed by Phase 1, implemented by Phases 2 and 3)

```text
GET  /api/v1/skills/{id}/resources
  -> { skill_id, items: [{ path, kind, size_bytes, digest, editable, reason }] }
GET  /api/v1/skills/{id}/resources/content?path=<workspace-relative path>
  -> { skill_id, path, size_bytes, digest, editable, reason, line_ending, truncated, content? }
POST /api/v1/skills/{id}/resources/preview
  body { path, content, expected_digest, idempotency_key? } -> SkillProposal (full_diff set)
POST /api/v1/skills/proposals/{proposal_id}/confirm   (existing, unchanged)
```

`path` is the workspace-relative path already used by `SkillResource.path` (for example
`skills/core/foo/scripts/run.sh`). `reason` is one of `""`, `binary`, `too_large`, `missing`, `not_regular`.
`line_ending` is one of `lf`, `crlf`, `mixed`, `none`. `content` is present only for text files.
`items` excludes `SKILL.md` and hub metadata. Errors: `404 not_found` (skill or path not a resource of the
skill), `400 invalid_request` (path outside the folder, not editable, new content binary or too large, no
change), `409 edit_conflict` (digest moved).

## Rules for every phase

- Frontend phases (1, 4, 5): load `/ak:frontend-design` for layout and visual decisions and
  `/ak:frontend-development` for the React implementation. Earlier implementing subagents could not invoke
  the Skill tool; if that happens, follow the existing `fg-*` classes and the patterns of `EditorTab.tsx`,
  `SourcesTab.tsx` and `ResourcesTab.tsx`, and say so in the phase report. The lead runs a visual review of
  the rebuilt UI after Phase 6.
- The `ActionBar` (`web/src/components/ActionBar.tsx`) sits at the top of the editor view; no sticky bottom bar.
- Do not edit `EditorTab.tsx`. No shared hook is extracted from it: its flow is bound to a multi-field form and
  field merge (`edit-fields.ts`), while the resource editor is one text value. Both reuse the same shared
  components and helpers instead (`ProposalPreview`, `ConflictDrawer`, `ActionBar`, `Markdown`, `Toast`,
  `state/drafts.ts`, `api/proposal-view.ts`).
- No plan IDs, phase numbers or audit labels in code comments, test names or commit messages.
- Phases 1 to 5 verify with Go tests, `go vet`, typecheck, lint and vitest only; no binary builds. Phase 6 owns
  `make web-build`, `go build ./cmd/skillhub` and `web/.e2e/skillhub`, after checking with the lead.
- Never write to `/home/vantt/skill-hub`; tests use temp workspaces. Do not touch `~/.local/bin`.

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| A path check gap lets the route read or write outside the skill folder | Low x High | Three independent layers (manifest allow-list, per-segment `Lstat` under `os.Root`, mutation `canonicalPath` and parent checks); Phase 2 and 3 tests cover `../`, absolute, backslash, NUL, symlinked file, symlinked parent directory, another skill's file, `SKILL.md`, `.meta/skill.yaml` |
| Saving a third-party skill's resource silently withholds it from agents | Medium x Medium | Preview shows a trust warning when `review.content_trust.third_party`; docs explain re-approval; behavior identical to SKILL.md edits |
| CRLF files rewritten as LF in every line | Medium x Medium | `line_ending` round-trip rule, tested in Phase 2 |
| `RenderFullDiff` changes under another agent while Phase 2 runs | Medium x Low | Phase 2 never edits `lifecycle.go`; its diff tests assert only that the changed line appears with `-`/`+` and the file header, not exact hunk layout |
| Frontend codes against JSON before Phase 3 lands | Medium x Low | Phase 1 fixes the TS shapes; Phase 3 route tests assert the same field names and write goldens |
| Large text file makes the browser slow | Low x Low | 1 MiB cap for edit and view |
| Catalog stale: a file added on disk is not listed until rebuild | Low x Low | Same as today's list; the existing notice already points to `skillhub rebuild` |

## Rollback

Each phase is one commit. Phase 4 or 5 reverts alone (Phase 1's shell keeps compiling; without 5 the Edit
button opens a placeholder). Phase 1 reverts only together with 4 and 5. Phase 3 reverts alone: the UI then
shows an error banner from a 404 on the new routes, and nothing is written. Phase 2 reverts only with 3.
No data migration exists; a confirmed edit is an ordinary `skill_edit` operation that Git can revert.

## Unresolved questions (for the user)

1. Should editing a resource of an **active** skill move it back to draft or require re-review? The plan keeps
   SKILL.md parity (stays active, served at once). Third-party skills already need re-approval through the
   content trust digest.
2. Should creating, renaming and deleting companion files come next? They are out of scope here.
3. Is 1 MiB the right edit and view cap (the hub allows 4 MiB per file)?
