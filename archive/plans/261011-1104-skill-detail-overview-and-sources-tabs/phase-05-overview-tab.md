---
phase: 5
title: "Overview tab"
status: completed
priority: P1
effort: "2h"
dependencies: [1]
---

# Phase 5: Overview tab

## Context

Phase 1 creates `OverviewTab.tsx` with its props (`skill`, `review`, `sources`, `sourcesLoading`,
`sourcesError`, `onOpenTab`) and registers it as the first, default tab. The user chose structured facts from
existing API fields, not the raw `.meta/skill.yaml`, and no new endpoint. All data is already fetched by
`SkillDetailScreen`:

- `SkillDetail` (`web/src/api/types.ts:75`): `name`, `description`, `lifecycle_state`, `routing` (`operations`, `triggers`, `not_for`, `min_scope`), `resources`.
- `SkillReviewResult` (`types.ts:92`): `collection`, `resource_status` (count, bytes), `provenance`.
- `SkillSourcesResult` (`types.ts:510`): `upstream` (may be `untracked` after Phase 2), `learning[]`.

Reuse `ProvenanceCard` as is (`onViewSources` -> `onOpenTab('sources')`), `StatusBadge`, `Skeleton`, and
`upstreamStatusBadge`, `resourceSummaryText` from `detail-format.ts`. Lifecycle text uses `useT()` with
`lifecycle_short.*` like the header (`SkillDetailScreen.tsx:256`).

Use `/ak:frontend-design` for the layout and `/ak:frontend-development` for the React implementation.

## Requirements

- Grid of cards in the style of `ReviewTab` (`auto-fit, minmax(min(100%, 320px), 1fr)`), readable at 390 px with no sideways scroll.
- **About** card: name, full description, collection, lifecycle state, files (`resourceSummaryText`). An **Edit** link calls `onOpenTab('editor')`.
- **Routing** card: min scope, and lists for operations, triggers and not-for; an empty list reads "None".
- **Provenance**: `ProvenanceCard`.
- **Sources** card: the upstream row (repository@ref, path, status badge, "Not tracked" for untracked) and one row per learning reference (source id, locator, last checked); skeleton while `sourcesLoading`; short message on `sourcesError`; "No sources yet" when empty. A **Manage sources** button calls `onOpenTab('sources')`. Read-only: add, unlink and Track upstream live on the Sources tab.
- No `ActionBar` on this tab: it has no primary action, and the header holds the lifecycle actions.
- Labels as module constants like the other tabs. No fields beyond the list above.

## Files

- Modify: `web/src/screens/skill-detail/OverviewTab.tsx`
- Create: `web/src/screens/skill-detail/overview-tab.test.tsx`
- Read only: `detail-format.ts`, `ProvenanceCard.tsx`, `web/src/api/*`, `web/src/components/*`, `web/src/i18n/*`

## Steps

1. Load `/ak:frontend-design`; lay out the four cards. Implement with `/ak:frontend-development` from props only (no fetching inside the component).
2. Tests (`MemoryRouter`; `loadGolden('skill-detail')`, `loadGolden('skill-review-third-party')`, inline `SkillSourcesResult` fixtures):
   - About facts and routing lists render; empty lists read "None";
   - provenance from the third-party golden shows its locator;
   - sources: tracked upstream with badge, untracked shows "Not tracked", learning rows, empty state, loading skeleton, error message;
   - Manage sources and Edit call `onOpenTab` with `'sources'` and `'editor'`.

## Validation

- `cd web && npm run typecheck && npm run lint`
- `cd web && npx vitest run src/screens/skill-detail/overview-tab.test.tsx src/screens/skill-detail/skill-detail-tabs.test.tsx`

## Risks

- Duplicating Review-tab diagnosis (validity, readiness) would blur the two tabs; Overview shows facts only.
- Long descriptions or paths overflow on mobile: use the existing `app-wrap` class and `overflowWrap: 'anywhere'`.

## Rollback

Revert the commit; the Phase 1 shell (description card) remains and compiles.
