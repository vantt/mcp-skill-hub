## Phase Implementation Report

### Executed Phase
- Phase: phase-05-skill-add-and-import-fidelity
- Plan: plans/261001-1139-curation-cli-intent-workflows
- Status: completed

### Files Modified
- `internal/app/source_import.go` (refactored to use shared `skill_discovery.go`, fixed BUG-04 folder-scoped companion drop, added BUG-16 license detection)
- `internal/app/source_import_test.go` (added regressions for BUG-04 folder-scoped companion preservation and BUG-16 license warnings)
- `plans/261001-1139-curation-cli-intent-workflows/phase-05-skill-add-and-import-fidelity.md` (updated status to completed and checked all todo items)

### Files Created
- `internal/app/skill_discovery.go` (shared deterministic skill discovery, companion inventory, frontmatter parsing/normalization, and license detection)
- `internal/app/skill_discovery_test.go` (unit tests for frontmatter preservation, malformed rejection, license detection, and companion discovery)
- `internal/app/skill_add.go` (direct draft skill addition service with immutable snapshot capture, privacy-safe origin, idempotency replay lookup before conflict checks, and Phase 3 proposal envelope integration)
- `internal/app/skill_add_test.go` (comprehensive regression tests for BUG-04, BUG-11, BUG-16, cache loss recovery, idempotency conflicts, selection semantics, and original mutation smoke proof)

### Tasks Completed
- [x] Extract shared deterministic skill discovery.
- [x] Fix folder-root companion handling in legacy import.
- [x] Use the shared kind-tagged skill proposal envelope and exact confirmation.
- [x] Look up replay receipts before ID conflicts.
- [x] Write privacy-safe direct-add origin without source records/links.
- [x] Enforce explicit selection/conflict semantics and complete resource inventory.
- [x] Surface license/resource inventory and transformations.
- [x] Add local/Git/cache-loss/idempotency/stale-proposal regressions.

### Tests Status
- Type check: pass (`go build ./internal/app`)
- Unit tests: pass (`go test ./internal/app -run 'Test.*(SkillAdd|SkillDiscovery|SourceImport|Companion|License|Idempotent)'`)
- Integration tests: pass (`go test -race ./internal/app -run 'Test.*(SkillAdd|SourceImport)'`)
- Smoke test: pass (`go test -v ./internal/app -run 'TestSkillAddSmokeMutateOriginalAfterPreview'`)

### Issues Encountered
None. All implementations aligned with Phase 1 canonical storage, Phase 3 proposal store and state facts, and Phase 4 locator snapshot capture.

### Next Steps
Phases 6 and 7 expose the skill add and source check application workflows to the CLI commands (`skillhub skill add ...`) and MCP server tools (`skill_add_preview` / `skill_add_confirm`).
