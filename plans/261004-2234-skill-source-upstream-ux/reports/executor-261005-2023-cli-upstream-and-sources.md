# Executor Report: Phase 6 - CLI: Upstream and Sources

**Phase:** Phase 6: CLI: upstream and sources
**Branch:** `feat/skill-source-upstream`
**Timestamp:** 2026-10-05 20:23 Asia/Saigon

## 1. What Changed

1. **`skill outdated` & `skill upstream` (`internal/delivery/cli/skill_upstream.go`, `internal/delivery/cli/skill.go`)**:
   - Implemented `skill outdated [--check] [--all] [--exit-code] [--json]`: shows repository skills differing from upstream, checks tracked sources on demand, formats columns (`SKILL`, `SOURCE`, `CURRENT`, `LATEST`, `UPDATED`, `CHANGED`, `LOCAL`, `STATUS`), and returns exit code 1 with `--exit-code` when attention is required.
   - Implemented `skill upstream <id> [--check] [--json]`: shows full tracking and drift detail for a repository skill, checking its source first when `--check` is passed, and returns exit code 2 on non-repository skills.
   - Defined `UpstreamListResult` for `--json` outputs.

2. **`skill update` & `skill confirm` (`internal/delivery/cli/skill_upstream.go`, `internal/delivery/cli/skill.go`)**:
   - Implemented `skill update <id> [--no-check] [--target <commit>] [--accept <path>=upstream|local|merged]... [--manual <path>=<file>]... [--write-conflicts <dir>] [--yes] [--verbose] [--json]`: performs 3-way merge updates from upstream, formats conflict tables and resolution hints, exports conflict files safely with `os.OpenRoot`, and confirms immediately when `--yes` is combined with zero unresolved conflicts.
   - Updated `skill confirm` dispatch to handle `app.UpstreamUpdateResult`, printing its summary and recommending `skill review <id>` when review is required after apply.

3. **Source CLI Commands (`internal/delivery/cli/source.go`)**:
   - Implemented `source attach <source-id|locator> --skill-id <id> [--yes]`, `source detach <source-id> --skill-id <id> [--yes]`, and `source unwatch <source-id> [--yes]`.
   - Implemented `source backfill [--skill <id> [--path <repo-path>]] [--yes]`, previewing actions and applying legacy backfills.
   - Updated `source list` to render grouped sources by repository with orphan source detection and hints.
   - Updated `source show <id>` to display roles, referencing skills, and last check timestamp.
   - Updated `check` and `source check` to print indented `<skill>: <status words>` lines under source bullets.

4. **Help Text and Untrusted String Escaping (`internal/delivery/cli/help.go`, `internal/delivery/cli/skill_upstream.go`)**:
   - Updated global usage and command usage for `skill`, `source`, and `check`.
   - Added distinction note between `skillhub skill update <id>` and `skillhub update`.
   - Implemented `printableText` replacing non-printable runes (including ANSI ESC and newlines) with `\xNN` escapes.

5. **Test Coverage (`internal/delivery/cli/skill_upstream_test.go`, `internal/delivery/cli/source_test.go`)**:
   - Added comprehensive tests for `outdated`, `upstream`, `update`, `confirm`, `source list`, `source attach/detach/unwatch/backfill`, help text, and terminal control sequence escaping.

## 2. Commands Executed and Results

| Command | Purpose | Result |
|---|---|---|
| `git branch --show-current` | Verify branch | `feat/skill-source-upstream` |
| `go test ./internal/delivery/cli/ -run 'TestSkillOutdated\|TestSkillUpstream' -count=1` | Task 6.1 verification | Pass (exit 0) |
| `go test ./internal/delivery/cli/ -run 'TestSkillUpdate' -count=1` | Task 6.2 verification | Pass (exit 0) |
| `go test ./internal/delivery/cli/ -run 'TestSource' -count=1` | Task 6.3 verification | Pass (exit 0) |
| `go test ./internal/delivery/cli/ -count=1` | Task 6.4 verification | Pass (exit 0) |
| `make check` | Task 6.5 gate verification (vet, golangci-lint, full test suite) | Pass (exit 0) |

## 3. Deviations and Why

None. All implementations and constraints strictly adhered to the Phase 6 plan and architecture decisions.

## 4. Open Questions

None.
