---
phase: 4
title: "Idle shutdown"
status: cancelled
priority: P3
effort: "0.5d"
dependencies: [3]
---

# Phase 4: Idle shutdown

## Goal
The daemon costs nothing when nobody uses it: it exits by itself after a period with no sessions, and a later session starts a fresh one.

## Files to Create / Modify
- Modify: `internal/delivery/mcpserver/http.go` or the daemon entry (count open sessions; start an idle timer when the count reaches zero; cancel it when a session opens)
- Modify: `internal/daemon/lock.go` (remove lock and socket on exit, including on a signal)
- Modify: workspace config (idle minutes, default 10, decision Q3)

## Tasks & Steps
1. Count sessions at connect and disconnect, not requests: a long session with no traffic still counts.
2. When the count has been zero for the idle time, shut down cleanly: stop accepting, flush telemetry, close the store, remove the lock and socket, exit with code 0.
3. Close the race at the edge: a session that connects while the daemon is shutting down must either be accepted (timer cancelled) or get a clear refusal that makes the proxy spawn a new daemon or fall back in-process.
4. Handle SIGTERM and Ctrl+C the same way as an idle exit.

## Verification
- With a short idle time in a test, the daemon exits after the last session closes, and the lock and socket are gone.
- A session that opens during the idle window keeps the daemon alive.
- A connect at the moment of shutdown never hangs: it is served or it falls back (stress test with many connect and disconnect cycles).
- Telemetry written just before exit is present after the daemon exits.
