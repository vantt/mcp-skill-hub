---
phase: 3
title: "serve and token hand-off"
status: pending
priority: P1
effort: "1.5d"
dependencies: [1, 2]
---

# Phase 3: serve and token hand-off

## Context

Today `serve mcp` is stdio unless `--addr` is given, refuses non-loopback addresses without
`--trust-private-network` and refuses 0.0.0.0 always (`internal/delivery/mcpserver/http.go:84-106`,
`internal/delivery/cli/mcp.go:142-165`). `serve web` picks 0.0.0.0 only when the machine has two or more usable IPv4
addresses (`internal/delivery/web/listen.go:30-53`). Each server has its own Host check: the MCP gate builds a fixed set
at start (loopback names, every interface IP for a wildcard bind, hostname, `--allow-host`; port ignored,
`allowedHosts`, `http.go:390-416`) and its Origin check reuses that same set (`originAllowed`, `http.go:327-333`, called
at `:243-246`); the web check refreshes interface IPv4 addresses every 5 s but requires the exact listen port and
ignores IPv6 interface addresses (`internal/delivery/web/security.go:26-90`), and web mutations require `Origin` equal
to `"http://" + r.Host` (`security.go:195`). The listener banner already derives the port from the actual address
(`hubURLs`, `internal/delivery/cli/mcp.go:291-307`). `token show` refuses when stdout is not a terminal
(`internal/delivery/cli/token.go:89`). Decisions 7 (revised) and 10; red-team items S5 (banner), S6, F7.

Agents launch `mcp serve` (stdio, hidden, unchanged; P1). `serve` becomes the network command only.

## Requirements

- Syntax: `skillhub serve [mcp|web] [--addr <host:port>[,<host:port>...]] [--token-file <file>] [--allow-host <name>]...`
  plus web-only `--no-open` and `--dev`. No subcommand means mcp. Remove `--trust-private-network`,
  `--loopback-only`, `SKILLHUB_WEB_ADDR` (`internal/delivery/cli/serve.go:44,78`), `--profile` on serve (the network
  server is runtime-only; stdio profiles stay on `mcp serve`). `serve mcp` without `--addr` no longer serves stdio.
- Defaults: mcp `0.0.0.0:7420`, web `0.0.0.0:7421`. `--dev` keeps its loopback-only rule.
- `--addr` list (F7): 1 to 3 entries; a missing port (`127.0.0.1` or `100.64.1.2`) takes the mode default; IPv6
  written `[::1]:7420`. Errors (exit 2, nothing bound): more than 3, unparsable entry, duplicate after normalization,
  a wildcard (`0.0.0.0`, `[::]`, empty host) combined with any other entry on the same port (overlap; `0.0.0.0` and
  `[::]` on the same port overlap too). Port 0 is exempt from duplicate and overlap detection (each `:0` gets its own
  port): tests, e2e and release smoke use `127.0.0.1:0` (`web/e2e/support/server.ts:71`, `.github/workflows/release.yml:732,840`).
- Bind all listeners first; if any fails, close the ones opened and stop with "Could not listen on <addr>: <reason>"
  plus the existing port-owner hint (exit 1).
- One server per mode across all listeners: `mcpserver.ServeHTTP` takes `[]net.Listener` and serves each with the same
  `http.Server`; the web server gets the list of listen ports instead of `ListenPort` (`web.Options`,
  `internal/delivery/web/server.go:30`).
- One shared Host policy (new package, for example `internal/delivery/hostpolicy`, free of web/mcp imports) used by
  both servers: accept any IP literal (IPv4 or IPv6; for web the port must be one of the bound ports) when a listener is
  a wildcard, otherwise the bound IPs; always accept `localhost`, the hostname and `<hostname>.local`, and
  `--allow-host` names. IP literals cannot be used for DNS rebinding, so accepting them on a wildcard bind keeps that
  protection while letting LAN and Tailscale clients in, including addresses that appear after start.
- Origin stays strict (S6): the MCP Origin check accepts only the bound addresses (each interface IP for a wildcard
  bind, as today), the request's own Host value, `localhost`/loopback and `--allow-host` names; the "any IP literal"
  relaxation applies to the Host check only. `originAllowed` stops reading the Host set. Web keeps comparing `Origin`
  to `"http://" + r.Host` (unchanged).
- Token: mcp keeps the hub token file (`hubtoken.DefaultPath`, created on first use); web keeps the per-run session
  token in the URL fragment. Both are always required (no flag disables them).
- Banner (mcp), every line copy-paste safe through `Printer.Command`:
  - "Skill Hub is serving skills to agents on other machines." and the token file path;
  - one block per usable address (non-loopback, up, unicast, not `docker*`, `br-*`, `veth*`, `virbr*`, `cni*`,
    `podman*`; Tailscale `100.64.0.0/10` labelled "Tailscale", others by interface name). Every URL is built from the
    listener's actual `Addr()` port, never a template port (F7). Per address, run on the client (S5):
    `scp -p <user>@<addr>:<relative token path> ~/.skillhub-token-<host>-<port>` where the remote path is relative to
    the hub owner's home when the hub is under home (no spaces or quotes to escape), and otherwise the banner says
    "cd to a folder without spaces in its path, or copy the file by another means" instead of printing a quoted remote
    path (legacy scp and SFTP mode quote remote paths differently);
    then `skillhub connect --hub http://<addr>:<port> --token-file ~/.skillhub-token-<host>-<port>` (path from
    `hubtoken.ClientPath`, Phase 2);
  - for a loopback-only bind, one SSH tunnel line instead (existing text, `internal/delivery/cli/mcp.go:275-279`);
  - Windows clients may need `$HOME\.skillhub-token-<host>-<port>` because Windows OpenSSH may not expand `~` in a local
    path [UNVERIFIED], so docs show both forms;
  - one warning line: "Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7420 to keep it on
    this machine." (printed for any non-loopback bind).
- Banner (web): same address filter; one URL per usable address with the session token, ports from `Addr()`; the same
  warning line with `--addr 127.0.0.1:7421`, plus "Anyone with this URL can change the skills every connected agent
  uses." (decision 7 revised: the web UI has write routes).
- `token show` keeps its terminal-only rule; its refusal text points at the `scp` line (`skillhub serve` prints it).
  `token rotate` acts directly (Phase 1, terminal only) and prints the `scp` line to run again on each client.
- Remote-doctor fix texts are reworded once, here (`internal/app/doctor_remote.go`): no `--trust-private-network`,
  `skillhub serve` wording for the hub machine, connect lines with the per-hub token path, and the S1 refusal text
  for client-remote `--fix`: "doctor --fix does not reconnect to a remote hub named in this project's files; if you
  trust <url>, run: `skillhub connect --hub <url> --token-file <file>`". Phase 4 wires the refusal into the engine.

## Files

- Modify: `internal/delivery/cli/serve.go` (single entry for both modes, `--addr` list parsing, binding),
  `mcp.go` (move `runMCPListener`, `writeListenerBanner` at `:249`, `hubURLs` at `:291` into the serve flow; `mcp serve`
  keeps only stdio flags), `token.go` (texts), `help.go` (serve, token usage)
- Modify: `internal/delivery/mcpserver/http.go` (`ServeHTTP` over several listeners; remove `CheckListenAddress` and
  `TrustPrivateNetwork`; Host check through the shared policy; `originAllowed` with its own strict set)
- Modify: `internal/delivery/web/listen.go` (drop the `ChooseListenAddr` interface-count rule at `:30`, filter in
  `UsableIPv4` at `:12`, new `WriteStartup`), `security.go` (shared Host policy), `server.go` (ports list)
- Create: shared Host policy file and test
- Modify: `internal/app/doctor_remote.go` (fix texts)
- Modify: `.github/workflows/release.yml:732,840` (`--loopback-only` -> nothing; keep `--addr 127.0.0.1:0`),
  `web/e2e/support/server.ts:71` (already passes `--addr 127.0.0.1:0`; keep). `scripts/remote-bench.sh` and
  `scripts/remote-timing.sh` pass none of the removed listener flags (re-checked 2026-10-11; `remote-timing.sh:110`
  was handled in Phase 1)
- Tests to update: `internal/delivery/cli/{serve,serve_mcp,remote_commands,mcp}_test.go` (incl. the listener banner
  golden), `internal/delivery/mcpserver/{http,remote_e2e,remote_sessions,hardening}_test.go`,
  `internal/delivery/web/{listen,security,serve}_test.go`

## Steps

1. Address-list parser with table tests (defaults, IPv6, duplicates, wildcard overlap incl. `0.0.0.0` + `[::]`, port 0
   twice allowed, more than 3).
2. Shared Host policy with table tests (wildcard + IPv4/IPv6 literal, wildcard + foreign name, specific bind + other
   IP, hostname, `.local`, `--allow-host`, wrong port for web).
3. Strict Origin set for MCP and its tests.
4. Multi-listener `ServeHTTP` and web server; bind-all-or-nothing.
5. Banners with the interface filter (inject interfaces, as `web.DefaultInterfaces` already allows) and actual ports.
6. Remove old flags and env var; update help, remote-doctor texts, CI.

## Validation

- `skillhub serve --addr 127.0.0.1:0,127.0.0.1:0` opens two listeners on distinct ports; one token works on both
  (`httptest` clients); a second process binding a used port fails with the address named and binds nothing.
- Requests with `Host: <any IP literal>:<port>` pass on a wildcard bind; `Host: evil.example` gets 421/403 on both
  servers. On a wildcard MCP bind, `Host: 203.0.113.9:7420` passes while `Origin: http://203.0.113.9:7420` from a
  request whose Host is the bound address gets 403 (S6).
- Banner golden (injected interfaces, listener on port 0 resolved to a real port): no `docker0`/`br-` entries, one
  `connect` and one `scp` line per usable address, URLs carry the actual port, the warning line present on mcp and web,
  no token in the mcp banner.
- `go test ./internal/delivery/...`; `make web-e2e` smoke still finds the startup URL; release smoke steps start with
  `--addr 127.0.0.1:0`.
- macOS and Windows: `127.0.0.2` does not exist by default, so multi-listener tests use `127.0.0.1:0` twice plus
  `[::1]:0` when IPv6 is available; CI is the check.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Token sniffed on an untrusted LAN; web write routes reachable from the LAN | High x High | Accepted by the user (plan.md Security trade-off); warning line on both banners every start; docs route untrusted networks to `--addr 127.0.0.1` + SSH tunnel |
| Host relaxation weakens the Origin check | Low x High | Separate strict Origin set; foreign-IP Origin test |
| Wildcard IPv4 bind does not cover IPv6 clients (Go dual-stack behavior differs per OS) [UNVERIFIED] | Medium x Low | Test on CI per OS; document `[::]` alone as the IPv6 option |
| Interface filter hides a legitimate address (custom VPN interface) | Low x Low | `--allow-host` and the full address still work; the filter only affects printing |

## Rollback

Revert the phase commits; agents are unaffected because they launch `mcp serve` or `mcp proxy`, not `serve`.
