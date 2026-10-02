---
phase: 5
title: "Migrate CLI commands to the renderer"
status: pending
priority: P1
effort: "8h"
dependencies: [2, 4]
---

# Phase 5: Migrate CLI commands to the renderer

## Goal

Every CLI command renders human output through `termui` and returns through one shared outcome writer, so output is wrapped, aligned, and consistent. JSON output, exit codes, and stdout/stderr routing do not change.

## Preconditions

- `reports/approvals/phase-04.approved` exists (created by the user). If it does not, STOP; the guard fails without it.
- Phase 2 is complete (CLI error classification uses `app.ClassifyError`).

## Context

Human-output code lives in these non-test files of `internal/delivery/cli/`:
`catalog.go, connect.go, curation.go, distill.go, evaluation.go, insight.go, mcp.go, migration.go, resolver.go, skill.go, skill_add.go, skill_editor.go, skill_review.go, source.go, source_watch.go, telemetry.go, telemetry_lifecycle.go, update.go, validate_staged.go, version.go, workspace.go, workspace_env.go, workspace_resolve.go, root.go`.

Error paths stay as they are: most commands report errors through `writeInvalidRequest` (code `invalid_request`, WHY = `err.Error()`, exit 2) and cancellation through exit 130. This phase changes only how those helpers *print* human text, never which code, WHY, or exit they produce.
`help.go` (static usage text) is exempt.

Baseline output to preserve or improve: `plans/261002-0936-pre-webui-architecture-and-cli-output/guard/baseline/cli-before/`.

## Style guide (apply consistently)

1. **Result summary first:** the service `Summary` sentence via `p.Line`.
2. **Detail pages** (`skill show`, `skill review`, `status`, proposals): one `p.Fields(...)` block, so labels align within the block. Sizes use `termui.Bytes`; counts use `termui.Plural`.
3. **Sections:** `p.Heading("Activation readiness")`, sentence case, no trailing colon, followed by `p.Bullets` or `p.Fields`.
4. **Lists:** `p.Table` with the same column headers the command prints today (for example `ID STATE COLLECTION NAME`).
5. **Next action:** `p.Next(label, command)`. The command is never embedded inside the sentence; for example status prints `Next: Review uncommitted changes` followed by `  $ git -C <path> add -A && git -C <path> commit -m "Update skills"`.
6. **Proposals:**
   - detected values in `Fields`;
   - pins as `Fields{"Proposal", id}, {"Digest", d}, {"Base version", v}`;
   - then `p.Next("Confirm the reviewed proposal", <exact confirm command printed today>)`.
7. **Warnings and errors:** warnings use `p.Warning` on the stream they use today. Errors use `p.Error(what, why, fix)` on the stream they use today (normally stderr).
8. **Raw content** (diffs, file contents, JSON fragments): `p.Raw`.
9. **Information parity:** every identifier, path, state, count, and command printed today must still be printed. The guard checks identifiers and paths mechanically. You may drop only duplicated text, such as a skill name repeated as its own ID.

## Files to Create / Modify

- Create: `internal/delivery/cli/output.go`, `internal/delivery/cli/human_output_width_test.go`
- Modify: every non-test file listed in Context (except `help.go`), and `internal/delivery/cli/*_test.go` only for expected human text (see the rules below).

## Rules for editing existing CLI tests (this is the only phase where it is allowed)

- You may change an expected **human-output** string only when the new renderer legitimately changes layout (wrapping, alignment, label wording, humanized bytes).
- The new expectation must assert the same fact: the same ID, state, value, or command. Do not shorten an expectation to a generic word.
- Never change an expected exit code, an expected stream (stdout vs stderr emptiness), or any JSON expectation. The guard counts exit-code expectations and compares JSON output byte-for-byte.
- For every edited assertion, add a row to the report table `file:line | old expectation | new expectation | why`.

## Tasks

### Task 5.1 — Shared result writer for successful values
- Target: `internal/delivery/cli/output.go`, exactly this signature:
  ```go
  // writeResult is the single exit path for a command value that was returned without a Go error.
  //   JSON mode:  writeJSON(stdout, value); returns 2 when app.ErrorOf(value, nil) != nil, else 0;
  //               returns 1 when writing fails.
  //   human mode: when app.ErrorOf(value, nil) != nil, prints its ERROR/WHY/FIX with termui to stderr
  //               and returns 2; otherwise calls render(termui.New(stdout)) and returns 0
  //               (1 if the printer recorded a write error).
  func writeResult(stdout, stderr io.Writer, jsonOutput bool, value any, render func(p *termui.Printer)) int
  ```
- Rules:
  - `writeResult` never handles a Go `error`. Callers keep their current error writers (`writeInvalidRequest`, `writeStructuredError`, `writeSkillError`, `writeSourceError`, the 130 cancellation path, and so on).
  - Do not add new functions to `root.go`. The guard freezes its function set; new helpers go in `output.go`.
  - Move only the *human* branch of `writeInvalidRequest` and `writeStructuredError` onto `termui.Printer.Error`. Their JSON branches and exit codes stay byte-identical.
- Verify: `go test -count=1 ./internal/delivery/cli/` exits 0. No separate unit test: `writeResult` is covered by the command tests and the guard's JSON capture (see the Testing section of `AGENTS.md`).

### Task 5.2 — Migrate commands, one file per commit
- Steps, for each file in Context order:
  1. Replace every `fmt.Fprint*`, `io.WriteString`, and hand-written `ERROR:`/`WHY:`/`FIX:`/`WARNING:` print with `termui` calls, following the style guide.
  2. Where a command's current success path (JSON document, exit code, stream) matches `writeResult`, route it through `writeResult`. Where it does not, for example `update.go` (exit 1 paths) or `distill.go` batch results (exit 1 when a batch item fails), keep its exact behavior, render through `termui` directly, and list it in the report under "not routed through writeResult" with the reason.
  3. Run `go test -count=1 ./internal/delivery/cli/`. If tests fail only because of human layout, update the expectations under the rules above. If any JSON, exit-code, or stream expectation fails, you changed behavior: revert that change.
  4. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/capture-cli-output.sh /tmp/cli-now && diff <(cat plans/261002-0936-pre-webui-architecture-and-cli-output/guard/baseline/cli-before/*.json.txt) <(cat /tmp/cli-now/*.json.txt)`. The output must be empty.
  5. Run `make lint LINT_BASE=$(cat plans/261002-0936-pre-webui-architecture-and-cli-output/guard/baseline/base_commit.txt)`. It must report `0 issues.`.
  6. Commit: `refactor(cli): render <command> output through termui`.
- Verify (after the last file):
  - `grep -nE 'fmt\.Fprint|io\.WriteString|\.Write\(\[\]byte' $(ls internal/delivery/cli/*.go | grep -v _test.go | grep -vE '/(root|help)\.go$')` prints nothing;
  - `grep -nE '"(ERROR|WHY|FIX|WARNING): ' $(ls internal/delivery/cli/*.go | grep -v _test.go | grep -v '/help\.go$')` prints nothing.

### Task 5.3 — Width test
- Target: `internal/delivery/cli/human_output_width_test.go`, `TestHumanOutputFitsWidth`.
- Steps:
  1. Use `initTestWorkspace` (`onboarding_test.go`) and `runCLI`. Set `SKILLHUB_WORKSPACE` with `t.Setenv`.
  2. Create a draft skill with the 280-character description used in `guard/capture-cli-output.sh`.
  3. Run `status`, `skill list`, `skill review <id>`, `skill activate <id> --yes` (an expected error), `source list`, and `validate`. Do **not** include `skill show`: it prints the raw `SKILL.md` through `Raw`, which is never wrapped by design.
  4. Assert that every line of stdout and stderr is at most 100 runes, except lines starting with `  $ ` and lines consisting of one unbreakable token (a single word, or a backtick-quoted span).
  5. Do not call `t.Parallel()` in this test: it uses `t.Setenv`.
- Verify: `go test -count=1 -run TestHumanOutputFitsWidth ./internal/delivery/cli/` exits 0.

### Task 5.4 — Before/after evidence, guard, report, and HUMAN GATE
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/capture-cli-output.sh plans/261002-0936-pre-webui-architecture-and-cli-output/reports/cli-after`.
  2. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 5`.
  3. Write `reports/phase-05-report.md` with:
     - the guard output;
     - the edited-assertion table;
     - the "not routed through writeOutcome" list;
     - for each `*.human.txt` case, the before (`guard/baseline/cli-before/`) and after (`reports/cli-after/`) text side by side in fenced blocks.
  4. Commit the report and `reports/cli-after/`.
  5. **STOP and ask the user to approve the before/after output.** The user records approval with `sha256sum <plan>/reports/phase-05-report.md | cut -d' ' -f1 > <plan>/reports/approvals/phase-05.approved`. Never create or edit files in `reports/approvals/` yourself.
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 5)`.

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
