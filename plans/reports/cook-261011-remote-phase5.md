# Cook report: remote hub Phase 5 (telemetry, sessions and privacy), tasks 1, 2 and 4

Date: 2026-10-11 (Asia/Saigon). Plan: `plans/261010-1652-remote-hub-mcp-over-network/`. Status: tasks 1, 2 and 4 done; task 3 (count a bundle fetch as a load) waits for the Phase 3 bundle endpoint. The phase stays open.

## Outcome

Remote sessions now produce the same observations as local ones, and their telemetry can be told apart from local telemetry. Each HTTP session keeps its own prior-resolution memory, so chains and session-aware cases work per remote session. Every event, case and rollup from a remote caller carries the client name `remote:<host>`, for example `remote:claude-code`. Stdio labels are unchanged. Neither the bearer token nor the client address reaches the telemetry store, the case journal, the diagnostics log, or any file in the workspace or the isolated home.

## What changed

| Area | Where | Change |
|---|---|---|
| Client label | `internal/telemetry/client.go` (new) | `ClientRemotePrefix = "remote:"` and `RemoteClient(Client) Client`. A missing name becomes `remote:other`, and calling it twice still gives one prefix. No event type, no payload field and no `EventVersion` changed. The label is a valid client-name token, so the published v1 schema still accepts it unchanged. |
| Label hook | `internal/delivery/mcpserver/server.go` | New `remote` field and one `sessionClient(session)` helper, now used by `clientForReq` (the tools/list size metric) and `callerContext` (resolutions, feedback through the source event, cases). In `recordLoad`, a remote load takes the session's remote label; locally a load with no earlier resolution is still labelled `skillhub`. |
| Listener | `internal/delivery/mcpserver/http.go` | `ServeHTTP` sets `adapter.remote = true` before it starts serving. net/http's own error log now goes through `httpErrorLog`, which removes IPv4 and bracketed IPv6 addresses. net/http prints the client address when it recovers a handler panic, and this keeps that address out of the log. |
| Doc comment | `internal/app/caller_context.go` | Says that each session has its own random hash, that remote callers carry the remote label, and that neither field ever holds an address or a credential. |
| Design doc | `docs/design/04-telemetry-reproducibility-evaluation.md` §4.4 | A new "Remote clients" section (in Vietnamese, like the rest of the doc) listing what is recorded and what is never recorded. |

`activation_tracker.go` did not change. It is already keyed by `*mcp.ServerSession`, and every HTTP session shares one `*mcp.Server`, so session isolation needed no code. The new tests below confirm it.

Why `remote:<host>` and not a separate field: the `client` object in `schemas/telemetry-event-v1.schema.json` has `additionalProperties: false`, so a new field would mean a schema change outside this phase. Every report that groups by client (`usage baseline`, `funnel --by client`, the `tools_list_bytes:<client>` rollup) already groups by `client.name`. With the prefix, remote numbers form their own bucket for each host, and local buckets stay comparable with earlier data.

## Acceptance

| Criterion | Evidence |
|---|---|
| Two concurrent remote sessions keep separate chains; a reconnect starts with empty memory | `TestRemoteSessionsKeepSeparateChainsAndCases`: one real listener, two concurrent sessions, then the first closes and a third connects. The second and third sessions reject the first session's resolution, and the hub records `rejected` (not verified) with `prior_verified: false`. All three session hashes differ. |
| `verified_reformulation` and `repeated_gap` are recorded for a remote session as for a local one | Same test: the first session gets `verified_reformulation` and exactly one `repeated_gap`. The second and third sessions resolve the same gap task once and record no case. `TestLocalSessionsRecordTheSameCasesWithUnchangedLabels` runs the same scenario on a local server and gets the same four cases. |
| Remote client label | The remote test checks that resolutions, loads (also a load with no earlier resolution), the tools/list metric and every case carry `{remote:claude-code, 1}`. `TestRemoteClientEventsAreStoredAndRolledUpPerClient` (telemetry) shows the label passes validation and gets its own rollup bucket. |
| Stdio labels unchanged (regression) | The local test checks the bare `{claude-code, 1}` label on resolutions, loads, the metric and cases, `skillhub` on a load with no earlier resolution, and that `New` never sets the remote flag. |
| Case journal stays opt-in | `TestRemoteCaseJournalStaysOptIn`: with the journal off, a remote reformulation writes no case but does write telemetry events. |
| Redactor runs on the hub for remote text | `TestRemoteCallsStoreNoTokenAndNoClientAddress`: the case task equals `telemetry.NewRedactor(root).Redact(raw)`, and the raw API key and email are found nowhere on disk. |
| No token and no client address in the store, journal, logs or files | Same test. The client dials from `127.0.0.77`; on systems that only configure 127.0.0.1, such as macOS, it falls back to the full `ip:port`. It makes four failed authentications: no token, a wrong token, the real token under the `Basic` scheme, and the real token in the query string. Then it makes a successful session. Afterwards it scans every file under the workspace (the SQLite store with its WAL and SHM files, `case_journal.json`, snapshots) and under the isolated HOME/XDG root, plus the server diagnostics, for the token, every client `ip:port` the listener accepted, and the client IP. `TestHTTPErrorLogDropsClientAddresses` makes a real handler panic and checks the address is gone from the log. |
| Tests catch the faults | Mutation check: with `adapter.remote` forced to false, the remote label test fails; with the address scrubbing removed, the error-log test fails. Both changes were reverted. |

Commands run (all pass):
- `go test -race -count=1 ./internal/telemetry/ ./internal/delivery/mcpserver/ ./internal/app/`
- `make check` (exit 0, golangci-lint 0 issues)
- `GOOS=windows go vet` and `GOOS=darwin go vet` on `./internal/telemetry/ ./internal/delivery/mcpserver/ ./internal/app/`

All servers in the tests bind loopback port 0 inside temp workspaces. No process outside the test binaries was started, and the live hub was not touched.

## Task 3 (deferred): how the bundle endpoint and the proxy should count loads

The hub side belongs in the bundle method in `distribution.go`, which is an MCP method, so it has an `*mcp.ServerSession`:

1. After the trust check, call the existing `adapter.recordLoad(ctx, session, app.SkillLoad{SkillID: id, ResourceKind: "entrypoint", Surface: "skill_bundle"})`. It already attributes the load to the session's latest resolution, sets the session hash and the remote label, marks `first_activation`, and writes `override` or `after_no_skill` cases. For an unapproved skill, call it with `Blocked: true` and the review reason codes, exactly as `readResource` does.
2. Count once per skill version and session. Add `activationTracker.markBundleFetch(session, skillID, digest) bool`, using the key `"bundle\x00"+skillID+"\x00"+digest` in `sessionState.activated` under the tracker mutex, and call `recordLoad` only when it returns true. `markActivation` dedupes per resolution, not per digest, so it is not enough on its own.
3. Telemetry, additive only: add `"skill_bundle"` to `skillLoadSurfaces` in `internal/telemetry/events.go` and to the `surface` enum of `skillPayload` in `schemas/telemetry-event-v1.schema.json`. Change nothing else. A 304 or "not modified" answer to a conditional fetch should go through the same step 2, so a session that revalidates still counts once.

Proxy cache hits (no content request reaches the hub):
- Recommended: return the cached `local.path` at once, without waiting on the network, then send the hub a fire-and-forget "known digest" call on the same bundle method (`known_digest` / `If-None-Match`). The hub answers "not modified" with zero bytes of content and counts the load through steps 1 and 2. The load stays server-observed (`basis: server-observed`), is attributed to the session's resolution, and stays within the cache-hit budget (at most 20 ms and zero bytes of content) because the client never waits for the answer.
- Fallback, if the hub is unreachable or the endpoint has no conditional form: when the proxy has a `resolution_id` from the `skill_resolve` it forwarded in the same session, it calls `skill_feedback` with `outcome: "loaded"`, `selected_skill: <id>`, no `utility` or `basis` (those two are accepted only together and only for a utility judgement), and a deterministic `event_id` such as `cache-<session>-<skill>-<digest-prefix>` so that retries dedupe. Without a resolution there is no source event to attach feedback to, so it drops the report and counts it in proxy diagnostics. Report `"used"` only when the host really reports use, never from a cache hit.

## Deviations and notes

- My worktree branch started before Phase 2 was merged, so I fast-forwarded it to `main` (`6ba7574`) before starting. No conflicts.
- The phase text said task text arrives "only over TLS or loopback". Decision Q4 removed TLS from this plan, so the ticked line now reads "loopback or a confirmed private network (Q4, no TLS in this plan)".
- I did not edit any file owned by Phase 3 (`skill_bundle.go`, `distribution.go`, `proxy.go`, `mcp_proxy.go`, `internal/skillruntime/`). The edits to the shared `server.go` and `http.go` are small and additive: one field, one helper, one flag assignment, and one error-log helper.

## Unresolved questions

1. Is `remote:<host>` the label the user wants, or should remote calls fold into the bare host bucket (one `claude-code` series) and keep the remote flag somewhere else? The second option needs an additive schema change for a `client.transport` field.
2. A local agent on machine B that connects through the HTTP listener, not stdio, is also labelled remote, because the label follows the transport and not the machine. Is that acceptable?
