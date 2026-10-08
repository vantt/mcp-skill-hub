---
title: "Phase 10 — Insight inbox, apply and outcomes"
status: done
---

# Phase 10 — Insight inbox, apply and outcomes

## Objective

Turn source learning into reviewed semantic changes with full traceability.

### Deliverables

- grouped/ranked insight inbox;
- plan/reject/obsolete decisions;
- preview/apply proposal;
- source-to-local mappings;
- incorporation/outcome records;
- changed-upstream impact query;
- operation diff/undo guidance.

### Tasks

1. Rank by evidence, impact and staleness without auto-adopting.
2. Keep rejection rationale and prevent same-evidence reproposal.
3. Reopen only with materially new evidence/rationale.
4. Generate immutable ApplicationProposal with base/digest/path preconditions.
5. Validate affected skill/routing state before confirm.
6. Apply skill, Insight decision, incorporation mapping and receipt atomically.
7. Query local artifact → insight → finding → source revision.
8. Query changed finding → affected local artifacts.
9. Record outcome only from explicit evidence after meaningful use/review.
10. Provide operation-level diff and safe Git restore/revert guidance.

### Exit gate

- Changed base/path makes proposal stale and applies nothing.
- Apply updates all provenance/mapping records or none.
- Same evidence cannot silently reopen rejected Insight.
- Outcome is never inferred merely from apply/use.

## Dependencies

- Phase 09 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
