---
phase: 1
title: "Syntax cleanup"
status: completed
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: Syntax cleanup

## Context

The command surface has aliases (`integrate`, `web`, `check`), a preview-then-`--yes` pattern on onboarding commands,
developer commands mixed into the main help, four quoting helpers and three `--since` parsers. This phase makes the
syntax final so later phases build on it, and lands the docs-command extractor every later test uses. Decisions 5 and
8 plus planner decisions P1-P4, P7 and P8 in [plan.md](./plan.md); red-team items S1 (fix text), S2, F10, F11, F12.

Scope limit (F12): removals, the `--yes` -> `--dry-run` switch, the permission rules, the quoting helper and the
extractor. Wording of resolution, connect, serve, doctor and status messages is changed once, later, in the phase that
owns each surface; this phase only drops `--yes` tokens from printed commands so nothing prints an invalid command.
`connect --workspace`/`--project` stay until Phase 2, so the suggested connect commands printed by
`internal/app/host-connections.go:97-99` remain valid here.

Agents' stdio registrations are not touched: `mcp serve` keeps its name (P1, revised).

## Requirements

- Remove `integrate` (`internal/delivery/cli/root.go:38`, `help.go:454`), `web` (`root.go:74-75`, `help.go:429-432`)
  and `check` (`root.go:60`, `help.go:215-226`; the command is `skillhub source check`).
- `mcp` (`mcp serve`, `mcp proxy`) is hidden from the global help; `skillhub help mcp` still prints its usage.
  `mcp serve` behavior is unchanged (stdio only; it already rejects network flags, `internal/delivery/cli/mcp.go:24-31`).
  `serve mcp` without `--addr` keeps serving stdio until Phase 3 replaces it; nothing else changes in serve here.
- Add `--version` (no short form): it prints the same line as `skillhub version`. `version` stays, unlisted (P2).
- Hide `eval`, `resolution`, `telemetry`, `resolve` from `skillhub help`; `skillhub help --dev` lists them.
- Remove `--yes` from `init`, `connect` (incl. `--remove`), `doctor --fix`, `cache clear`, `token rotate`; each acts
  directly and prints what it changed. Add `--dry-run` to each: same output as before under a "Would change:" heading,
  writes nothing. `--yes` stays on curation commands (P3); passing `--yes` to an onboarding command is an
  "unknown argument" error. Every printed command that named one of these with `--yes` drops the token (mechanical
  edit; inventory rows marked Phase 1 in plan.md), including `internal/delivery/mcpserver/proxy.go:944,949` (which also
  becomes `skillhub cache clear --hub <url>` with the real URL, P8) and `scripts/remote-timing.sh:110`.
- Fix the remote-doctor text that calls connect "a preview that writes nothing" (`internal/app/doctor_remote.go:475`):
  connect now writes, so the text becomes "check it with `skillhub connect --hub <url> --token-file <file> --dry-run`,
  which sends the token because you run it and writes nothing" (S1).
- `token rotate` refuses when stdout is not a terminal, like `token show` (`internal/delivery/cli/token.go:89`), unless
  `--dry-run`; the refusal names the command to run in a terminal (S2).
- Permission rules (P4, revised; S2) in `internal/hostintegration/cli-permissions.go:9-16`: keep the three existing
  entries and add `ask` rules `Bash(skillhub *init*)`, `Bash(skillhub *connect*)`, `Bash(skillhub *--fix*)`,
  `Bash(skillhub *token rotate*)`, `Bash(skillhub *cache clear*)`, `Bash(skillhub *serve*)`. Over-matching (for example
  a skill id containing `connect`) only adds a prompt and is acceptable.
- Curator skill text (both identical copies, `system-skills/curator/SKILL.md` and
  `internal/systemskills/curator/SKILL.md`, text edits only): `source check` instead of `check`
  (`SKILL.md:90,154,209,334`), no `--yes` on doctor (`:134`), and one instruction: "Ask the user before running
  `skillhub init`, `skillhub connect`, `skillhub doctor --fix`, `skillhub token rotate`, `skillhub cache clear` or
  `skillhub serve`; Codex and Gemini CLI do not prompt for these." Maintenance command names change in Phase 4.
- `cache clear [--hub <url>]`: no flag clears every hub; `--hub` always takes a URL (P8)
  (`internal/delivery/cli/cache.go:19-38`).
- One `--since` parser accepting `YYYY-MM-DD` or `Nd`, with one error text, used by telemetry funnel, baseline,
  chains and import-transcripts (`telemetry_funnel.go:73`, `telemetry_chains.go:67`, `telemetry_transcripts.go:71`);
  help (`help.go:283-294`) matches it.
- One usage string for `skill` subcommands: `skill.go:29` and `skill.go:271` list different sets; generate both from
  one list that matches `help.go:118-179`.
- One platform-aware quoting helper for every printed command, replacing all four: `app.shellArgument`
  (`internal/app/connect_remote.go:325`), `app.shellQuote` (`internal/app/operations.go:711`), `cli.shellArgument`
  (`internal/delivery/cli/next_commands.go:178`) and `cli.shellQuote` (`internal/delivery/cli/telemetry.go:261`).
  Single quotes on POSIX, PowerShell quoting on Windows. It lives in a package both `app` and `cli` can import.
- Docs-command extractor (F10), in `internal/delivery/cli` test code: `commandSegments(line string) []string` splits a
  command line on `&&` and `;`, trims each part, keeps parts that start with `skillhub`, and ignores `cd`, `mkdir`,
  `git` and `ssh`/`scp` parts. Contract: every `skillhub` segment of a printed or documented next command passes
  `checkGuideCommand` (`internal/delivery/cli/remote_guide_doc_test.go:51-88`). `checkGuideCommand` accepts a segment
  with no subcommand word when its flag is a global one (`skillhub --version`, checked against `skillhub help`). Lines
  labelled "on the hub machine" or plain Git lines are exempt by label. `TestRemoteHubGuideCommandsExist` switches to
  the extractor; later phases reuse it.
- Plain-language rewording is out of scope here (F12), except: every printed runnable command goes through
  `Printer.Command` (never wraps, `internal/delivery/cli/termui/termui.go:264`).

## Files

- Modify: `internal/delivery/cli/root.go`, `help.go`, `workspace.go` (init and doctor flags only), `connect.go` and
  `connect_remote.go` (`--yes` -> `--dry-run` only), `cache.go`, `token.go`, `skill.go`, `telemetry_funnel.go`,
  `telemetry_chains.go`, `telemetry_transcripts.go`, `telemetry_baseline.go`, `telemetry.go` (quoting),
  `next_commands.go`, `output.go`, `version.go`
- Modify: `internal/app/workspace.go` (replace the `yes` parameter of `Init`/`DoctorFix` with a dry-run option; the path
  parameter stays), `internal/app/connect.go`, `internal/app/connect_remote.go`, `internal/app/doctor_remote.go`
  (`--yes` tokens and the preview sentence only), `internal/app/errors.go`, `internal/app/host-connections.go`
  (`--yes` tokens only), `internal/app/curation_home.go` (`integrate` mention), `internal/app/operations.go` (quoting)
- Create: the shared quoting helper (for example `internal/shellquote/shellquote.go` + test)
- Modify: `internal/hostintegration/cli-permissions.go`, `internal/hostintegration/types.go` (`integrate` mention)
- Modify: `internal/delivery/mcpserver/proxy.go:944,949`, `scripts/remote-timing.sh:110`
- Modify: `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md` together
- Modify: `internal/delivery/cli/remote_guide_doc_test.go` (extractor, `--version` handling)
- Tests to update: the 33 test files under `internal/{delivery/cli,app,hostintegration}` that pass `--yes` (195 occurrences, about 70 call
  sites on onboarding commands; grep `--yes` together with `"init"|"connect"|"doctor"|"cache"|"token"`), plus
  `internal/delivery/cli/{curation_test.go:182,source_test.go,source_watch_test.go,telemetry_lifecycle_test.go:69,telemetry_help,telemetry_chains,telemetry_funnel,telemetry_transcripts,skill_add,next_commands,splitcli,human_output_width,version}_test.go`,
  `internal/hostintegration/cli-permissions_test.go`, `internal/systemskills/embed_test.go` (copies stay identical)

## Steps

1. Add the extractor and the `--version` case to the guide test first; keep the existing guide passing.
2. Remove `integrate`, `web`, `check` from `root.go` and `help.go`; add `--version`; split the global help into the
   default list and the `--dev` list; hide `mcp`.
3. Replace `--yes` with direct action plus `--dry-run` in init, connect, doctor `--fix`, cache clear, token rotate;
   remove the preview branches (`internal/app/workspace.go:40-57`, `199-208`, `internal/delivery/cli/token.go:126-143`,
   `cache.go`); drop the `--yes` tokens from every printed command; fix the remote-doctor preview sentence; add the
   `token rotate` terminal guard.
4. Add the `ask` rules; extend the permission test.
5. Merge the four quoting helpers; route every printed command through `Printer.Command`.
6. Unify `--since`; fix the `skill` usage strings.
7. Update the curator skill (both copies), `proxy.go`, `remote-timing.sh` and tests.
8. Verification step for the open item (manual, recorded in the phase report): in a sandbox project connected with the
   new binary, ask Claude Code to run `cd <project> && skillhub doctor --fix` and `skillhub doctor --json --fix`, and
   record whether each prompts. If the compound form does not prompt, add `Bash(*&& skillhub *)` to the `ask` list
   and its test.

## Todo

- [x] Docs-command extractor (`commandSegments`) and the `--version` case in `checkGuideCommand`; `TestRemoteHubGuideCommandsExist` uses the extractor
- [x] Remove `integrate`, `web`, `check`; add `--version`; global help vs `help --dev`; hide `mcp` (`help mcp` still prints usage)
- [x] `init`, `connect` (incl. `--remove`), `doctor --fix`, `cache clear`, `token rotate` act directly; `--dry-run` prints "Would change:" and writes nothing; `--yes` on them is an unknown argument
- [x] Drop `--yes` from every printed onboarding command (app, CLI, `mcpserver/proxy.go`, scripts, workflows, web e2e); `proxy.go` prints `cache clear --hub <url>`
- [x] Remote-doctor sentence names `connect --hub ... --dry-run`
- [x] `token rotate` terminal guard (unless `--dry-run`)
- [x] Position-independent `ask` rules and the permuted-flag test
- [x] Curator skill text (both copies): `source check`, ask-first sentence
- [x] `cache clear [--hub <url>]`
- [x] One `--since` parser with one error text; help matches
- [x] One list for the `skill` subcommand hints
- [x] One platform-aware quoting helper (`internal/shellquote`) replacing all four
- [x] Printed runnable commands through `Printer.Command` in the files this phase owns
- [x] Step 8: compound-command matching verified from the Claude Code permissions documentation (ask rules apply when any subcommand matches); no fallback rule needed. Not run by hand in an interactive session.
- [x] Grep gate clean; `go test ./...`, `go vet` (linux, windows, darwin) and `gofmt -l` clean

Report: [cook-261011-onboarding-phase1](../../../plans/reports/cook-261011-onboarding-phase1.md)

## Validation

- `go test ./internal/delivery/cli/... ./internal/hostintegration/... ./internal/app/... ./internal/systemskills/...`
  and `go test ./internal/delivery/mcpserver/...` pass.
- New tests: removed commands exit 2 with "not supported"; `--version` prints the version; `help` hides dev commands
  and `help --dev` lists them; `help mcp` prints usage; `--yes` on `connect` is an unknown argument; `--dry-run` writes
  nothing (file tree hash before/after); `token rotate` without a terminal exits 1 and does not change the token file.
- Permission rule test: a table of command lines must match an `ask` rule using Claude's glob semantics as implemented
  in the test (`*` matches any run of characters, including none): `skillhub doctor --fix`, `skillhub doctor --json
  --fix`, `skillhub doctor --fix --json`, `skillhub connect --dry-run`, `skillhub connect -g`, `skillhub connect --hub
  http://h:7420 --token-file f`, `skillhub init`, `skillhub init https://x/y.git`, `skillhub token rotate --token-file
  f`, `skillhub cache clear --hub http://h:7420`, `skillhub serve`, `skillhub serve web --addr 127.0.0.1:7421`; and
  `skillhub status` and `skillhub skill list` match only the `allow` rule.
- Quoting helper table test: a path with spaces, single quotes and `$` round-trips through `sh -c` on POSIX; PowerShell
  cases asserted as strings (Windows CI runs them through `powershell -Command`).
- Extractor unit test: `cd ~/skill-hub && skillhub init`, `mkdir -p a && cd a && skillhub init`, `cd a; skillhub init`,
  `skillhub --version`, `git -C h commit -m x` (no segments).
- Grep gate (repo-wide over `internal`, `web/src`, `web/e2e`, `scripts`, `.github`, `system-skills`, excluding
  `archive/`, `plans/`, `.claude/`, `docs/`): no `skillhub integrate`, `skillhub web`, `skillhub check`, and no
  `--yes` after `init|connect|doctor|cache clear|token rotate` in non-test code. Docs are swept in Phase 5.
- Windows/macOS: rely on CI for quoting and permission tests.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Agents bypass prompts once `--yes` is gone | Medium x High | Infix `ask` rules land in this phase, tested with permuted flags; curator text asks first for Codex and Gemini |
| Claude Code does not split `cd ... && skillhub ...` before matching | Unknown x Medium | Step 8 verification; fallback rule ready |
| PowerShell quoting regression in printed commands | Medium x Medium | One helper with a table test per OS; CI Windows job |
| A printed command still carries `--yes` and fails | Medium x Medium | Grep gate; the extractor-based guide test |

## Rollback

Revert the phase commits. No agent registration changes in this phase; new `ask` rules written by `connect` are extra
entries an older binary ignores. Do not release Phase 1 alone (see plan.md Rollback).
