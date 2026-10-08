---
phase: 6
title: "Hardening, docs, release"
status: superseded
priority: P1
effort: 28h
dependencies: [5]
---

<!-- Updated: Validation Session 1 - rewritten as an executor handover; npm ci without --ignore-scripts; exact doc edits and greps -->
> **Moved 2026-10-05:** tasks 6.1–6.4 and 6.6 to `plans/261004-2234-skill-source-upstream-ux/phase-12-webui-hardening-and-release.md`; task 6.5 (documentation) to `plans/261004-2234-skill-source-upstream-ux/phase-13-documentation.md`. Do not execute this file.


# Phase 6: Hardening, docs, release

## Goal

End-to-end and accessibility coverage of every user flow, Vietnamese glyph coverage, third-party notices, documentation that matches shipped behavior, and a release build that embeds and serves the UI.

## Before you start

- Read [plan.md](./plan.md) "Executor hard rules" and [decisions.md](./decisions.md) D3, D5, D7, D9, D11, D14.
- Read in full before editing each: `docs/design/01-system-architecture.md` (lines 125, 482, 555 say the Web UI is not in V1), `docs/design/05-curation-lifecycle.md` (line 5), `docs/contracts/error-codes.md` (the sentence "adds no web UI"), `docs/PRD.md` (lines 272 and 1433), `docs/use-cases/04-webui-user-flows-and-screen-specs.md` (§0 item 8, §6), `README.md` (line 62 build instructions), `docs/user-guide.md`, `docs/release-runbook.md`, `.github/workflows/release.yml` (job `smoke` at line 599).
- Documentation rule: change only what the shipped behavior makes false; link to schemas and golden files instead of copying them; verify each changed claim against code or a live run.

## Tasks

### Task 6.1 — Full user-flow journeys
- Goal: every flow of spec 04 §3 passes end to end.
- Target files: create `web/e2e/journeys.spec.ts`.
- Steps:
  1. Cover in one file, each as its own test: review and activate a draft (§3.1); create (§3.3); edit with conflict (§3.4); watch-check-handoff-run (§3.5, using the phase 4 seed); decide and apply an insight (§3.6); deprecate and archive (§3.7). Add (§3.2) is covered up to the invalid-URL and clone-failure states, because the web endpoint accepts only GitHub and CI must not depend on the network.
  2. Each test asserts that no request leaves the server origin.
- Success criteria: e2e passes.
- Verify: `make web-e2e` exits 0, and its output contains ` passed` and does not contain ` failed`.

### Task 6.2 — Accessibility and responsive sweep
- Goal: no serious or critical axe violations anywhere; no horizontal page scroll.
- Target files: create `web/e2e/a11y.spec.ts`.
- Steps:
  1. For every route (use seeded IDs for parameterized routes), at widths 360, 768, 1280 and 1440, in light and dark, run `AxeBuilder` and assert no violation with impact `serious` or `critical`; assert `document.documentElement.scrollWidth <= window.innerWidth`.
  2. Open the Proposal Preview, Conflict Drawer and Confirm dialog once each and run axe with the dialog open.
  3. Emulate `prefers-reduced-motion: reduce` and `forced-colors: active` on Home and Skill Detail and run axe again.
  4. Fix findings in the owning screen or component.
- Success criteria: e2e passes.
- Verify: `make web-e2e` exits 0, and its output contains ` passed` and does not contain ` failed`.

### Task 6.3 — Vietnamese glyph coverage
- Goal: know which theme faces render Vietnamese, with a fallback for the rest.
- Target files: modify `web/src/design-system/PATCHES.md`, and `web/src/design-system/fonts/<theme>.ts` only if a fallback is needed.
- Steps:
  1. In a Playwright script (add it as a test in `web/e2e/a11y.spec.ts`), for each theme render the string `Kỹ năng đã được kích hoạt — Ưu tiên cập nhật` in each of the theme's four font slots and use `document.fonts.check('16px "<family>"', text)` to record coverage.
  2. For each family without coverage, add a note in `PATCHES.md` and make sure the CSS stack falls back to a system font for that slot.
- Success criteria: a coverage table in `PATCHES.md`.
- Verify: `grep -c 'Vietnamese coverage' web/src/design-system/PATCHES.md` prints a number ≥ 1.

### Task 6.4 — Third-party notices and dependency audit
- Goal: shipped npm and font licenses are listed; audit results are recorded.
- Target files: create `web/scripts/notices.mjs`, `web/THIRD_PARTY_NOTICES.md`; modify `web/package.json` (script `"notices": "node scripts/notices.mjs"`), `.github/workflows/release.yml` (attach the file to the release assets).
- Steps:
  1. `notices.mjs` reads `web/package-lock.json`, takes every package reachable from `dependencies` (not `devDependencies`), reads each package's `package.json` `license` field and its `LICENSE*` file from `node_modules`, and writes `THIRD_PARTY_NOTICES.md` with one section per package (name, version, license, license text).
  2. Run it and commit the output.
  3. Run `cd web && npm audit --omit=dev` and paste the result in the phase report (do not change dependencies to silence it; report it).
- Success criteria: the notices file lists every runtime dependency.
- Verify: `cd web && npm run notices` exits 0; `grep -c '^## ' web/THIRD_PARTY_NOTICES.md` prints a number ≥ 5.

### Task 6.5 — Documentation
- Goal: no document contradicts the shipped WebUI.
- Target files: modify `docs/design/01-system-architecture.md`, `docs/design/05-curation-lifecycle.md`, `docs/contracts/error-codes.md`, `docs/PRD.md`, `docs/use-cases/04-webui-user-flows-and-screen-specs.md`, `README.md`, `docs/user-guide.md`, `docs/release-runbook.md`; create `docs/contracts/web-api.md`.
- Steps:
  1. Architecture (Vietnamese text): line 125, replace the sentence "Web UI không thuộc V1." with a sentence saying the WebUI ships as the delivery adapter `internal/delivery/web` (command `skillhub serve web`) and calls the same application services; line 482 in the Defaults list, replace "- Không Web UI trong V1." with "- Web UI local qua `skillhub serve web`, chỉ chạy khi người dùng khởi động."; line 555 in the out-of-scope list, delete the line "- Web UI trong V1."; mention `internal/delivery/web` wherever the module table names the delivery adapters.
  2. Curation lifecycle line 5: remove Web UI from the "not included" list.
  3. Error codes: replace "adds no web UI" with a sentence linking `docs/contracts/web-api.md` and saying the web adapter uses the same envelope plus HTTP status.
  4. PRD: mark the Simple Web UI as delivered by `skillhub serve web`, without rewriting unrelated text.
  5. Spec 04: in §0 item 8, state that the adapter exists (`internal/delivery/web`); in §6, state that the UI shows Not-found when a read endpoint returns HTTP 404, while the body code stays `invalid_request`; tick §9 items with links to evidence in the phase reports.
  6. `docs/contracts/web-api.md` (short): base path `/api/v1`, authentication (token in URL fragment, Bearer header), Host and Origin rules, the listen rule (D14), the endpoint list with the app method each calls, the status table (link to `internal/delivery/web/errors.go`), and links to `schemas/` and `internal/delivery/web/testdata/golden/`.
  7. README: a "Web UI" section with `skillhub serve web`, the token URL, the listen rule including its effect on machines with Docker or VPN addresses, `--loopback-only`, `--allow-host`, the plain-HTTP warning, and that source builds need `make web-build` before `go build`.
  8. User guide: the same in task-oriented form, plus the Windows firewall prompt on a wildcard bind.
  9. Release runbook: the `web` job, the artifact, the notices file, and the extended smoke test.
- Success criteria: every guard docs check passes.
- Verify: `grep -cE 'Web UI không thuộc V1|Không Web UI trong V1|^- Web UI trong V1\.$' docs/design/01-system-architecture.md` prints `0`; `grep -c 'adds no web UI' docs/contracts/error-codes.md` prints `0`; `grep -c 'skillhub serve web' README.md` prints a number ≥ 1; `grep -c -- '--loopback-only' docs/user-guide.md` prints a number ≥ 1.

### Task 6.6 — Release smoke test
- Goal: a published binary demonstrably serves the UI with authentication.
- Target files: modify `.github/workflows/release.yml` (job `smoke`).
- Steps:
  1. In the Unix smoke steps after install, add: start `skillhub serve web --loopback-only --no-open --addr 127.0.0.1:0` in the background with a temp workspace created by `skillhub init <dir> --yes`, read the URL from its output, `curl -fsS -H "Host: 127.0.0.1:<port>" -H "Authorization: Bearer <token>" http://127.0.0.1:<port>/api/v1/session` must succeed, the same call without the header must return 401, `curl -fsS -H 'Accept: text/html' http://127.0.0.1:<port>/` must contain `id="root"`, then stop the server.
  2. Add the equivalent PowerShell steps to the Windows smoke job.
- Success criteria: valid YAML containing the new steps.
- Verify: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"` exits 0; `grep -c 'serve web' .github/workflows/release.yml` prints a number ≥ 2.

### Task 6.7 — Phase and plan close
- Goal: final evidence.
- Target files: create `plans/261003-1645-webui-v1-implementation/reports/phase-06-report.md`.
- Steps:
  1. Run `make check` and `make web-check`.
  2. Record, on this machine: `ip -o -4 addr show up` output, the address chosen by `skillhub serve web --no-open` (first startup line), a request through one non-loopback address with the token (200) and the same with header `Host: evil.example:7421` (421).
  3. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 6`, paste its full output, commit.
  4. Mark the plan status `completed` with the plan CLI (`ak plan --help` for the exact command) only after the guard passes.
- Success criteria: guard passes; all phase reports exist.
- Verify: `make check` exits 0; `make web-check` exits 0; the guard's last line is exactly `GUARD RESULT: PASS (phase 6)`.

## Progress

- [ ] Task 6.1 — Full user-flow journeys
- [ ] Task 6.2 — Accessibility and responsive sweep
- [ ] Task 6.3 — Vietnamese glyph coverage
- [ ] Task 6.4 — Third-party notices and dependency audit
- [ ] Task 6.5 — Documentation
- [ ] Task 6.6 — Release smoke test
- [ ] Task 6.7 — Phase and plan close

## Failure Protocol
If any Verify step does not meet its stated pass condition:
1. You may make **one** fix attempt for that task. Change only the task's target files. Never edit a test's assertions to make it pass, never edit golden files by hand, never touch `plans/261003-1645-webui-v1-implementation/guard/`, `.golangci.yml`, or CI files unless the task lists them.
2. Re-run exactly the same Verify command.
3. If it still fails, STOP this phase. Do not try a second fix and do not reason around the failure.
4. If a `kongming` subagent can be spawned, give it: the phase and task id, the steps you ran, both Verify commands with their full output, and the pass condition. Apply its guidance, then re-run Verify once.
5. Otherwise, or if Verify still fails, report the same evidence to the user and wait.
Record every failure, the fix attempt and the outcome in the phase report.

## Rollback

Docs and release changes are separate commits; revert them individually.
