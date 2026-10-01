## Phase Implementation Report

### Executed Phase
- Phase: phase-07-mcp-workflow-surface
- Plan: plans/261001-1139-curation-cli-intent-workflows
- Status: completed

### Files Modified
- `internal/delivery/mcpserver/server.go`: Degraded startup without halting on EnsureCatalog failure; registered skill_add, source_watch, and skill_review tools; enriched distributionRPCError with structured resource_content_unavailable; enriched safeToolError with *app.Error Phase 1 error mapping, EditConflictError, and catalog unavailable handling; added enums for source_watch_preview.
- `internal/delivery/mcpserver/types.go`: Added DTOs for `skillAddPreviewInput`, `sourceWatchPreviewInput`, `skillReviewInput`; extended `skillUpdatePreviewInput` with `ExpectedContentDigest`; extended `skillGetResult` with `ContentDigest`, `StateBasis`, `LifecycleState`, `RoutingEligible`, `Diverged`, `ChangedResources`, `MissingResources`.
- `internal/delivery/mcpserver/skill_tools.go`: Computed `content_digest` and basis-aware state facts in `skill_get`; enforced non-empty pin validation in `skill_create_confirm` and `skill_transition_confirm`.
- `internal/delivery/mcpserver/insight_tools.go`: Forwarded `ExpectedContentDigest` in `skill_update_preview`; enforced non-empty pin validation in `skill_update_confirm` and `insight_apply_confirm`.
- `internal/delivery/mcpserver/server_test.go`: Added 5 new tools to `expectedToolAnnotations()` authority map; asserted exact 40-tool inventory length; verified schemas and annotations.
- `internal/delivery/mcpserver/skill_tools_test.go`: Asserted `content_digest` and basis facts in `skill_get`; added `TestSkillUpdatePreviewWithExpectedContentDigest` covering matching digest, stale conflict, and omitted blind replacement; added real content to stdio test to satisfy activation readiness.
- `internal/delivery/mcpserver/hardening_test.go`: Updated tool count assertion to use `len(expectedToolAnnotations())` (40).
- `internal/delivery/mcpserver/subprocess_test.go`: Added `TestStdioDegradedStartupWithValidFallback` and `TestStdioDegradedStartupWithNoCatalog` exercising real stdio transport with degraded catalog, offline diagnostics (hub_status, skill_review), locator rejection, pin enforcement, and tool isolation; added standalone build fallback for subprocess tests when peer edits cli package.
- `docs/mcp-compatibility-matrix.json`: Updated `generated_at` and added verified executed check for stdio degraded operations.
- `plans/261001-1139-curation-cli-intent-workflows/phase-07-mcp-workflow-surface.md`: Marked completed and checked off all todos.

### Files Created
- `internal/delivery/mcpserver/skill_add_tools.go`: Exposes `skill_add_preview` and `skill_add_confirm`; strictly rejects local filesystem and non-GitHub locators before enumeration.
- `internal/delivery/mcpserver/skill_add_tools_test.go`: Comprehensive regression tests for raw local locator rejection and missing/invalid pin rejection.
- `internal/delivery/mcpserver/source_watch_tools.go`: Exposes `source_watch_preview` and `source_watch_confirm`; verifies command and pins.
- `internal/delivery/mcpserver/source_watch_tools_test.go`: Tests local path rejection and pin validation.
- `internal/delivery/mcpserver/skill_review_tools.go`: Exposes `skill_review` for comprehensive offline read-only diagnostic review.
- `internal/delivery/mcpserver/skill_review_tools_test.go`: Tests offline execution without catalog, when catalog is missing, and when canonical metadata is corrupt.

### Tasks Completed
- [x] Add strict GitHub add/watch/review schemas and registration.
- [x] Reject raw local MCP locators before filesystem access.
- [x] Preserve exact-pin, proposal-kind-aware confirmations.
- [x] Add digest-aware update and basis-aware state outputs.
- [x] Keep MCP running for catalog-independent diagnostics in degraded/no-catalog states.
- [x] Migrate standard skill methods to resource-verified fallback behavior.
- [x] Map shared typed errors and consolidate the 40-tool inventory assertion.
- [x] Add in-memory/stdio regressions and record only executed compatibility evidence.

### Tests Status
- Type check: pass
- Unit tests: pass (`go test -v ./internal/delivery/mcpserver -run 'Test.*(SkillAdd|SourceWatch|SkillReview|Degraded|SkillTools|ToolSchemas|Annotations|ModernAndLegacy|SourceImport)'`)
- Race detector: pass (`go test -race ./internal/delivery/mcpserver -run 'Test.*(SkillAdd|SourceWatch|SkillReview|Degraded|SkillTools|ToolSchemas|Annotations|ModernAndLegacy|SourceImport)'`)

### Issues Encountered
None. All application contracts from Waves A, B, and C were consumed directly through application services without shims or duplication.

### Next Steps
Phase 8 reconciles and publishes updated user/protocol documentation after Phase 6 (CLI) and Phase 7 (MCP) complete.
