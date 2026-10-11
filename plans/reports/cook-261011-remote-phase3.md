# Cook report: remote hub Phase 3 (skill bundle delivery and client cache)

Date: 2026-10-11 (Asia/Saigon). Plan: `plans/261010-1652-remote-hub-mcp-over-network/`. Status: completed.

## Outcome

A remote client now gets a whole approved skill in one HTTP exchange, verifies it against the digest its skill URI pins, keeps it in a local cache keyed by that digest, and runs it there. `skillhub mcp proxy --hub <url>` is the stdio entry an agent host launches: it forwards `skill_resolve` and `skill_feedback`, answers `skill_get` from the cache, and returns `local.path` and `local.env` on the client machine, so `check`, `setup` and scripts run exactly as with a local hub. A repeat use of a resolved version makes no request at all. The network listener no longer returns any path on the hub's disk.

## What was built

| Area | Where | Notes |
|---|---|---|
| Bundle (hub) | `internal/app/skill_bundle.go` | `DistributionService.SkillBundle` opens the catalog generation once, evaluates trust in the same pass, reads every distributed file once, and writes a deterministic gzip tar: a metadata entry (`.skillhub-bundle.json`: skill id, manifest version, file list with digests and kinds, dependency key, runtime block, the skill detail without SKILL.md text) and then exactly the files the URI digest covers, `.meta/` excluded. A held digest returns `NotModified` without reading files. Unapproved third-party skills return `ContentReviewRequiredError` (a held digest never bypasses it). `SnapshotService.RemoteStatus` gives the local status with no path and without exporting a snapshot. `ClientLocalSkill` builds the `local` block on the client (state directory seeded with dependency manifests, config directory, env, preflight with the client's platform). |
| Bundle format and client cache | `internal/skillruntime/remote_cache.go` | `WriteBundle`, verifying extraction (every entry must be listed, regular, of the listed size and digest; the file list must hash to the requested digest; unsafe paths, symlinks, duplicates and extra entries are refused; compressed and decompressed size bounded by the catalog limits of 512 files and 16 MiB). `RemoteCache` layout `<state>/remote/<hub-id>/<skill-id>/<digest>/`, written in `<hub-id>/.staging` and renamed into place, never over an existing directory. `Lookup` checks directory, metadata and every listed file's presence and size (no re-hash; files are read-only and were verified at extraction). GC keeps 3 versions per skill, drops versions unused for 30 days, caps a hub at 512 MiB, never removes the version just installed, and removes crash leftovers in staging after an hour. `ClientStateDir`: `$XDG_STATE_HOME/skillhub`, else `%LocalAppData%\skillhub` (Windows), `~/Library/Application Support/skillhub` (macOS), `~/.local/state/skillhub`. State and config directories of remote skills live in `<hub-id>/.state/runtime/{envs,config}` and survive cache clearing. |
| Hub delivery | `internal/delivery/mcpserver/distribution.go` (new); one-line additive edits in `server.go` and `http.go` | `GET /mcp/bundle?skill=<id>` behind the existing gate (Host, token, rate limit): 200 with `ETag: "<digest>"`, 304 for `If-None-Match` naming the current digest, 403 `content_review_required` with the same message as `resources/read`, 404 `skill_not_found`. `skills/bundle` serves the same bytes over MCP (stdio) from the same code path. A delivered or confirmed bundle records a `skill_get` load, attributed to the caller's MCP session when the proxy sends `Mcp-Session-Id`. On the network listener `skill_get`, `skills/get` and `resources/read` carry no path (`remoteCallers`). |
| Proxy | `internal/delivery/mcpserver/proxy.go` | Offers the hub's three runtime tools with identical names and schemas (registered on a throwaway server and listed in memory, so they cannot drift). Remembers each resolved skill's digest for 5 minutes (the hub's cache TTL). `skill_get`: hint and cache hit, no request; otherwise one bundle GET naming cached digests; 200 installs, 304 uses the cache (both also count as a hint, so the next `skill_get` in the session needs no request); 403/404 forwards `skill_get` so the hub answers drafts, blocked skills and the curator exactly as for a local caller; a hub that cannot be reached at all leads to the cached copy with `remote.cache = "offline"` and a message, or `hub_unreachable`, but never for a skill the latest resolution flagged `review_required`; a hub that answers 429 or 5xx returns `hub_error` and no cached content; a refused token returns `hub_token_rejected` (also for forwarded tools). Redirects are never followed and the token is sent only to the hub's host. One bundle download is bounded to 2 minutes. Hub sessions are opened on first use with the host's client name and replaced once if they expire. Keep-alive HTTP/1.1 (HTTP/2 needs TLS, which this plan excludes); proxy environment variables are ignored. |
| CLI | `internal/delivery/cli/mcp_proxy.go`, `cache.go` (new); routing in `mcp.go`, `root.go`; `help.go` | `skillhub mcp proxy --hub <url> [--token-file <path>]` (token from the file, mode 0600 checked, or `SKILLHUB_HUB_TOKEN`; never printed). One start-up `Fields` block and all warnings and errors on stderr through `termui`; stdout carries only MCP. `skillhub cache clear --hub [<url>] [--yes]` previews (table per hub) and applies with `--yes`. Global help, `help mcp` and `help cache` updated. |

## Acceptance

| Criterion | Evidence |
|---|---|
| Extracted folder digest equals the hub's for test-audit and herdr-cook-plan | `TestProxyFetchesWholeSkillsOnceAndServesThemFromTheCache` (fixture skills of the same shape, `.meta/` present on the hub and absent in the copy), `TestBundleOverHTTPAndStdioShareOneCodePath`, `TestSkillBundleHoldsExactlyTheDistributedFiles` (byte comparison with the hub files). Also run on a read-only clone of `/home/vantt/skill-hub` (approval of test-audit only inside the clone) in `TestProxyFetchSpeed`. |
| One exchange fetches a whole skill (counted) | same proxy test: exactly 1 request, a bundle request, per first `skill_get`. |
| Second use makes zero content requests (counted) | same test: 0 requests of any kind after a resolution; a new session without a resolution makes 1 request answered 304 with 0 content bytes. |
| Changed skill refetches, unrelated change does not | same test, subtest: a file edit gives a new digest and one fetch; a routing-only `.meta` edit gives 0 requests. |
| Tampered archive rejected | `TestInstallRejectsTamperedArchives` (changed bytes, changed list, wrong digest, corruption, truncation), `TestInstallRejectsUnsafeArchiveEntries`, `TestProxyRejectsATamperedBundle` (corrupted in transit through the proxy; nothing cached). |
| Unapproved skill blocked like local | `TestBundleOfUnapprovedSkillIsBlockedLikeLocalReads` (HTTP 403 and stdio error with the `resources/read` message; held digest does not bypass), `TestProxyKeepsTheReviewGate` (the proxy's `skill_get` answer deep-equals the local hub's), `TestSkillBundleRefusesWhatLocalDeliveryRefuses`. |
| Crash during extract leaves no half skill | `TestInstallCrashLeavesNoPartialSkill` (crash right before the rename: nothing visible, leftover removed by GC, reinstall works), plus truncated and tampered archives leave no version or staging directory. |
| `check` runs on the client end to end on loopback | `TestMCPProxyRunsARemoteSkillCheckOverStdio`: real listener (`serve mcp --addr 127.0.0.1:0`), real proxy subprocess over stdio, resolve, `skill_get`, then the `local.preflight.check` command run in `working_directory` with `env`, reading a file from the cached folder. Also `TestProxyRunsASkillCheckOnTheClient` (checks nothing was prepared on the hub). |
| No hub path over HTTP | `TestNetworkListenerNeverReturnsHubPaths`, `TestRemoteStatusCarriesNoPath`. |
| stdio unchanged | existing `TestServeMCPWithoutAddrIsTheStdioServer`, subprocess and server tests green; stdio tool list unchanged (the new `skills/bundle` is a custom method, not a tool). |
| Proxy never serves content the hub refused | `TestProxyServesNoCachedCopyWhenTheHubAnswersAnError` (503 gives `hub_error`, no cached copy; only a closed hub falls back), `TestProxyServesNoCachedCopyAfterAReviewRequiredResolution`, `TestProxyFollowsNoRedirect` (no request reaches the redirect target). |
| termui golden output | `TestMCPProxyStartupErrorsGolden`, `TestProxyBannerGolden`, `TestCacheClearGolden`, `TestHelpDescribesTheRemoteCommands`. |

Commands run:
- `go test -race -count=1 ./internal/skillruntime/ ./internal/app/ ./internal/delivery/mcpserver/ ./internal/delivery/cli/ ./internal/hubtoken/`: pass.
- `make check LINT_BASE=main` (fmt-check, vet, golangci-lint new issues, full `go test ./...`): pass, 0 lint issues.
- `GOOS=windows go vet` and `GOOS=darwin go vet` on `./internal/skillruntime/ ./internal/app/ ./internal/delivery/... ./internal/hubtoken/`: pass.

## Speed (`SKILLHUB_PERF=1 go test -run TestProxyFetchSpeed ./internal/delivery/mcpserver/`)

20 runs per row, Linux, no race detector, other agents active on the machine. "First skill_get after resolve" reuses the connection the resolution opened (the normal resolve-then-get pair). "New connection" is a fresh proxy whose first call is the fetch, so it also pays the TCP handshake. The simulated link is a loopback TCP relay that delays every chunk by half the round trip in each direction and the first chunk of a connection by one more round trip (the handshake); it does not limit bandwidth.

Clone of the live hub (read-only `git clone` of `/home/vantt/skill-hub`):

| Skill | Link | Bundle | First get after resolve p50 / p95 | First get, new connection p50 / p95 | Cached repeat p50 / p95 | Repeat requests |
|---|---|---|---|---|---|---|
| test-audit | loopback | 6.2 KB | 29.0 / 32.9 ms | 33.5 / 44.6 ms | 0.8 / 1.4 ms | 0 |
| test-audit | 100 ms RTT | 6.2 KB | 128.7 / 156.0 ms | 233.9 / 260.5 ms | 0.8 / 4.3 ms | 0 |
| herdr-cook-plan | loopback | 60.0 KB | 31.1 / 66.2 ms | 36.6 / 55.5 ms | 2.2 / 5.2 ms | 0 |
| herdr-cook-plan | 100 ms RTT | 60.0 KB | 153.2 / 181.5 ms | 253.4 / 273.4 ms | 3.1 / 5.9 ms | 0 |

Against the budgets: first fetch of herdr-cook-plan 37 ms p50 on loopback (budget 500 ms on a LAN) and 0.25 s at 100 ms RTT including the handshake (budget 2 s; the stdio baseline projected 1.85 s for 16 round trips). Cached repeat at most 6 ms p95 with zero requests and zero bytes (budget 20 ms). On a real 100 ms link TCP slow start adds about two more round trips for the 60 KB body, so expect about 0.45 s; still well inside the budget. On the hub, about 10 of the 13 ms a bundle takes is `GetSkillDetail` (the same read model local `skill_get` returns); the rest of the loopback time is extraction and the local `skill_get` answer on the client.

## Review

An independent code review (security, atomicity, proxy behavior) found no critical issue. Extraction confinement, symlink handling in lookup, clear and GC, crash safety, the hub-side review gate (trust is checked before a 304), the absence of hub paths over HTTP, token handling and stdout cleanliness were confirmed. Fixed after the review, with tests:

- High: the offline fallback also fired on 429/5xx answers and right after a resolution flagged the skill for review, so a copy the hub had just refused could be served. Now only a hub that cannot be reached falls back, never for a flagged skill; 429/5xx return `hub_error`.
- Medium: redirects re-sent the bearer token (the token was added in the round tripper on every hop). Redirects are no longer followed and the token goes only to the hub's host. The reachability warning no longer flaps on error answers.
- Low: retry when another process removed the empty skill directory before the rename, or when Windows still holds a new file (5 short attempts); the rename comment was corrected; a connection attempt no longer blocks bundle requests (session ID kept in an atomic, client name under its own lock); a 2-minute deadline per bundle download; the stdio bundle limit leaves 64 KiB for the JSON-RPC envelope; `cache clear --yes` removes crash leftovers even when no version is cached; a cancelled or timed-out call is returned as such instead of "hub unreachable".

Kept, with the reason:

- Cache hits after a resolution send nothing to the hub, so the hub records no skill load for them (see the Phase 5 hooks). The plan and the task require zero requests for a repeat use; a 304 confirmation would record the load but costs a round trip (100 ms on a slow link, over the 20 ms budget).
- A resolution without a `setup` block counts as trusted for the cache-hit path. That is the normal answer for a trusted skill without a runtime block; the resolver omits it only for those, or when its own trust lookup fails. The exposure is limited to a version that was approved when it was cached.

## Deviations and decisions to confirm

- `skill_resolve` was not changed: its answer already carries `primary.version` and `supporting[].version` (the manifest digest), which is all the proxy needs to use the cache without a request.
- "Skip files over the existing size caps": a bundle must hold exactly the files the digest covers, so files cannot be skipped one by one. A skill over the caps (512 files or 16 MiB) is not servable at all, as for `resources/read`, and the client refuses archives beyond those caps.
- The proxy opens its hub connection on the first call, not at start-up: modern MCP clients skip `initialize`, so there is no reliable start-up hook, and a lazy connection keeps request counts exact. The first `skill_resolve` therefore pays the TCP connect and the MCP handshake.
- `skill_get` through the proxy adds a `remote` object (`hub`, `cache` = `hit` / `fetched` / `offline`, `message`) next to the usual fields; the tool output schema is open, so this is additive.
- When the hub cannot be reached, the proxy serves the most recently used cached version of a skill, which may be older than the hub's current one or, in principle, one whose approval was withdrawn while the hub was down; the result says so in `remote.message`. This follows the phase's failure rule.
- A cache hit reuses the skill detail captured with that version. Metadata-only hub changes (routing, rationale) therefore show up in `skill_get` only after the next fetch of a new version; content and trust are current because the version comes from a fresh resolution, and a resolution that marks the skill `review_required` always goes to the hub.
- Commit trailers: the task asked for `Co-Authored-By: Claude Sonnet 5.5`; the session attribution rule names the model that did the work, so the commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Amend if the lead wants the other line.

## Hooks for Phase 5 (files owned by the parallel phase, not edited)

- Cache hits make no request, so the hub records no skill load for them. Activation telemetry for remote clients counts fetches and 304 confirmations only. If Phase 5 wants every use counted, the proxy could batch use counts into the next `skill_resolve` or `skill_feedback` call; that needs a field the hub accepts.
- Bundle loads are attributed through `Mcp-Session-Id` to the proxy's hub session (`sessionByID` in `distribution.go`, then `recordLoad`); a bundle fetched before any resolution has no session and counts as unsolicited.
- The proxy's hub session uses the Go SDK client, which negotiates the modern protocol without `initialize`, so `InitializeParams().ClientInfo` may be empty on the hub and the client label falls back to `other`. Phase 5's client label should not rely on `InitializeParams` alone.

## Not verified

- Windows and macOS were checked with `go vet` only. The cache relies on `os.Rename` of a directory to a missing target (never over an existing one), `os.Chtimes` on directories (also used by the local snapshot code), and chmod-before-delete for read-only files. Deep skill paths under `%LocalAppData%\skillhub\remote\<hub>\<skill>\<64-hex digest>\` may exceed 260 characters; Go handles long absolute paths, but shells and tools that run skill scripts may not.
- The Tailscale path and a real 100 ms link were simulated only.

## Unresolved questions

1. Should cache hits be reported to the hub for telemetry (see the Phase 5 hooks), at the cost of a small piggy-backed field?
2. `skillhub skill env set` writes secrets into a hub workspace; for remote skills the client config directory is `<state>/remote/<hub-id>/.state/runtime/config/<skill-id>/`. Should Phase 4 add a client-side way to store them there?
3. The proxy exposes only the three tools; `skills/list`, `skills/get` and `resources/read` are not proxied (the bundle replaces them). The hub's `skill_resolve` description still mentions `skills/get` and `resources/read`. Keep, or adjust the description for remote clients?
