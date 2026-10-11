---
phase: 1
title: "Syntax cleanup"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Syntax cleanup

## Context

The command surface has aliases (`integrate`, `web`, `mcp serve`, `check`), a preview-then-`--yes` pattern on
onboarding commands, developer commands mixed into the main help, two quoting helpers, three `--since` parsers and
first-run text with internal words. This phase makes the syntax final so later phases build on it. Decisions 5 and 8
plus planner decisions P1-P4, P7 and P8 in [plan.md](./plan.md).

## Requirements

- Remove `integrate` (`internal/delivery/cli/root.go:37`, `help.go:454`), `web` (`root.go:73-74`,
  `help.go:429-432`), `check` (`root.go:59`, `help.go:215-226`; the command is `skillhub source check`), and
  `mcp serve` (`internal/delivery/cli/mcp.go:24-31`).
- The stdio server agents launch becomes the hidden `skillhub mcp stdio` (P1). Every registration writer uses it:
  `internal/hostintegration/integration.go:328,346,357,361,374`, `runtime_access.go:185,189,194`, and the TOML helper
  in `toml_update.go`. `ReadRegistration` (`connection.go:31`) keeps reading `--workspace` from both the old and the
  new argument list, so old entries are still recognized and reported as outdated by the plan comparison.
- `serve mcp` without `--addr` keeps serving stdio until Phase 3 replaces it; nothing else changes in serve here.
- `mcp` (`mcp stdio`, `mcp proxy`) is hidden from the global help but `skillhub help mcp` still prints its usage.
- Add `--version` (no short form): it prints the same line as `skillhub version`. `version` stays, unlisted (P2).
- Hide `eval`, `resolution`, `telemetry`, `resolve` from `skillhub help`; `skillhub help --dev` lists them.
- Remove `--yes` from `init`, `connect` (incl. `--remove`), `doctor --fix`, `cache clear`, `token rotate`; each acts
  directly and prints what it changed. Add `--dry-run` to each: same output as before under a "Would change:" heading,
  writes nothing. `--yes` stays on curation commands (P3); passing `--yes` to an onboarding command is an
  "unknown argument" error.
- Add `ask` permission rules for the commands that lost `--yes` (P4) in
  `internal/hostintegration/cli-permissions.go:13-15`; keep the existing three.
- `cache clear [--hub <url>]`: no flag clears every hub; `--hub` always takes a URL (P8) (`internal/delivery/cli/cache.go:19-38`).
- One `--since` parser accepting `YYYY-MM-DD` or `Nd`, with one error text, used by telemetry funnel, baseline,
  chains and import-transcripts (`telemetry_funnel.go:73`, `telemetry_chains.go:67`, `telemetry_transcripts.go:71`);
  help (`help.go:283-294`) matches it.
- One usage string for `skill` subcommands: `skill.go:29` and `skill.go:271` list different sets; generate both from
  one list that matches `help.go:118-143`.
- One platform-aware quoting helper for every printed command (merge `app.shellArgument`,
  `internal/app/connect_remote.go:325`, and `cli.shellArgument`/`shellQuote`, `internal/delivery/cli/next_commands.go:178`,
  `internal/app/operations.go:711`): single quotes on POSIX, double quotes with PowerShell escaping on Windows.
- Plain language in the onboarding output (init, connect, doctor, status, resolution errors): no "canonical",
  "proposal", "digest", "runtime block", "generation", "host integration". Rename user-facing "workspace" to "hub".
  Every printed runnable command goes through `termui.Command` (never wraps, `termui/termui.go:264`); every printed
  path is absolute (no "." preview path, `internal/app/workspace.go:52-55`, `internal/workspace/workspace.go:94`).

## Files

- Modify: `internal/delivery/cli/root.go`, `help.go`, `mcp.go`, `workspace.go` (init and doctor flags only),
  `connect.go` (flags only), `cache.go`, `token.go`, `skill.go`, `telemetry_funnel.go`, `telemetry_chains.go`,
  `telemetry_transcripts.go`, `telemetry_baseline.go`, `next_commands.go`, `output.go`, `version.go`,
  `workspace_resolve.go` (message text only)
- Modify: `internal/app/workspace.go` (drop the `yes` parameter of `Init`/`DoctorFix`; dry-run flag instead),
  `internal/app/connect.go`, `internal/app/connect_remote.go`, `internal/app/doctor_remote.go` (fix texts),
  `internal/app/errors.go`, `internal/app/host-connections.go` (suggested commands), `internal/app/curation_home.go`
  (`integrate` mention, plain words), `internal/app/operations.go` (quoting)
- Modify: `internal/hostintegration/integration.go`, `runtime_access.go`, `toml_update.go`, `connection.go`,
  `cli-permissions.go`, `matrix.json`, `types.go`
- Modify: `docs/mcp-compatibility-matrix.json` (registration args; `TestMatrixSynchronized` keeps both in sync)
- Modify: `Makefile:19` comment, `scripts/mcp-fanout-bench.sh` (`mcp stdio`)
- Modify: `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md` together (`source check`,
  no `--yes` on doctor/connect, `doctor --fix`)
- Tests to update: `internal/delivery/cli/{onboarding,serve_mcp,connect_profiles,connect_regression,connect_remote,doctor-safety,mcp,mcp_proxy,ux_behavior,workspace_next_steps,workspace_resolve,remote_commands,telemetry_help,telemetry_chains,telemetry_funnel,telemetry_transcripts,skill_add,next_commands,splitcli,human_output_width,version}_test.go`,
  `testdata/connect-workspace/*.golden`, `internal/hostintegration/{integration,cli-permissions,matrix,scope,doctor-skew}_test.go`,
  `internal/delivery/mcpserver/{hardening,memory_budget,remote_bench,remote_budget_bench,remote_e2e,skill_tools,subprocess}_test.go`,
  `internal/app/workspace_test.go`

## Steps

1. Add `mcp stdio` in `mcp.go`; switch every registration writer and the matrix to it; regenerate goldens; keep
   `ReadRegistration` tolerant of the old args.
2. Remove `integrate`, `web`, `check`, `mcp serve` from `root.go` and `help.go`; add `--version`; split the global
   help into the default list and the `--dev` list; hide `mcp`.
3. Replace `--yes` with direct action plus `--dry-run` in init, connect, doctor `--fix`, cache clear, token rotate;
   remove the preview branches (`internal/app/workspace.go:40-58`, `197-206`, `token.go:126-143`, `cache.go`).
4. Add the `ask` rules; extend the permission test to assert the full rule list.
5. Merge the quoting helpers; route every printed command through `termui.Command`.
6. Unify `--since`; fix the `skill` usage strings; reword onboarding messages; grep the onboarding output for the
   banned words.
7. Update the curator skill (both copies), `Makefile`, bench script and tests.

## Validation

- `go test ./internal/delivery/cli/... ./internal/hostintegration/... ./internal/app/... ./internal/systemskills/...`
  and `go test ./internal/delivery/mcpserver/...` pass.
- New tests: removed commands exit 2 with "not supported"; `--version` prints the version; `help` hides dev commands
  and `help --dev` lists them; `--yes` on `connect` is an unknown argument; `--dry-run` writes nothing (file tree
  hash before/after); the permission rule list contains the five new `ask` rules; the quoting helper round-trips a path
  with spaces and quotes through `sh -c` on POSIX (Windows case covered in CI).
- Grep gate: no `integrate`, `mcp serve`, `"web":`, `runCheck` registration in `internal/delivery/cli/root.go`.
- `go test ./internal/hostintegration -run TestMatrixSynchronized` passes.
- Windows/macOS: rely on CI for quoting and registration path tests.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Agents bypass prompts once `--yes` is gone | Medium x High | `ask` rules land in this phase, tested |
| Missed registration writer leaves a host launching `mcp serve` | Medium x High | Grep gate on `"mcp", "serve"` in non-test code; golden files |
| PowerShell quoting regression in printed commands | Medium x Medium | One helper with a table test per OS; CI Windows job |

## Rollback

Revert the phase commits. Registrations written by this phase (`mcp stdio`) need `connect` with the old binary after
a rollback; do not release Phase 1 alone (see plan.md Rollback).
