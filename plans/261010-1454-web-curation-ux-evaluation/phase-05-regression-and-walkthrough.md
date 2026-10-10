---
phase: 5
title: "Regression guard and user walk-through"
status: pending
priority: P2
effort: "0.5d"
dependencies: [2, 3, 4]
---

# Phase 5: Regression guard and user walk-through

## Goal
Keep the fixes from coming back and get the user's own judgement on the flows that matter most.

## Files to Create / Modify
- Modify: `web/e2e/a11y.spec.ts` (cover every nav item and every Tier 1 and 2 route at 390 px)
- Create: `web/e2e/ux/guards.spec.ts` (no raw API error text on any route; every copy button copies the full command; no horizontal scroll at 390 px)
- Modify: `docs/design/web-ux-scorecard.md` (final scores, before and after)
- Modify: `.github/workflows/ci.yml` only if `web-ux` guards need their own step

## Tasks & Steps
1. Write the guards; confirm each one fails when the old defect is put back (revert a fix locally and see it go red).
2. Re-run the whole harness and publish the final scorecard.
3. Prepare a one-page walk-through script for the top five flows: Home, find a skill, review one skill, edit routing, add from GitHub.
4. Ask the user to run it on their own hub (read-only steps only, or on a clone) and record what is still unclear; open follow-up items in `docs/plans/2026-10-10-observation-backlog.md` for anything not fixed.
5. When all success criteria in `plan.md` are met, archive this plan with `ak plan archive`.

## Verification
- Guards fail on a reintroduced defect and pass on main.
- Final scorecard has no 0 in Tier 1 or 2, and the user's walk-through notes are recorded.
- `make check`, `make web-check`, `make web-e2e` green on CI for ubuntu, macOS and Windows.
