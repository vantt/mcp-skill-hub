---
title: "Phase 09 — Distillation and learning pipeline"
status: done
---

# Phase 09 — Distillation and learning pipeline

## Objective

Transform pinned source revisions into durable findings/evidence/proposals without applying active skill changes.

### Deliverables

- prepare/start run;
- immutable source revision packages;
- findings/observations and stable identity;
- coverage ledger;
- tombstone/supersession;
- cross-source comparisons;
- insight proposal generation/validation;
- submit/auto-finalize;
- retry/cancel/AwaitingDecision recovery.

### Tasks

1. Pin source/from/to revisions and changed scope.
2. Require evidence reads from target revision; diff only scopes work.
3. Define stable finding identity and source vocabulary.
4. Validate evidence locators/digests.
5. Require every changed resource be analyzed/deferred/unreadable/out-of-scope with reason.
6. Generate tombstones for removed knowledge.
7. Build/update comparisons and staleness markers.
8. Keep Observation (“source says”) separate from Insight (“we should adopt”).
9. Auto-finalize valid submissions in one canonical WriteSet.
10. Advance cursor only with run/findings/coverage/insights atomically.
11. Enter AwaitingDecision only for blocking ambiguity/coverage.
12. Batch multiple sources while isolating failures.

### Exit gate

- Failed run never advances cursor.
- Finalized run always has valid artifacts and complete coverage classification.
- Normal successful run needs no user finalize action.
- Distillation never modifies active skill content.
- Findings are hidden from default UX but queryable on demand.

## Dependencies

- Phase 08 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
- Preserve authority from `docs/PRD.md`, `docs/design/*`, `archive/final.md`, and the V1 product boundary in `plan.md`.

## Related files

Expected file ownership will be refined before cooking this phase. The likely affected areas are:

- `/home/vantt/projects/mcp-skill-hub/cmd/skillhub/`
- `/home/vantt/projects/mcp-skill-hub/internal/`
- `/home/vantt/projects/mcp-skill-hub/schemas/`
- `/home/vantt/projects/mcp-skill-hub/system-skills/`
- `/home/vantt/projects/mcp-skill-hub/testdata/`
- `/home/vantt/projects/mcp-skill-hub/docs/`

## Validation gate

Before this phase is considered done, run the narrow tests added for the phase plus the current shared gates:

```bash
go test ./...
go vet ./...
```

Add phase-specific commands from the roadmap section above when implementation reaches this phase. If shared public contracts change, also run relevant CLI/MCP contract tests and delete-runtime/rebuild tests.

## Risks

- Implementing this phase before its prerequisites can weaken Git-first and derived-state invariants.
- Adding shortcuts to satisfy a command surface can create a second source of truth outside canonical files.
- User-facing behavior can drift from Phase 01 UX fixtures if adapters format results independently.

## Rollback

- Revert files touched by this phase using Git.
- If canonical workspace mutations were introduced, verify recovery can roll forward or restore the previous valid state before deleting runtime artifacts.
- Runtime databases and caches remain disposable and may be removed with `rm -rf runtime/` in test workspaces only.
