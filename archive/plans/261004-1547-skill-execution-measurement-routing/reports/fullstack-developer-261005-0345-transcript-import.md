# Phase 8: Claude Code transcript import report

## Summary
- Implemented `internal/transcripts`:
  - `ProjectDirs`: discovers project transcript directories under `<configDir>/projects/` using canonical root paths, worktree queries (`git worktree list --porcelain`), and encoded project directory names replacing path separators and dots with hyphens.
  - `Scan`: parses JSONL lines up to 4 MiB, decodes tool_use content blocks (including blocks sharing message IDs and nested subagent logs), filters records by canonical `cwd` and time window, maps observations (`mcp__skillhub__<tool>`, `Skill`, `ReadMcpResourceTool`), dedupes by `ToolUseID`, and determines `ResolvedBefore` for native `Skill` invocations occurring within 30 minutes of a `skill_resolve`.
  - Added test suite with synthetic fixtures covering deduplication, subagents, foreign cwds, malformed JSON lines, and privacy isolation.
- Implemented `app.TranscriptImportService.Import` in `internal/app/transcript_import.go`:
  - Enforces `--project <dir>` requirement and clamps `Since` to the raw retention cutoff (14 days) to prevent double-counting of pruned events.
  - Generates deterministic event IDs (`transcript-` + sha256 prefix) and hashed session IDs (`claude-code:` prefix).
  - Checks for existing event IDs in `telemetry.db` before insert, accurately reporting new vs duplicate counts.
  - Adds privacy assertions ensuring arguments and task contents (e.g. `SENTINEL-TRANSCRIPT-SECRET`) are never persisted to `telemetry.db` or its WAL.
- Implemented CLI subcommand `skillhub telemetry import-transcripts`:
  - Added flag parsing for `--project <dir>`, optional `--since <Nd|YYYY-MM-DD>`, `--workspace <path>`, and `--json`.
  - Added human-readable and JSON formatting, including explicit privacy notice output.
  - Updated CLI command help in `internal/delivery/cli/help.go`.

## Verification
- Unit & integration tests:
  - `go test -count=1 ./internal/transcripts/...` (PASS)
  - `go test -count=1 -run Transcript ./internal/app/` (PASS)
  - `go test -count=1 -run 'Transcript|Help' ./internal/delivery/cli/` (PASS)
  - `make check` (go vet, golangci-lint 0 issues, full test suite across all 24 packages) (PASS)
- Acceptance criteria:
  - Verified `telemetry.db` bytes contain no `SENTINEL-TRANSCRIPT-SECRET`.
  - Verified smoke run imports new observations on first run and 0 new on subsequent runs.
