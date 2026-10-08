---
title: "Phase 07 — Curated skill lifecycle"
status: done
---

# Phase 07 — Curated skill lifecycle

## Objective

Support manual skill creation/editing safely before upstream learning automation.

### Deliverables

- create/edit/activate/deprecate/archive commands;
- preview/confirm proposal flow;
- skill metadata/resource validation;
- catalog snapshot changes and generation publish;
- routing-impact hook interface;
- external editor validate/rebuild flow.

### Tasks

1. Generate minimal draft from explicit fields; Agent can supply inferred draft content.
2. Keep draft inactive until required fields validate and activation is approved.
3. Implement direct edit as pinned proposal, not uncontrolled overwrite.
4. Return diff summary/full diff on demand.
5. Preserve provenance/history during deprecate/archive.
6. Implement snapshot-expired behavior for old resource manifests.
7. Add operation receipts for every managed mutation.

### Exit gate

- Create → preview → activate → resolve/read basic skill works.
- Stale edit proposal is rejected.
- Invalid external edit does not publish a new generation.
- Successful mutation reports active local state and uncommitted Git state.

## Dependencies

- Phase 06 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
