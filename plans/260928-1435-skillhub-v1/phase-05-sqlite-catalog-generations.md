---
title: "Phase 05 — Rebuildable immutable SQLite catalog generations"
status: done
---

# Phase 05 — Rebuildable immutable SQLite catalog generations

## Objective

Build all query/search projections from canonical files and publish them atomically.

### Deliverables

- `skillhub rebuild`;
- `BuildCatalogGeneration` application service;
- immutable `runtime/catalog/generations/*.db`;
- atomic `runtime/catalog/current.json`;
- `runtime/operational.db`;
- `runtime/telemetry.db` placeholder;
- generation integrity/smoke checks;
- stale detector and old-generation GC policy.

### Tasks

1. Create immutable build-input scanner from Phase 2.
2. Build initial relational/FTS schema for:
   - skills/resources/routing metadata;
   - sources/revisions;
   - findings/comparisons/insights;
   - provenance/outcomes;
   - operation/idempotency lookup.
3. Insert all projections in one SQLite transaction.
4. Store schema versions, builder version, both digests and row counts.
5. Run integrity, FK and representative query checks.
6. Reacquire exclusive lock and recheck projection digest before publish.
7. Atomic-write generation pointer; never truncate live generation.
8. Let readers pin old generations; defer GC safely.
9. Recreate missing operational DB empty/default.
10. Never reconstruct telemetry.
11. Trigger same service from init, explicit rebuild, doctor fix and eligible startup.

### Tests

- delete all `runtime/` then rebuild offline;
- same canonical bytes on two OSes produce same logical digests/results;
- canonical changes during build prevent stale publish;
- corrupt new generation leaves previous pointer valid;
- source-only changes alter projection digest without altering catalog snapshot;
- operation receipts restore idempotency lookup without event replay.

### Exit gate

- No `.db` file is tracked by Git.
- Full rebuild is correctness oracle.
- SQLite bytes need not match, but logical rows/query results and digests do.
- Missing telemetry/operational history cannot change catalog behavior.

## Dependencies

- Phase 04 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
