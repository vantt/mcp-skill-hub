---
phase: 6
title: "Screenshots for README and user guide"
status: pending
priority: P2
effort: "0.5d"
dependencies: [2, 3, 4]
---

# Phase 6: Screenshots for README and user guide

## Goal
Collect a small, consistent set of web UI screenshots from a clean demo hub, so `README.md` (section "Web UI") and `docs/user-guide.md` can show the real screens without leaking private data.

## Why after the fixes
Screenshots of Tier 1 to Tier 3 screens taken before Phases 2 to 4 would show truncated commands, a broken Inbox and dead sections, and would go stale at once. Phase 6 runs after them and reuses the Phase 1 capture harness.

## Files to Create / Modify
- Create: `web/e2e/ux/shots.spec.ts` (deterministic capture of the shot list below; reuses `flows.ts` and `seed-states.ts`)
- Create: `web/e2e/ux/demo-hub.ts` (makes a demo hub from the CLI only: 4 to 5 invented skills in different states, one upstream update, one source; fixed dates)
- Create: `docs/images/web/*.png` (final images, at most 200 KB each) and `docs/images/web/manifest.json` (file, route, state, width, theme, caption, alt text)
- Modify: `Makefile` (`web-shots`: compile the binary, run `shots.spec.ts`, write to `docs/images/web/`)
- Modify: `README.md` section "Web UI" (2 to 3 images) and `docs/user-guide.md` (one image per task, with the steps next to it)

## Shot list (in the order of the Tier table; each row is one user-guide task)

| # | Image | State to show | Used in |
|---|---|---|---|
| 1 | `home` | Home with one real next action and healthy status | README, guide |
| 2 | `skills-list` | List with a draft, an active and a deprecated skill; filters labelled | README, guide |
| 3 | `skill-review` | Review tab of an active skill: trust, validity, readiness | README, guide |
| 4 | `skill-review-draft` | Draft with "missing activation requirements" and the Go to field links | guide |
| 5 | `skill-activate-confirm` | Confirm dialog before activating | guide |
| 6 | `editor-preview` | Editor tab with a proposal preview diff | guide |
| 7 | `editor-conflict` | Conflict drawer after an outside edit | guide |
| 8 | `create-skill` | Create form with the hints from Phase 3 | guide |
| 9 | `add-from-github-discover` and `add-from-github-review` | Both steps of the add flow, with a placeholder URL | guide |
| 10 | `upstream-updates` | List filtered to upstream updates and the update preview | guide |
| 11 | `sources` | Sources list with one source and a check result | guide |
| 12 | `distill-handoff` | Distill handoff brief with a selected source | guide |
| 13 | `mobile-skills` | Skills list at 390 px | README |
| 14 | `dark-skills-list` | Same screen in dark theme | README (optional) |

Terminal steps (approve content, `skillhub connect`, `skillhub status`) stay as text code blocks, not images, so they can be copied and do not go stale.

## Rules for the images
- **Demo data only.** Never capture the live hub or a clone of it. Skill names, sources and URLs are invented (for example `release-checklist`, `acme/agent-skills`).
- **No private values.** The session token, the home path, host names and emails never appear. The harness replaces the workspace path with `~/skill-hub` in the page before capture and fails if `token`, `/home/` or `@` is found in the page text.
- **Stable output.** Fixed viewport (1280 x 800 desktop, 390 x 844 mobile), fixed fonts, animations off, reduced motion, a fixed clock for dates, device scale factor 2 then downscaled, so rerunning gives near-identical files.
- **Light theme by default, one dark image.** Crop to the content when the empty area is large; no browser chrome.
- **Size budget.** PNG optimized, 200 KB each at most, about 2 MB for the whole set, so the repo stays light. If the set grows, move it to a release asset and link it.
- **Alt text and captions** come from `manifest.json`, so the README, the guide and the images stay in step.

## Tasks & Steps
1. Write `demo-hub.ts` and check by hand that no live-hub value appears in it.
2. Write `shots.spec.ts` from the shot list; add the private-value check and the fixed clock.
3. Add `make web-shots`; run it and review every image: readable at README width, no clipped text, nothing private.
4. Write `manifest.json` with caption and alt text for each image.
5. Add the images and steps to `docs/user-guide.md`, one task per section, wording taken from the Phase 2 to 4 scorecards (plain words, no internal terms).
6. Update the README "Web UI" section with the home, review and mobile images and a link to the guide.
7. Note in `docs/release-runbook.md` that `make web-shots` is rerun before each release when the web UI changed.

## Verification
- `make web-shots` runs twice and produces the same set (file list identical, pixel difference negligible).
- The private-value check passes, and fails when a token is put on the page on purpose.
- Every image is at most 200 KB; the README and guide links resolve; `manifest.json` lists every file.
- A reader who has never run the product can follow the guide with the images alone for the five top flows.
