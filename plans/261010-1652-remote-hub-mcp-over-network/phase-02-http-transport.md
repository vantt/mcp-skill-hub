---
phase: 2
title: "HTTP transport, runtime profile only"
status: pending
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
- Modify: `internal/delivery/cli/mcp.go` (`skillhub mcp serve --http <addr> [--tls-cert --tls-key] [--token-file]`; the profile is forced to `runtime`)
- Create: `internal/delivery/mcpserver/http_test.go`

## Tasks & Steps
1. Refactor the server construction so stdio and HTTP use the same tools, the same activation tracker and the same recorder.
2. Force `--profile runtime` for HTTP; passing `curation` or `all` is an error. Register no curation tool on that server at all, rather than hiding it.
3. Token: generated once into `$SKILLHUB_CONFIG_DIR` (mode 0600), constant-time comparison, `skillhub remote token rotate`; accepted only in the `Authorization: Bearer` header, never in a URL; never written to logs or telemetry.
4. Refuse to listen on a non-loopback address without TLS (Q4); loopback plain HTTP is allowed for SSH tunnels and tests.
5. Limits: request body cap (the existing frame limit), concurrent session cap, idle session timeout, per-token rate limit; keep-alive on so repeat calls reuse the connection (needed for the speed criterion).
6. Graceful shutdown: close sessions, flush the recorder (use a background context for the worker, as the handoff notes warn).

## Verification
- Tests: 401 without or with a wrong token; 403 on a bad Host or Origin; tools/list over HTTP shows exactly the three runtime tools; a curation tool name returns "unknown tool"; plain HTTP on a non-loopback address fails to start.
- `go test -race ./internal/delivery/mcpserver/...` and `make check` green.
- Loopback round trip time for `skill_resolve` recorded for Phase 6.
