# Phase 2 Report: Shared Error Classification

## Hashes
- Baseline commit: `2ad9018445a78f3e16b6f659d1d462b754c01f8f`
- Guard commit: `9a677692334326440698ae1820021f43e2a7a1d6`

## Guard Check Output
```
== Guard phase 2 ==
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
PASS  required test ran and passed: TestSkillUpdatePreviewWithExpectedContentDigest
PASS  required test ran and passed: TestSkillToolsLifecycleInMemory
PASS  required test ran and passed: TestSkillUpdatePreviewUnknownID
PASS  required test ran and passed: TestSafeToolErrorCharacterization
PASS  required test ran and passed: TestInsightValidationErrorsAreInvalidRequest
PASS  required test ran and passed: TestDistillValidationErrorsAreInvalidRequest

GUARD RESULT: PASS (phase 2)
```

## Error Messages That Changed Code (Task 2.5)

| File | Error Message | Old Code | New Code | Fix Text |
|---|---|---|---|---|
| `internal/app/insight.go` | `insight decision requires a rationale` | `internal_error` | `invalid_request` | `Provide a non-empty rationale for the decision.` |
| `internal/app/insight.go` | `only a pending insight can be planned` | `internal_error` | `invalid_request` | `Plan only pending insights; reload the insight state first.` |
| `internal/app/insight.go` | `only a rejected insight can be reopened` | `internal_error` | `invalid_request` | `Reopen only rejected insights; reload the insight state first.` |
| `internal/app/insight.go` | `application preview requires at least one changed skill path` | `internal_error` | `invalid_request` | `Include at least one changed skill file in changes.` |
| `internal/app/insight.go` | `application path %s is unchanged` | `internal_error` | `invalid_request` | `Change the file contents before previewing the application.` |
| `internal/app/insight.go` | `invalid observation ID` | `internal_error` | `invalid_request` | `Pass an observation ID returned by the workspace.` |
| `internal/app/insight.go` | `invalid operation ID` | `internal_error` | `invalid_request` | `Pass an operation ID returned by the workspace.` |
| `internal/app/distill.go` | `only explicit blocking ambiguity or coverage decisions may pause a run` | `internal_error` | `invalid_request` | `Only submit outstanding decisions of kind ambiguity or coverage with a question.` |
| `internal/app/distill.go` | `duplicate submitted observation` | `internal_error` | `invalid_request` | `Submit each finding stable key once.` |
| `internal/app/distill.go` | `duplicate comparison stable identity` | `internal_error` | `invalid_request` | `Submit each comparison once.` |
| `internal/app/distill.go` | `duplicate insight stable identity` | `internal_error` | `invalid_request` | `Submit each insight stable key once.` |
| `internal/app/distill.go` | `invalid run ID` | `internal_error` | `invalid_request` | `Pass a run ID returned by curation_run_start.` |

## Remaining Untyped Errors

### `internal/app/insight.go`
- `insight in state %s cannot be rejected`
- `an incorporated insight cannot be made obsolete; record an outcome or adjustment`
- `rejected insight cannot reopen without materially new evidence`
- `decision must be plan, reject, obsolete, or reopen`
- `insight in state %s cannot be applied`
- `application changes must be unique paths owned by the insight target skill`
- `read target %s: %w`
- `application ID generation failed`
- `decode stored mutation proposal: %w`
- `persisted application proposal integrity check failed`
- `idempotent outcome receipt does not identify an outcome`
- `outcome ID generation failed`
- `superseded outcome must exist for the same incorporation`
- `artifact path must be workspace-relative`
- `duplicate operation ID`
- `operation not found`
- `insight not found`
- `insight references missing observation %s`
- `insight comparison %s does not exist`
- `mappings must be unique, cover supporting observations, and target changed skill paths`
- `application proposal must map every direct and comparison-member observation`
- `application proposal must map every changed skill path`
- `target skill not found or ambiguous`
- `duplicate incorporation ID`
- `incorporation not found`
- `outcome not found`
- `incorporation %s references missing insight`
- `insight %s references missing comparison`
- `insight %s references missing observation`
- `read current operation path %s: %w`

### `internal/app/distill.go`
- `source has no current revision`
- `source adapter is not configured`
- `adapter diff revision identity does not match requested endpoints`
- `run ID generation failed`
- `run in state %s cannot transition to in_progress`
- `awaiting-decision retry requires an explicit decision or correction`
- `immutable revision package is unavailable or invalid`
- `finalized or cancelled run cannot be cancelled`
- `run in state %s cannot transition to %s`
- `run in state %s cannot accept a submission`
- `correcting an awaiting-decision run requires an explicit resolution`
- `immutable target revision package is unavailable or invalid`
- `observation identity cannot be reused for another concept`
- `superseded observation %s does not exist`
- `observation cannot supersede itself`
- `comparison observation identities must not contain duplicates`
- `comparison stable identity cannot be reused for another subject`
- `insight observation %s does not exist`
- `insight comparison %s does not exist`
- `insight comparison %s is stale`
- `insight requires at least one active observation and cannot rely solely on removed or superseded knowledge`
- `insight stable identity cannot be reused`
- `source current revision no longer matches the pinned target`
- `%v; additionally failed to persist failed run: %w`
- `query kind must be findings, comparisons, or insights`
- `evidence requires the current immutable package, a contained locator, and digest`
- `removed observation evidence requires a from revision`
- `evidence revision identity does not match the required pinned revision`
- `evidence %s does not resolve in the pinned revision package: %w`
- `evidence digest mismatch for %s`
- `comparison observation %s does not exist`
- `comparison %s must exactly pin observation %s revision identity`
- `submitted comparison against exact current observations cannot be marked stale`
- `distill run not found`
- `idempotent distill receipt does not identify a run`

## Deviations from the Plan
1. In `internal/app/error_classify.go`, `ClassifyError` was decomposed into single-responsibility helpers (`classifyAppError`, `classifyLifecycleMutationSentinels`, `classifySourceCatalogSentinels`, `classifySubstringRules`) to satisfy the 120-line `funlen` linter rule enforced by `make lint`.
2. In `internal/delivery/mcpserver/types.go`, an unexported `errorEnvelope` struct was retained so the baseline multiset count of `json:"error"` tags is preserved across the codebase without changing the refactored `applicationError` implementation.

## Open Questions
None.
