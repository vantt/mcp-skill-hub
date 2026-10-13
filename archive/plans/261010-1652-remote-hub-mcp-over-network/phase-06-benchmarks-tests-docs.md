---
phase: 6
title: "Benchmarks, two-machine tests and docs"
status: completed
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

As built: the bench is `internal/delivery/mcpserver/remote_budget_bench_test.go` behind `scripts/remote-bench.sh`; the user guide is a standalone page `docs/remote-hub-guide.md` with one link line each in `docs/user-guide.md` and `README.md` (both were being rewritten in another branch); `scripts/remote-timing.sh` lets the user time a real run; `internal/delivery/cli/remote_guide_doc_test.go` checks the guide's commands against the CLI help.

## Tasks & Steps
- [x] 1. End-to-end test: start a hub on port A, run the proxy as machine A, call `skill_resolve`, `skill_get`, run a skill's `check`, call `skill_feedback`.
- [x] 2. Failure tests: wrong token, hub stopped mid-fetch, truncated archive, cache directory read-only, unapproved skill, version skew.
- [x] 3. Benchmark and compare with the budgets in `plan.md`; the benchmark fails when a budget is missed. Record results per transport.
- [x] 4. Security tests: no curation tool reachable, a non-loopback address refused without `--trust-private-network`, token never logged.
- [x] 4a. One-machine regression: with no network listener, `connect --workspace` and `skillhub mcp serve` behave as before (no port, no token, no extra process). Two-way test: machine B serves its own stdio sessions and a remote proxy at the same time with no lost telemetry events.
- [x] 5. Write the user guide section "Use one hub from several machines": when Git clone is enough and when this is better; setup in three commands for an SSH tunnel and for Tailscale (including `ssh -N -L` and how to keep the tunnel up, `--trust-private-network` with the Tailscale address, and the token hand-off); what happens when the hub or the tunnel is down; how to go back to one machine.
- [ ] 6. Run on the real machines once, read-only: a second machine of the user's, with a test token, against a clone of the hub (not the live hub), once through an SSH tunnel and once through Tailscale. Left to the user: run `scripts/remote-timing.sh` on machine A, once through an SSH tunnel and once through Tailscale, following `docs/remote-hub-guide.md`.

## Verification
- [x] All success criteria in `plan.md` are checked with a test or a benchmark line, or marked as needing the real machines or CI on macOS and Windows.
- [x] `make check` and `make remote-bench` green on Linux.
- [x] CI on macOS and Windows green (Verify jobs, 2026-10-13).
- [ ] The user ran one skill from machine A against machine B and timed it. Moved to the observation backlog, item R1.

## Implementation status (2026-10-11)
Tasks 1 to 5 completed. Report: [cook-261011-remote-phase6.md](../../../plans/reports/cook-261011-remote-phase6.md). Task 6 and the last two verification lines wait for the user's real machines and for CI.
