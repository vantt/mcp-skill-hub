---
title: "Phase 11 — Evidence-first resolver"
status: done
---

# Phase 11 — Evidence-first resolver

## Objective

Resolve tasks against active curated skills with deterministic evidence and calibrated abstention.

### Deliverables

- request validation/normalization;
- FTS/rule candidate generation;
- requirement/exclusion tri-state logic;
- scoring and deterministic tie-breaking;
- `resolved`, `needs_context`, `no_skill` decisions;
- clarification questions;
- supporting-skill selection;
- explanation/reason codes;
- cache keyed by catalog/fact/activation context.

### Tasks

1. Implement rich structured request contract without requiring catalog taxonomy.
2. Separate task evidence from ambient repository facts.
3. Enforce hard incompatibility before score.
4. Add curated trigger/not-for/relationship signals.
5. Implement bounded clarification that can change decision.
6. Support canonical equivalent/near-duplicate policy.
7. Return one primary recommendation in normal path.
8. Keep vector retrieval and LLM rerank behind disabled interfaces.
9. Build initial golden corpus of roughly 150 cases/30–50 skills.
10. Calibrate thresholds from held-out cases, not anecdotes.

### Exit gate

- Golden evaluation meets agreed precision/abstention/ambiguity gates.
- No-skill and negation cases are explicitly covered.
- Same request + snapshot + policy + facts produces same deterministic result.
- Resolver never activates a skill.

## Dependencies

- Phase 10 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
