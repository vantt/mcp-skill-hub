# Phase 4 Report: CLI Terminal Renderer Package

## Guard Check Output
```
== Guard phase 4 ==
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
-- phase 4: termui package
PASS  internal/delivery/cli/termui/termui.go exists
PASS  golang.org/x/term is a direct dependency
PASS  termui imports no internal package
PASS  termui emits no ANSI escapes
PASS  reports/termui-samples.txt is exactly what TestWriteSamples renders
PASS  required test ran and passed: TestSkillUpdatePreviewWithExpectedContentDigest
PASS  required test ran and passed: TestSkillToolsLifecycleInMemory
PASS  required test ran and passed: TestSkillUpdatePreviewUnknownID
PASS  required test ran and passed: TestSafeToolErrorCharacterization
PASS  required test ran and passed: TestInsightValidationErrorsAreInvalidRequest
PASS  required test ran and passed: TestDistillValidationErrorsAreInvalidRequest
PASS  required test ran and passed: TestWrapKeepsLongTokensIntact
PASS  required test ran and passed: TestWrapKeepsBacktickSpansIntact
PASS  required test ran and passed: TestFieldsAlignLabels
PASS  required test ran and passed: TestErrorBlockOrder
PASS  required test ran and passed: TestCommandLinesAreNeverWrapped
PASS  required test ran and passed: TestBytesHumanized
PASS  required test ran and passed: TestTableAlignsColumns
PASS  required test ran and passed: TestWidthClamping

GUARD RESULT: PASS (phase 4)
```

## Rendered Output Samples (`reports/termui-samples.txt`)
```
===== width 100 =====
Review:       Long Review (long-review)
Collection:   core
Description:  Review a multi-module change for correctness, regressions, security posture,
              documentation impact and rollout risk, recording evidence for every finding before
              recommending a fix, and never approving a change whose cross-module contracts were not
              traced to their callers.
State:        draft (valid; routing: not eligible; servable)
Files:        2 file(s), 1.4 KB (entrypoint: skills/core/long-review/SKILL.md)
Origin:       local authoring (not watched)
Git:          clean

Activation readiness
  - Missing required field: trigger
  - Missing required field: not_for or rationale (quality.routing_review_rationale)
  - Missing required field: min_scope

Next: Complete required activation fields (trigger, not_for or rationale
      (quality.routing_review_rationale), min_scope) with `skillhub skill edit long-review`.

Workspace:  <TMP>/workspace
Status:     Git has uncommitted canonical changes.
Health:     Workspace valid; search index current; Git has uncommitted changes.
Inventory:  No skills yet.

Next: Review uncommitted changes
  $ git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m "Update skills"

ERROR: The request cannot be accepted.
WHY: skill long-review requires: trigger, not_for or rationale (quality.routing_review_rationale),
     min_scope
FIX: Run
     `skillhub skill edit long-review --trigger "<when to use>" --not-for "<when not to use>" --min-scope single_step --yes`,
     then retry `skillhub skill activate long-review --yes`.

ID                           STATE   COLLECTION  NAME
long-review                  draft   core        Review a multi-module change for correctness,
                                                 regressions, security posture, and docs
git-expert                   active  core        Git expert operations and repository analysis
super-comprehensive-auditor  active  community   A very comprehensive skill with a super long
                                                 description name that easily exceeds ninety
                                                 characters in length

===== width 60 =====
Review:       Long Review (long-review)
Collection:   core
Description:  Review a multi-module change for correctness,
              regressions, security posture, documentation
              impact and rollout risk, recording evidence
              for every finding before recommending a fix,
              and never approving a change whose
              cross-module contracts were not traced to
              their callers.
State:        draft (valid; routing: not eligible; servable)
Files:        2 file(s), 1.4 KB (entrypoint:
              skills/core/long-review/SKILL.md)
Origin:       local authoring (not watched)
Git:          clean

Activation readiness
  - Missing required field: trigger
  - Missing required field: not_for or rationale
    (quality.routing_review_rationale)
  - Missing required field: min_scope

Next: Complete required activation fields (trigger, not_for
      or rationale (quality.routing_review_rationale),
      min_scope) with `skillhub skill edit long-review`.

Workspace:  <TMP>/workspace
Status:     Git has uncommitted canonical changes.
Health:     Workspace valid; search index current; Git has
            uncommitted changes.
Inventory:  No skills yet.

Next: Review uncommitted changes
  $ git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m "Update skills"

ERROR: The request cannot be accepted.
WHY: skill long-review requires: trigger, not_for or
     rationale (quality.routing_review_rationale), min_scope
FIX: Run
     `skillhub skill edit long-review --trigger "<when to use>" --not-for "<when not to use>" --min-scope single_step --yes`,
     then retry `skillhub skill activate long-review --yes`.

ID                           STATE   COLLECTION  NAME
long-review                  draft   core        Review a
                                                 multi-module
                                                 change for
                                                 correctness,
                                                 regressions,
                                                 security
                                                 posture,
                                                 and docs
git-expert                   active  core        Git expert
                                                 operations
                                                 and
                                                 repository
                                                 analysis
super-comprehensive-auditor  active  community   A very
                                                 comprehensive
                                                 skill with
                                                 a super
                                                 long
                                                 description
                                                 name that
                                                 easily
                                                 exceeds
                                                 ninety
                                                 characters
                                                 in length
```

## Deviations from the Plan
None.

## Open Questions
None.
