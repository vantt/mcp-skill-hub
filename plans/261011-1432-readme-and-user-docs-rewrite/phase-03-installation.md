---
phase: 3
title: "Write docs/installation.md"
status: pending
priority: P2
effort: "1h"
wave: 1
dependencies: [1]
---

# Phase 3: Write docs/installation.md

## Audience and goal

A reader who needs more than the one-liner: pin a version, check signatures, change the install folder, upgrade,
uninstall or build from source. One page owns this text; the README and the release runbook link here.

## Owned files

- Create: `docs/installation.md` (<= 90 lines).

## Sources to read

- `README.md:5-19` (one-liners), `:44-64` (details, build from source).
- `docs/release-runbook.md:5-18` (platforms), `:71-110` (signature policy), `:111-150` (install, upgrade,
  uninstall).
- `scripts/install.sh` and `scripts/install.ps1` (read only, to confirm each environment variable and flag).
- Style guide and glossary in plan.md.

## Required sections (proposed H2s)

1. Intro: one line on what the installer does (places `skillhub` in `~/.local/bin` or
   `%LOCALAPPDATA%\skillhub\bin`, checks SHA-256, updates `PATH`).
2. `## Install with one command`: Linux and macOS, Windows (PowerShell); check with `skillhub version`.
3. `## Choose options`: `<details>` with `SKILLHUB_VERSION`, `SKILLHUB_PREFIX`, `SKILLHUB_NO_MODIFY_PATH`,
   `SKILLHUB_REQUIRE_SIGNATURE`; only variables that exist in the scripts.
4. `## Check signatures`: one paragraph on what is always checked and what needs `cosign`; link the runbook's
   "Manual artifact verification" for the full steps (do not copy them).
5. `## Upgrade`: `skillhub update`, or re-run the one-liner.
6. `## Uninstall`: Unix and Windows commands; what is never deleted (workspaces, skills, connection files).
7. `## Build from source`: Git, Go version from `go.mod`, Node.js; `make web-build` then `go build`. Use a
   neutral output path (`go build -o skillhub ./cmd/skillhub`) rather than `~/.local/bin`.
8. Supported platforms: one line (Linux, macOS, Windows on amd64 and arm64); the archive table stays in the
   runbook.

## Commands to verify

Do not run the installer against a real HOME. Verify each environment variable and flag by reading
`scripts/install.sh` and `scripts/install.ps1`, and run `sh scripts/install.sh --help` only if the script
supports it without side effects (check first by reading). Verify `skillhub help update` and `skillhub version`
with the Phase 1 binary in `SB=/tmp/skillhub-docs-rewrite/installation`. Do not run `make web-build` or
`go build` (Phase 1 already proved them; confirm the Go version in `go.mod`).

## Images

None.

## Acceptance criteria

- <= 90 lines; style guide met; long option lists in `<details>`.
- Every variable and flag exists in the scripts (cite `file:line` in the report).
- No duplication of the runbook's manual verification steps; one link instead.
- No banned claim; links resolve.

## Rollback

Revert the commit.
