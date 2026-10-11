---
phase: 15
title: "Write docs/README.md index"
status: completed
priority: P2
effort: "0.5h"
wave: 2
dependencies: [2, 3, 4, 5, 6, 7, 8]
---

# Phase 15: Write docs/README.md index

## Audience and goal

Anyone browsing `docs/` on GitHub. A short index by audience; the only place that links to the design records
(mostly Vietnamese), under "For maintainers" (decided).

## Owned files

- Create: `docs/README.md` (<= 50 lines).

## Required sections

1. `# Skill Hub documentation` and one line.
2. `## Start here`: Get started, Installation.
3. `## Guides`: Using Skill Hub (`user-guide.md`), Curating skills, Sources and learning, Web UI,
   Troubleshooting.
4. `## Reference`: Command reference, `contracts/` (API, error codes, result envelope), the compatibility matrix
   JSON, `../schemas/` request schema.
5. `## For maintainers`: `../CONTRIBUTING.md`, release runbook, `PRD.md`, `design/`, `use-cases/`,
   `distillery/`, `brainstorm/`, `plans/`, `references/`, with a note that the design records are mostly in
   Vietnamese.

Each link has a one-line description taken from the target page's intro (read each page; do not invent).

## Commands to verify

None. Check every link target exists (`ls`).

## Acceptance criteria

- <= 50 lines; every link resolves; design records appear only under "For maintainers".

## Rollback

Revert the commit.
