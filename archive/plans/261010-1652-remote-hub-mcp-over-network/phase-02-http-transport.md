---
phase: 2
title: "HTTP transport, runtime profile only"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: HTTP transport, runtime profile only

## Goal
Serve the three runtime tools over Streamable HTTP with authentication, so a remote agent can call them; nothing that writes or curates is reachable.

## Files to Create / Modify
- Modify: `internal/delivery/mcpserver/server.go` (second entry point next to the stdio `server.Run`, sharing one tool registration)
- Create: `internal/delivery/mcpserver/http.go` (handler using `mcp.NewStreamableHTTPHandler`, bearer-token middleware, Host and Origin checks, body size cap)
- Modify: `internal/delivery/cli/mcp.go` (`skillhub serve mcp --addr <host:port> [--trust-private-network] [--token-file]`; without `--addr` it is today's stdio server, and `mcp serve` stays as an alias; with `--addr` the profile is forced to `runtime`)
- Create: `internal/delivery/mcpserver/http_test.go`

## Tasks & Steps
- [x] 1. Refactor the server construction so stdio and HTTP use the same tools, the same activation tracker and the same recorder.
- [x] 2. Force `--profile runtime` for HTTP; passing `curation` or `all` is an error. Register no curation tool on that server at all, rather than hiding it.
- [x] 3. Token: generated once into `$SKILLHUB_CONFIG_DIR` (mode 0600), constant-time comparison, `skillhub token show` and `skillhub token rotate`; accepted only in the `Authorization: Bearer` header, never in a URL; never written to logs or telemetry.
- [x] 4. No TLS in this phase (Q2). Refuse to start on any non-loopback address unless `--trust-private-network` is given (Q4); loopback plain HTTP is allowed for SSH tunnels and tests. When the flag is given, say in one line on start-up that the transport is trusted to be a private network. The listener prints the `connect --hub` command to run on machine A, with the token file path and never the token.
- [x] 5. Limits: request body cap (the existing frame limit), concurrent session cap, idle session timeout, per-token rate limit; keep-alive on so repeat calls reuse the connection (needed for the speed criterion).
- [x] 6. Graceful shutdown: close sessions, flush the recorder (use a background context for the worker, as the handoff notes warn).

## Verification
- [x] Tests: 401 without or with a wrong token; 403 on a bad Host or Origin; tools/list over HTTP shows exactly the three runtime tools; a curation tool name returns "unknown tool"; a non-loopback address fails to start without `--trust-private-network` and starts with it; `skillhub serve mcp` without `--addr` behaves exactly as `mcp serve` did (regression test).
- [x] `go test -race ./internal/delivery/mcpserver/...` and `make check` green.
- [x] Loopback round trip time for `skill_resolve` recorded for Phase 6.

## Implementation notes (2026-10-11)
- The token file is `<workspace>/runtime/config/.hub-token` (the workspace config directory that also holds the per-skill `SKILLHUB_CONFIG_DIR` folders; the leading dot keeps it apart from skill ids). `--token-file` overrides it. A running listener rereads the file when it changes, so `token rotate` takes effect at the next request.
- The MCP endpoint path is `/mcp`; the hub URL printed for `connect --hub` is the base URL without the path. Phase 4 must append `/mcp`.
- Limits chosen: 128 open sessions, 1 hour idle session timeout, 100 requests per second (burst 200) for the shared token, 20 failed authentications per minute per remote address, 4 MiB request body. `--allow-host` adds Host names such as a Tailscale MagicDNS full name.
- `skill_get` over HTTP still returns `io.skillhub/local_path`; Phase 3 replaces it with bundle delivery.
- Report: `plans/reports/cook-261011-remote-phase2.md`.
- Decisions after review (2026-10-11): the limits above stay as fixed defaults, not flags (enough for about 20 agents); the hub reads its token from the file only (the environment variable `SKILLHUB_HUB_TOKEN` is for the client in Phase 4); `--trust-private-network` still refuses `0.0.0.0` and `[::]`, so a private interface address is required.
