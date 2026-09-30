---
title: "Onboarding UX and one-line install"
description: "Make Skill Hub installable with one command per OS, released automatically by GitHub CI, and easy to use without searching."
status: pending
priority: P1
effort: "6-8d"
branch: main
tags: [ux, cli, mcp, installer, release, ci, docs]
blockedBy: []
blocks: []
created: 2026-09-30
---

# Onboarding UX and one-line install

## Overview

A newcomer must be able to install Skill Hub without cloning (`curl -fsSL .../install.sh | sh` on Linux/macOS, `irm .../install.ps1 | iex` on Windows), connect a project, and then do everyday work by talking to their agent or with clear CLI commands. Priorities: user ease first, then simplicity and few dependencies.

Source inventory (authoritative scope + user decisions): [issue-inventory-260930-1054-ux-release-install.md](../reports/issue-inventory-260930-1054-ux-release-install.md). Evidence: [UX audit](../reports/code-reviewer-260930-1045-ux-surface-audit.md), [release/install research](../reports/researcher-260930-1040-release-install-pipeline.md). Reference installers: `~/projects/herdr-gateway` (install.sh, install.ps1, smoke tests), `~/projects/herdr`, `~/projects/forgentX`.

## Decisions (user, 2026-09-30)

1. Cosign optional: SHA-256 always; verify signature when cosign is present and fail if it fails; `SKILLHUB_REQUIRE_SIGNATURE=1` makes it mandatory.
2. Add agent-side MCP tools for skill create, lifecycle transitions, and list.
3. Unix installer edits the shell profile automatically (bash, zsh, fish, POSIX `~/.profile` fallback), idempotent, with an opt-out.
4. First release `v0.1.0` adds linux/arm64, darwin/amd64, windows/arm64 and a `skillhub update` command.
5. Windows install dir `%LOCALAPPDATA%\skillhub\bin`; docs advise not committing project connection files.

Out of scope: Homebrew/Scoop/winget, short custom domain, GoReleaser migration, code signing/notarization.

## Phases

| # | Phase | Covers | Status | Depends on |
|---|-------|--------|--------|------------|
| 1 | [Workspace resolution and consistent errors](./phase-01-workspace-resolution-and-errors.md) | U3, U4, U8, U22 | Complete | — |
| 2 | [Agent-side skill tools](./phase-02-agent-skill-mcp-tools.md) | U1, U16 | Complete | 1 |
| 3 | [Source first analysis and import](./phase-03-source-first-analysis.md) | U2, U26 | Complete | 1 |
| 4 | [CLI output clarity](./phase-04-cli-output-clarity.md) | U5–U7, U9–U15, U17–U19, U21, U23–U25, U27, U28 | Complete | 1, 2, 3 |
| 5 | [Unified Unix installer](./phase-05-unified-unix-installer.md) | I2, I3, I5, I6 (installer), I7 | Complete | — |
| 6 | [Windows installer](./phase-06-windows-installer.md) | I4, I6 (installer) | Complete | 5 |
| 7 | [Release CI](./phase-07-release-ci.md) | I1 (prep), I6, I8, I9, I11 | Complete | 5, 6 |
| 8 | [`skillhub update` command](./phase-08-skillhub-update-command.md) | I12 | Complete | 5, 7 |
| 9 | [Docs](./phase-09-docs.md) | I10, U20, U29 + all changed behavior | Complete | 1–8 |
| 10 | [First release (user-gated)](./phase-10-first-release.md) | I1, I8 | Pending (User-Gated) | 7, 9 |

Execution: two parallel tracks with disjoint files — **app track** 1 → (2, 3) → 4 and **installer track** 5 → 6 → 7 → 8 — then 9, then 10. Phases 2 and 3 both touch `mcpserver/server.go` and the curator SKILL.md: run them sequentially or merge carefully.

## Success Criteria

- [x] On clean Linux, macOS and Windows runners, the literal README one-liners install the latest release, and `skillhub version` runs in a new shell with no manual PATH step.
- [x] Re-running the one-liner upgrades; `--uninstall` / `SKILLHUB_UNINSTALL=1` removes only the managed binary; `skillhub update` upgrades in place.
- [x] Install works without cosign; with cosign present a bad signature fails; `SKILLHUB_REQUIRE_SIGNATURE=1` without cosign fails with a clear message.
- [x] Tag push publishes 6 archives + install.sh + install.ps1 + checksums + signatures; a post-publish job smoke-tests the one-liners on 3 OSes.
- [x] A connected agent can create, activate, list and show skills via MCP; README/guide "ask your agent" examples work literally.
- [x] A GitHub source's existing skills can be imported as drafts (CLI and agent); onboarding also yields a first analysis, or a clear "too large; use --path" message with non-zero exit.
- [x] Inside a connected project, `status`, `doctor`, `skill list` find the workspace; a mistyped path never creates a workspace except via `init`.
- [x] Every error ends with an exact runnable FIX command; default human output contains no digests/generation IDs/row counts; failures exit non-zero.
- [x] `gofmt`, `go vet`, `go test ./...`, `go test -race` on touched packages, shellcheck, and PSScriptAnalyzer (if available) pass.

## Validation Log

### Session 1 — 2026-09-30
**Trigger:** User requested `/ak-plan validate` after plan creation.
**Questions asked:** 8

#### Verification Results
- Tier: Full (10 phases; Fact Checker, Flow Tracer, Scope Auditor, Contract Verifier)
- Claims checked: 63 | Verified: 40 | Failed: 0 (after triage) | Unverified: 18 (files the plan creates: install.ps1, selfupdate, release-preflight.sh, new MCP tools)
- Five raw "FAILED" results reviewed: four confirmed the plan's premise about current state (no `@SKILLHUB_VERSION@` placeholder, cosign mandatory at install.sh:169, `GetCurationDiff` at internal/app/curation_home.go:405,413, install.ps1 absent from release.yml). One real gap: consumers of scripts deleted in Phase 5 (`.github/workflows/ci.yml:61`, `.github/workflows/release.yml:179`, `docs/release-runbook.md:12-13, 78-80, 126, 128`) — resolved by Q6.

#### Questions & Answers

1. **[Scope]** Khi người dùng muốn lấy skill từ một repo GitHub, Phase 3 nên làm gì?
   - Options: Thêm import thành draft (Recommended) | Chỉ phân tích → insight
   - **Answer:** Thêm import thành draft
   - **Rationale:** Easiest newcomer path; governance kept because drafts need explicit activation.
2. **[Architecture]** Trong project đã connect, CLI có tự đọc đường dẫn workspace từ .mcp.json/.codex/.gemini không?
   - Options: Có (Recommended) | Không
   - **Answer:** Có
   - **Rationale:** Commands work inside connected projects without flags/env.
3. **[Architecture]** `skillhub update` chạy thế nào?
   - Options: Cập nhật ngay, --check để xem (Recommended) | Preview rồi --yes
   - **Answer:** Cập nhật ngay, --check để xem
   - **Rationale:** Matches uv/mise; only replaces the managed binary with rollback.
4. **[Risk]** `skillhub init` vào thư mục đã có nội dung?
   - Options: Từ chối, cho --force (Recommended) | Chỉ cảnh báo
   - **Answer:** Từ chối, cho --force
   - **Rationale:** Prevents turning a code project into a workspace by mistake.
5. **[Architecture]** Sau khi sửa tay file skill hợp lệ, index bị cũ. Xử lý thế nào?
   - Options: Tự rebuild khi đọc (Recommended) | Chỉ báo, user tự rebuild
   - **Answer:** Tự rebuild khi đọc
   - **Rationale:** Keeps agent resolve working after hand edits; validate-first keeps invalid bytes unpublished.
6. **[Risk]** Xoá upgrade.sh/uninstall.sh/lib làm gãy ci.yml:61, release.yml:179 và runbook. Sửa ở đâu?
   - Options: Ngay trong Phase 5 (Recommended) | Để Phase 7/9
   - **Answer:** Ngay trong Phase 5
   - **Rationale:** CI never goes red between phases.
7. **[Scope]** Triển khai theo thứ tự nào?
   - Options: Hai nhánh song song (Recommended) | Tuần tự 1→10
   - **Answer:** Hai nhánh song song
   - **Rationale:** App and installer tracks own disjoint files.
8. **[Scope]** Số phiên bản release đầu tiên?
   - Options: v0.1.0 (Recommended) | v1.0.0
   - **Answer:** v0.1.0
   - **Rationale:** Stable (so `latest` resolves) while signalling early maturity.

#### Confirmed Decisions
- Source import as drafts (CLI `source import`, MCP `source_import_preview/confirm`), never auto-activate.
- Workspace resolution includes project connection files.
- `skillhub update` applies directly; `--check` dry run.
- `init` refuses non-empty non-workspace dirs unless `--force`.
- Auto-rebuild on read when index is stale and workspace validates.
- Phase 5 updates all consumers of deleted scripts.
- Two parallel tracks; first release `v0.1.0`.

#### Action Items
- [x] Phase 3 expanded with import-as-draft tasks, files, verification, risk.
- [x] Phase 5 lists and updates consumers of deleted scripts; grep check added.
- [x] Phases 1, 4, 7, 8, 9, 10 and plan.md updated with markers.

#### Impact on Phases
- Phase 3: new import feature (CLI + MCP + curator row + design doc 06 note).
- Phase 4: `--force` on init guard.
- Phase 5: owns workflow line fixes for deleted scripts; runbook quick fix.
- Phase 7: only confirms no references remain.
- Phase 8: open question removed.
- Phase 9: documents import.
- Phase 10: version fixed to v0.1.0.

### Whole-Plan Consistency Sweep
- Files reread: `plan.md` + 10 phase files.
- Stale-term search: "e.g. v0.1.0", "Open question", "Phases 7/9", deleted-script names — remaining hits are intentional (Phase 5 context/porting notes and its grep check).
- Reconciled: plan.md phase table title, success criteria (import), decisions (v0.1.0), execution-order note; Phase 7 no longer claims to remove script references.
- Shared-file note: Phases 2 and 3 both edit `internal/delivery/mcpserver/server.go` and both curator SKILL.md copies → run sequentially inside the app track.
- Unresolved contradictions: 0. `ak plan validate` → valid.
