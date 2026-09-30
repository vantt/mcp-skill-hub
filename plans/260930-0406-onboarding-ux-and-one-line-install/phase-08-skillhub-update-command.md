---
phase: 8
title: "skillhub update command"
status: complete
priority: P2
effort: "0.75d"
dependencies: [5, 7]
---

# Phase 8: `skillhub update` command
<!-- Updated: Validation Session 1 - applies directly, --check for dry run -->

## Goal
`skillhub update` upgrades a managed install in place to the latest stable release (or `--version X`), verifying SHA-256 (and cosign when available), using only the Go standard library.

## Context
- Research P2 #11; decision 4 (name `skillhub update`).
- Release asset naming `skillhub-<VERSION>-<os>-<arch>.(tar.gz|zip)`, `checksums.txt` (release.yml:279, :402-411); managed marker `.skillhub-managed` next to the binary (install.sh / install.ps1).
- Reference: `~/projects/herdr-gateway/src/update` (update flow), uv/mise self-update behavior.

## Files to Create / Modify
- Create: `internal/selfupdate/selfupdate.go` (+ tests with `httptest` fixture server)
- Create: `internal/delivery/cli/update.go`; modify `internal/delivery/cli/root.go`, `help.go`
- Modify: `internal/version` only if a helper for current version/os/arch is needed

## Tasks & Steps
- [x] Refuse unless the running executable sits next to a managed marker; otherwise print the one-liner to reinstall (e.g. built from source).
- [x] Resolve target: `--version` or latest stable via `https://github.com/vantt/mcp-skill-hub/releases/latest` redirect (no API token); `--check` only reports.
- [x] Download archive + `checksums.txt` with timeouts and size limits; verify SHA-256; if `cosign` on PATH verify bundle (fail closed), honor `SKILLHUB_REQUIRE_SIGNATURE`.
- [x] Extract binary only (validate member name), write next to current, atomic replace (Windows: rename running exe to `.old`, clean up on next run), run new binary `version`, roll back on failure.
- [x] Human output: "Updated 0.1.0 -> 0.2.0." / "Already up to date (0.2.0)."; `--json` via result envelope. `update` applies directly (it only replaces the managed binary); `--check` reports without changing anything.
- [x] Update base URL overridable only via hidden env for tests (`SKILLHUB_UPDATE_BASE_URL`), not documented.

## Verification
- `go test ./internal/selfupdate ./internal/delivery/cli` with fixture server: newer version installs; same version no-op; checksum mismatch aborts without touching binary; unmanaged install refused; rollback on failing new binary.
- Post-publish smoke (Phase 7) from second release: `skillhub update` from previous tag.

## Risks
- Replacing a running binary on Windows; follow rename-then-replace and test on windows-latest in CI.
