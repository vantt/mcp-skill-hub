## Phase Implementation Report

### Executed Phase
- Phase: phase-02-catalog-continuity-and-status
- Plan: plans/261001-1139-curation-cli-intent-workflows
- Status: completed

### Files Modified
- `internal/catalog/catalog.go`: added ServingMode enum (`current`, `fallback`, `unavailable`), extended `Status` with `ServingMode`, `Generation`, `Warning`, and updated `Handle` to carry `Status`.
- `internal/catalog/open.go`: added lock/retry state machine in `OpenWithFallback` / `OpenWithFallbackLocked` (and `OpenServable` / `OpenServableLocked` aliases), atomic status and pointer pinning under shared lock, strict `EnsureFreshOrRebuild`, and fallback only when canonical validation fails.
- `internal/catalog/servable.go`: added `StateBasis`, `CanonicalSkillFacts`, `ServedSkillFacts`, `SkillStateAssessment`, and `AssessSkillState` / `AssessSkillStateWhileLocked` for basis-aware state projection and resource verification.
- `internal/catalog/generation_lookup.go`: filtered candidate generations by `derived_schema_version = DerivedSchemaVersion` to exclude incompatible generations.
- `internal/catalog/open_test.go`: added test coverage for `OpenWithFallback` serving current when healthy, serving fallback when canonical is invalid, auto-rebuilding when canonical is valid, rejecting corrupt generation when canonical is invalid, and locking retention.
- `internal/catalog/servable_test.go`: added test coverage for `AssessSkillState` with healthy skills, invalid canonical metadata, and changed live resources.
- `internal/app/catalog.go`: added `InspectCatalog` and `AssessSkill` to `CatalogService`.
- `internal/app/catalog_test.go`: added test coverage for `InspectCatalog` and `AssessSkill`.
- `internal/app/distribution.go`: migrated to `OpenWithFallbackLocked`, defined `ErrResourceContentUnavailable`, and added live resource digest checks across all distributed resources of active skills in `buildDistributedSkill`.
- `internal/app/distribution_test.go`: added tests for serving unchanged fallback skills when canonical has invalid files, and rejecting changed/tampered live resources with `ErrResourceContentUnavailable`.
- `internal/app/resolver.go`: migrated `Resolve` to `OpenWithFallbackLocked`, appended fallback degraded warnings to `response.Warnings`, and isolated/excluded skills whose live resources are unservable or unavailable.
- `internal/app/resolver_continuity_test.go`: added `TestResolverContinuityDuringInvalidCanonicalEdit` covering BUG-07 (unrelated invalid file does not disable unchanged skills; modified live entrypoint excludes skill and fails get/read with `ErrResourceContentUnavailable`) and updated corrupt catalog tests.
- `internal/app/curation_home.go`: added internal unexported-from-JSON `CountsKnown bool` to `CurationHome`, marked catalog categories unavailable when workspace health is invalid, and ensured counts are never treated as known zero when health is invalid (BUG-08).
- `internal/app/curation_home_test.go`: added `TestGetCurationHomeNeverReportsCountsKnownWhenInvalid` asserting that invalid workspaces have `CountsKnown == false` and prioritize repair guidance.
- `plans/261001-1139-curation-cli-intent-workflows/phase-02-catalog-continuity-and-status.md`: updated status to completed and checked all todo items.

### Tasks Completed
- [x] Extend existing catalog status with atomic serving-mode/pointer pinning.
- [x] Preserve strict rebuild while adding verified resource-aware fallback reads.
- [x] Migrate resolver and distribution consumers.
- [x] Publish basis-aware servable/routing-state assessment.
- [x] Correct unknown-count semantics without changing the v1 JSON shape.
- [x] Add canonical-state race, changed-resource, corruption, and isolation tests.

### Tests Status
- Type check: pass (`go build ./internal/catalog ./internal/app`)
- Unit tests: pass (`go test ./internal/catalog -run 'Test.*(Open|Status|Fallback|Servable|Integrity|Race)'` and `go test ./internal/app -run 'Test.*(Catalog|ResolverContinuity|Distribution|CurationHome)'`)
- Integration tests: pass (`TestResolverContinuityDuringInvalidCanonicalEdit`, `TestGetCurationHomeNeverReportsCountsKnownWhenInvalid`)

### Issues Encountered
None. Coordinated with concurrent peer `phase1CanonicalAuthority` on error interface and import resolution.

### Next Steps
- Phase 3 consumes `AssessSkillState` and basis-aware projection facts for review and mutation results.
- Phase 6 consumes `CountsKnown` and truthful status rendering.
