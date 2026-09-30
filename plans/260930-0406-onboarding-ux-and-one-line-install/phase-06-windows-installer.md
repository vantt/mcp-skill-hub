---
phase: 6
title: "Windows installer"
status: complete
priority: P1
effort: "0.75d"
dependencies: [5]
---

# Phase 6: Windows installer

## Goal
`powershell -ExecutionPolicy ByPass -c "irm .../install.ps1 | iex"` installs Skill Hub on Windows amd64/arm64 into `%LOCALAPPDATA%\skillhub\bin`, adds it to the user PATH, upgrades on re-run, and uninstalls with `SKILLHUB_UNINSTALL=1`.

## Context
- Research P0 #3, target design (Windows block); decision 5.
- Reference: `~/projects/herdr-gateway/install.ps1`, `windows-install-smoke.ps1`; `~/projects/herdr/distribution/` install.ps1, `scripts/windows_arm64_installer_test.ps1`; uv install.ps1 (registry PATH with `DoNotExpandEnvironmentNames`).

## Files to Create / Modify
- Create: `scripts/install.ps1`
- Create: `scripts/test-installer-windows.ps1` (local-fixture mode mirroring the sh test)

## Tasks & Steps
- [x] PowerShell 5.1+ compatible; no external modules. Params via env only (`SKILLHUB_VERSION`, `SKILLHUB_INSTALL_DIR`, `SKILLHUB_UNINSTALL`, `SKILLHUB_REQUIRE_SIGNATURE`, `SKILLHUB_NO_MODIFY_PATH`) since `irm | iex` cannot pass args.
- [x] Version placeholder `@SKILLHUB_VERSION@` stamped by CI, same rules as install.sh.
- [x] Arch detect (`PROCESSOR_ARCHITECTURE`/`PROCESSOR_ARCHITEW6432`: AMD64, ARM64). Download zip + `checksums.txt` with TLS 1.2 forced; `Get-FileHash` SHA-256; cosign verify if `cosign.exe` on PATH (same rules as Unix).
- [x] Install: `Expand-Archive` to temp, validate members, write managed marker; re-run upgrades (rename running `skillhub.exe` to `.old`, swap, verify `skillhub version`, roll back); unmanaged existing file → refuse.
- [x] PATH: add install dir to HKCU `Environment\Path` preserving `REG_EXPAND_SZ` and unexpanded entries; update current session `$env:Path`; broadcast `WM_SETTINGCHANGE`; tell user to open a new terminal. Opt-out env prints instructions.
- [x] Uninstall: remove managed binary, marker, PATH entry; never touch workspaces.
- [x] Errors: `$ErrorActionPreference = 'Stop'`, readable messages, no stack traces for expected failures.

## Verification
- `pwsh -File scripts/test-installer-windows.ps1` (fixture mode) on Windows runner in CI (Phase 7) and locally with pwsh on Linux for logic that does not touch the registry (guarded).
- PSScriptAnalyzer (if installed) clean of errors.

## Risks
- Registry PATH corruption: read raw value with `DoNotExpandEnvironmentNames`, append only if absent, write same value kind. Test with an existing `%USERPROFILE%`-style entry.
- Defender/SmartScreen on unsigned exe: note in docs (Phase 9); `irm` downloads carry no Mark-of-the-Web.
