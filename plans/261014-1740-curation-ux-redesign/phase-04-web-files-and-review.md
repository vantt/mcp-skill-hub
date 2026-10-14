# Phase 4: Web file tree, commit and review

## Goal

On the web, a person edits a whole skill (edit, add, rename, delete, upload) as pending changes, reviews them, and
commits them as one hub commit. A "Waiting for review" page collects what needs a human. A full-skill review screen
lets them read every file and approve third-party content on the web. A "Checkout to a project" button gives the
command for the try-in-an-agent path.

## Decisions implemented

- **D6**:
  - a file tree on the left with edit, add, rename, delete and upload;
  - a bottom bar "N changes not committed · agents still get the current version" with [Discard] [Review →];
  - Review shows each file (A/M/D, line counts, collapsible diff), routing lint and an editable commit message;
  - [Commit to the hub] makes one commit (D3) and then says who gets it ("this machine now; others after you push");
  - a "Checkout to a project" button with the command to copy;
  - the separate Editor and Resources tabs and "save one file, live at once" are removed.
- **D9**:
  - the "Waiting for review" page lists new drafts and skills whose content changed (submissions come in phase 5);
  - the full-skill review screen shows every file, the diff against the served version, the license and the origin;
  - Approve works on the web and is enabled only after every changed file was opened;
  - every commit says who gets the new version.

## Prerequisites

Phase 1 (commit outcome, `Reach`) and phase 2 (`buildSkillTreeChanges`, the routing lint on a proposed tree).

## Context

- Web runs on its own listener (default 7421), binds `0.0.0.0` unless `--dev`, and uses a per-start session bearer
  token (`internal/delivery/cli/serve.go:117-131, 235-270`, `internal/delivery/web/security.go:81-120`). Mutations need
  a matching `Origin` and JSON (`security.go:122-145`). Bodies are capped at 4 MiB (`:67-79`). `decodeJSON` rejects
  unknown fields (`routes_skill_write.go:40-51`).
- Single-file resource edit: `POST /api/v1/skills/{id}/resources/preview` (`routes_skill_resources.go:18-22, 67-88`),
  confirmed through `POST /api/v1/skills/proposals/{proposal_id}/confirm` (`routes_skill_write.go:241-277`).
- SPA: `web/src/screens/skill-detail/SkillDetailScreen.tsx` tabs via `?tab=` (`:21-28, 92, 126, 326-330`):
  Overview, Editor, Resources, Review, Runtime, Sources, Distill. Components: `DiffView.tsx`, `ProposalPreview.tsx`,
  `ConflictDrawer.tsx`. Drafts are in `localStorage` (`web/src/state/drafts.ts`, `local-store.ts`). Routes are in
  `web/src/routes.tsx`. The API client is `web/src/api/client.ts` and `queries.ts`.
- Approval today is CLI only. `docs/web-ui.md:6, 56-61` says so, and `ContentTrustCard.test.tsx:8-31` asserts no
  Approve button. Both change in this phase.
- The approval mutation is `skill edit --approve-content <digest>` (`app/skill_review.go:391-411`, CLI parse at
  `cli/skill.go:363`), through `PreviewSkillUpdate`.

## Design

### API (all under `/api/v1`, documented in `docs/contracts/web-api.md`)

| Method and path | Body / result | Notes |
|-----------------|---------------|-------|
| `GET /skills/{id}/files` | tree of content files: `path, size, digest, text, editable` | `SKILL.md` first; hub meta hidden |
| `POST /skills/{id}/changes/preview` | `{base_version, changes:[{action: add\|modify\|delete\|rename, path, from, content, encoding: utf8\|base64, expected_digest}]}` → proposal with `files[]` (A/M/D/R, added, removed), per-file `diff`, `routing_lint`, `default_message`, pins | Builds one proposal through `buildSkillTreeChanges`. Rename = delete + add in the proposal |
| `POST /skills/proposals/{proposal_id}/confirm` | existing body plus optional `commit_message` | Additive. The result carries `commit` with `reach` |
| `GET /review-queue` | `items:[{kind: draft\|content_changed, skill_id, title, since, origin}]` | `submission` is added in phase 5 |
| `GET /skills/{id}/review-content` | every content file with text (or a binary marker), `changed` per file, the diff against the served version (or the last approved digest), license, origin, `content_digest` | For drafts, every file counts as changed |
| `POST /skills/{id}/approve/preview` | `{content_digest}` → proposal | Same app path as `--approve-content` |

- The single-file `POST /skills/{id}/resources/preview` and `GET /skills/{id}/resources*` are removed once the SPA uses
  the new routes. This is listed as a contract change. The read route stays if anything else uses it (check the web
  e2e tests before removing).
- Limits: the request stays at 4 MiB, editable text at 1 MiB per file, and uploads at 2 MiB per file in base64 (open
  question 19). The existing path and symlink rules of the resource editor apply to every change.
- No new error codes: reuse `stale_proposal`, `edit_conflict` and `resource_limits_exceeded`.

### SPA

- New `FilesTab.tsx` with `FileTree.tsx`, `PendingChangesBar.tsx` and `ReviewChangesScreen.tsx`. Pending changes are a
  changeset in `localStorage`, keyed by skill id and `base_version` (open question 18). Reuse `DiffView` and
  `ConflictDrawer`. A stale `base_version` on preview opens the conflict drawer with "the hub changed".
- Remove `EditorTab.tsx` (its `SKILL.md` editing) and `ResourcesTab.tsx` and `ResourceFileView.tsx` (moved into
  Files). The routing and metadata form becomes a "Routing" tab, and each save there is one reviewed commit (open
  question 18).
- After commit: a toast in words ("review-checklist committed") and a card with the commit hash and the reach line.
- `CheckoutButton.tsx` opens a dialog with `skillhub skill checkout <id>`, a copy button, and one sentence: "Run it in
  your project, try the skill with your agent, then `skillhub skill commit <id>`."
- New route `/review` → `WaitingForReviewScreen.tsx` (a nav item with a count badge). A new route
  `/skills/:id/review-content` → `FullReviewScreen.tsx`: every file in order, changed files expanded with their
  diffs, license and origin at the top, a per-file "opened" check, and [Approve] disabled until every changed file
  was opened. Approve runs preview, then confirm, then shows the reach line.
- `ContentTrustCard.tsx` links to the full review screen instead of the copy-a-command block. Its test changes from
  "no Approve button" to "Approve lives on the review screen".

## Files

- Go: create `internal/delivery/web/routes_skill_files.go` and `routes_review.go` with tests. Modify `server.go` (route
  table), `routes_skill_write.go` (`commit_message`), and `routes_skill_resources.go` (remove the preview route). Add
  app helpers `internal/app/review_queue.go` and `skill_review_content.go` with tests.
- SPA:
  - create `web/src/screens/skill-detail/FilesTab.tsx`, `FileTree.tsx`, `PendingChangesBar.tsx`,
    `ReviewChangesScreen.tsx`, `CheckoutButton.tsx`, `RoutingTab.tsx`;
  - create `web/src/screens/review/WaitingForReviewScreen.tsx` and `FullReviewScreen.tsx`;
  - modify `SkillDetailScreen.tsx`, `ContentTrustCard.tsx` and its test, `routes.tsx`, `api/queries.ts`, and
    `state/drafts.ts`;
  - delete `EditorTab.tsx`, `ResourcesTab.tsx` and `ResourceFileView.tsx` after their tests move.
- E2E: replace `web/e2e/skill-resources*` with `skill-files.spec.ts`, and add `review-queue.spec.ts`. Extend
  `web/e2e/ux/guards.spec.ts` (no raw ids, a "git dirty" fixture state).
- Contracts: `docs/contracts/web-api.md`.

## Steps (tests first)

1. Go route tests on a temp hub:
   - a changeset with modify, add (binary base64), rename and delete gives one proposal;
   - confirm with `commit_message` makes one commit with that message and exactly those paths;
   - a stale `base_version` returns `stale_proposal`;
   - an oversized upload returns `resource_limits_exceeded`;
   - review-queue lists a draft and a third-party skill with stale approval;
   - review-content diffs against the served version;
   - approve preview and confirm make the skill servable, and the commit has the approval.
2. Implement the routes and app helpers.
3. Vitest:
   - the tree renders nested paths with `SKILL.md` first;
   - the pending bar counts changes and Discard clears them;
   - Approve stays disabled until every changed file is marked opened;
   - the checkout dialog copies the command.
4. Build the SPA screens and remove the old tabs.
5. Playwright on a fixture hub with an isolated HOME:
   - edit `SKILL.md`, add `references/ocr.md`, rename one file, delete one, then Review, then Commit; assert the
     reach line, and that `git log -1` (via an API read) shows one commit;
   - a third-party skill goes Waiting → full review → open each file → Approve;
   - a 390 px layout has no horizontal scroll.
6. `make web-check`, `make web-e2e`, and `make web-ux` (screenshots for review).

## Mock (web)

```text
┌ pdf-tools · Files ───────────────────────────────────────────────────────────┐
│ SKILL.md                │  references/tables.md                       [Edit]  │
│ ▾ references            │  …                                                  │
│    forms.md          M  │                                                     │
│    ocr.md            A  │                                                     │
│    tables.md            │                                                     │
│ ▾ scripts               │                                                     │
│    extract.sh           │                                                     │
│ [+ File] [+ Folder] [Upload]        [Checkout to a project]                   │
├──────────────────────────────────────────────────────────────────────────────┤
│ 2 changes not committed · agents still get the current version  [Discard] [Review →] │
└──────────────────────────────────────────────────────────────────────────────┘

Review changes
  M references/forms.md   +4 −1   ▸ diff
  A references/ocr.md     +22     ▸ diff
  Routing lint: no findings.
  Commit message: [Commit pdf-tools: 2 files                ]
                                                  [Commit to the hub]
→ Committed 4be19c0. This machine's agents get it now; others after you push (git -C ~/skill-hub push).

Waiting for review (3)
  pdf             content changed upstream · needs approval     2026-10-14
  team-release    new draft                                     2026-10-14
  deploy-notes    new draft                                     2026-10-13

pdf · review content           License: Proprietary · Origin: github.com/anthropics/skills@main
  ✓ SKILL.md            M  +12 −3   (opened)
  ○ scripts/fill.py     M  +2 −2
  … 9 unchanged files (expand)
  [Approve]  (open every changed file first: 1 left)
```

## Validation

```bash
go test ./internal/app -run 'ReviewQueue|ReviewContent' -count=1
go test ./internal/delivery/web -count=1
make web-check
make web-e2e
make check
```

## Risks

- **Approve on a web UI that binds `0.0.0.0`.** It sits behind the session token and the origin check. D9 accepts the
  browser-automation risk for now. It is recorded in plan.md and `docs/web-ui.md`.
- **Pending changes live in one browser.** The bar says they are not committed, and Discard is explicit.
- **Removing tabs breaks deep links (`?tab=editor`, `?tab=resources`).** Map them to `?tab=files` and `?tab=routing`.
- **Large e2e churn.** Move the existing resource-editor assertions into the new spec rather than deleting coverage.

## Rollback

Revert the phase commits. The old tabs and single-file routes come back. The commits already made stay in the hub.

## Effort

4 days.
