---
phase: 4
title: "Tier 3: occasional flows and dead screens"
status: completed
priority: P2
effort: "0.5d"
dependencies: [2]
---

# Phase 4: Tier 3, occasional flows

## Goal
Remove dead and misleading screens left by the simplified model, and check the rarely used ones.

## Files to Create / Modify
- Modify: `web/src/**` nav, Home chips, Sources page, Distill page, Inbox route; `web/src/domain/handoff-brief.ts`
- Modify: `internal/delivery/web/routes_distill.go` if the badge count comes from a removed concept
- Modify: `docs/design/web-ux-scorecard.md` (Tier 3 rows)

## Tasks & Steps
1. **Inbox (D1):** decide with the user: if lessons are meant to be triaged in the web UI, build a minimal list from `.meta/distill.yaml` candidates (read-only, with the CLI command to decide); otherwise remove the nav item, route and badge. No page may show "Unknown API path".
2. **Sources (D4):** remove "Open a run" and "Recent runs"; keep list, check, add; empty state says what a source is and why to add one.
3. **Distill handoff (D5):** with no source selected, offer the sources to pick on that page instead of a dead end; fix the brief text to the current distill flow (no `curation_run_start`; use the real CLI or MCP step) and update the E2E assertion that checks the old text.
4. **Other tabs:** Resources, Usage, Runtime, Sources tabs and the theme menu: check wording, empty states and mobile; list defects.
5. Grep the web source and tests for `run`, `insight`, `inbox`, `curation_run` and remove stale strings.

## Verification
- `grep -rniE "insight|curation_run|Recent runs" web/src web/e2e` returns nothing, apart from deliberate lesson wording.
- Every nav item opens a working page; no route returns a raw API error.
- `make web-check`, `make web-e2e`, `make web-ux` green.
