---
title: "Review fixes, onboarding UX, and one-line install plan"
date: 2026-09-30
summary: "22 review bugs fixed, connect/help/docs onboarding shipped locally, validated 10-phase plan for one-line install and UX"
---

# Review fixes, onboarding UX, and one-line install plan

## What happened
- Reviewed uncommitted skillhub-v1 work (phases 11-16) with 4 parallel Opus reviewers; 22 confirmed bugs (storage/migration, resolver/MCP, telemetry/eval, host integration) fixed by Sonnet agents; full `go test ./...`, `-race` and serial perf gate green.
- Perf test `TestCatalogPerformanceBudgets` flaked under parallel package load; wall-clock budgets now run only with `SKILLHUB_PERF=1` in a serial CI step.
- Onboarding audit: host config was written only into the workspace dir, so agents in real projects never saw the hub; no `--help`; README had no Quickstart.
- Added `skillhub connect` (project default, `-g/--global` user scope), `help`/`--help`, trimmed `init` output, `SKILLHUB_WORKSPACE`, `skill list`, plus 8 more CLI UX fixes; rewrote README (Quickstart) and added docs/user-guide.md.
- Research: release CI exists but never ran; installer requires cosign, no Windows installer, upgrade/uninstall need a clone. Surveyed uv, gh, chezmoi, bun, herdr-gateway, herdr, forgentX.

## Decision
- Plan `plans/260930-0406-onboarding-ux-and-one-line-install` (10 phases, two parallel tracks) validated: cosign optional, MCP tools for skill create/transition/list, installer edits shell profiles (bash/zsh/fish/profile), 6 platforms, `skillhub update` (applies; `--check`), source import as drafts, `init` refuses non-empty dirs unless `--force`, auto-rebuild on stale read, first release `v0.1.0`.
- Durable decisions belong in docs/design (05 §17, 06 import note) when phases land.

## Next steps
- `/ak:cook plans/260930-0406-onboarding-ux-and-one-line-install/plan.md`.
- Nothing committed yet; commit/push/tag are user-gated (Phase 10).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
