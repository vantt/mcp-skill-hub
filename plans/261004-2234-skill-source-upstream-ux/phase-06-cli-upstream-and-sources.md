---
title: "Phase 6: CLI: upstream and sources"
status: in-progress
---

# Phase 6: CLI: upstream and sources

<!-- Updated: Validation Session 1 - UPDATED (upstream commit date) column; CHANGED = files in the skill folder; CLI is one of the two places updates are applied -->

## Context

- Plan: [plan.md](./plan.md) (D6, D12). Depends on phases 3–5. Research: [upstream UX](./reports/researcher-261004-2234-upstream-update-ux-patterns.md) recommendations 5–6.
- Read first: `internal/delivery/cli/skill.go` (dispatch at lines 25-160, `parseSkillFlags` at line 290), `internal/delivery/cli/source.go` (dispatch, `parseSourceFlags` at line 227, `runCheck` at line 301, printers at lines 394-460), `internal/delivery/cli/skill_add.go`, `internal/delivery/cli/output.go` (`writeResult` exit codes), `internal/delivery/cli/termui/termui.go` (`Table` at line 185, `Next` at line 269), `internal/delivery/cli/help.go:87-172`, `internal/delivery/cli/human_output_width_test.go`, `docs/use-cases/03-cli-and-curator-mcp-mapping.md`.

## Overview

First-class CLI for the skill-centric model: `skill outdated`, `skill upstream`, `skill update`, upstream-aware `skill confirm`, a grouped `source list`, and `source attach|detach|unwatch|backfill`, with consistent human output, `--json`, exit codes, help text, and next-step hints.

## Requirements

1. **`skillhub skill outdated [--check] [--all] [--exit-code] [--json] [--workspace <p>]`**
   - `--check` first runs `SourceService.CheckSources` for every source with tracked skills (explicit IDs, not `allDue`), printing one warning line per unavailable source.
   - Default rows: statuses `update_available`, `diverged`, `upstream_removed`, `unknown`, `unavailable`, `untracked`. `--all` adds `up_to_date`, `modified`, `pinned`.
   - Columns `SKILL SOURCE CURRENT LATEST UPDATED CHANGED LOCAL STATUS`; commits shortened to 7; UPDATED is the date (`YYYY-MM-DD`) of the newest upstream commit read (`latest_committed_at`, `-` when unknown); CHANGED (files changed within the skill folder, never a commit count) is `N files`, `1 file`, `?` (unknown), or `-`; STATUS uses words: `update available`, `diverged`, `removed upstream`, `modified locally`, `up to date`, `pinned`, `unreachable`, `not checked`, `not tracked`.
   - Exit codes: 0 normally; with `--exit-code`, 1 when any skill is `update_available`, `diverged`, or `upstream_removed`; 2 for invalid flags. `--json` writes the `UpstreamListResult` and does not change the exit code.
2. **`skillhub skill upstream <id> [--check] [--json]`** — detail view of `GetSkillUpstream` (with `--check`, checks that skill's source first). Non-repository skill → exit 2 with the service's `invalid_request`.
3. **`skillhub skill update <id> [--no-check] [--target <commit>] [--accept <path>=upstream|local|merged]... [--manual <path>=<file>]... [--write-conflicts <dir>] [--idempotency-key <k>] [--yes] [--json] [--verbose]`**
   - Unless `--no-check`, checks the skill's source first.
   - Builds `UpstreamUpdateInput` (paths are relative to the skill folder; `--manual` reads the file).
   - The first line names the repository URL and the source ID (`Update <id> from <B7> to <U7> (<repository>, source <source-id>), upstream commit of <YYYY-MM-DD>.`) so a reviewer sees where the content comes from. CHANGE words: `changed upstream`, `changed here`, `changed here and upstream`, `added upstream`, `added here`, `removed upstream`, `removed here`, `blocked (contains lines Skill Hub reads as conflict markers)`.
   - Human preview prints the file table, unified diffs for changed files (`ResultDiff`; with `--verbose` also `UpstreamDiff` and `LocalDiff`), the trust notice, and `Next: skillhub skill confirm <proposal>`. With `--yes` and no unresolved files it confirms immediately. With `--yes` and unresolved files it exits 2 (`invalid_request`: `N file(s) need a decision before this update can be applied.`).
   - `--write-conflicts <dir>` writes `MergedWithMarkers` for each unresolved text file to `<dir>/<path>` through `os.OpenRoot(<dir>)` (so symlinked parents cannot escape), creating parents and refusing to overwrite existing files, then prints the `--manual` command to use after editing.
4. **`skillhub skill confirm`** adds a case for `app.UpstreamUpdateResult` printing its summary and `Next: skillhub skill review <id>` when `TrustImpact.ReviewRequiredAfterApply`.
5. **Sources**
   - `source list` calls `ListSourceGroups` and prints `Groups` (sample below); `--json` keeps `candidates`, `sources`, and adds `groups`. `source show` keeps using `ListSources` plus the one source's group entry.
   - `source show <id>` adds `Roles`, linked skills with upstream status, and last check.
   - `source watch <locator> --skill-id <id> ...` (new flag for watch; required by phase 5).
   - `source attach <source-id|locator> --skill-id <id> [--ref <r>] [--path <p>] [--cadence <c>] [--yes]`, `source detach <source-id> --skill-id <id> [--yes]`, `source unwatch <source-id> [--yes]`: preview by default; `--yes` confirms with fresh pins; otherwise print the existing `source confirm --proposal … --proposal-digest … --base-version …` hint.
   - `source triage <candidate> --decision accept (--skill-id <id> | --new-skill <id>)` and `--decision import [--skill <name>]... [--all]` (import prints the skill-add preview and `Next: skillhub skill confirm <proposal>`).
   - `source backfill [--skill <id> [--path <repo-path>]] [--yes]`: preview table `SKILL ACTION SOURCE PATH REASON`; `--yes` applies.
   - `check` / `source check` human output adds, per source with tracked skills, `  <skill>: <status words>` lines under the source bullet.
6. **Untrusted strings in output.** Every upstream-derived string printed by these commands (paths, repository URLs, source IDs, error text) passes through one helper `printableText(s string) string` that replaces non-printable runes (`unicode.IsPrint` false, including ESC and newlines) with `\xNN` escapes, so a crafted file name cannot inject terminal control sequences.
7. **Help text** (`help.go`): update `skill`, `source`, and `check` usage with every new subcommand and flag, the exit-code sentence for `outdated`, and one line distinguishing `skillhub skill update <id>` (update a skill from its upstream) from `skillhub update` (update the skillhub binary). Global usage line for `skill` becomes `List, show, create, edit, review, add, update from upstream, and change the state of skills`; for `source`: `Repositories and documents your skills come from or learn from`.

## Related code files

Create: `internal/delivery/cli/skill_upstream.go` (outdated, upstream, update, rendering helpers including `relativeTime`), `internal/delivery/cli/skill_upstream_test.go`.

Modify: `internal/delivery/cli/skill.go` (dispatch, `skillFlags` fields `accepts`, `manuals []string`, `target`, `writeConflicts string`, `check`, `noCheck`, `exitCode bool`; `parseSkillFlags` validation per subcommand; `skill confirm` case), `internal/delivery/cli/source.go` (new subcommands, flags `--new-skill`, `--skill-id` for watch/attach/detach, `--all` for triage import, grouped list, show, check lines), `internal/delivery/cli/source_watch.go` (pass `SkillID`), `internal/delivery/cli/help.go`, `internal/delivery/cli/source_test.go`, `internal/delivery/cli/source_watch_test.go`, `internal/delivery/cli/ux_behavior_test.go` (only if an existing assertion pins old help text).

Do not modify any other file.

## Implementation steps

### Task 6.1 — `skill outdated` and `skill upstream`
- Steps: implement Requirements 1–2. Tests seed a workspace without network: create a skill through `SkillService`, write a meta with `provenance.source_id` and a `github` origin plus a source record (helpers in the test file), record `skill_upstream_state` through `sourcepkg.OperationalStore`, then run the CLI. Cases: empty workspace prints `No skills track an upstream repository.` and exits 0; one `changed` state prints a row containing `update available`; `--exit-code` exits 1 for it and 0 after the state is `same`; `--json` output decodes into `app.UpstreamListResult` with `status == "update_available"`; `skill upstream <local-skill>` exits 2.
- Verify: `go test ./internal/delivery/cli/ -run 'TestSkillOutdated|TestSkillUpstream' -count=1` exits 0 and prints `ok`.

### Task 6.2 — `skill update` and `skill confirm`
- Steps: implement Requirements 3–4. Flag parsing tests: `--accept SKILL.md=theirs` → exit 2 naming the allowed values; `--manual` with a missing file → exit 2; `--write-conflicts` with an existing target file → exit 2. Rendering test: build an `app.UpstreamUpdatePreview` value with one unresolved and one `upstream_only` file and assert the exact table rows and the three resolution hint lines; build one with pins and assert `Next: skillhub skill confirm <id>` and the trust notice.
- Verify: `go test ./internal/delivery/cli/ -run 'TestSkillUpdate' -count=1` exits 0 and prints `ok`.

### Task 6.3 — Source commands
- Steps: implement Requirement 5. Tests: `source list --json` contains `"groups"`; human list prints the repository heading and `upstream`; `source watch <github-url> --workspace <ws>` without `--skill-id` exits 2 with `A watched source must belong to a skill.`; restore the network-error case of `TestSourceWatchRemoteValidationAndJSON` by passing `--skill-id <existing skill>` and expecting the previous repository error text; `source backfill` on a workspace without candidates prints `Nothing to backfill.` and exits 0; `source unwatch missing-id` exits 2.
- Verify: `go test ./internal/delivery/cli/ -run 'TestSource' -count=1` exits 0 and prints `ok`.

### Task 6.4 — Help and width
- Steps: implement Requirements 6–7. Extend the existing help test (or `ux_behavior_test.go` help case) to assert `skill update <id>`, `skill outdated`, `source backfill`, and the `skillhub update` distinction line appear. Run the width test. Add one rendering test where a file path contains `\x1b[31m` and a newline and assert the output contains `\x1b` escaped as text, not the raw byte.
- Verify: `go test ./internal/delivery/cli/ -count=1` exits 0 and prints `ok`.

### Task 6.5 — Gate
- Verify: `make check` exits 0.

## Todo

- [x] Task 6.1 outdated + upstream
- [x] Task 6.2 update + confirm
- [x] Task 6.3 source commands
- [x] Task 6.4 help + width
- [x] Task 6.5 `make check`

## Success criteria

Every new command has help text, `--json`, documented exit codes, and a `Next:` hint where an action follows.

## UX acceptance

`skillhub skill outdated`:

```text
2 of 5 repository skills need attention. Checked 2h ago.

SKILL  SOURCE                  CURRENT  LATEST   UPDATED     CHANGED  LOCAL     STATUS
pdf    anthropics-skills@main  3f9c2a1  81d04be  2026-10-03  2 files  clean     update available
docx   anthropics-skills@main  3f9c2a1  81d04be  2026-10-03  1 file   modified  diverged

Next: skillhub skill update pdf
```

Nothing to report: `All 5 repository skills are up to date. Checked 2h ago.` Never checked: `3 repository skills have not been checked yet.` then `Next: skillhub skill outdated --check`. No repository skills: `No skills track an upstream repository.` then `Next: skillhub skill add <github-url>`.

`skillhub skill upstream pdf`:

```text
pdf tracks anthropics-skills (https://github.com/anthropics/skills@main, skills/pdf).
Status   update available (local copy is clean)
Current  3f9c2a1b4d5e
Latest   81d04be7c2aa, committed 2026-10-03 (checked 2h ago)
Changed upstream (2):
  modified  SKILL.md
  added     scripts/fill.py

Next: skillhub skill update pdf
```

`skillhub skill update docx` with a conflict:

```text
Update docx from 3f9c2a1 to 81d04be (https://github.com/anthropics/skills, source anthropics-skills), upstream commit of 2026-10-03.

FILE            CHANGE                     ACTION
SKILL.md        changed here and upstream  needs decision (1 conflict)
scripts/run.sh  changed upstream           take upstream
4 file(s) unchanged.

1 file(s) need a decision before this update can be applied. For each, pass one of:
  --accept SKILL.md=upstream   take the upstream file (drops your edits in it)
  --accept SKILL.md=local      keep your file
  --manual SKILL.md=<file>     use a file you resolved yourself (no conflict markers)
To edit the conflict: skillhub skill update docx --write-conflicts ./docx-conflicts
```

Clean preview tail:

```text
After applying, agents cannot use pdf until you approve the new content:
  skillhub skill review pdf
No collection files changed.
Next: skillhub skill confirm PROP-…
```

`skillhub source list`:

```text
2 sources from 2 repositories; 1 candidate in intake.

https://github.com/anthropics/skills
  SOURCE             REF   ROLE      SKILLS                  CHECKED  WATCH
  anthropics-skills  main  upstream  pdf (update), docx, xlsx 2h ago   weekly

https://github.com/acme/agent-docs
  acme-agent-docs    main  learning  review-code             never    weekly

Intake: SRC-7K2M https://github.com/foo/bar (pending)
Next: skillhub skill outdated
```

An orphan row shows `no skills` in SKILLS and the `Next:` line becomes `skillhub source attach <id> --skill-id <skill>` or `skillhub source unwatch <id>`.

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Wide tables exceed narrow terminals | M×L | `termui.Table` wraps only the last column (`internal/delivery/cli/termui/termui.go:185-262`), so the caller truncates the SKILLS cell to 40 runes with `…` before rendering; `human_output_width_test.go` covers width. |
| `skill update` vs `skillhub update` confusion | M×M | Help distinction line; `skill update` without an ID fails with a hint naming both. |
| `--write-conflicts` path traversal | L×H | Resolve each path under the target directory with `filepath.Rel` checks; refuse escapes and existing files. |

## Security considerations

`--manual` and `--write-conflicts` touch only user-named paths; no upstream bytes are executed. Approval stays `skill edit --approve-content`.

## Rollback

Revert the phase commit; services from phases 3–5 remain callable through the WebUI and tests (upstream updates have no MCP path).

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
