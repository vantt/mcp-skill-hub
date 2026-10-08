---
title: "Phase 04 — Canonical mutation transaction and recovery"
status: done
---

# Phase 04 — Canonical mutation transaction and recovery

## Objective

Provide the only sanctioned write path for CLI/MCP/domain commands.

### Deliverables

- shared/exclusive cross-process workspace lock;
- `WriteSet`, proposal, precondition and operation abstractions;
- `.skillhub/transactions/<op>/` WAL;
- staged after/before images;
- atomic path replacement;
- operation receipts;
- idempotency lookup contract;
- recovery classifier and `doctor --fix` actions;
- conflict-safe external edit handling.

### Tasks

1. Implement `PlanMutation` and `ConfirmMutation` boundaries.
2. Verify proposal ID, digest, base catalog snapshot and per-path before digests.
3. Validate virtual result before any canonical replacement.
4. Stage and fsync manifest/after-images.
5. Apply domain paths in deterministic order and receipt last.
6. Detect path state by before/after/unknown digest.
7. Implement default approved roll-forward recovery.
8. Implement explicit rollback only when all before-image conditions hold.
9. Persist/rebuild idempotency keys from operation receipts.
10. Return operation ID, changed paths, snapshots and Git dirty state.
11. Refuse normal writes while recovery is pending.

### Fault-injection matrix

Inject failure after:

- transaction directory creation;
- each staged file/fsync;
- manifest phase update;
- first/middle/last canonical rename;
- operation receipt write;
- canonical post-validation;
- journal cleanup.

### Exit gate

Every injected case recovers to exactly:

```text
old valid canonical state
or
intended new valid canonical state
```

No accepted mixed state, silent overwrite or duplicate retry is possible.

## Dependencies

- Phase 03 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
- Preserve authority from `docs/PRD.md`, `docs/design/*`, `final.md`, and the V1 product boundary in `plan.md`.

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
