# Observer handoff after wave 2 (D) and wave 3 (E)

Input for the next observer prompt (F: §5.1 profile split, Phase 5 (a), observer Phase 3),
which runs after simplify Phase 3 merges. Main at c5696e4 when written (2026-10-09).

## Weak spots and gotchas

- Tracker RAM (`notedResolution` in activationTracker): 1h TTL, 16 resolutions per session,
  lost on restart. A restart between `skill_resolve` and `skills/get` turns the load into
  `unsolicited` with empty top-k. Read `unsolicited_share` per baseline bucket before calling
  anything an ignore.
- A load is attributed only to the newest resolution in its chain. Blocked
  (`review_required`) loads are left out of `totalLoads`.
- Case journal flag: read on every `RecordCase` (`SKILLHUB_CASE_JOURNAL`, then
  `<workspace>/runtime/case_journal.json`). Event `content_mode` is always `none`.
  Case limits live in `telemetry.Config` (50/day, 500 total, 90 days).
- `topk_channels` on load events are joined with `+` to fit the token regex; split on `+`.
- SQLite store uses `busy_timeout(5000)` and WAL. Keep that when a new code path opens the DB
  (for example `RecordCase` during a batch flush), or you get `SQLITE_BUSY`.
- `internal/source` `TestGitRevisionAt` can flake with a network broken pipe. Rerun once.

## Re-measure tools/list after simplify Phase 3

`go test -v ./internal/delivery/mcpserver -run TestToolsListSize`

Compare with docs/plans/observer-phase-01b-baseline.md §6:

| Set | 2026-10-09 |
|---|---|
| All tools | 43 tools, 223,867 bytes (~56k tokens), outputSchema 79.1% |
| Runtime {skill_resolve, skill_get, skill_feedback} | 18,681 bytes (~4.7k tokens), outputSchema 60.2%; 7,432 bytes without it |

## Baseline (O7)

`skillhub telemetry baseline [--min-chains 30] [--json] [--write <path>]`. Counting starts
when the live hub runs a v3 binary (the snapshot changes on migration). Capture the report
only when the newest snapshot's main clients are `sufficient`.
