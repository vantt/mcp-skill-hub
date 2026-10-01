# Phase 4 Implementation Report: Locator Resolution and Source Watch

## Executed Phase
- Phase: phase-04-locator-and-source-watch
- Plan: plans/261001-1139-curation-cli-intent-workflows
- Status: completed

## Files Modified
- `internal/source/types.go` (added `ErrAmbiguousRef`, `ErrAmbiguousLocator`, `ErrLocalWatchUnsupported`, `ErrUnsafeFile`, `AmbiguousRefError`, and snapshot fields to `Locator`)
- `internal/source/policy.go` (added `URLValidationOptions` and `ValidateRemoteURLWithOptions`)
- `internal/source/git_repository.go` (added `AllowFileProtocol`, tag mirroring and peeling in `resolveCommit`, `AdvertisedRef`, `ListAdvertisedRefs`, `ResolveRefCommit`, and `CommitHasSkill`)
- `internal/source/filesystem.go` (added `CaptureAuthorizedRoot`, refactored `captureDirectory`, snapshot digest handling in `CurrentRevision`, and wrapped errors with `ErrUnsafeFile`)
- `internal/source/records.go` (updated `ValidateLocator` to handle snapshot locators and safe source locator paths)
- `internal/source/nofollow_other.go` (updated build tags for `!windows`)
- `internal/app/source.go` (fixed BUG-01 monitoring re-enable override, fixed BUG-10 tree/blob triage URL parsing, fixed BUG-11 `normalizeCapturedLocator` path handling)
- `internal/app/source_test.go` (added regression tests for BUG-01, BUG-10, and BUG-11)
- `plans/261001-1139-curation-cli-intent-workflows/phase-04-locator-and-source-watch.md` (marked status completed and todos checked)

## Files Created
- `internal/source/locator.go` (`AuthorizedLocalRoot` with serialization protection, `SnapshotReference`, `NormalizedLocator`, structural GitHub URL parsing, advertised ref resolution)
- `internal/source/locator_test.go` (unit tests for authorized root serialization blocking, structural GitHub routes, ref resolution, tag peeling, collision detection, and privacy scanning)
- `internal/source/nofollow_windows.go` (Windows-specific handle-verified traversal rejecting reparse points and path swaps via `GetFinalPathNameByHandle`)
- `internal/source/nofollow_windows_test.go` (Windows nofollow unit tests)
- `internal/app/source_watch.go` (`PreviewSourceWatch` and `ConfirmSourceWatch` application services with policy-aware idempotency)
- `internal/app/source_watch_test.go` (comprehensive tests for local watch rejection, GitHub preview/confirm, idempotency, conflicts, and BUG-01)

## Tasks Completed
- [x] Add separate transient (`AuthorizedLocalRoot`) and serializable (`SnapshotReference`, `NormalizedLocator`) locator contracts.
- [x] Resolve and fetch advertised heads and tags, including slash refs, annotated tags, deletion, and ambiguity.
- [x] Add safe external local immutable capture with workspace-private cache and no host path persistence.
- [x] Add Windows reparse-safe traversal and swap-race tests.
- [x] Fix explicit no-monitor handling (BUG-01).
- [x] Implement persisted source-watch preview and confirm with policy-aware idempotency.
- [x] Add URL, path, policy, collision, idempotency, and no-write rejection tests.

## Tests Status
- Type check: pass (both Linux and Windows targets compile cleanly via `go build` and `GOOS=windows go build`)
- Unit tests: pass (`go test ./internal/source -run 'Test.*(Locator|Git|Filesystem|Snapshot|Symlink|Limit|Record|Proposal)'` and `go test ./internal/app -run 'Test.*(SourceWatch|SourceOnboarding|Monitoring)'`)
- Race detector: pass (`go test -race ./internal/source` and `go test -race ./internal/app -run 'Test.*(SourceWatch|SourceOnboarding|Monitoring|LocalWatch)'`)

## Issues Encountered
None. All error code contracts were coordinated with Phase 1, and no file conflicts occurred.

## Next Steps
Phase 5 consumes the normalized locator and snapshot layer for skill add and companion fidelity.
Phases 6 and 7 expose source watch and check intent commands in CLI and MCP surfaces.
