---
phase: 5
title: "Unified Unix installer"
status: complete
priority: P1
effort: "1d"
dependencies: []
---

# Phase 5: Unified Unix installer
<!-- Updated: Validation Session 1 - update consumers of deleted scripts in this phase -->

## Goal
One standalone `install.sh` installs, upgrades (re-run) and uninstalls (`--uninstall`) Skill Hub on Linux/macOS (amd64 + arm64) without a clone, verifies SHA-256 always and cosign when available, and puts `skillhub` on PATH by editing the user's shell profile.

## Context
- Research Part A (A1, A2, A4, A5, A7, A9) and Part C target design ([report](../reports/researcher-260930-1040-release-install-pipeline.md)); decisions 1, 3, 4.
- Current: `scripts/install.sh` (version required at :75, cosign mandatory at :169, refuses existing target :360, platforms :49-54), `scripts/upgrade.sh` (rollback logic :82-111), `scripts/uninstall.sh`, `scripts/lib/installer-common.sh`, `scripts/test-installer-lifecycle.sh` (fake curl/cosign fixtures).
- Reference: `~/projects/herdr-gateway/install.sh` (`--uninstall`, latest default), `~/projects/forgentX/install.sh`, uv installer (profile edits for bash/zsh/fish).

## Files to Create / Modify
- Modify: `scripts/install.sh` (self-contained; inline needed helpers from `scripts/lib/installer-common.sh`)
- Modify: `scripts/test-installer-lifecycle.sh` (cover new behavior)
- Delete: `scripts/upgrade.sh`, `scripts/uninstall.sh`, `scripts/lib/installer-common.sh` (after logic is folded in)
- Modify (consumers of deleted scripts, same phase so CI never breaks): `.github/workflows/ci.yml:61` and `.github/workflows/release.yml:179` (syntax-check steps → check `scripts/install.sh` only); `docs/release-runbook.md:12-13, 78-80, 126, 128` (replace with the new `install.sh` re-run / `--uninstall` commands; full rewrite stays in Phase 9)

## Tasks & Steps
- [x] Version: `@SKILLHUB_VERSION@` placeholder default (stamped by release CI); `--version`/`SKILLHUB_VERSION` override; unrendered placeholder + no override → current clear error.
- [x] Platforms: linux|darwin × amd64|arm64 detection (`uname -s/-m`, map aarch64/arm64); clear error listing supported set otherwise.
- [x] Verification: download `checksums.txt`, verify archive SHA-256 (sha256sum | shasum -a 256 | openssl). If `cosign` on PATH: verify `checksums.txt.sigstore.json` against exact tag workflow identity; failure = abort. If absent: one line "Signature not checked (cosign not installed). To verify: <cmd>". `SKILLHUB_REQUIRE_SIGNATURE=1` without cosign = abort with install hint.
- [x] Re-run = upgrade when target has the `.skillhub-managed` marker: stage, back up, swap, run `skillhub version`, roll back on failure (port from upgrade.sh:82-111). Unmanaged existing binary: refuse with explanation. Same version: "already installed" no-op.
- [x] `--uninstall`: remove managed binary + marker only; `--purge-config --yes` semantics preserved from uninstall.sh; also remove the PATH line the installer added (marked block). Never touch workspaces.
- [x] PATH: if install dir not on PATH, append an idempotent marked block (`# >>> skillhub >>>` … `# <<< skillhub <<<`) to: `~/.bashrc` (and `~/.bash_profile` on macOS if it exists), `~/.zshrc` (respect `$ZDOTDIR`), `~/.config/fish/conf.d/skillhub.fish` (fish), else `~/.profile`. Only edit files for shells present/used ($SHELL + existing rc files). Opt-out: `SKILLHUB_NO_MODIFY_PATH=1` or `--no-modify-path` → print exact line instead. Print "Open a new terminal or run: <source cmd>".
- [x] Final output: installed version, path, next step `skillhub init ~/skillhub --yes`.
- [x] Keep: archive-member validation, atomic rename, download size/time limits, `set -eu`, POSIX sh (dash-compatible), no bashisms.

## Verification
- `sh scripts/test-installer-lifecycle.sh` extended: fresh install; re-run same version no-op; upgrade with rollback on failing new binary; uninstall removes binary+marker+PATH block; no-cosign path succeeds with notice; bad signature with fake cosign aborts; REQUIRE_SIGNATURE without cosign aborts; profile edits idempotent across 2 runs for bash/zsh/fish/profile; opt-out prints line only; arm64 mapping.
- `grep -rn "upgrade.sh\|uninstall.sh\|installer-common" .github docs README.md scripts` returns nothing.
- `shellcheck -s sh scripts/install.sh`; run under `dash` and `bash --posix`.

## Risks
- Editing profiles: only append marked blocks; never rewrite files; handle missing trailing newline; skip read-only files with a printed line.
- Piped `sh` has no TTY: no prompts; all behavior via flags/env.
