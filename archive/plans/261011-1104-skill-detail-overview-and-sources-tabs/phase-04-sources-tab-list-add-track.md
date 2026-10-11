---
phase: 4
title: "Sources tab: list, add, track"
status: completed
priority: P1
effort: "3.5h"
dependencies: [1]
---

# Phase 4: Sources tab: list, add, track

## Context

`SourcesTab.tsx` renders `UpstreamSection` (null when there is no upstream) and `LearningSection`, which holds
the only add form (GitHub URL only, auto-confirms without a preview, `LearningSection.tsx:40-63`). After
Phase 2, `upstream` can be `status: "untracked"`; `POST .../upstream/check` rejects that state, and the only
way to track it today is the CLI hint at `UpstreamSection.tsx:118-125`.

API calls available (from Phase 1 and earlier, all in `web/src/api/queries.ts`):

- `previewAttachSource(id, { source_id } | { locator, ref, path })` (`:243`) -> `SourceProposal`; `status: ok` without pins when already linked (`internal/app/source_links.go:164-169`); locators must be GitHub (`routes_sources.go:158-163`). It always writes the role `learning-source`.
- `previewTrackUpstream(id)` (Phase 1) -> `UpstreamTrackProposal` with `full_diff`, `diff`, `candidate`, pins; no pins when there is nothing to track.
- `confirmSourceProposal(proposalId, pins)` (`:274`) confirms both kinds.
- `useSources()` (`:203`) lists watched sources (`SourceListItem.record.id`, `.skills`).

Components: `ProposalPreview` (`web/src/components/ProposalPreview.tsx:20`, supports `diff`, `paths`, `warning`, `isLoading`, `networkError`), `ActionBar`, `EmptyState`, `StatusBadge`, `ConfirmDialog`, `CommandBlock`, `Toast`.

Use `/ak:frontend-design` for layout, the add form and the empty state, and `/ak:frontend-development` for the React implementation.

## Requirements

- Top of the tab: `ActionBar` with a status summary (for example "Upstream: owner/repo · 2 learning references" or "No sources yet") and a primary **Add source** button that opens the add form below it.
- One `EmptyState` when there is no upstream and no learning reference; it explains the two kinds of source (wording consistent with `docs/user-guide.md:30`) and offers Add source.
- `UpstreamSection`:
  - Uses `upstreamStatusBadge` from `detail-format.ts`; delete the private `getStatusProps`.
  - Tracked: unchanged behavior.
  - Untracked (`!isUpstreamTracked`): Check now and Review update are disabled; a primary **Track upstream** button calls `previewTrackUpstream`, then opens `ProposalPreview` with title `Track upstream for <skill>?`, `diff` = `full_diff`, `paths` = `diff.added` + `diff.modified`, an impact line saying whether a new source record is created (`candidate.create_source`), confirm label "Track upstream". Confirm calls `confirmSourceProposal`, then invalidates `['skill-sources', id]`, `['skill-review', id]`, `['skill', id]`, `['skills']` and `['sources']`, and shows a toast. A response without pins shows its summary as an info banner. Errors use `err.render.WHY || err.render.ERROR || err.message`.
  - The `skillhub source backfill --skill <id>` `CommandBlock` stays below the button as a fallback, introduced as "Or run this in a terminal:".
- `LearningSection` keeps the list and Unlink (unchanged confirm flow) and loses its inline add form and add state; its intro points to Add source.
- New `AddSourceForm.tsx` (adds a learning reference only):
  - **Link a watched source**: select over `useSources()` items not already linked to this skill and not the skill's upstream `source_id`; disabled with a one-line reason when none.
  - **Watch a GitHub repository**: URL (required), optional ref and folder path.
  - Preview calls `previewAttachSource` ("Checking repository…" while waiting on the URL choice). With pins: `ProposalPreview` (title `Add source <id>?`, paths = `diff.added` + `diff.modified`, warning = first `warnings[].summary`, confirm label "Add source"); confirm calls `confirmSourceProposal`, invalidates `['skill-sources', id]` and `['sources']`, closes the form, shows a toast. Without pins: info banner with the summary. Errors as above. Cancel sends nothing.
- Nothing is written without the user pressing confirm in a preview.

## Files

- Modify: `web/src/screens/skill-detail/SourcesTab.tsx`, `UpstreamSection.tsx`, `LearningSection.tsx`, `sources-tab.test.tsx`
- Create: `web/src/screens/skill-detail/AddSourceForm.tsx`, `web/src/screens/skill-detail/add-source-form.test.tsx`
- Read only: `detail-format.ts`, `web/src/api/*`, `web/src/components/*`, `UpstreamReview.tsx`, `ProvenanceCard.tsx`

## Steps

1. Load `/ak:frontend-design`; sketch: ActionBar, optional form card, upstream card (with Track upstream when untracked), learning card, or the single empty state. Then implement with `/ak:frontend-development`.
2. Build `AddSourceForm`.
3. Rework `SourcesTab` around the ActionBar and empty state; keep loading and error/Retry states.
4. Update `UpstreamSection` (shared badge, untracked buttons, Track upstream flow, fallback hint).
5. Strip the add form from `LearningSection`.
6. Tests:
   - `sources-tab.test.tsx`: new empty state for a local skill; untracked upstream shows "Not tracked", Check now and Review update disabled, Track upstream opens the preview with the full diff, confirm sends the pins and invalidates the five query keys, no-pins response shows the summary, the fallback command is visible; keep the update-available, `UpstreamReview` and unlink tests; move the add half of the `LearningSection` test to the new file.
   - `add-source-form.test.tsx`: link a watched source (`{ source_id }`, preview paths, confirm with pins, both keys invalidated); watch a URL (`locator`, `ref`, `path`); already linked shows the summary and no preview; API error shows WHY; already-linked sources and the upstream source are not offered.

## Validation

- `cd web && npm run typecheck && npm run lint`
- `cd web && npx vitest run src/screens/skill-detail/sources-tab.test.tsx src/screens/skill-detail/add-source-form.test.tsx`
- Visual check is done in Phase 6 on the rebuilt binary.

## Risks

- A confirm without a visible preview would bypass the approval policy; tests cover both flows.
- The smoke e2e asserts the old learning empty-state text (`web/e2e/smoke.spec.ts:74-76`); Phase 6 updates it, so put the final empty-state text in the phase report.

## Rollback

Revert the commit. Phase 1 helpers and types stay unused but harmless.
