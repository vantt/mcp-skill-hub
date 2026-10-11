# Cook report: remote hub Phase 5 task 3 (count skill loads from bundles and cache hits)

Date: 2026-10-11 (Asia/Saigon). Plan: `plans/261010-1652-remote-hub-mcp-over-network/`. Status: completed; Phase 5 is now completed.

## Outcome

Remote use now counts skill loads the way local use does. Each time the hub delivers a whole skill (HTTP 200) or confirms the copy a client already holds (HTTP 304), it records one server-observed `skill.loaded` event with `surface: skill_bundle`. The event goes through the same `recordLoad` path as a local `skill_get`, so it is attributed to the session's latest resolution, sets `first_activation`, and writes `override` and `after_no_skill` cases. The hub counts each skill version once per session. A cache hit on the client is still answered at once. In the background, the proxy asks the hub to confirm the version, and the hub counts that confirmation. The proxy's hub session is labelled `remote:<host>`. This was checked with a real proxy process against a real listener.

## What changed

| Area | Where | Change |
|---|---|---|
| Hub load | `internal/delivery/mcpserver/distribution.go` | `skillBundle` calls the new `recordBundleLoad` after a delivered or not-modified bundle (`ResourceKind: entrypoint`, `Surface: skill_bundle`). For `ContentReviewRequiredError` it calls `recordBundleLoad` with `Blocked: true` and the review reason codes, from the same trust verdict that `resources/read` uses. The surface was `skill_get` before; it is now `skill_bundle`. |
| Dedupe | `internal/delivery/mcpserver/activation_tracker.go` | `markBundleFetch(session, skillID, digest) bool` uses the key `"bundle\x00"+skill+"\x00"+digest` in the session's `activated` map, under the tracker mutex. A blocked skill passes an empty digest, because the refusal does not name a version, so it counts one blocked load per skill and session. A nil session is the tracker's existing `anon` state, now documented as the "no session" state: it has its own hash and never touches a real session. The no-session state is shared by every client of the hub, so its loads are not deduplicated on the hub; otherwise one machine's load of a version would hide every other machine's for the hub's lifetime (found in review). The proxy's own `counted` map still keeps one proxy from repeating its no-session confirmations. |
| Session attribution | `distribution.go` (comment only) | The HTTP handler already resolves `Mcp-Session-Id` through `server.Sessions()`. When the header is missing or names a session that is no longer live, the request goes to the no-session state, with `attribution: unsolicited` and its own `session_id_hash`. |
| Proxy cache hit | `internal/delivery/mcpserver/proxy.go` | `confirmLoad` runs on a hit. If the hub has not yet counted this version in the current hub session (`counted` map, key = hub session + skill + digest), it starts one goroutine. That goroutine sends a conditional GET with `If-None-Match: "<cached digest>"` and `Mcp-Session-Id`, through the existing `requestBundle`. A 304 marks the version counted. A 200 means the hub has a newer version: it is installed and the proxy remembers it. The proxy also marks versions counted after its own foreground 200 or 304 answers, so later hits in the session send nothing. An in-flight set allows at most one confirmation per key. `close` cancels the background context and waits for the confirmations before it closes the hub session. |
| Unreachable hub | `proxy.go` (`reportCachedLoad`) | When the background request cannot reach the hub, the proxy does not retry and does not warn. It makes one attempt only when the hint carries a `resolution_id` from the hub session that is still open: `skill_feedback` with `outcome: loaded`, `selected_skill`, and `event_id = "cache-" + hex(sha256(session, skill, digest))[:40]`. The attempt goes over the existing session and never reconnects. It never reports `used`. |
| Client label | `proxy.go` (`noteClient`) | `noteClient` now reads `request.ClientInfo()`, which takes the host name from the initialize handshake or from the per-request `_meta` that hosts on the newer protocol send without an initialize. Before, it read only `InitializeParams`. |
| Telemetry (additive) | `internal/telemetry/events.go`, `schemas/telemetry-event-v1.schema.json` | `skill_bundle` is added to `skillLoadSurfaces` and to the `surface` enum. Nothing was removed, and `EventVersion` is still `"1"`. |
| Docs and plan | `docs/design/04-telemetry-reproducibility-evaluation.md` §4.4 (Vietnamese), `phase-05-telemetry-and-privacy.md` | The doc section now describes how loads are counted. In the phase file, task 3 and its verification line are ticked and the status is completed. |

## Client label check (task item 3)

I ran a real `skillhub mcp proxy` subprocess against an in-process `serve mcp --addr 127.0.0.1:0` listener, with the agent host named `claude-code`. The hub recorded `{remote:claude-code, 1}` on the resolutions and on the load. The Phase 3 concern did not apply: against the stateful HTTP handler, the Go SDK client falls back to `initialize`, and the proxy forwards the host's own `clientInfo`. A host that sends no name still gets `skillhub-proxy`, which normalizes to `remote:other`. The only fix needed was the per-request `_meta` case described above. It is pinned by `TestProxyNotesTheHostNameFromRequestMetadata` and, end to end, by `TestMCPProxyProcessCountsLoadsOnTheHub`.

## Acceptance

| Criterion | Evidence |
|---|---|
| One load per skill version and session | `TestBundleRequestsCountOneLoadPerVersionAndSession`: two fetches and a revalidation in session A give 1 load. A revalidation and a fetch in session B give 1 load. A changed version fetched twice in A gives 1 more load (2 in A in total). A request without a header plus one with an unknown session give 2 loads under a single third hash, each `attribution: unsolicited` (the shared no-session state is not deduplicated). Session loads are `recommended` and attributed to their own session's resolution. Also covered by the unit test `TestMarkBundleFetchCountsEachVersionOncePerSession`. |
| Blocked skill records a blocked load | `TestBlockedBundleRecordsOneBlockedLoad`: two 403s give one load with `status: review_required`, `reason_codes: [content_review_required]` and `first_activation: false`. |
| Cache hit through a real proxy process records one load quickly, without latency or content | `TestMCPProxyProcessCountsLoadsOnTheHub`: a counting link with 250 ms RTT sits in front of the real listener. The hit took 1.4 ms, and the hub recorded the load about 270 ms later (one round trip). Exactly one bundle request was sent, answered 304 with 0 content bytes. The repeat hit sent no request. The hub has 2 `skill_bundle` loads (fetch, hit) in 2 sessions, both `remote:claude-code` and `recommended`. In process, `TestProxyCacheHitCountsOneLoadOnTheHub` uses a 300 ms RTT link: hits took 2 to 7 ms without the race detector, and the hub confirmed after about 310 ms. |
| Unreachable hub does not slow or break the hit | `TestProxyCacheHitWithTheHubDownStaysFast`, with a hub that refuses connections and with a hub that accepts but never answers. Three hits each stay inside the budget. A silent hub leaves exactly one confirmation in flight and no warning, and stopping the proxy cancels it at once. The process test also makes the link silent: three hits stay within 20 ms, no warning is printed, and the proxy exits 0. |
| Fallback when the bundle request fails | `TestProxyCacheHitWithoutTheBundleEndpointFallsBackToFeedback`: two hits make one attempt each (no retry loop) and give exactly one host-reported `skill.loaded` with the fixed event_id. No `skill.used` event is written. |
| Schema still passes, EventVersion unchanged | `schemas/embed_test.go` accepts `skill_bundle` and refuses an unknown surface. `TestSkillLoadAcceptsTheBundleSurface` (telemetry) checks every old surface, the new one, and that `EventVersion == "1"`. |
| No token and no client address stored | `TestRemoteCallsStoreNoTokenAndNoClientAddress` now also makes a bundle 200 and a 304 from the distinct client IP with the session header, and asserts that 1 bundle load was recorded before scanning. The process test scans the hub workspace for the token. |
| Tests catch the faults | Mutation check: with the `markBundleFetch` guard disabled, the count, blocked and privacy-scan tests fail. With the `confirmLoad` call removed, both cache-hit tests and the process test fail. Both mutations were reverted. |

Commands run (all pass):
- `go test -race -count=1 ./internal/delivery/mcpserver/ ./internal/telemetry/`
- `make check` (fmt-check, vet, golangci-lint 0 issues, full `go test ./...`)
- `GOOS=windows go vet` and `GOOS=darwin go vet` on `./internal/delivery/mcpserver/ ./internal/telemetry/ ./internal/delivery/cli/ ./schemas/`

Every listener and proxy in the tests binds loopback port 0 inside temp workspaces, with HOME and XDG isolated by `TestMain`. The only processes started were test binaries and their proxy subprocesses, and each test stops its own. The live hub was not touched.

## Review

An independent code review found no critical or high issue. It confirmed the background lifecycle: `Add` runs under `mu` only while background work is live, `close` cancels under `mu` before it waits, there is no deadlock with `reportCachedLoad`, and the maps are only touched under `mu`. It confirmed that no new network wait was added to the hit path. Fixed after the review, with tests:

- Medium: the hub deduplicated the shared no-session state across all clients. It no longer deduplicates that state (see the Dedupe row). `TestBundleRequestsCountOneLoadPerVersionAndSession` now expects 2 loads for 2 no-session requests.
- Medium: a successful background confirmation could print "The Skill Hub ... answers again." Background requests (`requestBundle(..., quiet=true)`) no longer touch the reachability notice. `TestProxyCacheHitCountsOneLoadOnTheHub` marks the hub unreachable first and asserts that the notice is absent. With the fix removed, the test fails.
- Low: a background request no longer runs cache garbage collection, which needs the fetch lock it does not hold.
- Low: the `skill_feedback` fallback gets its own 5 s deadline, because the bundle request may have used up the old one.

Kept: when the hub has a newer version, the background confirmation downloads it (content bytes, but never on the hit path), so the next `skill_get` needs no fetch. The 20 ms wall-clock budget includes a stdio round trip and could trip on a heavily loaded CI host. The "no further request" checks use a 100 ms wait. They fail safely, but they cannot prove that nothing was sent.

## Bug found and fixed while testing

The first version of `confirmLoad` held the skill's fetch lock while it waited for the hub. With a hub that accepts connections but never answers, the next `skill_get` of that skill waited 15 s, the confirmation timeout. The background confirmation no longer takes the fetch lock. The cache install is atomic and already tolerates concurrent writers, which it needs for several proxy processes anyway. The silent-hub test pins this.

## Deviations and notes

- Worktree: my branch started at `c4bf1eb`, before Phase 3 was merged, so I fast-forwarded it to `main` (`36ade7f`) first. There were no conflicts.
- Existing tests touched: only `remote_sessions_test.go` (bundle requests added to the privacy scan, plus the `io` import). No Phase 3 assertion had to change. The "zero requests on a repeat" checks still hold, because the proxy sends nothing for a version the hub already counted in the session.
- New test-only files `race_detector_{on,off}_test.go` in `mcpserver` and `cli` set the hit budget: 20 ms normally, 100 ms under `-race`. The tests that check the hit does not wait add a 250 to 300 ms round trip, so a hit that waited would fail under either budget.
- `http.go` and `server.go` were not edited.
- Closing a proxy whose hub silently stopped answering takes about 5 s. The cause is the MCP client's session close (the DELETE to the hub), not the background confirmation, which is cancelled at once. This was already the case before this change.
- Commit trailer: the task asked for `Co-Authored-By: Claude Sonnet 5.5`. The session attribution rule names the model that did the work, so the commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Amend if you want the other line.

## Unresolved questions

1. With the key skill + digest per session, a second resolution in the same session that recommends the same, already-counted version records no new load, so no second `recommended` activation. A local `skill_get` would record one, because `first_activation` is tracked per resolution. So in long sessions that resolve to the same skill more than once, the remote acceptance rate can still be lower than the local one. Adding the attributed resolution id to the key (once per version, session and resolution) would match local exactly. It still counts once for repeated fetches under one resolution. Should the key change?
2. When a skill is blocked, the proxy forwards `skill_get` after the 403, so the first blocked attempt through the proxy records two blocked loads (`skill_bundle` and `skill_get`). Accept this, or have the hub skip one of them for remote callers?
3. The fallback `skill_feedback` "loaded" event is host-reported, so the rollup does not count it as an activation. It only keeps the load visible in the feedback funnel. Is that enough, or should a later reachable hit (which is counted server-observed, as now) be the only path?
