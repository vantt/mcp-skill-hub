---
phase: 7
title: "MCP workflow surface"
status: completed
priority: P1
effort: "3-4d"
dependencies: [1, 2, 3, 4, 5]
---

# Phase 7: MCP workflow surface

## Context Links

- [Curation lifecycle shared-service requirement](../../../docs/design/05-curation-lifecycle.md#1-product-decision)
- [Current MCP compatibility matrix](../../../docs/mcp-compatibility-matrix.json)
- [Independent CLI-to-web mapping](./reports/curation-ux-cli-independent-evaluation.md#8-cli-mcp-and-future-web-mapping)

## Objective

Expose focused add/watch/review workflows through strict MCP tools backed by the same application services, preserve exact-pin mutation confirmation, and enrich skill reads/updates with digest and explicit state facts.

## Tool Contract

Add:

- `skill_add_preview`
- `skill_add_confirm`
- `source_watch_preview`
- `source_watch_confirm`
- `skill_review`

Extend compatibly:

- `skill_get`: include content digest and basis-aware current/served state facts.
- `skill_update_preview`: accept optional `expected_content_digest`. Supplying it selects safe read-modify-write semantics; omission retains documented blind-replacement compatibility.

MCP confirm tools MUST require `proposal_id`, `proposal_digest`, and `base_version`. The CLI short-confirm convenience is not exposed through MCP. `skill_add_preview` accepts public GitHub locators only; raw relative/home/absolute local filesystem locators are rejected before enumeration because MCP has no host-granted file-selection capability.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/server.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/types.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/skill_tools.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/insight_tools.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/source_tools.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/server_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/skill_tools_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/source_import_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/hardening_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/subprocess_test.go`
- `/home/vantt/projects/mcp-skill-hub/docs/mcp-compatibility-matrix.json`

Create:

- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/skill_add_tools.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/skill_add_tools_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/source_watch_tools.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/source_watch_tools_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/skill_review_tools.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/mcpserver/skill_review_tools_test.go`

No other phase edits these files.

## Implementation Steps

1. Add strict input/output DTOs that mirror application contracts without duplicating defaults/domain validation. Keep `additionalProperties: false`, bounded arrays/strings, and portable schemas.
2. Register focused tools with accurate annotations. Review is read-only/closed-world; add/watch previews are open-world network operations but non-destructive; confirmations are destructive/idempotent and persisted.
3. Reject every local-filesystem add locator at the MCP adapter before app capture. Do not expose an allow-root flag or token system in this release.
4. Require all confirmation pins and load the exact proposal by ID/kind. Omitted/mismatched pins return typed error before mutation.
5. Forward optional expected digest. Document omission as blind replacement; return `edit_conflict` with no proposal when supplied digest is stale. Return `content_digest` from the same bytes as content.
6. Return basis-aware current/served skill facts and fallback diagnostics. Map Phase 1 typed errors; remove new string-based classifications.
7. Change MCP startup so canonical validation failure does not prevent tool registration. Use a verified fallback when available; catalog-independent tools (`hub_status`, validation, review) remain callable without one. Catalog-dependent calls fail individually on missing/corrupt/incompatible state.
8. For standard `skills/list/get/resources/read`, omit changed-resource skills, retain unchanged fallback skills, log degraded state to stderr, and return structured unavailable errors for changed requested resources.
9. Keep existing source/skill tools compatible. No MCP orchestration chains old primitives to imitate new workflows.
10. Use one expected-tool map as inventory authority; assert its length (40 after five additions) rather than separate 33/35 literals. Verify names, annotations, schemas, error codes, and pins.
11. Update the compatibility matrix only with checks actually executed.

## Todo

- [x] Add strict GitHub add/watch/review schemas and registration.
- [x] Reject raw local MCP locators before filesystem access.
- [x] Preserve exact-pin, proposal-kind-aware confirmations.
- [x] Add digest-aware update and basis-aware state outputs.
- [x] Keep MCP running for catalog-independent diagnostics in degraded/no-catalog states.
- [x] Migrate standard skill methods to resource-verified fallback behavior.
- [x] Map shared typed errors and consolidate the 40-tool inventory assertion.
- [x] Add in-memory/stdio regressions and record only executed compatibility evidence.

## Success Criteria

- Tool discovery includes exactly the five new tools with correct schemas/annotations and one authoritative 40-name inventory.
- GitHub add/watch previews are non-mutating; raw local add is refused before path enumeration. Confirmations apply only exact persisted write sets.
- Confirmation without any one pin fails and changes no canonical/operational state.
- `skill_review` and other catalog-independent diagnostics work when rebuild is blocked and when no valid catalog exists.
- Unchanged skills remain available through verified fallback; changed resources are omitted/unavailable, never served from guessed bytes.
- `skill_get` digest matches returned bytes; supplied stale digest fails, while omitted digest retains documented blind-replacement compatibility.
- Existing tool behavior and protocol negotiation remain compatible.

## Verification

```bash
go test ./internal/delivery/mcpserver -run 'Test.*(SkillAdd|SourceWatch|SkillReview|Degraded|SkillTools|ToolSchemas|Annotations|ModernAndLegacy)'
go test -race ./internal/delivery/mcpserver
```

Exercise actual stdio twice: invalid canonical with a valid fallback, and invalid canonical with no catalog. In both cases tools register and review/status run. Also prove raw local add and omitted-pin confirmation fail without filesystem enumeration or writes.

## Risks and Security

- MCP inputs are untrusted. Preserve strict schemas, runtime validation, resource limits, URL policy, and path confinement.
- Do not expose host filesystem paths without an explicit host-granted capability; none is in scope.
- Degraded startup must not make corrupt/incompatible catalogs servable or weaken per-tool availability checks.
- Tool descriptions must not imply host approval, activation, background monitoring, or automatic source updates.
- Compatibility matrix timestamps/evidence are facts, not aspirational documentation.

## Next Step

Run in parallel with Phase 6; Phase 8 reconciles docs only after both land.