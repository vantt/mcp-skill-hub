---
phase: 4
title: "Client setup: connect --hub and doctor"
status: completed
priority: P2
effort: "0.5d"
dependencies: [3]
---

# Phase 4: Client setup

## Goal
One command on machine A makes the agent host use the hub on machine B (through an SSH tunnel or Tailscale), and `doctor` explains a broken connection in plain words. The one-machine setup must stay exactly as it is.

## Files to Create / Modify
- Modify: `internal/hostintegration/` (a remote entry type: the host launches `skillhub mcp proxy --hub <url>` with the token read from `SKILLHUB_HUB_TOKEN` or a 0600 file, never written into the host config file)
- Modify: `internal/delivery/cli/connect.go` (`skillhub connect --hub <url> [--token-file <path>]`, mutually exclusive with `--workspace`; `skillhub connect --remove` removes the entry for the scope; preview first, apply with `--yes`, same preview/confirm pattern as today)
- Modify: `internal/app/doctor*.go` (remote checks: reachable, token accepted, version skew, cache writable, last fetch time; when the hub is unreachable the fix line says to check the SSH tunnel or the Tailscale connection and that `skillhub serve mcp --addr` is running on machine B)
- Modify: `docs/mcp-compatibility-matrix.json` only if a host needs a different entry shape

## Tasks & Steps
- [x] 1. Reuse the runtime-only entry that `connect` already writes for Claude Code; replace the command with the proxy and add the remote URL.
- [x] 2. Keep the four CLI permission rules and the runtime-only rule intact.
- [x] 3. `doctor` reports each remote check with one fix command; no check prints the token.
- [x] 4. `connect --remove` removes the entry for the scope and, on request, the cache. Switching a scope between `--workspace` and `--hub` replaces its entry; one scope points at one hub.
- [x] 5. Hand-off of the token from B to A is documented, not automated: the listener prints the command; the user copies the token file (for example with `scp`) or sets `SKILLHUB_HUB_TOKEN`.

## Verification
- [x] `connect --hub` in a temp `HOME` writes exactly the expected files and no token.
- [x] `connect --workspace` in a temp `HOME` writes byte-for-byte what it writes today (regression test), and `connect --workspace` followed by `connect --hub` for the same scope leaves one entry.
- [x] `doctor` on a stopped hub, a wrong token and an old hub version each prints a distinct, understandable message.
- [x] `make check` green.

## Implementation status (2026-10-11)
Completed. Report: [cook-261011-remote-phase4.md](../../../plans/reports/cook-261011-remote-phase4.md). The token hand-off is explained in the `connect --hub` preview and in `skillhub help connect`; the user guide is Phase 6.
