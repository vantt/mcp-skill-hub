---
title: "Remote hub: agents on machine A use the hub on machine B over MCP"
description: "Let an agent on one machine call skill_resolve, skill_get and skill_feedback against a hub running on another machine, over authenticated, encrypted MCP, with runtime tools only."
status: pending
priority: P3
effort: 4d
branch: main
tags: [mcp, remote, security, distribution, telemetry]
blockedBy: []
blocks: []
created: 2026-10-10
---

# Remote hub: agents on machine A use the hub on machine B over MCP

## Overview

Today the MCP server only speaks stdio (`internal/delivery/mcpserver/server.go:132`, `mcp.StdioTransport`), so the hub must live on the same machine as the agent. Moving a hub to another machine works only through Git (`git clone`, `skillhub rebuild`), and each machine then keeps its own telemetry and cases.

This plan adds a second way: one hub, many machines. The agent on machine A talks to `skillhub serve mcp --addr ...` on machine B and gets the same three runtime tools. Curation stays local to the hub machine; it is never exposed over the network.

Decision (user, 2026-10-11): go. One user, several machines, one machine curates and serves the skills; only the network option is planned here (the other options discussed were Git clone, the web UI over an SSH tunnel and a shared multi-user hub).

## What the code already gives us (checked 2026-10-10)

| Fact | Where | Effect on this plan |
|---|---|---|
| Go MCP SDK v1.8.0 ships `NewStreamableHTTPHandler` | `go.mod`, SDK `mcp/` | The transport itself is small; no new dependency |
| Session state is keyed by `*mcp.ServerSession`, not by process | `internal/delivery/mcpserver/activation_tracker.go` | Prior-resolution checks and the case journal keep working per HTTP session |
| `skill_get` and resources carry `io.skillhub/local_path` | `server.go:459` | Wrong for a remote client; the path is on the server's disk |
| The runtime profile has 3 tools (about 7 KB without output schemas) | observer plan §5.1 | Safe subset to expose |
| The web UI already uses a session token and warns about plain HTTP | `internal/delivery/web/listen.go` | Pattern to reuse for the token; TLS must be stricter here |
| Skill URIs are pinned to a content digest and return `snapshot_expired` with `current_uri` | `distribution.go` | Remote clients get consistent versions for free |

## Measured today: one stdio process per agent session (2026-10-11)

Today every agent session starts its own `skillhub mcp serve` over stdio and the process ends when the host closes stdin. The user runs up to 20 agents at once, so the plan needs to know what that costs before deciding whether a shared server is worth it. Measured on Linux, in a throwaway workspace with an isolated `HOME` and `XDG_*` (the live hub was not touched), with 20 processes each calling `skill_resolve` for 30 seconds. The catalog was nearly empty, so a real hub with many skills may cost more.

| Scenario | Calls | Events written / expected | Lost | Total RSS |
|---|---|---|---|---|
| 1 call per second per process | 600 | 1200 / 1200 | 0 | 650 MB |
| 10 calls per second per process (about 190 per second in total) | 5,792 | 11,584 / 11,584 | 0 | 755 MB |
| Back-to-back calls (about 5,300 per second in total) | 160,207 | 56,197 / 320,414 | 264,217 dropped, 27 storage errors | 836 MB |

- Each `skill_resolve` writes 2 telemetry events (single-process baseline: 2,744 calls, 5,488 events).
- Correction (2026-10-11, after the local-shared-hub Phase 1 measurements): at 1 call per second per process nothing was lost, but at 10 calls per second per process between 0 and 6 of 12,000 events are dropped on some runs, so "loses nothing" above was a lucky run. The cause was found: every telemetry write batch reopens SQLite and runs `PRAGMA quick_check`, so with 20 writers a few batches hit the 5 s timeout (`internal/telemetry/store.go`, `writeEvents` and `openDatabase`). A shared server with one writer would remove this; fixing the writer does too.
- Startup is about 20 ms per process; `skill_resolve` takes about 3 ms at the median and 9 ms at p95 over stdio.
- Memory, corrected: the "Total RSS" column above adds up resident memory, which counts about 18 MB of `skillhub` binary pages in every process although those pages are loaded once and shared. Measured with PSS (shared pages split between the processes that use them) one session costs about 14 to 15 MB idle and 17 to 18 MB working, and 20 sessions about 0.29 to 0.36 GB in total. Read the RSS column as an upper bound, not as the real cost. The full tables are in `plans/261011-0455-local-shared-hub-process/plan.md`.
- All 20 processes exit with code 0 when stdin closes, so a closed session leaves no orphan.

What this means for the plan: a shared server is not needed for correctness at the user's load. Its benefit is some memory (at most about 0.25 GB for 20 sessions, less if each still keeps a stdio proxy) and a single telemetry writer. Whether it is worth running a long-lived server is a decision for Phase 1, and it also depends on whether the agent hosts can talk MCP over HTTP directly (not yet checked for Claude Code, Codex or Gemini CLI); if they cannot, every session still keeps a small stdio proxy and the process count does not drop. The test harness is a throwaway Python script outside the repo; Phase 6 should turn it into a repeatable check if the user wants one.

## Decisions (answered by the user on 2026-10-11)

| # | Question | Decision |
|---|---|---|
| Q1 | Who connects | One user with several machines. One machine curates and serves the skills; the others only use them. No accounts: a single shared bearer token |
| Q2 | How machine A reaches machine B | Both an SSH tunnel and Tailscale (or another private VPN) are supported. No TLS in this plan and no exposure to the open internet. The hub listens on loopback by default (SSH tunnel). For Tailscale the user binds it to the Tailscale address and confirms it with an explicit flag (`--trust-private-network`) |
| Q3 | Skills with scripts, setup or check commands | Run only on the client, after the client pulls the files; never on the hub for a remote caller |
| Q4 | Task text sent over the network | The hub refuses to listen on any non-loopback address unless `--trust-private-network` is given; plain HTTP on loopback is allowed (SSH tunnel, tests) |
| Q5 | Telemetry from remote clients | Written on the hub with a client label; the case journal stays opt-in on the hub |

Go or stop (Phase 1 task 5) is settled: go. The user wants one machine to curate and serve the skills to the others, which the Git route does not do live.

## Two ways of working, both must keep working

1. **One machine (today, unchanged).** The host starts `skillhub mcp serve` over stdio. No network port, no token, no extra process, and the same registrations `skillhub connect --workspace` writes today. A regression test pins this output.
2. **Machine A uses machine B (new).** B runs a network listener over the same workspace; A's host starts a small stdio proxy that talks to it. Machine B can still serve its own agents over stdio at the same time as it serves A, because the listener is a separate process over the same workspace (the telemetry writer was made safe for several processes on 2026-10-11).

Each host scope (project or global) points at one hub: `connect --workspace` for a local one or `connect --hub` for a remote one. Switching a scope between the two replaces its entry.

## Command syntax (one pattern for every surface)

Hub machine B:

```
skillhub serve web                                   # web UI, as today
skillhub serve mcp                                   # MCP over stdio, what hosts launch today (`mcp serve` stays as an alias)
skillhub serve mcp --addr 127.0.0.1:7420             # network listener, loopback: reach it through an SSH tunnel
skillhub serve mcp --addr <tailscale-ip>:7420 --trust-private-network
skillhub token show | rotate                         # shared token; written once to $SKILLHUB_CONFIG_DIR with mode 0600
```

Machine A:

```
skillhub connect --workspace ~/skillhub --yes        # hub on this machine (as today)
skillhub connect --hub http://127.0.0.1:7420 --token-file <path> --yes    # via an SSH tunnel
skillhub connect --hub http://<tailscale-name>:7420 --token-file <path> --yes
skillhub connect --remove --yes                      # remove the entry for this scope (works for both kinds)
```

`--workspace` and `--hub` exclude each other. The token comes from `--token-file` or the environment variable `SKILLHUB_HUB_TOKEN`, and is never written into a host config file. The listener prints the exact `connect --hub` line to run on A when it starts, with the token file path, never the token itself. Network mode always serves the runtime profile only. The stdio proxy `skillhub mcp proxy --hub <url>` is an internal entry that `connect --hub` writes; users do not type it.

## Phases

| # | Phase | Status | Priority |
|---|---|---|---|
| 1 | [Baseline, budgets and decisions](./phase-01-baseline-and-decisions.md) | Pending | P1 |
| 2 | [HTTP transport, runtime profile only](./phase-02-http-transport.md) | Pending | P1 |
| 3 | [Skill bundle delivery and client cache (skill runs, fetch is fast)](./phase-03-bundle-delivery-and-cache.md) | Pending | P1 |
| 4 | [Client setup: connect --hub and doctor](./phase-04-client-setup.md) | Pending | P2 |
| 5 | [Telemetry, sessions and privacy](./phase-05-telemetry-and-privacy.md) | Pending | P2 |
| 6 | [Benchmarks, two-machine tests and docs](./phase-06-benchmarks-tests-docs.md) | Pending | P1 |

Phases run in order. Phase 3 is the heart of the two headline criteria and is the largest phase.

## Success Criteria (set by the user on 2026-10-10: the skill runs, and fetching it is fast)

The two headline criteria come first; the security criteria below them are the price of exposing a network listener and are not optional.

**1. The skill runs on the remote machine.**
- [ ] For every approved skill in a test hub (including `herdr-cook-plan`: 14 files, about 186 KB, and `test-audit`: 4 files), the remote client gets the whole skill folder: `SKILL.md`, `references/`, `scripts/` and any other file, byte-identical to the hub (digest equal).
- [ ] The files are materialized in a local directory on machine A, and `SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR` and `SKILLHUB_CONFIG_DIR` point at it, so a skill's `check`, `setup` and scripts run exactly as they do on a local hub (an end-to-end test runs one skill's `check` command on the client).
- [ ] The same skill gives the same result locally and remotely (same files, same digest, same runtime spec).
- [ ] Unapproved skill content stays blocked for remote callers exactly as it is for local ones.

**2. Getting a skill is fast.**
Proposed budgets (Phase 1 measures the local stdio baseline first and confirms or adjusts them; numbers are targets, not measurements):
- [ ] `skill_resolve` over a LAN adds at most 50 ms at p95 over the local stdio time (one round trip, no extra calls).
- [ ] The first `skill_get` of `herdr-cook-plan` (the largest skill) takes one network exchange for the whole folder, not one per file, and finishes within 500 ms on a LAN and 2 s on a 100 ms-RTT link.
- [ ] Every later use of the same skill version costs no file transfer at all (local cache keyed by the skill digest): at most 20 ms and zero bytes of skill content.
- [ ] A changed skill refetches only that skill (digest changed), and a metadata-only hub change refetches nothing.
- [ ] A reproducible benchmark (`make remote-bench`) reports these numbers for stdio, loopback HTTP and a simulated 100 ms link, and the plan fails if a budget is missed.

**3. Safe to expose.**
- [ ] Curation tools, the case journal CLI, and every write path are unreachable through the network listener (test).
- [ ] No request is accepted without a valid bearer token, and the listener refuses any non-loopback address unless `--trust-private-network` is given (no TLS in this plan; the transport is an SSH tunnel or a private VPN such as Tailscale).
- [ ] A remote client never receives a path on the hub's disk.
- [ ] Session-aware cases (`verified_reformulation`, `repeated_gap`) still work per remote session.
- [ ] `skillhub connect --hub` writes a working entry for Claude Code, and `doctor` explains a failed connection in plain words (hub stopped, tunnel down, wrong token, old hub version).

**4. Both ways of working keep working.**
- [ ] One machine: `connect --workspace` and `skillhub mcp serve` behave exactly as before, with no port, token or extra process (regression test on the written entry and on the server start).
- [ ] A to B: the same skills resolve and run through an SSH tunnel and through a Tailscale address (the second checked by hand on the user's machines; the first also in an automated loopback test).
- [ ] Machine B serves its own agents over stdio and machine A over the network at the same time, with no lost telemetry events.

## Risks

| Risk | Mitigation |
|---|---|
| Token leaks or is guessed | Long random token, constant-time compare, rotate command, rate limit, no token in URLs or logs |
| Task text and skill content travel in clear | The listener refuses a non-loopback address without `--trust-private-network`; the guide steers to an SSH tunnel or Tailscale and never to plain HTTP over a shared network |
| The listener widens the attack surface of a curation host | Separate listener, runtime profile hard-wired, no workspace mutation reachable |
| Skills with scripts run on the wrong machine | Q3 rule enforced in `skill_get` output and tested |
| Hub down means no routing for remote agents | Client falls back to "no skill" with a clear message; documented |
| Version skew between client and hub | `doctor` compares versions; tool schema stays additive |

## Rules for every phase

- Do not write to `/home/vantt/skill-hub`; do not push without asking.
- Never remove telemetry event types or change `EventVersion`.
- Tests read only the repo or a temp dir; the two-machine test uses two loopback ports and two temp workspaces.
- Conventional commits; `make check` green before a merge.
- Do not edit `.claude/skills/distill-lab`.
