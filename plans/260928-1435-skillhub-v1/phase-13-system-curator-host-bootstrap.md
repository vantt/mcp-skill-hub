---
title: "Phase 13 — Bundled System Curator Skill and host bootstrap"
status: todo
---

# Phase 13 — Bundled System Curator Skill and host bootstrap

## Objective

Make the UX usable through ordinary Agent Hosts without requiring command memorization.

### Deliverables

- embedded/versioned System Curator Skill;
- Curation Home dialogue;
- progressive evidence/diff disclosure;
- batch maintenance orchestration;
- bootstrap managed instruction block;
- MCP client registration remediation;
- full `doctor --fix` dependency-ordered plan.

### Tasks

1. Encode natural-language intents and tool selection guidance.
2. Enforce “one primary question” and approval matrix in skill instructions/fixtures.
3. Translate internal terminology to user-facing wording.
4. Orchestrate check → distill → inbox without applying active content.
5. Present interrupted work before optional work.
6. Bundle skill/tool compatibility metadata into binary.
7. Implement host-specific MCP registration adapters where stable.
8. Insert/update idempotent managed bootstrap blocks.
9. Clearly label instruction-only activation coordination as best effort.
10. Ensure CLI remains independent recovery path if MCP/Agent fails.

### Exit gate

- “Curate my Skill Hub” works end-to-end in each supported stock client.
- Normal user does not navigate entities/states manually.
- System skill never bypasses binary validation/mutation services.
- Re-running doctor fix does not duplicate registrations/instructions.

## Dependencies

- Phase 12 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
