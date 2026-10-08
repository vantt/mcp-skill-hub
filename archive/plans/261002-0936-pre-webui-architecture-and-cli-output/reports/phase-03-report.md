# Phase 3 Report: Proposal Confirmation Helper and Service Splitting

## Guard Check Output
```
== Guard phase 3 ==
== Baseline commit: 2ad9018445a78f3e16b6f659d1d462b754c01f8f | Guard commit: 9a677692334326440698ae1820021f43e2a7a1d6 ==
== The user must confirm both hashes match the ones recorded when the plan was handed over. ==
PASS  guard files match pinned checksums
PASS  guard directory is committed and has no uncommitted or untracked changes
PASS  go build ./... exits 0
PASS  go vet ./... exits 0
PASS  go test -count=1 ./... exits 0
PASS  golangci-lint reports no new issues since the baseline commit
PASS  gofmt -l reports no files
PASS  no baseline test function was removed or renamed
PASS  t.Skip count unchanged (36)
PASS  no JSON field tag was removed or renamed
PASS  MCP tool name set unchanged
PASS  no forbidden patterns in added code (skip, nolint, build ignore, TODO/FIXME, not implemented, testing.Testing/Short, TestMain)
PASS  no added strings.Contains(x, "") assertions
PASS  no test file deleted
PASS  no existing test file modified (phases 0-4 may only add new test files)
PASS  assertions in baseline CLI test files did not drop (867 >= 867)
PASS  CLI test exit-code expectations unchanged
PASS  CLI --json output identical to baseline for all captured cases
PASS  CLI human output keeps exit codes and stdout/stderr routing
PASS  CLI human output still contains every identifier, placeholder, state, and path from baseline
-- phase 1: skill detail read model
PASS  internal/app/skill_detail.go exists
PASS  GetSkillDetail is a SkillService method
PASS  skill_detail.go owns AssessSkillState\(
PASS  skill_detail.go owns ReadSkillRationale\(
PASS  skill_detail.go owns ReadSkillRouting\(
PASS  skill_detail.go owns sha256\.Sum256\(
PASS  skill_detail.go owns BasisCanonical
PASS  skill_get handler calls GetSkillDetail
PASS  no mcpserver production file assesses skill state or reads routing/rationale
PASS  skill_tools.go no longer hashes content or finds the entrypoint
PASS  GetSkillDetail does not discard errors with blank identifiers
-- phase 2: shared error classification
PASS  internal/app/error_classify.go exists
PASS  ClassifyError defined
PASS  ErrorOf defined
PASS  Result.ApplicationError defined
PASS  safeToolError delegates to app.ClassifyError
PASS  safeToolError keeps no sentinel or substring rules
PASS  appResult/applicationError use app.ErrorOf
PASS  no substring marker list remains in delivery code
PASS  applicationError no longer JSON round-trips results
PASS  error_characterization_test.go is byte-identical to the plan fixture
PASS  insight.go: "insight decision requires a rationale" uses NewInvalidRequestError on the same line
PASS  insight.go: "insight decision requires a rationale" is no longer a plain error
PASS  insight.go: "only a pending insight can be planned" uses NewInvalidRequestError on the same line
PASS  insight.go: "only a pending insight can be planned" is no longer a plain error
PASS  insight.go: "only a rejected insight can be reopened" uses NewInvalidRequestError on the same line
PASS  insight.go: "only a rejected insight can be reopened" is no longer a plain error
PASS  insight.go: "application preview requires at least one changed skill path" uses NewInvalidRequestError on the same line
PASS  insight.go: "application preview requires at least one changed skill path" is no longer a plain error
PASS  insight.go: "application path %s is unchanged" uses NewInvalidRequestError on the same line
PASS  insight.go: "application path %s is unchanged" is no longer a plain error
PASS  insight.go: "invalid observation ID" uses NewInvalidRequestError on the same line
PASS  insight.go: "invalid observation ID" is no longer a plain error
PASS  insight.go: "invalid operation ID" uses NewInvalidRequestError on the same line
PASS  insight.go: "invalid operation ID" is no longer a plain error
PASS  distill.go: "only explicit blocking ambiguity or coverage decisions may pause a run" uses NewInvalidRequestError on the same line
PASS  distill.go: "only explicit blocking ambiguity or coverage decisions may pause a run" is no longer a plain error
PASS  distill.go: "duplicate submitted observation" uses NewInvalidRequestError on the same line
PASS  distill.go: "duplicate submitted observation" is no longer a plain error
PASS  distill.go: "duplicate comparison stable identity" uses NewInvalidRequestError on the same line
PASS  distill.go: "duplicate comparison stable identity" is no longer a plain error
PASS  distill.go: "duplicate insight stable identity" uses NewInvalidRequestError on the same line
PASS  distill.go: "duplicate insight stable identity" is no longer a plain error
PASS  distill.go: "invalid run ID" uses NewInvalidRequestError on the same line
PASS  distill.go: "invalid run ID" is no longer a plain error
PASS  failSubmission stores failureText(cause)
-- phase 3: proposal helper and service split
PASS  user approval for phase 02 matches the approved file
PASS  internal/app/proposal_confirm.go exists
PASS  verifyProposalPins defined with the planned signature
PASS  internal/app/skill_add.go calls verifyProposalPins
PASS  internal/app/source_watch.go calls verifyProposalPins
PASS  internal/app/source.go calls verifyProposalPins
PASS  internal/app/source_import.go calls verifyProposalPins
PASS  internal/app/insight.go calls verifyProposalPins
PASS  internal/app/skill_lifecycle.go calls verifyProposalPins
PASS  SkillAddService.PreviewSkillAdd is 78 lines (<= 80)
PASS  DistillService.SubmitDistillRun is 72 lines (<= 80)
PASS  SkillService.ReviewSkill is 66 lines (<= 80)
PASS  SourceService.PreviewSourceWatch is 54 lines (<= 80)
PASS  no app function over 120 lines was added or grew
PASS  no new internal/app function signature has more than 6 parameters
PASS  no package-level func literals in internal/app (they would hide step functions from length checks)
PASS  required test ran and passed: TestSkillUpdatePreviewWithExpectedContentDigest
PASS  required test ran and passed: TestSkillToolsLifecycleInMemory
PASS  required test ran and passed: TestSkillUpdatePreviewUnknownID
PASS  required test ran and passed: TestSafeToolErrorCharacterization
PASS  required test ran and passed: TestInsightValidationErrorsAreInvalidRequest
PASS  required test ran and passed: TestDistillValidationErrorsAreInvalidRequest

GUARD RESULT: PASS (phase 3)
```

## Split Methods and Extracted Functions

### 1. `SkillAddService.PreviewSkillAdd` (`internal/app/skill_add.go`)
- `SkillAddService.PreviewSkillAdd`: 78 lines (<= 80)
- `validateSkillAddInput`: 20 lines
- `resolveSkillAddSource`: 7 lines
- `captureLocalSkillAddSource`: 58 lines
- `captureRemoteSkillAddSource`: 116 lines
- `selectSkillAddCandidates`: 75 lines
- `buildSkillAddChanges`: 90 lines
- `lookupReplayedSkillAdd`: 48 lines
- `checkSkillAddConflicts`: 18 lines
- `planSkillAddProposal`: 93 lines

### 2. `DistillService.SubmitDistillRun` (`internal/app/distill.go`)
- `DistillService.SubmitDistillRun`: 72 lines (<= 80)
- `checkReplayedDistillRun`: 20 lines
- `gateDistillRunState`: 9 lines
- `DistillService.recordDistillSubmissionTelemetry`: 31 lines
- `DistillService.handleDistillBlockingDecisions`: 36 lines
- `DistillService.validateAndBuildObservations`: 102 lines
- `DistillService.validateAndBuildComparisons`: 60 lines
- `DistillService.validateAndBuildInsights`: 93 lines
- `DistillService.finalizeDistillRun`: 74 lines

### 3. `SkillService.ReviewSkill` (`internal/app/skill_review.go`)
- `SkillService.ReviewSkill`: 66 lines (<= 80)
- `locateSkillDir`: 26 lines
- `parseSkillReviewMeta`: 8 lines
- `inspectCanonicalEntrypoint`: 11 lines
- `checkCanonicalIssues`: 11 lines
- `checkActivationReadiness`: 26 lines
- `inventorySkillResources`: 40 lines
- `assessServedSkillFacts`: 11 lines
- `extractSkillProvenance`: 25 lines
- `getSkillGitSummary`: 29 lines
- `appendSkillReviewItems`: 37 lines

### 4. `SourceService.PreviewSourceWatch` (`internal/app/source_watch.go`)
- `SourceService.PreviewSourceWatch`: 54 lines (<= 80)
- `validateSourceWatchLocator`: 18 lines
- `resolveSourceWatchRoute`: 53 lines
- `deriveSourceWatchConfig`: 49 lines
- `checkExistingSourceCollisions`: 56 lines
- `buildSourceWatchRecord`: 45 lines
- `buildAndStoreSourceWatchProposal`: 36 lines
- `appendSourceSizeWarnings`: 26 lines

## Documented Exception: `ConfirmSourceWatch` Ordering Change
In `ConfirmSourceWatch` (`internal/app/source_watch.go`), when both the proposal digest and another pin differ, `verifyProposalPins` reports the digest mismatch first (`proposalDigestMismatch`), whereas the previous code reported pins mismatch (`pins.ProposalID != ... || pins.BaseVersion != ...`) first. This behavior change was documented in the plan (Task 3.2, rule 3) as the single allowed behavior adjustment in phase 3.

## Deviations from the Plan
None.

## Open Questions
None.
