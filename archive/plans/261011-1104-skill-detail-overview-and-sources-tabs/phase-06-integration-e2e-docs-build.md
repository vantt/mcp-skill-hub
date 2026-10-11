---
phase: 6
title: "Integration: e2e, docs, build"
status: completed
priority: P1
effort: "2.5h"
dependencies: [2, 3, 4, 5]
---

# Phase 6: Integration: e2e, docs, build

## Context

The Go server embeds the web build: `make web-build` writes `internal/delivery/web/dist`, then
`go build ./cmd/skillhub`; `make web-e2e` builds `web/.e2e/skillhub` and runs Playwright
(`Makefile:46-59`, `web/e2e/support/server.ts:23`). Another agent may be rebuilding binaries: coordinate with
the lead before building, and never install into `~/.local/bin`.

Specs that assume the Review tab opens by default or assert the old Sources copy:

- `web/e2e/tier2.spec.ts:108`: opens `/skills/smoke-skill` and expects "Go to field" from the Review tab.
- `web/e2e/smoke.spec.ts:72-76`: expects "No learning references yet. …" on `?tab=sources`.
- `web/e2e/ux/flows.ts:53,62,71`: Review-tab flows routed with `skillPath(id)` (no tab).
- `web/e2e/ux/shots.spec.ts:76,83`: `skill-review` and `skill-review-draft` screenshots routed with no tab.
- `web/e2e/a11y.spec.ts:69`: route list containing `/skills/consumer-review?tab=sources`.
- `docs/user-guide.md:262`: "Open a skill; the **Review** tab opens first."

`web/e2e/support/seed-workspace.ts` exports `makeCLIRunner` and `attachFilesystemSource`. Do not change that
shared file: the `/sources` empty-state specs rely on the default seed.

## Requirements

- New `web/e2e/skill-sources.spec.ts` on its own temp workspace (`init`, `skill create`):
  1. Write an unattached filesystem source record into `sources/catalog/` (same YAML as `attachFilesystemSource`), and add an upstream `sources[]` entry with `id: ""`, `kind: github` and a GitHub `repo` to the skill's `.meta/skill.yaml`.
  2. Open `/skills/<id>`: Overview is selected and lists the upstream as "Not tracked".
  3. Sources tab: Check now is disabled; Track upstream shows a preview whose diff adds `sources/catalog/<id>.yaml` and changes `.meta/skill.yaml`; confirm; the badge is no longer "Not tracked". Backfill uses no network.
  4. Add source -> Link a watched source -> Preview shows `sources/skills/LINK-…yaml` -> confirm; the learning row appears.
  5. Back on Overview the new learning reference is listed.
  6. All requests stay loopback-only (same assertion as `smoke.spec.ts:77-82`).
- Pin the listed specs to `?tab=review` where they need Review; update the smoke empty-state text to the Phase 4 wording; add `/skills/consumer-review?tab=overview` next to the sources route in `a11y.spec.ts`; add one Overview entry to `ux/flows.ts` in the style of the existing ones.
- `docs/user-guide.md`: Overview opens first (what it shows), Review is one click away; a short paragraph on the Sources tab: add a learning reference (link a watched source or watch a GitHub repository, preview, confirm) and Track upstream for a skill copied from GitHub that is not tracked yet. Do not regenerate `docs/images/web/`.

## Files

- Create: `web/e2e/skill-sources.spec.ts`
- Modify: `web/e2e/smoke.spec.ts`, `web/e2e/tier2.spec.ts`, `web/e2e/a11y.spec.ts`, `web/e2e/ux/flows.ts`, `web/e2e/ux/shots.spec.ts`, `docs/user-guide.md`

## Steps

1. Merge Phases 2 to 5; confirm with the lead that no other build is running.
2. Edit the listed specs and write the new spec.
3. `make check` and `make web-check`.
4. `make web-e2e` (builds `web/.e2e/skillhub` from the merged tree).
5. Manual check on a scratch clone of the live hub with isolated `HOME`/`XDG_*` (never the hub itself): `herdr-cook-plan` shows its GitHub upstream as Not tracked on Overview and Sources, Track upstream previews a diff that touches only its upstream entry (plus the new catalog file) and applies in the clone; `markdown-to-epub` shows the empty state; screens at 1280 and 390 px.
6. Update the docs.

## Validation

- `make check`, `make web-check`, `make web-e2e` all green.
- `go build ./cmd/skillhub` succeeds after `make web-build`.
- The new spec fails on the pre-change build (upstream hidden, no Add source or Track upstream button) and passes after.

## Risks

- Hidden default-tab dependencies in other specs: run the full e2e suite, not only the edited files.
- Concurrent binary rebuild by another agent: build only into `web/.e2e/` and the scratch dir.

## Rollback

Revert the commit; the spec and doc edits do not affect runtime code.
