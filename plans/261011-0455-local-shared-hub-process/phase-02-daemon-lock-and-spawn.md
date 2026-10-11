---
phase: 2
title: "Daemon lock, spawn gate and detach"
status: cancelled
priority: P2
effort: "1d"
dependencies: [1]
---

# Phase 2: Daemon lock, spawn gate and detach

## Goal
Make it possible for exactly one background hub process per machine and workspace to exist, to be found by anyone, and to be started by whoever needs it first, on Linux, macOS and Windows.

## Files to Create / Modify
- Create: `internal/daemon/lock.go` (lock file: pid, address, binary version, workspace path, started time; atomic write; a stale lock reads as "not running")
- Create: `internal/daemon/spawn_gate.go` (create-new gate file, 15 s stale steal, re-check under the gate)
- Create: `internal/daemon/detach_unix.go` and `internal/daemon/detach_windows.go` (build-tagged: `setsid`; `DETACHED_PROCESS` and `CREATE_NEW_PROCESS_GROUP`)
- Create: `internal/daemon/health.go` (probe with a 500 ms timeout; ask the running process for its real version)
- Modify: `internal/delivery/cli/mcp.go` (hidden `skillhub mcp daemon` entry that runs the shared server; not documented as a user command)
- Reuse: the HTTP MCP handler from the remote plan, Phase 2 (`internal/delivery/mcpserver/http.go`), bound to a Unix socket or to loopback (decision Q2)
- Runtime directory for the lock and socket: under the user's state directory, mode 0700, keyed by a hash of the workspace path

## Tasks & Steps
1. Pick the transport per OS (Q2). Check on the Windows CI image that Go `net.Listen("unix", ...)` works; if not, use loopback plus a token file with mode 0600.
2. Write the lock and the probe; the probe never trusts the lock alone.
3. Write the spawn gate and its tests: two spawners at once produce one daemon; a dead gate owner is stolen after the stale time.
4. Write the detach helpers and test them on their own, without starting the full daemon.
5. Make the daemon refuse a second start for the same workspace, and write its own version into the lock.

## Verification
- Race test: 20 callers start at once, exactly one daemon exists afterwards.
- A stale lock (process killed) is ignored and replaced.
- On Windows CI the daemon starts detached, answers the probe, and survives the spawner exiting.
- No file in the runtime directory is readable by another user (mode check on Unix).
