# Cook report: remote hub Phase 2 (HTTP transport, runtime profile only)

Date: 2026-10-11 (Asia/Saigon). Plan: `plans/261010-1652-remote-hub-mcp-over-network/` (decisions Q1 to Q5 as answered in the primary checkout's `plan.md`). Status: completed.

## Outcome

`skillhub serve mcp --addr <host:port>` now serves the three runtime tools (`skill_resolve`, `skill_get`, `skill_feedback`) over Streamable HTTP behind a shared bearer token. No curation tool is registered on that server. `skillhub serve mcp` without `--addr`, and the alias `skillhub mcp serve`, run the same stdio server as before; `mcp serve` keeps exactly its old flags. A subprocess regression test pins this.

## What was built

| Area | Where | Notes |
|---|---|---|
| Shared startup | `internal/delivery/mcpserver/server.go` (`startServing`, `useRecorder`) | Catalog ensure, tool registration, idle memory releaser, snapshot GC and the telemetry recorder are now built once for either transport. `Serve` (stdio) is a thin wrapper. All HTTP sessions share one `*mcp.Server`, so they share the tools, the activation tracker (keyed by `*mcp.ServerSession`) and the recorder. |
| Network listener | `internal/delivery/mcpserver/http.go` | `ServeHTTP(ctx, listener, HTTPOptions)`, `CheckListenAddress`, endpoint `/mcp`. Request gate order: Host, Origin, path, shutting-down, failed-auth throttle, bearer token, rate limit, session cap, then the SDK handler with a 4 MiB body cap (`MaxFrameBytes`). Shutdown refuses new requests, lets running requests finish (up to 3 s), closes every session (which ends their event streams), stops the HTTP server with a background-context deadline, then flushes and closes the recorder. |
| Token storage | `internal/hubtoken/` (new, small) | `<workspace>/runtime/config/.hub-token`, 32 random bytes hex, created with `O_EXCL` and mode 0600 in a 0700 directory; refuses symlinks, wrong content, and group/other permission bits (not checked on Windows, which has no such bits). `Rotate` writes a temp file and renames it. `Source` rereads the file only when it changes, so a rotation takes effect at the next request and the old token stops working at once. Constant-time comparison. |
| CLI | `internal/delivery/cli/mcp.go`, `token.go`, `serve.go`, `root.go`, `help.go` | `serve mcp` routing, one flag parser for both spellings, `--profile curation|all` with `--addr` is an error, network-only flags without `--addr` are an error, `skillhub token show|rotate`. All output goes through `termui`. |

Limits chosen (constants in `http.go`): 128 open sessions (503 with `Retry-After` beyond), 1 hour idle session timeout, 100 requests per second with a burst of 200 for the shared token, 20 failed authentications per minute per remote address (429 for further requests without a valid token), 10 s header timeout, keep-alive on with a 2 minute idle connection timeout, 64 KiB headers.

Host rule: accepted names are `localhost`, `127.0.0.1`, `::1`, the listen IP (every interface IP for a wildcard address), this machine's host name and `<hostname>.local`, plus any `--allow-host` name. The port in the Host header is ignored on purpose, because an SSH tunnel may forward a different local port. An `Origin` header, when present, must name one of the same hosts. The SDK's own loopback DNS-rebinding check is disabled because this gate is stricter and the SDK check would refuse `--allow-host` names.

Start-up banner (stdout, via `termui`): listen address, workspace, token file path, transport, an `ssh -N -L` tunnel line for loopback, and the exact `skillhub connect --hub <url> --token-file <path> --yes` line. With `--trust-private-network` one `WARNING:` line says the plain-HTTP transport is trusted to be a private network. The token value is never printed.

## Acceptance

| Criterion | Evidence |
|---|---|
| 401 with no or a wrong token (also Basic scheme and token in the query string) | `TestHTTPServesRuntimeToolsOnlyWithToken` |
| 403 on a bad Host or Origin (also `Origin: null`) | same test, `TestHTTPGateHostNames` |
| tools/list over HTTP is exactly the three runtime tools | same test |
| A curation tool name returns "unknown tool" | same test (`workspace_rebuild`, `skill_create_confirm`, `source_import_confirm`, `curation_session_record`) |
| Non-loopback refused without `--trust-private-network`, starts with it | `TestCheckListenAddress`, `TestHTTPNonLoopbackNeedsTrustedPrivateNetwork` (fake listener address, real loopback socket, so no reachable port is opened), CLI golden error in `TestServeMCPNetworkFlagErrors` |
| `serve mcp` without `--addr` and `mcp serve` behave as before | `TestServeMCPWithoutAddrIsTheStdioServer` (real subprocess over stdio: same tool list for both spellings, runtime and default profiles, exit code 0 on stdin close, no token file created), `TestServeMCPAndMCPServeShareFlagErrors`, existing `TestMCPServeFlags` |
| Token never in output, logs or telemetry | `TestHTTPServesRuntimeToolsOnlyWithToken` scans diagnostics and the telemetry export; `TestServeMCPListenerLifecycle` scans the CLI stdout and stderr; `TestTokenCommands` scans `rotate` output; `TestHTTPGateThrottlesFailedAuthentication` checks rejections |
| Sessions keep their own prior-resolution memory | `TestHTTPSessionsKeepSeparatePriorResolutions` |
| Graceful shutdown with open sessions, telemetry flushed | `TestHTTPShutdownClosesOpenSessions`, telemetry preview after stop in the first test |
| Limits | `TestHTTPGateRateLimitAndSessionCap`, `TestHTTPGateThrottlesFailedAuthentication` |
| termui golden output | `TestListenerBannerGolden` (loopback and trusted banners), `TestServeMCPNetworkFlagErrors`, `TestTokenCommands` |

Commands run:
- `go test -race -count=1 ./internal/delivery/... ./internal/hubtoken/...`: pass.
- `make check` (fmt-check, vet, golangci-lint new issues since origin/main, full `go test ./...`): pass, 0 lint issues.
- `GOOS=windows go vet ./internal/delivery/... ./internal/hubtoken/...` and `GOOS=darwin ...`: pass. File-mode checks are skipped on Windows by design (`hubtoken.Read`, tests skip the permission and symlink cases).

## Loopback round trip for the benchmark phase

`TestHTTPLoopbackResolveLatency`, 200 sequential `skill_resolve` calls over one HTTP session on 127.0.0.1, test workspace with one active skill, Linux, no race detector, three runs:

| Run | Median | p95 | Max |
|---|---|---|---|
| 1 | 2.21 ms | 3.41 ms | 4.14 ms |
| 2 | 2.45 ms | 3.69 ms | 4.13 ms |
| 3 | 2.39 ms | 3.68 ms | 4.07 ms |

For comparison the plan records stdio at about 3 ms median and 9 ms p95, but that was 20 processes on a nearly empty catalog, so the numbers are not directly comparable. Phase 6 should measure both in one harness.

## Review

An independent code review (security, concurrency, shutdown, portability) found no token leak and no authentication bypass. Fixed after the review, with tests:

- The failed-authentication throttle was checked before the token and keyed by remote address; behind an SSH tunnel every client is 127.0.0.1, so one agent with a rotated-out token could lock all others out. A valid token now always passes; only requests without it are throttled (`TestHTTPGateThrottlesFailedAuthentication`).
- The SDK's own loopback DNS-rebinding check refused `--allow-host` names on a loopback listener. It is disabled because the gate already checks every Host against the hub's names (`TestHTTPAllowHostOnLoopbackListener`, real socket).
- Shutdown closed sessions before running calls finished. It now refuses new requests, waits up to 3 s for running requests (counted before the shutdown check, so none can start a session afterwards), then closes sessions and stops the server (`TestHTTPShutdownDrainsRunningRequests`).
- `skillhub mcp serve` now keeps exactly its old flags: the network flags stay "unknown MCP argument" there, as before; the listener is reached only through `skillhub serve mcp --addr`.
- Paths in printed commands are shell-quoted when needed; with `--trust-private-network` on a wildcard address the banner adds a second warning that it listens on every interface.

Not changed (low, documented): the session cap is soft under concurrent first requests; the Host list of a wildcard listener is fixed at start-up; a same-host Origin on another port passes the Origin check (harmless, a browser cannot add the token).

## Deviations and decisions to confirm

- Token location: the task says `$SKILLHUB_CONFIG_DIR`, which in this codebase is a per-skill directory (`runtime/config/<skill-id>`). The hub token is a hub-wide secret, so it lives in the parent workspace config directory as `runtime/config/.hub-token` (ignored by Git through `/runtime/`). `--token-file` overrides it.
- `--allow-host` was added to `serve mcp --addr` (it mirrors `serve web --allow-host`). Without it a Tailscale MagicDNS full name such as `machine-b.tailnet.ts.net` would be refused by the Host check; the short host name already works.
- `skillhub token rotate` previews by default and needs `--yes`, matching the CLI's preview-then-apply convention, because rotation disconnects every client.
- `skillhub token show` creates a token when none exists (only when stdout is a terminal), so a new hub can show its token before the first listener start.
- The global help keeps a `mcp serve` line next to `serve mcp`; an existing test pins it and hosts already launch it.
- Help text keeps the existing `commandUsage` string mechanism shared by every command (printed as is, not through `termui`), so it matches the other commands; all runtime messages of the new commands use `termui`.
- The updated `plan.md` with decisions Q1 to Q5 exists only uncommitted in the primary checkout. This branch updates only `phase-02-http-transport.md` (from the primary checkout's text, with the boxes ticked and status completed); merging may need the lead to reconcile that file with the uncommitted copy.

## Left for later phases (not in code)

- `skill_get` and resource reads over HTTP still return the `io.skillhub/local_path` meta, a path on the hub's disk. Phase 3 replaces it with bundle delivery.
- The hub URL printed for `connect --hub` is the base URL; the MCP endpoint is `<hub>/mcp`. Phase 4 (`connect --hub`, proxy) must append `/mcp`.
- An idle session is closed after 1 hour; the client then gets 404 for its session id and must initialize again. The Phase 3/4 proxy should do that transparently.
- Remote telemetry has no client label yet (Q5 is Phase 5).

## Unresolved questions

1. Are the limit values (128 sessions, 1 h idle, 100 rps burst 200) right for the user's 20-agent load, or should they be flags?
2. Should `--trust-private-network` accept a wildcard address (`0.0.0.0`, `[::]`) at all? Today it does, with a warning, because the plan only asks for the flag; refusing wildcards would force binding the Tailscale address.
3. Should the listener also accept the token from `SKILLHUB_HUB_TOKEN` on the hub side, or is that variable only for machine A as the plan says?
