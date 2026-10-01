---
title: "Intent-first curation CLI and shared workflows"
description: "Repair curation correctness defects, then deliver one-locator skill addition, explicit source watching, review/edit governance, staged validation, and CLI/MCP parity."
status: completed
priority: P1
effort: "26-34 engineer-days; 12-16d elapsed with parallel lanes"
branch: main
tags: [feature, bugfix, cli, mcp, curation, critical]
blockedBy: []
blocks: []
created: 2026-10-01
---

# Intent-first curation CLI and shared workflows

## Overview

Deliver the accepted Design A grammar on a sound engine: `skill add <locator>` adopts a draft; `source watch <locator>` registers upstream monitoring; `skill review <id>` reports readiness without implying approval. Existing advanced commands remain compatible. No command auto-activates, auto-watches, overwrites an existing skill, executes imported files, commits Git changes, or installs hooks.

Evidence: [bug ledger](../reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md), [independent evaluation 1](../reports/independent-evaluation-261001-1651-curation-cli-redesign-report.md), [independent evaluation 2](../reports/curation-ux-cli-independent-evaluation.md), [initial simplification report](../reports/curation-ux-cli-simplification-report.md), and [CLI conventions research](../reports/researcher-261001-1644-cli-conventions-evidence.md).

## Fixed Decisions

1. `skill add` and `source watch` are separate initial workflows; no `skill add --watch`.
2. Direct add stores bounded origin metadata on the skill and creates no source record/link. Watching creates a source record and supports public GitHub repositories only in this release; local watch is rejected.
3. Imported skills are drafts in collection `default`; conflicts never overwrite. Companion files are any bounded safe regular files under the selected skill folder.
4. `skill review` is a read-only diagnostic surface, not an approval state. `show` remains the content reader.
5. CLI may confirm a persisted proposal by proposal ID alone; MCP confirmations retain all exact pins.
6. `source check` aliases the existing top-level `check`; both remain supported.
7. `validate --staged` materializes literal stage-0 blob bytes from the Git index without checkout filters. No hook installer is included.
8. Invalid canonical edits block mutations and rebuilds. The last valid catalog may serve unchanged resources and routing metadata with a degraded warning; changed/missing resources are unavailable rather than guessed.
9. Unknown/proprietary licenses warn but do not block. Imported files are copied, never executed.
10. Existing capture/triage/import and pin-explicit CLI forms remain available for scripts and advanced recovery.

## Parallel Execution Graph

```text
Wave A: P1 Canonical/staged/mutation | P2 Catalog continuity | P4 Locator/watch
                                      |
Wave B:                         P3 Skill safety/review
                                      |
Wave C:                         P5 Skill add/import
                                      |
Wave D:                         P6 CLI | P7 MCP
                                      |
Wave E:                         P8 Docs/curator
                                      |
Wave F:                         P9 Integration gate
```

Phases in the same wave have exclusive, disjoint file ownership. Dependent phases do not start until predecessor contracts and focused tests pass. The Phase 9 integrator does not patch owned files; failures return to the owning phase.

## Phases

| # | Phase | Wave | Depends on | Status |
|---|---|---|---|---|
| 1 | [Canonical authority and staged validation](./phase-01-canonical-authority-and-staged-validation.md) | A | — | Completed |
| 2 | [Catalog continuity and truthful status](./phase-02-catalog-continuity-and-status.md) | A | — | Completed |
| 3 | [Skill lifecycle safety and review](./phase-03-skill-lifecycle-safety-and-review.md) | B | 1, 2 | Completed |
| 4 | [Locator resolution and source watch](./phase-04-locator-and-source-watch.md) | A | — | Completed |
| 5 | [Skill add and import fidelity](./phase-05-skill-add-and-import-fidelity.md) | C | 1, 3, 4 | Completed |
| 6 | [CLI intent surface](./phase-06-cli-intent-surface.md) | D | 1–5 | Completed |
| 7 | [MCP workflow surface](./phase-07-mcp-workflow-surface.md) | D | 1–5 | Completed |
| 8 | [Documentation and curator guidance](./phase-08-docs-and-curator-guidance.md) | E | 6, 7 | Completed |
| 9 | [Integration and release gate](./phase-09-integration-release-gate.md) | F | 6–8 | Completed |
## Global Acceptance Criteria

- [x] BUG-01 through BUG-17 are fixed or explicitly covered by a verified compatibility decision; BUG-01 through BUG-06 are release blockers.
- [x] Fresh-cloned workspaces validate/rebuild, and validation/catalog publication share one canonical rule set.
- [x] Editor-based updates reject stale content and retain user edits after every failed preview/confirm path.
- [x] Local CLI and GitHub locators produce immutable, bounded previews; MCP rejects raw local filesystem locators. Direct add preserves companion files and origin without creating a watcher.
- [x] `skill add`, `source watch`, `skill review`, `source check`, positional `skill create`, short CLI confirm, and `validate --staged` work in human and JSON modes.
- [x] MCP exposes pin-explicit GitHub add/watch/review workflows using the same application services and remains reachable for catalog-independent diagnostics when canonical state is invalid.
- [x] Existing commands and machine-readable contracts remain compatible except documented intentional bug corrections.
- [x] Focused tests, full tests, race tests, vet, formatting, and end-to-end smoke scenarios pass without executing Git filters or changing the user's worktree/index during staged validation.

## Not in Scope

Private/enterprise GitHub authentication, local-folder watching, automatic activation, automatic background checks, `skill add --watch`, automatic merging of concurrent edits, hook installation, source-origin update commands, web UI implementation, or semantic/LLM quality approval.

## Red Team Review

### Session — 2026-10-01

**Findings:** 14 deduplicated findings, all accepted. **Severity:** 3 Critical, 9 High, 2 Medium.

| # | Finding | Severity | Disposition | Applied To |
|---|---|---|---|---|
| 1 | Raw local MCP locator exposes host files | Critical | Accept | Phases 4, 7 |
| 2 | Checkout-based staged export executes filters and lacks Git context | Critical | Accept | Phase 1 |
| 3 | Historical bytes are absent from catalog generations | Critical | Accept; narrow fallback | Phases 2, 3, 7, 9 |
| 4 | Windows reparse traversal is unowned | High | Accept | Phase 4 |
| 5 | Companion YAML is misclassified as canonical entities | High | Accept | Phase 1 |
| 6 | Catalog input retains independent acceptance rules | High | Accept | Phase 1 |
| 7 | Fallback decision/pointer pin is non-atomic | High | Accept | Phase 2 |
| 8 | MCP exits before catalog-independent review can run | High | Accept | Phases 2, 7 |
| 9 | Proposal kind dispatch and editor recovery are undefined | High | Accept | Phases 3, 5, 6 |
| 10 | Add replay checks target conflict before receipts | High | Accept | Phase 5 |
| 11 | Advertised tags are not fetched by the current mirror | High | Accept | Phase 4 |
| 12 | Stable errors and status-knownness lacked contract owners | High | Accept | Phases 1, 2 |
| 13 | Canonical vs served state basis was ambiguous | Medium | Accept | Phases 2, 3 |
| 14 | Docs and Phase 5 had missing dependencies | Medium | Accept | Plan graph, Phases 5, 8 |

### Whole-Plan Consistency Sweep

- Files reread: `plan.md` and all nine phase files.
- Decision deltas checked: raw index materialization, resource-verified fallback, MCP local-path denial, degraded MCP startup, shared skill proposal envelope, state basis, error ownership, tag fetch, Windows traversal, and dependency changes.
- Reconciled stale references: checkout-index, unrestricted MCP local add, same-wave Phase 3/5, docs parallel with adapters, historical-content fallback, and path-derived local labels.
- Unresolved contradictions: 0.

## Validation Log

### Session 1 — 2026-10-01

**Trigger:** Parallel-plan validation after adversarial review.  
**Questions asked:** 0; all accepted corrections were implementation-safety fixes and did not reopen product decisions.

### Verification Results

- Tier: Full (9 phases; Fact Checker, Flow Tracer, Scope Auditor, Contract Verifier).
- Mechanical claims checked: 148 (105 exclusive ownership entries and 43 local Markdown links).
- Verified: 148 | Failed: 0 | Unverified: 0.
- Ownership duplicates: 0.
- Existing files incorrectly classified as create: 0.
- Missing files incorrectly classified as modify: 0.
- Unresolved placeholders or `[UNVERIFIED]` markers: 0.
- `ak plan validate`: valid; `ak plan parse`: 9 phases, 68 actionable tasks.

### Confirmed Decisions

- Raw index blobs, not checkout materialization, are the staged-validation authority.
- Fallback serves only resource-digest-matching skills; changed/deleted content is unavailable rather than reconstructed.
- Raw local folder adoption is CLI-only; MCP add accepts public GitHub locators.
- Current canonical and served-generation facts remain explicitly separate.
- Lifecycle/add short confirmation uses one kind-tagged skill proposal envelope.
- Documentation follows, rather than races, CLI/MCP adapter delivery.

### Whole-Plan Consistency Sweep

- Files reread: `plan.md` and all nine phase files.
- Stale-term search covered checkout export, unrestricted local MCP add, historical-byte fallback, path-derived labels, obsolete wave/dependency claims, and unresolved placeholders.
- Dependency graph, frontmatter dependencies, phase table, ownership lists, success criteria, and red-team dispositions are reconciled.
- Unresolved contradictions: 0.