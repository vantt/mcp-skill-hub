---
title: "One-time invite to hand the hub token to another machine"
description: "skillhub token invite prints a single-use, short-lived code; skillhub connect --hub <url> --invite <code> redeems it over the network listener and saves the shared hub token in the user config directory, so the scp step is no longer needed."
status: pending
priority: P2
effort: 8h
branch: main
tags: [remote-hub, security, cli, mcp-listener]
blockedBy: []
blocks: []
created: 2026-10-14
---

# One-time invite for the remote hub

## Outcome, constraints, non-goals

- **Outcome.** On the hub machine the user runs `skillhub token invite` and gets, for every address that is safe to
  use, one line to paste on the agent machine:
  `skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD`. That line redeems the code, saves the
  hub token on the agent machine and connects the project, in one step. `scp` stays as the manual fallback.
- **Decisions (user, 2026-10-14).**
  - One shared hub token for every machine (no per-machine tokens, no revocation list).
  - Command names exactly as above; `token invite` sits next to `token show` and `token rotate`.
  - The client saves the token under the user config directory in a `hub-tokens` folder, using the platform
    standard location from `userconfig.Dir()`: `~/.config/skillhub` on Linux,
    `~/Library/Application Support/skillhub` on macOS, `%AppData%\skillhub` on Windows,
    `$SKILLHUB_USER_CONFIG_DIR` overriding all of them. The path is never hard-coded to `~/.config`.
  - `skillhub serve --print` (unreleased, commit 73c8f47) is removed once `token invite` exists: invite replaces it as
    the way to get the connect line again. Its address-listing code is reused, not thrown away.
  - Code must be fast and light, clean, hexagonal where it helps.
  - After it is done and reviewed, it is released (the release itself stays with session `mcp-skill-hub-68`).
- **Constraints.**
  - Never write to the live hub `/home/vantt/skill-hub` or `/home/vantt/skillhub`; tests use temp dirs, isolated HOME
    and `SKILLHUB_USER_CONFIG_DIR`. Never print a token or an invite code in logs, errors, telemetry or test output.
  - Telemetry event types and `EventVersion` do not change. No plan ids or phase numbers in code, tests or commits.
  - The invite code is never accepted as a bearer token, and the hub token is never accepted as an invite.
  - Only 68 pushes, tags and releases.
- **Non-goals.** Per-machine tokens or revocation; a web UI for invites; pairing over plain HTTP on a LAN (see
  decision 1 below); moving existing `~/.skillhub-token-*` files (they keep working as written in `.mcp.json`).

## Design in one page

```text
hub machine                                         agent machine
-----------                                         -------------
skillhub token invite  ──> invite.Service.Issue      skillhub connect --hub U --invite CODE
   writes runtime/config/invites/<sha256(code)>         app.PairHub(U, CODE)  ── POST U/mcp/pair {"invite":CODE}
skillhub serve  ──> mcpserver gate ── /mcp/pair  <─────────────────────────────┘
   invite.Service.Redeem(code)  (consume = atomic remove)
   on success: reply {"token": <hub token>}  ──────────> hubtoken client store (config dir, 0700/0600)
                                                         then the existing ConnectRemote flow runs
```

- **Invite code.** 12 characters of Crockford base32 (60 bits of entropy from `crypto/rand`), printed in groups of
  four, case-insensitive, `O`→`0`, `I`/`L`→`1` on input. Dashes and spaces are ignored on input.
- **Single use without a lock.** The store keeps one empty file per pending invite, named by the SHA-256 of the
  normalised code (`<hub>/runtime/config/invites/<hex>`, mode 0600, folder 0700, not a symlink), whose
  **modification time is the expiry**. Nothing is ever opened to claim it: `Claim` does `os.Stat` (expiry check) and
  then `os.Rename` to `<hex>.claimed`, then removes the claimed file. A rename is atomic and exactly one caller
  wins, across threads and across the `token invite` and `serve` processes, and it works on Windows because no
  handle is held open (an `os.Remove` of an open file fails there). `Put` writes a temp file, sets its mtime to
  the expiry and renames it into place, so a half-written invite is never visible. Only the hash is stored. No
  lock file, no JSON database.
- **One generic failure.** Unknown, expired and already used all return the same `403 invite not valid`. The
  transport refusal comes before any lookup, so a different message there reveals nothing about codes.
- **Where pairing is allowed (fail closed).** `/mcp/pair` answers only when **both** the local address the
  connection arrived on and the remote address it came from are loopback or in 100.64.0.0/10 (after unmapping IPv4-
  mapped IPv6), and the request carries no `Forwarded`, `X-Forwarded-*` or `Via` header (a same-host reverse proxy,
  `tailscale serve` or Funnel would otherwise look like loopback). Anything else answers `403 pairing is only
  offered over Tailscale or an SSH tunnel` and does not consume the invite. Residual risk, documented: 100.64.0.0/10
  is the generic carrier-grade NAT range, and a local user on the hub machine can use the route like any client.
  `token invite` prints only the addresses where it will work.
- **Route in the gate.** `/mcp/pair` is a new entry in the path allowlist of `httpGate`. It keeps the Host and Origin
  checks, skips the bearer check (the invite is its credential), and reuses the existing per-remote `authFailures`
  throttle for failed redeems (counted only after the transport check passed, so an outsider cannot lock pairing
  out). `POST` only, body limited to 1 KiB with a 5 second read deadline, response `Cache-Control: no-store`. The
  handler reads the hub token **before** it redeems, so a token problem never burns an invite.
- **Client.** `connect --invite` first validates everything it can without the network (URL, that the URL host is
  loopback or in the Tailscale range so the code is never sent over a plain LAN, the code's format, and that the
  destination folder can be created and written), then redeems, then writes the token file atomically to
  `<userconfig.Dir()>/hub-tokens/<host>-<port>`, then runs the normal remote connect with that path, so nothing
  after the redeem changes. `--token-file` given together with `--invite` names where to save instead. `--invite`
  needs `--hub` and cannot be combined with `--dry-run` (a dry run would burn the code). The HTTP client is the same
  proxy-free, redirect-free client the hub probe uses.

## Hexagonal split (follows the layout the repo already has)

The repo has no `ports/` or `adapters/` directories: use cases and small interfaces live in `internal/app` or in
the domain package, adapters in `internal/delivery/*`. The feature follows that, adding one package:

| Layer | Piece | Notes |
|---|---|---|
| Domain | `internal/invite`: `Code`, `Service{Issue, Redeem}`, `ErrInvalid` | Pure. Takes a `Store`, a clock function and a random source. No `net/http`. |
| Port | `invite.Store{Put, Claim}` | Two methods. `Claim` is "atomically take this invite if it exists and has not expired". |
| Adapter (outbound) | `invite.NewFileStore(dir)` | The one-file-per-invite store above. Same package, like `userconfig.fileStore`. |
| Adapter (inbound, server) | `mcpserver` pair handler + gate entry; the `Redeemer` interface is defined here, where it is consumed | Decodes JSON, reads the token via a new `hubtoken.Source.Token()`, asks the redeemer, replies. No invite logic of its own. |
| Adapter (inbound, hub CLI) | `token invite` | Parses flags, calls `Service.Issue`, prints targets. |
| Use case (client) | `app.PairHub(ctx, url, code) (token, error)` next to `ProbeHub`, built on `hubProbeClient()` | No proxy (a proxy would see the code and the token) and no redirects. |
| Adapter (outbound, client) | `hubtoken.Save(path, token)` extracted from `Rotate`, plus `hubtoken.ClientStorePath(configDir, url)` | Atomic write, 0700 folder and 0600 file; `userconfig` is not touched. |
| Delivery (client CLI) | `connect --invite` | Flags and messages only. |

## Phases

| # | Phase | Depends on | Files (owner: the phase) |
|---|-------|-----------|--------------------------|
| 1 | [Invite domain and file store](phase-01-invite-domain-and-store.md) | none | `internal/invite/*` |
| 2 | [Pair route on the network listener](phase-02-pair-route.md) | 1 | `internal/delivery/mcpserver/http.go` and new `http_pair.go`, tests |
| 3 | [token invite, shared address list, remove serve --print](phase-03-token-invite-command.md) | 1 | `internal/delivery/cli/token.go`, `serve.go`, `serve_banner.go`, `help.go`, tests |
| 4 | [connect --invite and the client token store](phase-04-connect-invite.md) | 1, 2 | `internal/app/pair_hub.go`, `connect_remote.go`, `internal/hubtoken`, `internal/delivery/cli/connect*.go`, tests |
| 5 | [Docs, review, release](phase-05-docs-review-release.md) | 2, 3, 4 | README (both), `docs/remote-hub-guide.md`, `docs/command-reference.md`, `docs/design/remote-hub.md` |

Phases 2, 3 and 4 touch different files and may run in parallel after phase 1 merges, each in its own worktree;
phase 5 starts when all three are merged.

## Risks

| Risk | Effect | Mitigation |
|---|---|---|
| Guessing the code | Attacker gets the hub token | 60-bit random code, single use, 10 minute life, per-remote failure throttle, generic error, only reachable over Tailscale or loopback |
| Shared 127.0.0.1 behind an SSH tunnel | Tunnel clients share one throttle bucket | Entropy carries the security, not the throttle; only transport-checked failures are counted |
| Code sniffed on the wire | Token stolen | Server refuses plain-LAN connections; client refuses to send a code to a non-Tailscale, non-loopback host |
| Proxy or redirect sees the POST | Code and token leak | Client uses the proxy-free, redirect-free probe client; server refuses requests with forwarding headers |
| Two machines redeem the same code at once | Both get the token | Single atomic `os.Rename` claim: exactly one wins, also on Windows |
| Invite burned by a later failure | User needs a new invite | Server reads the token before redeeming; client validates and checks the destination first; errors say the invite was used |
| Code visible in the command line on a shared machine | Another local user could read it for a few minutes | Documented; single use plus the 10 minute life |
| Windows mode bits are not enforced | Token file readable by others | Same limit the existing token file has; `%AppData%` is per-user by default; documented, no new ACL code |
| Leftover invite files on disk | Clutter | Only the hub owner can create invites; expired ones are removed on every issue and when seen |
| Invite redeemed but later connect step fails | Token saved, project not connected | Say so in the error and tell the user to rerun `connect --hub <url> --token-file <saved path>` |

## Red-team resolution

Full report: [red-team-261014-invite-plan.md](../reports/red-team-261014-invite-plan.md) (2 critical, 5 high, 8 medium).
Every critical and high finding was checked against the code and is folded into the design above:

| Finding | Resolution |
|---|---|
| Critical: an open file cannot be removed on Windows, so a concurrent claim could fail | `Claim` never opens the file: expiry in mtime, `os.Stat`, then atomic `os.Rename`; the 20-goroutine test runs on the Windows job |
| Critical: invite burned before later steps could fail | Server reads the token before redeeming; client validates and checks the destination before the request; errors say the invite was used |
| High: default HTTP client honours `HTTP_PROXY` | `PairHub` is built on `hubProbeClient()` (no proxy, no redirects), verified in `doctor_remote.go` |
| High: a global failure budget lets anyone lock pairing out | Reuse per-remote `authFailures`, count only failures that passed the transport check |
| High: local address alone does not prove an encrypted path | Local and remote address both loopback or Tailscale, no forwarding headers, fail closed, residual risk documented |
| High: invite and serve may use different hub folders or ports | `token invite` prints the hub folder and says to use the same `--workspace` and `--addr` |
| High: no read deadline, directory scan per request | 5 second read deadline; pruning happens only in `Issue` and when an expired invite is seen |
| Medium: plan and code contradictions (Store methods, gate clock, `Source` accessor) | Store is `{Put, Claim}`; no new clock; add `Source.Token()` |
| Scope cuts accepted | No `Count`, `Prune`, pending cap or `--expires`; `userconfig` untouched (`hubtoken.Save` extracted from `Rotate`); `Redeemer` defined in `mcpserver` |
| Medium: code in the command line, IPv6 filename collisions, symlinked invites folder, client sending over plain LAN | Documented; `:` becomes `_` in file names; folder created 0700 and symlink refused; client refuses non-loopback, non-Tailscale hosts |

## Decisions still open (recommended answer first)

1. **Plain LAN addresses.** Recommended: refuse pairing there and keep `scp` for it. A `--allow-insecure` flag can be
   added later if someone needs it; adding it now widens the attack surface for little gain.
2. **Life fixed at 10 minutes** (no `--expires` flag in this release; add it only if someone asks). Recommended.
3. **Release number.** v0.6.6 is the latest tag. This feature, the earlier `serve --print` and the `update` fix
   ship together as **v0.6.7**, tagged by 68 on the user's instruction ("sau khi xong thì release luôn").

## Acceptance criteria

- [ ] `skillhub token invite` prints, per eligible address, one pasteable `connect --invite` line, then exits; it needs
      a terminal like `token rotate` does.
- [ ] `connect --hub <url> --invite <code>` saves the token under `userconfig.Dir()/hub-tokens/` with mode 0600 in a
      0700 folder and connects the project; the printed output never contains the token or the code.
- [ ] A second redeem of the same code, an expired code, an unknown code and a malformed code all fail with the same
      message; one code redeemed by 20 goroutines succeeds exactly once, including on the Windows CI job.
- [ ] `/mcp/pair` refuses a plain-LAN connection without consuming the invite, and the hub token is never accepted as an
      invite nor an invite as a bearer token.
- [ ] Existing connections that point at `~/.skillhub-token-*` files keep working unchanged.
- [ ] `gofmt`, `go vet` on Linux, Windows and macOS, `make check`, an independent security review with no open
      blocker, and green CI on all three operating systems before the release.
- [ ] `serve --print` is gone from the code, `help serve`, both READMEs, the guide and the command reference, with no
      leftover test or doc reference.
- [ ] Docs describe invite as the default way and `scp` as the fallback, in both READMEs and the guide.
