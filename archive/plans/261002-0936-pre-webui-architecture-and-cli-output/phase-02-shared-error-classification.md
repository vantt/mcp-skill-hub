---
phase: 2
title: "Shared error classification"
status: completed
priority: P1
effort: "7h"
dependencies: [1]
---

# Phase 2: Shared error classification

## Goal

Give every delivery adapter one application-owned way to turn a service outcome into a stable `*app.Error`, delete the duplicate classifiers in the adapters, and fix the validation errors currently misreported as `internal_error`. Every existing error code returned by MCP or the CLI must stay the same, except the listed misclassified errors, which become `invalid_request`.

## Context

- Two error channels exist and both stay:
  - (a) a returned `error`;
  - (b) a result whose embedded `app.Result` has a non-nil `Error`, returned with a nil error (about 137 `ErrorResult(...)` sites).
- `internal/delivery/mcpserver/server.go`:
  - `safeToolError(err error) toolError` (~line 478) classifies errors: `*app.Error` first, then `errors.Is`/`errors.As` sentinels, then substring rules, then `knownRequestError(message)` (~line 598), then `internal_error` with a correlation ID.
  - For an `*app.Error` it overrides `Retryable` to true only for codes `stale_context`, `snapshot_expired`, `source_unavailable`, and `index_stale`.
  - `applicationError(value any) *app.Error` (~line 708) finds channel (b) by JSON-marshalling the value. `appResult` (~line 722) uses it.
- `internal/delivery/cli/skill.go` (~lines 571-660, `writeSkillErrorFor` and helpers) re-implements part of this mapping with its own `errors.Is` and substring checks to choose CLI wording.
- `internal/delivery/cli/insight.go` `writeInsightResult` prints `err.Error()` as WHY for every error.
- `internal/app/distill.go` `failSubmission` stores `cause.Error()` in the persisted run's `Failure` field. `*app.Error.Error()` returns `Render.Error + ": " + Render.Why`, which would change persisted text unless handled (Task 2.5).

## Files to Create / Modify

- Create: `internal/delivery/mcpserver/error_characterization_test.go`. It must be an **exact copy** of `plans/261002-0936-pre-webui-architecture-and-cli-output/guard/fixtures/error_characterization_test.go.txt`; the guard compares bytes.
- Create: `internal/app/error_classify.go`
- Create: `internal/app/insight_validation_errors_test.go`
- Create: `internal/app/distill_validation_errors_test.go`
- Modify: `internal/app/result.go`, `internal/app/insight.go`, `internal/app/distill.go`
- Modify: `internal/delivery/mcpserver/server.go`
- Modify: `internal/delivery/cli/insight.go`, `internal/delivery/cli/distill.go` (only their `err != nil` branches)

Do not modify `internal/delivery/cli/skill.go` in this phase: its error helpers choose CLI wording and the cancellation exit code (130), not error codes, and its behavior is pinned by existing tests.

## Tasks

### Task 2.1 — Pin current behavior BEFORE changing code
- Goal: freeze today's MCP error codes with a planner-written test.
- Steps:
  1. Run `cp plans/261002-0936-pre-webui-architecture-and-cli-output/guard/fixtures/error_characterization_test.go.txt internal/delivery/mcpserver/error_characterization_test.go`.
  2. Do not edit the copied file, ever.
  3. Commit it alone: `test(mcpserver): pin tool error classification`.
- Success criteria: the test passes on unmodified production code.
- Verify: `go test -count=1 -run TestSafeToolErrorCharacterization ./internal/delivery/mcpserver/` exits 0. This must pass **before** Task 2.2. If it fails here, STOP: it means production code already changed.

### Task 2.2 — Create `app.ClassifyError`, `app.ErrorOf`, and `Result.ApplicationError`
- Goal: one classifier in the application layer.
- Target: `internal/app/error_classify.go`, `internal/app/result.go`.
- Steps:
  1. In `result.go`, add `func (r Result) ApplicationError() *Error { return r.Error }` with a doc comment. Value receiver, so it is promoted to every result type that embeds `Result`.
  2. In `error_classify.go`, add `func ClassifyError(err error) *Error`:
     - `nil` → return `nil`.
     - `errors.As(err, &appErr)` with `*Error` → return that same `*Error` pointer unchanged. Do not copy it or change `Retryable`.
     - Otherwise apply, in the same order as `safeToolError` today, every sentinel, `errors.As`, and substring rule, plus the `knownRequestError` marker list (move that function into this file as unexported `knownRequestMessage`).
     - Each rule returns `&Error{Code: <same code>, Retryable: <same flag safeToolError sets>, Render: ErrorRender{Error: <same Message string>, Fix: <same SuggestedAction string>}}`.
     - For `*MissingActivationRequirementsError`, keep the same message and suggested action.
     - The final fallback returns code `internal_error`, `Retryable: true`, message `The operation failed internally.`, and fix `Retry once; if the failure persists, use the correlation ID with stderr diagnostics.`.
     - For `mutation.ErrWorkspaceBusy`, use message `The workspace lock could not be acquired.` with code `internal_error`.
  3. Add `func ErrorOf(value any, err error) *Error`:
     - If `err != nil`, return `ClassifyError(err)`.
     - Otherwise, if `value` implements `interface{ ApplicationError() *Error }`, return that result.
     - Otherwise return `nil`.
  4. Add a comment above the substring rules: they exist only for errors that are not yet typed; new code must return `*Error` or a sentinel.
- Success criteria: compiles; `app` gains no new imported package outside those `internal/app` already imports. Check with `go list -f '{{join .Imports "\n"}}' ./internal/app` before and after.
- Verify: `go build ./internal/app/` exits 0.

### Task 2.3 — Rewire the MCP adapter
- Goal: MCP delegates classification and keeps only adapter policy.
- Target: `internal/delivery/mcpserver/server.go` (`safeToolError`, `applicationError`, `appResult`, delete `knownRequestError`).
- Steps:
  1. Rewrite `safeToolError(err)` as:
     - `appErr := app.ClassifyError(err)`;
     - build a `toolError{Code: string(appErr.Code), Message: appErr.Render.Error, SuggestedAction: appErr.Render.Fix}`;
     - if `Message` is empty, use `appErr.Error()`; if `SuggestedAction` is empty, use `Check request parameters and retry.` (same defaults as today);
     - set `Retryable` exactly as today: for an error that was an `*app.Error` originally, true only for `stale_context`, `snapshot_expired`, `source_unavailable`, `index_stale`; for every other path, `appErr.Retryable`;
     - for code `internal_error`, call the existing `internalToolError(message)` so the correlation ID and diagnostics logging still happen.
  2. Delete `knownRequestError` from `server.go`.
  3. Replace the body of `applicationError(value)` with `return app.ErrorOf(value, nil)` (or inline `app.ErrorOf` in `appResult` and delete `applicationError`). Remove the JSON round-trip.
- Success criteria: `server.go` contains `app.ClassifyError(` and `app.ErrorOf(`; no `func knownRequestError`; no `json.Unmarshal(data, &envelope)`.
- Verify: `go test -count=1 ./internal/delivery/mcpserver/` exits 0 (this includes `TestSafeToolErrorCharacterization`).

### Task 2.4 — No parity or helper unit tests
- Goal: avoid duplicate proofs. The pinned characterization test at the MCP boundary owns the error-code contract, and the guard forbids `safeToolError` from keeping its own rules, so a second table test against `app.ClassifyError` would re-assert the same contract. `ErrorOf` is exercised by the existing MCP tests that return errors through `Result.Error` (for example skill add conflicts).
- Steps: add no test file in this task.
- Verify: `go test -count=1 ./internal/delivery/mcpserver/ ./internal/app/` exits 0.

### Task 2.5 — Type the misclassified validation errors
- Goal: request and state validation failures become `invalid_request` instead of `internal_error`.
- Target and steps:
  1. In `internal/app/insight.go`, replace each of these `errors.New(...)`/`fmt.Errorf(...)` calls with `NewInvalidRequestError(<same message text>, <fix>)`. Keep the message literal on the same line as `NewInvalidRequestError(`. For formatted messages, use `NewInvalidRequestError(fmt.Sprintf("<same format>", args...), <fix>)`. Use these fixes:

     | Message (exact) | Fix text |
     |---|---|
     | `insight decision requires a rationale` | `Provide a non-empty rationale for the decision.` |
     | `only a pending insight can be planned` | `Plan only pending insights; reload the insight state first.` |
     | `only a rejected insight can be reopened` | `Reopen only rejected insights; reload the insight state first.` |
     | `application preview requires at least one changed skill path` | `Include at least one changed skill file in changes.` |
     | `application path %s is unchanged` | `Change the file contents before previewing the application.` |
     | `invalid observation ID` | `Pass an observation ID returned by the workspace.` |
     | `invalid operation ID` | `Pass an operation ID returned by the workspace.` |

  2. In `internal/app/distill.go`, do the same for:

     | Message (exact) | Fix text |
     |---|---|
     | `only explicit blocking ambiguity or coverage decisions may pause a run` | `Only submit outstanding decisions of kind ambiguity or coverage with a question.` |
     | `duplicate submitted observation` | `Submit each finding stable key once.` |
     | `duplicate comparison stable identity` | `Submit each comparison once.` |
     | `duplicate insight stable identity` | `Submit each insight stable key once.` |
     | `invalid run ID` | `Pass a run ID returned by curation_run_start.` |

  3. Keep persisted and batch failure text unchanged. Add a helper `failureText(cause error) string` in `distill.go`: it returns `appErr.Render.Why` when `errors.As(cause, &appErr)` succeeds and `Why` is non-empty; otherwise `cause.Error()`.
     - In `failSubmission`, write `run.Failure = boundedDistillMessage(failureText(cause), 2000)`. The guard checks for the literal `failureText(cause)`. Leave the returned error as `cause`.
     - In `SubmitDistillRuns` (the batch loop) and in `sanitizeDistillError` (~line 1265), make the per-item `Error` text come from `failureText(err)` instead of `err.Error()`, so batch results keep today's text.
  4. Do not change any other error site. List the remaining untyped `errors.New`/`fmt.Errorf` sites in `insight.go` and `distill.go` in the phase report under "remaining untyped errors".
- Verify: `go test -count=1 ./internal/app/` exits 0.

### Task 2.6 — Tests for the typed errors
- Steps:
  1. `internal/app/insight_validation_errors_test.go`, `TestInsightValidationErrorsAreInvalidRequest`, using `workspaceWithPendingInsight` from `internal/app/insight_test.go`. It covers:
     - `DecideInsight` with an empty rationale;
     - `DecideInsight` with decision `plan` on an insight first moved to `rejected`;
     - `DecideInsight` with decision `reopen` on a pending insight;
     - `PreviewInsightApplication` with empty `Changes`;
     - `PreviewInsightApplication` whose single change has the current file content (read it from disk first).

     For each case assert `ClassifyError(err).Code == ErrorInvalidRequest` and that `err.Error()` contains the original message text.
  2. `internal/app/distill_validation_errors_test.go`, `TestDistillValidationErrorsAreInvalidRequest`, using `newDistillWorkspace` and `prepareAndStart` from `internal/app/distill_test.go`. Cover:
     - a submission with two findings sharing one stable key: assert `invalid_request`, and that the persisted run (`GetDistillRun`) has `State == "failed"` and `Failure == "duplicate submitted observation"`;
     - `GetDistillRun` with run ID `"../bad"`: assert `invalid_request`.
- Verify: `go test -count=1 -run 'ValidationErrorsAreInvalidRequest' ./internal/app/` exits 0.

### Task 2.7 — Render typed errors in the CLI insight and distill commands
- Goal: the CLI shows the new typed errors cleanly. Without this, WHY would read `The request cannot be accepted.: <message>`.
- Target: `internal/delivery/cli/insight.go` (`writeInsightResult`, `err != nil` branch) and `internal/delivery/cli/distill.go` (`writeDistill`, `err != nil` branch).
- Steps:
  1. In each branch, first try `var appErr *app.Error; if errors.As(err, &appErr) { return writeStructuredError(stdout, stderr, jsonOutput, appErr) }` (`writeStructuredError` is in `resolver.go`). Otherwise keep the current code path unchanged.
  2. Do not touch any other CLI file.
- Success criteria: existing CLI tests pass unmodified.
- Verify: `go test -count=1 ./internal/delivery/cli/` exits 0.

### Task 2.8 — Guard, report, and HUMAN GATE
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 2`.
  2. Write `reports/phase-02-report.md`. Include:
     - the baseline and guard commit hashes printed at the top of the guard output;
     - the guard output;
     - the "remaining untyped errors" list;
     - a table of every error message that changed code (old code → new code), which must match Task 2.5 exactly.
  3. Commit the report.
  4. **STOP and ask the user to review `reports/phase-02-report.md`.** Approval is recorded **by the user** with `sha256sum <plan>/reports/phase-02-report.md | cut -d' ' -f1 > <plan>/reports/approvals/phase-02.approved`. Never create or edit files in `reports/approvals/` yourself. The phase 3 guard fails without that file, and editing the report afterwards invalidates it.
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 2)`.

## Failure Protocol
If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP and report the same
failure evidence to the user. Never continue by self-reasoning.
