---
phase: 5
title: "Telemetry, sessions and privacy"
status: pending
priority: P2
effort: "0.5d"
dependencies: [2]
---

# Phase 5: Telemetry, sessions and privacy

## Goal
Remote use produces the same observations as local use (chains, cases, loads) without leaking more than local use does.

## Files to Create / Modify
- Modify: `internal/delivery/mcpserver/activation_tracker.go` and `internal/app/caller_context.go` (client label for remote callers; session hash from the HTTP session)
- Modify: `internal/telemetry/` only by adding allowed attribute values; never remove event types or change `EventVersion`
- Modify: `docs/design/04-telemetry-reproducibility-evaluation.md` (remote clients section)

## Tasks & Steps
- [x] 1. Session identity: the tracker is keyed by `*mcp.ServerSession`, so each HTTP session gets its own prior-resolution memory; confirm with a test that two remote clients do not see each other's resolutions and that a reconnect starts a new session. Done 2026-10-11: no code change was needed; `TestRemoteSessionsKeepSeparateChainsAndCases` runs two concurrent sessions and a reconnect on one listener.
- [x] 2. Client label: record `remote` plus the client name the host reports; baseline metrics stay comparable per client. Done 2026-10-11: client name `remote:<host>` (`telemetry.RemoteClient`); stdio labels unchanged (`TestLocalSessionsRecordTheSameCasesWithUnchangedLabels`).
- [ ] 3. Skill loads: count a bundle fetch as a load once per skill version and session; cache hits on the client are reported by the proxy through `skill_feedback`-style usage so "used" is not undercounted (needed for the baseline's acceptance rate). Waits for the bundle endpoint (Phase 3); the hook to call is described in `plans/reports/cook-261011-remote-phase5.md`.
- [x] 4. Privacy: the case journal remains opt-in on the hub; task text arrives only over loopback or a confirmed private network (Q4, no TLS in this plan); the redactor runs on the hub for remote text as it does for local text; the token and the client IP are not stored. Done 2026-10-11: `TestRemoteCaseJournalStaysOptIn`, `TestRemoteCallsStoreNoTokenAndNoClientAddress`, `TestHTTPErrorLogDropsClientAddresses`.

## Verification
- [x] Two concurrent remote sessions keep separate chains (test).
- [x] `verified_reformulation` is recorded for a remote session as for a local one.
- [x] Telemetry store contains no token and no IP (test with a scan).
- [x] Existing telemetry tests stay green.
- [ ] A bundle fetch counts one load per skill version and session (with task 3).

- Decisions after review (2026-10-11): the client label `remote:<host>` stays (it needs no schema change); the label follows the connection type, so an agent on the hub machine that connects through the HTTP listener is also labelled remote, which is acceptable. Task 3 (count a bundle fetch once per skill version and session) is done after the bundle endpoint is merged, with the hook described in `plans/reports/cook-261011-remote-phase5.md`.
