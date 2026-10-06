# Executor Report: Phase 11 — Inbox, Insight, Patch Composer

**Phase:** Phase 11: Inbox, Insight, Patch Composer  
**Status:** Complete  
**Branch:** `feat/skill-source-upstream`  
**Timestamp:** 2026-10-06 12:50 +0700 (Asia/Saigon)  

---

## 1. What Changed

### Task 11.1 — Paging Package
- Extracted shared pagination code from `internal/delivery/mcpserver/` into new package `internal/delivery/paging`:
  - Exported `DefaultLimit` (25), `MaximumLimit` (100), `Page[T]` (`items`, `has_more`, `next_cursor`, `total`), `Owner`, `EncodeCursor`, `DecodeCursor`, `NormalizeLimit`, and `Make[T]`.
  - Moved `TestOpaqueCursorMultiPageAndIntegrity` from `mcpserver/server_test.go` to `paging/paging_test.go`.
  - Updated `curation_tools.go`, `insight_tools.go`, `source_tools.go`, `workspace_tools.go`, `server.go`, `types.go`, `server_test.go`, and `hardening_test.go` to use `paging`.
  - Verified fuzz test `FuzzMCPFrameAndInputHelpers` and MCP test suite passed with identical behavior.

### Task 11.2 — Sentinel
- Exported `ErrInsightNotFound = errors.New("insight not found")` in `internal/app/insight.go` and used it in `GetInsightDetail` when an insight ID does not exist.

### Task 11.3 — Endpoints
- Created `internal/delivery/web/routes_insights.go`:
  - `GET /api/v1/inbox?limit=&cursor=`: pagination via `paging.NormalizeLimit` and `paging.Make`. Returns HTTP 410 `snapshot_expired` if the cursor is invalid, tampered, or expired due to underlying inbox changes.
  - `GET /api/v1/insights/{id}`: returns insight detail with findings and comparisons; returns 404 for nonexistent insight IDs.
  - `POST /api/v1/insights/{id}/decision`: records decisions (`plan`, `reject`, `reopen`, `obsolete`) with required rationale. Automatically derives deterministic idempotency key; rejects unknown fields like `idempotency_key` in request body.
  - `POST /api/v1/insights/{id}/apply/preview`: generates patch preview targeting the skill's canonical `SKILL.md` path. Rejects unchanged content or missing observation mappings.
  - `POST /api/v1/insights/apply/confirm`: confirms proposal using exact pins (`proposal_id`, `proposal_digest`, `base_version`). Returns HTTP 409 `stale_proposal` on digest mismatch.
- Added helper `seedPendingInsight(t, root)` to `internal/delivery/web/fixtures_test.go`.
- Created `internal/delivery/web/routes_insights_test.go` with `TestInboxPaging`, `TestInsightEndpoints`, `TestInboxGolden`, and `TestInsightDetailGolden`. Generated normalized goldens `testdata/golden/inbox.json` and `testdata/golden/insight-detail.json`.

### Task 11.4 — Pure Domain Logic
- Extended `web/src/api/types.ts` with `Insight`, `Observation`, `Comparison`, `InsightInboxItem`, `InsightInboxGroup`, `InboxPageResult`, `InsightDetailResult`, `InsightApplicationPreview`, and `InsightApplicationResult`.
- Added queries and mutations to `web/src/api/queries.ts`: `fetchInboxPage`, `useInsightDetail`, `decideInsight`, `previewInsightApply`, and `confirmInsightApply`.
- Created `web/src/domain/evidence-set.ts` and `evidence-set.test.ts`:
  - `requiredObservations`: computes deduplicated union of direct and comparison observations, tagging their provenance.
  - `coverage`: checks mappings against required observations, verifying non-empty concept mappings.
  - `addMapping`: appends concepts while deduplicating case-insensitive observation/concept pairs.
- Created `web/src/domain/insight-staleness.ts` and `insight-staleness.test.ts`:
  - `isInsightStale`: detects when a direct finding is missing or not active, or when a comparison is stale.
- Created `web/src/state/composer-draft.ts` and `composer-draft.test.ts`:
  - Persists draft editor content and mappings, tracking `needsRebase` (when skill content digest changes) and `staleEvidence` (when evidence digest changes).

### Task 11.5 — Screens & Route Wiring
- Created `web/src/screens/inbox/InboxScreen.tsx`:
  - Grouped inbox view with filters for status, category, and priority over loaded pages with `Filtering N loaded groups`.
  - `Load more` pagination and 410 `snapshot_expired` notice with reload button.
  - Two distinct empty states: no insights at all vs. no matches in loaded groups with `Clear filters` button.
- Created `web/src/screens/insight/InsightDetailScreen.tsx`:
  - Full detail view displaying recommendation, rationale, priority/category chips, findings, comparisons with stale warning, and decision history.
  - Status actions (`Plan`, `Reject`, `Reopen`, `Obsolete`, `Compose patch`).
  - Decision modal requiring non-empty rationale; `Obsolete` action protected by `ConfirmDialog`.
  - Disables `Compose patch` with reason when insight evidence is stale.
- Created `web/src/screens/composer/PatchComposerScreen.tsx`:
  - Target `SKILL.md` path heading and full-replacement textarea editor.
  - Evidence mapping sidebar with `mapped/required` live region counter and concept tagging.
  - `Preview apply` button enabled only when content differs and coverage is complete.
  - Pre-preview conflict detection: checks live skill `content_digest` against draft baseline, opening `ConflictDrawer` on mismatch.
  - `ProposalPreview` modal diff inspection and confirmation with pins.
  - Receipt card with CTA linking to `/skills/:id?tab=review`.
  - Query cache invalidations across skill, review, runtime, sources, inbox, and home.
- Updated `web/src/routes.tsx` to map `/inbox`, `/inbox/:id`, and `/inbox/:id/apply`.
- Deleted `web/src/components/LaterPhasePage.tsx` as all routes in Skill Hub are now real.

### Task 11.6 — E2E Journey & Accessibility
- Created `web/e2e/insight-apply.spec.ts`:
  - Tests end-to-end flow: inbox viewing -> reject with rationale -> reopen refusal assertion for unchanged evidence -> plan insight -> compose patch.
  - Tests conflict detection: modifies skill outside via CLI while composer is open, triggers preview, asserts `ConflictDrawer` opens, and resolves via "Use latest as base".
  - Completes patch application, verifies receipt, and confirms skill review tab navigation.
  - Runs Axe accessibility checks on `/inbox`, `/inbox/:id`, and `/inbox/:id/apply`.

---

## 2. Mockup-Parity Checklist

Checklist against `docs/design/webui-mockup/Skill Hub WebUI.dc.html` lines 447–552:

### Inbox Screen (Mockup lines 447–475)
- [x] Status select (`All`, `Pending`, `Planned`, `Rejected`, `Obsolete`).
- [x] Category select (`All`, plus unique loaded categories).
- [x] Priority select (`All`, `Critical`, `High`, `Medium`, `Low`).
- [x] Filtering summary caption (`Filtering N loaded groups`).
- [x] Skeleton loading state.
- [x] Empty state when no insights exist (`No pending or planned insights.`).
- [x] Filtered empty state with `Clear filters` button (`No matches in the loaded groups.`).
- [x] Snapshot expired notice banner with `Reload` action.
- [x] Group headers showing `skill_id · category` and item count badge.
- [x] Insight cards showing priority badge, recommendation title, caption (`Score X · Impact Y · N findings`), stale marker, status badge, and `Open →` link.
- [x] `Load more` button active when `has_more` is true.

### Insight Detail Screen (Mockup lines 476–509)
- [x] Header card with ID, recommendation title, rationale text, and priority/category/skill chips.
- [x] Stale evidence badge when supporting evidence is no longer current.
- [x] Findings card listing direct observations with ID and text.
- [x] Comparisons card with subject, verdict, trade-offs, and stale warning tag.
- [x] Decision history timeline showing past decisions with decision kind, timestamp, and rationale.
- [x] Sidebar showing status badge and context-valid action buttons (`Plan`, `Reject`, `Reopen`, `Obsolete`, `Compose patch`).
- [x] `Compose patch` button disabled with explanatory title when insight evidence is stale.
- [x] Modal for Plan, Reject, and Reopen requiring non-empty rationale.
- [x] Obsolete action behind `ConfirmDialog` with required rationale.
- [x] 404 Not Found state with link to `/inbox`.

### Patch Composer Screen (Mockup lines 510–552)
- [x] Target file path caption (`Target file: skills/.../SKILL.md`).
- [x] Full-replacement textarea editor seeded with current canonical content.
- [x] Evidence mapping section with live counter `<span aria-live="polite">{mapped}/{required}</span>`.
- [x] Required observations list with observation ID, source tag (direct vs comparison), concept tags with remove button, and input to add concept.
- [x] Sticky bottom action bar with `Save local draft`, `Mapped X of Y required` caption with disabled reason, `Cancel` button, and `Preview apply` button.
- [x] `Preview apply` disabled until content differs (normalizing trailing newline) and all observations are mapped.
- [x] Concurrent edit conflict detection: refetches skill digest before preview and opens `ConflictDrawer` on mismatch.
- [x] `ProposalPreview` modal displaying diff, proposal ID, proposal digest, and base version pins.
- [x] Confirmation receipt displaying success message and CTA button linking to skill Review tab.

---

## 3. Screenshots

Captured in desktop (1280×800) and mobile (360×740) viewports under `plans/261004-2234-skill-source-upstream-ux/reports/screenshots/phase-11/`:

- Inbox Screen:
  - Desktop: [inbox-desktop.png](./screenshots/phase-11/inbox-desktop.png)
  - 360px: [inbox-360px.png](./screenshots/phase-11/inbox-360px.png)
- Insight Detail Screen:
  - Desktop: [insight-desktop.png](./screenshots/phase-11/insight-desktop.png)
  - 360px: [insight-360px.png](./screenshots/phase-11/insight-360px.png)
- Patch Composer Screen:
  - Desktop: [composer-desktop.png](./screenshots/phase-11/composer-desktop.png)
  - 360px: [composer-360px.png](./screenshots/phase-11/composer-360px.png)

---

## 4. Verification Commands & Results

### Task 11.1 Baseline vs Post-Paging Test Output
- Baseline: `go test -count=1 ./internal/delivery/mcpserver/`  
  `ok  	github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver	24.531s`
- Post-paging: `go test -count=1 ./internal/delivery/mcpserver/ ./internal/delivery/paging/`  
  `ok  	github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver	21.462s`  
  `ok  	github.com/vantt/mcp-skill-hub/internal/delivery/paging	0.002s`
- Standalone integrity test: `go test -count=1 -v -run '^TestOpaqueCursorMultiPageAndIntegrity$' ./internal/delivery/paging/` -> `--- PASS: TestOpaqueCursorMultiPageAndIntegrity (0.00s)`
- Removed check: `grep -nE '^func (decodeCursor|encodeCursor|normalizeLimit|makePage|pageOwner)' internal/delivery/mcpserver/*.go | wc -l` -> `0`
- Fuzz check: `go test -count=1 -run '^$' -fuzz '^FuzzMCPFrameAndInputHelpers$' -fuzztime=10s ./internal/delivery/mcpserver/` -> `PASS`

### Test Verification Matrix
| Command | Exit Code | Result Summary |
|---|---|---|
| `go test -count=1 ./internal/app/... ./internal/delivery/...` | 0 | Task 11.2 sentinel pass |
| `go test -count=1 -v -run '^(TestInboxPaging\|TestInsightEndpoints)$' ./internal/delivery/web/` | 0 | Task 11.3 inbox paging & endpoints pass |
| `cd web && npx vitest run src/domain/evidence-set.test.ts src/domain/insight-staleness.test.ts src/state/composer-draft.test.ts` | 0 | Task 11.4 domain logic tests pass (13/13 tests) |
| `cd web && npx vitest run src/screens/inbox src/screens/insight src/screens/composer` | 0 | Task 11.5 screen unit tests pass (11/11 tests) |
| `make web-e2e` | 0 | Task 11.6 Playwright E2E tests pass (8/8 passed, including insight apply journey and axe scans) |
| `make web-check` | 0 | Task 11.7 frontend gate pass (typecheck, eslint, vitest 109/109 tests, production build) |
| `make check` | 0 | Task 11.7 full repository gate pass (go vet, golangci-lint 0 issues, full Go test suite pass across all 25 packages) |

---

## 5. Deviations & Rationale

1. **Synchronous draft state in `PatchComposerEditor`:**
   - *Deviation:* Extracted `PatchComposerEditor` as a child component of `PatchComposerScreen` so draft loading and initial state derivation run synchronously on mount.
   - *Rationale:* Eliminates synchronous `setState` in `useEffect`, preventing React cascading render warnings and ensuring full compliance with React hooks rules.
2. **Accessible color contrast adjustments:**
   - *Deviation:* Used `var(--color-text-muted)` for captions and `tone="neutral"` for medium priority and pending badges instead of `var(--color-text-subtle)` / `info` chip in light theme.
   - *Rationale:* `var(--color-text-subtle)` and `.fg-chip--info` yielded contrast ratios under 4.0:1 on `#faf8f2`; switching to muted and neutral exceeds the WCAG 2.1 AA 4.5:1 minimum threshold required by Axe scans.

---

## 6. Open Questions

None. All Phase 11 deliverables, tests, accessibility checks, and quality gates pass cleanly.
