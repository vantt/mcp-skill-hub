---
title: "Phase 9: Web UI"
status: in-progress
---

# Phase 9: Web UI

<!-- Updated: Validation Session 1 - one Sources tab (Upstream + Learning) with an update dot; commit date shown; WebUI is one of the two places updates are applied -->
<!-- Updated: 2026-10-05 - provenance type corrected to the flat Go JSON; Runtime query invalidated after an upstream update; distill and run routes now owned by phase 10 -->

## Context

- Plan: [plan.md](./plan.md). Depends on phase 8 and on the runtime plan's WebUI parity phase (it edits `SkillDetailScreen.tsx`, `ReviewTab.tsx`-adjacent trust UI, `api/types.ts`, `api/queries.ts`). Re-read those files before editing; keep their changes intact.
- Ownership assumption: the runtime plan's phase 12a owns `ContentTrustCard.tsx`, `RuntimeTab.tsx`, the `content_trust` types, and the `Runtime` tab; this phase never edits those files (the runtime plan is complete, so the one exception is the `SkillProvenance` type correction in Requirement 1) and adds its `Sources` tab next to `Runtime`. This phase shows trust impact only from its own upstream responses (`trust_impact`).
- Read first: `web/package.json` (scripts), `web/src/routes.tsx`, `web/src/api/{client,queries,types}.ts`, `web/src/screens/skill-detail/{SkillDetailScreen,ReviewTab}.tsx`, `web/src/screens/skills/{SkillsScreen.tsx,skills.test.tsx}`, `web/src/screens/home/{action-cta.ts,home.test.tsx}`, `web/src/components/{DiffView,ProposalPreview,ConfirmDialog,CommandBlock,EmptyState,Skeleton,StatusBadge,Toast,Banner}.tsx`, `web/src/i18n/en.ts`, `web/e2e/smoke.spec.ts`, `docs/use-cases/05-webui-design-brief.md` §4.6, `docs/design/webui-mockup/README.md`.

## Overview

Make sources visible where users think about them: on the skill. Skill Detail gets a **Sources** tab with an Upstream section (status, check, review and apply update) and a Learning references section; the Review tab's hard-coded provenance becomes real data; the Skills list gets an upstream badge and filter; `/sources` becomes a grouped secondary view; Home links the new actions.

## Requirements

1. **Types and queries** (`types.ts`, `queries.ts`): `SkillUpstream`, `UpstreamListResult`, `UpstreamFile`, `UpstreamUpdatePreview`, `UpstreamUpdateResult`, `SkillSourcesResult`, `LearningReference`, `SourceRepositoryGroup`, `SourceSummary`, `SourceListResult`, `SourceProposal`, `SourceMutationResult`, `SourceImportProposal`, `SourceCheckResult`; `SkillListItem.upstream_status?`. Correct the existing `SkillProvenance` type (`web/src/api/types.ts`, added by the runtime plan) to the JSON that Go actually returns: `app.SkillProvenance` in `internal/app/skill_review.go` is flat — `created_by`, `created_at`, `source_id`, `source_locator`, `source_revision`, `upstream_path`, all optional. The current TypeScript fields `source_url`, `source_path`, and the nested `origin` object do not exist in that JSON; remove them after checking with `grep -rn 'source_url\|source_path\|provenance' web/src` that nothing else reads them (update any reader to the flat fields). Do not change the Go struct. Keep `SkillReviewResult.provenance?`. Hooks/functions: `useSkillSources(id)`, `useSources()`, `checkSkillUpstream(id)`, `reviewSkillUpdate(id, body)`, `confirmSkillUpdate(proposalId, pins)` (`POST /api/v1/upstream/proposals/{proposal_id}/confirm`), `checkSources(body)` (body `{source_ids}` | `{all: true}` | `{due: true}`), `previewAttachSource(id, body)`, `previewDetachSource(id, sourceId)`, `previewUnwatchSource(id)`, `confirmSourceProposal(proposalId, pins)`, `previewSourceImport(id, body)`, `confirmSourceImport(pins)`. Query keys: `['skill-sources', id]`, `['sources']`.
2. **Skill Detail → Sources tab** (`?tab=sources`, label `Sources`, an indicator dot on the tab label when `upstream.status` is `update_available` or `diverged`, with an accessible label `Sources, update available`; one tab holds both the Upstream and the Learning sections):
   - New `SourcesTab.tsx` composing `UpstreamSection.tsx` and `LearningSection.tsx`.
   - `UpstreamSection`: hidden when `upstream == null`. Shows status badge (tone: success `up_to_date`; warning `update_available`, `modified`, `unknown`; danger `diverged`, `upstream_removed`, `unavailable`; neutral `pinned`, `untracked`), facts (repository@ref, path, current commit, latest commit and its commit date (`latest_committed_at`), files changed in the skill folder, last checked), changed-files list, and the next action. Buttons: `Check now` (spinner, disabled while running), `Review update` (enabled for `update_available`/`diverged`). `untracked` shows `CommandBlock` with `skillhub source backfill --skill <id>`; `upstream_removed` explains the local copy keeps working and offers no update.
   - `UpstreamReview.tsx` (opened by Review update): calls `reviewSkillUpdate`; renders a file table (`FILE`, `CHANGE`, `ACTION` select with `Take upstream`, `Keep mine`, `Auto-merged` only when `conflicts == 0`, `Edit manually`); a `DiffView` for the selected file with a toggle between upstream, local, and result diffs; a textarea prefilled with `merged_with_markers` for `Edit manually`; `Apply choices` re-calls review with the resolutions; `unresolved` non-empty → apply button disabled and a banner `N file(s) need a decision`; pins present → `Confirm update` (via `ProposalPreview` or an equivalent confirm footer) with a trust banner when `trust_impact.review_required_after_apply`: `After applying, agents cannot use <id> until you approve the new content.` plus `CommandBlock` `skillhub skill review <id>`. Success → toast `Updated · <operation_id>`, a link `See what changed since approval →` to `?tab=review` (where the runtime plan's `ContentTrustCard` shows `changes_since_approval`), and invalidate `['skill', id]`, `['skill-review', id]`, `['skill-runtime', id]`, `['skill-sources', id]`, `['skills']`, `['home']` (the update changes the content digest, so the Runtime tab and the trust card must refetch rather than show the pre-update state). A 409 shows `The skill or upstream changed since this review. Review again.` with a `Review again` button.
   - `LearningSection`: list of references (source, locator, role, last checked, pending insights with a link to `/inbox`), `Add learning reference` (GitHub URL input → `previewAttachSource` → confirm via `confirmSourceProposal`), per-row `Unlink` (`ConfirmDialog` → `previewDetachSource` → confirm). Empty state: `No learning references yet. Link a repository or document whose ideas should improve this skill.`
3. **Review tab provenance**: replace the hard-coded Provenance card body (`ReviewTab.tsx:174-198`, constant `REF_MAIN_CANONICAL`) with a new `ProvenanceCard.tsx` rendering the flat `review.provenance` fields (`source_locator`, `source_revision`, `upstream_path`, `source_id`, `created_by`, `created_at`) or `Created in this workspace` when absent, with a `View sources →` button switching to `?tab=sources`. No other edit to `ReviewTab.tsx`.
4. **Skills list**: chip next to the lifecycle badge for `update_available` (`Update available`), `diverged` (`Diverged`), `upstream_removed` (`Removed upstream`); a filter select bound to URL param `upstream` with options `All`, `Updates` (`update_available` + `diverged`), `Modified locally` (`modified` + `diverged`), `Not tracked` (`untracked`); a header chip `Updates (N)` that sets `upstream=updates`.
5. **Sources screen** (`/sources` → new `screens/sources/SourcesScreen.tsx`; `/sources/watch`, `/sources/distill`, `/sources/runs/:id` stay `LaterPhasePage` until phase 10, which adds distill selection to this screen): toolbar `Add skills from GitHub` (→ `/skills/add`) and `Check all` (`checkSources({all: true})`); one block per repository group; rows show source ID, ref, role chips, linked skills (links to `/skills/:id` with status chip), last checked, watch cadence, and actions `Check now`, `Import more` (dialog: `previewSourceImport` → checklist of discoverable skills with `imported` ones disabled → `confirmSourceImport`), `Unwatch` (`ConfirmDialog` → `previewUnwatchSource` → confirm). Orphan rows show a warning chip `No linked skills`. States: loading (Skeleton rows), empty (`No sources yet. Sources appear when you add skills from a repository or link a learning reference.` + `Add skills from GitHub`), error (danger banner with message and `Retry`). Check results show per-source outcome text from `SourceCheckResult` for the current session only.
6. **Home**: `action-cta.ts` cases `review_upstream_updates` → link `/skills?upstream=updates` (`Review updates →`), `track_upstream_skills` → command `skillhub source backfill`, `link_orphan_sources` → link `/sources` (`Open sources →`). Add matching i18n keys `action.review_upstream_updates`, `action.track_upstream_skills`, `action.link_orphan_sources`.
7. Strings live in `i18n/en.ts` where the surrounding screen already uses `t()`; screens that use local constants (e.g. `SkillDetailScreen.tsx`) keep that pattern. No new npm dependency.

## Related code files

Create: `web/src/screens/skill-detail/{SourcesTab,UpstreamSection,UpstreamReview,LearningSection,ProvenanceCard}.tsx`, `web/src/screens/skill-detail/sources-tab.test.tsx`, `web/src/screens/sources/SourcesScreen.tsx`, `web/src/screens/sources/sources.test.tsx`.

Modify: `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/screens/skill-detail/SkillDetailScreen.tsx` (tab button + panel only), `web/src/screens/skill-detail/ReviewTab.tsx` (provenance card only), `web/src/screens/skills/SkillsScreen.tsx`, `web/src/screens/skills/skills.test.tsx`, `web/src/screens/home/action-cta.ts`, `web/src/screens/home/home.test.tsx`, `web/src/routes.tsx` (`/sources` only), `web/src/i18n/en.ts`, `web/e2e/smoke.spec.ts`.

Do not modify any other file.

## Implementation steps

### Task 9.1 — Types and queries
- Steps: implement Requirement 1, matching the Go JSON field names from phases 3–5 and 8 (read the structs; do not guess).
- Verify: `cd web && npm run typecheck` exits 0.

### Task 9.2 — Sources tab and provenance
- Steps: implement Requirements 2–3. Tests in `sources-tab.test.tsx` include a `ProvenanceCard` case built from the remote-skill golden `internal/delivery/web/testdata/golden/skill-review-third-party.json` (it pins the flat shape; a local fixture alone cannot catch a shape mismatch) and a case asserting `['skill-runtime', id]` is invalidated after confirm, plus these cases (pattern of `skills.test.tsx`: `QueryClient` with `setQueryData`, `MemoryRouter`): loading skeleton; local skill (upstream null) hides the Upstream section and shows the learning empty state; `update_available` shows `Review update` enabled; a review response with `unresolved: ["SKILL.md"]` disables confirm and shows `1 file(s) need a decision`; a response with pins and `review_required_after_apply` shows `skillhub skill review <id>`; API error shows the danger banner.
- Verify: `cd web && npx vitest run src/screens/skill-detail` exits 0 and reports all tests passed.

### Task 9.3 — Skills list and Home
- Steps: implement Requirements 4 and 6; extend `skills.test.tsx` (chip renders for `update_available`; `?upstream=updates` filters to it) and the `home.test.tsx` table with the three new kinds.
- Verify: `cd web && npx vitest run src/screens/skills src/screens/home` exits 0 and reports all tests passed.

### Task 9.4 — Sources screen
- Steps: implement Requirement 5 with `sources.test.tsx`: empty state text; two groups render repository headings; orphan chip; error banner with `Retry`.
- Verify: `cd web && npx vitest run src/screens/sources` exits 0 and reports all tests passed.

### Task 9.5 — E2E smoke
- Steps: in `e2e/smoke.spec.ts` add steps: open `/sources` → empty state text visible; open `/skills/<fixture skill>?tab=sources` → learning empty state visible; keep the axe check covering both pages.
- Verify: `make web-e2e` exits 0. If Playwright browsers cannot be installed in this environment, record that in the phase report and continue only after `make web-check` passes (the Failure Protocol applies to every other failure).

### Task 9.6 — Gate
- Verify: `make web-check` exits 0 and `make check` exits 0.

## Todo

- [x] Task 9.1 types + queries
- [x] Task 9.2 Sources tab + provenance
- [x] Task 9.3 Skills list + Home
- [x] Task 9.4 Sources screen
- [x] Task 9.5 e2e smoke
- [ ] Task 9.6 gates

## Success criteria

A user can, without the CLI: see which skills have upstream updates, check one now, review a 3-way update file by file, apply it, see that re-approval is needed, link or unlink learning references, and manage sources grouped by repository.

## UX acceptance

| Surface | Loading | Empty | Error | Key states |
|---|---|---|---|---|
| Sources tab / Upstream | Skeleton facts | hidden (local skill) | danger banner + Retry | badge per status; `Check now` spinner; `Review update` enabled only when actionable |
| Upstream review | Skeleton table | `Upstream is unchanged.` | banner with service message; 409 → `Review again` | unresolved → confirm disabled; pins → trust banner + confirm |
| Learning section | Skeleton rows | `No learning references yet. …` | banner + Retry | rows with pending-insight counts |
| Skills list | existing | existing | existing | `Update available` / `Diverged` / `Removed upstream` chips; `Updates (N)` chip |
| `/sources` | Skeleton rows | `No sources yet. …` + `Add skills from GitHub` | banner + Retry | repository groups; orphan chip; per-row Check now / Import more / Unwatch |

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Merge conflicts with the runtime plan's Skill Detail work | M×M | Runs after it; edits limited to tab button/panel and the provenance card; new components in new files. |
| Large diffs render slowly | L×L | Diffs are per file and server-generated; render one selected file at a time. |
| Users approve content from the browser by mistake | L×H | No approve action in the WebUI; only the copyable CLI command. |

## Security considerations

Upstream diffs are rendered as text through `DiffView` (no `dangerouslySetInnerHTML`). Markdown preview is not used for upstream content. All calls go through `apiFetch` with the session token.

## Rollback

Revert the phase commit; `/sources` returns to `LaterPhasePage`.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
