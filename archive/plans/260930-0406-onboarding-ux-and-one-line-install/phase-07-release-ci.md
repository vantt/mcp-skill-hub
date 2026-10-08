---
phase: 7
title: "Release CI"
status: complete
priority: P1
effort: "1d"
dependencies: [5, 6]
---

# Phase 7: Release CI

## Goal
A `v*.*.*` tag push builds 6 platform archives, stamps and publishes `install.sh` + `install.ps1`, signs everything, and then smoke-tests the literal one-liners on Linux, macOS and Windows; CI tests both installers on every PR.

## Context
- Research Part A (A6, A7, A10, A12, A13, A14) and P0 #5/#6, P1 #8/#9, P2 #12/#15.
- Current: `.github/workflows/release.yml` (targets :205-217, install.sh copy :396-400, checksums :402-411, sign :413-448, duplicated verify :450-492 vs :547-593, publish :595-609, `check-latest: true` :145,229), `.github/workflows/ci.yml` (installer test :50-64, build matrix :97-101; perf step added earlier).
- Reference: `~/projects/herdr-gateway/.github/workflows/release.yml` + smoke scripts, `~/projects/herdr/.github/workflows/release.yml`, `windows-arm64.yml`, `~/projects/herdr-gateway/release.sh`, `~/projects/forgentX/scripts/prepare-release.mjs`.

## Files to Create / Modify
- Modify: `.github/workflows/release.yml`, `.github/workflows/ci.yml`
- Create: `scripts/release-preflight.sh` (clean tree, on main, tag unused, CI green via `gh run list`, then tag+push only when invoked with `--push`)

## Tasks & Steps
- [x] Build matrix: add linux/arm64, darwin/amd64, windows/arm64 (CGO_ENABLED=0; same deterministic archive + SBOM steps).
- [x] Stamp `@SKILLHUB_VERSION@` in install.sh and install.ps1 during asset copy; include install.ps1 in checksums, signing, attestation.
- [x] Confirm no workflow step references the deleted scripts (Phase 5 already updated ci.yml:61, release.yml:179); keep gate jobs (tests, race, vet, fuzz, perf serial step).
- [x] Deduplicate verify blocks (verify once after signing; publish job reuses outputs); one shared SemVer validation step; pin exact Go version (from go.mod `toolchain`/`go` line) instead of `check-latest`.
- [x] Post-publish smoke job (needs publish): ubuntu-latest, ubuntu-24.04-arm (if available), macos-latest, macos-13 (Intel), windows-latest run the literal one-liners against the new tag's pinned URL, then `skillhub version` in a fresh shell (verifies PATH edit), then uninstall. From the second release on, upgrade-from-previous-tag smoke (skip when no previous stable tag).
- [x] ci.yml: run `scripts/test-installer-lifecycle.sh` on ubuntu + macOS and `scripts/test-installer-windows.ps1` on windows-latest for every PR; add new platforms to the cross-build check.
- [x] Prereleases: keep `--prerelease` for suffixed tags; document that `latest` ignores them (Phase 9).
- [x] `scripts/release-preflight.sh`: checks + prints next command; `--push` creates annotated tag and pushes.

## Verification
- `actionlint` (if available) on both workflows; `shellcheck scripts/release-preflight.sh`.
- `workflow_dispatch` draft run on a branch/fork succeeds end to end and produces 6 archives + 2 installers + checksums + bundles (user-triggered; see Phase 10).
- ci.yml passes on a PR with installer tests on 3 OSes.

## Risks
- ARM runner availability: fall back to cross-compiled build only (no smoke) if `ubuntu-24.04-arm`/`macos-13` unavailable; record in runbook.
- Keep pinned action SHAs and `permissions: {}` defaults; post-publish job needs only `contents: read`.
