# Onboarding UX redesign, phase 3 (serve and token hand-off): implementation report

Branch `worktree-agent-a5c101367df3589e1`, 2026-10-12. Plan:
[plan.md](../261011-2250-onboarding-ux-redesign/plan.md),
[phase-03](../261011-2250-onboarding-ux-redesign/phase-03-serve-and-token-handoff.md). Status: completed.

## What changed

Commits on the branch (nothing pushed):

| Commit | Change |
|---|---|
| `5fb5ac6` feat(hostpolicy) | New package `internal/delivery/hostpolicy`, free of web and mcpserver imports. `Policy.Allows` is the one Host check: any IP literal on a wildcard bind, otherwise the bound IPs; loopback IPs, `localhost`, the host name, `<hostname>.local` and `--allow-host` names always. `RequirePort` (web) also requires a bound port; a web `--allow-host name:port` entry matches exactly. `Reachable` lists the addresses a banner prints (up, non-loopback, global unicast, not `docker*`, `br-*`, `veth*`, `virbr*`, `cni*`, `podman*`; `100.64.0.0/10` labelled "Tailscale"). `ExposureWarning` is the shared warning line. `Interface`/`SystemInterfaces` replace `web.InterfaceInfo`/`web.DefaultInterfaces`. |
| `8227140` feat(cli)! | `skillhub serve [mcp\|web] [--addr <list>] [--token-file] [--allow-host]... [--workspace] [--no-open] [--dev]`. Defaults `0.0.0.0:7420` / `0.0.0.0:7421` (`--dev`: `127.0.0.1:7421`). `parseListenAddresses` (1 to 3 entries, missing port = mode default, IPv6 in brackets, duplicate and wildcard-overlap errors, port 0 exempt), `bindListeners` (all or nothing; "Could not listen on <addr>: <reason>." plus a per-OS port-owner hint, exit 1). `mcpserver.ServeHTTP(ctx, []net.Listener, ...)` and `web.Server.Serve(ctx, []net.Listener)`: one `http.Server` per mode, one token, stop on any listener failure; `Ready` receives every bound address. `CheckListenAddress`, `IsLoopbackAddress`, `ErrNonLoopbackAddress`, `ErrWildcardAddress`, `TrustPrivateNetwork`, `ChooseListenAddr`, `UsableIPv4`, `SKILLHUB_WEB_ADDR`, `--loopback-only`, `--trust-private-network` and `--profile` on serve are removed. `mcp serve` is stdio only (network flags are unknown there, with a hint to `skillhub serve`). MCP Origin: own strict set (`originHosts`: loopback, bound IPs or every interface IP for a wildcard, `--allow-host`) plus the request's own Host. New banners, help for serve, token, mcp and connect, `token show`/`token rotate` hand-off. |
| `dfdde3b` fix(app) | Remote-doctor fix texts and the connect hand-off name `skillhub serve` and its scp line; no trust flag. New `app.RemoteFixRefusal(hubURL, tokenFile)` with the S1 text (Phase 4 wires it into the engine). |
| `ce93560` ci | Release smoke (bash and PowerShell) starts `serve web --no-open --addr 127.0.0.1:0`. |

Files: `internal/delivery/hostpolicy/{policy,interfaces,policy_test}.go` (new), `internal/delivery/cli/{serve_addr,serve_banner,serve_addr_test}.go` (new),
`internal/delivery/cli/{serve,mcp,help,token,connect_remote}.go`, `internal/delivery/mcpserver/http.go`, `internal/delivery/web/{listen,security,server}.go`,
`internal/app/{doctor_remote,connect_remote}.go`, `.github/workflows/release.yml`, `docs/remote-hub-guide.md` (mechanical, see Deviations).

### Banners

MCP (`skillhub serve`), wildcard bind, injected interfaces:

```text
Skill Hub is serving skills to agents on other machines.
It serves the runtime tools only (skill_resolve, skill_get, skill_feedback); curation stays on this
machine.

Hub folder:  /home/ann/skill-hub
Token file:  /home/ann/skill-hub/runtime/config/.hub-token
WARNING: Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine.

eth0 (192.168.1.20), on the other machine:
  $ scp -p ann@192.168.1.20:skill-hub/runtime/config/.hub-token ~/.skillhub-token-192.168.1.20-7420
  $ skillhub connect --hub http://192.168.1.20:7420 --token-file ~/.skillhub-token-192.168.1.20-7420

Tailscale (100.101.102.103), on the other machine:
  ...

The token itself is never printed. Stop the server with Ctrl-C.
```

A loopback listener (or a wildcard bind with no usable address) prints a tunnel block instead: `ssh -N -L <port>:127.0.0.1:<port> <user>@<this-machine>`, then the scp and connect lines with `<this-machine>`. Web: optional warning plus "Anyone with this URL can change the skills every connected agent uses.", then `Skill Hub web UI: <first URL>` and `  <label>: <URL>` per further address (the release smoke reads field 5 of the first line; e2e matches `http://127.0.0.1:<port>/#token=`).

## Tests

- New: `TestPolicyAllows` (28 rows), `TestExposureWarning`, `TestReachableFiltersVirtualInterfaces`; `TestParseListenAddresses` (14 accepted, 18 refused), `TestServeDefaultAddresses`, `TestServeFlagErrors` (17 refusals, nothing bound), `TestMCPServeIsTheStdioServer`, `TestMCPServeFlagErrors`, `TestMCPBannerGolden` (wildcard with a port-0-resolved port, loopback tunnel block, hub outside home and with a space, wildcard without addresses; every printed `skillhub` line through `checkGuideCommand`; no `docker0`/`br-`/`virbr0`/down interface/template port), `TestScpTokenPath`, `TestServeListensOnSeveralAddresses` (`serve --addr 127.0.0.1:0,127.0.0.1:0`, one token works on both), `TestServeBindsAllOrNothing` (mcp and web, busy port named, exit 1, stdout empty); `TestHTTPWildcardAcceptsIPLiteralsButKeepsOriginStrict` (S6: Host `203.0.113.9:7420` passes, `Origin: http://203.0.113.9:7420` with the bound Host gets 403, same-origin by IP passes), `TestHTTPServesSeveralListenersWithOneToken`; web `TestServeSeveralListenersWithOneToken`, `TestStartupOutput` (goldens), security table rows for wildcard/specific binds and a foreign-IP Origin; `TestRemoteFixRefusalNamesTheConnectCommand`, `TestRemoteFixTextsNameTheCurrentServeCommand`.
- Updated: listener lifecycle test (client token path, no warning on loopback, token never printed), token test (refusal text, help), connect and remote-doctor goldens, web test fixtures (`ListenAddrs`), `serve_test` table (bare `serve` now listens, so the "requires a subcommand" case became an unknown-mode case).
- Removed: `TestCheckListenAddress`, `TestHTTPNonLoopbackNeedsTrustedPrivateNetwork`, `TestListenRule`, the `serve mcp` stdio parity tests, the trusted-banner golden, and the web test that bound `0.0.0.0:0` (replaced by a two-loopback-listener test; no test binds outside loopback).
- Results (isolated HOME/XDG_*/`SKILLHUB_USER_CONFIG_DIR`, `GIT_CONFIG_GLOBAL=/dev/null`): `go test -count=1 ./...` passes; `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=darwin go vet ./...` clean; `gofmt -l .` empty. The intermediate commit `8227140` was checked from a `git archive` copy (vet, plus the cli, web and app packages).
- Hand check of an installed binary in a scratch sandbox: `init`, `serve --addr 127.0.0.1:0,127.0.0.1:0` (two tunnel blocks, scp lines with the home-relative path), busy port (exit 1, nothing started), wildcard overlap (exit 2), `serve web --addr 127.0.0.1:0` (one URL line), `token show` off a terminal, `serve --help`/`serve web -h`.

## Decisions where the plan was silent

1. **Host policy accepts loopback IP literals on every bind** (as both servers did before), in addition to the plan's list; an IP literal cannot be used for DNS rebinding.
2. **`web.Options.ListenAddrs []net.Addr` instead of a ports list**: the policy needs to know whether a listener is a wildcard and which IPs are bound, not only the ports. `Options.Interfaces` is gone (the Host check no longer reads interfaces); `Hostname` stays, read once at start.
3. **MCP Origin set** follows the plan's list (no host name in it); the request's own Host covers a same-origin request by host name. Ports are ignored in the MCP Origin comparison, as in its Host check.
4. **Network per entry**: `0.0.0.0` and IPv4 addresses `tcp4`, a specific IPv6 address `tcp6`, `[::]` and an empty host `tcp` (dual-stack where the OS allows), so `0.0.0.0` stays IPv4 (banner shows `0.0.0.0`) and `[::]` alone is the IPv6 option the risk table mentions.
5. **`--addr` hosts are IP literals, an empty host or `localhost`** (mapped to `127.0.0.1`); host names are refused, because a name can stand for several addresses and the duplicate/overlap rules need addresses. `--addr` given twice is an error (use commas). An empty entry, a bare IPv6 address without brackets and `host:` (empty port) are errors.
6. **Warning wording for a specific non-loopback bind**: "Listening on a network address over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine." The plan's "all network interfaces" sentence is printed only when a listener is a wildcard, so the line stays true. It is printed unwrapped (one line) on both banners.
7. **scp line**: printed only when the token file is under the hub owner's home (also via the resolved home symlink) and the home-relative path has only `[A-Za-z0-9._+/-]` (no leading `-`); otherwise one advice line. The login name comes from `os/user` (Windows domain stripped), else `<user>`. scp uses `-p` so the copy keeps mode 0600, which the proxy requires; the hand-off texts no longer ask for `chmod 600`.
8. **Loopback listeners** get a tunnel block (ssh line, then scp and connect with `<this-machine>`); a wildcard bind with no usable address gets the same block so the banner always has a runnable path.
9. **`token rotate`** prints `scp -p <user>@<this-machine>:<home-relative path> ~/.skillhub-token-<host>-<port>` as its next step when the path is printable, else the old sentence. `rotate` cannot know the client's host and port, so those two stay placeholders.
10. **`serve` refusals**: `--profile` on serve gets its own reason ("serve has no --profile ...") instead of "unknown argument"; `--token-file` on web and `--no-open`/`--dev` on mcp are refused with the mode they belong to; an unknown mode word is refused.
11. **Web banner**: the first URL is the loopback one when a listener allows it (also the URL the browser opens); further URLs are indented `  <label>: <url>` lines, so `grep "Skill Hub web UI:"` in the release smoke still finds exactly one line.
12. **`internal/app/connect_remote.go`** (the `hub_unreachable` warning) and `internal/delivery/cli/connect_remote.go` (`tokenHandOff`) were reworded here although only `doctor_remote.go` is named in the phase files list: they printed `skillhub serve mcp --addr ...` and the old copy advice, and the inventory row "`token show` advice" assigns `connect_remote.go` to this phase.

## Deviations

- `docs/remote-hub-guide.md`: two command lines used `--trust-private-network` and failed the guide test (`TestRemoteHubGuideCommandsExist`); the Tailscale start step and its refusal/banner samples were replaced by `skillhub serve --addr <tailscale-ip>:7420` and the new warning line, and the cheat-sheet row lost the flag. The wording pass stays in Phase 5.
- `scripts/web-dev.sh` needed no change (`serve web --dev` defaults to `127.0.0.1:7421`); `web/e2e/support/server.ts` already passes `--addr 127.0.0.1:0` and was not touched.

## Not done / for later phases

- Docs other than the remote guide still name `--trust-private-network`, `--loopback-only`, `SKILLHUB_WEB_ADDR` and `serve mcp --addr` (README, `command-reference`, `getting-started`, `release-runbook`, `sources-and-learning`, `troubleshooting`, `web-ui`, `contracts/web-api.md`, `design/remote-hub.md`): Phase 5, with its grep gate. The `web/e2e/smoke.spec.ts` test name says "loopback-only requests" about browser traffic, not the flag; left as is.
- `RemoteFixRefusal` exists with tests but is not yet printed by `doctor --fix`: Phase 4 wires it into the check engine (S1).
- Web typecheck and e2e were not run: no `web/node_modules` in the worktree (not installed). No web source changed; the e2e startup regex and the release smoke parsing were checked against the new banner by tests.
- Windows and macOS were not run; CI is the check. Platform-sensitive: `TestServeBindsAllOrNothing` (a second bind of a used loopback port must fail; Go sets no `SO_REUSEADDR` on Windows), `TestMCPBannerGolden`/`TestScpTokenPath` (backslash paths become forward slashes; long temp paths do not wrap because `Fields` wraps only at spaces), `currentUserName` on Windows (`DOMAIN\user`). Whether `~` in the scp local path expands in Windows PowerShell stays UNVERIFIED (plan); docs show both forms in Phase 5.
- Dual-stack behaviour of `[::]` per OS stays as the plan's risk row describes; no test binds a wildcard.

## Unresolved questions

- None blocking. Phase 5 may want the README to explain that `skillhub serve` with two loopback listeners prints two tunnel blocks (one per port).

Status: DONE
