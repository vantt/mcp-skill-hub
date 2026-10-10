---
title: "Web curation UX: test, score and fix, most-used flows first"
description: "Evaluate how easy and understandable the skill-curation web UI is, with repeatable screenshot and task runs on a hub clone, ordered by how often each feature is used; fix what the evaluation finds."
status: pending
priority: P1
effort: 3d
branch: main
tags: [web, ux, curation, e2e, accessibility]
blockedBy: []
blocks: []
created: 2026-10-10
---

# Web curation UX: test, score and fix, most-used flows first

## Overview

The web UI is where the user curates skills (review, edit, activate, add, source learning). Nobody has
judged it as a user would since the hub model was simplified (Git-native, `.meta/`, no runs or
insights). A first look on 2026-10-10 (screenshots of a clone of the live hub at 1280 px and 390 px,
installed binary `c84abe7`) already shows dead screens and unclear wording. This plan turns that look
into a repeatable evaluation and a fix list, and tests the screens in order of how often they are used.

Out of scope: new features, the CLI, the MCP server, visual rebranding. Changes stay in `web/` and
`internal/delivery/web`, plus small wording fixes in `internal/app` result text if a screen shows it.

Start after the CI-fix branches are merged (the web E2E branch `4d7cce1` rewrites `web/e2e/` and
deletes the specs for removed inbox, insights and runs routes). This plan builds on that suite.

## How the evaluation runs

- **Fixture:** a `git clone` of the live hub into the scratchpad with isolated `HOME` and `XDG_*`, then
  `skillhub rebuild` and `skillhub serve web --addr 127.0.0.1:<port> --no-open`. The live hub is never
  written. Extra states (upstream update, conflict, outdated skill, strict-invalid skill) come from a
  seed script, not from the live hub.
- **Capture:** Playwright (already in `web/node_modules`, chromium cached) takes full-page screenshots
  at 1280 and 390 px, dumps visible text, and runs axe. The agent reads the screenshots.
- **Judging:** each flow gets the rubric in Phase 1. A flow passes only if a first-time user can finish
  the task without leaving the page to read docs.
- **Honest limit:** an agent reading screenshots finds broken, truncated, unclear and inconsistent
  screens well. It cannot say what feels pleasant. Phase 5 therefore ends with a short walk-through by
  the user on the top five flows.

## Test order, by how often a curator uses each feature

Assumption: the web UI records no usage, so the order below is reasoned from the curation workflow
(daily loop first, set-up tasks later). The user can reorder it in Phase 1 before any test runs.

| Tier | Frequency | Flow | Screen | Seen on 2026-10-10 |
|---|---|---|---|---|
| 1 | every visit | Open the hub, see what needs me | Home: next action, status | Next action is only "host integration missing"; cards repeat the same counts twice |
| 1 | every visit | Find a skill | Skills list: search, 3 filters, table | Three unlabeled "All" selects; on mobile the table clips the Create button |
| 1 | every visit | Check one skill: can agents use it | Skill detail, Review tab: trust, validity, readiness | "Approve content" command is cut off (`skillhub skill ec`); Provenance text overlaps |
| 1 | daily | Activate, deprecate, archive | Skill detail header actions, confirm dialog | Draft shows a disabled Activate with the reason split across two places |
| 2 | weekly | Fix a skill's routing fields | Editor tab, preview, confirm, conflict drawer | Not yet captured |
| 2 | weekly | Create a skill | Create form | "Not for / rationale" is unclear; Preview draft sits under the fold |
| 2 | weekly | Add skills from GitHub | Add flow: discover, review, confirm | Example URL looks like a real value; "Advanced" is closed with no hint |
| 2 | weekly | Act on upstream updates | Skills list `?upstream=updates`, update preview | Not yet captured |
| 3 | monthly | Link and check learning sources | Sources list, check, ready filter | Empty state is fine, but the page still shows "Open a run" and "Recent runs" for removed runs |
| 3 | monthly | Hand a distill job to the curator agent | Distill with Curator Agent | Dead end when no source is selected; brief text still says `curation_run_start` |
| 3 | monthly | Triage lessons | Inbox (nav badge 39) | Error "Unknown API path" while the badge shows 39 |
| 3 | rare | Resources, Usage, Runtime, Sources tabs on a skill | Skill detail tabs | Not yet captured |
| 3 | rare | Theme and workspace status | Header menu, sidebar | Not yet captured |

## Known defects before testing (fix list seed)

| ID | Severity | Defect | Evidence |
|---|---|---|---|
| D1 | High | Inbox page fails with "Unknown API path" and the nav badge still shows 39 | `d2-inbox.png`; the API route was removed in simplify Phase 3 |
| D2 | High | "Approve content" command is truncated, so the main trust action cannot be copied by reading it | `desktop-skill-draft.png` |
| D3 | Medium | Home "pending insights 39" and category chips repeat the same numbers; no insights concept exists any more | `desktop-home.png` |
| D4 | Medium | Sources page keeps "Open a run" and "Recent runs on this browser" | `d2-sources2.png` |
| D5 | Medium | Handoff brief tells the agent to call `curation_run_start` | `web/src/domain/handoff-brief.ts` (reported by the E2E agent) |
| D6 | Medium | Mobile skills table clips the "Create skill" button and the Routing column | `mobile-skills.png` |
| D7 | Low | Provenance fields overlap on narrow cards | `desktop-skill-draft.png` |
| D8 | Low | Three filters are all labelled "All" with no field name | `desktop-skills.png` |

## Phases

| # | Phase | Status | Priority |
|---|---|---|---|
| 1 | [Harness, fixtures and rubric](./phase-01-harness-and-rubric.md) | Pending | P1 |
| 2 | [Tier 1: daily flows, test and fix](./phase-02-tier1-daily-flows.md) | Pending | P1 |
| 3 | [Tier 2: weekly flows, test and fix](./phase-03-tier2-weekly-flows.md) | Pending | P2 |
| 4 | [Tier 3: occasional flows, dead screens](./phase-04-tier3-occasional-flows.md) | Pending | P2 |
| 5 | [Regression guard and user walk-through](./phase-05-regression-and-walkthrough.md) | Pending | P2 |

Phases run in order so the most-used flows are fixed first. Phase 2 can ship alone.

## Success Criteria

- [ ] Every flow in the test-order table has a recorded score for each rubric item, with screenshots at 1280 and 390 px.
- [ ] D1 and D2 are fixed and no Tier 1 or Tier 2 screen shows a raw API error, a truncated command or a dead end.
- [ ] No screen mentions runs, insights or `curation_run_start`.
- [ ] axe reports no serious or critical issue on any screen; the mobile layout has no clipped primary action.
- [ ] The UX checks run in `make web-e2e` and fail when a dead route or truncated copy command comes back.
- [ ] The user walked through the top five flows and signed off, or listed what is still wrong.

## Risks

| Risk | Mitigation |
|---|---|
| Frequency order is a guess | Phase 1 asks the user to confirm or reorder before testing; add a cheap local click counter only if the user wants real data |
| Screenshot review misses feel and speed | Phase 5 user walk-through; timings from Playwright for slow screens only |
| Fixes break E2E | Run `make web-check` and `make web-e2e` per phase; keep one commit per defect |
| Tests touch the live hub | Fixture is a clone with isolated HOME and XDG; never point a run at `/home/vantt/skill-hub` |
| Web and CLI wording drift | Wording that comes from `internal/app` is fixed there once, not patched in the UI |

## Rules for every phase

- Do not write to `/home/vantt/skill-hub`; do not push without asking.
- Never remove telemetry event types or change `EventVersion`.
- Tests read only the repo or a temp dir; fixtures live in `testdata`.
- Conventional commits; `make check` and `make web-check` green before a merge.
- Do not edit `.claude/skills/distill-lab`.
