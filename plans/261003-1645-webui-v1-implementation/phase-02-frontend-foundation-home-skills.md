---
phase: 2
title: "Frontend foundation, Home, Skills catalog"
status: pending
priority: P1
effort: 38h
dependencies: [1]
---

<!-- Updated: Validation Session 1 - rewritten as an executor handover; exact Makefile targets, workflow edits, font procedure, required test files -->

# Phase 2: Frontend foundation, Home, Skills catalog

## Goal

A `web/` React + TypeScript + Vite project, built into `internal/delivery/web/dist` and embedded in the binary, delivering the app shell, Appearance menu, Home and Skills catalog against the real adapter, with CI and release producing the embedded UI.

## Before you start

- Read [plan.md](./plan.md) "Executor hard rules" and [decisions.md](./decisions.md) D1, D3, D4, D5, D6, D8, D9, D10, D12.
- Behavior authority: `docs/use-cases/04-webui-user-flows-and-screen-specs.md` §1, §2.1, §2.2, §5, §7, §8. Copy authority: `docs/use-cases/05-webui-design-brief.md` §5. Visual authority: `docs/design/webui-mockup/Skill Hub WebUI.dc.html` (shell lines 31–99, Appearance menu 40–76, Home 100–141, Skills 142–178, theme list constant `THEMES` from line 616, breakpoints at line 724: mobile `<768`, tablet `<1280`, desktop otherwise).
- The mockup's markup is a template for a React class component: `<sc-if value="{{ x }}">` becomes `{x && ...}`, `<sc-for list="{{ xs }}" as="x">` becomes `xs.map(...)`, `{{ x }}` becomes a prop or state value, `onClick="{{ f }}"` becomes `onClick={f}`. Keep the `.fg-*` class names and token-based inline styles. Do not copy the mockup's sample data into production code; data comes from the API.
- Phase 1 golden files in `internal/delivery/web/testdata/golden/` are the only API fixtures. Synthetic bodies in tests are allowed only for states the goldens do not cover, and must be shaped like the matching golden file.

## Tasks

### Task 2.1 — Pin dependency versions
- Goal: exact versions recorded before scaffolding.
- Target files: the phase report (draft).
- Steps:
  1. For each package run `npm view <name> version` and record name and version in a table: `react`, `react-dom`, `react-router`, `@tanstack/react-query`, `typescript`, `vite`, `@vitejs/plugin-react`, `vitest`, `jsdom`, `@testing-library/react`, `@testing-library/user-event`, `@testing-library/jest-dom`, `eslint`, `@eslint/js`, `typescript-eslint`, `eslint-plugin-react`, `eslint-plugin-react-hooks`, `globals`, `@playwright/test`, `@axe-core/playwright`, `@types/react`, `@types/react-dom`, `@types/node`.
- Success criteria: every package has a version.
- Verify: no verification needed (task 2.2 installs them).

### Task 2.2 — Scaffold `web/` and the Go guard
- Goal: an empty app that builds into the embed directory without disturbing Go tooling.
- Target files: create `web/go.mod`, `web/.nvmrc`, `web/package.json`, `web/package-lock.json` (generated), `web/tsconfig.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/test/setup.ts`, `web/eslint.config.js`; modify `Makefile`.
- Steps:
  1. `web/go.mod` content: the line `module github.com/vantt/mcp-skill-hub/web`, a blank line, and the line `go 1.26.0`. It contains no Go code; its only purpose is to keep `web/` (including `node_modules`) out of the root module's `./...`.
  2. `web/.nvmrc` content: `24`.
  3. `package.json`: `"private": true`, `"type": "module"`, exact versions from task 2.1 (no `^` or `~`), scripts `"dev": "vite"`, `"build": "tsc --noEmit && vite build"`, `"typecheck": "tsc --noEmit"`, `"lint": "eslint ."`, `"test": "vitest run"`, `"e2e": "playwright test"`. Run `npm install` once inside `web/` to create the lockfile.
  4. `tsconfig.json`: `strict: true`, `noUncheckedIndexedAccess: true`, `jsx: "react-jsx"`, `moduleResolution: "bundler"`, `types: ["vite/client", "vitest/globals", "node"]`, include `src`, `e2e`, `vite.config.ts`, `playwright.config.ts`.
  5. `vite.config.ts`: React plugin; `build.outDir` = `path.resolve(__dirname, '../internal/delivery/web/dist')`, `build.emptyOutDir: false`, `build.assetsDir: 'assets'`; `server: { host: '127.0.0.1', port: 5421, strictPort: true, proxy: { '/api': { target: 'http://127.0.0.1:7421', changeOrigin: false } } }`; `test: { environment: 'jsdom', globals: true, setupFiles: 'src/test/setup.ts', include: ['src/**/*.test.{ts,tsx}'] }`.
  6. `index.html`: `<div id="root"></div>` and `<script type="module" src="/src/main.tsx"></script>`; no inline script or style element (CSP forbids inline scripts).
  7. `eslint.config.js`: `@eslint/js` recommended, `typescript-eslint` recommended, `eslint-plugin-react` with rule `'react/jsx-no-literals': ['error', { noStrings: true, ignoreProps: true, allowedStrings: ['·', '/', '—', '→', '✕', '⧉', '▾', '◆', '▲', '✎', '?'] }]`, `react-hooks` recommended; ignore `dist`, `node_modules`, `test-results`, `playwright-report`, `.e2e`.
  8. Makefile: append these targets and add them to `.PHONY` (recipe lines start with a tab):
```make
web-install:
	cd web && npm ci

web-build: web-install
	find internal/delivery/web/dist -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
	cd web && npm run build

web-test:
	cd web && npm run typecheck && npm run lint && npm test

## web-check: frontend typecheck, lint, unit tests and build
web-check: web-install web-test web-build

## web-e2e: build the UI and binary, then run Playwright against the real server
web-e2e: web-build
	go build -o web/.e2e/skillhub ./cmd/skillhub
	cd web && npx playwright install chromium && npm run e2e

web-dev:
	bash scripts/web-dev.sh
```
- Success criteria: the empty app builds into the embed directory; Go tooling ignores `web/`.
- Verify (all must hold): `make web-build` exits 0; `test -f internal/delivery/web/dist/index.html` exits 0; `go vet ./...` exits 0; `go list ./... | grep -cE 'mcp-skill-hub/web(/|$)'` prints `0`; `grep -cE '"[~^][0-9]' web/package.json` prints `0`.

### Task 2.3 — Dev workflow script
- Goal: one command that runs API and Vite, refuses busy ports, and leaves no process behind.
- Target files: create `scripts/web-dev.sh`.
- Steps:
  1. `set -euo pipefail`. For each port in `7421 5421`: if `command -v ss` succeeds, test with `ss -ltn "sport = :$port" | grep -q LISTEN`; else test with `lsof -iTCP:"$port" -sTCP:LISTEN -t`. If busy, print `Port $port is already in use by:` followed by `ss -ltnp "sport = :$port"` (or `lsof -iTCP:$port -sTCP:LISTEN`), then `Stop that process and retry. This script never picks another port.` and exit 1.
  2. `go build -o web/.e2e/skillhub-dev ./cmd/skillhub`, then start `web/.e2e/skillhub-dev serve web --dev --no-open "$@" &`, store `API_PID=$!`, and `trap 'kill "$API_PID" 2>/dev/null || true' EXIT INT TERM`.
  3. `cd web && npm run dev` in the foreground.
- Success criteria: the script exits with an error when a port is busy.
- Verify: `bash -n scripts/web-dev.sh` exits 0; then start `python3 -m http.server 7421 --bind 127.0.0.1 &`, record its PID, run `bash scripts/web-dev.sh`, kill the recorded PID. Pass: the script exited 1 and printed `Port 7421 is already in use`.

### Task 2.4 — Vendor the design system and self-host fonts
- Goal: the fgDesign System CSS available to the app, with no CDN font requests.
- Target files: create `web/src/design-system/` (copied files), `web/src/design-system/PATCHES.md`, `web/src/design-system/fonts/<theme>.ts` for the six themes; modify `web/package.json`.
- Steps:
  1. Copy `styles.css`, `contract/` and `themes/` from `docs/design/webui-mockup/_ds/fgdesign-system-3658085c-54db-48c0-9ea6-9159ec1d267e/` into `web/src/design-system/` unchanged.
  2. In `web/src/design-system/themes/*.css`, delete every line that starts with `@import url('https://fonts.googleapis.com`. List each deleted line, per file, in `PATCHES.md`, with one sentence: "Fonts are self-hosted by the app; see fonts/<theme>.ts."
  3. Families per theme (from the deleted lines): precision: Fraunces, Newsreader, Geist, Geist Mono, Space Grotesk. berich: Be Vietnam Pro, Spectral, IBM Plex Mono, Plus Jakarta Sans. clickup: Plus Jakarta Sans, Inter, Sometype Mono, Space Grotesk. terminal: JetBrains Mono, IBM Plex Mono, Sometype Mono. atelier: Manrope, Space Grotesk. moday: Figtree, Poppins.
  4. For each family, slug = lowercase with spaces replaced by `-` (for example `be-vietnam-pro`). Run `npm view @fontsource/<slug> version`; if that fails run `npm view @fontsource-variable/<slug> version`. Install the first that exists with an exact version. If neither exists, install nothing for that family and record in `PATCHES.md` that it falls back to the next family in the theme's CSS stack.
  5. In `fonts/<theme>.ts`, import only the subset files `latin`, `latin-ext` and `vietnamese` for the weights named in the deleted `@import` line (static packages: `@fontsource/<slug>/<subset>-<weight>.css`; variable packages: `@fontsource-variable/<slug>/<subset>-wght.css`). If a subset file does not exist in the installed package (check with `ls web/node_modules/@fontsource*/<slug>/`), skip it and record the gap in `PATCHES.md`.
- Success criteria: no Google Fonts reference remains in the app.
- Verify: `grep -rl "fonts.googleapis" web/src | wc -l` prints `0`; `cd web && npm run typecheck` exits 0.

### Task 2.5 — Local storage layer
- Goal: one safe, scoped, versioned storage API (D10).
- Target files: create `web/src/state/local-store.ts`, `web/src/state/local-store.test.ts`.
- Steps:
  1. Export `PREFIX = 'skillhub.web.v1'`, `scopedKey(workspaceId: string, name: string)` returning `` `${PREFIX}.${workspaceId}.${name}` ``, and `globalKey(name)` returning `` `${PREFIX}.${name}` ``.
  2. `writeJSON(key, value, now = Date.now()): boolean` stores `{ v: 1, savedAt: now, value }`; returns `false` (never throws) when storage is unavailable, the quota is exceeded, or the serialized string exceeds 1,000,000 characters.
  3. `readJSON<T>(key, now = Date.now()): T | undefined` returns `undefined` (never throws) on missing key, parse error, wrong `v`, or age over 30 days (and removes expired entries).
  4. `remove(key)` and `purgeExpired(now)` (removes expired entries under `PREFIX` only).
  5. Tests: round trip; scoping keeps two workspace IDs apart; expiry at 30 days plus 1 ms; corrupt JSON returns `undefined`; oversize write returns `false`; a `localStorage` whose methods throw makes every function return its safe value; `purgeExpired` leaves keys outside `PREFIX` untouched.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/state/local-store.test.ts` exits 0.

### Task 2.6 — i18n catalog
- Goal: every visible string goes through a typed catalog.
- Target files: create `web/src/i18n/en.ts`, `web/src/i18n/index.ts`, `web/src/i18n/index.test.ts`.
- Steps:
  1. `en.ts`: `export const en = { ... } as const` with dotted keys; copy wording from brief 05 §5 for navigation, status labels, CTAs and empty states.
  2. `index.ts`: `type MessageKey = keyof typeof en`; `useT()` returns `t(key, vars?)` that replaces `{name}` placeholders; plural keys use suffixes `.one` and `.other` chosen with `new Intl.PluralRules('en').select(count)`; a missing key returns the key itself.
  3. Tests: placeholder replacement, plural selection for 1 and 2, missing-key behavior.
- Success criteria: tests pass; lint forbids literal JSX text.
- Verify: `cd web && npx vitest run src/i18n/index.test.ts` exits 0; `cd web && npm run lint` exits 0.

### Task 2.7 — Session and API client
- Goal: authenticated calls and typed errors.
- Target files: create `web/src/state/session.ts`, `web/src/api/client.ts`, `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/test/golden.ts`, `web/src/api/client.test.ts`.
- Steps:
  1. `session.ts`: `captureTokenFromHash()` reads `#token=<hex>` from `location.hash`, stores it in `sessionStorage` key `skillhub.web.token`, and calls `history.replaceState(null, '', location.pathname + location.search)`; `getToken()`; an `EventTarget` that emits `session-expired`.
  2. `client.ts`: `apiFetch<T>(path, init?)` prefixes `/api/v1`, sets `Authorization: Bearer <token>` and, for bodies, `Content-Type: application/json`; on a non-2xx response parses the JSON envelope and throws `ApiError { status, code, render: { ERROR, WHY, FIX }, retryable, correlationId? }`; on 401 also emits `session-expired`.
  3. `types.ts`: types for `/session`, `/home`, `/skills`, `/skills/{id}`, `/skills/{id}/review`, written from the golden files and spec 04 §1.3.
  4. `queries.ts`: TanStack Query hooks `useSession`, `useHome`, `useSkills`, `useSkillDetail`, `useSkillReview` with query keys `['session']`, `['home']`, `['skills']`, `['skill', id]`, `['skill-review', id]`.
  5. `test/golden.ts`: `loadGolden(name)` reads `internal/delivery/web/testdata/golden/<name>.json` with `node:fs`, resolving the path from `import.meta.url` (three levels up from `web/src/test/`, then into `internal/...`).
  6. Tests: the token header is sent; a 404 with `skill-unknown.json` throws `ApiError` with status 404 and code `invalid_request`; a 401 emits `session-expired`; `captureTokenFromHash` stores the token and strips the hash.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/api/client.test.ts` exits 0.

### Task 2.8 — App shell, navigation, Appearance menu, routes
- Goal: the mockup shell with all routes registered.
- Target files: create `web/src/routes.tsx`, `web/src/state/appearance.ts`, `web/src/components/AppShell.tsx`, `web/src/components/NavRail.tsx`, `web/src/components/AppearanceMenu.tsx`, and `web/src/components/` `Banner.tsx`, `CommandBlock.tsx`, `StatusBadge.tsx`, `Skeleton.tsx`, `EmptyState.tsx`, `CopyButton.tsx`, `LaterPhasePage.tsx`, `SessionExpired.tsx`, `NotFoundPage.tsx`; modify `web/src/main.tsx`, `web/src/App.tsx`.
- Steps:
  1. `main.tsx`: call `captureTokenFromHash()`, import `./design-system/styles.css` and `./design-system/fonts/precision`, add `fg-root` to `document.body`, render `<App/>` inside `QueryClientProvider` and the router.
  2. `appearance.ts`: global key `appearance`; value `{ scheme: 'light' | 'dark' | 'system', theme, accent }`; default `{ scheme: 'system', theme: 'precision', accent: <the theme's first accent> }`; apply `data-theme`, `data-scheme` (resolve `system` with `matchMedia('(prefers-color-scheme: dark)')` and follow its changes) and `data-accent` on `document.documentElement`; load a theme's fonts through a static map `{ precision: () => import('../design-system/fonts/precision'), ... }`. Theme names, notes and accent lists come from the mockup's `THEMES` constant.
  3. Shell per mockup lines 31–99: header with breadcrumb and `h1`; left rail on desktop, collapsed rail on tablet, bottom navigation on mobile; Inbox count from `home_summary.pending_insights` only when `workspace.health === 'valid' && workspace.index === 'current'`; workspace health summary in the rail footer; skip link to `#main`. No help (`?`) button (D12).
  4. On route change set `document.title` to `<page title> · Skill Hub` and move focus to the `h1`.
  5. Routes: `/` Home, `/skills` Skills; `/skills/add`, `/skills/create`, `/skills/:id`, `/sources`, `/sources/watch`, `/sources/distill`, `/sources/runs/:id`, `/inbox`, `/inbox/:id`, `/inbox/:id/apply` render `LaterPhasePage` ("This screen arrives in a later delivery phase."); unknown paths render `NotFoundPage`; the `session-expired` event renders `SessionExpired`.
- Success criteria: build passes; navigation is covered by task 2.11.
- Verify: `cd web && npm run typecheck && npm run lint` exits 0.

### Task 2.9 — Home screen
- Goal: spec 04 §2.1 Home with every action kind and the degraded state.
- Target files: create `web/src/screens/home/HomeScreen.tsx`, `web/src/screens/home/action-cta.ts`, `web/src/screens/home/home.test.tsx`.
- Steps:
  1. Port mockup lines 100–141.
  2. `action-cta.ts` maps each `kind` to its CTA as in spec 04 §2.1: `repair_workspace` and `recover_workspace` → copy-command `skillhub doctor --fix`; `resume_run` → link `/sources/runs/<actions[].id>`, plus the "and N other runs" note when `count > 1`; `rebuild_index` → copy-command `skillhub rebuild`; `retry_unavailable_sources` and `check_due_sources` → `/sources`; `distill_changed_sources` → `/sources?filter=ready`; `review_insights` → `/inbox`; `first_run_commit` → guidance with a copy-command; `review_git_changes` → copy-command `git status`; any other kind → summary text only, no CTA.
  3. Counts render only when `workspace.health === 'valid' && workspace.index === 'current'`; otherwise each count reads "Unavailable".
  4. Category availability text uses only the three enum values.
  5. Tests: golden `home.json` renders; one synthetic body per kind (the ten kinds plus `future_kind`) asserts the CTA; `health: invalid` and `index: stale` show "Unavailable" and no digit in the summary list.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/screens/home/home.test.tsx` exits 0.

### Task 2.10 — Skills catalog screen
- Goal: spec 04 §2.2 catalog.
- Target files: create `web/src/screens/skills/SkillsScreen.tsx`, `web/src/screens/skills/skills.test.tsx`.
- Steps:
  1. Port mockup lines 142–178: table on tablet and desktop, cards on mobile.
  2. Search by `id` and `name` (client side); lifecycle filter All, Draft, Active, Deprecated, Archived; collection filter built from result values; all three synced to the URL query (`q`, `state`, `collection`).
  3. Row menu: Review (to `/skills/:id`); Deprecate for `active` and Archive for `deprecated`, both linking to `/skills/:id?tab=review` in this phase. Primary actions `Add from GitHub` (`/skills/add`) and `Create skill` (`/skills/create`).
  4. If the response `summary` mentions a fallback generation, show the summary verbatim in an info banner (no further parsing).
  5. Tests (golden `skills.json` plus synthetic lists): search narrows rows; filters change the URL; empty list shows "No skills yet."; filtered-empty shows "No skills match these filters." with Clear filters; row menu items per lifecycle state.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/screens/skills/skills.test.tsx` exits 0.

### Task 2.11 — Playwright smoke and accessibility
- Goal: the real binary serves Home and Skills with no non-loopback requests and no serious axe violations.
- Target files: create `web/playwright.config.ts`, `web/e2e/support/server.ts`, `web/e2e/smoke.spec.ts`.
- Steps:
  1. `server.ts`: `startServer()` creates a temp dir, runs `web/.e2e/skillhub init <ws> --yes`, writes a Markdown file and runs `web/.e2e/skillhub skill create smoke-skill --collection core --name "Smoke Skill" --description "Smoke test skill." --content-file <file> --yes` with the environment variable `SKILLHUB_WORKSPACE=<ws>` (skill subcommands resolve the workspace from that variable; do not pass `--workspace` to them), spawns `web/.e2e/skillhub serve web --addr 127.0.0.1:0 --no-open --workspace <ws>`, reads stdout until a line matches `http://127.0.0.1:(\d+)/#token=([0-9a-f]+)`, and returns `{ url, origin, stop() }`, where `stop()` sends SIGTERM and awaits exit. Each spec file starts and stops its own server (`test.beforeAll` / `test.afterAll`); no fixed port is used.
  2. `playwright.config.ts`: Chromium only, `fullyParallel: false`, `retries: 0`, reporter `list`.
  3. `smoke.spec.ts`: open `url`; the Home `h1` is visible; navigate to Skills; the `smoke-skill` row is visible; type in search and see the URL query change; switch the scheme to dark in the Appearance menu, reload, and the scheme is still dark; record every request URL and assert each starts with `origin`; run `AxeBuilder` on `/` and `/skills` at widths 1280 and 390 in light and dark and assert no violation with impact `serious` or `critical`.
- Success criteria: e2e passes locally.
- Verify: `make web-e2e` exits 0, and its output contains ` passed` and does not contain ` failed`.

### Task 2.12 — CI and release workflows
- Goal: CI tests the frontend and release builds embed the real UI.
- Target files: modify `.github/workflows/ci.yml`, `.github/workflows/release.yml`.
- Steps:
  1. Pin `actions/setup-node` by full commit SHA with a version comment, like the other actions: `gh api repos/actions/setup-node/releases/latest --jq .tag_name`, then `gh api repos/actions/setup-node/git/ref/tags/<tag> --jq '.object.sha,.object.type'`; if the type is `tag`, dereference with `gh api repos/actions/setup-node/git/tags/<sha> --jq .object.sha`. Reuse the existing pinned SHAs for `actions/checkout`, `actions/setup-go`, `actions/upload-artifact` (`ea165f8d65b6e75b540449e92b4886f43607fa02 # v4.6.2`) and `actions/download-artifact` (`634f93cb2916e3fdff6788551b99b062d0335ce0 # v5.0.0`).
  2. `ci.yml`: add job `web` (`runs-on: ubuntu-latest`): checkout, setup-go `1.26.0`, setup-node with `node-version-file: web/.nvmrc`, `make web-check`, `npx --prefix web playwright install --with-deps chromium`, `make web-e2e`, then upload `internal/delivery/web/dist` as artifact `web-dist`. Change job `cross-compile` to `needs: web` and add a `download-artifact` step for `web-dist` into `internal/delivery/web/dist` before its `go build`.
  3. `release.yml`: add job `web` (`needs: prepare`) that checks out, sets up Node and Go, runs `make web-build` and uploads `web-dist`. Add `web` to the `needs:` list of job `build` (currently `prepare` and `verify`) and add a `download-artifact` step into `internal/delivery/web/dist` before the `go build` step (near line 267).
- Success criteria: both workflows are valid YAML and use the artifact.
- Verify: `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/ci.yml .github/workflows/release.yml` exits 0; `grep -c 'web-dist' .github/workflows/ci.yml` prints a number ≥ 2; `grep -c 'web-dist' .github/workflows/release.yml` prints a number ≥ 2.

### Task 2.13 — Phase close
- Goal: prove completion and record measurements.
- Target files: create `plans/261003-1645-webui-v1-implementation/reports/phase-02-report.md` and `plans/261003-1645-webui-v1-implementation/reports/screenshots/phase-02/*.png`.
- Steps:
  1. Record: the version table from task 2.1; `du -sh internal/delivery/web/dist`; the binary size with the built UI minus the size with only `.gitkeep` in `dist` (two `go build -o` runs into a temp directory); the total size of `.woff2` files in `dist`.
  2. Save screenshots of `/` and `/skills` at widths 1440, 1024 and 390 in light and dark, named `<route>-<width>-<scheme>.png`. They are for later review, not a gate.
  3. Write the mockup-parity checklist: each element in mockup lines 31–178 is marked done, or dropped with the reason (no field in spec 04 §1.3).
  4. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 2`, paste its full output into the report, and commit.
- Success criteria: guard passes.
- Verify: the guard's last line is exactly `GUARD RESULT: PASS (phase 2)`.

## Progress

- [ ] Task 2.1 — Pin dependency versions
- [ ] Task 2.2 — Scaffold `web/` and the Go guard
- [ ] Task 2.3 — Dev workflow script
- [ ] Task 2.4 — Vendor the design system and self-host fonts
- [ ] Task 2.5 — Local storage layer
- [ ] Task 2.6 — i18n catalog
- [ ] Task 2.7 — Session and API client
- [ ] Task 2.8 — App shell, navigation, Appearance menu, routes
- [ ] Task 2.9 — Home screen
- [ ] Task 2.10 — Skills catalog screen
- [ ] Task 2.11 — Playwright smoke and accessibility
- [ ] Task 2.12 — CI and release workflows
- [ ] Task 2.13 — Phase close

## Failure Protocol
If any Verify step does not meet its stated pass condition:
1. You may make **one** fix attempt for that task. Change only the task's target files. Never edit a test's assertions to make it pass, never edit golden files by hand, never touch `plans/261003-1645-webui-v1-implementation/guard/`, `.golangci.yml`, or CI files unless the task lists them.
2. Re-run exactly the same Verify command.
3. If it still fails, STOP this phase. Do not try a second fix and do not reason around the failure.
4. If a `kongming` subagent can be spawned, give it: the phase and task id, the steps you ran, both Verify commands with their full output, and the pass condition. Apply its guidance, then re-run Verify once.
5. Otherwise, or if Verify still fails, report the same evidence to the user and wait.
Record every failure, the fix attempt and the outcome in the phase report.

## Rollback

Revert the phase's commits. Phase 1 keeps working and serves the "not built" page.
