---
phase: 9
title: "Integration and release gate"
status: completed
priority: P0
effort: "2-3d"
dependencies: [6, 7, 8]
---

# Phase 9: Integration and release gate

## Context Links

- [Plan acceptance criteria](./plan.md#global-acceptance-criteria)
- [Full defect ledger](../reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)
- [Existing CI workflow](../../.github/workflows/ci.yml)
- [Release runbook](../../docs/release-runbook.md)

## Objective

Integrate the disjoint lanes, prove the accepted end-to-end behavior through the real CLI/MCP surfaces, run project-wide quality gates once, and reject release if any P0/P1 correctness or compatibility requirement remains unverified.

## Exclusive Ownership

This phase owns no production, test, schema, fixture, or documentation files. It may update only plan phase status/checklists through the controller. Any failure returns to the phase that owns the failing file; the integrator does not patch across ownership boundaries.

## Integration Steps

1. Merge/reconcile in dependency order: Wave A (1, 2, 4), Wave B (3), Wave C (5), Wave D (6, 7), then Wave E (8). Confirm no phase edited another phase's files.
2. Run focused package tests from each phase before broad checks. Fix failures in the owning phase and rerun the failed focused path first.
3. Build the actual `skillhub` binary. Create disposable Git workspaces; do not reuse developer state or credentials.
4. Execute the defect matrix:
   - explicit no-monitor remains disabled;
   - stale editor update conflicts and preserves the winning bytes;
   - failed editor preview retains recoverable content;
   - folder-root companions survive import/add;
   - validate/build reject identical invalid state;
   - clone-like workspace validates/rebuilds;
   - an unrelated invalid canonical edit leaves unchanged skills available through an identified fallback, while a changed skill is excluded/content-unavailable and mutation/rebuild remain blocked;
   - status never maps unknown counts to empty;
   - untouched scaffold cannot activate;
   - GitHub heads/tags with slash refs resolve; stale/deleted refs do not; local folders resolve safely;
   - Windows junction/reparse and path-swap attempts cannot escape the selected root;
   - editor preview prints exact confirm rather than rerun advice and recovery artifacts expire safely;
   - current canonical and served-generation state fields remain basis-labeled;
   - workspace/diff/status/license outputs are corrected.
5. Execute the beginner CLI journey: init → local add preview → short confirm → review → edit → activate; then source watch → source check alias. Assert no hidden watch/activation/commit and no persisted source path.
6. Execute staged-governance scenarios with opposite staged/worktree validity, unmerged entries, and a malicious smudge/process filter sentinel. Prove literal blob validation, no filter execution, and unchanged worktree/index identity.
7. Execute MCP parity over in-memory and stdio transports: degraded startup with/without fallback, GitHub add/watch previews, raw local add refusal before enumeration, exact-pin confirms, review, digest-aware conflict, blind-replacement compatibility, and unchanged legacy tools.
8. Execute fault/race scenarios: invalid↔valid fallback transition, cancellation before/after mutation displacement, cross-process editor stale confirm/recovery, successful add with lost response and deleted proposal cache, and watch-policy conflict.
9. Run one bounded live public GitHub smoke against a pinned repository/ref/path. If external access is unavailable, record it as unverified; deterministic local Git/ref tests still pass.
10. Compare CLI help, docs, system-curator tool names, schemas, error codes, proposal kinds, and compatibility matrix. No stale command or unsupported promise may remain.
11. Run broad quality gates and inspect output. Do not weaken or skip failures.

## Todo

- [x] Confirm ownership compliance and dependency merge order.
- [x] Run all phase-focused tests.
- [x] Reproduce all 17 ledger cases against the built binary.
- [x] Complete CLI, staged-index/filter, and MCP degraded-mode journeys.
- [x] Complete fallback/resource, cancellation, proposal-loss, editor-recovery, and watch-policy race/fault scenarios.
- [x] Complete Windows reparse/path-swap coverage on Windows CI.
- [x] Run bounded live GitHub smoke or record exact external blocker.
- [x] Run full test/race/vet/format/build gates.
- [x] Complete docs/help/tool/schema consistency sweep.
- [x] Mark plan ready for shipping only with evidence for every release blocker.

## Success Criteria

- BUG-01 through BUG-17 have passing regression or smoke evidence; no P0/P1 item is waived.
- CLI and MCP use the same application semantics, typed errors, and basis-aware state/effect facts.
- No partial mutation occurs on ambiguity, stale proposal/edit, source change, limit/pin failure, or pre-displacement cancellation; post-displacement interruption has an explicit durable recovery outcome.
- Existing advanced commands and old pin-explicit CLI/MCP flows still pass.
- Fresh clone, Linux/macOS/Windows CI paths, and cross-compilation remain viable; malicious Git filters never execute during staged validation.
- Repository contains no throwaway smoke artifacts, expired editor recovery files, credentials, serialized local source paths, or generated binaries.

## Verification

```bash
gofmt -l cmd internal schemas
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/skillhub
```

Targeted smoke assertions must record command, exit code, bounded output, and before/after canonical/index digests in the implementation report. Existing CI already covers Linux, macOS, Windows, race on non-Windows, fuzz targets, and six cross-compile targets; change CI only if implementation introduces an uncovered platform dependency.

## Rollback

- New CLI/MCP adapters can be reverted independently because existing primitives remain.
- Application workflows revert with their proposal artifacts; snapshot/proposal/editor artifacts expire under bounded runtime cleanup and canonical receipts remain auditable.
- Canonical schema/layout changes require compatibility-preserving rollback: do not write newly accepted companion/origin shapes and then downgrade to a binary that rejects them. Restore the previous canonical tree from Git before downgrade.

## Risks and Security

- Live GitHub smoke is external evidence, not a deterministic test gate. Never use private credentials.
- Race tests can expose real shared-state defects; fix causes rather than serializing tests or loosening assertions.
- Integration owner must not erase user changes or kill unrelated processes. All smoke state stays in private temporary directories.

## Handoff

When all gates pass, sync checkboxes/status to the plan and proceed with the normal review/ship workflow. This plan does not authorize commit, push, release, or merge.