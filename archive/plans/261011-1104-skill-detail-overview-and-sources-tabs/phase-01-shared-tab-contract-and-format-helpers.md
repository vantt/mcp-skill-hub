---
phase: 1
title: "Shared frontend contracts"
status: completed
priority: P1
effort: "1.75h"
dependencies: []
---

# Phase 1: Shared frontend contracts

## Context

Phases 4 and 5 both need the upstream status labels that live privately in `UpstreamSection.tsx:54-77`
(`getStatusProps`), and Phase 5 needs the file-count text `ReviewTab.tsx:52-54` computes inline. The tab row
and panels are hand-written in `SkillDetailScreen.tsx:321-427`; the default tab is `'review'` (`:90`).
`useSkillSources(skillId)` is already called there (`:98`). Phase 4 also needs a client call and type for the
new track-upstream preview route that Phase 3 adds. This phase fixes all of these shared contracts so Phases 4
and 5 never edit the same file and never edit `web/src/api/*`.

Use `/ak:frontend-design` for the tab placement and `/ak:frontend-development` for the React work.

## Requirements

- `web/src/screens/skill-detail/detail-format.ts`:
  - `type Tone = 'success' | 'warning' | 'danger' | 'neutral'`
  - `upstreamStatusBadge(status: string): { tone: Tone; label: string }`, moved verbatim from `getStatusProps` (`untracked` -> "Not tracked").
  - `isUpstreamTracked(upstream: SkillUpstream | null | undefined): boolean`: present, `status !== 'untracked'` and `source_id !== ''`.
  - `resourceSummaryText(count: number, totalBytes: number): string`: the exact text `ReviewTab` shows today.
- `ReviewTab.tsx` uses `resourceSummaryText` (behavior unchanged).
- API contract for the route of Phase 3, in `web/src/api/types.ts`:

  ```ts
  export interface BackfillCandidate {
    skill_id: string;
    kind: string;          // "origin_without_source" | "source_without_origin"
    source_id: string;
    repo_path: string;
    create_source: boolean;
  }
  export interface UpstreamTrackProposal extends SourceProposal {
    full_diff?: string;
    candidate?: BackfillCandidate;
  }
  ```
  and in `web/src/api/queries.ts`:
  `previewTrackUpstream(id: string): Promise<UpstreamTrackProposal>` -> `POST /skills/{id}/upstream/track/preview`.
  Confirm reuses the existing `confirmSourceProposal` (`queries.ts:274`).
- New `OverviewTab.tsx` with its final props, rendering only a real description card so the tab compiles and shows true data; Phase 5 fills the rest:

  ```ts
  export interface OverviewTabProps {
    skill: SkillDetail;
    review?: SkillReviewResult;
    sources?: SkillSourcesResult;
    sourcesLoading: boolean;
    sourcesError: unknown;
    onOpenTab: (tab: 'sources' | 'editor') => void;
  }
  ```
- `SkillDetailScreen.tsx`: `LABEL_TAB_OVERVIEW = 'Overview'`, Overview tab button first, default `activeTab` `'overview'`, panel `<OverviewTab ... onOpenTab={setTab} />` using `data`/`isLoading`/`error` of the existing `useSkillSources` call. No second query. `hasSourcesUpdate` (`:99-101`) unchanged.
- Every existing `?tab=` value keeps opening its tab.

## Files

- Create: `web/src/screens/skill-detail/detail-format.ts`, `detail-format.test.ts`, `OverviewTab.tsx`, `skill-detail-tabs.test.tsx`
- Modify: `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/screens/skill-detail/SkillDetailScreen.tsx`, `web/src/screens/skill-detail/ReviewTab.tsx`
- Do not modify: `UpstreamSection.tsx` (Phase 4 swaps its private helper for the shared one).

## Steps

1. Write `detail-format.ts` with unit tests for every status label, `isUpstreamTracked` (null, untracked, tracked, empty `source_id`) and singular/plural file text.
2. Switch `ReviewTab` to `resourceSummaryText`.
3. Add the `UpstreamTrackProposal` types and `previewTrackUpstream`.
4. Create the `OverviewTab` shell; register the tab first and default in `SkillDetailScreen`.
5. `skill-detail-tabs.test.tsx`: seed a `QueryClient` with `['skill', id]`, `['skill-review', id]`, `['skill-sources', id]` (goldens via `loadGolden` from `web/src/test/golden.ts`), render under `MemoryRouter` with a `/skills/:id` route; assert Overview is first and selected with no `?tab`, and `?tab=review` selects Review.

## Validation

- `cd web && npm run typecheck && npm run lint`
- `cd web && npx vitest run src/screens/skill-detail/detail-format.test.ts src/screens/skill-detail/skill-detail-tabs.test.tsx src/screens/skill-detail/sources-tab.test.tsx`
- No `make web-build` here.

## Risks

- The track-upstream type must match the Go JSON of Phase 3 (`full_diff`, `candidate.*`). Phase 3 asserts these names in its route test.
- Changing the default tab affects e2e specs; Phase 6 owns those edits.

## Rollback

Revert the commit. Revert Phases 4 and 5 first if they have landed.
