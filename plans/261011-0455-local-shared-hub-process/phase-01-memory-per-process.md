---
phase: 1
title: "Measure and slim one process"
status: pending
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 1: Measure and slim one process

## Goal
Find out why a working `mcp serve` process uses 32 to 42 MB when an idle one uses 12 MB, shrink it if that is cheap, and decide with numbers whether Phases 2 to 4 are worth building.

## Files to Create / Modify
- Create: `internal/delivery/mcpserver/memory_budget_test.go` (skipped unless `SKILLHUB_PERF=1`, like `internal/catalog/performance_test.go`; starts one `mcp serve` over stdio in a temp workspace, drives `skill_resolve`, reads the process's resident memory)
- Create: `scripts/mcp-fanout-bench.sh` and `make mcp-fanout` (N processes, each calling `skill_resolve` at a fixed rate; reports total memory, latency and telemetry events written against expected; uses only a temp workspace and an isolated `HOME` and `XDG_*`)
- Modify (only if a measured gain): `internal/delivery/mcpserver/server.go` (`Serve`) or `cmd/skillhub` start-up, for memory limits or releasing memory when idle

## Tasks & Steps
1. Turn the 2026-10-11 throwaway harness into the two files above, so the numbers are repeatable. Record, for one process: resident memory right after start, after one resolve, after 1,000 resolves, and after 60 seconds idle.
2. Find where the memory goes with a heap profile (`pprof`) on a working process: catalog load, SQLite telemetry (WAL, page cache), the resolver, Go runtime free-but-retained memory.
3. Try the cheap options one at a time and keep only those with a clear gain: a soft memory limit (`GOMEMLIMIT` or `debug.SetMemoryLimit`), returning memory to the OS after idle (`debug.FreeOSMemory`), a smaller SQLite cache, opening telemetry lazily on the first event.
4. Re-run the 20-process fan-out and record total memory before and after.
5. Write the go or stop for Phases 2 to 4 in `plan.md` (decision Q1), and ask the user Q2 to Q4.

## Verification
- A table of before and after memory exists with the command that produced it, run twice with similar results.
- No change lengthens the p95 of `skill_resolve` beyond 5 ms over the baseline (about 9 ms today) or loses a telemetry event at the realistic rates already tested.
- `make check` stays green.
