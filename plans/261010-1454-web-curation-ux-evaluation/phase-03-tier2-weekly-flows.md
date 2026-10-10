---
phase: 3
title: "Tier 2: weekly flows, test and fix"
status: pending
priority: P2
effort: "1d"
dependencies: [2]
---

# Phase 3: Tier 2, weekly flows

## Goal
Score and fix the set-up and change flows: Editor tab (preview and confirm), Create skill, Add from GitHub, upstream updates.

## Files to Create / Modify
- Modify: `web/src/**` for the Editor tab, ProposalPreview, ConflictDrawer, Create form, Add flow
- Modify: `internal/delivery/web/routes_skill_write.go` only for missing error detail
- Modify: `docs/design/web-ux-scorecard.md` (Tier 2 rows)

## Flows and what to check
1. **Editor:** change a routing field, see a preview diff in plain words, confirm, then see the result. Conflict (someone edited meanwhile): is the drawer understandable and does it offer a way forward?
2. **Create:** explain "Not for / rationale" and "Min scope" with one-line hints and an example; make Preview draft visible without scrolling; say that creating makes a draft that is not routed yet.
3. **Add from GitHub:** the sample URL must not look like a real value (use a placeholder style); say what "Discover" does and that nothing is written yet; "Advanced" tells what it holds; the Review step shows conflicts and trust state before confirm.
4. **Upstream updates:** the list filter `?upstream=updates` is reachable from Skills without typing a URL; the update preview shows what changed upstream and what local edits would be kept.

## Tasks & Steps
1. Seed the states in `seed-states.ts` (conflict, upstream update) and run the harness on Tier 2.
2. Score, list defects, fix by severity, one commit each.
3. Add E2E assertions for the new hints and for reaching the upstream-updates list from the UI.

## Verification
- No 0 on Task success, Wording or Safety for Tier 2.
- A new user can create a draft skill from the form without opening the CLI docs (checked by running the flow with only on-screen text).
- `make web-check`, `make web-e2e`, `make web-ux` green.
