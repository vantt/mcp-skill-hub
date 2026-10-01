## Phase Implementation Report

### Executed Phase
- Phase: Phase 1: Canonical authority and staged validation
- Plan: plans/261001-1139-curation-cli-intent-workflows/phase-01-canonical-authority-and-staged-validation.md
- Status: completed

### Files Modified
- `internal/canonical/canonical.go`: added `ValidationOptions`, `ValidateWithOptions`, `ValidateDetached`, updated `entityPath` to include `skill.meta.yaml`, and constrained YAML identity parsing strictly to canonical entity paths.
- `internal/canonical/skill.go`: centralized `SKILL.md` entrypoint validation and frontmatter-name rule into canonical validation, relaxed companion resources to bounded opaque files beneath skill root, and implemented backward-compatible structured origin validation under `skill.meta.yaml.provenance.origin`.
- `internal/canonical/canonical_test.go`: added coverage for frontmatter-name mismatch enforcement, clone-like workspace acceptance, companion resource bounds, detached validation, and structured origin validation.
- `internal/canonical/resource_limits_test.go`: added test verifying that 0-byte companion resources are valid while size bounds are enforced.
- `internal/workspace/workspace.go`: added `InspectWithOptions` with detached mode support, and made absent empty required directories valid for fresh clones while maintaining non-directory and symlink collision detection.
- `internal/workspace/workspace_test.go`: added tests for clone-like absent empty directories, collision detection, and detached mode repository omission.
- `internal/catalog/input.go`: added `isCanonicalEntityPath` so companion YAML files under skill directories are not misclassified or parsed as catalog entities.
- `internal/catalog/catalog_test.go`: added tests for clone-like builds, companion resource preservation without entity projection, and parity on frontmatter mismatch.
- `internal/mutation/mutation.go`: added `Context` field to mutation `Options`, wired context into lock acquisition in `CommitWithOptions`, and added early cancellation check in `commitWhileLocked`.
- `internal/mutation/transaction.go`: added pre-displacement context cancellation cleanup and post-displacement explicit recovery outcome enforcement.
- `internal/mutation/mutation_test.go`: added unit tests for cancellation waiting for lock, cancellation before displacement, and post-displacement durable recovery.
- `internal/app/errors.go`: added typed error codes (`ErrorAmbiguousLocator`, `ErrorAmbiguousRef`, `ErrorSkillSelectionRequired`, `ErrorSkillConflict`, `ErrorSourceConflict`, `ErrorResourceLimitsExceeded`, `ErrorSourceChanged`, `ErrorEditConflict`, `ErrorValidationFailed`, `ErrorLocalWatchUnsupported`, `ErrorResourceContentUnavailable`), constructors, and implemented the standard `error` interface on `*Error`.
- `internal/app/operations.go`: removed redundant frontmatter check from `ValidateWorkspace` in favor of `canonical.Validate`, unified validation result formatting with `formatValidationResult`, and implemented `GetGitPathSummary` for reusable path-safe staged and unstaged porcelain status summaries.
- `internal/app/operations_test.go`: added parity test asserting `ValidateWorkspace` adds no validation rules beyond `canonical.Validate`.
- `internal/app/git_index_snapshot.go` (new): implemented literal Git index stage-0 enumeration, blob materialization without smudge filters or checkout conversion, index identity cryptographic pinning, and detached canonical validation.
- `internal/app/git_index_snapshot_test.go` (new): added unit tests for opposite staged/worktree validation outcomes, smudge filter non-execution verification, index digest and status non-mutation, and path summary extraction.
- `schemas/skill-metadata.schema.json`: updated `provenance` and added `$defs/origin` supporting `github`, `git`, and `local` origins with strict path safety constraints.
- `schemas/error-envelope.schema.json`: added all 11 new typed error codes to the `code` enum.
- `schemas/embed_test.go`: added validation tests for legacy provenance, structured origin schemas, invalid local paths/labels, and new error envelope codes.

### Tasks Completed
- [x] Centralize all publication acceptance rules in canonical validation.
- [x] Accept absent empty canonical directories after clone without weakening unsafe-path checks.
- [x] Treat noncanonical skill companions as opaque bounded regular resources.
- [x] Add backward-compatible, privacy-safe structured origin validation.
- [x] Implement detached canonical validation and literal Git-index blob materialization.
- [x] Add typed shared errors and committed schema coverage.
- [x] Add context-aware pre-displacement cancellation semantics.
- [x] Add filter-execution, index-race, workspace/index non-mutation, and validator-parity tests.

### Tests Status
- Type check: pass
- Unit tests: pass (`go test -v ./internal/canonical ./internal/workspace ./internal/catalog -run 'Test.*(Validate|Input|Entity|Resource|Clone)'`, `go test -v ./internal/app -run 'Test.*(ValidateWorkspace|Staged|GitIndex|Error)'`, `go test -v ./internal/mutation -run 'Test.*(Cancel|Confirm|Recovery|Lock)'`, `go test -v ./schemas`)
- Integration tests: pass (end-to-end staged validation without Git filters and clone-like rebuilds verified)

### Issues Encountered
None. Coordinated error constants and type definitions with `phase2CatalogContinuity` and `phase4LocatorAndSourceWatch`.

### Next Steps
Phases 3 and 5 consume the frozen canonical/origin/resource contracts and reusable Git path summaries. Phase 6 consumes staged index validation.
