---
title: "Local shared hub process: fewer processes and less memory when many agents run"
description: "Cut the memory cost of running many agent sessions on one machine: slim each stdio process first, then add an optional per-machine daemon that sessions attach to and that exits when idle, always falling back to today's in-process serving."
status: pending
priority: P3
effort: 3d
branch: main
tags: [mcp, performance, daemon, local]
blockedBy: []
blocks: []
created: 2026-10-11
---

# Local shared hub process

## Overview

Today every agent session starts its own `skillhub mcp serve` over stdio, and the process ends when the host closes stdin (verified: all test processes exit with code 0). The user runs up to 20 agents at once.

Measured on 2026-10-11 (Linux, throwaway workspace, 20 processes, see `plans/261010-1652-remote-hub-mcp-over-network/plan.md`, section "Measured today"):

| Fact | Number |
|---|---|
| Memory per process while working | about 32 to 42 MB (12 MB for the two idle ones on the user's machine) |
| Memory for 20 processes | about 0.65 to 0.85 GB |
| Telemetry loss at realistic call rates (up to about 190 calls per second in total) | none |
| Telemetry loss at about 5,300 calls per second | 82% dropped (cause not established; not realistic load) |
| Start-up per process | about 20 ms |

So the case for change is memory, not correctness. This plan does the cheap thing first (slim each process), and only then adds a shared process, in a shape that cannot make anything worse than today.

Not measured (the user chose to skip it): how many `mcp serve` processes exist during a real 20-agent session. If subagents share their parent's MCP connection the count may be far below 20, which would make Phases 2 to 4 unnecessary. Phase 1 ends with a go or stop on Phases 2 to 4 using the numbers it produces.

## Terms (so the phases read the same way)

- **Server**: a program that waits for requests and answers many clients. The HTTP MCP listener planned in the remote hub plan is one.
- **Daemon**: a server that runs in the background on its own, detached from whoever started it, with a lock so only one copy runs.
- **Proxy**: the small stdio command a host starts for each session (`skillhub mcp serve` as the host registers it today). In this plan it either attaches to the daemon or serves in-process.

## The four ideas and where they go

| Idea | Phase |
|---|---|
| 4. Slim each process before adding anything | 1 |
| 3. Reuse the HTTP server from the remote plan as the shared local server, so there is one server codebase | 2 and 3 (depends on the remote plan, Phase 2) |
| 2. The daemon is an optimisation, never a requirement: attach if possible, otherwise serve in-process as today | 3 |
| 5. The daemon exits by itself when it has had no sessions for a while | 4 |

## Decisions (Phase 1 asks the user after the numbers are in)

| # | Question | Recommended |
|---|---|---|
| Q1 | Is a daemon worth building at all? | Decide from Phase 1 numbers: go only if memory per process stays above about 20 MB after slimming and the user really runs many sessions |
| Q2 | How does a proxy reach the daemon on the same machine? | A Unix socket with mode 0600 on Linux and macOS; on Windows a loopback address with a token from a 0600 file (verify that Go's `net` unix sockets work on the CI Windows image before choosing) |
| Q3 | Idle time before the daemon exits | 10 minutes with zero sessions, configurable |
| Q4 | Should hosts that support MCP over HTTP skip the proxy? | Only after Phase 1 checks which hosts can; until then every session keeps a stdio proxy |

## Phase 1 result and decision Q1 (measured 2026-10-11)

**Recommendation for Q1: stop Phases 2 to 4.** Measured correctly, one `mcp serve` session really costs about 14 to 15 MB when idle and 17 to 18 MB while working. That is below the 20 MB threshold, so the threshold says not to build a daemon. The earlier figure of 32 to 42 MB per process (0.65 to 0.85 GB for 20) came from adding up RSS. RSS counts the roughly 18 MB of `skillhub` binary pages in every process, but those pages are loaded once and shared by every process that runs the same file. PSS (proportional set size) splits shared pages between the processes that use them, so summed PSS is the real total: about 0.29 to 0.36 GB for 20 sessions. Even a perfect daemon could save at most about 0.25 GB. If hosts still need a stdio proxy per session, it saves less, because each proxy is itself a Go process; how much less is not measured.

Setup: Linux, 24 cores, a temp workspace with 32 skills, isolated `HOME` and `XDG_*`, and `--profile runtime` (what hosts register). Resolves are sent one after another, as a host does. Memory is shown as RSS / PSS / private.

### One process: before and after

| Run | Right after start | After 1 resolve | After 1,001 resolves | After 60 s idle | Resolve p50 / p95 | Events written / expected |
|---|---|---|---|---|---|---|
| Before, run 1 | 26.7 / 24.9 / 24.9 MB | 28.1 / 26.4 / 26.4 MB | 38.1 / 36.3 / 36.3 MB | 37.0 / 35.2 / 35.2 MB | 6.3 / 14.1 ms | 2002 / 2002 |
| Before, run 2 | 27.6 / 25.9 / 25.8 MB | 28.6 / 26.8 / 26.8 MB | 37.1 / 35.3 / 35.3 MB | 37.0 / 35.2 / 35.2 MB | 7.4 / 16.0 ms | 2002 / 2002 |
| After, run 1 | 27.0 / 25.2 / 25.2 MB | 28.8 / 27.0 / 27.0 MB | 37.6 / 35.8 / 35.8 MB | **34.5 / 32.8 / 32.7 MB** | 5.1 / 16.6 ms | 2002 / 2002 |
| After, run 2 | 27.1 / 25.3 / 25.3 MB | 28.8 / 27.0 / 27.0 MB | 37.8 / 36.0 / 36.0 MB | **34.3 / 32.5 / 32.5 MB** | 6.8 / 14.8 ms | 2002 / 2002 |

With a single process, PSS is close to RSS because no other process shares the binary pages. Each `skill_resolve` still writes 2 telemetry events.

Where the memory of one working process goes, read from `/proc/<pid>/smaps` after 1,001 resolves:

| Part | Resident | Note |
|---|---|---|
| `skillhub` binary pages | 17.6 MB | Shared between all processes that run the same binary file |
| Go heap | 10.0 MB (7.2 MB after the idle release) | Live data is about 3 MB; the GC goal is 6 to 7 MB (`GODEBUG=gctrace=1`) |
| Unnamed anonymous memory | 4.8 to 5.1 MB | Memory allocated by the SQLite library and thread stacks |
| Go runtime metadata | 2.5 MB | |
| libc and the dynamic loader | 1.8 MB | Shared |

The heap profile (`pprof`, in-process, runtime profile) shows about 1.8 MB of sampled live data. Most of it is the tool schemas built at start-up (`registerRuntimeTools`, 0.66 MB) plus request goroutines. Allocation is the real cost: about 1.1 MB is allocated per resolve, which causes about 450 GC cycles per 1,000 resolves. About 0.42 MB of that per resolve comes from `catalog.InspectWhileLocked` → `canonical.Scan`, which rereads every canonical file of the workspace to check that the catalog is fresh, and that grows with hub size. Telemetry adds more: each batch opens SQLite again (`telemetry.writeEvents` → `openDatabase`).

### Options tried (one at a time; only clear gains were kept)

| Option | Idle private memory (baseline 35.2 MB) | Latency | Kept? |
|---|---|---|---|
| Return freed heap to the OS after 30 s with no request (`debug.FreeOSMemory`, in `Serve`) | 32.5 to 32.8 MB (−2.7 MB), run twice | No change: it never runs while a request is in progress or within 30 s of one | **Yes** |
| `GOMEMLIMIT=16MiB` | 32.4 MB | p50 14.8 ms, p95 24.9 ms (breaks the +5 ms budget) | No |
| `GOMEMLIMIT=10MiB` | 31.9 MB | p95 29.6 ms | No |
| `GOGC=50` | 32.9 MB alone; 32.0 MB with the idle release (−0.5 MB more) | Twice as many GC cycles; p95 19.9 ms in the combined run | No (gain too small) |
| Open telemetry lazily | Start PSS 25.0 and 25.9 MB with telemetry vs 25.4 and 25.0 MB without | — | No (noise) |
| Smaller SQLite cache | At most about 2 MB: unnamed anonymous memory is only 5 MB, and that includes thread stacks | — | No (it would need changes in `internal/telemetry` and `internal/catalog`; gain too small) |

### Twenty processes (`make mcp-fanout`, 30 s of calls, then 45 s idle), total for all 20

| Binary | Rate per process | Events written / expected | Total PSS at start / working / after 45 s idle | Total RSS working / after idle | Per process PSS (working / idle) |
|---|---|---|---|---|---|
| Before, run 1 | 1/s | 1200 / 1200 | 199.7 / 336.5 / 332.4 MB | 697.9 / 693.2 MB | 16.8 / 16.6 MB |
| Before, run 2 | 1/s | 1200 / 1200 | 196.2 / 337.9 / 334.5 MB | 699.6 / 695.8 MB | 16.9 / 16.7 MB |
| After, run 1 | 1/s | 1200 / 1200 | 199.0 / 336.7 / **288.5** MB | 695.0 / 646.2 MB | 16.8 / **14.4** MB |
| After, run 2 | 1/s | 1200 / 1200 | 197.2 / 335.6 / **289.2** MB | 694.0 / 647.0 MB | 16.8 / **14.5** MB |
| Before | 10/s | 11996 / 12000 | 199.4 / 365.3 / 357.0 MB | 729.9 / 720.9 MB | 18.3 / 17.9 MB |
| After | 10/s | 11996 / 12000 | 197.2 / 364.7 / **305.7** MB | 727.9 / 668.4 MB | 18.2 / **15.3** MB |

The idle release saves about 45 to 51 MB across 20 idle sessions (−13 to −14%). With 20 processes on one machine, latency varies a lot between runs, both before and after: p95 was 18 to 45 ms at 1/s and about 40 ms at 10/s. Single-process p95 stays within the budget.

**Telemetry loss at 10 calls per second, before and after alike.** Between 0 and 6 of 12,000 events are dropped, and not on every run. The earlier "0 lost" result was a lucky run. The cause, found by logging the recorder's error temporarily: every write batch opens the SQLite database again, runs `PRAGMA quick_check`, and reruns the schema statements, which include a write to `telemetry_meta`. With 20 writers, a few batches exceed the 5 s operation timeout (`context deadline exceeded`) and are dropped. The fix is in `internal/telemetry`, which this phase may not change (see the follow-ups below).

### Web UI and other code an `mcp serve` process does not use (baseline for Phase 6)

| Measure | Number |
|---|---|
| Linked packages | `cmd/skillhub` 414; `internal/delivery/mcpserver` 406. Only the full binary has `delivery/cli`, `cli/termui`, `delivery/web`, `selfupdate`, `archive/tar`, `archive/zip`, `x/term` |
| Package init for `mcp serve` (`GODEBUG=inittrace=1`) | 176 packages, 2.0 to 3.3 ms, 2.2 MB allocated in total. `delivery/web`: 0.006 to 0.024 ms and 2 KB. `delivery/cli`: 0.009 to 0.017 ms and 20 KB. The largest init allocations are all on the MCP path: `telemetry` 365 KB, `skillruntime` 357 KB, `app` 354 KB, `mcpserver` 247 KB, `canonical` 226 KB, `distill` 205 KB |
| Binary size | Full `skillhub` with the web UI assets: 35.8 MB (the assets are 5.6 MB). Throwaway binary that links only the MCP server: 27.6 MB |
| Resident binary pages | 17.8 MB full vs 16.3 MB MCP-only. The 5.6 MB of web assets are never loaded into memory |
| One process after 1,001 resolves and 60 s idle (private) | 32.4 MB full vs 30.8 MB MCP-only (−1.6 MB). With 20 processes the binary pages are shared, so the difference per session is close to zero |

So in an `mcp serve` process the web UI costs disk space, not memory. It runs no noticeable start-up work. Phase 6 can treat this as its baseline and add a guard; no code split is needed for memory.

### Follow-ups for the user to decide (not done in Phase 1)

- Fix the telemetry writer so it keeps its database open and skips `quick_check` on every batch (`internal/telemetry/store.go` `writeEvents` and `openDatabase`). This removes the occasional loss and part of the work done per resolve.
- Check catalog freshness without rereading every canonical file on each resolve (`internal/catalog` `InspectWhileLocked`). This is the largest per-resolve allocation and grows with hub size.
- 1,000 resolves sent at once without waiting (a burst real hosts do not send) leave one process at 112 MB RSS, including 58 MB in 960 small unnamed mappings from the SQLite allocator that are not returned. Not a realistic load; noted only.

### How to repeat the numbers

```sh
# one process: start, after 1 and 1,001 resolves, after 60 s idle (also part of make test-perf)
SKILLHUB_PERF=1 go test -count=1 -run TestMCPMemoryBudgetPerformance -v ./internal/delivery/mcpserver/
# heap profile of a working server
SKILLHUB_PERF=1 SKILLHUB_HEAP_PROFILE=/tmp/heap.pprof go test -count=1 -run TestMCPHeapProfilePerformance -v ./internal/delivery/mcpserver/
go tool pprof -sample_index=inuse_space -top /tmp/heap.pprof
# 20 processes at 1 and 10 calls per second, then 45 s idle
make mcp-fanout
# compare two binaries on the same fixture (before/after above used binaries compiled from HEAD and from the change)
SKILLHUB_MEMORY_BINARY=/path/to/skillhub make mcp-fanout
# package init cost
GODEBUG=inittrace=1 skillhub mcp serve --profile runtime --workspace <temp workspace> </dev/null
```

## Phases

| # | Phase | Status | Priority |
|---|---|---|---|
| 1 | [Measure and slim one process](./phase-01-memory-per-process.md) | Pending | P1 |
| 2 | [Daemon lock, spawn gate and detach](./phase-02-daemon-lock-and-spawn.md) | Pending | P2 |
| 3 | [Attach or fall back in-process](./phase-03-attach-or-fallback.md) | Pending | P2 |
| 4 | [Idle shutdown](./phase-04-idle-shutdown.md) | Pending | P3 |
| 5 | [Cross-OS tests and docs](./phase-05-cross-os-tests-and-docs.md) | Pending | P2 |

| 6 | [Lean MCP process from one binary](./phase-06-lean-mcp-process-single-binary.md) | Pending | P1 |

Design principle (user, 2026-10-11): the MCP process and the daemon must be as small and cheap as possible. The web UI is not a background service: the user starts it by hand with `skillhub serve web` when needed and stops it with Ctrl+C when done, exactly as today, so nothing is built for it here. It must never be started by the MCP process or the daemon. Phases 1 and 6 hold the MCP process to this; Phase 4 does it for the daemon.

Run order: 1, then 6, then 2 to 5. Phase 6 (added at the user's request) keeps one `skillhub` binary but makes an `mcp serve` process, and later the daemon, load none of the web UI's start-up work; it is numbered last only to avoid renumbering. Phase 1 can end Phases 2 to 5 early. Phases 2 to 4 start only after the remote plan has an HTTP server to reuse (its Phase 2).

## Lessons taken from `~/projects/mdview` (read on 2026-10-11, Rust, one daemon per machine)

| Pattern in mdview | Where | Used here |
|---|---|---|
| Lock file with pid, host, port and the binary's version; a stale lock reads as "not running" because the health probe fails | `crates/mdview-core/src/daemon.rs` | Yes, same fields plus the workspace path |
| Spawn gate: a `.spawning` file created with create-new, so only one racer spawns; an old gate (15 s) is stolen; re-check under the gate | `daemon.rs` `acquire_spawn_gate_at` | Yes |
| Detach: `setsid` on Unix; `DETACHED_PROCESS` and `CREATE_NEW_PROCESS_GROUP` on Windows; tested separately | `crates/mdview-core/src/process.rs` | Yes |
| Ask the running daemon for its real version, not the lock; restart a lower version | `daemon_version`, `runtime.rs` | Yes: a binary update must not leave an old daemon serving |
| Health probe with a 500 ms timeout | `daemon.rs` | Yes |
| The daemon is required; there is no fallback | whole design | No: here the daemon is optional (Phase 3) |
| No idle shutdown; only a manual `restart` | whole design | No: added in Phase 4 |

## Success Criteria

- [ ] Phase 1 produces a table of memory per process (idle, after one resolve, after 1,000 resolves) before and after slimming, with the command that produced it, and records the go or stop for Phases 2 to 4.
- [ ] With the daemon unavailable, disabled, too old, or failing to start, `skillhub mcp serve` behaves exactly as today (a test per case; no session ever hangs).
- [ ] 20 concurrent sessions attached to one daemon use clearly less total memory than 20 in-process ones, measured the same way as the baseline, and lose no telemetry event at realistic rates.
- [ ] Two sessions started at the same instant spawn one daemon (test).
- [ ] The daemon exits after the idle time with zero sessions and removes its lock and socket; a new session starts a fresh one.
- [ ] After `skillhub update`, a session never talks to an older daemon.
- [ ] Another user on the machine cannot reach the daemon.
- [ ] All of it passes on Linux, macOS and Windows CI.

## Risks

| Risk | Mitigation |
|---|---|
| Daemon dies or hangs and sessions hang with it | Proxy health-probes with a short timeout and serves in-process on any failure; never waits on the daemon |
| Two spawns race | Spawn gate, as in mdview |
| Old daemon after an update | Version check against the running daemon; proxy asks it to exit and falls back for this session |
| Windows detach and socket behaviour | Fallback keeps users safe; Windows CI proves the happy path (the last release cycle showed Windows is where this breaks) |
| Another local user connects | Socket mode 0600 or a token in a 0600 file; never a world-reachable port |
| Sessions with different workspaces | The lock and socket are keyed by workspace path |
| Telemetry and session identity differ from today | Each attached session keeps its own tracker key (the remote plan, Phase 5, covers this) |

## Rules for every phase

- Do not write to `/home/vantt/skill-hub`; do not push without asking.
- Never remove telemetry event types or change `EventVersion`.
- Tests read only the repo or a temp dir; use temp workspaces and a temp runtime directory, never the user's real one.
- Conventional commits; `make check` green before a merge; do not edit `.claude/skills/distill-lab`.
- Before starting a long-running process in a test, follow `.claude/rules/process-management.md`: track it and stop it.
