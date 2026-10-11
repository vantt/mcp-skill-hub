---
phase: 6
title: "Integration: e2e, docs, build, visual review"
status: completed
priority: P2
effort: "2h"
dependencies: [3, 4, 5]
---

# Phase 6: Integration: e2e, docs, build, visual review

## Context

The Go server embeds the web build (`make web-build` writes `internal/delivery/web/dist`, then
`go build ./cmd/skillhub`); `make web-e2e` builds `web/.e2e/skillhub` and runs Playwright (`Makefile:46-64`).
Another agent may be rebuilding binaries: check with the lead before building and never install into
`~/.local/bin`. Existing e2e touching the Resources tab: `web/e2e/a11y.spec.ts:66` (`?tab=resources`) and
`web/e2e/ux/flows.ts:230-234` (`skill-tab-resources`). The SKILL.md conflict flow e2e
(`web/e2e/skill-lifecycle.spec.ts:143-174`) must keep passing unchanged. `docs/user-guide.md` describes the
skill tabs under "Check one skill" (`:258-275`).

## Requirements

- New `web/e2e/skill-resources.spec.ts` on its own temp workspace (CLI `init`, `skill create`, then write
  `scripts/run.sh` (mode 0755), `references/guide.md` with YAML frontmatter, `assets/logo.png` with NUL bytes
  into the skill folder, then `skillhub rebuild`):
  1. Resources tab: Edit on `run.sh` and `guide.md`, View on `logo.png`.
  2. View `logo.png`: binary notice, no content, no Edit.
  3. Edit `guide.md`: the Markdown preview shows the frontmatter YAML block; change a line; Preview shows a diff
     touching only that file; confirm; the file on disk has the new text; reload shows it.
  4. Edit `run.sh`, then change the file on disk from the test; Preview opens the conflict drawer titled
     "run.sh changed since you opened it"; keep my edits -> preview -> confirm; mode is still 0755.
  5. All requests stay loopback-only (same assertion as `smoke.spec.ts`).
- `a11y.spec.ts`: add `?tab=resources&file=<path>` and `&edit=1` routes for the seeded skill if it has a
  companion file; otherwise leave the list route only.
- `ux/flows.ts`: one entry for the resource editor in the style of `skill-tab-resources`.
- `docs/user-guide.md`: a short paragraph under "Check one skill": the Resources tab lists companion files;
  **Edit** opens a simple editor for text files up to 1 MiB, saving shows the exact change first and writes
  only on confirm (Git keeps history; commit yourself); binary or larger files open read-only; editing a skill
  from another repository needs a new content approval in the terminal. Do not regenerate `docs/images/web/`.

## Files

- Create: `web/e2e/skill-resources.spec.ts`
- Modify: `web/e2e/a11y.spec.ts`, `web/e2e/ux/flows.ts`, `docs/user-guide.md`

## Steps

1. Merge Phases 2 to 5; confirm with the lead that no other build is running.
2. Write the spec and the e2e edits.
3. `make check` and `make web-check`; then `make web-e2e`.
4. Manual check on a scratch copy of the live hub with isolated `HOME`/`XDG_*` (never the hub itself): open a
   skill with scripts and references, view and edit at 1280 and 390 px, confirm one edit, check `git diff`
   shows only that file.
5. Update the docs.
6. Hand the rebuilt UI to the lead for the visual review (editor layout, ActionBar placement, binary and
   truncated states, conflict drawer wording).

## Validation

- `make check`, `make web-check`, `make web-e2e` all green; `go build ./cmd/skillhub` succeeds after `make web-build`.
- The new spec fails on the pre-change build (no Edit button) and passes after.

## Risks

- Concurrent binary rebuild by another agent: build only into `web/.e2e/` and the scratch dir.
- Hidden dependence on the old Resources notice text in specs: run the full suite.

## Rollback

Revert the commit; spec and doc edits do not affect runtime code.
