---
phase: 9
title: "Docs"
status: complete
priority: P1
effort: "0.5d"
dependencies: [1, 2, 3, 4, 5, 6, 7, 8]
---

# Phase 9: Docs
<!-- Updated: Validation Session 1 - document source import -->

## Goal
README, user guide and release runbook describe exactly the shipped behavior: one-liner install per OS, agent-first daily use, and no contradictions.

## Context
- Research A11, P1 #10; UX U20, U29. Current docs: `README.md` (80 lines), `docs/user-guide.md` (~308 lines), `docs/release-runbook.md` (:57-83 fail-closed wording to reverse per decision 1).

## Files to Create / Modify
- Modify: `README.md`, `docs/user-guide.md`, `docs/release-runbook.md`
- Modify: `docs/design/05-curation-lifecycle.md` only if Phase 2 did not already update §17

## Tasks & Steps
- [x] README Quickstart step 1 = the two one-liners (Linux/macOS, Windows); build-from-source moves to an "Other install options" section; `SKILLHUB_WORKSPACE` becomes a real step (or note that connected projects auto-resolve after Phase 1).
- [x] Install section: what the installer does (dir, PATH edit + opt-out, SHA-256, optional cosign, `SKILLHUB_REQUIRE_SIGNATURE=1`), pin a version, upgrade (`skillhub update` or re-run), uninstall commands per OS.
- [x] User guide: agent phrasing for create/activate/list/show now works (Phase 2); source onboarding explains import-as-draft, first analysis and `--path` (Phase 3); updated outputs (Phase 4); troubleshooting: host approval/trust prompts after connect for Claude Code/Codex/Gemini (verify current host behavior via docs), Windows SmartScreen note, symlinked `~/.claude`.
- [x] Connection files guidance: don't commit project connection files (machine-specific absolute paths); suggest `.gitignore` entries; re-run `skillhub connect` after moving binary/workspace. Workspace's own `.mcp.json` likewise.
- [x] Release runbook: new flow (preflight script → tag → CI → post-publish smoke), platforms, prerelease vs `latest`, signature policy (optional by default, strict mode), manual verification commands (cosign, `gh attestation verify`).
- [x] Verify every command in docs by running it (scratch HOME), including installers against a draft/fixture.

## Verification
- Link check (relative links resolve); each documented command executed successfully in scratch env; README ≤ ~120 lines.
