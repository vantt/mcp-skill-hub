# Phase 1 report: measure and slim one `mcp serve` process (2026-10-11)

Plan: `plans/261011-0455-local-shared-hub-process/`. All tables, commands and the Q1 reasoning are in `plan.md`, section "Phase 1 result and decision Q1". This report covers only the outcome and the open questions.

## Outcome

- **Q1 recommendation: stop Phases 2 to 4.** One session really costs about 14 to 15 MB idle and 17 to 18 MB working (PSS across 20 processes), which is below the 20 MB threshold. The earlier figure of 32 to 42 MB per process (0.65 to 0.85 GB for 20) added up RSS, which counts about 18 MB of shared binary pages in every process. The real total for 20 sessions is 0.29 to 0.36 GB.
- **One production change kept.** `Serve` now returns freed heap to the OS once no MCP request has arrived for 30 s. It saves 2.7 MB per idle process (35.2 to 32.5-32.8 MB private, run twice) and about 45 to 51 MB across 20 idle sessions. Latency does not change, because the release never runs while a request is in progress.
- **Rejected:** `GOMEMLIMIT` (breaks the +5 ms p95 budget), `GOGC=50` (only 0.5 MB more on top of the idle release), lazy telemetry open (the difference is noise), and a smaller SQLite cache (at most about 2 MB, and it would need changes in other packages).
- **Web UI cost in an `mcp serve` process (Phase 6 baseline):** almost nothing in memory. Init is 0.006 to 0.024 ms and 2 KB. The 5.6 MB of assets are never loaded. A comparison binary that links only the MCP server is 8.2 MB smaller on disk but only 1.6 MB smaller resident in a lone process, and close to 0 per session in a fan-out.

## Pre-existing problems found (outside Phase 1 file ownership, not fixed)

1. **Telemetry loss at 10 calls per second, already present in today's code.** 0 to 6 of 12,000 events are dropped, and not on every run. Every write batch opens SQLite again, runs `PRAGMA quick_check` and reruns the schema statements; with 20 writers, some batches exceed the 5 s operation timeout. The fix belongs in `internal/telemetry/store.go` (`writeEvents`, `openDatabase`). The earlier "0 lost at 10/s" result was a lucky run.
2. **Each resolve rereads the whole workspace** to check catalog freshness (`catalog.InspectWhileLocked` → `canonical.Scan`, about 0.42 MB of allocation per resolve with 32 skills). This is the largest per-resolve cost and grows with hub size.

## Files

- `internal/delivery/mcpserver/server.go`: idle memory releaser (`idleMemoryReleaser`, used by `Serve`)
- `internal/delivery/mcpserver/memory_budget_test.go`: releaser unit tests (always run), plus the single-process memory budget, heap profile and fan-out driver (only with `SKILLHUB_PERF=1`)
- `scripts/mcp-fanout-bench.sh`, plus the `make mcp-fanout` target in `Makefile`
- `plans/261011-0455-local-shared-hub-process/plan.md` (new Phase 1 result section; phase table not touched) and `phase-01-memory-per-process.md` (todos ticked)

## Verification

- `make check`: fmt-check, vet, lint (0 issues) and the full test suite all pass.
- Releaser unit tests pass with `-race -count=3`.
- Not run: the full `make test-perf` suite. Only the new perf tests were run, several times.

## Open questions for the user

- Q1: do you accept stopping Phases 2 to 4 based on the numbers above?
- Q2: if the daemon goes ahead anyway, how does a proxy reach it? (Recommended: a 0600 Unix socket; on Windows, loopback plus a token in a 0600 file.)
- Q3: if the daemon goes ahead, how long should it wait with zero sessions before exiting? (Recommended: 10 minutes, configurable.)
- Q4: should hosts that can speak MCP over HTTP skip the stdio proxy? Not checked in Phase 1 for Claude Code, Codex or Gemini CLI.
- Should the two pre-existing problems above (telemetry writer, catalog freshness scan) become their own small tasks?
