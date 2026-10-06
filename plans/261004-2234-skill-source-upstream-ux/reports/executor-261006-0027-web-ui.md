# Executor Report: Phase 9 - Web UI

**Phase:** Phase 9: Web UI
**Branch:** `feat/skill-source-upstream`
**Timestamp:** 2026-10-06 00:27 Asia/Saigon

## 1. What Changed

1. **TypeScript Types & Query Hooks (`web/src/api/types.ts`, `web/src/api/queries.ts`)**:
   - Corrected `SkillProvenance` to the flat structure returned by `app.SkillProvenance` (`created_by`, `created_at`, `source_id`, `source_locator`, `source_revision`, `upstream_path`).
   - Added `upstream_status?: string` to `SkillListItem`.
   - Added all new types for upstream updates and sources: `SkillUpstream`, `UpstreamFile`, `UpstreamTrustImpact`, `UpstreamUpdatePreview`, `UpstreamUpdateResult`, `LearningReference`, `SkillSourcesResult`, `SourceSummary`, `SourceRepositoryGroup`, `SourceCandidate`, `SourceRecord`, `SourceListItem`, `SourceListResult`, `SourceProposal`, `SourceMutationResult`, `DiscoveredImportSkill`, `SourceImportProposal`, `SourceCheckItem`, `SourceCheckResult`, and `UpstreamResolution`.
   - Added query hooks `useSkillSources`, `useSources`, and mutation functions `checkSkillUpstream`, `reviewSkillUpdate`, `confirmUpstreamUpdate`, `previewAttachSource`, `previewDetachSource`, `previewUnwatchSource`, `confirmSourceProposal`, `checkSources`, `previewSourceImport`, `confirmSourceImport`.

2. **Skill Detail Sources Tab & Provenance Card (`web/src/screens/skill-detail/`)**:
   - Implemented `ProvenanceCard`: replaces hard-coded provenance in `ReviewTab.tsx` with live data from `review.provenance`, provides a `View sources →` shortcut to `?tab=sources`, and falls back to "Created in this workspace" when absent.
   - Added `Sources` tab in `SkillDetailScreen.tsx` with an indicator badge when `upstream.status` is `update_available` or `diverged`, accessible via `?tab=sources`.
   - Implemented `SourcesTab`: composes `UpstreamSection` and `LearningSection`.
   - Implemented `UpstreamSection`: hidden for local skills; shows status badge with semantic tones, commit metadata, changed file list, `Check now` trigger with spinner, and `Review update` button.
   - Implemented `UpstreamReview`: file table with action selection (`Take upstream`, `Keep mine`, `Auto-merged`, `Edit manually`), `DiffView` with mode toggles (result/upstream/local), prefilled manual conflict editor textarea, `Apply choices`, trust impact notice, and `Confirm update` that invalidates `skill-sources` and `skill-runtime`.
   - Implemented `LearningSection`: list of learning references with pending insight links to `/inbox`, unlinking via `ConfirmDialog`, and inline GitHub locator attachment form.

3. **Skills List & Home CTAs (`web/src/screens/skills/`, `web/src/screens/home/`, `web/src/i18n/en.ts`)**:
   - Updated `SkillsScreen.tsx`: renders status chips next to lifecycle badge for `update_available`, `diverged`, and `upstream_removed`; added `upstream` select filter (`all`, `updates`, `modified`, `untracked`) and header `Updates (N)` chip.
   - Updated `action-cta.ts`: added cases for `review_upstream_updates` (`/skills?upstream=updates`), `track_upstream_skills` (`skillhub source backfill`), and `link_orphan_sources` (`/sources`).
   - Added i18n keys in `en.ts`.

4. **Sources Screen (`web/src/screens/sources/SourcesScreen.tsx`, `web/src/routes.tsx`)**:
   - Mapped `/sources` to `SourcesScreen`.
   - Implemented toolbar (`Add skills from GitHub`, `Check all`), repository group blocks with source details and linked skills, orphan warning chips, `Check now`, `Import more` dialog with discovery checklist, and `Unwatch` confirmation dialog.

5. **Test Coverage & Accessibility (`web/src/`, `web/e2e/smoke.spec.ts`)**:
   - Added unit and integration tests: `sources-tab.test.tsx` (17 tests covering provenance golden, empty states, review flow, query invalidation, link/unlink mutations) and `sources.test.tsx` (empty state, groups, orphan chip, retry on error).
   - Extended `smoke.spec.ts`: verifies `/sources` empty state, `/skills/smoke-skill?tab=sources` learning empty state, and passed Axe WCAG 2.1 AA accessibility checks across light/dark schemes and mobile/desktop viewports.

## 2. Commands Executed and Results

| Command | Purpose | Result |
|---|---|---|
| `git branch --show-current` | Verify branch | `feat/skill-source-upstream` |
| `cd web && npm run typecheck` | Task 9.1 verification | Pass (exit 0) |
| `cd web && npx vitest run src/screens/skill-detail` | Task 9.2 verification | Pass (17/17 passed) |
| `cd web && npx vitest run src/screens/skills src/screens/home` | Task 9.3 verification | Pass (10/10 passed) |
| `cd web && npx vitest run src/screens/sources` | Task 9.4 verification | Pass (3/3 passed) |
| `make web-e2e` | Task 9.5 verification (Playwright smoke + Axe checks) | Pass (5/5 passed) |
| `make web-check` | Task 9.6 frontend verification (typecheck, lint, test, build) | Pass (exit 0) |
| `make check` | Task 9.6 backend verification (vet, golangci-lint, full test suite) | Pass (exit 0) |

## 3. Deviations and Why

None. All implementations and constraints strictly adhered to the Phase 9 plan and architecture decisions.

## 4. Open Questions

None.
