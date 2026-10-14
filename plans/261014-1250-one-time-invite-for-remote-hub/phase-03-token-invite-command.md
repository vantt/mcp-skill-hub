# Phase 3: token invite, the shared address list, and removing serve --print

## Context
Plan: [plan.md](plan.md). Depends on phase 1. Independent of phases 2 and 4 (different files).
`serve --print` (commit 73c8f47, unreleased) is removed here because `token invite` replaces it; the user asked for
that on 2026-10-14.

## Requirements
- `skillhub token invite [--workspace <p>] [--addr <host:port>[,...]]`:
  - Needs a terminal like `token rotate` (it prints a credential-like code); otherwise refuses and says why.
  - The invite lives 10 minutes (fixed; no `--expires` in this release).
  - `--addr` has the same meaning as for `serve`; the default is `0.0.0.0:7420`. It only decides which addresses are
    listed. State this in the help text: pass the same `--addr` the server uses.
  - Calls `invite.Service.Issue` on `<workspace>/runtime/config/invites`, then prints the expiry time and, per
    eligible address, one line: `skillhub connect --hub http://<addr>:<port> --invite <CODE>`.
  - Eligible = loopback (with a one-line SSH tunnel hint, as the serve banner prints today) and Tailscale. Other
    addresses are not listed; one line says pairing needs Tailscale or an SSH tunnel and that `scp` still works.
  - Prints the hub folder the invite belongs to and a line telling the user to run it with the same `--workspace` and
    `--addr` the server uses, plus a note that the code is not the hub token, works once, and that the command can be
    run again for a new one.
  - Does not create the hub token and does not start anything. If the server is not running it still issues the
    invite and says "start `skillhub serve`".
- Shared address list: extract from `writeMCPBanner` a pure function that turns bound addresses plus interfaces into
  a list of targets (`label`, `machine`, `hubURL`, `kind` loopback or tailscale or lan). The serve banner, and the
  new command, both use it. The serve banner also gains one line pointing to `skillhub token invite`.
- Remove `serve --print`: the `printConnect` field and flag, `runServePrint`, the `preview` and `tokenMissing` banner
  fields, the help text entries, the README and guide mentions, and `serve_print_test.go`. Keep any generally useful
  test of the shared function.
- `runToken` accepts `invite` next to `show` and `rotate`; help text for `token` and the top-level overview lists it.

## Files
`internal/delivery/cli/token.go`, `serve.go`, `serve_banner.go`, `help.go`, `serve_print_test.go` (delete), new
`token_invite_test.go`; `docs/command-reference.md` and `docs/remote-hub-guide.md` only to delete the `--print` text
(phase 5 writes the invite text).

## Steps
1. Extract the target list from the banner behind a failing-then-passing test (existing banner tests must stay
   green unchanged; that proves the extraction).
2. Add `token invite` with tests: output for Tailscale plus LAN plus loopback fixtures, refuses
   without a terminal, prints the hub folder, never prints the hub token.
3. Delete `serve --print` and everything that only existed for it; `git grep -n "print" -- internal/delivery/cli/serve*`
   and `git grep -n "serve --print"` must come back clean.
4. `gofmt`, `go vet` for three platforms, `go test ./internal/delivery/cli -count=1`.

## Validation
`token invite` against a temp hub prints the lines and leaves one file in `runtime/config/invites`; `help token`,
`help serve` and the overview are correct; `make check` passes.

## Risk and rollback
Removing `--print` must not break the serve banner: the banner tests are the guard. Rollback is `git revert` of the phase.
