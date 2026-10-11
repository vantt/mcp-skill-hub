---
phase: 4
title: "Resources tab list with Edit and View"
status: completed
priority: P2
effort: "1.5h"
dependencies: [1]
---

# Phase 4: Resources tab list with Edit and View

## Context

`ResourcesTab.tsx` renders a table of `skill.resources` (catalog rows, including SKILL.md, `:46-96`) with a
hard-coded "Active" chip and the notice "Resource content is not viewable in v1" (`:9-10`). The empty state
filters out SKILL.md (`:19-20`). After Phase 1 it receives `workspaceId` and `thirdParty`, and these are
available from `web/src/api/queries.ts`: `useSkillResources(id)` (editability per path, working tree).
`ResourceFileView` (shell from Phase 1, filled by Phase 5) takes
`{ skill, workspaceId, path, startInEdit, thirdParty, onClose }`. URL contract: `?tab=resources&file=<path>`
opens a file, `&edit=1` opens it in edit mode.

Use `/ak:frontend-design` for the table action column and the view switch, and `/ak:frontend-development`
for the React work. If the Skill tool is unavailable, follow the existing `fg-*` table, button and caveat
classes and say so in the report; the lead reviews the UI visually after Phase 6.

## Requirements

- When `file` is set in the URL, render only `<ResourceFileView ... path={file} startInEdit={edit === '1'} onClose={...} />`;
  `onClose` removes `file` and `edit` from the URL (react-router `useSearchParams`). Otherwise render the list.
- List: keep the existing table and empty state; drop the SKILL.md row from the table (the Editor tab owns
  it) and add a last **Actions** column:
  - item `editable` -> primary-small **Edit** button (`aria-label="Edit <path>"`) that sets `file` and `edit=1`;
  - otherwise -> **View** button (`aria-label="View <path>"`) that sets `file` only, plus a one-word reason
    under the size (Binary, Too large to edit, Missing on disk, Not a regular file);
  - while `useSkillResources` loads, buttons are disabled; on error, one danger banner with
    `err.render.WHY || err.render.ERROR || err.message` and the table still shows.
  - A catalog row missing from the editability list (catalog older than the files) shows View only.
- Replace the "not viewable in v1" sentence with: changed resources still point to `skillhub rebuild`, and
  edits go through a preview you confirm; nothing is saved without it.
- The "Active" chip stays as is (out of scope).

## Files

- Modify: `web/src/screens/skill-detail/ResourcesTab.tsx`
- Create: `web/src/screens/skill-detail/resources-tab.test.tsx`
- Read only: `web/src/api/*`, `web/src/components/*`, `ResourceFileView.tsx`

## Steps

1. Load the two skills (or follow the patterns), sketch the action column on 1280 and 390 px.
2. Implement the URL switch and the actions.
3. Tests (`MemoryRouter`, `QueryClient` seeded with `['skill-resources', id]` inline fixtures typed by
   `SkillResourceList`): Edit shown only for editable rows; View plus reason for binary and too-large rows;
   SKILL.md not in the table; clicking Edit sets `file` and `edit=1` and renders the file view (assert the
   shell's Back button); Back removes both params; list error shows the banner.

## Validation

- `cd web && npm run typecheck && npm run lint`
- `cd web && npx vitest run src/screens/skill-detail/resources-tab.test.tsx`

## Risks

- `a11y.spec.ts:66` visits `?tab=resources`; new buttons need accessible names (covered above). Phase 6 runs it.

## Rollback

Revert the commit; Phase 1's props stay harmless.
