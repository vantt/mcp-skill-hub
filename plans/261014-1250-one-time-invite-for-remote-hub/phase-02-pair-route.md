# Phase 2: Pair route on the network listener

## Context
Plan: [plan.md](plan.md). Depends on phase 1 (`invite.Service`). Scout notes: `httpGate.ServeHTTP` in
`internal/delivery/mcpserver/http.go` checks Host, Origin, the path allowlist, then the bearer token.

## Requirements
- New route `POST /mcp/pair`, body `{"invite":"<code>"}` limited to 1 KiB, reply `200 {"token":"<hub token>"}` with
  `Cache-Control: no-store`. Any other method: `405`.
- Order inside the gate: Host check, Origin check, path allowlist (add `/mcp/pair`), then for this path only:
  skip the bearer check, run the transport check, the route's failure budget, then `Service.Redeem`.
- Transport check, fail closed: the local address from `http.LocalAddrContextKey` **and** the remote address must
  each be loopback or in `100.64.0.0/10` (unmap IPv4-mapped IPv6 first), and the request must have no `Forwarded`,
  `X-Forwarded-*` or `Via` header. Otherwise `403` "pairing is only offered over Tailscale or an SSH tunnel" and do
  not call `Redeem`. Put the classification in `hostpolicy` (it already knows the Tailscale range) as
  `PairingAllowed(local, remote net.Addr, header http.Header) bool`, not in the handler.
- Failure throttle: reuse the existing `authFailures` (per remote address, same limit as bad bearer tokens). Count only
  failed redeems that passed the transport check, so an outsider cannot lock pairing out. Tunnel clients share one
  bucket; the code's entropy carries the security, not the throttle.
- Read the body with `http.MaxBytesReader` (1 KiB) and a 5 second read deadline (`http.NewResponseController`).
- The handler reads the hub token **first** (add `Source.Token() (string, error)`, a thin exported wrapper over the
  existing `current()`), and only if that works calls `Redeem`; a token that cannot be read never burns an invite. It
  never logs the invite, the token or the request body.
- `HTTPOptions` gains `Invites Redeemer`, an interface with `Redeem(string) error` **defined in `mcpserver`** (the consumer
  owns it; `internal/invite` is not imported by `mcpserver`); nil disables the route
  (`404`), so the existing listener behaviour and tests are untouched. `skillhub serve` wires a real service on
  `<workspace>/runtime/config/invites`.
- The bearer token is never accepted as an invite, and an invite is never accepted on `/mcp`.

## Files
Edit `internal/hubtoken/hubtoken.go` (`Source.Token`), `internal/delivery/mcpserver/http.go` (allowlist, wiring, option) and add `http_pair.go` (handler, budget),
`http_pair_test.go`; add `PairingAllowed` to `internal/delivery/hostpolicy`. Wire in `internal/delivery/cli/serve.go`
(`runServeMCP`: build `invite.Service` with `invite.NewFileStore`). Nothing else.

## Steps
1. Tests first with `startHTTPTestHub`-style helpers and a fake `Redeemer`: valid invite returns the token once; a second
   call `403`; wrong method `405`; oversized body `413`; missing or wrong content `400`; bearer token as invite `403`;
   an invite as `Authorization: Bearer` on `/mcp` `401`; route disabled when `Invites` is nil.
2. Transport check tests: loopback and `100.101.102.103` allowed; `192.168.1.20` as local or as remote refused;
   IPv4-mapped IPv6 loopback allowed; any forwarding header refused; the invite is still redeemable afterwards.
3. Throttle test: after the allowed failures from one remote the next call (even with a valid code) is throttled; a
   failure from a refused transport is not counted (use the throttle's injectable clock like the existing test);
   a token that cannot be read returns an error and leaves the invite unredeemed.
4. Implement `http_pair.go` and the `hostpolicy` helper; wire `serve.go`.
5. Check that no log line or error string contains the code or the token (grep the test output).

## Validation
`go test ./internal/hubtoken ./internal/delivery/mcpserver ./internal/delivery/hostpolicy -race -count=1`, then `make check`.

## Risk and rollback
The gate is security-critical: the diff must show the bearer check unchanged for every other path. Rollback is
removing the allowlist entry and `Invites`.
