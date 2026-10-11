---
phase: 1
title: "Shared web contracts"
status: completed
priority: P2
effort: "1.5h"
dependencies: []
---

# Phase 1: Shared web contracts

## Context

Phases 4 and 5 both need the API types and client calls for the new routes, the Resources tab needs new
props from `SkillDetailScreen.tsx` (`<ResourcesTab skill={skill} />` at `:437`), and the conflict drawer is
hard-wired to SKILL.md (`web/src/components/ConflictDrawer.tsx:4,16,96,100`). This phase fixes those shared
pieces so Phases 4 and 5 never edit the same file and never edit `web/src/api/*` or `web/src/components/*`.

Use `/ak:frontend-design` for any visual decision and `/ak:frontend-development` for the React work. If the
Skill tool is unavailable, follow the existing `fg-*` patterns and note it in the report; the lead reviews
the UI visually after Phase 6.

## Requirements

- `web/src/api/types.ts`:

  ```ts
  export type ResourceUnavailableReason = '' | 'binary' | 'too_large' | 'missing' | 'not_regular';
  export interface SkillResourceItem {
    path: string;          // workspace-relative, same as SkillResource.path
    kind: string;
    size_bytes: number;
    digest: string;        // working-tree digest, "sha256:..."; "" when missing
    editable: boolean;
    reason: ResourceUnavailableReason;
  }
  export interface SkillResourceList { skill_id: string; items: SkillResourceItem[]; }
  export interface SkillResourceContent {
    skill_id: string;
    path: string;
    size_bytes: number;
    digest: string;
    editable: boolean;
    reason: ResourceUnavailableReason;
    line_ending: 'lf' | 'crlf' | 'mixed' | 'none';
    truncated: boolean;
    content?: string;      // text files only
  }
  ```
- `web/src/api/queries.ts`:
  - `useSkillResources(id)` -> `GET /skills/{id}/resources`, key `['skill-resources', id]`.
  - `useSkillResourceContent(id, path)` -> `GET /skills/{id}/resources/content?path=<encodeURIComponent(path)>`,
    key `['skill-resource', id, path]`, `enabled` only when both are set.
  - `previewSkillResourceUpdate(id, body: { path: string; content: string; expected_digest: string })`
    -> `POST /skills/{id}/resources/preview`, returns `SkillProposal`.
  - Confirm reuses `confirmSkillMutation` (`queries.ts:106`); no new confirm call.
- `ConflictDrawer.tsx`: one optional prop `fileLabel?: string` (default `'SKILL.md'`) used in the title
  ("<fileLabel> changed since you opened it"), the compare label ("Compare the full <fileLabel>") and the
  download name (`draft-<basename>` for a non-default label, `draft.md` otherwise, plain-text MIME for non-`.md`).
  With no prop the output is byte-for-byte what it is today, so `EditorTab` is unaffected.
- `SkillDetailScreen.tsx`:
  - `setTab` (`:126-131`) and `openEditor` (`:134-140`) also delete the `file` and `edit` query params.
  - Render `<ResourcesTab skill={skill} workspaceId={workspaceId} thirdParty={review?.content_trust?.third_party ?? false} />`.
- `ResourcesTab.tsx`: widen the props interface only (accept and ignore `workspaceId`, `thirdParty`); no
  behavior change. Phase 4 takes the file over.
- New `ResourceFileView.tsx` shell with its final props; it renders the file path, a "Back to resources"
  button that calls `onClose`, and nothing else. Phase 5 takes the file over.

  ```ts
  export interface ResourceFileViewProps {
    skill: SkillDetail;
    workspaceId: string;
    path: string;          // workspace-relative path from the list
    startInEdit: boolean;  // true when opened with Edit
    thirdParty: boolean;
    onClose: () => void;
  }
  ```
- URL contract used by Phases 4 and 5: `?tab=resources&file=<path>` opens the file view; `&edit=1` opens it in
  edit mode. Every existing `?tab=` value keeps working.

## Files

- Modify: `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/components/ConflictDrawer.tsx`,
  `web/src/components/conflict-drawer.test.tsx`, `web/src/screens/skill-detail/SkillDetailScreen.tsx`,
  `web/src/screens/skill-detail/skill-detail-tabs.test.tsx`, `web/src/screens/skill-detail/ResourcesTab.tsx` (props only)
- Create: `web/src/screens/skill-detail/ResourceFileView.tsx` (shell)
- Do not modify: `EditorTab.tsx`, `editor-tab.test.tsx`.

## Steps

1. Add the types and the three client functions.
2. Add `fileLabel` to `ConflictDrawer`; add one test that a custom label appears in the title and compare
   label, and keep every existing assertion passing unchanged.
3. Create the `ResourceFileView` shell; widen `ResourcesTab` props; wire `SkillDetailScreen`.
4. Extend `skill-detail-tabs.test.tsx`: switching from `?tab=resources&file=x&edit=1` to another tab drops
   `file` and `edit`.

## Validation

- `cd web && npm run typecheck && npm run lint`
- `cd web && npx vitest run src/components/conflict-drawer.test.tsx src/screens/skill-detail/skill-detail-tabs.test.tsx src/screens/skill-detail/editor-tab.test.tsx`

## Risks

- The TS shapes must match the Go JSON of Phase 3; Phase 3 asserts these exact field names.
- A changed default in `ConflictDrawer` would alter the SKILL.md editor; the unchanged existing tests guard it.

## Rollback

Revert the commit after reverting Phases 4 and 5.
