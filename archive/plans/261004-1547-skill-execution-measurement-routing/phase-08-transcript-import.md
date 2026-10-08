---
phase: 8
title: "Claude Code transcript import"
status: done
priority: P2
effort: 8h
dependencies: [2, 7]
---

# Phase 8: Claude Code transcript import

## Goal

An explicit, opt-in command reads local Claude Code transcripts for one project and records only which skill tools were called (tool name, skill ID, timestamp, session hash), so the funnel can compare hub-observed activations with what the host actually did, including native `Skill` use without a hub resolve.

## Context (read these first)

- `plan.md` → "Executor notes", "Verified starting facts" (one JSONL line per content block; dedupe by `tool_use.id`; subagent files).
- Reference implementation (behavior only, do not copy code): `~/projects/forgentX/packages/observe/rust/src/sources/claude_transcripts.rs` — `encode_project_dir` replaces `/` and `.` with `-`; honors `CLAUDE_CONFIG_DIR`, else `$HOME/.claude`; matches `<enc>` and `<enc>--claude-worktrees-*`; filters by record `cwd`; skips files whose mtime predates the window minus one day (verified: `encode_project_dir` at `:117`, `--claude-worktrees-` prefix at `:151`, `get_valid_cwds` at `:82`, mtime window at `:202-207`). If the file is unavailable, implement from this list.
- Tool-use shapes in local transcripts: `{"type":"tool_use","id":"toolu_…","name":"Skill","input":{"skill":"<id>","args":"…"}}`; `"name":"mcp__skillhub__skill_get","input":{"skill_id":"…"}`; `mcp__skillhub__skill_resolve`; `ReadMcpResourceTool` with `input.server` and `input.uri`. The MCP server key is `skillhub` (`internal/hostintegration/integration.go:280,288`).
- Skill resource URIs: `skill://skillhub/<manifest>/<id>/<path>` (`internal/delivery/mcpserver/server.go:151` URI template; parser `parseDistributedURI` in `internal/app/distribution.go:566`).
- Telemetry: event type `telemetry.EventTranscriptToolObserved` (`internal/telemetry/events.go:35`), payload allowlist `tool, skill_id, source, basis, resolved_before` (`events.go:155`), required `source = telemetry.TranscriptSourceClaude` ("claude-code") and `basis = telemetry.TranscriptBasis` ("transcript"). Rollups already map it to `transcript:<tool>` and, for `Skill`, `native:no_resolve` / `native:resolved_before` (`internal/telemetry/rollup.go`). Raw retention is 14 days (`internal/telemetry/recorder.go:18`).
- Event IDs: `INSERT OR IGNORE` dedupes by event ID (`insertEventOrIgnoreSQL`, `rollup.go`), so deterministic IDs make re-imports idempotent.
- CLI: `internal/delivery/cli/telemetry.go` (`runTelemetry` `:34`), help `internal/delivery/cli/help.go:269`. Phase 7 added `funnel` there; keep its dispatch intact.
- Interaction with content trust: a transcript may show `skill_get`/`ReadMcpResourceTool` calls for a `review_required` skill. They are recorded like any other call (the transcript records the attempt; the hub's own `blocked:review_required` metric shows the refusal).

## Requirements

1. **Package `internal/transcripts`** (no dependency on `internal/app`):
   - `ProjectDirs(configDir, projectRoot string) ([]string, error)`: canonicalize the root (`filepath.EvalSymlinks`), add `git worktree list --porcelain` paths when git is available (5 s timeout; failure ignored), encode each, match directories under `<configDir>/projects/` including the `--claude-worktrees-` prefix.
   - `Scan(dirs []string, since, until time.Time, validCwds []string) (ScanResult, error)`: read `*.jsonl` in each dir and `*/subagents/*.jsonl` one level down; skip lines without `"tool_use"`; decode only `type`, `timestamp`, `cwd`, `sessionId`, and `message.content[]` blocks of type `tool_use` (`id`, `name`, `input`); keep records whose `cwd` is inside a valid cwd (records without `cwd` are kept); filter by timestamp; line cap 4 MiB; malformed lines skipped and counted.
   - `Observation{ToolUseID, Tool, SkillID, SessionID string; At time.Time}`: `mcp__skillhub__<tool>` → `Tool=<tool>`, `SkillID=input.skill_id`; `Skill` → `Tool="Skill"`, `SkillID=input.skill`; `ReadMcpResourceTool` with `input.server=="skillhub"` → `Tool="resources_read"`, `SkillID` parsed from the URI. Everything else is ignored. No other input field is kept.
   - Dedupe by `ToolUseID` within the scan. `ResolvedBefore` for `Skill` observations: same session had a `skill_resolve` observation in the preceding 30 minutes.
2. **`app.TranscriptImportService.Import(ctx, workspace string, in ImportInput) (ImportResult, error)`**, `ImportInput{Project string; Since time.Time; ConfigDir string}`:
   - `Since` defaults to now − 14 days and is clamped to the raw retention window so a re-import never re-inserts events whose raw rows were pruned (which would double-count rollups).
   - Records `transcript.tool_observed` with `ID = "transcript-" + hex(sha256(tool_use_id))[:32]`, `OccurredAt = At`, `SessionIDHash = hex(sha256("claude-code:" + sessionId))[:32]`, payload `tool`, `skill_id` (only when it matches the telemetry token pattern), `source`, `basis`, `resolved_before`.
   - Returns counts: files scanned, lines skipped, observations, inserted, duplicates, per tool.
3. **CLI** `skillhub telemetry import-transcripts --project <dir> [--since <Nd|YYYY-MM-DD>] [--workspace <path>] [--json]`. `--project` is required (no default scanning). Output states that only tool name, skill ID, timestamp, and a session hash were stored.

## Files

Create:
- `internal/transcripts/claude_code.go`, `internal/transcripts/claude_code_test.go`
- `internal/transcripts/testdata/` (synthetic JSONL: two tool_use blocks sharing one `message.id` on separate lines, a repeated `tool_use.id`, a subagent file, a foreign `cwd`, a malformed line, `Skill`, `mcp__skillhub__skill_get`, `mcp__skillhub__skill_resolve`, `ReadMcpResourceTool`, and an `args` value containing the sentinel `SENTINEL-TRANSCRIPT-SECRET`)
- `internal/app/transcript_import.go`, `internal/app/transcript_import_test.go`
- `internal/delivery/cli/telemetry_transcripts.go`, `internal/delivery/cli/telemetry_transcripts_test.go`

Modify:
- `internal/delivery/cli/telemetry.go` (dispatch `import-transcripts`), `internal/delivery/cli/help.go` (`telemetry` usage)

`internal/app/usage.go` needs no change: the funnel already reads `transcript:*` and `native:*`.

## Steps

- [x] **1. Directory encoding and matching** tests (paths with `.`, `CLAUDE_CONFIG_DIR`, worktree suffix).
  Pass: `go test -count=1 -run ProjectDirs ./internal/transcripts/` → `ok`.
- [x] **2. Scanner** with fixtures: both blocks sharing a `message.id` survive; repeated `tool_use.id` collapses; foreign `cwd` dropped; malformed line counted; subagent file read.
  Pass: `go test -count=1 ./internal/transcripts/` → `ok`.
- [x] **3. Import service:** second import inserts 0; events older than the clamp are not imported; per-tool counts.
  Pass: `go test -count=1 -run Transcript ./internal/app/` → `ok`.
- [x] **4. Privacy test:** after import, the raw bytes of `telemetry.db` (and its `-wal` if present) do not contain `SENTINEL-TRANSCRIPT-SECRET`.
  Pass: included in step 3 command → `ok`.
- [x] **5. CLI wiring and help.**
  Pass: `go test -count=1 -run 'Transcript|Help' ./internal/delivery/cli/` → `ok`; `go run ./cmd/skillhub help telemetry` lists `import-transcripts` and `funnel`.
- [x] **6. Gate.** Pass: `make check` exits 0.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Claude Code transcript format changes | Medium × Low | Tolerant decoding; unknown shapes skipped and counted; import is optional. |
| Transcripts contain secrets | Medium × High | Allowlisted fields only; payload validated by the telemetry allowlist; sentinel test. |
| Double counting after raw prune | Low × Medium | Window clamp to raw retention plus deterministic event IDs. |

## Rollback

Revert; imported events age out (14 days raw, 180 days rollups) or `skillhub telemetry purge --yes`.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol": stop, write `reports/<agent>-<YYMMDD-HHMM>-transcript-import.md`, set `status: blocked`, report the blocker.

## Implementation Note
Implemented the `internal/transcripts` package to discover project and worktree transcript folders under Claude Code's config directory and scan JSONL tool-use blocks without leaking sensitive arguments or tasks. Added `TranscriptImportService` in `internal/app` with a 14-day retention clamp, sha256-based deterministic event IDs, pre-insertion duplicate checks, and privacy validations. Exposed the functionality through `skillhub telemetry import-transcripts --project <dir>`, returning formatted counts and privacy notices.
