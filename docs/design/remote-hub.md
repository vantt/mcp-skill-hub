# Remote hub: threat model, decisions and the stdio baseline

Status: decisions settled and baseline measured on 2026-10-11, before any network code exists.
Plan: [plans/261010-1652-remote-hub-mcp-over-network/plan.md](../../plans/261010-1652-remote-hub-mcp-over-network/plan.md).

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
| Q4 | Task text sent over the network | The hub refuses to listen on any non-loopback address unless `--trust-private-network` is given; plain HTTP on loopback is allowed (SSH tunnel, tests) |
| Q5 | Telemetry from remote clients | Written on the hub with a client label; the case journal stays opt-in on the hub |

Go or stop: go. The user wants one machine to curate and serve the skills to the others, which the
Git route (clone and `skillhub rebuild` on every machine) does not do live.

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

`--workspace` and `--hub` exclude each other. The token comes from `--token-file` or the environment
variable `SKILLHUB_HUB_TOKEN`, and is never written into a host config file. The listener prints the
exact `connect --hub` line to run on A when it starts, with the token file path, never the token
itself. Network mode always serves the runtime profile only. The stdio proxy
`skillhub mcp proxy --hub <url>` is an internal entry that `connect --hub` writes; users do not type it.

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
and routing metadata, plus writing feedback and resolve telemetry into B's telemetry store. It does
not allow any curation tool, any canonical write, approving content, reading unapproved content, or
running anything on B. Rotation (`skillhub token rotate`) revokes it. Mitigations: long random
token, constant-time compare, rate limit, never in URLs, logs or host config files.

**What travels over the connection.**

- Task text in `skill_resolve` (it can describe private work, so it only travels over loopback, an
  SSH tunnel or a private VPN).
- Approved skill content: `SKILL.md`, references, scripts and other files, with their digests.
- Routing metadata, the runtime spec (requirements, setup and check commands) and resolve results.
- Feedback and client labels for telemetry.

**What must never travel.**

- Curation tools (skill create, edit, activate, approve, source import, workspace mutations) and the
  case journal CLI. Network mode hard-wires the runtime profile.
- Any write path into the canonical files or the Git repository.
- Secrets from `$SKILLHUB_CONFIG_DIR/env` or any skill's stored environment values; the client
  keeps its own.
- Paths on the hub's disk, such as `local.path`, `local.state_directory` and the
  `io.skillhub/local_path` resource meta that local stdio clients get today.
- The bearer token itself, in any response, log line or printed command.
- Content of skills awaiting review: blocked for remote callers exactly as for local ones.

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
