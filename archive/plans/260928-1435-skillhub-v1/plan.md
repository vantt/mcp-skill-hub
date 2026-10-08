---
title: "Skill Hub V1 Implementation"
description: "Cook-ready phased plan for the local-first Git-backed Curated Skill Hub V1."
status: completed
priority: P1
effort: "multi-week"
issue: null
branch: main
tags: [feature, backend, mcp, cli, infra]
blockedBy: []
blocks: []
created: 2026-09-28
---

# Skill Hub V1 Implementation Plan

## Overview

Build Skill Hub V1 as a single Go binary that manages a Git-first curated skill workspace, exposes CLI and MCP stdio surfaces, distributes skills through the official MCP skills extension, and provides evidence-first skill resolution without requiring Node, Python, cloud infrastructure, or a web UI.

This plan converts the roadmap in [`../skillhub-v1-implementation-plan.md`](../skillhub-v1-implementation-plan.md) into CK-valid phase files suitable for `/ak:cook`. The architecture authority is [`docs/PRD.md`](../../../docs/PRD.md), [`archive/final.md`](../../final.md) (historical), and [`docs/design/`](../../../docs/design/).

## Product boundary

### In scope

- One installable Go binary, `skillhub`.
- One primary local Git data workspace per OS user/machine.
- CLI and MCP stdio surfaces backed by shared application services.
- Bundled System Curator Skill.
- `doctor`, `doctor --fix`, `init`, `validate`, `rebuild`, and operational curation commands.
- Git-backed canonical skill, source, learning, decision, operation, policy, and eval files.
- Crash-safe multi-file mutation and recovery.
- Rebuildable immutable SQLite/FTS catalog generations.
- Source intake, revision checks, distillation, insight proposals, pinned preview/confirm apply, resolver, distribution, telemetry, evaluation, and release hardening.

### Out of scope for V1

- Web UI.
- Cloud/multi-tenant service.
- Required Node or Python runtime.
- Automatic Git commit/push/pull.
- Server-side LLM as a normal routing dependency.
- Vector retrieval before measured need.
- Auto-apply upstream changes.
- Global canonical `eventlog.jsonl`.
- Database export/import as workspace portability mechanism.

## Execution principle

Implementation order stays UX-first and contract-first:

```text
UX acceptance transcripts
→ CLI/MCP contracts
→ application commands
→ domain model and invariants
→ canonical repositories/mutations
→ derived SQLite projections
→ integrations and optimization
```

Do not start from CRUD tables. Each vertical slice must prove a user intent through shared application services; CLI and MCP are adapters over the same semantics.

## Phases

| Phase | Name | Status | Dependencies | Cook readiness |
|---:|---|---|---|---|
| 00 | [Freeze UX behavior before implementation](./phase-01-ux-contract-freeze.md) | Complete | None | Delivered |
| 01 | [Go foundation, CI and release skeleton](./phase-02-go-foundation.md) | Complete | Phase 01 | Delivered |
| 02 | [Workspace bootstrap and canonical read model](./phase-03-workspace-canonical-read-model.md) | Complete | Phase 02 | Delivered |
| 03 | [Canonical mutation transaction and recovery](./phase-04-canonical-mutation-recovery.md) | Complete | Phase 03 | Delivered |
| 04 | [Rebuildable immutable SQLite catalog generations](./phase-05-sqlite-catalog-generations.md) | Complete | Phase 04 | Delivered |
| 05 | [Curation Home, status and operational CLI](./phase-06-curation-home-operational-cli.md) | Complete | Phase 05 | Delivered |
| 06 | [Curated skill lifecycle](./phase-07-curated-skill-lifecycle.md) | Complete | Phase 06 | Delivered |
| 07 | [Source intake, adapters and monitoring](./phase-08-source-intake-adapters-monitoring.md) | Complete | Phase 07 | Delivered |
| 08 | [Distillation and learning pipeline](./phase-09-distillation-learning-pipeline.md) | Complete | Phase 08 | Delivered |
| 09 | [Insight inbox, apply and outcomes](./phase-10-insight-inbox-apply-outcomes.md) | Complete | Phase 09 | Delivered |
| 10 | [Evidence-first resolver](./phase-11-evidence-first-resolver.md) | Complete | Phase 10 | Delivered |
| 11 | [MCP protocol and skill distribution](./phase-12-mcp-protocol-skill-distribution.md) | Complete | Phase 11 | Delivered |
| 12 | [Bundled System Curator Skill and host bootstrap](./phase-13-system-curator-host-bootstrap.md) | Complete | Phase 12 | Delivered |
| 13 | [Telemetry, reproducibility and evaluation](./phase-14-telemetry-reproducibility-evaluation.md) | Complete | Phase 13 | Delivered |
| 14 | [Hardening, migrations and V1 release](./phase-15-hardening-v1-release.md) | Complete | Phase 14 | Delivered; hosted release evidence pending first tag |
| 15 | [Post-core capabilities, evidence-gated](./phase-16-post-core-evidence-gated-capabilities.md) | Complete | Phase 15 | Gate held; no optional capability justified |

## Cross-phase quality gates

Every phase must preserve these gates:

- CLI human output and `--json` derive from the same result envelope.
- CLI and MCP map to the same application service for equivalent semantics.
- Canonical mutation flows through the mutation service only.
- Runtime databases never become business authority.
- `status` and `rebuild` do not require network.
- Untrusted source and skill content are data, not executable policy.
- Path and symlink containment are tested.
- Receipts and telemetry exclude secrets and absolute local paths.
- UX keeps one recommended next action and uses progressive disclosure.
- Reproducibility pins request, catalog snapshot, policy revision, builder version, and input digest as needed.

## V1 acceptance scenarios

1. Fresh install → init → doctor → healthy status.
2. Clone canonical repo → delete runtime → rebuild → equivalent resolver behavior.
3. Create draft skill → preview → activate → resolve → load resource.
4. External valid edit → stale detection → rebuild → new snapshot.
5. External invalid edit → old generation preserved + actionable error.
6. Capture source → onboard → unchanged check leaves Git clean.
7. Detect source revision → batch distill → findings/insights, active skill unchanged.
8. Crash during cursor/finding write → doctor roll-forward → atomic finalized run.
9. Review insight → preview → concurrent edit → stale apply rejected.
10. Regenerate proposal → approve → skill/provenance/receipt all change atomically.
11. Agent Host loads recommended skill with pinned digest.
12. Two Agent Hosts race a mutation → one succeeds, one conflicts or idempotently observes the result.
13. Corrupt derived DB → rebuild while canonical remains untouched.
14. Telemetry DB deletion → behavior unchanged.
15. Git checkout to older valid revision → rebuild → matching historical behavior.

## Dependencies

- Supported Go version and pure-Go SQLite/FTS5 decision are finalized in Phase 1.
- MCP skills extension and SDK behavior must be pinned before Phase 12 implementation.
- Golden UX fixtures from Phase 0 are authoritative for user-facing copy and action policy.
- Storage and mutation invariants from Phases 2–4 are prerequisites for curation, resolver, and distribution work.

## Cook handoff

Execute phases in dependency order. With `/ak:cook --auto`, continue automatically after each phase passes its validation and review gates; pause only for a real blocker, a required product decision, or an action outside the authorized scope.

```bash
/ak:cook --auto --parallel /home/vantt/projects/mcp-skill-hub/plans/260928-1435-skillhub-v1/plan.md
```

## Validation Log

### Session 1 — 2026-09-28
**Trigger:** User requested `validate plans/260928-1435-skillhub-v1` before cooking.
**Questions asked:** 1

### Verification Results
- **Tier:** Full
- **Claims checked:** 6 structural/contract checks across the plan directory
- **Verified:** 6 | **Failed:** 0 | **Unverified:** 0
- **CK directory validation:** `ak plan validate plans/260928-1435-skillhub-v1 --json` returned `valid: true`.

#### Questions & Answers

1. **[Scope / Plan structure]** Validation found three decisions before cooking. How should I handle them?
   - Options: Apply recommended fixes: normalize later phases with explicit Objective sections, include all Phase 01 fixtures in validation command, keep Go module/CLI library choice as Phase 02 cook-time decision (Recommended) | Only record findings in Validation Log; do not edit phase files | Make Phase 02 stricter now by choosing a specific Go module path and no external CLI library | Other
   - **Answer:** Apply recommended fixes: normalize later phases with explicit Objective sections, include all Phase 01 fixtures in validation command, keep Go module/CLI library choice as Phase 02 cook-time decision (Recommended)
   - **Rationale:** These edits make the plan more consistent and cook-ready without prematurely choosing implementation details that should be decided while creating the Go skeleton.

#### Confirmed Decisions
- Later phases must expose explicit `## Objective` sections, even when they remain structured outlines.
- Phase 01 validation must check every fixture listed in its Related files section.
- Phase 02 keeps Go module path and CLI-library selection as implementation-time decisions, constrained by V1 requirements and license review.

#### Action Items
- [x] Normalized phase 02–15 objective sections.
- [x] Added `unavailable-source.yaml`, `git-dirty-after-apply.yaml`, and `partial-distill-failure.yaml` to the Phase 01 validation command.
- [x] Re-ran CK directory validation successfully.

#### Impact on Phases
- Phase 01: Validation command now covers all required fixture files listed in the phase.
- Phase 03–15: Added or normalized explicit Objective sections for cook readability.
- Phase 02: No file change required; implementation choices remain bounded by the phase requirements.

### Whole-Plan Consistency Sweep
- Files reread: `plan.md` and 16 `phase-*.md` files.
- Decision deltas checked: 3
- Reconciled stale references: 2
- Unresolved contradictions: 0
