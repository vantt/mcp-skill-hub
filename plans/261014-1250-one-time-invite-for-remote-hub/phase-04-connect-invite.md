# Phase 4: connect --invite and the client token store

## Context
Plan: [plan.md](plan.md). Depends on phases 1 and 2. Scout notes: `ConnectRemote` in `internal/app/connect_remote.go`
runs `NormalizeConnectHubURL`, then `checkTokenFile`, then the host changes, then `ProbeHub`; the flags are parsed in
`internal/delivery/cli/connect.go`.

## Requirements
- `connect --hub <url> --invite <code> [--token-file <dest>]`: `--invite` requires `--hub`; refused with
  `--dry-run` ("a dry run would use up the invite"), with `--remove`, and without `--hub`. A malformed code is rejected
  locally before any network call. The client also refuses to send an invite to a URL whose host is neither loopback
  nor in `100.64.0.0/10` (fail closed: the code must not cross a plain LAN), and checks that the destination folder
  can be created and written **before** the request, so a later failure cannot waste the code.
- `app.PairHub(ctx, hubURL, code) (token string, err error)` next to `ProbeHub`, built on `hubProbeClient()` (no proxy,
  no redirects, so a proxy never sees the code or the token): `POST <url>/mcp/pair`, JSON body, 10 second timeout, response body limited to 4 KiB, status mapping to
  messages: `403` "this invite is not valid: it expired, was already used, or is wrong. On the hub machine run
  `skillhub token invite` for a new one", `403` with the transport message passed through, `404` "this hub does not
  offer invites; use the scp line from `skillhub serve`", `429` "the hub is refusing attempts for a minute", network
  errors reuse the existing "nothing answers" text. The token is validated with `hubtoken`'s own format check.
- `hubtoken.ClientStorePath(configDir, hubURL)` returns `<configDir>/hub-tokens/<host>-<port>`; the host keeps letters,
  digits, dots and dashes and turns `:` into `_` with the IPv6 brackets dropped, so `::1` and `1` never collide.
  `hubtoken.Save(path, token)` is extracted from the temp-file, chmod 0600, fsync, rename code in `Rotate` (not
  copied, and `userconfig` stays untouched); it creates the folder 0700 and overwrites an older copy (re-pairing
  after a rotation).
- After a successful save, the normal `ConnectRemote` flow runs with `TokenFile` set to the saved path, so the host
  files, the cache folder and the final reachable-and-accepted check do not change.
- If a later step fails, the error says the token was saved at `<path>` and gives the rerun command
  `skillhub connect --hub <url> --token-file <path>`.
- Output on success includes "Invite accepted. Hub token saved to <path>" and never the token or the code. The path is
  printed as an absolute path.
- Existing `~/.skillhub-token-*` connections are untouched; `ClientPath` stays for the `scp` lines.

## Files
New `internal/app/pair_hub.go` and test; `internal/hubtoken/client_store.go` and test; edit
`internal/hubtoken/hubtoken.go` (extract `Save` from `Rotate`), `internal/app/connect_remote.go` (call point),
`internal/delivery/cli/connect.go` and `connect_remote.go` (flags, messages), their tests; `help.go` entry for
`connect`.

## Steps
1. `ClientStorePath` and `Save` tests: path from a fixed config dir (the platform dir comes from `userconfig.Dir()` at the call site), atomic overwrite, modes (not
   asserted on Windows), host sanitising for IPv6 and odd names.
2. `PairHub` tests against `httptest` servers: success, each status code, redirect refused, oversized body, timeout.
3. Wire `connect --invite`; CLI tests with the in-process server from phase 2 and a temp `SKILLHUB_USER_CONFIG_DIR`:
   the whole path (issue, redeem, file written, project connected, reachable and token accepted); a second run with
   the same code fails and does not touch the saved file.
4. Check that nothing in the output or in `.mcp.json` contains the token or the code.
5. `gofmt`, `go vet` three platforms, `go test ./internal/... -count=1`.

## Validation
`go test ./internal/app ./internal/hubtoken ./internal/delivery/cli -race -count=1`.

## Risk and rollback
Extracting `Save` from `Rotate` touches the rotate path: the existing `hubtoken` tests are the guard. Rollback is `git revert`.
