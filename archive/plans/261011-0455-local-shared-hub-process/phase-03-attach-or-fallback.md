---
phase: 3
title: "Attach or fall back in-process"
status: cancelled
priority: P2
effort: "0.75d"
dependencies: [2]
---

# Phase 3: Attach or fall back in-process

## Goal
Make `skillhub mcp serve` (what hosts already launch) attach to the daemon when it can, and otherwise serve in-process exactly as today. The daemon must never be a reason a session fails.

## Files to Create / Modify
- Modify: `internal/delivery/cli/mcp.go` and `internal/delivery/mcpserver/server.go` (`Serve`: try attach, else serve in-process)
- Create: `internal/delivery/mcpserver/attach.go` (stdio-to-daemon relay: reads frames from stdin, forwards to the daemon session, writes replies to stdout)
- Modify: `internal/config` or the workspace config (opt-out switch, for example `mcp.shared: auto | off`; default decided after Phase 1)

## Tasks & Steps
1. Decision order at start-up: if shared mode is off, serve in-process. Otherwise probe the daemon; if it is healthy and its version equals this binary's, attach. If it is missing, take the spawn gate, spawn it, wait for health up to a short limit, then attach. If anything fails or times out, serve in-process and write one line to stderr.
2. Version rule: a daemon older than this binary is asked to exit and this session serves in-process; a newer daemon is kept. Never talk to a daemon of a different schema.
3. Attached session behaviour: each stdio session maps to one daemon session, so the activation tracker keeps one prior-resolution memory per session (the remote plan, Phase 5, covers the tracker and the client label).
4. If the daemon dies mid-session, the relay reports a clear error on the next call and the next session starts fresh; do not try to hide a lost session by silently re-serving, because the tracker state is gone.
5. Tool profile: the daemon serves the runtime profile only; a session that asks for the curation profile always serves in-process.

## Verification
- A test per fallback: shared mode off, no daemon and spawn fails, daemon older, daemon unhealthy, daemon killed between two calls. No test hangs; each ends with a defined result.
- With the daemon healthy, `tools/list` and `skill_resolve` return the same results as in-process on the same workspace.
- Two attached sessions keep separate resolution memory (the `verified_reformulation` case works per session).
- 20 attached sessions: total memory measured with `make mcp-fanout` against the Phase 1 baseline; no telemetry loss at realistic rates.
