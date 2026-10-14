# Review: one-time invite implementation

Scope: `git diff 32db29d..HEAD` (10 commits, 34 files, +2643/-228) at a286e85. This review only read code. It ran `go vet` and
`go test -race -count=1` on `internal/invite`, `internal/delivery/{mcpserver,hostpolicy,cli}`, `internal/hubtoken`
and `internal/app`, and all of them passed. No token or invite value was printed.

Overall: the design is sound and the red-team critical items are fixed in code. Nothing leaks the token or the
invite through logs, errors, JSON output or `.mcp.json`. The server checks the transport first, reads the token
before it redeems, and the claim is a real single-use rename. **No blockers.** One high finding: the failure
throttle is shared with the bearer check and keyed by remote address. Over an SSH tunnel every client is
127.0.0.1, so this locks pairing out on the SSH-tunnel path, one of the two documented transports. The phase-5 docs
(residual risks, the code showing in the process list) are not written yet, so release must wait for them.

## Red-team findings: verified status

| Item | Status | Evidence |
|---|---|---|
| C1 Windows claim / no open | Fixed. Residual is Low L1 | `internal/invite/filestore.go:98-114`: Stat then Rename, never opened. `Put` uses temp, Chtimes, Rename (`:55-75`) |
| C2a token read before redeem | Fixed | `internal/delivery/mcpserver/http_pair.go:75-84`. Test `TestPairUnreadableTokenLeavesInviteUnused` |
| C2b client validates before redeem | Partial | `connect_remote.go:120-146` checks the URL, the host, the destination and writability. Scope and target resolution and `tokenFileUsedElsewhere` (`internal/app/connect_remote.go:149-155`) still run after redeem. This is softened: the token is already saved and `noteSavedToken` prints the command to run again (`connect_remote.go:164-178`). See M4 for the one case where the order does real harm |
| C2c Save failure message | Fixed | `connect_remote.go:155-157` |
| H1 proxy | Fixed | `pair_hub.go:40` uses `hubProbeClient()` (`doctor_remote.go:133-141`: `Proxy=nil`, no redirects). Test `TestPairHubIgnoresProxySettings` |
| H2 throttle DoS | **Partial, see H1 below** | per remote, counted after the transport check, but shares one counter with the bearer check |
| H3 transport | Fixed in code (a, b, c, e); docs (d) pending | `hostpolicy/pairing_request.go:137-158` checks both ends, fails closed and refuses forwarding headers |
| H4 hub folder mismatch | Partial | the invite output prints `Hub folder` (`token.go`, runTokenInvite). The client's 403 text gives no hint (`pair_hub.go:69-70`) |
| H5 slow body / prune | Fixed | `http_pair.go:57` sets a 5 s read deadline. Prune runs only in `Put` (`filestore.go:73`) |
| M2 code shows in argv | Not addressed, not documented yet | see M5 |
| M6 overwrite notice | Not addressed | see M4 |

## Blocker

None.

## High

### H1. Pairing failures share the bearer-auth throttle, keyed by remote address, so the SSH-tunnel path can be locked out indefinitely
- Evidence: `http_pair.go:51-56` checks `gate.failures.blocked(remote)`, and `:81` records into the same `authFailures`
  map that the bearer path fills at `http.go:254-261` (limit 20 per minute, `http.go:42-43`). The key is
  `remoteHost(request.RemoteAddr)`. Behind `ssh -L` every client, and every local user of the hub machine, is
  `127.0.0.1`.
- Effect: after `skillhub token rotate`, agents still running with the old token keep getting 401 from 127.0.0.1.
  Each 401 is recorded, so 20 per minute keeps `127.0.0.1` blocked. The natural fix is to pair again with
  `connect --invite` through the tunnel, and that request gets 429 every time. Any local user on the hub machine
  can get the same result on purpose with 20 junk invites a minute. The bearer path does not suffer this, because a
  valid token skips the throttle. The pairing path has no such escape. This is the lockout the red-team H2 warned
  about, still present for one of the two transports the feature advertises.
- Fix: give pairing its own namespace in the counter, for example `gate.failures.blocked("pair|" + remote)` and
  `record("pair|" + remote)`. Bearer failures then never block pairing. 2^60 codes against 20 guesses a minute is
  still out of reach, even with all loopback clients sharing one budget. Add a test: 20 bad bearer requests from
  127.0.0.1, then a valid invite from 127.0.0.1 returns 200.

## Medium

### M1. Client reports a Host- or Origin-refused request as "invite not valid"
- Evidence: the gate refuses an unknown Host with 403 and plain text before the pair handler runs (`http.go:218-226`).
  `PairHub` treats every 403 that lacks "Tailscale or an SSH tunnel" as an invalid invite (`pair_hub.go:67-70`).
  Example: `--hub http://box.tailnet.ts.net:7420` without `--allow-host` on the server. The user is told the invite
  expired, was used or is wrong, and to make a new one. The invite was never touched, and the real fix is
  `--allow-host`.
- Fix: match the server's JSON body `{"error":"invite not valid"}` exactly for the invalid-invite case. Map a 403
  that mentions the Host header to "add this name with `skillhub serve --allow-host <name>`, or use the Tailscale IP".
  Add both to `TestPairHubStatusMessages`.

### M2. `skillhub serve` now refuses to start because of the optional invite folder
- Evidence: `serve.go:188-198` makes `invite.NewFileStore` failure fatal. `NewFileStore` refuses an existing folder
  with any group or other bits (`filestore.go:43`), and `MkdirAll` fails on a read-only `runtime/config`.
- Effect: a folder made by hand, restored from a backup, or created under a loose umask by older tooling stops the
  whole network listener, and that includes agents that never pair.
- Fix: on error, print a warning ("invites are off: <reason>; fix with `chmod 700 <dir>`") and pass
  `Invites: nil` (the route then answers 404). Alternatively `Chmod 0700` a folder this user owns before refusing it.

### M3. DNS can be rebound between `checkPairingHost` and the request
- Evidence: `connect_remote.go:97-108` resolves the name once to check it. `PairHub` then dials through
  `http.DefaultTransport`'s dialer, which resolves again (`pair_hub.go:40-48`, `doctor_remote.go:134`). The
  `localhost` shortcut (`:94-96`) skips the check altogether.
- Effect: anyone who can spoof or rebind DNS for the name (unencrypted DNS on a LAN) gets the invite sent to a LAN
  or public address in clear text. The impact is limited: a stolen code only helps someone who can reach the
  pairing route over Tailscale or loopback, and the bearer flow already trusts the same resolution. It still breaks
  the stated rule that the code is "never sent where it was not checked".
- Fix: check the address actually dialed. Give the client in `PairHub` a transport whose
  `net.Dialer.Control` (or `DialContext`) parses the dialed `address` and refuses unless
  `hostpolicy.PairingIP(ip)`. Keep `checkPairingHost` for the early, friendly message. Add a test with a dialer
  that resolves to a LAN IP.

### M4. The client token file is keyed only by host and port: collisions and silent overwrite
- Evidence: `client_store.go:28-44` drops every character except `[a-z0-9.-]` and maps `:` to `_`. As a result
  `a_b` and `ab` share a file, as do the zone in `fe80::1%eth0` and its plain form, and `http://h:443` and
  `https://h` both become `h-443`. With the default destination there is no existence check
  (`connect_remote.go:139-143` checks only for an explicit `--token-file`). `Save` runs before
  `tokenFileUsedElsewhere` (`connect_remote.go:155` versus `app/connect_remote.go:153`). So when two different hub
  URLs collide, the new token overwrites the file another project uses for the other hub, and only then does the
  connect refuse. That other project then sends this hub's token to the wrong hub, which is exactly what
  `tokenFileUsedElsewhere` exists to prevent. The common case needs no odd names: two hubs both reached through
  `ssh -L 7420:...` share `http://127.0.0.1:7420`, so pairing the second one silently replaces the first one's
  token.
- Fix: (a) before `PairHub`, call the same `tokenFileUsedElsewhere` check (export it or add a `CheckTokenFileFor`)
  against the destination. (b) When the default file already exists, print "replaced the saved token for
  <host>:<port>" after saving (red-team M6). (c) Append a short hash of the normalised hub URL when the sanitised
  host differs from the original host. (d) In the remote guide, tell users to give each hub its own local tunnel
  port.

### M5. Docs and residual-risk statements not written; the code shows in the process list on the agent machine
- Evidence: `grep -rn "token invite\|--invite" docs README*.md` finds nothing. Phase 5 is still pending. The plan
  promises documented residual risks: the CGNAT range, a local user on the hub, and proxies that strip headers. The
  code is passed in argv (`connect.go` `--invite`), so on a shared agent machine another user can see it in `ps` or
  `/proc/<pid>/cmdline` during the short window before it is redeemed, and it stays in shell history.
- Fix: write the phase-5 docs before tagging a release. State: pairing works only to the Tailscale IP or a tunnel,
  not through `tailscale serve` or Funnel; the CGNAT caveat; the argv and history exposure ("use on a personal
  machine"). Consider `--invite -` (read from stdin) as red-team M2 proposed.

### M6. A pairing request that times out after the redeem tells the user the invite is still valid
- Evidence: `pair_hub.go:51-54` gives every transport error the fix "The invite is still valid until it expires".
  The server redeems before it writes the reply (`http_pair.go:80-85`), so a reply lost or cut off at 10 s by the
  client timeout leaves the invite used up.
- Fix: say "If the hub received the request, the invite may be used up; run `skillhub token invite` again if this
  command reports it is not valid."

## Low

- **L1. Claim maps every error to "not valid".** `filestore.go:100-111` returns `(false, nil)` for any `Stat` or
  `Rename` error. A transient Windows sharing violation (antivirus) or an EACCES error gives a 403 "not valid",
  counts as a failure, and tells the user to make a new invite while the old one is still pending. Return `false`
  only for `fs.ErrNotExist`. Return other errors and answer 503 without recording a failure (red-team C1 asked for
  this).
- **L2. Leftover files are never pruned.** `prune` only looks at names of 64 characters (`filestore.go:86`). Any
  `<hash>.claimed` left after a crash between rename and remove, and any `.tmp-*` left after a crash in `Put`, stay
  forever. Remove `.claimed` and stale `.tmp-*` files older than `Life` in `prune`.
- **L3. Forwarding-header list.** `pairing_request.go:13`. Also add `Tailscale-User-Login` and
  `Tailscale-Funnel-Request` (headers Tailscale's own proxy adds), plus `X-Forwarded-Port`, `X-Forwarded-Server`,
  `True-Client-Ip` and `Cf-Connecting-Ip`. That is cheap defence in depth for a same-host proxy configured to send
  only those headers. A plain nginx `proxy_pass` adds none of these headers by default. The client-side address
  check (`checkPairingHost`) is what keeps honest clients off such a path, so say so in the docs.
- **L4. Throttle does not count 400 or 413 responses** (`http_pair.go:58-74`). That is harmless (no code is
  checked) but contradicts red-team M5. Accept it, or count them.
- **L5. The banner hint drops flags.** `serve_banner.go:73` prints a bare `skillhub token invite` even when the
  server runs with `--workspace` or a non-default `--addr`. That is the mismatch red-team H4 warned about. Build
  the hint with `tokenInviteCommand(tokenConfig{workspacePath, addrFlag})`.
- **L6. The rerun command is not quoted.** `noteSavedToken` (`connect_remote.go:165`) does not shell-quote
  `flags.hub`. Use `shellquote.Arg`.
- **L7. Symlink behaviour is safe but undocumented.** An existing `--token-file`, including a dangling symlink, is
  refused (`connect_remote.go:139-143`, using Lstat). A symlink at the default path is replaced by a regular file
  (rename does not follow it), and its target is untouched. That is correct. A symlinked parent folder is followed.
  No path traversal is possible: `/`, `\` and `..` segments cannot survive the sanitiser, because a lone `..`
  becomes the filename `..-<port>`.
- **L8. Phantom assertion.** `TestPairDoesNotMixTokenAndInvite` (`http_pair_test.go:138-159`) and the network test
  (`:333`) check "token as invite → 403" against `fakeRedeemer`, which refuses anything unlisted, so the assertion
  proves nothing about the real `Service`. The property is actually enforced and tested by `ParseCode` rejecting
  more than 12 characters (`code_test.go:36`, case "K7QF-9M2X-4TBDD"). Add one `Service.Redeem(<64-hex>)` case in
  `service_test.go`, and stop presenting the gate test as proof of it.

## Requested checks

1. **Leaks.** None found. The server never logs the body. Error texts are fixed strings (`http_pair.go:48-85`). The
   gate does not log requests. `PairHub` errors carry the URL and status, never the body or the code
   (`pair_hub.go:44-78`). `Save` errors carry no token. The JSON output and `.mcp.json` hold only the path
   (checked by `TestConnectInviteSavesTheTokenAndConnects`). `token invite` prints only to a terminal. Remaining
   exposure: argv and shell history on the agent machine (M5), and agent transcripts when an agent runs the
   command.
2. **Single use.** Rename is atomic on Linux, macOS and Windows, and exactly one caller wins
   (`TestFileStoreConcurrentClaimHasOneWinner`, 20 goroutines). Two processes cooperate
   (`TestFileStoreSharesInvitesAcrossInstances`). Expiry is checked against the mtime before the rename. The window
   between Stat and Rename is microseconds, which is acceptable. On Windows nothing holds a handle, so the only risk
   is antivirus locks (L1), which fail safe: the invite is not burned. NFS and SMB are not guaranteed; the docs
   should require a hub folder on a local disk.
3. **Token and invite mix-up.** Not possible. A 64-hex token fails `ParseCode` (`code.go:61`, length). An invite
   fails `validToken` (12 characters, under the 32 minimum, `hubtoken.go:30`) and never equals the token. Pairing
   ignores `Authorization`.
4. **Transport.** IPv4-mapped addresses are unmapped (`pairing.go:11`, `pairing_request.go:39-40`). IPv6 Tailscale
   and link-local addresses are refused (fail closed). A missing or non-TCP address is refused. Both ends are
   required, which blocks a LAN source sending to the 100.x address. Header names are canonicalised. Behind
   `tailscale serve` or Funnel, the proxy adds `X-Forwarded-*` and rewrites Host, so pairing is refused (fail
   closed); this must be documented (M5, L3). A same-host proxy that strips every header passes as loopback, so
   the client-side check is the second guard. The Host and Origin checks still run (`TestPairKeepsHostAndOriginChecks`).
   The CGNAT ambiguity remains (docs).
5. **Client.** Proxy settings are ignored and redirects are not followed (tested). DNS rebinding is possible (M3).
   `Save` writes a temp file, fsyncs and renames it, with mode 0600 and a 0700 folder; it does not tighten an
   existing folder. An existing `--token-file` is refused, but the default path is overwritten silently (M4). Host
   names cannot traverse paths (L7).
6. **DoS.** An unauthenticated tailnet peer can, per its own IP, make 20 redeem attempts a minute. It can also send
   unlimited cheap 400 requests: each is one small read with a 5 s deadline, no file system access, and counted in
   `inFlight`. It cannot exhaust anything new beyond the existing connection limits. Over loopback the shared key
   lets anyone lock pairing out (H1).
7. **Token rotated or unreadable.** `Source.Token` re-reads the file after a rotation. An unreadable file gives 503
   before the redeem, so the invite survives (tested). A rotation between the read and the redeem returns the old
   token, a window of microseconds. Client copies go stale after a rotation until the machine pairs again, which
   then overwrites the default file (M4b).
8. **`serve --print` removal.** Clean. No `printConnect`, `runServePrint`, `preview` or `tokenMissing` remains in
   code. README, README.vi, the command reference, the guide and help no longer mention it. A test guards help
   (`token_invite_test.go:176`). The plan files still mention it, which is expected.
9. **Test mutations (reasoned).**
   - Swap the token read and the redeem in `servePair` → `TestPairUnreadableTokenLeavesInviteUnused` fails because
     the redeemer is called. Strong.
   - Drop the `os.Rename` claim (Stat, then return true) → `TestFileStoreConcurrentClaimHasOneWinner` sees 20
     winners, and `TestFileStoreClaimTwice` fails too. Strong. Replacing the rename with `os.Remove` passes
     everywhere: the behaviour is equivalent on POSIX, and the Windows open-handle case has no test.
   - Drop the remote-end check or the header loop in `PairingAllowed` → the "lan remote" and header cases in
     `TestPairTransportMatrix` and `TestPairingAllowed*` fail. Strong. Gaps: no gate-level IPv4-mapped Tailscale
     case (covered only in `TestPairingIP`), no test for bearer failures blocking pairing (H1), and the phantom
     assertion in L8.

## Recommended order
1. H1 (separate throttle key) plus a test.
2. M1, M2, M4(a,b), M6: small changes to messages and ordering.
3. M3 (check at dial time).
4. M5 docs before the release tag.
5. The Lows as time allows.

## Unresolved questions
- Should loopback pairing be allowed for every local user of the hub machine (a documented residual risk), or only
  when the request arrives on a loopback listener? That is a product decision.
- Is `tailscale serve` (HTTPS on the tailnet) a deployment users expect to pair through? If so, the header refusal
  needs an opt-in, for example `--allow-pair-proxy`.

Status: DONE_WITH_CONCERNS
