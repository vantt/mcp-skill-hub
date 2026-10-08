---
title: "Phase 06 — Curation Home, status and operational CLI"
status: done
---

# Phase 06 — Curation Home, status and operational CLI

## Objective

Deliver the first useful UX slice before implementing all domain capabilities.

### Deliverables

- `GetCurationHome` read model;
- `skillhub status`, `--json`, `--quiet`;
- `workspace_validate`, `workspace_rebuild`, `workspace_diff` application/tool contracts;
- prioritized `ActionItem` rules;
- grouped Git/canonical diff summary;
- long-operation progress events.

### Tasks

1. Implement priority ordering from UX spec.
2. Keep `status` strictly local/offline.
3. Detect workspace invalid, recovery pending, stale index and Git dirty.
4. Show unsupported action categories as zero/not-configured without misleading errors.
5. Render one recommended next action.
6. Ensure human and JSON outputs share one read model.
7. Make rebuild progress visible and cancellable before publish where safe.

### Exit gate

- UX fixtures from Phase 0 pass for all currently implemented action types.
- Healthy home is one concise response.
- `status` performs zero network calls.

## Dependencies

- Phase 05 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
