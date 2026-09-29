---
title: "Phase 08 — Source intake, adapters and monitoring"
status: done
---

# Phase 08 — Source intake, adapters and monitoring

## Objective

Capture and monitor upstream sources without changing curated skills or creating routine Git noise.

### Deliverables

- source candidate capture/list/triage;
- source catalog/link records;
- adapters for Git repository, filesystem and immutable/living documents as scoped;
- explicit network check command/tool;
- runtime check/scheduler state;
- due/changed/unavailable action items;
- source trust/license metadata and limits.

### Tasks

1. Implement capture with locator + reason only.
2. Detect identity/default branch/path/license where available.
3. Present one consolidated onboarding proposal.
4. Implement adapter contract: identify/current revision/diff/read/list.
5. Enforce protocol, size, timeout, traversal and credential policies.
6. Persist new meaningful revision/digest canonically.
7. Store unchanged check time/latency/retry/transient availability only in operational DB.
8. Ensure one source failure does not fail batch checks.
9. Add scheduler only to `serve`/explicit OS timer mode.
10. Never execute source scripts.

### Exit gate

- Repeated unchanged checks leave `git status` unchanged.
- New revision creates durable actionable work.
- `hub_status` itself performs no fetch.
- Clone/rebuild retains source revisions/decisions but may legitimately lose check timestamps.

## Dependencies

- Phase 07 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
