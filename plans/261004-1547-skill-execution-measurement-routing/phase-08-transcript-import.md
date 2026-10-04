---
phase: 8
title: "Claude Code transcript import"
status: pending
priority: P2
effort: 8h
dependencies: [2, 7]
---

# Phase 8: Claude Code transcript import

## Context

- [plan.md](./plan.md) "Verified starting facts" (transcript line shape, dedupe key, subagent files). Reference implementation: `~/projects/forgentX/packages/observe/rust/src/sources/claude_transcripts.rs` (`encode_project_dir` replaces `/` and `.` with `-`; honors `CLAUDE_CONFIG_DIR`, else `$HOME/.claude`; matches `<enc>` and `<enc>--claude-worktrees-*` dirs; filters by record `cwd`; skips files whose mtime predates the window minus one day).
- Verified tool_use shapes in local transcripts: `{"type":"tool_use","id":"toolu_…","name":"Skill","input":{"skill":"ak-plan","args":"…"}}`; `name":"mcp__skillhub__skill_get","input":{"skill_id":"…"}`; `mcp__skillhub__skill_resolve`. The MCP server key is `skillhub` (`internal/hostintegration/integration.go:242`).
- Event type `transcript.tool_observed` and rollup metrics from Phase 2. Telemetry stays `content_mode none`.

## Requirements

1. New package `internal/transcripts` (no app dependency):
   - `ProjectDirs(configDir, projectRoot string) ([]string, error)`: canonicalize the root, add `git worktree list --porcelain` paths when git is available (bounded 5 s timeout; failure ignored), encode each, match directories under `<configDir>/projects/`, including the `--claude-worktrees-` prefix.
   - `Scan(dirs, window, validCwds) ([]Observation, error)`: read `*.jsonl` in each dir and `*/subagents/*.jsonl` one level down; skip lines without `"tool_use"`; decode only the fields needed (`type`, `timestamp`, `cwd`, `sessionId`, `message.content[]` blocks of type `tool_use` with `id`, `name`, `input`); keep records whose `cwd` is inside a valid cwd (records without `cwd` are kept, as in forgentX); filter by timestamp window; line size cap 4 MiB; malformed lines skipped and counted.
   - `Observation{ToolUseID, Tool, SkillID, SessionID, At}` where: `mcp__skillhub__<tool>` → `Tool=<tool>`, `SkillID=input.skill_id` when present; `Skill` → `Tool="Skill"`, `SkillID=input.skill`; `ReadMcpResourceTool` with `input.server=="skillhub"` → `Tool="resources_read"`, `SkillID` parsed from the `skill://skillhub/<digest>/<id>/…` URI. Everything else is ignored. No other input field is read into memory beyond decoding.
   - Dedupe by `ToolUseID` within the scan.
   - `resolved_before`: for `Skill` observations, true when the same session had a `skill_resolve` observation in the preceding 30 minutes.
2. `app.TranscriptImportService.Import(ctx, workspace, ImportInput{Project string; Since time.Time; ConfigDir string})`:
   - `Since` defaults to now − 14 days and is clamped to the raw retention window, so a re-import can never re-insert events whose raw rows were already pruned (which would double-count rollups).
   - Records `transcript.tool_observed` with `ID = "transcript-" + hex(sha256(tool_use_id))[:32]` (deterministic, so re-imports dedupe through `INSERT OR IGNORE`), `OccurredAt = At`, `SessionIDHash = hex(sha256("claude-code:" + sessionId))[:32]`, payload `tool`, `skill_id` (only when it is a valid token), `source=claude-code`, `basis=transcript`, `resolved_before`.
   - Returns counts: files scanned, lines skipped, observations, inserted, duplicates, per tool.
3. CLI `skillhub telemetry import-transcripts --project <dir> [--since <Nd|date>] [--workspace <path>] [--json]`. `--project` is required (explicit opt-in, no daemon, no default scanning of all projects). Output states that only tool name, skill ID, timestamp, and a session hash were stored.

## Files

Create:
- `internal/transcripts/claude_code.go`, `internal/transcripts/claude_code_test.go`
- `internal/transcripts/testdata/` (synthetic JSONL: multi-line message IDs, subagent file, foreign cwd, malformed line, `Skill`, `mcp__skillhub__skill_get`, `ReadMcpResourceTool`)
- `internal/app/transcript_import.go`, `internal/app/transcript_import_test.go`
- `internal/delivery/cli/telemetry_transcripts.go`, `internal/delivery/cli/telemetry_transcripts_test.go`

Modify:
- `internal/delivery/cli/telemetry.go` (dispatch `import-transcripts`)
- `internal/delivery/cli/help.go` (`telemetry` usage)

The funnel already reads `transcript:*` and `native:*` rollup metrics (Phase 7), so `internal/app/usage.go` needs no change.

## Steps

1. Encoding and directory matching tests (including `.` in paths and `CLAUDE_CONFIG_DIR`).
2. Scanner with fixtures; assert two tool_use blocks sharing one `message.id` both survive and a repeated `tool_use.id` collapses.
3. Import service: re-run imports zero new rows; events older than the window are not imported.
4. CLI wiring and help.

## Tests and validation

- `go test ./internal/transcripts/ ./internal/app/ -run Transcript ./internal/delivery/cli/ -run Transcript`
- Privacy test: fixture `args` and file contents containing a sentinel never appear in `telemetry.db` (scan the raw DB bytes).
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Claude Code transcript format changes | Medium × Low | Tolerant decoding; unknown shapes are skipped and counted; import is optional. |
| Transcripts contain secrets | Medium × High | Only allowlisted fields are kept; payload validated by the telemetry allowlist; sentinel test. |
| Double counting after raw prune | Low × Medium | Window clamp to raw retention plus deterministic event IDs. |

## Rollback

Revert; imported events age out in 14 days, rollups in 180 days, or `skillhub telemetry purge`.
