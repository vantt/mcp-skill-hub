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

## Phases

| # | Phase | Status | Priority |
|---|---|---|---|
| 1 | [Measure and slim one process](./phase-01-memory-per-process.md) | Pending | P1 |
| 2 | [Daemon lock, spawn gate and detach](./phase-02-daemon-lock-and-spawn.md) | Pending | P2 |
| 3 | [Attach or fall back in-process](./phase-03-attach-or-fallback.md) | Pending | P2 |
| 4 | [Idle shutdown](./phase-04-idle-shutdown.md) | Pending | P3 |
| 5 | [Cross-OS tests and docs](./phase-05-cross-os-tests-and-docs.md) | Pending | P2 |

Phase 1 can end the plan early. Phases 2 to 4 start only after the remote plan has an HTTP server to reuse (its Phase 2).

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
