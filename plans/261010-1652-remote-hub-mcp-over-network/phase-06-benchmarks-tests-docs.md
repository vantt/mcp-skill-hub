---
phase: 6
title: "Benchmarks, two-machine tests and docs"
status: pending
priority: P1
effort: "0.5d"
dependencies: [3, 4, 5]
---

# Phase 6: Benchmarks, two-machine tests and docs

## Goal
Prove the two headline criteria with repeatable measurements, and document the feature for users.

## Files to Create / Modify
- Create: `internal/delivery/mcpserver/remote_e2e_test.go` (two processes, two temp workspaces, two loopback ports; no real network)
- Create: `scripts/remote-bench.sh` and `make remote-bench` (stdio, loopback HTTP, and a simulated link with added delay; uses a local delay proxy, no root needed)
- Modify: `docs/user-guide.md` (section "Use one hub from several machines"), `README.md` (one paragraph and a link), `docs/release-runbook.md` (note on the new listener)
- Modify: `docs/design/remote-hub.md` (final numbers)

## Tasks & Steps
1. End-to-end test: start a hub on port A, run the proxy as machine A, call `skill_resolve`, `skill_get`, run a skill's `check`, call `skill_feedback`.
2. Failure tests: wrong token, hub stopped mid-fetch, truncated archive, cache directory read-only, unapproved skill, version skew.
3. Benchmark and compare with the budgets in `plan.md`; the benchmark fails when a budget is missed. Record results per transport.
4. Security tests: no curation tool reachable, plain HTTP refused off loopback, token never logged.
5. Write the user guide section: when to prefer Git clone, SSH tunnel for the web UI, or this remote MCP; setup in three commands; what happens when the hub is down.
6. Run on the real machines once, read-only: a second machine of the user's, with a test token, against a clone of the hub (not the live hub).

## Verification
- All success criteria in `plan.md` are checked with a test or a benchmark line.
- `make check`, `make remote-bench` and CI on Linux, macOS and Windows green.
- The user ran one skill from machine A against machine B and timed it.
