---
phase: 1
title: "Harness, fixtures and rubric"
status: completed
priority: P1
effort: "4h"
dependencies: []
---

# Phase 1: Harness, fixtures and rubric

## Goal
Make the evaluation repeatable: one command builds a hub clone, serves the web UI, captures every flow at desktop and mobile width, and prints a scorecard skeleton.

## Files to Create / Modify
- Create: `web/e2e/ux/capture.spec.ts` (screenshots + text dump + axe per flow, no assertions on looks)
- Create: `web/e2e/ux/flows.ts` (flow list in the Tier order with route, setup, and the task a user should finish)
- Create: `web/e2e/ux/seed-states.ts` (extra states: upstream update, conflict, outdated skill, strict-invalid skill)
- Create: `docs/design/web-ux-rubric.md` (the rubric below, one page)
- Modify: `web/e2e/support/seed-workspace.ts` only to export the extra states

## Rubric (scored 0 fail, 1 weak, 2 good, per flow)

| Item | Question |
|---|---|
| Task success | Can a first-time user finish the flow from this screen alone? |
| Next step | Is the next action obvious and the primary action visible without scrolling at 1280 and 390 px? |
| Wording | Would the user understand every label without reading a doc? Any internal word (digest, proposal, canonical, served)? |
| State | Are empty, loading, error and disabled states explained, with a way forward? |
| Safety | Is the preview vs apply boundary clear? Does it say what will change before it changes? |
| Copyable | Can every command shown be read and copied whole? |
| Mobile | Is nothing clipped or overlapping at 390 px? |
| Access | Does axe report no serious or critical issue? Is it keyboard reachable? |

## Tasks & Steps
1. Ask the user to confirm or reorder the Tier table in `plan.md` (usage is not recorded; the order is reasoned).
2. Write `flows.ts` with every row of the Tier table: route, preconditions, and the one-sentence task.
3. Write `seed-states.ts` using the CLI only (no direct file edits) on a temp workspace.
4. Write `capture.spec.ts`: for each flow and each width, save `<flow>-<width>.png`, `<flow>.txt`, axe JSON under the Playwright output dir (gitignored).
5. Add `make web-ux` that builds the binary, runs only the `ux` project, and prints the output dir.
6. Write the rubric doc; add a scorecard template (`flow | 8 scores | notes | defect ids`).
7. Run once on the clone of the live hub; commit the harness, not the screenshots.

## Verification
- `make web-ux` finishes and lists one screenshot pair and one text dump per flow.
- `git status` shows no change in `/home/vantt/skill-hub` (clone only).
- `make web-check` and `make web-e2e` stay green.
