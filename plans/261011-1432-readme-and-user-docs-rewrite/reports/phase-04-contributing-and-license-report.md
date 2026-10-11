# Report: CONTRIBUTING.md and LICENSE

## Files

- `CONTRIBUTING.md` (new, 53 lines, budget 60). H2s match the frozen map: Build and test, Keep tests isolated,
  Release, Design records.
- `LICENSE` (new). SPDX MIT text unchanged except the line `Copyright (c) 2026 Van Tran`.

## Verification

- Every Makefile target named exists: `check`, `test-race`, `lint-all`, `web-check`, `web-e2e`, `web-shots`.
  Each one-line description comes from the `## <target>:` comment in the Makefile (`check` expanded from its
  prerequisites `fmt-check vet lint test` and the `lint` comment's `LINT_BASE ?= origin/main`).
- Go 1.26 from `go.mod` (`go 1.26.0`); Node.js 24 from `web/.nvmrc` (also used by CI via `node-version-file`).
- `TestMain` HOME/USERPROFILE/XDG isolation confirmed in `internal/{app,delivery/cli,delivery/mcpserver,delivery/web,hostintegration}/main_test.go`;
  the `GOCACHE`/`GOMODCACHE`/`GOPATH` fill from `go env -json` is in the same files.
- Parallel and serial rule matches `AGENTS.md` lines 42-43.
- `skillhub help` ran with the sandbox binary. `make check` was not run (left to the automated verification phase).

## Notes

- `docs/README.md` does not exist yet; the docs index phase creates it. The link resolves once that lands.
- `web-dev` has no Makefile comment and is not listed (not required).

## Owner to confirm

- LICENSE copyright holder "Van Tran" (taken from the git author). Confirm, or give the legal name or entity.

## Unverified

- none

Status: DONE
Summary: CONTRIBUTING.md and MIT LICENSE written and verified against the Makefile, go.mod, web/.nvmrc and the TestMain files.
