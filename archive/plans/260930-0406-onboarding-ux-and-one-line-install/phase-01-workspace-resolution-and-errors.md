---
phase: 1
title: "Workspace resolution and consistent errors"
status: complete
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Workspace resolution and consistent errors
<!-- Updated: Validation Session 1 - connection-file fallback confirmed -->

## Goal
Every command finds the right workspace (flag → `SKILLHUB_WORKSPACE` → current project's connection file → upward discovery), reports "not found" plainly, never creates a workspace except `init`, and surfaces real argument errors before workspace errors.

## Context
- UX audit P0-3, P0-4, P1 "Generic FIX lines", "Exit codes" ([report](./reports/code-reviewer-260930-1045-ux-surface-audit.md)).
- Current code: `internal/delivery/cli/workspace_env.go` (`defaultWorkspace`), `internal/delivery/cli/workspace.go` (`workspaceFlag`, `writeInvalidWorkspace`), `internal/app/workspace.go:41-54` (doctor treats a missing path as "will be created"), `internal/app/connect.go` (good "no workspace" wording to reuse).

## Files to Create / Modify
- Modify: `internal/delivery/cli/workspace_env.go`, `workspace.go`, `root.go`, `skill.go`, `source.go`, `curation.go`, `distill.go`, `insight.go`, `telemetry.go`, `migration.go`, `resolver.go`, `evaluation.go`
- Modify: `internal/app/workspace.go`, `internal/app/errors.go` (add a `workspace_not_found` code only if the error-envelope schema allows; else reuse and update `schemas/error-envelope.schema.json` consistently)
- Create: `internal/delivery/cli/workspace_resolve.go` (resolution chain incl. reading `.mcp.json` / `.codex/config.toml` / `.gemini/settings.json` `skillhub` entry `--workspace` arg), tests alongside

## Tasks & Steps
- [x] Parse subcommand and flags fully before resolving the workspace, so typos/missing flags are reported first.
- [x] Resolution order: `--workspace` → `SKILLHUB_WORKSPACE` → connection file in cwd or nearest parent (skillhub MCP entry's `--workspace` value; project scope only) → upward discovery of `.skillhub/schema-version`. Never scan home.
- [x] One "not found" message everywhere (reuse `connect` wording): WHY names the path tried; FIX gives `--workspace <path>`, `export SKILLHUB_WORKSPACE=<path>`, or `skillhub init <path> --yes`.
- [x] Non-existent path for any command except `init`: "No Skill Hub workspace at <path>" (optionally suggest a close existing sibling dir). `doctor --fix` must not create a workspace at a missing path; only `init` creates.
- [x] Remove "non-interactive use" wording from user-facing errors.
- [x] Unknown ids (e.g. `check <unknown-id>`, `skill show <unknown>`) return a not-found error with exit ≠ 0; wrap raw OS errors (e.g. "statat") into user wording.
- [x] Audit all commands: any failure path exits non-zero (table test over command × failure).

## Verification
- `go test ./internal/delivery/cli ./internal/app` with new tests: resolution precedence (flag > env > connection file > discovery); typo path does not create dirs (`doctor --fix --yes` on missing path → error, no dir created); `skill lst` reports unknown subcommand not workspace error; `status` inside a connected project succeeds; unknown id exits non-zero.
- `gofmt -l internal` empty; `go vet ./...`; `go test ./...`.

## Risks
- Reading connection files: treat as untrusted input; accept only an absolute existing dir containing `.skillhub/schema-version`; bounded read, no symlink follow outside project.
- Changing doctor's create behavior: `init` must still create (init reuses doctor service) — gate creation on an explicit init flag in the app request, keep init tests green.
