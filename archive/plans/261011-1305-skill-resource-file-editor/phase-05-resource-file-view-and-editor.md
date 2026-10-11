---
phase: 5
title: "Resource file view and editor"
status: completed
priority: P2
effort: "3.5h"
dependencies: [1]
---

# Phase 5: Resource file view and editor

## Context

The SKILL.md editor (`web/src/screens/skill-detail/EditorTab.tsx`) is the pattern to mirror, not to edit:

- draft restore and autosave through `state/drafts.ts` (`:69`, `:116-130`), keyed by workspace, kind, id and
  base digest (`web/src/state/drafts.ts:15-54`);
- `ActionBar` at the top with status text and **Preview changes** (`:258-274`);
- error text `err.render.WHY || err.render.ERROR || err.message` (`:133-139`);
- preview -> `ProposalPreview` with `proposalPaths`, `proposalPatch`, `proposalWarning` from
  `web/src/api/proposal-view.ts` and pins from `proposal.confirmation.confirmation.pins` (`:253`, `:505-523`);
- `edit_conflict` -> fetch latest -> `ConflictDrawer` with keep-mine / discard (`:172-193`, `:229-251`, `:524-538`);
- confirm -> `confirmSkillMutation`, clear draft, toast, invalidate queries, refresh (`:195-222`);
- wide layout shows text area and Markdown preview side by side, narrow layout an Edit/Preview switch (`:296-380`).

API (Phase 1): `useSkillResourceContent(id, path)` -> `SkillResourceContent`
(`content`, `digest`, `editable`, `reason`, `line_ending`, `truncated`); `previewSkillResourceUpdate(id, {path, content, expected_digest})`
-> `SkillProposal`; confirm with `confirmSkillMutation`. `ConflictDrawer` accepts `fileLabel` (Phase 1).

Use `/ak:frontend-design` for the view and editor layout and `/ak:frontend-development` for the React work.
If the Skill tool is unavailable, follow `EditorTab.tsx` and the `fg-*` classes and say so in the report; the
lead reviews the UI visually after Phase 6.

## Requirements

- Props as fixed by Phase 1. `kind` for drafts: `'resource'`; draft id: `${skill.skill_id}:${path}`; base digest:
  the content response `digest`.
- Header: file path in mono (card title), size and kind; `ActionBar` at the top of the view (never a sticky
  bottom bar) holding **Back to resources** (calls `onClose`) and the mode actions below.
- Read-only mode (default for View, and for every non-editable file):
  - text: `<pre>` mono block with the content; for `.md` files a Read/Source switch where Read uses `Markdown`
    (frontmatter shows as its YAML block);
  - `truncated`: caveat "Showing the first 1 MiB; this file is too large to edit here.";
  - `binary`: caveat "Binary file: its content is not shown. Size N, digest sha256:…"; `missing` and
    `not_regular` explain the state and point to `skillhub rebuild` or restoring the file;
  - editable files show an **Edit** button in the `ActionBar`.
- Edit mode (Edit button, or `startInEdit` and the file is editable; otherwise fall back to read-only with the
  reason shown):
  - text area (mono, `aria-label="<file name> content"`), restored from a draft when present;
    `.md` files get the side-by-side / switch Markdown preview like the SKILL.md editor, others only the text area;
  - status text: no changes / "Draft saved in this browser. Preview shows what will change. Nothing is saved
    until you confirm." (same wording as the SKILL.md editor);
  - **Preview changes** (disabled while unchanged or loading) -> `ProposalPreview` titled
    `Save changes to <file name>?`, `target` = skill id, `toState` = lifecycle state, `paths`, `diff`, a
    `changes` line "Edited <path>", confirm label "Save changes"; when `thirdParty` is true, `warning` says
    "This skill came from another repository. After you save, agents get no content from it until you approve
    it again in your terminal (see the Content trust card).";
  - **Cancel edit** returns to read-only and keeps the draft;
  - confirm -> `confirmSkillMutation`, clear the draft, toast "Saved <file name>.", invalidate
    `['skill', id]`, `['skill-review', id]`, `['skill-resources', id]`, `['skill-resource', id, path]`,
    `['skills']`, `['home']`, then show the saved text in read-only mode;
  - `ApiError` code `edit_conflict` -> refetch the content (`staleTime: 0`) and open `ConflictDrawer` with
    `fileLabel` = file name, `draftContent` = text area value, `latestContent` = latest content,
    digests, `mine` = ["Edited <file name> here"], `theirs` = ["<file name> changed on disk"],
    `bothFields` = [file name]; keep-mine re-bases the draft on the latest digest and requests a new preview;
    discard loads the latest text and clears the draft;
  - a confirm result with `status: error` (stale proposal) shows its WHY in the banner and keeps the draft.
- Nothing is written without the confirm click; Escape or Cancel in the preview sends nothing.

## Files

- Modify: `web/src/screens/skill-detail/ResourceFileView.tsx` (takes over the Phase 1 shell)
- Create: `web/src/screens/skill-detail/resource-file-view.test.tsx`
- Read only: `EditorTab.tsx`, `web/src/api/*`, `web/src/components/*`, `web/src/state/drafts.ts`

## Steps

1. Load the two skills (or follow `EditorTab.tsx`); sketch read-only text, binary, truncated and edit states at
   1280 and 390 px.
2. Implement read-only mode, then edit mode, then preview/confirm, then conflict.
3. Tests with `vi.mock('../../api/queries')` and `vi.mock('../../api/client')` (same approach as
   `editor-tab.test.tsx:10-20`):
   - text file read-only, Markdown file Read view shows the YAML frontmatter block;
   - binary shows the caveat and no content, no Edit button; truncated shows the notice, no Edit button;
   - `startInEdit` on a non-editable file falls back to read-only;
   - edit -> Preview sends `{path, content, expected_digest}` -> preview dialog shows the diff -> confirm calls
     the confirm route with pins, clears the draft and invalidates the listed keys;
   - third-party warning appears only when `thirdParty`;
   - draft restored after remount; Cancel edit keeps it;
   - `edit_conflict` opens the drawer titled "<file name> changed since you opened it"; keep-mine requests a
     preview with the new digest; discard shows the latest text;
   - stale confirm result shows the WHY and keeps the draft.

## Validation

- `cd web && npm run typecheck && npm run lint`
- `cd web && npx vitest run src/screens/skill-detail/resource-file-view.test.tsx src/screens/skill-detail/editor-tab.test.tsx`

## Risks

- Copying too much of `EditorTab` would duplicate logic; keep only the single-text flow and import every shared
  piece. Do not edit `EditorTab.tsx`.
- A confirm without a visible preview would bypass the approval policy; tests cover the flow.

## Rollback

Revert the commit; the Phase 1 shell returns (path and Back button only).
