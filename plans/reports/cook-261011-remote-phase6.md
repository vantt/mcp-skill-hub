# Cook report: remote hub Phase 6 (benchmarks, two-machine tests and docs)

Date: 2026-10-11 (Asia/Saigon). Plan: `plans/261010-1652-remote-hub-mcp-over-network/`. Status: tasks 1 to 5 completed; task 6 (the run on the user's real machines) is left to the user.

## Outcome

Both headline criteria are now proved by repeatable checks. `make remote-bench` runs the hub and the proxy as real processes over stdio, loopback HTTP and a simulated 100 ms link, and fails when a budget is missed; on a read-only clone of the live hub every budget is met with a wide margin. New end-to-end, failure, security, one-machine and two-way tests fill the gaps the earlier phases left. Users get a standalone guide, `docs/remote-hub-guide.md`, and a script, `scripts/remote-timing.sh`, to time a real run from machine A. One production bug the new tests exposed was fixed in the skill cache, with a follow-up reorder in the proxy found in review.

## What was built

| Area | Where | Notes |
|---|---|---|
| End-to-end test | `internal/delivery/mcpserver/remote_e2e_test.go`, `TestRemoteHubEndToEndThroughATunnel` | Listener (`serve mcp --addr 127.0.0.1:0`) and proxy as real processes; a tunnel-like relay on a second OS-chosen port (what `ssh -N -L` gives); separate home, config and state directories for machine A and machine B; token copied to `~/.config/skillhub/hub-token` with mode 0600; `connect --hub` run on A and the proxy started from the written `.mcp.json`. For every skill of the hub: resolve, `skill_get`, folder digest and file count equal the hub's, the `SKILLHUB_*` variables point at A, and the answer equals machine B's own stdio `skill_get` (content, version, skill detail, runtime spec). The check runs on A from the cache. `skill_feedback` is recorded on the hub under `remote:claude-code`, with one bundle load and one resolution per skill. The token appears in no output, answer or file except the two token files; no answer names a path on B. |
| Failure tests | same file, `TestRemoteFetchFailures` | A "front" server in front of a real hub cuts a bundle download midway, truncates the archive with a valid HTTP answer, or plays a hub from before bundles (404 on `/mcp/bundle` and `/mcp/status`). Plus a read-only cache directory. Each case checks the agent's answer and that nothing half-written is cached; the interrupted case then retries successfully. Wrong token, unapproved skill, tampered archive, hub error answers and a stopped hub were already covered by Phase 3 tests and are referenced, not duplicated; version skew is doctor's job (Phase 4 tests). |
| Security tests | same file, `TestNetworkListenerReachesNoCurationTool` | Lists every tool of the full local server and calls each non-runtime tool through the listener and through the proxy: all "unknown tool". Web UI paths on the listener answer 404 even with the token. The refused addresses, the token-never-logged scans and doctor not sending the token to a hub only a project names are existing tests (listed in the table below). |
| One-machine regression | same file, `TestOneMachineStdioServerOpensNoPortAndStartsNoProcess` | Linux: the stdio server holds no socket (`/proc/<pid>/fd`), has no child process (`/proc/<pid>/task/*/children`) and creates no hub token. The written registrations stay pinned by the Phase 4 goldens. |
| Two-way test | same file, `TestHubServesLocalStdioAndRemoteProxiesAtOnce` | Three `mcp serve` processes and two proxy processes resolve at the same time (25 calls each at 50 per second) on one workspace with the listener; every resolution event is written and exactly the proxies' events carry the remote label. |
| Timing script test | same file, `TestRemoteTimingScriptTimesAFetchAndACachedRepeat` | Runs `scripts/remote-timing.sh --hub ...` against a real listener; checks the two lines, no token in the output, and that the user's own cache stays untouched. |
| Benchmark | `internal/delivery/mcpserver/remote_budget_bench_test.go` (`TestRemoteBudgetBench`, `SKILLHUB_PERF=1`), `scripts/remote-bench.sh`, `make remote-bench` | stdio: a new `mcp serve` per session. Remote: a new proxy with an empty cache per session, behind a relay that counts bytes and HTTP requests (the earlier delay relay in `proxy_test.go`, now `benchLink`). Per transport: session start, first and warm `skill_resolve`, first and cached `skill_get` per skill with requests and bytes. Then a metadata-only change to every skill and a content change to one skill on the hub, on both links. A plain-text report with aligned columns (also written to `REMOTE_BENCH_OUT`), one PASS/FAIL line per budget; a missed budget fails the test and `make`. `REMOTE_HUB` measures a read-only clone. It replaces `TestProxyFetchSpeed`. |
| Timing script | `scripts/remote-timing.sh` | POSIX sh with an embedded python3 MCP driver (pure sh cannot time in milliseconds portably). Uses the `connect --hub` entry of `./.mcp.json` or `~/.claude.json`, or `--hub` and `--token-file`. Runs the proxy with a new, empty cache in a temp dir (removed on exit), resolves, gets the skill twice, prints a small table. Never reads or prints the token. No PowerShell twin: on Windows run it in WSL or Git Bash with python3. |
| Guide | `docs/remote-hub-guide.md` (new) | Same voice and layout as the user guide: when Git clone is enough, the two ways of working, what runs where, setup for an SSH tunnel (listener, token hand-off with scp and `chmod 600` to `~/.config/skillhub/hub-token` or `SKILLHUB_HUB_TOKEN`, `ssh -N -L`, `connect --hub`), keeping the tunnel up (ssh config entry, autossh, systemd user unit) and the listener running (systemd user unit, linger), Tailscale (Tailscale address, `--trust-private-network`, why a wildcard address is refused, MagicDNS names and `--allow-host`, prefer `-g` because of doctor's token rule), doctor output and a cause/fix table, hub or tunnel down, the cache and `cache clear --hub`, security in plain words, `token rotate`, going back to one machine (`connect --remove --cache`, `connect --workspace`), timing a run, a cheat sheet. All output blocks were copied from real runs in an isolated temp HOME (paths shortened to `/home/you`, the OS-chosen port shown as 7420, versions as `<version>`; lines reflowed to the 100-column width the CLI uses when not on a terminal). |
| Guide check | `internal/delivery/cli/remote_guide_doc_test.go` | Every `skillhub ...` command in the guide's command blocks, printed `$ skillhub` lines and inline code: the command and subcommands exist and every flag appears in `skillhub help <command>`. Verified to fail on a wrong flag and a wrong subcommand. |
| Other docs | `docs/release-runbook.md`, `docs/design/remote-hub.md`, one line each in `README.md` and `docs/user-guide.md` | Runbook: what to check before a release that touches the remote path (`make remote-bench`, compatibility and `app.RemoteAPILevel`, the token is never in an artifact, plain HTTP must stay refused off loopback). Design note: the command syntax as built (token location, `--allow-host`, `--remove [--cache]`, `cache clear`, doctor, the three endpoints), Q4 with the wildcard rule, and a "Final numbers" section. |

## Bug found and fixed

`internal/skillruntime/remote_cache.go`, `writeBundleFile`: when an archive ended inside a file's contents (a download cut off when the hub stops, or a truncated body), the read error came back wrapped as a write failure. The proxy then answered `internal_error`, "The skill could not be stored in the local cache ...", not retryable, with the advice to check disk space and permissions. The Phase 3 truncation test cut the archive at a point that failed earlier, so it did not see this. Fix: a small reader wrapper remembers the first read error of the archive; such an error is now `ErrBundleInvalid`, which the proxy reports as `resource_digest_mismatch`, retryable, "Retry once; if it fails again, check the connection to the hub". Write errors on this machine keep their old message. Test: `TestInstallRefusesAnArchiveCutShortAnywhere` cuts a 30 KB archive at every 101st byte (failed before the fix at byte 708); a cut that loses only the closing padding and gzip trailer still installs, because every file was read in full and matched its digest. Commit `f707c50`. The review then found that this made a timed-out or cancelled download look like a broken archive in the proxy; see the Review section for that fix.

## Review

An independent code review (production fix, test flakiness and portability, the request counter, the bench's failure path, the scripts' token handling, and the guide against the code) confirmed that a real local write error is still reported as a storage failure, that the request counter can neither miss nor double-count, that the bench fails on a missed budget, that the timing script never reads or prints the token, and that the guide's messages, paths and rules match the code. Fixed after the review, with tests:

- High: the fix above made a cancelled or timed-out download look like a broken archive, because the proxy checked for an invalid bundle before it checked the call's context. A hub that stalls mid-download then gave `resource_digest_mismatch` instead of falling back to the cached copy, and a cancelled call got a tool error instead of the cancellation. `requestBundle` in `internal/delivery/mcpserver/proxy.go` now checks the context first (a minimal reorder of existing branches). New subtest of `TestRemoteFetchFailures`: a front hub sends half the archive and then stalls; with a short deadline the agent gets the cached copy (`remote.cache = "offline"`), and a cancelled call returns the cancellation. It fails without the reorder.
- Medium: `make remote-bench REMOTE_BENCH_OUT=<relative path>` wrote the report into the package directory; the script now makes the path absolute first.
- Medium: the one-machine test skips when the kernel does not list child processes, instead of passing without checking.
- Low: the timing script stops the proxy before it exits on an error, so the temporary cache is removed after the proxy is gone, and its test now checks that the temporary directory is empty afterwards; the guide check matches whole flags only; the design note no longer names a personal path.

Kept: the benchmark requires exactly 0 bytes on a cached repeat; a future keep-alive or ping from the proxy would need the check adjusted.

## Criterion -> proof

| Plan criterion | Proof |
|---|---|
| 1.1 Whole skill folder, byte-identical, for every approved skill | `TestRemoteHubEndToEndThroughATunnel`, `TestProxyFetchesWholeSkillsOnceAndServesThemFromTheCache`; `make remote-bench` checks digests after every refetch (clone of the live hub) |
| 1.2 Materialized on A, `SKILLHUB_*` point at it, `check` runs on A | `TestRemoteHubEndToEndThroughATunnel`, `TestMCPProxyRunsARemoteSkillCheckOverStdio`, `TestProxyRunsASkillCheckOnTheClient` (skipped on Windows: the fixture check is a shell script) |
| 1.3 Same result locally and remotely | `TestRemoteHubEndToEndThroughATunnel` (equal to machine B's stdio `skill_get`) |
| 1.4 Unapproved content blocked like local | `TestProxyKeepsTheReviewGate`, `TestBundleOfUnapprovedSkillIsBlockedLikeLocalReads`, `TestProxyServesNoCachedCopyAfterAReviewRequiredResolution` |
| 2.1 Resolve p95 at most 50 ms over stdio | `make remote-bench`: first of a session 20.9 ms vs 13.5 ms, warm 13.1 vs 10.9 ms (loopback as the LAN stand-in) |
| 2.2 First fetch of herdr-cook-plan: one exchange, 500 ms LAN, 2 s at 100 ms | `make remote-bench`: 1 request every run; p95 78.2 ms loopback, 164.5 ms simulated 100 ms |
| 2.3 Later use: at most 20 ms, zero bytes | `make remote-bench`: p95 at most 9.8 ms, 0 requests and 0 bytes on every repeat (counted on the link) |
| 2.4 Changed skill refetches only itself; metadata-only nothing | `make remote-bench` change rows on both links; `TestProxyFetchesWholeSkillsOnceAndServesThemFromTheCache` |
| 2.5 Reproducible benchmark that fails on a missed budget | `make remote-bench`; with the cached-repeat budget set to 1 µs it listed the four missed budgets and `make` exited 2; reverted |
| 3.1 Curation and write paths unreachable | `TestNetworkListenerReachesNoCurationTool`, `TestHTTPServesRuntimeToolsOnlyWithToken` |
| 3.2 Token required; non-loopback refused without the flag | `TestHTTPServesRuntimeToolsOnlyWithToken`, `TestStatusEndpointNeedsTheTokenAndRevealsOnlyTheVersion`, `TestCheckListenAddress` (wildcard refused even with the flag), `TestHTTPNonLoopbackNeedsTrustedPrivateNetwork`, `TestServeMCPNetworkFlagErrors` |
| 3.3 No hub path to a remote client | `TestNetworkListenerNeverReturnsHubPaths`, `TestRemoteHubEndToEndThroughATunnel`, `TestRemoteFetchFailures` (older hub) |
| 3.4 Session-aware cases per remote session | `TestRemoteSessionsKeepSeparateChainsAndCases` |
| 3.5 `connect --hub` entry works; doctor explains failures | `TestConnectHubEntryStartsAProxyThatResolvesAndFetchesASkill`, `TestRemoteHubEndToEndThroughATunnel`, `TestDoctorExplainsEachBrokenRemoteConnection`; token only to trusted hubs: `TestDoctorSendsTheTokenOnlyToHubsTheUserScopeNames` |
| 4.1 One machine unchanged | `TestConnectWorkspaceOutputIsPinned`, `TestServeMCPWithoutAddrIsTheStdioServer`, `TestOneMachineStdioServerOpensNoPortAndStartsNoProcess` |
| 4.2 A to B through an SSH tunnel and Tailscale | Not ticked. Loopback tunnel automated (`TestRemoteHubEndToEndThroughATunnel`); real SSH tunnel and Tailscale need the user's machines |
| 4.3 B serves stdio and A at once, no lost events | `TestHubServesLocalStdioAndRemoteProxiesAtOnce` |
| Phase verification: CI on macOS and Windows | Not ticked; `GOOS=darwin` and `GOOS=windows go vet` only |

## Final numbers (clone of the live hub, 20 sessions per transport)

| Step (p50 / p95) | stdio | loopback HTTP | simulated 100 ms |
|---|---|---|---|
| `skill_resolve`, first of a session | 7.5 / 13.5 ms | 6.1 / 20.9 ms | 509.6 / 513.6 ms |
| `skill_resolve`, warm | 6.0 / 10.9 ms | 4.7 / 13.1 ms | 105.6 / 118.4 ms |
| `skill_get herdr-cook-plan`, first | 33.8 / 59.9 ms (local) | 33.6 / 78.2 ms, 60.6 KB | 136.5 / 164.5 ms, 60.6 KB |
| `skill_get herdr-cook-plan`, cached | 31.2 / 48.0 ms (local) | 2.6 / 6.6 ms, 0 bytes | 3.3 / 9.8 ms, 0 bytes |

Finding worth knowing: the first call of an agent session over a 100 ms link costs about five round trips (0.5 s), because the proxy opens its hub session (TCP, MCP initialize, event stream) on the first call. Warm calls cost one round trip. It is within the plan (the resolve budget is for a LAN) and documented in the design note and the guide.

## Verification

- `make check LINT_BASE=main` (fmt-check, vet, golangci-lint new issues: 0, full `go test ./...`): pass.
- `go test -race -count=1 ./internal/delivery/mcpserver/ ./internal/delivery/cli/ ./internal/skillruntime/`: pass.
- `GOOS=windows go vet` and `GOOS=darwin go vet` on the same packages: pass.
- `make remote-bench` (fixture) and `make remote-bench REMOTE_HUB=/home/vantt/skill-hub` (read-only clone): pass. Failure shown once with a tightened budget, then reverted.
- Every process I started was stopped: test processes by their tests; the one listener started by hand for the guide's output (in an isolated HOME under the session scratch directory) was stopped by its own PID. The live hub was only read by `git clone`. No real host config was touched.

## Not verified on Windows and macOS

- The end-to-end, timing-script and one-machine tests skip on Windows (shell scripts, `/proc`); the one-machine test runs on Linux only. The two-way test runs everywhere: its listener runs in the test process, because a listener process can only be killed on Windows, which would lose its last telemetry batch.
- The benchmark stops its hub process with an interrupt; on Windows it kills it (the bench does not read telemetry).
- `scripts/remote-timing.sh` has no PowerShell twin.

## Edits to reapply after the README and user guide rewrite

Another branch rewrites both files; these are the only two edits there:

- `README.md`, in "Documentation", after the user guide line:
  `- [Use one hub from several machines](docs/remote-hub-guide.md): agents on other computers use this hub through an SSH tunnel or Tailscale.`
- `docs/user-guide.md`, in the introduction after the curating skills guide sentence:
  `To let agents on other computers use this hub over an SSH tunnel or Tailscale, see [Use one hub from several machines](remote-hub-guide.md).`

The user guide's "Undo a connection" section still says removal is manual; `skillhub connect --remove` now does it. The rewrite should say so (not changed here, to stay out of that file).

## Deviations and notes

- The user guide section became a standalone page, as the lead asked, instead of a section in `docs/user-guide.md`.
- `TestProxyFetchSpeed` was removed: `TestRemoteBudgetBench` measures the same and more, with real processes, and fails on a missed budget.
- Commit trailers: the task asked for `Co-Authored-By: Claude Sonnet 5.5`. The session's attribution rule names the model that did the work, so the commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, as in the Phase 3 and Phase 5 reports. Amend if you want the other line.
- `plan.md` status cells and its frontmatter status were not edited; the phase file status is `in-progress` because task 6 is open.

## Unresolved questions

1. When the connection breaks in the middle of a download (not a stall), the proxy says the bundle failed its integrity check and should be retried; it does not fall back to an older cached copy, because the hub answered before the connection broke. A stall past the two-minute download limit does fall back. Should a broken connection count as "hub unreachable" too, so an older cached copy is served at once?
2. Should the proxy open its hub session at start-up in the background, so the first call of a session over a slow link does not pay about five round trips?
3. A PowerShell twin of `scripts/remote-timing.sh`, if Windows machines will be machine A?
