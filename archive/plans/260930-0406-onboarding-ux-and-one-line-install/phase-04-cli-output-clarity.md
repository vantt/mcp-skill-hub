---
phase: 4
title: "CLI output clarity"
status: complete
priority: P2
effort: "1.5d"
dependencies: [1, 2, 3]
---

# Phase 4: CLI output clarity
<!-- Updated: Validation Session 1 - init refuses non-empty non-workspace dir unless --force; auto-rebuild on read confirmed -->

## Goal
Human CLI output uses plain words, shows one clear next step, gives exact runnable fix commands, and hides internals unless `--verbose`/`--json`.

## Context
- UX audit P1/P2 items U5–U7, U9–U15, U17–U19, U21, U23–U25, U27, U28 (see inventory). UX golden fixtures in `testdata/ux/` are authoritative for copy — update consistently, never weaken assertions.
- Code: `internal/delivery/cli/skill.go:405,415`, `source.go:255`, `insight.go:238`, `connect.go`, `workspace.go`, `curation.go`, `help.go`; `internal/app/connect.go`, `curation_home.go`, `workspace.go`, `skill_lifecycle.go`; `internal/hostintegration/bootstrap.go` (instruction block), `integration.go`.

## Files to Create / Modify
- Modify: files above; `testdata/ux/*` as needed; tests alongside.

## Tasks & Steps
- [x] Previews print the exact confirm command (with real values) and prefer `--yes` in examples; label matches flag (`--base-version`); wrong digest → "digest does not match the preview", not "stale". (U5)
- [x] connect/doctor output: one line per host, e.g. "Claude Code: connected (.mcp.json, CLAUDE.md, curator skill)"; drop "native-skill-instruction-coordination"; show the best-effort warning only when files are written. (U6)
- [x] `doctor` also checks the current project's connection (if cwd has one) and global connection (if present) and reports broken/outdated entries with `skillhub connect [-g] --yes` fix. (U7)
- [x] Activation errors list all missing requirements with one combined `skill edit ... --yes` command (uses Phase 2 app support). (U9)
- [x] `--content-file` rule aligned in help, validation and `validate` (frontmatter `name` must match id if present). (U10)
- [x] After create/activate: "Next:" line; IDs/digests only with `--verbose`; better starter SKILL.md template (headings: When to use, Steps, Examples) instead of repeating description. (U11)
- [x] `skill show` prints triggers, not-for, min-scope, file path. (U12)
- [x] Valid hand edits: read commands auto-rebuild when index is stale and workspace validates (reuse rebuild service under lock), else one clear message. (U13)
- [x] `validate` prints each finding with file:line and fix; doctor's FIX for content errors points to `skill edit`/file, not back to doctor. (U14)
- [x] Empty hub status: "No skills yet. Next: ask your agent 'create a skill for …' or run `skillhub skill create …`"; uncommitted changes → exact `git -C <ws> add -A && git -C <ws> commit -m …`. (U15)
- [x] `init` on a non-empty non-workspace dir (e.g. a project with source files): refuse with FIX suggesting a dedicated dir like `~/skillhub`; `--force` overrides. (U17)
- [x] Symlinked `~/.claude` (connect -g): specific message naming the symlink and workaround (connect per project, or `--host` subset). (U18)
- [x] `resolve` clarification output explains how to answer (flag/prior field) with an example. (U19)
- [x] `skillhub diff` groups draft files under "draft skills". (U21)
- [x] Wording sweep: replace user-facing canonical/generation/pins/Agent Host/substantive with plain terms; one term each for watched source, search index, host names (`claude` accepted, output "Claude Code"). (U23, U24)
- [x] Help: allowed values for `--operation`, `--trust`, `--cadence`; `skill` examples; `insight apply --proposal-file` format; fuller `init` preview. (U25)
- [x] Global + project connect: project block detects the global block and skips duplication (or marks as inherited). (U27)
- [x] `rebuild` default output: one summary line; details with `--verbose`. (U28)

## Verification
- `go test ./...`, `gofmt -l internal`, UX fixture tests; new tests for each item where behavior (not just copy) changes (doctor project check, auto-rebuild, init refusal, diff grouping, dedupe block).
- Manual walkthrough with scratch HOME following README Quickstart + guide tasks: no digests/row counts in default output; every error has a runnable FIX.

## Risks
- Auto-rebuild on read must stay offline and respect the catalog lock; never publish invalid bytes (keep validate-first).
