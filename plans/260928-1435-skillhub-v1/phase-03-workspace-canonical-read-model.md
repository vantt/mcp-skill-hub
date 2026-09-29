---
title: "Phase 03 — Workspace bootstrap and canonical read model"
status: done
---

# Phase 03 — Workspace bootstrap and canonical read model

## Objective

Create/open/validate a Git-first workspace without yet implementing business mutations.

### Deliverables

- workspace locator and primary-workspace config;
- `skillhub init <path>` skeleton;
- `skillhub validate`;
- canonical schema version reader;
- deterministic scanner;
- strict parsers for initial canonical entities;
- Git state inspection;
- `.gitignore` for `runtime/` and `.skillhub/transactions/`;
- first `doctor` findings and fix-plan model.

### Tasks

1. Implement workspace containment and nested/source-repository safety checks.
2. Create canonical empty directory/files and initialize Git when approved.
3. Implement one-file-per-ID layouts for source-learning entities.
4. Define schemas for:
   - skill metadata/resources;
   - source candidates/sources/links;
   - run/finding/comparison/insight;
   - proposal/incorporation/outcome;
   - operation receipt;
   - config/eval entities.
5. Implement two-pass validation:
   - identity/shape;
   - references/invariants.
6. Detect unresolved Git merge conflicts and unsafe symlinks.
7. Compute deterministic per-file digests, `catalog_snapshot` and `projection_input_digest`.
8. Make `init` and `doctor --fix --workspace` call one remediation application service.
9. Ensure `doctor` is read-only and fix plan requires approval/`--yes`.

### Tests

- empty directory initialization;
- valid existing compatible repository;
- reject nested/source checkout without explicit approval;
- duplicate ID and broken reference fixtures;
- path/symlink escape fixtures;
- deterministic digest across OS path separators;
- repeated init/fix is idempotent.

### Exit gate

- A workspace can be cloned/opened and fully validated without network.
- Invalid canonical state reports exact path/entity and actionable fix.
- No durable ID/cursor exists only in memory or SQLite.

## Dependencies

- Phase 02 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
