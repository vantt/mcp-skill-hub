# Red-team review: one-time invite plan

Plan: `plans/261014-1250-one-time-invite-for-remote-hub/`. Read-only review against the code at main (73c8f47).
User decisions (shared token, command names, `hub-tokens/<host>-<port>`, remove `serve --print`, release by 68) are not challenged.

## Critical

### C1. Windows: claim-by-`os.Remove` can fail for every concurrent redeemer, and the read-then-remove design is fragile
- Finding: phase-01 `Take` reads the file, closes, then removes. Go opens files on Windows without `FILE_SHARE_DELETE`, so while one `Take` holds the file open for reading, another caller's `os.Remove` fails with a sharing violation. The plan maps "failed remove" to "not present", so two simultaneous redeems (or a redeem racing the `Count`/`Prune` scan that also opens files) can both return false and the legitimate user gets a 403 for a valid code. On Unix it is also a read-then-remove pair that serves no purpose: the read only exists to fetch the expiry.
- Evidence: plan phase-01 "Take reads the file, removes it" and its Risk line ("os.Remove of an open file fails on Windows: Take reads fully and closes before removing") only addresses single-caller; the 20-goroutine test runs on all three OSes in CI and will be flaky on Windows.
- Fix: never open the file in `Take`. Store the expiry in the file's mtime (`os.Chtimes` at `Put`) or in the name (`<hash>.<unix-expiry>`), then `Lstat` + `os.Remove` only. Alternative: `os.Rename(file, file+".claimed-<rand>")` as the claim (atomic on all three OSes), then read and delete the renamed file. Treat only `fs.ErrNotExist` as "not present"; any other remove error is a 500 that does not burn the invite. Also write the file atomically in `Put` (write temp, then `os.Link`/`Rename`) so a crash or a concurrent reader never sees an empty file; `Count`/prune must treat an unparseable file as expired.

### C2. The invite is burned before anything that can fail without it is checked
- Finding: three paths burn the code and leave the user with nothing:
  1. Phase-04 order is "redeem, save, then `ConnectRemote`". But `ConnectRemote` validates after the token is needed: `NormalizeConnectHubURL`, root/scope resolution, `tokenFileUsedElsewhere`, host detection (connect_remote.go:137-165, 322-345). Any of these failing after redeem wastes the code.
  2. `Save` fails (disk, permissions, Windows AV lock) after redeem: the plan only covers "a later step fails" ("token saved at <path>").
  3. Server side: if the handler calls `Redeem` and then reads the token from `hubtoken.Source` and that read fails (file missing, mode wrong after a botched rotate), the invite is consumed and the reply is a 500. Same when the response write fails (client timeout).
- Evidence: phase-02 "Order inside the gate ... then `Service.Redeem`" and "reads the token ... at request time" do not say which comes first; phase-04 Requirements.
- Fix: (a) server: read and validate the token via the Source first, then `Redeem`, then write the response; (b) client: run all request validation that does not need the token before `PairHub` (split `ConnectRemote` into plan/validate and apply, or call `NormalizeConnectHubURL`, `checkScope`/root resolution and a dry plan first); (c) when `Save` fails after a successful redeem, the error must say "the invite was used; run `skillhub token invite` again" and name the cause; (d) add tests for each.

## High

### H1. Client ignores proxy settings: invite and the returned token can go through `HTTP_PROXY`
- Finding: plan.md says `PairHub` "uses `net/http` with a 10 s timeout, never follows redirects" but nothing about proxies. A default `http.Client` honours `HTTP_PROXY`/`ALL_PROXY`, so the code (request) and the shared hub token (response) transit a proxy. The repo already has the right client.
- Evidence: `internal/app/doctor_remote.go:136-141` (`hubProbeClient`: `transport.Proxy = nil`, `CheckRedirect: ErrUseLastResponse`) and its doc comment at :92-93.
- Fix: phase-04 must reuse `hubProbeClient()` (or a shared constructor) for `PairHub`; add a test with `HTTP_PROXY` set to a counting server.

### H2. Global failure budget lets anyone who can reach the route lock out pairing
- Finding: 10 failed redeems/min globally then 429 "even with a valid code" (phase-02 step 3). Anyone on the tailnet, any local user on the hub machine (127.0.0.1), or a browser page (a cross-origin POST without `Origin` is blocked, but a non-browser tool is not) can send 10 junk codes a minute and make `connect --invite` unusable indefinitely. The 60-bit code does not need this throttle (at 10/min brute force is already hopeless), so the throttle mostly buys a denial of service.
- Evidence: http.go:241-252 (existing throttle is per remote and only throttles bad tokens so one client cannot lock out others, the exact property the plan breaks); http.go:40-44 constants.
- Fix: raise the limit to a size where guessing is still infeasible and honest retries survive (for example 60 failures/min, 2^60 / 60 per min is still astronomically large), reuse `authFailures` with a fixed key instead of a new counter type, and count only failures after the transport check. Document that a locked route clears itself in one minute. Alternatively key by remote address except when the remote is loopback.

### H3. `/mcp/pair` transport check does not mean "encrypted path"
- Finding: `http.LocalAddrContextKey` tells the destination IP of the socket, not how the packet travelled. Cases verified against the design:
  - Direct, SSH tunnel (sshd connects to 127.0.0.1 or the hub IP), Tailscale peer: works.
  - TLS-terminating reverse proxy, `tailscale serve`, Tailscale Funnel, Caddy/nginx on the same host: local address is loopback, so pairing is allowed even when the original client is on the public internet. Over HTTPS that is fine, over a misconfigured plain-HTTP proxy it leaks code and token, and Funnel exposes the route to the internet.
  - A LAN host can send plain HTTP to this machine's Tailscale address (100.x) via eth0 when the kernel accepts it (weak host model, loose rp_filter): the destination is in 100.64/10 so the check passes with no encryption.
  - 100.64.0.0/10 is the general CGNAT range; an ISP-assigned address on a WAN interface also matches, so the range is not proof of Tailscale (same imprecision exists in `hostpolicy.label`, interfaces.go:54-73, but there it only labels).
  - IPv4-mapped IPv6 on `[::]` listeners: `net.IP.IsLoopback` and `tailscaleRange.Contains(ip.To4())` handle it, but a `netip` implementation must `Unmap()` (policy.go:62-64 shows the existing pattern); Tailscale's IPv6 range `fd7a:115c:a1e0::/48` is not covered and fails closed.
  - Missing context value or non-`*net.TCPAddr`: must fail closed.
- Evidence: phase-02 Requirements "Transport check"; hostpolicy/interfaces.go:54 (unexported, IPv4-only `tailscaleRange`).
- Fix: (a) refuse when `Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host` or `X-Real-IP` is present (cheap proxy detector; Funnel/serve and nginx all add them); (b) fail closed on any unexpected address type; (c) for the Tailscale branch also require that the remote address is itself in 100.64/10 or fd7a::/48 (a LAN peer reaching the 100.x destination will have a LAN source); (d) state the residual risk (CGNAT, proxies that strip headers) in the docs rather than "only over Tailscale or an SSH tunnel"; (e) test with IPv4-mapped values and a proxy header.

### H4. `token invite` and `serve` can disagree on the hub folder or port, and the failure is silent
- Finding: `token invite` writes to `<workspace>/runtime/config/invites` from `--workspace`/resolution, `serve` reads from its own workspace (service mode, a different CWD or `--workspace`, two hubs on one machine). The invite is then simply "not valid" with no clue. Likewise `--addr` defaults to `0.0.0.0:7420` and only picks the printed URLs, so a server on another port gets a connect line that points nowhere.
- Evidence: phase-03 Requirements ("`--addr` ... pass the same `--addr` the server uses"; "If the server is not running it still issues the invite"); serve.go:252 resolves the token path from the workspace.
- Fix: print the hub folder in the invite output ("Invite stored in <hub folder>; the running `skillhub serve` must use this folder"); make the server's 403 generic text on the client include "check that the hub machine's server uses the same hub folder"; optionally let `token invite` probe `http://127.0.0.1:<port>/mcp/status` unauthenticated? (it needs the token, so skip) and instead have `serve` write nothing extra. Keep it to wording plus the folder line.

### H5. Slow-body and per-request cost on an unauthenticated route
- Finding: `http.Server` has only `ReadHeaderTimeout` (http.go:104-110); no read timeout. `MaxBytesReader` caps size but a client that trickles the body holds a goroutine and the in-flight counter (http.go:229-232, used by shutdown drain) for as long as it likes, delaying graceful shutdown by the full drain. Also the plan says "Prune on every issue and redeem": an unauthenticated request must not trigger a directory scan.
- Evidence: phase-02 (no deadline mentioned); plan.md Risks row "Leftover invite files ... Prune on every issue and redeem".
- Fix: in the handler call `http.NewResponseController(w).SetReadDeadline(now+5s)` before reading the 1 KiB body; prune only in `Issue`, never in `Redeem`.

## Medium

### M1. Plan documents contradict each other
- `Store` is `{Put, Take, Prune}` in plan.md (Hexagonal table) but `{Put, Take, Count}` in phase-01. Pick one.
- plan.md "Acceptance criteria": a malformed code "fails with the same message" as unknown/expired/used; phase-04 rejects a malformed code locally with a different message, and the server answers 400/403/429/404 with different texts. Reword: same message for the three server-side invalid cases only.
- phase-02 says the gate's "injectable clock"; `httpGate` has no clock, only `newAuthFailures(time.Now)` (http.go:209). Tests must build the gate with a fake clock through `newAuthFailures`, which the plan should name.
- phase-02 "reads the token through the existing `hubtoken.Source`": `Source` has no accessor for the token, only `Valid`, `Check`, `Path` (hubtoken.go:244-260). Add `Source.Token() (string, error)` (reusing `current()`), and list `internal/hubtoken` in phase-02 files (phase-02 currently says "Nothing else").
- phase-04 says "host sanitised like `ClientPath` does", but the sanitising is inline in `ClientPath` and returns a `~/.skillhub-token-` string (hubtoken.go:53-76). Extract the sanitiser; note it drops colons, so `[::1]` and `1`, or `2001:db8::1` and `2001:db81` map to the same file. Add a short hash suffix for IPv6/odd hosts or refuse to merge on collision.

### M2. Code visible in argv on the agent machine
- `connect --invite K7QF-...` shows in `ps` and shell history; on a shared agent machine another local user can redeem first and get the shared token. Single-use limits the history leak, not the race.
- Fix: also accept `--invite -` (stdin) or `SKILLHUB_INVITE` env, document that the printed line is for a personal machine; keep the one-line paste as the default.

### M3. Client-side downgrade guard missing
- The server refuses plain LAN, but the client sends the code to whatever URL the user pastes (a DNS name over the internet, a LAN IP, a misspelled host). The code and the response travel in clear before the server can refuse. Fix: in `PairHub`, before sending, refuse `http://` to hosts that are not loopback or in 100.64.0.0/10 (allow `https://` anywhere), with a message pointing to `scp`.

### M4. Directory creation and mode of the invites folder
- `os.MkdirAll(dir, 0700)` does not tighten an existing directory; `runtime/config` already exists (hubtoken.DefaultPath) and may be 0755. `Put` should `Lstat` the invites dir, refuse a symlink or a dir writable by others (Unix), and `Chmod` 0700 if it created it. Filename is `hex(sha256)`: no traversal risk, but state that the code must be parsed to 12 chars before hashing. `Count` must return 0 when the folder is missing.

### M5. Error and status oracle is wider than "one generic failure"
- 429 (budget exhausted) vs 403 tells an observer when someone is probing; 400 for bad JSON vs 403 for well-formed vs 404 when disabled are distinguishable by a non-holder. None leaks a valid code, so this is Medium: say so in the plan, count 400s toward the budget, and keep identical body/headers/status for the three invalid cases. Timing differences (expired file removed vs absent) are negligible and need no work; constant-time compare is unnecessary because the lookup key is a SHA-256 of the guess.

### M6. Re-pair overwrites a file other projects use
- `Save` replaces `hub-tokens/<host>-<port>` in place; every project already pointing at it picks up the new token on next start, which is wanted after a rotation but also means a stale token cannot be rolled back. Fine, but connect should print "replaced the saved token for <host>:<port>" when overwriting. The doctor hint `tokenCopyFix` (scp text, doctor_remote.go:246) should also mention `skillhub connect --hub <url> --invite <code>` for a missing file under `hub-tokens`.

### M7. Multiple servers on one hub folder, restarts, clocks
- Two `serve` processes on one hub folder both see the same invites: the rename/remove claim is atomic per filesystem, so single use holds (no work needed on a local FS; NFS/SMB do not guarantee it, document "hub folder on a local disk"). Restart and service mode lose nothing (state is on disk). Expiry uses the wall clock of two processes on one machine, so skew is zero; a suspend/resume or manual clock change can expire an invite early, which is the safe direction.
- Windows: `0700/0600` are no-ops; the file inherits the ACL of `%AppData%` (per user), as the plan says.

### M8. Seams the plan assumes, verified
- `app.Clock` exists (internal/app/clock.go): but `internal/invite` must not import `internal/app` (plan says stdlib only), so it needs its own one-method `Clock`; say so rather than "reuse `app.Clock` shape" being read as an import.
- `HTTPOptions` exists (http.go:53-66) and can take a field; path allowlist is the single condition at http.go:225 and bearer check at :241: adding the pair branch before :241 needs its own copy of the shutdown-closing check (:233) and `inFlight` accounting (:229) to stay inside the drain. Say so in phase-02.
- Doctor does read the token path from the connection entry (`ReadHubToken(hubURL, tokenFile)`, doctor_remote.go:451) so no doctor change is needed.
- `userconfig.Dir()` exists and honours `SKILLHUB_USER_CONFIG_DIR` (userconfig.go:49-58).

## Scope, YAGNI and layering

Minimum viable cut (about 5-6h instead of 10h):
1. `internal/invite`: `code.go` (generate, parse, hash) and one file store with `Issue`, `Redeem`. Drop the `Store` interface and `Count`/`Prune`/`maxPending`: the only issuer is the hub owner at a terminal (so nobody can fill the 10 slots remotely; `/mcp/pair` can only call `Redeem`), there is one adapter, and a 10-invite cap protects against the owner. Prune expired files inside `Issue` only. Keep the injected clock and random reader for tests; a `Service` struct is fine, a port interface is gold plating.
2. Pair handler plus gate branch; reuse `authFailures`, no new budget type.
3. `token invite`; remove `serve --print`.
4. `connect --invite`.
5. Drop as gold plating: Crockford `I/L/O` aliasing and space/dash tolerance beyond stripping dashes and case-folding (keep if cheap, but do not test a table of it); `--expires` flag (fixed 10 minutes; revisit on demand); `Redeemer` interface exported from `invite` (define it in `mcpserver`, the consumer); a separate `hubtoken.ClientStore` type (two functions `ClientTokenPath` and `Save` suffice).
6. Do not touch `userconfig` for the atomic write: `hubtoken.Rotate` already holds the same temp-fsync-rename code (hubtoken.go:140-180). Extract `hubtoken.Save(path, token)` from it and have `Rotate` call it. This removes the phase-04 edit to `internal/userconfig` and its regression risk.
7. Phases 2, 3, 4 "in parallel in worktrees" is fine; phase 3 and 4 both edit `internal/delivery/cli` (`help.go`, tests) so give `help.go` to phase 3 only and have phase 4 append after merge.

Layering: `internal/invite` importing stdlib only is right; `mcpserver` importing `invite` is inward-pointing and fine. `PairingAllowed(net.Addr)` in `hostpolicy` is acceptable only because the Tailscale range lives there (unexported at interfaces.go:54); the package doc ("Host header values, banner addresses, warning") should be extended, and the function must take the remote address too if H3(c) is adopted. The shared address list extracted from `writeMCPBanner` (phase-03) belongs in `hostpolicy` or `cli`, not `invite`; keep `kind` (loopback/tailscale/lan) computed from existing `IsLoopback`/`label` rather than a second range check. The `preview`/`tokenMissing` banner fields (serve_banner.go:28-33) disappear with `--print`, as planned.

Lightness: three code paths touch the file system per redeem in the plan (read, remove, prune/count); after C1 and the cuts it is one `Lstat` and one `Remove`.

## Summary of fixes to apply to the plan
1. Rewrite `Take` to claim without opening the file (C1).
2. Reorder server (token first) and client (validate before redeem), add post-redeem failure messages (C2).
3. `PairHub` reuses `hubProbeClient` (H1).
4. Budget: higher, reuse `authFailures`, count after transport check (H2).
5. Transport: proxy headers, remote-address check, fail closed, honest docs (H3).
6. Hub-folder line in invite output (H4); body read deadline, prune only on issue (H5).
7. Fix the contradictions and missing seams in M1; adopt the YAGNI cuts above.

Status: DONE
