---
phase: 2
title: "Tier 1: daily flows, test and fix"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Tier 1, daily flows

## Goal
Score and fix the screens a curator sees on every visit: Home, Skills list, skill Review tab, and activate / deprecate / archive.

## Files to Create / Modify
- Modify: `web/src/**` for Home, Skills list, skill detail Review tab, confirm dialog (find with `grep -rn` on the screen titles)
- Modify: `internal/delivery/web/routes_read.go` only if a screen needs data the API does not give
- Modify: `internal/app/**` result text only where the wording itself is the problem
- Create: `docs/design/web-ux-scorecard.md` (Tier 1 rows filled)

## Flows and what to check
1. **Home:** is there one clear next action? Is "host integration missing" explained (what it is, how to fix, why it matters)? Remove duplicate counts (D3).
2. **Skills list:** label each filter (state, collection, routing) (D8); search finds by id and name; empty and loading states; mobile table does not clip the primary button (D6).
3. **Skill Review tab:** every command is fully readable and copyable (D2): wrap or show in a block with a copy button that copies the full command, not a truncated one; plain explanation of "content trust", "validity", "activation readiness"; fix overlapping Provenance (D7).
4. **Activate / deprecate / archive:** the disabled Activate says once, next to the button, what is missing and links to the field; the confirm dialog says what changes.

## Tasks & Steps
1. Run the harness on Tier 1 flows; fill the scorecard with screenshots at both widths.
2. List defects with severity; fix High first (D2), then the rest, one commit per defect.
3. Re-run the harness; attach before/after screenshots to the scorecard.
4. Add Vitest cases for wording or layout logic that can be tested without a browser.

## Verification
- Scorecard: no 0 on Task success, Next step or Copyable for Tier 1.
- `make web-check`, `make web-e2e`, `make web-ux` green; axe has no serious issue on these screens.
- Mobile 390 px: the Create button and every table column are reachable.
