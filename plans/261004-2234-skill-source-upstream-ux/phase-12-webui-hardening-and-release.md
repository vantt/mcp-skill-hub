---
title: "Phase 12: WebUI hardening and release"
status: in-progress
---

# Phase 12: WebUI hardening and release

<!-- Moved from plans/261003-1645-webui-v1-implementation phase 6 (tasks 6.1–6.4, 6.6) on 2026-10-05; its documentation task moved to phase 13 -->

## Context

- Plan: [plan.md](./plan.md). Depends on phases 9–11 (every WebUI screen is shipped).
- Origin: the closed WebUI plan's phase 6 (`plans/261003-1645-webui-v1-implementation/phase-06-hardening-docs-release.md`), adapted to the screens actually shipped after the runtime plan and this plan: Skill Detail has Usage, Runtime, and Sources tabs; `/sources` is the grouped view; there is no Watch page.
- Read first: `web/e2e/support/{server,seed-workspace}.ts`, `web/e2e/*.spec.ts`, `web/src/routes.tsx`, `web/src/design-system/PATCHES.md`, `web/src/design-system/fonts/`, `web/package.json`, `web/package-lock.json`, `.github/workflows/release.yml` (job `web` near line 187, job `smoke` near line 635, Unix and Windows steps), `docs/use-cases/04-webui-user-flows-and-screen-specs.md` §3.
- Facts verified on 2026-10-05: no `web/THIRD_PARTY_NOTICES.md`, no `web/scripts/`, no `Vietnamese coverage` table in `PATCHES.md`, and `release.yml` has a `web` build job but no `serve web` smoke step.

## Overview

End-to-end and accessibility coverage of every shipped user flow, Vietnamese glyph coverage, third-party notices, and a release smoke test proving the published binary serves the UI with authentication.

## Requirements

1. **Journeys** `web/e2e/journeys.spec.ts`, one test per spec 04 §3 flow as shipped: review and activate a draft (§3.1, including the Runtime tab and the CLI approve command shown for third-party content); create (§3.3); edit with conflict (§3.4); sources: open `/sources`, `Check now` on `source-c`, select it (the seed's only distillable source), hand off, open the finalized run (§3.5, using `seedDistillWorkspace()` and `startServer({workspace})` from phase 10); decide and apply an insight (§3.6); deprecate and archive (§3.7). Add (§3.2) and upstream updates are covered up to the invalid-URL and clone-failure states, because the web add endpoint accepts only GitHub and CI must not depend on the network. Every test asserts that no request leaves the server origin.
2. **Accessibility and responsive sweep** `web/e2e/a11y.spec.ts`: every route in `routes.tsx` (seeded IDs for parameterized routes, including `?tab=usage`, `?tab=runtime`, `?tab=sources` on Skill Detail), at widths 360, 768, 1280, 1440, light and dark: no axe violation of impact `serious` or `critical`, and `document.documentElement.scrollWidth <= window.innerWidth`. Run axe once with `ProposalPreview`, `ConflictDrawer`, and `ConfirmDialog` open. Emulate `prefers-reduced-motion: reduce` and `forced-colors: active` on Home and Skill Detail. Fix findings in the owning screen or component.
3. **Vietnamese glyph coverage**: a test in `a11y.spec.ts` renders `Kỹ năng đã được kích hoạt — Ưu tiên cập nhật` in each theme's four font slots and records `document.fonts.check('16px "<family>"', text)`; `PATCHES.md` gains a `Vietnamese coverage` table, and each family without coverage falls back to a system font in its CSS stack (`web/src/design-system/fonts/<theme>.ts` only when a fallback is missing).
4. **Third-party notices**: `web/scripts/notices.mjs` reads `web/package-lock.json`, takes every package reachable from `dependencies` (not `devDependencies`), and writes `web/THIRD_PARTY_NOTICES.md` with one `## <name>@<version>` section per package (license field and `LICENSE*` text from `node_modules`); `web/package.json` gains `"notices": "node scripts/notices.mjs"`; the release workflow places the file in the `release/` directory that the `publish` job uploads with `gh release create ... release/*` (generate it in the `web` job and pass it through that job's existing artifact). `npm audit --omit=dev` output goes in the phase report; dependencies are not changed to silence it.
5. **Release smoke**: in the `smoke` job's Unix step (`Test literal one-liner install, PATH, upgrade, and uninstall (Linux & macOS)`), after the install and PATH checks and before the uninstall section, start `skillhub serve web --loopback-only --no-open --addr 127.0.0.1:0` in the background on a workspace made by `skillhub init <dir> --yes`, read the URL from its output, then: `GET /api/v1/session` with `Host: 127.0.0.1:<port>` and `Authorization: Bearer <token>` succeeds; the same without the header returns 401; `GET /` with `Accept: text/html` contains `id="root"`; stop the server. Add the equivalent PowerShell steps at the same point of the Windows step.
6. Hardening evidence on this machine, recorded in the phase report: `ip -o -4 addr show up`, the address chosen by `skillhub serve web --no-open` (first startup line), a request through one non-loopback address with the token (200) and the same with header `Host: evil.example:7421` (421).

## Related code files

Create: `web/e2e/journeys.spec.ts`, `web/e2e/a11y.spec.ts`, `web/scripts/notices.mjs`, `web/THIRD_PARTY_NOTICES.md` (generated).

Modify: `web/package.json` (script only), `web/src/design-system/PATCHES.md`, `web/src/design-system/fonts/<theme>.ts` (fallback only, if needed), `.github/workflows/release.yml` (notices asset and smoke steps), and the owning screen or component of each accessibility finding (list every such file in the phase report).

Do not modify any other file.

## Implementation steps

### Task 12.1 — Journeys
- Steps: Requirement 1.
- Verify: `make web-e2e` exits 0, its output contains ` passed` and not ` failed`.

### Task 12.2 — Accessibility sweep
- Steps: Requirement 2.
- Verify: `make web-e2e` exits 0, its output contains ` passed` and not ` failed`; `make web-check` exits 0.

### Task 12.3 — Glyph coverage
- Steps: Requirement 3.
- Verify: `grep -c 'Vietnamese coverage' web/src/design-system/PATCHES.md` prints a number ≥ 1.

### Task 12.4 — Notices
- Steps: Requirement 4.
- Verify: `cd web && npm run notices` exits 0; `grep -c '^## ' web/THIRD_PARTY_NOTICES.md` prints a number ≥ 5.

### Task 12.5 — Release smoke
- Steps: Requirement 5.
- Verify: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"` exits 0; `grep -c 'serve web' .github/workflows/release.yml` prints a number ≥ 2; `grep -c 'THIRD_PARTY_NOTICES' .github/workflows/release.yml` prints a number ≥ 1.

### Task 12.6 — Evidence and gate
- Steps: Requirement 6.
- Verify: `make check` exits 0 and `make web-check` exits 0; the phase report contains the four evidence items.

## Todo

- [x] Task 12.1 journeys
- [x] Task 12.2 a11y sweep
- [x] Task 12.3 glyph coverage
- [x] Task 12.4 notices
- [x] Task 12.5 release smoke
- [ ] Task 12.6 evidence + gates

## Success criteria

Every shipped flow passes end to end on the real binary, no route has serious or critical axe violations at any tested width or theme, notices ship with the release, and the release smoke proves an installed binary serves the authenticated UI.

## UX acceptance

No horizontal page scroll at 360 px on any route; dialogs pass axe while open; reduced motion and forced colors keep every control visible.

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Accessibility fixes spill into many screens | M×M | Fix only in the owning file; list each file in the report. |
| Release smoke flaky on Windows runners | M×L | Read the URL from output instead of a fixed port; stop the server in a `finally`-equivalent step. |

## Security considerations

Journeys assert no cross-origin requests. The release smoke checks that an unauthenticated request is refused (401) and the hardening evidence checks Host validation (421).

## Rollback

Test, notices, and workflow changes are separate commits; revert them individually.

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
