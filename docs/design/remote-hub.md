# Remote hub: threat model, decisions, baseline and final numbers

> Superseded in part by the onboarding redesign of 2026-10: `skillhub serve` binds `0.0.0.0` by default, `--trust-private-network` is gone, and clients connect with `skillhub connect --hub <url> --token-file <file>`; see [docs/remote-hub-guide.md](../remote-hub-guide.md).

Status: implemented. Decisions settled and the stdio baseline measured on 2026-10-11 before any
network code existed; the final command syntax and the measured numbers of the finished feature are
below. The user-facing guide is [docs/remote-hub-guide.md](../remote-hub-guide.md).
Plan: [archive/plans/261010-1652-remote-hub-mcp-over-network/plan.md](../../archive/plans/261010-1652-remote-hub-mcp-over-network/plan.md).

One user has several machines. Machine B curates the hub and serves it; machine A only uses the
skills. An agent on A calls the three runtime tools (`skill_resolve`, `skill_get`, `skill_feedback`)
against B over MCP, then runs the skill on A. Curation never leaves B.

## Two ways of working, both must keep working

1. **One machine (today, unchanged).** The host starts `skillhub mcp serve` over stdio. No network
   port, no token, no extra process, and the same registrations `skillhub connect --workspace` writes
   today.
2. **Machine A uses machine B (new).** B runs a network listener over the same workspace; A's host
   starts a small stdio proxy that talks to it. B keeps serving its own agents over stdio at the same
   time, because the listener is a separate process over the same workspace.

Each host scope (project or global) points at one hub: `connect --workspace` for a local one or
`connect --hub` for a remote one. Switching a scope between the two replaces its entry.

## Decisions (answered by the user on 2026-10-11)

| # | Question | Decision |
|---|---|---|
| Q1 | Who connects | One user with several machines. One machine curates and serves the skills; the others only use them. No accounts: a single shared bearer token |
| Q2 | How machine A reaches machine B | Both an SSH tunnel and Tailscale (or another private VPN) are supported. No TLS in this plan and no exposure to the open internet. The hub listens on loopback by default (SSH tunnel). For Tailscale the user binds it to the Tailscale address and confirms it with an explicit flag (`--trust-private-network`) |
| Q3 | Skills with scripts, setup or check commands | Run only on the client, after the client pulls the files; never on the hub for a remote caller |
| Q4 | Task text sent over the network | The hub refuses to listen on any non-loopback address unless `--trust-private-network` is given; plain HTTP on loopback is allowed (SSH tunnel, tests). Even with the flag an address that binds every interface (`0.0.0.0`, `[::]`) is refused: give the address of the private interface, for example the Tailscale address |
| Q5 | Telemetry from remote clients | Written on the hub with a client label; the case journal stays opt-in on the hub |

Go or stop: go. The user wants one machine to curate and serve the skills to the others, which the
Git route (clone and `skillhub rebuild` on every machine) does not do live.

## Command syntax (as built)

Hub machine B:

```
skillhub serve web                                   # web UI, as before
skillhub serve mcp                                   # MCP over stdio, what hosts launch (`mcp serve` stays as an alias)
skillhub serve mcp --addr 127.0.0.1:7420             # network listener on loopback: reach it through an SSH tunnel
skillhub serve mcp --addr <tailscale-ip>:7420 --trust-private-network [--allow-host <name>]
skillhub token show                                  # prints the token, on a terminal only
skillhub token rotate [--yes]                        # preview, then replace the token
```

The token lives in `<workspace>/runtime/config/.hub-token` (mode 0600, under the untracked
`runtime/` directory; `--token-file` overrides it), not in a skill's `$SKILLHUB_CONFIG_DIR`.
`--allow-host` accepts a further Host name, for example a full Tailscale MagicDNS name.

Machine A:

```
skillhub connect --workspace ~/skillhub --yes                              # hub on this machine (as before)
skillhub connect --hub http://127.0.0.1:7420 --token-file <path> --yes     # via an SSH tunnel
skillhub connect --hub http://<tailscale-ip>:7420 --token-file <path> --yes
skillhub connect --remove [--cache] --yes            # remove the entry of this scope (either kind); --cache also clears fetched skills
skillhub cache clear --hub [<url>] [--yes]           # clear skills fetched from remote hubs
skillhub doctor                                      # also checks every remote entry of this project and the user scope
```

`--workspace` and `--hub` exclude each other. The token comes from `--token-file` or the environment
variable `SKILLHUB_HUB_TOKEN`, and is never written into a host config file. The listener prints the
exact `connect --hub` line to run on A when it starts, with the token file path, never the token
itself. Network mode always serves the runtime profile only. The stdio proxy
`skillhub mcp proxy --hub <url> [--token-file <path>]` is the entry that `connect --hub` writes; users
do not type it. On the wire the listener answers `/mcp` (MCP Streamable HTTP), `GET /mcp/bundle` (a
whole approved skill as one digest-addressed archive, with `ETag` and `304`) and `GET /mcp/status`
(version and remote protocol level), all behind the same bearer token, and `POST /mcp/pair`, which takes
a one-time invite instead of the bearer token and answers with the hub token. The pair route is off unless
the server was given an invite store. By default it answers any connection the listener accepts, so a hub
at home or in an office pairs with no extra setup, at the price that the code and the token travel in plain
HTTP. With `--strict-pairing` it answers only when both the local and the remote address of the
connection are loopback or in `100.64.0.0/10` and the request has no forwarding header; otherwise it
answers 403 without looking at the invite, and the client refuses to send an invite to any other address. An invite is a 12 character code kept on the hub as an empty
file named by its SHA-256, whose modification time is its expiry; it is claimed by creating a `.used` marker
with exclusive creation, which exactly one request can do on every system, and it expires after 10 minutes. Failures are counted per remote address apart from
bearer failures, every failure answers the same 403, and the hub token is read before the invite is claimed
so a token problem never wastes an invite.

## Threat model

**What the listener protects.** Machine B holds the canonical skills (a Git repository), the runtime
databases (catalog, telemetry, optional case journal), per-skill secrets in
`$SKILLHUB_CONFIG_DIR/env` (mode 0600), and the curation tools that change all of these. Only the
read side of the runtime profile is meant to be reachable from another machine.

**Who can reach it.**

- Default bind `127.0.0.1`: any process on machine B, and anyone who can open an SSH tunnel to B
  (which already means they can log in to B). Other local users on B can connect to a loopback port,
  so the bearer token is still required on loopback.
- `--addr <tailscale-ip> --trust-private-network`: machines on the user's private network (tailnet).
  Traffic is encrypted by WireGuard, not by the hub. The flag is the user's explicit statement that
  every machine on that network is theirs.
- Never: the open internet. The listener refuses a non-loopback address without the flag, and this
  plan ships no TLS, so the guide never steers to plain HTTP over a shared network.

**What a stolen token allows.** The same as an agent on machine A: the three runtime tools, the
skills listing and the pinned resource reads. That is read-only access to approved skill content
and routing metadata, plus writing feedback and resolve telemetry into B's telemetry store. It can
also fill the submission queue (`POST /mcp/submissions`): at most 50 waiting submissions and
256 MiB in total, each at most 512 files and 16 MiB, refused beyond that. A submission only waits in
`runtime/submissions/` on B, which Git does not track; it changes no skill and nothing is served from
it until a person on B reviews it and approves it as third-party content. It does not allow any
curation tool, any canonical write, approving content, reading unapproved content, or running
anything on B. Rotation (`skillhub token rotate`) revokes it. Mitigations: long random token,
constant-time compare, rate limit, the queue caps, never in URLs, logs or host config files. The
client label that comes with a submission is shown as "submitted from" text and is never used for
trust. A per-invite permission to submit stays deferred.

**What travels over the connection.**

- Task text in `skill_resolve` (it can describe private work, so it only travels over loopback, an
  SSH tunnel or a private VPN).
- Approved skill content: `SKILL.md`, references, scripts and other files, with their digests.
- Routing metadata, the runtime spec (requirements, setup and check commands) and resolve results.
- Feedback and client labels for telemetry.
- From A to B only, a skill A sends for review (`skillhub skill commit` in a project connected to
  B): its files, the version it was checked out from, and a label for the machine. B queues it.

**What must never travel.**

- Curation tools (skill create, edit, activate, approve, source import, workspace mutations) and the
  case journal CLI. Network mode hard-wires the runtime profile.
- Any write path into the canonical files or the Git repository. A submission is not one: it waits
  in an untracked queue and reaches the repository only when a person on B approves it there.
- Secrets from `$SKILLHUB_CONFIG_DIR/env` or any skill's stored environment values; the client
  keeps its own.
- Paths on the hub's disk, such as `local.path`, `local.state_directory` and the
  `io.skillhub/local_path` resource meta that local stdio clients get today.
- The bearer token itself, in any response, log line or printed command.
- Content of skills awaiting review: blocked for remote callers exactly as for local ones. That
  includes the submissions waiting in the queue, which are not skills and are served to no one.

**Where scripts run.** Only on machine A, after A has the files (Q3). The hub never runs a skill's
check, setup or scripts for a remote caller.

## Measured today: one stdio process per agent session

Copied from the plan (measured 2026-10-11 on Linux, throwaway workspace, isolated `HOME` and
`XDG_*`, 20 processes each calling `skill_resolve` for 30 seconds on a nearly empty catalog).

| Scenario | Calls | Events written / expected | Lost | Total RSS |
|---|---|---|---|---|
| 1 call per second per process | 600 | 1200 / 1200 | 0 | 650 MB |
| 10 calls per second per process (about 190 per second in total) | 5,792 | 11,584 / 11,584 | 0 | 755 MB |
| Back-to-back calls (about 5,300 per second in total) | 160,207 | 56,197 / 320,414 | 264,217 dropped, 27 storage errors | 836 MB |

- Each `skill_resolve` writes 2 telemetry events.
- At 10 calls per second per process, 0 to 6 of 12,000 events are dropped on some runs; the cause
  was every telemetry batch reopening SQLite with `PRAGMA quick_check`. The writer has since been
  made safe for several processes.
- Total RSS counts about 18 MB of shared binary pages in every process. Measured as PSS, one session
  costs about 14 to 15 MB idle and 17 to 18 MB working, 20 sessions about 0.29 to 0.36 GB.
- Startup is about 20 ms per process; warm `skill_resolve` takes about 3 ms at the median and 9 ms at
  p95 over stdio. All 20 processes exit with code 0 when stdin closes.

## Baseline: fetching a whole skill over stdio today

**What a remote client must do today.** It cannot read `local.path` on the hub's disk, so it must
pull every file over MCP: `skill_resolve`, `skill_get` (returns `SKILL.md` content and the file
list but no fetchable URIs), `skills/list` (one page per 25 skills, to find the version-pinned
`SKILL.md` URI), `skills/get` (the resource URIs and digests), then one `resources/read` per file.
Every read is checked against the announced digest and size, so the numbers describe a complete,
byte-identical fetch.

**Command.**

```sh
make remote-baseline REMOTE_HUB=/home/vantt/skill-hub
# same as:
SKILLHUB_PERF=1 SKILLHUB_REMOTE_HUB=/home/vantt/skill-hub \
  go test -count=1 -run '^TestRemoteFetchBench$' -v ./internal/delivery/mcpserver/
```

The test (`internal/delivery/mcpserver/remote_bench_test.go`) clones the hub with
`git clone --no-hardlinks` into a temp directory, runs `skillhub rebuild` there, and approves the
two skills' content in the clone only (both await review in a fresh clone). `HOME` and `XDG_*` are
isolated by the package's `TestMain`. The live hub is only read. Without `REMOTE_HUB` it uses
fixture skills with the same file count and sizes. Each of the 30 runs is a new `skillhub mcp serve
--profile runtime` process and a new client, because the Go SDK client caches `resources/read`
results for their TTL.

Machine: Linux, 24 cores, load average about 2.4 to 3.0 from the user's other work during the runs
(the user's real `skillhub mcp serve` processes kept running). Clone of hub commit `9c704c7`.

**Shape.** `skills/get` serves the files outside `.meta/`, so the hub's metadata (`.meta/skill.yaml`,
and `.meta/distill.yaml` for `test-audit`) is not part of the fetch.

| Skill | Files in folder | Served files | Content | Round trips, resolve to last file | Round trips for the folder |
|---|---|---|---|---|---|
| `test-audit` | 4 | 2 | 13,217 bytes | 6 | 5 |
| `herdr-cook-plan` | 14 | 13 | 186,343 bytes | 17 | 16 |

**Timings, two runs** (p50 / p95 over the first fetch in each of 29 later sessions; "first" is the
first fetch ever, when the hub exports the skill snapshot; KB are JSON-RPC response bytes).

| Skill | Step | Run 1 first | Run 1 p50 / p95 | Run 2 first | Run 2 p50 / p95 | Response KB |
|---|---|---|---|---|---|---|
| `test-audit` | `skill_resolve` | 28.7 ms | 12.6 / 22.2 ms | 25.0 ms | 12.5 / 22.9 ms | 2.5 |
| `test-audit` | `skill_get` | 119.1 ms | 58.7 / 88.2 ms | 53.2 ms | 57.5 / 88.7 ms | 20.4 |
| `test-audit` | `skills/list` | 41.8 ms | 9.6 / 22.1 ms | 18.2 ms | 9.9 / 24.2 ms | 6.1 |
| `test-audit` | `skills/get` | 35.0 ms | 8.7 / 21.1 ms | 13.6 ms | 8.4 / 26.9 ms | 1.9 |
| `test-audit` | `resources/read` x2 | 43.9 ms | 18.7 / 45.0 ms | 23.1 ms | 19.0 / 50.7 ms | 14.2 |
| `test-audit` | **folder** (5 calls) | 239.8 ms | **98.8 / 170.8 ms** | 108.1 ms | **95.2 / 162.0 ms** | 42.6 |
| `herdr-cook-plan` | `skill_resolve` | 10.4 ms | 5.8 / 14.2 ms | 18.9 ms | 5.8 / 16.3 ms | 2.9 |
| `herdr-cook-plan` | `skill_get` | 69.8 ms | 44.4 / 80.3 ms | 70.1 ms | 55.6 / 80.8 ms | 54.9 |
| `herdr-cook-plan` | `skills/list` | 36.9 ms | 10.6 / 22.1 ms | 25.8 ms | 11.6 / 20.8 ms | 6.1 |
| `herdr-cook-plan` | `skills/get` | 27.6 ms | 10.4 / 19.4 ms | 25.0 ms | 10.5 / 20.8 ms | 7.2 |
| `herdr-cook-plan` | `resources/read` x13 | 397.2 ms | 189.0 / 250.1 ms | 260.3 ms | 189.6 / 304.0 ms | 206.7 |
| `herdr-cook-plan` | **folder** (16 calls) | 531.5 ms | **247.9 / 335.5 ms** | 381.2 ms | **265.7 / 391.9 ms** | 274.8 |

Process start and initialize: p50 48 ms, p95 69 to 75 ms. `skill_resolve` chose the measured skill
as primary in 30 of 30 fetches in both runs. A fixture run (no `REMOTE_HUB`) moved the same bytes
with folder p50 93 ms and 279 ms.

**What the baseline says.**

- **Round trips dominate on a real link.** The `herdr-cook-plan` folder needs 16 sequential round
  trips. At 100 ms RTT they alone add 1.6 s, so today's fetch would take about 1.85 s on such a link
  before any bandwidth cost, already at the 2 s budget. One exchange for the whole folder removes
  15 of them.
- **Each file read costs about 15 ms on the hub, even on stdio.** Every `resources/read` opens the
  catalog generation, rebuilds the skill's distributed manifest and checks the local snapshot
  (`internal/app/distribution.go` `ReadResource`, `internal/delivery/mcpserver/server.go`
  `localResourceMeta`). A bundle should be produced in one pass over the snapshot, not by looping the
  per-file path.
- **`SKILL.md` travels twice** (inside `skill_get` and again as a resource): the folder moves 275 KB
  of JSON for 182 KB of content, about 1.5 times.
- **A second fetch in the same session is not free today.** With the Go SDK's resource cache the
  repeat still costs 61 to 78 ms (p50) and up to 68 KB, because `skill_get`, `skills/list` and
  `skills/get` are not cached, and hosts that are not Go SDK clients may cache nothing.
- **Warm resolve vs first resolve.** The first `skill_resolve` of a session takes 6 to 13 ms (p50) and
  14 to 23 ms (p95); warm calls in a long session take about 3 / 9 ms (fan-out measurement above).

## Budgets

The budgets in the plan's "Success Criteria" part 2 were checked against this baseline on
2026-10-11. The three numeric budgets are kept and the yes/no budget is unchanged:

| Budget | Today on stdio | Verdict |
|---|---|---|
| Remote `skill_resolve` p95 at most 50 ms above local stdio | local p95 14 to 23 ms, so at most about 75 ms | Confirmed |
| First fetch of `herdr-cook-plan`: one exchange, 500 ms on a LAN, 2 s at 100 ms RTT | 16 round trips; 248 to 266 ms p50 and 336 to 392 ms p95 on stdio; projected 1.85 s at 100 ms RTT | Confirmed: one exchange plus the measured hub time fits both, with room for TCP slow start over a tunnel |
| Later use of the same version: at most 20 ms, zero bytes of skill content | 61 to 78 ms and up to 68 KB per repeat today | Confirmed: a digest-keyed local cache needs no exchange |
| Changed skill refetches only itself; metadata-only change refetches nothing | not measurable today (no client cache) | Yes/no test, unchanged |

## Final numbers

Measured with `make remote-bench` (`scripts/remote-bench.sh`, test `TestRemoteBudgetBench` in
`internal/delivery/mcpserver/remote_budget_bench_test.go`) on 2026-10-11: Linux, 24 cores, other work
running on the machine; a read-only clone of the live hub (`make remote-bench REMOTE_HUB=<hub repository>`,
the two skills approved in the clone only); 20 agent sessions per transport. The hub
(`skillhub serve mcp --addr 127.0.0.1:0`) and every proxy (`skillhub mcp proxy`) are real processes.
The simulated link is a loopback relay that delays every chunk by half the round trip in each
direction and the first chunk of a connection by one more round trip; it does not limit bandwidth.
The relay also counts the HTTP requests and the bytes between proxy and hub. Each remote session
starts with an empty cache. The benchmark fails when a budget is missed.

| Step (p50 / p95) | stdio (local hub) | loopback HTTP | simulated 100 ms link |
|---|---|---|---|
| Session start (process and initialize) | 23.9 / 31.6 ms | 17.3 / 37.1 ms | 16.0 / 32.6 ms |
| `skill_resolve`, first of a session | 7.5 / 13.5 ms | 6.1 / 20.9 ms | 509.6 / 513.6 ms |
| `skill_resolve`, warm | 6.0 / 10.9 ms | 4.7 / 13.1 ms | 105.6 / 118.4 ms |
| `skill_get test-audit`, first | 29.2 / 45.9 ms (local snapshot) | 25.1 / 48.3 ms; 1 request, 6.7 KB | 125.2 / 133.4 ms; 1 request, 6.7 KB |
| `skill_get test-audit`, repeat | 29.6 / 65.9 ms (local snapshot) | 1.1 / 1.8 ms; 0 requests, 0 bytes | 1.3 / 2.5 ms; 0 requests, 0 bytes |
| `skill_get herdr-cook-plan`, first | 33.8 / 59.9 ms (local snapshot) | 33.6 / 78.2 ms; 1 request, 60.6 KB | 136.5 / 164.5 ms; 1 request, 60.6 KB |
| `skill_get herdr-cook-plan`, repeat | 31.2 / 48.0 ms (local snapshot) | 2.6 / 6.6 ms; 0 requests, 0 bytes | 3.3 / 9.8 ms; 0 requests, 0 bytes |

Changes on the hub, on both links: after a routing-only change to every skill, each `skill_get` after
a fresh resolution was a cache hit with 0 requests and 0 bytes; after a content change to
`herdr-cook-plan`, only that skill was fetched again (1 request, about 62 KB) and `test-audit` stayed a
hit with 0 requests. These numbers predate counting a load per resolution (see
[Telemetry](04-telemetry-reproducibility-evaluation.md)). The hub now counts a load once for each
version, hub session and resolution. So a cache hit under a resolution the hub has not counted yet
still answers from the cache at once. The proxy then sends one conditional `GET /mcp/bundle` in the
background, and the hub answers it 304 with no body. On the fixture (2026-10-15) each such hit costs
the agent 0 requests and 0 bytes. The background request is 1 request of about 550 bytes in both
directions. A cached repeat under the same resolution still costs no request at all.

The benchmark measures the hub-change rows in two parts, by following each HTTP exchange through the
relay:

- **Waited.** The requests whose answer reached the proxy before `skill_get` returned. While a hit is
  expected, the relay holds back everything the hub sends until the call returns. A hit that needs the
  hub cannot return then, so it shows up as a waited request and is not mistaken for background
  traffic.
- **Background.** Every other request.

Before each change on the hub and before each measurement, the bench waits until the relay has
delivered every answer. So a background request never races a change on the hub. The proxy is a
separate process and gives no signal when its background request is done, so the bench reads the
link instead. It waits until the relay has seen the proxy's request and delivered its answer. The
wait gives up after 2 s only when a hit sends no background request, which the budget allows.

| Budget | Measured | Verdict |
|---|---|---|
| Remote `skill_resolve` p95 at most 50 ms above local stdio (LAN) | first of a session 20.9 ms vs 13.5 ms; warm 13.1 ms vs 10.9 ms | Met |
| First fetch of `herdr-cook-plan`: one exchange, 500 ms on a LAN, 2 s at 100 ms RTT | 1 request on every run; p95 78.2 ms on loopback, 164.5 ms at 100 ms RTT | Met |
| Later use of the same version: at most 20 ms, zero bytes of skill content | p95 at most 9.8 ms; 0 requests and 0 bytes on every repeat | Met |
| Changed skill refetches only itself; metadata-only change refetches nothing | as described above. Checked as: a changed skill is fetched in 1 request the agent waits for, with nothing in the background. Every other skill is a hit that waits for 0 requests and 0 bytes, and costs at most 1 background conditional request answered 304 with no body | Met |

A fixture run (`make remote-bench` without `REMOTE_HUB`: skills of the same file count and sizes but
more compressible text, so 7.3 KB on the wire for `herdr-cook-plan`) meets every budget as well.

What the numbers say:

- A whole skill now costs one round trip plus the transfer: 0.16 s at 100 ms RTT for
  `herdr-cook-plan`, against 16 sequential round trips (a projected 1.85 s) in the stdio baseline. A
  real link adds TCP slow start for the 60 KB body, about two more round trips.
- A repeat use costs no request at all, and is faster than a local `skill_get` (about 30 ms), which
  reads the catalog and prepares the local snapshot answer on every call.
- The first call of each agent session over a slow link pays about five round trips (0.5 s at
  100 ms): the TCP handshake, the MCP initialize exchange and the session's event stream, because the
  proxy opens its hub session on the first call. Warm calls pay one round trip. The resolve budget is
  set for a LAN, where this cost is a few milliseconds. The proxy does not open the session eagerly
  when it starts: the hub labels a session `remote:<host>` from the host name the first call carries
  (a host on the newer protocol sends it only then), and an eager session would also put a hub
  session and a token check behind every agent start, used or not. The cost is
  accepted; measure it with `scripts/remote-timing.sh`.
- A real 100 ms link and a Tailscale address were not measured here; the user times them on the real
  machines with `scripts/remote-timing.sh` (see the guide).

Tests that cover the other criteria, all in `go test ./...`:

- `TestRemoteHubEndToEndThroughATunnel`: listener and proxy as real processes, a tunnel-like relay on
  a second port, separate homes for both machines. Every skill arrives byte-identical, matches
  machine B's own stdio answer (content, version, skill detail, runtime spec), runs its `check` on
  machine A, and `skill_feedback` is recorded on the hub under `remote:claude-code`. The token is in
  no output and in no file except the two token files.
- `TestRemoteFetchFailures`: a download cut off midway, a truncated archive, a read-only cache, and a
  hub from before bundles; with the earlier proxy tests (wrong token, review gate, tampered archive,
  hub error, stopped hub) and doctor's version check.
- `TestNetworkListenerReachesNoCurationTool`: every tool of the full server is an unknown tool through
  the listener and through the proxy, and web UI paths answer 404.
- `TestOneMachineStdioServerOpensNoPortAndStartsNoProcess` and `TestConnectWorkspaceOutputIsPinned`:
  the one-machine path is unchanged.
- `TestHubServesLocalStdioAndRemoteProxiesAtOnce`: three stdio servers and two proxies resolve at the
  same time on one workspace, and every telemetry event is written.
