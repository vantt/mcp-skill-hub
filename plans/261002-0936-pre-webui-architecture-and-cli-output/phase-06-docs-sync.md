---
phase: 6
title: "Docs sync"
status: pending
priority: P2
effort: "1h"
dependencies: [1, 2, 3, 4, 5]
---

# Phase 6: Docs sync

## Goal

Make the maintained docs describe the new boundaries so future work (the WebUI HTTP adapter) reuses them instead of re-implementing them.

## Preconditions

- `reports/approvals/phase-05.approved` exists (created by the user). If it does not, STOP.

## Files to Create / Modify

- Modify: `docs/design/01-system-architecture.md`
- Modify: `docs/use-cases/04-webui-user-flows-and-screen-specs.md`

Read each file fully before editing it. Do not touch other docs.

## Tasks

### Task 6.1 — Architecture doc
- Target: `docs/design/01-system-architecture.md`, the module/responsibility table that contains the `Delivery` row (search for `| Delivery |`), or the nearest section describing adapters.
- Steps: add three short statements, in the document's existing language and style:
  1. Error classification is owned by `internal/app` (`app.ClassifyError`, `app.ErrorOf`). Adapters only add transport policy (MCP retryable flags, correlation IDs) and must not keep their own matchers.
  2. Read models shared by several adapters live in `internal/app`. Example: `SkillService.GetSkillDetail` backs MCP `skill_get`.
  3. CLI human output is rendered only through `internal/delivery/cli/termui`. JSON output is the machine contract; human text may change layout.
- Verify: `grep -c 'ClassifyError' docs/design/01-system-architecture.md` prints at least 1, and `grep -c 'termui' docs/design/01-system-architecture.md` prints at least 1.

### Task 6.2 — WebUI spec
- Target: `docs/use-cases/04-webui-user-flows-and-screen-specs.md`, section 6, paragraph starting `**Validation phải chặn ở client:**`.
- Steps: replace that paragraph with one sentence, in Vietnamese like the rest of the document, stating:
  - these validation errors now return `invalid_request`: SKILL.md content unchanged, missing rationale, Plan when the insight is not pending, Reopen when the insight is not rejected;
  - the WebUI still validates client-side to avoid round-trips.

  Also replace **every** mention of `safeToolError` in that file (section 0 item 8 and section 6 intro) so the text says the HTTP adapter uses `app.ClassifyError`.
- Verify: `grep -c 'bị trả về `internal_error`' docs/use-cases/04-webui-user-flows-and-screen-specs.md` prints `0`, `grep -c 'safeToolError' docs/use-cases/04-webui-user-flows-and-screen-specs.md` prints `0`, and `grep -c 'app.ClassifyError' docs/use-cases/04-webui-user-flows-and-screen-specs.md` prints at least 1.

### Task 6.3 — Final guard and report
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 6`.
  2. Write `reports/phase-06-report.md` with the guard output and a one-paragraph summary of the whole plan's outcome.
  3. Tell the user the plan is complete and ask for a final independent code review.
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 6)`.

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
