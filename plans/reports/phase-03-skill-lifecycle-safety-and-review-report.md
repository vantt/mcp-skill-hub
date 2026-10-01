## Phase Implementation Report

### Executed Phase
- Phase: phase-03-skill-lifecycle-safety-and-review
- Plan: plans/261001-1139-curation-cli-intent-workflows
- Status: completed

### Files Modified
- `internal/skill/lifecycle.go`: added `EditableContent`, `ReadEditableSkill`, `ExpectedContentDigest` to `UpdateInput`, `EditConflictError` and `ErrEditConflict`, `ScaffoldMarker` and `IsUntouchedScaffold`, activation guard rejecting untouched scaffolds, `ProposalKind` enum, `Kind` and `RecoveryID` fields to `Proposal`, atomic `BeforeDigest` verification under mutation planning's lock, and backward-compatible blind replacement when `ExpectedContentDigest` is omitted.
- `internal/skill/proposal_store.go`: added kind-tagged envelope serialization and deserialization, legacy proposal artifact compatibility (defaulting omitted kind to `lifecycle`), strict rejection of unknown proposal kinds or unrecognized fields, bounded editor recovery storage (`SaveEditorRecovery`, `ReadEditorRecovery`, `DeleteEditorRecovery`, `CleanupExpiredRecoveries`), and proposal dispatcher `ProposalDispatcher`.
- `internal/skill/proposal_store_test.go`: added test coverage for kind-tagged envelope round-trip, legacy artifact loading without kind, unknown kind rejection, unknown field rejection via `DisallowUnknownFields`, editor recovery lifecycle and TTL cleanup, and proposal dispatching by kind.
- `internal/skill/resource.go`: integrated with `catalog.OpenWithFallback` for manifest and resource reads.
- `internal/app/skill_lifecycle.go`: added `RecoveryID` to `SkillProposal`, corrected `ActiveLocally` to `(state == "active")` (BUG-13), added `LifecycleState` and `RoutingEligible`, mapped `EditConflictError` to typed `ErrorEditConflict`, added untouched scaffold check to `CheckActivationRequirements` and `previewTransition` (BUG-09), mapped resource digest mismatches in `ReadSkill` to typed `ErrorResourceContentUnavailable` without guessing historical bytes, and added `ConfirmProposal` (supporting short confirmation by proposal ID and explicit pins) and `DispatchConfirmProposal`.
- `internal/app/skill_lifecycle_test.go`: added assertions verifying `ActiveLocally` and `RoutingEligible` are false for drafts (BUG-13), added `TestPreviewSkillUpdateEditConflictPrecondition` (BUG-02 at application layer), `TestCheckActivationRequirementsBlocksUntouchedScaffold` (BUG-09), `TestConfirmProposalDispatcherByIDAndExplicitPins`, `TestEditorRecoveryLifecycleInService`, and `TestSkillListBasisAwareAndFallback`.
- `internal/app/skill_list.go`: migrated `ListSkills` to `catalog.OpenWithFallback` so listing remains resilient when canonical workspace has invalid files, and populated basis-aware projection fields `LifecycleState`, `ActiveLocally`, and `RoutingEligible` on `SkillListEntry`.
- `plans/261001-1139-curation-cli-intent-workflows/phase-03-skill-lifecycle-safety-and-review.md`: updated status to completed and marked all todo items.

### Files Created
- `internal/app/skill_review.go`: implemented `ReviewSkill` reading canonical skill files directly offline; collecting canonical validation issues, activation readiness, resource inventory and digest status, served-generation differences via `catalog.AssessSkillState`, provenance, Git staged/unstaged path summaries, and deterministic next actions without rebuilding catalog, fetching network content, or mutating state.
- `internal/app/skill_review_test.go`: added test coverage for `ReviewSkill` on untouched draft scaffolds, ready drafts, active skills, broken/missing catalogs, deprecated and archived skills, and verified `ReadSkill` returns typed `resource_content_unavailable` when live bytes differ from the served manifest.
- `internal/skill/lifecycle_test.go`: added test coverage for `ReadEditableSkill`, `PreviewUpdate` optimistic concurrency conflict detection and writer B preservation (BUG-02), blind replacement backward compatibility, untouched scaffold activation rejection (BUG-09), and genuine non-marker skill activation.

### Tasks Completed
- [x] Add digest-pinned editable reads and explicit blind-replacement compatibility.
- [x] Add a backward-compatible kind-tagged skill proposal envelope/dispatcher.
- [x] Associate bounded editor recovery IDs with persisted proposals.
- [x] Add a domain-level untouched-scaffold activation guard.
- [x] Add basis-aware current/served state facts and correct the deprecated active alias.
- [x] Implement catalog-independent `ReviewSkill`.
- [x] Cover concurrent writes, proposal-kind dispatch, and broken-catalog review.

### Tests Status
- Type check: pass (`go build ./internal/skill ./internal/app`)
- Unit tests: pass (`go test ./internal/skill -run 'Test.*(Update|Conflict|Transition|Scaffold|Proposal|Dispatch|Resource)'` and `go test ./internal/app -run 'Test.*Skill(Lifecycle|Review|List)|TestReviewSkill'`)
- Race detector: pass (`go test -race ./internal/skill` and `go test -race ./internal/app -run 'Test(ReviewSkill|SkillLifecycle|SkillList|PreviewSkillUpdate|CheckActivation|ConfirmProposal|EditorRecovery)'`)

### Issues Encountered
None. All tests passed with zero data races and complete backwards compatibility.

### Next Steps
- Phases 5 and 6 consume the kind-tagged proposal dispatcher and short proposal confirmation (`skillhub skill confirm PROPOSAL_ID`).
- Phase 6 consumes `ReviewSkill` for the `skillhub skill review <id>` CLI command.
- Phase 7 exposes the basis-aware state projection and review facts through MCP tool surfaces.
