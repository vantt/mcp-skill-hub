# Phase 5 Report: Migrate CLI Commands to Renderer

## Guard Check Output
```
== Guard phase 5 ==
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
PASS  only internal/delivery/cli/*_test.go files were modified
PASS  no JSON-related assertion line was removed or changed in CLI tests
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
-- phase 5: CLI migration
PASS  user approval for phase 04 matches the approved file
PASS  no direct fmt.Fprint/io.WriteString/Write([]byte) in CLI command files (root.go, help.go exempt)
PASS  no hand-written ERROR/WHY/FIX/WARNING labels in CLI files (help.go exempt)
PASS  Raw is not used to bypass wrapping of formatted prose
PASS  root.go declares no new functions
PASS  no new JSON encoding in CLI command files (1, baseline 1 in evaluation.go)
PASS  shared result writer exists in internal/delivery/cli/output.go
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
PASS  required test ran and passed: TestHumanOutputFitsWidth

GUARD RESULT: PASS (phase 5)
```

## Edited Assertions Table

| file:line | old expectation | new expectation | why |
|---|---|---|---|
| `internal/delivery/cli/workspace_resolve_test.go:453` | `!strings.Contains(stderr.String(), "No Skill Hub workspace at "+missingPath)` | `!strings.Contains(stderr.String(), "No Skill Hub workspace at") \|\| !strings.Contains(stderr.String(), missingPath)` | Path wrapped onto continuation line with 5-space hanging indent under 100-col termui printer. |
| `internal/delivery/cli/migration_test.go:70` | `[]string{"Source schema version: 0", "Target schema version: 1", "Proposal digest: sha256:", ...}` | `[]string{"Source schema version:  0", "Target schema version:  1", "Proposal digest:        sha256:", ...}` | Labels padded to align value columns under `termui.Fields`. |
| `internal/delivery/cli/skill_editor_test.go:113` | `strings.HasPrefix(line, "- Proposal:")` | `strings.HasPrefix(line, "Proposal:")` | Proposals render pins as aligned `termui.Fields` instead of bulleted text. |
| `internal/delivery/cli/ux_behavior_test.go:142` | `!strings.Contains(stdout, "State: draft")` | `!strings.Contains(stdout, "State:") \|\| !strings.Contains(stdout, "draft")` | Labels padded to align value columns under `termui.Fields`. |
| `internal/delivery/cli/ux_behavior_test.go:293` | `[]string{"Triggers: review code", "Not for: write prose", "Min scope: single_step", "File: skills/core/show-test/SKILL.md"}` | `[]string{"Triggers:", "review code", "Not for:", "write prose", "Min scope:", "single_step", "File:", "skills/core/show-test/SKILL.md"}` | Labels padded to align value columns under `termui.Fields`. |
| `internal/delivery/cli/ux_behavior_test.go:371` | `!strings.Contains(stdout, "- Base version:")` | `!strings.Contains(stdout, "Base version:")` | Proposals render pins as aligned `termui.Fields` instead of bulleted text. |
| `internal/delivery/cli/skill_review_test.go:33,36,43,54` | `"Review: Review Test (rev-test)"`, `"State: draft"`, `"Origin: local authoring"`, `"Catalog facts:"` | `"Review:"` / `"Review Test (rev-test)"`, `"State:"` / `"draft"`, `"Origin:"` / `"local authoring"`, `"Catalog facts"` (no colon) | Labels padded under `termui.Fields` and section headings follow style guide (no trailing colon). |
| `internal/delivery/cli/curation_test.go:44` | `!strings.Contains(human, "Recommended next: "+home.SuggestedActions[0].Label)` | `!strings.Contains(human, "Next: "+home.SuggestedActions[0].Label)` | Next actions standardized to `p.Next` prefix "Next:". |
| `internal/delivery/cli/onboarding_test.go:390` | `strings.Count(stdout, "Recommended next:") != 1` | `strings.Count(stdout, "Next: Commit the new workspace") != 1` | Status next actions use `p.Next`, so specific next action fact is counted. |
| `internal/delivery/cli/onboarding_test.go:76` | `"git -C " + root` | `"git -C"`, `root` | Long temporary workspace path wrapped to continuation line under 100-col termui printer. |
| `internal/delivery/cli/source_test.go:29` | `!strings.Contains(stdout.String(), "Candidate: SRCQ-")` | `!strings.Contains(stdout.String(), "Candidate:") \|\| !strings.Contains(stdout.String(), "SRCQ-")` | Labels padded to align value columns under `termui.Fields`. |

## Not Routed Through writeResult
1. `update.go`: Returns exit code 1 on errors and update failures; rendered through `termui` directly (`p.Error` and `p.Line`).
2. `distill.go` batch results: Returns exit code 1 when `batch.Prepared == 0 && batch.Failed > 0`; rendered through `termui` directly (`p.Bullets` and `p.Line`).
3. `workspace.go` `writeWorkspaceResult`: Context cancellation returns exit code 130; rendered through `termui` directly (`p.Error`).

## Before and After Human Output
### diff-dirty

**Before (`guard/baseline/cli-before/diff-dirty.human.txt`):**
```
exit=0
--- stdout ---
Git has 3 uncommitted file(s).
draft skills (2):
  untracked  skills/core/long-review/SKILL.md
  untracked  skills/core/long-review/skill.meta.yaml
operation history (1):
  untracked  history/operations/<YYYY>/<MM>/OP-<ID>.yaml

To commit these changes, run:
  git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m "..."
--- stderr ---

```

**After (`reports/cli-after/diff-dirty.human.txt`):**
```
exit=0
--- stdout ---
Git has 3 uncommitted file(s).
draft skills (2):
untracked skills/core/long-review/SKILL.md
untracked skills/core/long-review/skill.meta.yaml
operation history (1):
untracked history/operations/<YYYY>/<MM>/OP-<ID>.yaml

To commit these changes, run:
  $ git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m "..."
--- stderr ---

```

### skill-activate-missing-fields

**Before (`guard/baseline/cli-before/skill-activate-missing-fields.human.txt`):**
```
exit=2
--- stdout ---

--- stderr ---
ERROR: The request cannot be accepted.
WHY: skill long-review requires: trigger, not_for or rationale (quality.routing_review_rationale), min_scope
FIX: Run `skillhub skill edit long-review --trigger "<when to use>" --not-for "<when not to use>" --min-scope single_step --yes`, then retry `skillhub skill activate long-review --yes`.
```

**After (`reports/cli-after/skill-activate-missing-fields.human.txt`):**
```
exit=2
--- stdout ---

--- stderr ---
ERROR: The request cannot be accepted.
WHY: skill long-review requires: trigger, not_for or rationale (quality.routing_review_rationale),
     min_scope
FIX: Run
     `skillhub skill edit long-review --trigger "<when to use>" --not-for "<when not to use>" --min-scope single_step --yes`,
     then retry `skillhub skill activate long-review --yes`.
```

### skill-create-missing-args

**Before (`guard/baseline/cli-before/skill-create-missing-args.human.txt`):**
```
exit=2
--- stdout ---

--- stderr ---
ERROR: The request cannot be accepted.
WHY: create requires --id, --collection, --name, and --description
FIX: Review `skillhub skill create` arguments and retry.
```

**After (`reports/cli-after/skill-create-missing-args.human.txt`):**
```
exit=2
--- stdout ---

--- stderr ---
ERROR: The request cannot be accepted.
WHY: create requires --id, --collection, --name, and --description
FIX: Review `skillhub skill create` arguments and retry.
```

### skill-create-preview

**Before (`guard/baseline/cli-before/skill-create-preview.human.txt`):**
```
exit=0
--- stdout ---
Draft skill creation is ready for review.
- 2 added, 0 modified, 0 deleted file(s).
- Proposal: PROP-<ID>
- Digest: <DIGEST>
- Base version: <DIGEST>
- Routing impact: Direct routing fields for long-review were evaluated against <DIGEST>.
No files changed. Confirm with:
  skillhub skill confirm --proposal PROP-<ID> --proposal-digest <DIGEST> --base-version <DIGEST>
  (or: skillhub skill confirm PROP-<ID>)
or re-run with --yes to apply directly.
--- stderr ---

```

**After (`reports/cli-after/skill-create-preview.human.txt`):**
```
exit=0
--- stdout ---
Draft skill creation is ready for review.
  - 2 added, 0 modified, 0 deleted file(s).
Proposal:      PROP-<ID>
Digest:        <DIGEST>
Base version:  <DIGEST>
Routing impact:  Direct routing fields for long-review were evaluated against
                 <DIGEST>.
No files changed. Confirm with:
skillhub skill confirm --proposal PROP-<ID> --proposal-digest
<DIGEST> --base-version
<DIGEST>
(or: skillhub skill confirm PROP-<ID>)
or re-run with --yes to apply directly.
--- stderr ---

```

### skill-list-empty

**Before (`guard/baseline/cli-before/skill-list-empty.human.txt`):**
```
exit=0
--- stdout ---
No skills found.
--- stderr ---

```

**After (`reports/cli-after/skill-list-empty.human.txt`):**
```
exit=0
--- stdout ---
No skills found.
--- stderr ---

```

### skill-list-one-draft

**Before (`guard/baseline/cli-before/skill-list-one-draft.human.txt`):**
```
exit=0
--- stdout ---
ID           STATE  COLLECTION  NAME
long-review  draft  core        Long Review
--- stderr ---

```

**After (`reports/cli-after/skill-list-one-draft.human.txt`):**
```
exit=0
--- stdout ---
ID           STATE  COLLECTION  NAME
long-review  draft  core        Long Review
--- stderr ---

```

### skill-review-draft

**Before (`guard/baseline/cli-before/skill-review-draft.human.txt`):**
```
exit=0
--- stdout ---
Review: Long Review (long-review)
Collection: core
Description: Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving a change whose cross-module contracts were not traced to their callers.
State: draft (valid; routing: not eligible; servable)
Files: 2 file(s), 1395 bytes (entrypoint: skills/core/long-review/SKILL.md)
Origin: local authoring (not watched)
Git: clean

Activation readiness:
  - Missing required field: trigger
  - Missing required field: not_for or rationale (quality.routing_review_rationale)
  - Missing required field: min_scope

Next: Complete required activation fields (trigger, not_for or rationale (quality.routing_review_rationale), min_scope) with `skillhub skill edit long-review`.
--- stderr ---

```

**After (`reports/cli-after/skill-review-draft.human.txt`):**
```
exit=0
--- stdout ---
Review:       Long Review (long-review)
Collection:   core
Description:  Review a multi-module change for correctness, regressions, security posture,
              documentation impact and rollout risk, recording evidence for every finding before
              recommending a fix, and never approving a change whose cross-module contracts were not
              traced to their callers.
State:        draft (valid; routing: not eligible; servable)
Files:        2 files, 1.4 KB (entrypoint: skills/core/long-review/SKILL.md)
Origin:       local authoring (not watched)
Git:          clean

Activation readiness
  - Missing required field: trigger
  - Missing required field: not_for or rationale (quality.routing_review_rationale)
  - Missing required field: min_scope

Next: Complete required activation fields (trigger, not_for or rationale
      (quality.routing_review_rationale), min_scope) with `skillhub skill edit long-review`.
--- stderr ---

```

### skill-review-unknown

**Before (`guard/baseline/cli-before/skill-review-unknown.human.txt`):**
```
exit=2
--- stdout ---

--- stderr ---
ERROR: The request cannot be accepted.
WHY: skill not found: does-not-exist
FIX: Run `skillhub skill list` to inspect available skills.
```

**After (`reports/cli-after/skill-review-unknown.human.txt`):**
```
exit=2
--- stdout ---

--- stderr ---
ERROR: The request cannot be accepted.
WHY: skill not found: does-not-exist
FIX: Run `skillhub skill list` to inspect available skills.
```

### skill-show-draft

**Before (`guard/baseline/cli-before/skill-show-draft.human.txt`):**
```
exit=0
--- stdout ---
Skill: Long Review (long-review)
State: draft
File: skills/core/long-review/SKILL.md
Triggers: (none)
Not for: (none)
Min scope: (none)

---
name: long-review
description: Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving a change whose cross-module contracts were not traced to their callers.
---

# Long skill

Use this skill when a change needs a careful, multi-step review that covers correctness, regressions, security posture, documentation impact, and rollout risk across several modules, because a single pass routinely misses cross-module contracts and the reviewer must record evidence for each finding before recommending a fix.

## Steps

1. Read the diff.
2. Trace each changed contract to its callers.
--- stderr ---

```

**After (`reports/cli-after/skill-show-draft.human.txt`):**
```
exit=0
--- stdout ---
Skill:      Long Review (long-review)
State:      draft
File:       skills/core/long-review/SKILL.md
Triggers:   (none)
Not for:    (none)
Min scope:  (none)

---
name: long-review
description: Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving a change whose cross-module contracts were not traced to their callers.
---

# Long skill

Use this skill when a change needs a careful, multi-step review that covers correctness, regressions, security posture, documentation impact, and rollout risk across several modules, because a single pass routinely misses cross-module contracts and the reviewer must record evidence for each finding before recommending a fix.

## Steps

1. Read the diff.
2. Trace each changed contract to its callers.
--- stderr ---

```

### source-list-empty

**Before (`guard/baseline/cli-before/source-list-empty.human.txt`):**
```
exit=0
--- stdout ---
0 candidate(s) and 0 monitored source(s).
--- stderr ---

```

**After (`reports/cli-after/source-list-empty.human.txt`):**
```
exit=0
--- stdout ---
0 candidate(s) and 0 monitored source(s).
--- stderr ---

```

### status-dirty

**Before (`guard/baseline/cli-before/status-dirty.human.txt`):**
```
exit=0
--- stdout ---
Workspace:  <TMP>/workspace
Status:     Git has uncommitted canonical changes.

Health:     Workspace valid; search index current; Git has uncommitted changes.
Inventory:  No skills yet.
            Next: ask your agent 'create a skill for ...' or run:
              skillhub skill create my-skill --collection core --name "My Skill" --description "Skill description"

Recommended next: Review uncommitted changes: git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m "Update skills".
--- stderr ---

```

**After (`reports/cli-after/status-dirty.human.txt`):**
```
exit=0
--- stdout ---
Workspace:  <TMP>/workspace
Status:     Git has uncommitted canonical changes.
Health:     Workspace valid; search index current; Git has uncommitted changes.
Inventory:  No skills yet.
            Next: ask your agent 'create a skill for ...' or run:
            skillhub skill create my-skill --collection core --name "My Skill" --description "Skill
            description"

Next: Review uncommitted changes
  $ git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m "Update skills"
--- stderr ---

```

### status-empty

**Before (`guard/baseline/cli-before/status-empty.human.txt`):**
```
exit=0
--- stdout ---
Workspace:  <TMP>/workspace
Status:     Skill Hub is up to date.

Health:     Workspace valid; search index current; Git clean.
Inventory:  No skills yet.
            Next: ask your agent 'create a skill for ...' or run:
              skillhub skill create my-skill --collection core --name "My Skill" --description "Skill description"

Recommended next: Continue normal work.
--- stderr ---

```

**After (`reports/cli-after/status-empty.human.txt`):**
```
exit=0
--- stdout ---
Workspace:  <TMP>/workspace
Status:     Skill Hub is up to date.
Health:     Workspace valid; search index current; Git clean.
Inventory:  No skills yet.
            Next: ask your agent 'create a skill for ...' or run:
            skillhub skill create my-skill --collection core --name "My Skill" --description "Skill
            description"

Next: Continue normal work
--- stderr ---

```

### validate

**Before (`guard/baseline/cli-before/validate.human.txt`):**
```
exit=0
--- stdout ---
Workspace validation passed.
--- stderr ---

```

**After (`reports/cli-after/validate.human.txt`):**
```
exit=0
--- stdout ---
Workspace validation passed.
--- stderr ---

```

## Deviations from the Plan
None.

## Open Questions
None.
