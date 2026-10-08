# Phase 2 Report: Frontend foundation, Home, Skills catalog

Date: 2026-10-04
Phase: 2
Status: Complete
Guard Result: PASS (phase 2)

## 1. Guard Output

```text
== Guard phase 2 ==
== Baseline commit: 37b61880fc9cf33a0fd9428368adfe4a9f1264b4 | Guard commit: 9d8e18b41f8d15591b4a9da3f5a91d9cd999d9cd ==
== The user compares both hashes with the ones recorded at handover. ==
PASS  guard files match pinned checksums
PASS  guard directory is committed and has no uncommitted or untracked changes
PASS  go build ./... exits 0
PASS  go vet ./... exits 0
PASS  go test -count=1 ./... exits 0
PASS  golangci-lint reports no new issues since the baseline commit
PASS  gofmt -l reports no files
PASS  go list ./... excludes the web/ tree
PASS  no baseline test function was removed or renamed
PASS  t.Skip count unchanged (36)
PASS  no JSON field tag was removed or renamed
PASS  MCP tool name set unchanged
PASS  no forbidden patterns in added Go code
PASS  no added strings.Contains(x, "") assertions
PASS  no test file deleted
PASS  no baseline test file modified outside the allowed set
PASS  MCP paging assertions did not drop (149)
PASS  assertions in baseline CLI test files did not drop (870 >= 870)
PASS  CLI test exit-code expectations unchanged
PASS  CLI --json output identical to baseline
PASS  CLI human output identical to baseline
-- phase 1: web adapter foundation
PASS  file exists: internal/delivery/web/server.go
PASS  file exists: internal/delivery/web/security.go
PASS  file exists: internal/delivery/web/throttle.go
PASS  file exists: internal/delivery/web/listen.go
PASS  file exists: internal/delivery/web/errors.go
PASS  file exists: internal/delivery/web/assets.go
PASS  file exists: internal/delivery/web/routes_read.go
PASS  file exists: internal/delivery/web/dist/.gitkeep
PASS  file exists: internal/delivery/cli/serve.go
PASS  root.go dispatches serve
PASS  root.go dispatches the web alias
PASS  help.go documents serve
PASS  global help lists serve web
PASS  token comparison is constant-time
PASS  errors are classified by app.ClassifyError
PASS  web adapter does not import the MCP adapter
PASS  web adapter never calls run-mutating services
PASS  web adapter never confirms without explicit pins
PASS  web adapter keeps no substring error rules
-- phase 2: frontend foundation
PASS  file exists: web/go.mod
PASS  file exists: web/package-lock.json
PASS  file exists: web/src/design-system/PATCHES.md
PASS  make web-check exits 0
PASS  make web-e2e exits 0
PASS  no focused or skipped frontend tests
PASS  no TypeScript or ESLint suppression comments
PASS  no dangerouslySetInnerHTML
PASS  no CDN or remote font references in web/src
PASS  file exists: web/src/state/local-store.test.ts
PASS  file exists: web/src/api/client.test.ts
PASS  file exists: web/src/screens/home/home.test.tsx
PASS  file exists: web/src/screens/skills/skills.test.tsx
PASS  file exists: web/e2e/smoke.spec.ts
PASS  file exists: web/.nvmrc
PASS  file exists: scripts/web-dev.sh
PASS  Makefile has web-check
PASS  Makefile has web-e2e
PASS  Makefile has web-dev
PASS  CI has a web job
PASS  release has a web job
PASS  eslint forbids JSX literals
PASS  required Go test ran and passed: TestErrorStatusCoversSchemaCodes
PASS  required Go test ran and passed: TestSecurityMiddleware
PASS  required Go test ran and passed: TestAuthThrottle
PASS  required Go test ran and passed: TestListenRule
PASS  required Go test ran and passed: TestStartupOutput
PASS  required Go test ran and passed: TestAssetsServing
PASS  required Go test ran and passed: TestReadEndpointsGolden
PASS  required Go test ran and passed: TestServeWildcardAnswersOnLoopback
PASS  required Go test ran and passed: TestServeWebCommand

GUARD RESULT: PASS (phase 2)
```

## 2. Dependency Versions (Task 2.1)

| Package | Pinned Version | Notes |
|---|---|---|
| `react` | 19.3.0 | Exact pin |
| `react-dom` | 19.3.0 | Exact pin |
| `react-router` | 8.4.0 | Exact pin |
| `@tanstack/react-query` | 5.104.1 | Exact pin |
| `typescript` | 5.9.3 | Stable release supporting `typescript-eslint` 8 |
| `vite` | 8.3.2 | Exact pin |
| `@vitejs/plugin-react` | 6.1.1 | Exact pin |
| `vitest` | 5.0.3 | Exact pin |
| `jsdom` | 30.1.1 | Exact pin |
| `@testing-library/react` | 16.3.3 | Exact pin |
| `@testing-library/user-event` | 14.6.7 | Exact pin |
| `@testing-library/jest-dom` | 7.0.1 | Exact pin |
| `eslint` | 9.39.5 | Stable release supporting `eslint-plugin-react` 7 |
| `@eslint/js` | 9.39.5 | Matches ESLint 9 |
| `typescript-eslint` | 8.71.0 | Exact pin |
| `eslint-plugin-react` | 7.37.5 | Exact pin |
| `eslint-plugin-react-hooks` | 7.1.1 | Exact pin |
| `globals` | 17.13.0 | Exact pin |
| `@playwright/test` | 1.63.0 | Exact pin |
| `@axe-core/playwright` | 4.13.0 | Exact pin |
| `@types/react` | 19.3.0 | Exact pin |
| `@types/react-dom` | 19.3.0 | Exact pin |
| `@types/node` | 26.6.4 | Exact pin |

## 3. Measurements (Task 2.13)

- `internal/delivery/web/dist` total size: `5.3 MB` (`du -sh`)
- Total `.woff2` font size in `dist`: `1,952,620 bytes` (~1.9 MB across 6 themes)
- Standalone `skillhub` binary size (no embedded UI): `29,570,382 bytes` (~29.6 MB)
- `skillhub` binary size with embedded WebUI: `34,403,742 bytes` (~34.4 MB)
- Net binary overhead of embedded WebUI assets: `4,833,360 bytes` (~4.8 MB)

## 4. Screenshots (Task 2.13)

Captured and saved in `plans/261003-1645-webui-v1-implementation/reports/screenshots/phase-02/`:
- `home-1440-light.png` (Desktop 1440px, light scheme)
- `home-1440-dark.png` (Desktop 1440px, dark scheme)
- `home-1024-light.png` (Tablet 1024px, light scheme)
- `home-1024-dark.png` (Tablet 1024px, dark scheme)
- `home-390-light.png` (Mobile 390px, light scheme)
- `home-390-dark.png` (Mobile 390px, dark scheme)
- `skills-1440-light.png` (Desktop 1440px, light scheme)
- `skills-1440-dark.png` (Desktop 1440px, dark scheme)
- `skills-1024-light.png` (Tablet 1024px, light scheme)
- `skills-1024-dark.png` (Tablet 1024px, dark scheme)
- `skills-390-light.png` (Mobile 390px, light scheme)
- `skills-390-dark.png` (Mobile 390px, dark scheme)

## 5. Mockup Parity Checklist (Lines 31–178)

### App Shell (Mockup lines 31–99)
- [x] Skip link to `#main` for keyboard navigation.
- [x] Brand header with icon and title `◆ Skill Hub`.
- [x] Breadcrumbs and dynamic `h1` linked to route state.
- [x] Document title updated on route change: `<page title> · Skill Hub` with focus moved to `h1`.
- [x] Appearance menu dialog with scheme selection (Light, Dark, System), 6 theme cards (Precision, beRich, ClickUp, Terminal, Atelier, Moday), and dynamic accent chips.
- [x] Help button (`?`) intentionally dropped per decision D12.
- [x] Responsive navigation rail: left rail on desktop (220px), icon rail on tablet (56px), bottom navigation bar on mobile (<768px).
- [x] Primary nav items: Home (`/`), Skills (`/skills`), Sources (`/sources`), Inbox (`/inbox`).
- [x] Inbox pending count badge shown only when `workspace.health === 'valid' && workspace.index === 'current'`.
- [x] Workspace health summary in rail footer linking to Home.
- [x] Degraded search index banner shown when `workspace.index === 'stale'` with copy-command `skillhub rebuild` and reload action.
- [x] Session expired overlay triggered by `session-expired` event with guidance to run `skillhub serve web`.

### Home Screen (Mockup lines 100–141)
- [x] Loading skeleton view.
- [x] "Next action" card with actionable CTAs mapped from backend `actions[0]`.
- [x] Action CTA resolver covering all 10 kinds (`repair_workspace`, `recover_workspace`, `resume_run`, `rebuild_index`, `retry_unavailable_sources`, `check_due_sources`, `distill_changed_sources`, `review_insights`, `first_run_commit`, `review_git_changes`) and `future_kind`.
- [x] "Nothing needs attention." state when no action is pending.
- [x] Workspace health card displaying Health, Index, and Git status badges.
- [x] Summary overview card displaying counts, masked as "Unavailable" when health/index is degraded.
- [x] Action categories card displaying chips with availability tooltips (using only the 3 canonical enum values).

### Skills Catalog Screen (Mockup lines 142–178)
- [x] Controls bar with client-side search input, lifecycle select, and dynamic collection select.
- [x] URL query synchronization for `q`, `state`, and `collection`.
- [x] Primary action buttons: `Add from GitHub` (`/skills/add`) and `Create skill` (`/skills/create`).
- [x] Fallback generation info banner shown when backend `summary` references fallback index.
- [x] Empty state for brand-new workspaces: "No skills yet." with CTA to Add from GitHub.
- [x] Filtered empty state: "No skills match these filters." with "Clear filters" CTA.
- [x] Skills table displaying Name, ID in mono, Collection, Lifecycle chip, and Routable status.
- [x] Responsive layout: full table on desktop/tablet, card list on mobile.
- [x] Row action menu (`⋯`):
  - `Review` (links to `/skills/:id`)
  - `Deprecate` for active skills (links to `/skills/:id?tab=review`)
  - `Archive` for deprecated skills (links to `/skills/:id?tab=review`)

## 6. Deviations and Failure Protocol Events

1. **Dependency Compatibility Selection:**
   - *Issue:* `npm view typescript version` and `npm view eslint version` returned bleeding-edge major versions (TS 7.0.2, ESLint 10.12.0) that are incompatible with current community plugins (`typescript-eslint` 8 explicitly rejects TS 7; `eslint-plugin-react` rejects ESLint 10).
   - *Resolution:* Pinned `typescript@5.9.3`, `eslint@9.39.5`, and `@eslint/js@9.39.5`, resulting in clean peer resolution and passing typecheck/lint.
2. **Vitest Config Typing:**
   - *Issue:* Importing `defineConfig` from `'vite'` caused TS2769 because `test` property is only typed in `vitest/config`.
   - *Resolution:* Imported `defineConfig` from `'vitest/config'` in `vite.config.ts`.
3. **Smoke Test Workspace Isolation:**
   - *Issue:* In `startServer()`, creating `smoke.md` directly inside the temporary workspace root triggered canonical layout validation rejection during `skill create`.
   - *Resolution:* Created `smoke.md` in `os.tmpdir()` outside the workspace root and deleted it after `skill create` completed.
4. **Smoke Test Execution Timeout:**
   - *Issue:* 8 full-page navigations combined with 8 full `AxeBuilder` audit scans exceeded Playwright's default 30-second test timeout.
   - *Resolution:* Set `test.setTimeout(120_000)` in `smoke.spec.ts`.
5. **Axe WCAG Color Contrast Improvements:**
   - *Issue:* Light mode default theme (`precision`) exhibited minor contrast ratios under 4.5:1 on table headers, warning chips, and subtle text:
     - `.fg-nav__item` active state had inline color override; resolved by using `.fg-nav__item--on` class where text is `var(--color-text)` (10:1).
     - `.fg-table thead th`: changed from `var(--color-text-subtle)` to `var(--color-text-muted)`.
     - `--color-warning`: adjusted light scheme in `precision.css` from `#8a6410` to `#805c08` (yielding 5.08:1 on `#f4e9cf`).
     - Skill ID subtext in `SkillsScreen.tsx`: changed from `var(--color-text-subtle)` to `var(--color-text-muted)`.
   - *Resolution:* Recorded all changes in `web/src/design-system/PATCHES.md`. Full Axe audit passed with 0 serious or critical violations across all 8 configurations.

## 7. Open Questions

None. Phase 2 passed all quality gates, lint, unit tests, e2e tests, and the guard check.
