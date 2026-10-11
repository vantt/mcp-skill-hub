---
phase: 11
title: "Fix the Home count copy and refresh home.png"
status: pending
priority: P2
effort: "1.5h"
wave: 1
dependencies: [1]
---

# Phase 11: Fix the Home count copy and refresh home.png

## Context (traced)

- Home shows the top action title as `help?.title ?? topAction.summary`
  (`web/src/screens/home/HomeScreen.tsx:58-59,94`). `actionHelp(kind)` only gives titles for a few kinds
  (`web/src/screens/home/action-cta.ts:108-132`), so count-based kinds print the Go summary verbatim.
- The Go server builds those summaries with "(s)": `retry_unavailable_sources` "%d source(s) are temporarily
  unavailable", `review_upstream_updates` "%d skill(s) have upstream changes to review",
  `distill_changed_sources` "%d source(s) are ready to distill", `check_due_sources` "%d source(s) are due for
  an update check", and the lessons summary "%d candidate lesson(s) ready for review ..."
  (`internal/app/curation_home.go:445-480`). Each action carries `count`.
- The web app already has plural-aware i18n: `t(key, {count})` picks `<key>.one` or `<key>.other` with
  `Intl.PluralRules` (`web/src/i18n/index.ts:6-14`; example keys `home.unreachable_sources.one/other` in
  `web/src/i18n/en.ts:98-99`).

Decision: fix the rendering in the web app only. The Go summaries are shared with `skillhub status` and MCP output
and have golden tests; changing them is a separate follow-up (plan.md, follow-up 8).

## Owned files

- Modify: `web/src/screens/home/action-cta.ts` (a count-aware title for the five count kinds),
  `web/src/screens/home/HomeScreen.tsx` (use it), `web/src/i18n/en.ts` (new `.one`/`.other` keys),
  `web/src/screens/home/home.test.tsx` (tests).
- Regenerate: `docs/images/web/home.png`, and the `home.png` entry of `docs/images/web/manifest.json`.
- Read-only: `internal/app/*`, `web/e2e/*`, every other screenshot.

## Requirements

- Strings (English, sentence case, no "(s)"):
  - `distill_changed_sources`: "1 source is ready to distill" / "{count} sources are ready to distill".
  - `check_due_sources`: "1 source is due for an update check" / "{count} sources are due for an update check".
  - `retry_unavailable_sources`: "1 source is temporarily unavailable" / "{count} sources are temporarily
    unavailable".
  - `review_upstream_updates`: "1 skill has upstream changes to review" / "{count} skills have upstream changes
    to review".
  - `review_lessons`: "1 candidate lesson is ready for review" / "{count} candidate lessons are ready for
    review" (the Go text's mention of a script name stays in the CLI only).
- When `count` is missing or not a positive number, fall back to the current behavior (`help?.title ??
  summary`), so unknown or future kinds keep working.
- Keys follow the existing naming (`home.action.<kind>.one|other`); `actionHelp` titles keep priority for the
  kinds that have one.
- Skills: try `/ak:frontend-development` for the React change; if the Skill tool is unavailable, follow the
  existing i18n and `action-cta.ts` patterns and say so.

## Tests (in `home.test.tsx`)

- For each of the five kinds: count 1 renders the singular sentence; count 3 renders the plural sentence; the
  text "(s)" never appears.
- Missing count falls back to the summary.
- A kind with an `actionHelp` title still shows that title.
- Existing tests stay green unchanged.

## Screenshot refresh

The harness (`web/e2e/ux/shots.spec.ts`) captures the whole shot list in one test and first deletes every PNG and
`manifest.json` in its output folder (`shots.spec.ts:251-257`); it cannot shoot one image. So: shoot all into a
scratch folder and copy only `home.png`.

```bash
cd /home/vantt/projects/mcp-skill-hub-docs
make web-shots SHOTS_OUT_DIR=/tmp/skillhub-docs-rewrite/shots
```

Then copy `/tmp/skillhub-docs-rewrite/shots/home.png` over `docs/images/web/home.png`, and replace only the
`home.png` object in `docs/images/web/manifest.json` with the one from the scratch manifest (width, height,
caption and alt should be unchanged; if the height changed, the new value goes in). Leave the other images as
they are: they did not change and re-committing them would add byte churn. Check the new `home.png` is <= 300 KB.

`make web-shots` runs `make web-build` and `go build -o web/.e2e/skillhub` inside the worktree; no other phase
builds during wave 1 (Phase 1 built earlier into `/tmp`). Never install into `~/.local/bin`.

## Steps

1. Implement the strings, the helper and the HomeScreen call.
2. `make web-test` (typecheck, lint, vitest) green.
3. `make web-shots SHOTS_OUT_DIR=/tmp/skillhub-docs-rewrite/shots`; copy `home.png` and its manifest entry.
4. Look at the new `home.png`: the next action reads "1 source is ready to distill".
5. Commit the four source files and the two image files together (`fix(web): use singular and plural counts in
   the Home next action`).

## Validation

- `make web-test` passes; the new tests fail on the old code (check by reading or a quick stash-free revert in a
  scratch copy, not in the shared worktree).
- `git diff --stat docs/images/web/` shows only `home.png` and `manifest.json`.
- The lead looks at the new `home.png`.

## Risks

- The demo hub state changes and the top action is no longer `distill_changed_sources`: then the screenshot shows
  another action; report it, do not edit `web/e2e/ux/demo-hub.ts` (read-only here).
- The harness fails on a private-value guard: report and stop; keep the old `home.png` (Phase 17 then removes
  `home.png` from `web-ui.md`).

## Rollback

Revert the single commit (code, test and image together).
