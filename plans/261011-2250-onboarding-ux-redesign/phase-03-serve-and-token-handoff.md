---
phase: 3
title: "serve and token hand-off"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 3: serve and token hand-off

## Context

Today `serve mcp` is stdio unless `--addr` is given, refuses non-loopback addresses without
`--trust-private-network` and refuses 0.0.0.0 always (`internal/delivery/mcpserver/http.go:84-106`,
`internal/delivery/cli/mcp.go:142-165`). `serve web` picks 0.0.0.0 only when the machine has two or more usable IPv4
addresses (`internal/delivery/web/listen.go:30-53`). Each server has its own Host check: the MCP gate builds a fixed set
at start (loopback names, every interface IP for a wildcard bind, hostname, `--allow-host`; port ignored,
`http.go:390-418`); the web check refreshes interface IPv4 addresses every 5 s but requires the exact listen port and
ignores IPv6 interface addresses (`internal/delivery/web/security.go:26-90`). Verified on 0.0.0.0: foreign names and IPs
not on this machine get HTTP 421; the banner lists `docker0` and `br-*` bridges. `token show` refuses when stdout is not
a terminal (`internal/delivery/cli/token.go:88-100`). Decision 7 and 10.

After Phase 1, agents use `mcp stdio`, so `serve` is free to be the network command.

## Requirements

- Syntax: `skillhub serve [mcp|web] [--addr <host:port>[,<host:port>...]] [--token-file <file>] [--allow-host <name>]...`
  plus web-only `--no-open` and `--dev`. No subcommand means mcp. Remove `--trust-private-network`,
  `--loopback-only`, `SKILLHUB_WEB_ADDR` (`internal/delivery/cli/serve.go:78`), `--profile` on serve (the network
  server is runtime-only; stdio profiles live on `mcp stdio`).
- Defaults: mcp `0.0.0.0:7420`, web `0.0.0.0:7421`. `--dev` keeps its loopback-only rule.
- `--addr` list: 1 to 3 entries; a missing port (`127.0.0.1` or `100.64.1.2`) takes the mode default; IPv6 written
  `[::1]:7420`. Errors (exit 2, nothing bound): more than 3, unparsable entry, duplicate after normalization, a
  wildcard (`0.0.0.0`, `[::]`, empty host) combined with any other entry on the same port (overlap).
- Bind all listeners first; if any fails, close the ones opened and stop with "Could not listen on <addr>: <reason>"
  plus the existing port-owner hint (exit 1).
- One server per mode across all listeners: `mcpserver.ServeHTTP` takes `[]net.Listener` and serves each with the same
  `http.Server`; the web server gets the list of listen ports instead of `ListenPort` (`web.Options`,
  `internal/delivery/web/server.go:31`).
- One shared host policy (new `internal/delivery/hostpolicy` or a function in an existing shared package) used by both
  servers: accept any IP literal (IPv4 or IPv6, port must be one of the bound ports for web) when a listener is a
  wildcard, otherwise the bound IPs; always accept `localhost`, the hostname and `<hostname>.local`, and `--allow-host`
  names. IP literals cannot be used for DNS rebinding, so accepting them on a wildcard bind keeps the protection while
  letting LAN and Tailscale clients in, including addresses that appear after start.
- Token: mcp keeps the hub token file (`hubtoken.DefaultPath`, created on first use); web keeps the per-run session
  token in the URL fragment. Both are always required (no flag disables them).
- Banner (mcp), every line copy-paste safe through `termui.Command`:
  - "Skill Hub is serving skills to agents on other machines." and the token file path;
  - per usable address (non-loopback, up, unicast, not `docker*`, `br-*`, `veth*`, `virbr*`, `cni*`, `podman*`;
    Tailscale `100.64.0.0/10` labelled "Tailscale", others by interface name), two commands:
    `scp -p <user>@<addr>:<token-file> ~/.skillhub-hub-token` and
    `skillhub connect --hub http://<addr>:7420 --token-file ~/.skillhub-hub-token`;
    for a loopback-only bind, one SSH tunnel line instead (existing text, `mcp.go:275-279`);
  - the token path on the client is a plain file in the home folder so `scp` needs no directory; Windows clients
    may need `$HOME\.skillhub-hub-token` because Windows OpenSSH may not expand `~` in a local path [UNVERIFIED],
    so docs show both forms;
  - one warning line: "Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine."
- Banner (web): same address filter; one URL per usable address with the session token; the same warning line with
  `--addr 127.0.0.1:7421`.
- `token show` keeps its terminal-only rule; its refusal text points at the `scp` line (`skillhub serve` prints it).
  `token rotate` acts directly (Phase 1) and prints the `scp` line to run again on each client.
- `internal/app/doctor_remote.go` fix texts drop `--trust-private-network` and use `skillhub serve` wording.

## Files

- Modify: `internal/delivery/cli/serve.go` (single entry for both modes, `--addr` list parsing, binding),
  `mcp.go` (move `runMCPListener`, `writeListenerBanner`, `hubURLs` into the serve flow; remove listener flags from
  `mcp stdio`), `token.go` (texts), `help.go` (serve, token usage)
- Modify: `internal/delivery/mcpserver/http.go` (`ServeHTTP` over several listeners; remove `CheckListenAddress`
  refusal and `TrustPrivateNetwork`; use the shared host policy)
- Modify: `internal/delivery/web/listen.go` (drop `ChooseListenAddr` interface-count rule, filter in `UsableIPv4`,
  new `WriteStartup`), `security.go` (shared policy), `server.go` (ports list)
- Create: shared host policy file and test (location decided by the implementer; keep it free of web/mcp imports)
- Modify: `internal/app/doctor_remote.go` (fix texts)
- Modify: `.github/workflows/release.yml:732,840` (`--loopback-only` -> `--addr 127.0.0.1:0`),
  `web/e2e/support/server.ts:71` (already passes `--addr`; keep). `scripts/remote-bench.sh` and
  `scripts/remote-timing.sh` pass none of the removed flags (checked)
- Tests to update: `internal/delivery/cli/{serve,serve_mcp,remote_commands,mcp}_test.go` (incl. the listener banner
  golden), `internal/delivery/mcpserver/{http,remote_e2e,remote_sessions,hardening}_test.go`,
  `internal/delivery/web/{listen,security,serve}_test.go`

## Steps

1. Address-list parser with table tests (defaults, IPv6, duplicates, wildcard overlap, more than 3).
2. Shared host policy with table tests (wildcard + IPv4/IPv6 literal, wildcard + foreign name, specific bind + other
   IP, hostname, `.local`, `--allow-host`, wrong port for web).
3. Multi-listener `ServeHTTP` and web server; bind-all-or-nothing.
4. Banners with the interface filter (inject interfaces, as `web.DefaultInterfaces` already allows).
5. Remove old flags and env var; update help, doctor texts, CI and scripts.

## Validation

- `skillhub serve --addr 127.0.0.1:0,127.0.0.2:0` (Linux) opens two listeners; one token works on both (test with
  `httptest` clients); a second process binding the same port fails with the address named and binds nothing.
- Requests with `Host: <any IP literal>:<port>` pass on a wildcard bind; `Host: evil.example` gets 421/403 on both
  servers.
- Banner golden contains no `docker0`/`br-` entries for an injected interface list and one `connect` line per usable
  address; no token in the mcp banner.
- `go test ./internal/delivery/...`; `make web-e2e` smoke still finds the startup URL.
- macOS and Windows: `127.0.0.2` does not exist by default on macOS and Windows, so multi-listener tests use
  `127.0.0.1:0` twice on distinct ports plus `[::1]:0` when IPv6 is available; CI is the check.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Token sniffed on an untrusted LAN | High x Medium | Accepted trade-off (plan.md Security trade-off); warning line every start; docs route untrusted networks to `--addr 127.0.0.1` + SSH tunnel |
| Wildcard IPv4 bind does not cover IPv6 clients (Go dual-stack behavior differs per OS) [UNVERIFIED] | Medium x Low | Test on CI per OS; document `--addr 0.0.0.0,[::]` is an overlap error and `[::]` alone as the IPv6 option if needed |
| Interface filter hides a legitimate address (custom VPN interface) | Low x Low | `--allow-host` and the full address still work; the filter only affects printing |

## Rollback

Revert the phase commits; agents are unaffected because they launch `mcp stdio` or `mcp proxy`, not `serve`.
