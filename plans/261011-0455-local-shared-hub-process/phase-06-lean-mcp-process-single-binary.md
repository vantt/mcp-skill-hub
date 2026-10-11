---
phase: 6
title: "Lean MCP process from one binary"
status: pending
priority: P1
effort: "0.75d"
dependencies: [1]
---

# Phase 6: Lean MCP process from one binary

## Goal
Keep one `skillhub` binary, but make a process started as `skillhub mcp serve` (and later the daemon) load and hold nothing that belongs to the web UI or other commands. Starting the MCP server must not drag the web along.

The number in this phase is set by Phase 1 (which measures what the web and other packages cost inside an MCP process); this phase turns that finding into a permanent guard. It can run right after Phase 1 and does not wait for Phases 2 to 5; run it before them, because the daemon reuses the same start-up path.

## What "separate at load time" can and cannot mean in Go
- Go cannot unload code. A single binary always contains the web code and its embedded assets.
- What a process actually pays for is the work done at start-up (package `init`, package-level maps and regexps, command tables) and the heap it creates. Code and embedded data that are never executed or read stay on disk: the operating system maps the file lazily and does not count untouched pages as resident memory.
- So the target is: the MCP path runs none of the web's start-up work and holds none of its state, and the cost that remains is file size on disk, not resident memory. Phase 1's numbers decide whether that is already true.

## Files to Create / Modify
- Modify: `cmd/skillhub/main.go` (or the current entry) and `internal/delivery/cli/` dispatch: route `mcp serve` before the full command table, flag sets and terminal UI are constructed
- Modify: packages found by Phase 1's init trace (expected: `internal/delivery/web`, `internal/delivery/cli/termui`, anything with package-level `regexp.MustCompile`, large maps or `init()` work): make that state lazy (`sync.Once` at first use), so merely linking the package costs nothing
- Create: `internal/delivery/mcpserver/lean_process_test.go` (guard; skipped unless `SKILLHUB_PERF=1` for the memory part, always on for the static part)
- Create: a small static check (Go test using `go/parser` or `go list`) that the web package and other non-MCP packages declare no `init()` functions and no package-level work that allocates

## Tasks & Steps
1. Take the init trace and memory comparison from Phase 1 as the baseline: which packages run start-up work in an `mcp serve` process, and how much heap and time each costs.
2. Move the entry dispatch so `mcp serve` does not build the whole CLI first. Keep the public command syntax unchanged, including the registrations hosts already wrote.
3. Make the offending package-level state lazy, one package at a time, keeping only changes with a measured gain.
4. Add the guard: an init-trace budget for `mcp serve` (total init time and heap under a limit set from the measured result) and the static check that web and other non-MCP packages do no start-up work.
5. Check resident versus mapped memory with `/proc/<pid>/smaps` on Linux: confirm the web assets and unused code are not resident in an MCP process.
6. If, after steps 2 and 3, the MCP process still pays a measured cost that only a separate binary or a build-tag split would remove, do not do it here. Write the measured gain and the cost of that change (release archives, installer, host registrations, update path) as a recommendation for the user to decide. The user prefers one binary.

## Verification
- `mcp serve` init trace and resident memory before and after, same command and workspace, run twice; the table is in `plan.md`.
- The static check fails if someone adds an `init()` or a package-level `MustCompile` or large map to the web package (prove it by adding one temporarily).
- `skillhub mcp serve` and `skillhub serve web` both still work with the same flags; the existing host registrations keep working without being rewritten.
- `make check` green; release archives and install steps unchanged.
