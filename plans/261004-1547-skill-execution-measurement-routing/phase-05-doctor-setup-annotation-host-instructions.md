---
phase: 5
title: "Doctor CLI, setup annotation, host instructions"
status: done
priority: P1
effort: 8h
dependencies: [2, 3, 4]
---

# Phase 5: Doctor CLI, setup annotation, host instructions

## Context

- [plan.md](./plan.md) D4, D5.
- `app.ResolverService.Resolve` (`internal/app/resolver.go:31`) builds primary/supporting manifests after ranking (`:121-145`) while the catalog handle is open; that is where the annotation goes.
- Recommendation types: `internal/resolver/types.go` (`Recommendation`, `Supporting`); committed contract `schemas/skill-resolve-response-v1.schema.json` (`$defs.primary` and `$defs.supporting` are `additionalProperties: false`). The MCP server replaces its derived schema with the committed one (`server.go:278-291`).
- Host bootstrap text: `internal/hostintegration/bootstrap.go:21` (`bootstrapBlock`, marker v1). The same block is checked into this repo's `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`.
- Tool descriptions: `skill_resolve` (`internal/delivery/mcpserver/resolver_tools.go:14`), `skill_get` (`skill_tools.go:135`). Request contract `schemas/skill-resolve-request-v1.schema.json` (`task.description`).
- CLI dispatch: `internal/delivery/cli/skill.go:24` (`runSkill`); help text `internal/delivery/cli/help.go:87`.

## Requirements

1. `skillhub skill doctor <id> [--workspace <path>] [--json]`:
   - Calls `app.SkillDoctorService.Run(ctx, workspace, id)`: `SnapshotService.Ensure` (working dir), live checks, `skillruntime.RunDoctor` with `allowCheck = verdict.Trusted`, writes the doctor cache, records `skill.doctor_checked` through an owned recorder (pattern: `startCommandTelemetry` in `internal/delivery/cli/distill.go:65`).
   - A skill without a `runtime` block reports `ready` with the note "no runtime requirements declared" and writes nothing.
   - Human output: one line per check (`PASS`/`FAIL`/`SKIP`, name, detail), the final state, and for `setup_required` the declared `setup` command as a suggestion (never run). `check` output is shown truncated (last 2 KiB) and never stored.
   - Exit codes: 0 `ready`, 1 `setup_required` / `unsupported_platform`, 2 invalid request or unknown skill.
2. Resolver annotation (app layer only): for primary and each supporting skill with a `runtime` block, compute `skillruntime.SetupState(spec, live checks, cached doctor result)` and set `Setup *SetupStatus` (`state`, `reason_codes`, `checked_at` omitempty). `unknown` never changes status or ranking. Add `setup_state` of the primary to the resolution telemetry payload.
3. Contract: add optional `setup` (object, `additionalProperties: false`, `state` enum, `reason_codes` string array, `checked_at` date-time) to `$defs.primary` and `$defs.supporting` in `skill-resolve-response-v1.schema.json`; Go fields `json:"setup,omitempty"`.
4. Host instruction text (bootstrap block, same marker version so `updateBootstrap` replaces it in place). Add, after the `skill_resolve` paragraph:
   - "Send `task.description` in English; translate the user's request first if it is in another language."
   - "When an activated skill response includes `local.path`, resolve the skill's relative file references (for example `scripts/…`) against that directory. If it includes `local.preflight`, run its `check` command in `working_directory` under your own permissions before using the scripts, and ask the user before running `setup`. If `local.status` is `scripts_withheld`, do not run the skill's scripts; tell the user the scripts need review (`skillhub skill review <id>`)."
5. Tool descriptions: `skill_resolve` mentions the English normalization and that `primary.setup` may report `setup_required`; `skill_get` mentions `local.path` and `local.preflight`. Add `"description"` to `task.description` in `skill-resolve-request-v1.schema.json`: "Task summary in English (translate non-English requests first)."
6. Refresh the generated bootstrap blocks in this repo's `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` by running `go run ./cmd/skillhub connect` in preview to confirm the diff touches only the block, or by editing only the text between the markers.

## Files

Create:
- `internal/app/skill_doctor.go`, `internal/app/skill_doctor_test.go`
- `internal/delivery/cli/skill_doctor.go`, `internal/delivery/cli/skill_doctor_test.go`

Modify:
- `internal/delivery/cli/skill.go` (dispatch `doctor`; usage string at `:26`)
- `internal/delivery/cli/help.go` (`skill` usage)
- `internal/app/resolver.go` (annotation after manifests; `setup_state` payload)
- `internal/app/resolver_test.go`
- `internal/resolver/types.go` (`SetupStatus`, fields on `Recommendation`, `Supporting`)
- `schemas/skill-resolve-response-v1.schema.json`, `schemas/skill-resolve-request-v1.schema.json`
- `internal/hostintegration/bootstrap.go`, `internal/hostintegration/integration_test.go` (expected block text)
- `internal/delivery/mcpserver/resolver_tools.go`, `internal/delivery/mcpserver/skill_tools.go` (descriptions only)
- `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` (bootstrap block text only)

## Steps

1. Doctor service and CLI with tests using a skill whose `check` is `sh -c 'exit 0'` / `exit 3` (Unix-only test guard) and fake bins via a temp `PATH`.
2. Resolver annotation; prove ranking is unchanged by resolving the same request with and without a failing doctor cache and comparing everything except `setup`.
3. Schema edits; run the schema embed tests and MCP contract tests.
4. Bootstrap text and descriptions; update expected strings.

## Implementation status

- [x] Doctor service and CLI (`app.SkillDoctorService.Run`, `skillhub skill doctor <id> [--workspace] [--json]`, exit codes 0/1/2, cache write, `skill.doctor_checked` telemetry)
- [x] Resolver setup annotation (post-ranking `setup` on primary and supporting; `setup_state` of the primary in resolution telemetry)
- [x] Contract (`$defs.setup` referenced from `$defs.primary` and `$defs.supporting`; request `task.description` description)
- [x] Bootstrap text (same v1 markers) and tool descriptions for `skill_resolve` and `skill_get`
- [x] Refreshed bootstrap blocks in `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` (text between markers only)

Implementation notes (decisions the spec left open):
- The doctor cache key comes from one helper, `doctorFingerprint(manifestVersion, spec)` in `internal/app/skill_doctor.go`. The doctor, the activation preflight, and the resolver annotation all pass the distributed manifest version (`DistributedSkill.Version`, which is `LocalSkill.ManifestVersion`). A test proves that a doctor write is read back by both the preflight and the resolver annotation.
- `SnapshotService.Ensure` now wraps an internal `ensure` that also returns the parsed spec, the trust verdict, and the workspace root. The doctor reuses the snapshot path, version, and verdict instead of re-deriving them. The non-executing live checks moved into a shared `runtimeProbe` that the preflight and the resolver both use. This touched `internal/app/skill_snapshot.go`, which the phase file list does not name; behavior is unchanged and the snapshot tests pass.
- For an untrusted skill the doctor never shows the `setup` command. It suggests `skillhub skill review <id>` instead, matching the activation preflight.
- A skill without a `runtime` block writes no cache and records no telemetry ("writes nothing").
- Doctor telemetry reason codes are the review reason codes plus `<kind>_<status>` for each check that did not pass. Check names are left out because env var names can be private. A cache-write failure records `status=failed`.
- Setup annotation reason codes: `<kind>_<detail>` for failing live checks (`bin_not_found`, `env_missing`, `platform_unsupported`), `doctor_setup_required` for a failing cached result, and `doctor_not_run` when the state is `unknown`. If the manifest cannot be read during resolution, the annotation is left out; the resolution does not fail.
- The human output strips terminal control characters from check lines and from check output.
- `schemas/embed_test.go` got one contract test for the optional `setup` object; it is outside the file list but tests a schema the phase owns.
- `CLAUDE.md` and `GEMINI.md` are gitignored in this repository, so only the `AGENTS.md` change shows up in `git status`.

## Tests and validation

- `go test ./internal/app/ -run 'Doctor|Resolve' ./internal/delivery/cli/ -run Doctor ./internal/resolver/ ./internal/hostintegration/ ./internal/delivery/mcpserver/ ./schemas/`
- Doctor: missing bin → exit 1; version below constraint → exit 1 with detail; unsupported platform → exit 1, check not run; untrusted third-party skill → check `SKIP` with `scripts_review_required`; cache file written with no env values; telemetry event recorded with allowed payload only.
- Resolver: setup absent for skills without runtime; `unknown` without cache; `setup_required` with a failing cache or a missing bin; `ready` with a passing cache; response validates against the committed schema.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| MCP server env differs from the user's shell, so live env checks mislead | Medium × Low | Live checks only add `setup_required`/`unknown` hints; nothing is filtered; doctor runs in the user's shell. |
| Hosts ignore instructions and run scripts anyway | Medium × Medium | Withheld scripts are physically absent from the snapshot; text instructions are a second layer. |
| Bootstrap text change churns user files | Low × Low | Same marker version; `connect` replaces only the block. |

## Rollback

Revert. Cached doctor results are disposable (`runtime/cache/doctor`).
