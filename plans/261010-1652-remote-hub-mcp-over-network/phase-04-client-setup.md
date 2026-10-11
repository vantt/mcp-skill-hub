---
phase: 4
title: "Client setup: connect --remote and doctor"
status: pending
priority: P2
effort: "0.5d"
dependencies: [3]
---

# Phase 4: Client setup

## Goal
One command on machine A makes the agent host use the remote hub, and `doctor` explains a broken connection in plain words.

## Files to Create / Modify
- Modify: `internal/hostintegration/` (a remote entry type: the host launches `skillhub mcp proxy --remote <url>` with the token read from an environment variable or a 0600 file, never written into the host config file)
- Modify: `internal/delivery/cli/connect.go` (`skillhub connect --remote <url> [--token-file <path>]`; preview first, apply with `--yes`, same preview/confirm pattern as today)
- Modify: `internal/app/doctor*.go` (remote checks: reachable, token accepted, TLS valid, version skew, cache writable, last fetch time)
- Modify: `docs/mcp-compatibility-matrix.json` only if a host needs a different entry shape

## Tasks & Steps
1. Reuse the runtime-only entry that `connect` already writes for Claude Code; replace the command with the proxy and add the remote URL.
2. Keep the four CLI permission rules and the runtime-only rule intact.
3. `doctor` reports each remote check with one fix command; no check prints the token.
4. `disconnect` removes the remote entry and, on request, the cache.

## Verification
- `connect --remote` in a temp `HOME` writes exactly the expected files and no token.
- `doctor` on a stopped hub, a wrong token and an old hub version each prints a distinct, understandable message.
- `make check` green.
