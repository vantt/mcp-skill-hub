---
title: "Phase 12 — MCP protocol and skill distribution"
status: done
---

# Phase 12 — MCP protocol and skill distribution

## Objective

Expose resolver, curation and progressive resource loading through MCP stdio.

### Deliverables

- MCP server lifecycle;
- `skill_resolve` and `skill_feedback`;
- standards-compatible skill manifest/list/get/resources;
- curation tools from the UX mapping;
- pagination and structured errors;
- snapshot/resource digest pinning;
- client capability negotiation where available.

### Tasks

1. Map MCP handlers directly to application commands/read models.
2. Implement request/response JSON Schemas and versioning.
3. Keep recommendation separate from distribution.
4. Return manifest before content and resources only on demand.
5. Ensure resolved snapshot and loaded digests match.
6. Return `snapshot_expired` instead of silently serving changed resource.
7. Bound payload sizes and paginate findings/diffs.
8. Add protocol conformance and malformed-input tests.
9. Test multiple stdio processes against one workspace lock/generation model.

### Exit gate

- CLI and MCP produce semantically equivalent results for shared commands.
- No MCP handler writes canonical files directly.
- Resolve → get → read preserves identity/version/digests.
- Stock-client compatibility matrix has no undocumented assumptions.

## Dependencies

- Phase 11 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
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
