# Phase 7: Funnel aggregation, CLI, WebUI Usage tab report

## Summary
- Fixed double-counting of `feedback:negative` and `feedback:negative_after_load` in `internal/telemetry/rollup.go`: companion `skill.utility_reported` events now query their corresponding primary event in the same transaction; if the primary status was negative (`failed`, `rejected`, `abandoned`), the negative feedback metric is not incremented a second time.
- Implemented `app.UsageService.Funnel` in `internal/app/usage.go`:
  - Normalizes and clamps date windows up to 180 days (defaulting to 30 days).
  - Aggregates daily rollups across overall workspace and per-skill metrics (resolutions, recommendations, activations by attribution, acceptance rate, overrides, misses, unsolicited, blocked loads, setup requirements, loads by resource kind, doctor runs and failure rates, setup failures and rates, negative feedback, and transcripts).
  - Derives actionable list classifications: `dead_skills` (active with 0 recommendations, loads, and blocked loads in window), `recommended_never_activated`, `blocked_by_review` (sorted by blocked and setup review count descending), `negative_after_load`, and `setup_failures`.
- Added CLI command `skillhub telemetry funnel` (`internal/delivery/cli/telemetry_funnel.go`):
  - Flags: `--since <Nd|YYYY-MM-DD>`, `--until <YYYY-MM-DD>`, `--skill <id>`, `--workspace <path>`, `--json`.
  - Formatted human output: Overall summary, per-skill table (top 20 or single skill), and list categories. Exits 2 on invalid flags or unknown skills.
  - Updated CLI help in `internal/delivery/cli/help.go`.
- Added Web API endpoint `GET /api/v1/skills/{id}/usage?since=<7d|30d|90d|180d>` (`internal/delivery/web/routes_usage.go`):
  - Returns per-skill `FunnelReport` with `overall` omitted.
  - Unknown skill returns 404; invalid `since` returns 400.
  - Generated golden response `internal/delivery/web/testdata/golden/skill-usage.json`.
- Implemented frontend Usage tab and `UsagePanel` in `web/`:
  - Added types `FunnelSince`, `FunnelWindow`, `SkillFunnel`, and `FunnelReport` in `web/src/api/types.ts`.
  - Added `useSkillUsage` React Query hook in `web/src/api/queries.ts`.
  - Added Usage tab button and panel rendering in `SkillDetailScreen.tsx`.
  - Implemented `UsagePanel.tsx` with window selector (7d, 30d, 90d, 180d), basis captions (`Basis: server-observed`, `Basis: host-reported`, `Basis: terminal`), stat rows, and `EmptyState` when all counters are zero.

## Verification
- Unit & integration tests:
  - `go test -count=1 ./internal/telemetry/` (PASS)
  - `go test -count=1 -run Usage ./internal/app/` (PASS)
  - `go test -count=1 -run 'Funnel|Help' ./internal/delivery/cli/` (PASS)
  - `go test -count=1 ./internal/delivery/web/` (PASS)
  - `make web-test` (typecheck, eslint, vitest) (PASS)
  - `make web-build` (PASS)
  - `make check` (go vet, golangci-lint 0 issues, go test full suite) (PASS)
- Acceptance criteria:
  - `skillhub telemetry funnel --json --since 7d` in clean workspace output verified: all counts 0, rates null, exit 0.
  - Third-party unapproved skill correctly categorized under `blocked_by_review` and not `recommended_never_activated`.
