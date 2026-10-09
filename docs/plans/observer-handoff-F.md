# Observer handoff after Worktree F (review fixes on main at 93cc888)

Context for the next observer agent (§5.1 profile split, Phase 5 (a), Phase 3).

## Gotchas and Concurrency Rules

- **Worker queue context**: Do NOT pass caller request context (`ctx`) into worker ops; MCP tool calls cancel `ctx` upon return. Worker ops must use `context.WithTimeout(context.Background(), opTimeout)`.
- **Gate vs. Purge ordering**: `Purge` must hold `gate.Lock()` while queuing/waiting on the worker so racing caller `RecordCase`/`Record` (under `gate.RLock()`) cannot reopen or rewrite SQLite files during/after store deletion.
- **Transactions & fail-closed caps**: `recordCaseStore` runs on the worker in a single `BEGIN IMMEDIATE` transaction. Always fail closed (`return err`) if count queries fail. Total cap eviction must delete `totalCount - limit + 1` rows, not 1.
- **Redactor ordering**: Match credentials strictly: header `(?i)\bAuthorization:\s*(?:Basic|Bearer)\s+\S+` first, then case-sensitive standalone `Basic\s+[A-Za-z0-9+/=]{8,}` and `Bearer\s+<token-with-digit>` ($\ge 16$ chars), and `sk-<token-with-digit>` ($\ge 20$ chars). Without `(?i)`, prose words `basic`/`bearer` stay intact.
- **Rollup metric keys vs. skills**: Rollups with non-empty `skill_id` populate `SkillFunnel` entries. Operational server metrics like `tools_list_bytes` must use empty `skill_id` and formatted metric `tools_list_bytes:<client>` to keep client names out of the funnel skills list.
- **Transcript client attribution**: `transcript_import` sets `Client{Name: "skillhub"}` and `source: "claude-code"`. Baseline bucket mapping must map source `claude`/`claude-code` to client `"claude-code"` and never spawn buckets from counts alone.
- **Test clock races**: Recorders share `config.Clock()` with the background worker. Test mutable clocks must be guarded by a mutex (see `usage_test.go`).

## Noticed but Unfixed (Out of Scope / Worktree C Ownership)

- `internal/delivery/mcpserver/server.go:221,266`: Returns error code `snapshot_expired` for malformed/missing URI parameters (`> 4096` bytes or empty) instead of `invalid_params`.
- `internal/delivery/mcpserver/server.go:450-452`: `snapshot_expired_requests` counts `skill.ErrNotFound` alongside `skill.ErrSnapshotExpired` (detailed in `followup-worktree-c.md`).

## Slow and Flaky Tests

- `go test -count=1 ./internal/source -run TestGitRevisionAt`: Flakes intermittently with shallow fetch network broken pipe. Safe to rerun once.
- `go test -race ./internal/app ./internal/telemetry ./internal/delivery/mcpserver`: Serial runtime ~60–80s due to SQLite disk locks and in-memory MCP transports.
