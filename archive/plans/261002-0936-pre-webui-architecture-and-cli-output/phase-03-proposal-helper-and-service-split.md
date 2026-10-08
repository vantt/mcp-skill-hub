---
phase: 3
title: "Proposal confirmation helper and service splitting"
status: completed
priority: P2
effort: "7h"
dependencies: [2]
---

# Phase 3: Proposal confirmation helper and service splitting

## Goal

Replace the duplicated proposal-pin checks with one helper, and split four oversized service methods into named single-purpose steps, **without changing behavior**. One documented edge case is the only exception (Task 3.2).

## Context

Duplicated confirmation checks (expired, digest mismatch, pins mismatch), each with its own wording and result type:

| File | Function | Current order | Zero `expiresAt` means |
|---|---|---|---|
| `internal/app/skill_add.go` (~700-720) | `SkillAddService.ConfirmSkillAdd` | expired → digest → pins | not expired |
| `internal/app/source_watch.go` (~328-344) | `SourceService.ConfirmSourceWatch` | expired → pins(ID, base) → digest | expired |
| `internal/app/source.go` (~407-421) | `SourceService.ConfirmSourceProposal` | expired → digest → pins | expired |
| `internal/app/source_import.go` (~334-348) | `SourceImportService.ConfirmSourceImport` | expired → digest → pins | expired |
| `internal/app/insight.go` (~518-530) | `InsightService.ConfirmInsightApplication` | digest → pins(ID, base); expiry handled by `insightpkg.LoadRuntimeProposal` | n/a |
| `internal/app/skill_lifecycle.go` (~195-212) | `SkillService.ConfirmSkillMutation` | see the code around the `digestMismatchSkillProposal`/`staleSkillProposal` calls | read the code |

Each site keeps its own result type and user-facing wording. Only the *decision* is shared.

Oversized methods (baseline lengths): `SkillAddService.PreviewSkillAdd` 541 lines, `DistillService.SubmitDistillRun` 399, `SkillService.ReviewSkill` 309, `SourceService.PreviewSourceWatch` 285.

## Preconditions

- `reports/approvals/phase-02.approved` exists (created by the user). If it does not, STOP; the guard fails without it.

## Files to Create / Modify

- Create: `internal/app/proposal_confirm.go`
- Modify: `internal/app/skill_add.go`, `source_watch.go`, `source.go`, `source_import.go`, `insight.go`, `skill_lifecycle.go`, `distill.go`, `skill_review.go`

Extracted step functions stay in the **same file** as the method they came from.

## Tasks

### Task 3.1 — Create the shared decision helper
- Target: `internal/app/proposal_confirm.go`.
- Steps:
  1. Add exactly this API (doc comments required):
     ```go
     // proposalRefusal names why supplied confirmation pins cannot confirm a reviewed proposal.
     type proposalRefusal int

     const (
         proposalAccepted proposalRefusal = iota
         proposalExpired
         proposalDigestMismatch
         proposalPinsMismatch
     )

     // verifyProposalPins decides whether supplied pins confirm the reviewed proposal.
     // Checks run in this order: expiry, digest, remaining pins.
     // zeroExpiryIsExpired preserves each caller's policy for proposals without an expiry time;
     // pass checkExpiry=false when the caller has already enforced expiry.
     func verifyProposalPins(now, expiresAt time.Time, checkExpiry, zeroExpiryIsExpired bool, expected, supplied ConfirmationPins) proposalRefusal
     ```
  2. Implement it in at most 25 lines.
- Verify: `go build ./internal/app/` exits 0.

### Task 3.2 — Use the helper at all six sites
- Steps, for each file in the Context table:
  1. Replace the inline expired/digest/pins comparisons with one `switch verifyProposalPins(...)`. Each `case` returns exactly the result the old branch returned: same `Summary`, `Error`, `Why`, `Fix`, `Items`, `SuggestedActions`, and `Confirmation`.
  2. Keep every other check (stored-proposal reload, canonical replan, `mutation.ErrConflict` handling) at the call site, unchanged.
  3. **Documented exception:** in `ConfirmSourceWatch`, when both the digest and another pin differ, the helper now reports the digest mismatch first, where the old code reported the pins mismatch. This is the only allowed behavior change in this phase. Record it in the report.
- Success criteria: each of the six files calls `verifyProposalPins(`; no file still compares `pins.ProposalDigest` with an expected digest outside `proposal_confirm.go`.
- Verify: `grep -nE 'ProposalDigest *!= *|!= *[a-zA-Z.]*ProposalDigest' internal/app/skill_add.go internal/app/source_watch.go internal/app/source.go internal/app/source_import.go internal/app/skill_lifecycle.go` prints nothing, and `go test -count=1 ./internal/app/ ./internal/delivery/...` exits 0.

### Task 3.3 — No new tests
- Goal: this phase is behavior-preserving. Each service's existing confirm tests (stale, expired, and wrong-pin cases) own the confirmation contract, so a unit test of the private helper would duplicate them (`test-audit` junk pattern: private predicate tests duplicated at real boundaries).
- Steps: add no test file.
- Verify: `go test -count=1 ./internal/app/ ./internal/delivery/...` exits 0.

### Task 3.4 — Split `SkillAddService.PreviewSkillAdd` (`internal/app/skill_add.go`)
- Goal: the method becomes an orchestrator of at most 80 lines that calls named steps in order.
- Steps:
  1. Read the whole method first and list its phases in the report.
  2. Extract one unexported function or method per phase, named by what it does: for example `validateSkillAddInput`, `resolveSkillAddSource` (local vs git), `discoverSkillAddCandidates`, `selectSkillAddCandidates`, `buildSkillAddChanges`, `planSkillAddProposal`, `assembleSkillAddProposal`.
  3. If several steps share many values, introduce one small private struct (for example `skillAddRequest`) holding the resolved inputs. Do not create a struct that holds every local variable of the old method.
  4. Early returns that produce `SkillAddProposal{Result: ErrorResult(...)}` must keep exactly the same error constructor, WHY, and FIX text.
- Rules:
  - Each extracted function is at most 120 lines and has at most 6 parameters. `make lint LINT_BASE=<baseline commit>` enforces both (funlen, revive argument-limit) on changed code.
  - Extract named functions or methods only; do not assign function literals to package-level `var`s (the guard forbids `var x = func`).
  - No extracted function may be called from only one place *and* be just a numbered continuation of the previous one (no `partOne`/`partTwo`).
- Verify: `go test -count=1 ./internal/app/ ./internal/delivery/...` exits 0.

### Task 3.5 — Split `DistillService.SubmitDistillRun` (`internal/app/distill.go`)
- Same rules as Task 3.4. Suggested steps:
  - load and gate the run state;
  - load the package and normalize the submission;
  - validate and build findings (observations);
  - validate and build comparisons;
  - validate and build insights;
  - handle blocking decisions (`awaiting_decision`);
  - finalize atomically;
  - emit telemetry.
- Keep the deferred telemetry behavior identical: the same events, fields, and conditions.
- Verify: `go test -count=1 ./internal/app/ ./internal/delivery/...` exits 0.

### Task 3.6 — Split `SkillService.ReviewSkill` (`internal/app/skill_review.go`)
- Same rules as Task 3.4. Suggested steps:
  - locate the skill;
  - canonical validity and issues;
  - activation readiness;
  - resource status;
  - served facts and divergence;
  - provenance;
  - Git summary;
  - next action.
- Note: an existing helper in this file already has a 13-entry parameter list (it is in the guard baseline). Do not add parameters to it.
- Verify: `go test -count=1 ./internal/app/ ./internal/delivery/...` exits 0.

### Task 3.7 — Split `SourceService.PreviewSourceWatch` (`internal/app/source_watch.go`)
- Same rules as Task 3.4. Suggested steps:
  - validate the locator (local path rejection included);
  - resolve the GitHub route and ref;
  - derive the source ID and monitoring policy;
  - detect conflicts or identical existing records;
  - build the record and proposal.
- Verify: `go test -count=1 ./internal/app/ ./internal/delivery/...` exits 0.

### Task 3.8 — Guard and report
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 3`.
  2. Write `reports/phase-03-report.md`. Include:
     - the guard output;
     - for each split method, the list of extracted functions with line counts (from `awk` or the guard's function-length output);
     - the documented `ConfirmSourceWatch` ordering change.
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 3)`.

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
