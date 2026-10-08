---
title: "Implement onboarding UX, agent tools, and one-line install"
date: 2026-09-30
summary: "Shipped phases 1-9 across two parallel tracks: unified Unix/Windows installers, release CI for 6 platforms, skillhub update command, agent MCP tools, draft source import, CLI output clarity, and docs"
---

# Implement onboarding UX, agent tools, and one-line install

## What happened
- Executed the validated 10-phase plan across two parallel tracks (app track and installer track):
  * **Phase 1 (Workspace resolution and errors):** Subcommands/flags parse first. Resolution order `--workspace` -> `SKILLHUB_WORKSPACE` -> project connection file (.mcp.json, config.toml, settings.json) -> upward discovery. Plain "not found" messages with exact runnable fixes. `doctor --fix` never creates on missing path; only `init` creates. Non-interactive phrasing removed.
  * **Phase 5 (Unified Unix installer):** Consolidated `scripts/install.sh` into a standalone POSIX sh installer supporting install, upgrade (rollback on failure), and uninstall (`--uninstall` / `SKILLHUB_UNINSTALL=1`). Added shell profile modifications (bash, zsh, fish, profile) with opt-out (`SKILLHUB_NO_MODIFY_PATH=1`). Verification with SHA-256 and optional cosign with strict mode (`SKILLHUB_REQUIRE_SIGNATURE=1`). Removed deprecated scripts.
  * **Phase 6 (Windows installer):** Created `scripts/install.ps1` and test suite `scripts/test-installer-windows.ps1` for PowerShell 5.1+ supporting `%LOCALAPPDATA%\skillhub\bin`, registry user PATH, upgrade rollback, uninstall, and SHA-256/cosign verification.
  * **Phase 7 (Release CI):** Updated `.github/workflows/release.yml` with 6 platform targets (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64), stamped installers, cosign signing + provenance attestation, and 3-OS post-publish smoke tests. Updated `ci.yml` with installer tests on Ubuntu, macOS, and Windows. Created `scripts/release-preflight.sh`.
  * **Phase 8 (skillhub update):** Built `internal/selfupdate` and CLI `skillhub update` for atomic self-updates with rollback, `--check` dry-run, and SHA-256/cosign verification using only the Go standard library.
  * **Phase 2 (Agent skill MCP tools):** Implemented `skill_create_preview/confirm`, `skill_transition_preview/confirm`, `skill_list`, and `skill_get` MCP tools. Updated curator skill and bumped `CuratorSkillVersion` with byte-identical check.
  * **Phase 3 (Source first analysis and import):** Added source draft import via CLI `skillhub source import` and MCP `source_import_preview/confirm`. Initial analysis triage, size limit warnings, and duplicate capture idempotency.
  * **Phase 4 (CLI output clarity):** Streamlined CLI messages, exact runnable confirmation and fix commands, one-line host connection summaries, auto-rebuild on read for valid hand edits, `--force` guard on `init`, and plain wording sweep.
  * **Phase 9 (Docs):** Updated `README.md` (concise 85-line quickstart with literal one-liners), `docs/user-guide.md`, and `docs/release-runbook.md`.
- Code review performed by `reviewer` subagent: resolved two concrete findings (fixing unmanaged self-update fix URLs to point to release downloads, and fixing Windows uninstall one-liner syntax in docs).
- Full verification: `go test ./...` passed (19 packages ok), `sh scripts/test-installer-lifecycle.sh` passed, and `pwsh -File scripts/test-installer-windows.ps1` passed (all 15 tests).

## Decision
- All code, scripts, tests, and documentation for Phases 1-9 are complete and verified.
- Phase 10 is user-gated: pushing tags or commits to GitHub requires explicit user confirmation.

## Next steps
- User approval to commit and trigger `v0.1.0` release workflow.
