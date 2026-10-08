---
title: "Phase 16 — Post-core capabilities, evidence-gated"
status: done
---

# Phase 16 — Post-core capabilities, evidence-gated
## Objective

Evaluate optional post-core capabilities only after V1 usage and evaluation evidence demonstrates need.

Only start after V1 usage/evaluation demonstrates need.

### 15.1 Deep-dive mode

- targeted source area expansion;
- explicit budget/coverage;
- same finding/insight/mutation invariants.

### 15.2 Consult mode

- compare multiple sources for a curator question;
- pin all source revisions;
- create Comparison/Insight proposals, never direct active edits.

### 15.3 Additional source adapters

- non-Git APIs/living docs;
- opaque revisions/content digests;
- adapter-specific trust and replay limitations.

### 15.4 Vector retrieval

- only after FTS/rule error analysis;
- derived/disposable index;
- ablation proves quality gain worth complexity.

### 15.5 LLM fallback

- only for unresolved ambiguity;
- strict structured output;
- deterministic fallback behavior;
- privacy/cost budgets and evaluation.

### 15.6 Remote/multi-tenant mode

Requires separate design for authn/authz, tenancy, encryption, remote canonical storage and concurrency. Do not extrapolate local filesystem assumptions.

## Dependencies

- Phase 15 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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

## Gate decision

The phase was evaluated and deliberately closed without adding optional capability code. Current committed evidence does not demonstrate a V1 need for deep-dive mode, consult mode, additional adapters, vector retrieval, LLM fallback, or remote/multi-tenant operation:

- the 150-case calibration/held-out corpus passes its accepted resolver gates with deterministic FTS/rule retrieval;
- the committed real-CLI suite resolves all four expected outcomes;
- no error analysis or ablation demonstrates a vector/LLM quality gain worth additional authority, privacy, cost, or reproducibility complexity;
- no V1 usage evidence requires remote tenancy or another source adapter.

These capabilities remain out of the V1 product boundary. Reopening one requires a concrete measured failure population, an accepted scope, and its capability-specific privacy/cost/replay gate; it must not be inferred from this phase's completed status.

## Rollback

- Revert files touched by this phase using Git.
- If canonical workspace mutations were introduced, verify recovery can roll forward or restore the previous valid state before deleting runtime artifacts.
- Runtime databases and caches remain disposable and may be removed with `rm -rf runtime/` in test workspaces only.
