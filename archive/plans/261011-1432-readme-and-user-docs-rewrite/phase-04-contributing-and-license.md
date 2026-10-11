---
phase: 4
title: "Write CONTRIBUTING.md and LICENSE"
status: completed
priority: P2
effort: "0.5h"
wave: 1
dependencies: [1]
---

# Phase 4: Write CONTRIBUTING.md and LICENSE

## Audience and goal

`CONTRIBUTING.md`: a developer who wants to build, test or release Skill Hub. It takes the README "Development"
section so the README can stay a pitch. `LICENSE`: the owner chose MIT (decided).

## Owned files

- Create: `CONTRIBUTING.md` (repo root, <= 60 lines).
- Create: `LICENSE` (repo root, standard MIT text).

## Sources to read

- `README.md:114-132` (Development).
- `Makefile` targets (`check`, `test-race`, `lint-all`, `web-check`, `web-e2e`, `web-shots`, `web-dev`).
- `AGENTS.md` (read only, to avoid contradicting its test rules).
- Synthesis "Owner decisions of 2026-10-11", item 1 and 3.

## Required content

`CONTRIBUTING.md`:
1. One line on who this is for; prerequisites (Go version from `go.mod`, Node.js for the web UI).
2. `## Build and test`: `make check`, `make test-race`, `make lint-all`, `go run ./cmd/skillhub help`, and the
   web targets (`make web-check`, `make web-e2e`, `make web-shots`) with one line each taken from the Makefile
   comments.
3. `## Keep tests isolated`: the parallel and serial rule, the HOME/XDG isolation in `TestMain`, and the Go cache
   note (rewritten from README in the plan's voice).
4. `## Release`: one line linking `docs/release-runbook.md`.
5. `## Design records`: one line linking `docs/README.md` (maintainer section; the records are mostly in
   Vietnamese).

`LICENSE`: the standard MIT License text (from the SPDX `MIT` template), first line `MIT License`, then
`Copyright (c) 2026 Van Tran`. No other changes to the template.

## Commands to verify

Read the Makefile to confirm each target and its one-line comment. Do not run `make check` here (Phase 17 runs
it). Run `go run ./cmd/skillhub help` only if the Go build cache is warm; otherwise use the Phase 1 binary's
`skillhub help`.

## Acceptance criteria

- `CONTRIBUTING.md` <= 60 lines; every target named exists in the Makefile.
- `LICENSE` matches the SPDX MIT text byte for byte except the copyright line.
- The report flags the holder name "Van Tran" for the owner to confirm.

## Rollback

Revert the commit.
