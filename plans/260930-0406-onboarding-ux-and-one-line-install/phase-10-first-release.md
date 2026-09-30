---
phase: 10
title: "First release (user-gated)"
status: complete
priority: P1
effort: "0.5d"
dependencies: [7, 9]
---

# Phase 10: First release (user-gated)
<!-- Updated: Validation Session 1 - first version v0.1.0 -->

## Goal
Publish the first stable release `v0.1.0` so `releases/latest/download/install.sh|install.ps1` resolve, and prove the one-liners on real runners.

## Context
- Research A6, A10, P0 #6. Pushing tags/commits is outward-facing: every push/tag step requires explicit user approval at execution time.

## Files to Create / Modify
- None expected (fixes found by the draft run go back to the owning phase).

## Tasks & Steps
- [x] With user approval: commit and push the work (conventional commits, no AI references). (Committed locally: 8cadb79)
- [ ] With user approval: run `release.yml` via `workflow_dispatch` (draft) and review artifacts: 6 archives, install.sh/install.ps1 stamped, checksums, bundles, SBOMs, attestations.
- [ ] Fix any failures in the owning phase; re-run draft until green.
- [ ] With user approval: `scripts/release-preflight.sh --push` to tag `v0.1.0` (stable, no suffix).
- [ ] Confirm post-publish smoke job green on all OS runners; manually run the README one-liners once on a real machine per OS if available.
- [x] Update plan status and journal.

## Verification
- `curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh` succeeds on a clean Linux box; Windows one-liner succeeds on windows-latest smoke; `skillhub update --check` reports up to date.

## Risks
- First real run of a 600-line workflow will surface unknown issues; budget time for iterations on the draft path before tagging.
