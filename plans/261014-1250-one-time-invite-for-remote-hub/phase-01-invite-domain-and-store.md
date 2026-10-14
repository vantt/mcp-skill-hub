# Phase 1: Invite domain and file store

## Context
Plan: [plan.md](plan.md). Pure logic plus one filesystem adapter. No networking, no CLI.

## Requirements
- `Code`: 12 Crockford base32 characters from `crypto/rand` (60 bits). `NewCode(rand io.Reader)`, `Format()` groups
  of four with dashes, `ParseCode(string)` that upper-cases, drops dashes and spaces, maps `O`→`0` and `I`/`L`→`1`,
  and rejects anything that is not exactly 12 valid characters. `Hash()` is SHA-256 of the 12 canonical characters.
- `Service{store Store, now func() time.Time, rand io.Reader}`:
  - `Issue() (Code, time.Time, error)`: fixed life of 10 minutes; the store drops expired invites first.
  - `Redeem(code string) error`: parse; on any problem return the one `ErrInvalid`; otherwise `store.Claim`.
- `Store` interface (two methods): `Put(hash [32]byte, expires time.Time) error` (also removes expired invites) and
  `Claim(hash [32]byte, now time.Time) (bool, error)`.
- `FileStore{dir}`: one empty file per invite named `hex(hash)` whose **mtime is the expiry**. The constructor creates
  the folder 0700 and refuses a symlink or a folder with group or other permissions (skipped on Windows). `Put`
  writes a temp file in the folder, sets the mtime, chmods 0600 and renames it into place. `Claim` does `os.Stat`
  (never an open): missing or expired means not present (expired files are removed); otherwise `os.Rename` to
  `<hex>.claimed`, then removes that file, and reports true only if the rename succeeded. A failed rename is "not
  present", not an error. No lock files; `Claim` never opens the file.
- Constants live in one place: code length, alphabet, the 10 minute life.

## Files
Create `internal/invite/code.go`, `service.go`, `filestore.go`, with `*_test.go` next to each. Do not edit anything else.

## Steps
1. Write `code.go` and its tests first: alphabet has 32 distinct symbols, round trip, normalisation table
   (`k7qf 9m2x-4tbd`, `O`/`o`, `I`/`l`), rejection of wrong length and symbols, two codes from a fixed reader differ.
2. Write the `Store` interface and `Service` with a fake in-memory store and a fake clock; tests for ttl bounds,
   pending cap, expiry, and "all failures return `ErrInvalid`".
3. Write `FileStore`; tests on a `t.TempDir()`: file modes (skip the mode assertion on Windows), expired invite
   removed, `Claim` twice returns true then false, 20 goroutines claiming the same invite yield exactly one true (the
   Windows CI job runs this too), a second `FileStore` on the same folder sees the first one's invites (stands in for
   two processes), a symlinked folder is refused.
4. `gofmt`, `go vet ./internal/invite` for linux, windows, darwin.

## Validation
`go test ./internal/invite -race -count=1` passes; coverage of `Redeem` and `Claim` failure paths; no import of
`net/http`, `os/exec` or any other `internal/*` package from `internal/invite` except the standard library.

## Risk and rollback
Self-contained package; delete the folder to roll back. The subtle point is Windows: an `os.Remove` of an open file fails there, so `Claim` never opens the file and
claims by renaming.
