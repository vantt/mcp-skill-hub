---
phase: 5
title: "Cross-OS tests and docs"
status: pending
priority: P2
effort: "0.5d"
dependencies: [4]
---

# Phase 5: Cross-OS tests and docs

## Goal
Prove the daemon behaves on all three operating systems, leaves no stray processes behind, and is explained to users in plain words.

## Files to Create / Modify
- Create: `internal/daemon/daemon_e2e_test.go` (real processes, temp workspace, temp runtime directory; skipped under `-short` if it is slow)
- Modify: `.github/workflows/ci.yml` and `.github/workflows/release.yml` (keep both in step: the Windows scratch-space setup lives in both; a new test must not need a different setup in only one of them)
- Modify: `docs/user-guide.md` (short section: "Running many agents at once"; what shared mode does, how to switch it off, what happens when it fails)
- Modify: `docs/release-runbook.md` (the daemon must be stopped before an update replaces the binary; version check behaviour)
- Modify: `docs/design/remote-hub.md` (or the design note the remote plan creates) with the final memory numbers

## Tasks & Steps
1. End-to-end test: spawn gate race, attach, fallback cases, idle exit, version mismatch after a simulated update, one test per OS in CI.
2. Process hygiene: every test that starts a daemon records its pid and stops it at the end, and a final check asserts that no daemon from the test remains (follows `.claude/rules/process-management.md`).
3. Run the fan-out benchmark in CI as an informational job (not a gate) and keep the numbers in the job summary.
4. Write the user guide section and the runbook note.
5. Once, by hand on the user's machine and with the user's consent, run a real multi-agent session with shared mode on and compare memory with off.

## Verification
- `make check` and CI green on Linux, macOS and Windows.
- No daemon process remains after the test suite on any OS.
- The guide explains in plain words how to turn shared mode off and what the user sees when the daemon is down.
- The final memory table in the design note matches a fresh run of `make mcp-fanout`.
