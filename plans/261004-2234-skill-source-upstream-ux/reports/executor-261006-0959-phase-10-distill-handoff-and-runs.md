# Executor Report: Phase 10 — Distill handoff and runs

**Phase:** Phase 10: Distill handoff and runs  
**Status:** Complete  
**Branch:** `feat/skill-source-upstream`  
**Timestamp:** 2026-10-06 09:59 +0700 (Asia/Saigon)  

---

## 1. What Changed

### Task 10.1 — Sentinel
- Updated `internal/app/distill.go` to export `ErrDistillRunNotFound = errors.New("distill run not found")` and returned it from `readRun` when a requested run ID has no stored run YAML file.

### Task 10.2 — Run Endpoints & Route Safety
- Created `internal/delivery/web/routes_runs.go` with HTTP routes:
  - `GET /api/v1/runs/{id}`: calls `s.distill.GetDistillRun`, returns 404 when `errors.Is(err, app.ErrDistillRunNotFound)`, and returns 400 for path-traversal / malformed IDs.
  - `POST /api/v1/runs/{id}/cancel`: calls `s.distill.CancelDistillRun`, handles idempotent cancellation cleanly (200 with state `cancelled` if already cancelled).
- Added `fakeSourceAdapter`, `writeSourceRecord`, `seedRun`, and `seedFinalizedRun` helpers in `internal/delivery/web/fixtures_test.go`.
- Created `internal/delivery/web/routes_runs_test.go`:
  - `TestRunEndpoints`: verifies in-progress run retrieval (200), nonexistent run (404), invalid run ID (400), cancellation (200), and idempotent re-cancellation (200).
  - `TestRoutesNeverMutateRuns`: static AST inspection parsing non-test files in `package web` ensuring every route pattern is a string literal, verifying both GET and cancel endpoints exist, verifying no `start`, `retry`, or `submit` routes exist, and sending POST requests to `/runs/RUN-X/{start|retry|submit}` asserting 404 or 405.
  - `TestRunGolden`: generates and verifies normalized golden JSON for `GET /api/v1/runs/{id}` (`internal/delivery/web/testdata/golden/run.json`).

### Task 10.3 — Pure Domain Logic & Recent-Runs Store
- Extended `web/src/api/types.ts` with `DistillRun`, `DistillRunResult`, and distill fields on `SourceSummary` and `SourceRecord`.
- Added TanStack Query hook `useDistillRun` and mutation `cancelDistillRun` in `web/src/api/queries.ts`.
- Created `web/src/domain/source-distill.ts` and `source-distill.test.ts`:
  - `distillLabel(summary, sessionCheck?)` returning `{ label, selectable }` following strict priority: unavailable check takes precedence over changed status.
- Created `web/src/domain/handoff-brief.ts` and `handoff-brief.test.ts`:
  - `buildHandoffBrief`: provides exact numbered instructions for the Curator Agent including `curation_run_start`, reading revision packages, submitting analysis, and never leaking `#token=`.
  - `buildResumeBrief`: outputs resume guidance, including `curation_run_retry` for `failed` or `awaiting_decision` states, and never leaking `#token=`.
- Created `web/src/state/recent-runs.ts` and `recent-runs.test.ts`:
  - Local storage management scoped per workspace with functions `addRecentRun`, `updateRecentRun`, `removeRecentRun`, `listRecentRuns` (newest first, capped at 50 entries), and storage-unavailable error tolerance.

### Task 10.4 — Screens & Unit Tests
- Updated `web/src/routes.tsx`: removed `/sources/watch`, routed `/sources/distill` to `DistillHandoffScreen`, and `/sources/runs/:id` to `RunScreen`.
- Updated `web/src/components/AppShell.tsx`: removed `/sources/watch` title branch.
- Added translation keys to `web/src/i18n/en.ts`.
- Enhanced `web/src/screens/sources/SourcesScreen.tsx`:
  - Displays `distillLabel` badge on every source row.
  - Adds selection checkbox only on selectable rows.
  - "Distill with Curator Agent" button disabled with reason title until a source is selected, navigating to `/sources/distill?source=<id>`.
  - Supports URL query parameter `filter=ready` to filter only selectable rows.
  - "Check due sources" button next to "Check all".
  - "Open a run" box navigating to `/sources/runs/:id`.
  - "Recent runs on this browser (not a full workspace history)" panel with per-entry removal.
  - Added unit tests in `sources.test.tsx` for selection and `filter=ready`.
- Created `web/src/screens/distill/DistillHandoffScreen.tsx`:
  - Lists selected sources with current vs. distilled revisions and error diagnostics for unready/unknown sources.
  - Stable `handoff_request_id` draft persistence keyed by sorted source IDs.
  - Copyable brief code block with "Copy handoff" button.
  - "Paste run IDs returned by agent" input with per-ID loading, success, and error display, auto-adding successful runs to recent runs.
  - Empty selection view with "Back to Sources" link.
  - Added unit tests in `distill.test.tsx`.
- Created `web/src/screens/run/RunScreen.tsx`:
  - Header with clickable copyable run ID, attempt count, and live-region status badge.
  - Facts card with Revision transition, Package digest, and Changed resources list.
  - State panels for `prepared`, `in_progress`, `awaiting_decision` (with required decision field before enabling resume copy), `failed` (with failure message), `finalized` (with findings/comparisons/insights counts and "Open Inbox →" CTA), and `cancelled`.
  - Declarative polling interval (5 s) during active states (`prepared` and `in_progress`).
  - Cancel dialog with consequence text; cancel button hidden for finalized/cancelled runs.
  - Advisory caveat shown when run has no stored browser key.
  - 404 Not Found state with "Back to Sources" link.
  - Added unit tests in `run.test.tsx`.

### Task 10.5 — Seed & E2E Journey
- Created `web/e2e/support/seed-workspace.ts` implementing `seedDistillWorkspace()` using the real CLI:
  - Initializes workspace, creates & activates `consumer-review`.
  - Sets up `source-a` (finalized run with observations and insights), `source-b` (in_progress run), and `source-c` (checked and linked, never distilled).
  - Uses `runtime/fixtures/` to remain compatible with workspace canonical file boundaries.
- Updated `web/e2e/support/server.ts` to support optional custom workspace serving.
- Created `web/e2e/sources-runs.spec.ts`:
  - Test `@seed`: verifies via CLI `distill get` that `finalizedRunId` is `finalized` and `inProgressRunId` is `in_progress`.
  - Journey test: starts server on seeded workspace, navigates to `/sources?filter=ready` (shows `source-c`, hides `source-a`), selects `source-c`, clicks "Distill with Curator Agent", verifies brief content on `/sources/distill`, pastes `finalizedRunId`, follows link to Run screen, verifies "Finalized" panel with "Open Inbox →" link, navigates back to `/sources` and verifies recent runs panel, navigates to `inProgressRunId`, clicks "Cancel run", confirms dialog with consequence text, and verifies run status updates to "Cancelled".

---

## 2. Mockup-Parity Checklist

Checklist against `docs/design/webui-mockup/Skill Hub WebUI.dc.html` lines 386–446 (Distill, Run) and Sources additions:

### Sources Additions
- [x] Every row displays `distillLabel` status badge (e.g. `Changed`, `Distill pending`, `Never distilled`, `Up to date`, `Not a learning source`, `Unreachable (last check)`).
- [x] Checkboxes appear only on selectable rows (`!upstream_only && ready_to_distill`).
- [x] "Distill with Curator Agent" button is disabled with tooltip "Select at least one learning source" when selection is empty.
- [x] Button enables upon selecting a source, updating label to `Distill with Curator Agent (N)` and linking to `/sources/distill?source=...`.
- [x] `?filter=ready` query parameter renders only selectable sources, hiding unready groups.
- [x] "Check due sources" button invokes `checkSources({ due: true })`.
- [x] "Open a run" box accepts run IDs and navigates to `/sources/runs/:id`.
- [x] "Recent runs on this browser (not a full workspace history)" lists runs opened on the browser with links to `/sources/runs/:id`, status chips, and per-item "Remove" button.

### Distill Handoff (Mockup lines 386–406)
- [x] "Selected sources (N)" card shows source ID, revision transition (`curRev ← distRev`), and status chip.
- [x] Code block titled "Handoff for your Curator Agent" with "Copy handoff" button.
- [x] Brief includes exact curation instructions, source IDs, and draft-persisted idempotency key; never contains `#token=`.
- [x] "Paste run IDs returned by agent" textarea with placeholder `RUN-… one per line`.
- [x] Caption note: `ⓘ The WebUI never starts a run. Your agent does.`.
- [x] "Open runs" button loads each run independently, reporting per-ID progress, success links, and error banners.
- [x] Empty selection shows `Select learning sources on the Sources screen first.` with "Back to Sources" link.

### Run Return (Mockup lines 409–444)
- [x] Header displays click-to-copy run ID with copied feedback indicator, status chip with `aria-live="polite"`, source ID, and attempt number.
- [x] Facts card displays Revision (`from → to`), Package digest, and Changed resources count with expandable list.
- [x] Awaiting decision panel: displays category chip, path/detail text, required decision textarea marked with `*` and helper hint, and enables "Copy resume handoff" only when decision text is entered.
- [x] Finalized panel: displays Coverage/Findings/Comparisons/Insights counts and "Open Inbox →" CTA button linking to `/inbox`.
- [x] Failed panel: displays danger banner with failure explanation and enables "Copy resume handoff" when key is present.
- [x] Cancelled panel: read-only card stating the source cursor was not advanced.
- [x] Advisory caveat displayed when run has no stored browser key: resume is unavailable.
- [x] Actions bar: "Refresh status", "Copy resume handoff" (when available), and danger "Cancel run" button.
- [x] Cancel dialog: ConfirmDialog presents consequence text (`Cancelling this run will stop processing. The source cursor does not advance, and an agent working on this run will fail to submit.`) before executing `POST /api/v1/runs/{id}/cancel`.
- [x] 404 Not Found state displays "Run not found" and link to `/sources`.

---

## 3. Screenshots

Captured in both desktop (1280×800) and mobile (360×740) viewports under `plans/261004-2234-skill-source-upstream-ux/reports/screenshots/phase-10/`:

- Sources Screen:
  - Desktop: [sources-desktop.png](./screenshots/phase-10/sources-desktop.png)
  - 360px: [sources-360px.png](./screenshots/phase-10/sources-360px.png)
- Distill Handoff Screen:
  - Desktop: [distill-desktop.png](./screenshots/phase-10/distill-desktop.png)
  - 360px: [distill-360px.png](./screenshots/phase-10/distill-360px.png)
- Run Screen:
  - Desktop: [run-desktop.png](./screenshots/phase-10/run-desktop.png)
  - 360px: [run-360px.png](./screenshots/phase-10/run-360px.png)

---

## 4. Verification Commands & Results

| Command | Exit Code | Result Summary |
|---|---|---|
| `go test -count=1 ./internal/app/... ./internal/delivery/...` | 0 | Task 10.1 sentinel pass |
| `go test -count=1 -v -run '^(TestRunEndpoints\|TestRoutesNeverMutateRuns)$' ./internal/delivery/web/` | 0 | Task 10.2 run endpoints and AST route safety pass |
| `cd web && npx vitest run src/domain src/state/recent-runs.test.ts` | 0 | Task 10.3 domain logic and store tests pass (14/14 tests) |
| `cd web && npx vitest run src/screens/sources src/screens/distill src/screens/run` | 0 | Task 10.4 screen tests pass (17/17 tests) |
| `make web-e2e` | 0 | Task 10.5 Playwright E2E tests pass (7/7 passed, including `@seed` and distill journey) |
| `make web-check` | 0 | Task 10.6 frontend gate pass (typecheck, eslint, vitest 85/85 tests, production build) |
| `make check` | 0 | Task 10.6 full repo gate pass (go vet, golangci-lint 0 issues, full test suite pass across all packages) |

---

## 5. Deviations & Rationale

1. **`runtime/fixtures/` for seed workspace:**
   - *Deviation:* In `web/e2e/support/seed-workspace.ts`, filesystem source fixtures were placed in `<ws>/runtime/fixtures/<source_id>` instead of `<ws>/fixtures/<source_id>`.
   - *Rationale:* The workspace canonical validator strictly enforces that files under `<ws>` must reside within canonical entity directories (`skills/`, `sources/`, `distill/`, etc.) unless excluded by `isNonCanonicalPath`. `<ws>/runtime/` is explicitly excluded from canonical validation, while allowing the filesystem adapter to resolve paths relative to the workspace root.
2. **Declarative polling interval in `RunScreen.tsx`:**
   - *Deviation:* Replaced `useEffect` + `setState` with TanStack Query's functional `refetchInterval: (query) => ...`.
   - *Rationale:* Satisfies React Hooks and ESLint rules against triggering cascading renders via synchronous `setState` in effects.

---

## 6. Open Questions

None. All Phase 10 tasks, unit tests, E2E journeys, and quality gates pass cleanly.
