---
title: "Pre-WebUI architecture fixes and CLI output renderer"
description: "Move business logic out of delivery adapters, centralize error classification, deduplicate proposal confirmation, split oversized services, and give the CLI one wrapped, consistent renderer before WebUI work starts."
status: completed
priority: P1
effort: 30h
branch: main
tags: [refactor, backend, tech-debt, cli]
blockedBy: []
blocks: []
created: 2026-10-02
---

# Pre-WebUI architecture fixes and CLI output renderer

## Overview

An architecture review on 2026-10-02 found a sound modular monolith (delivery → `internal/app` → domain/infra; `go vet` clean; all 19 packages pass). Four problems would force the upcoming WebUI HTTP adapter to copy logic or inherit bugs, and the CLI output is hard to read:

1. `skill_get` business logic lives in the MCP adapter (`internal/delivery/mcpserver/skill_tools.go:138-200`), which `docs/design/01-system-architecture.md:192` forbids.
2. Error classification is duplicated and string-based (`mcpserver/server.go:478-609`, `cli/skill.go:571-660`). Seventeen-plus validation errors fall through to `internal_error`.
3. Proposal pin/expiry/digest checks are copied across six services; four methods are 285–541 lines long.
4. CLI human output is ad hoc `fmt.Fprintf` (33 hand-written ERROR/WHY/FIX blocks, 37 JSON/exit branches). Nothing wraps, labels do not align, and sizes print as raw bytes.

This plan is written for a **separate executor model**. Every phase file is self-contained, every check is mechanical, and a guard script enforces the rules below.

## Decisions (validated with the user on 2026-10-02)

| Topic | Decision |
|---|---|
| Error channel | Keep both channels (`error` and `Result.Error`). Add one shared classifier `app.ClassifyError` plus `app.ErrorOf`. Convert only the listed misclassified sites to typed errors. |
| CLI renderer | Plain text. Wrap to terminal width via `golang.org/x/term` when stdout is a TTY (clamped 60–120); 100 columns otherwise. No color. |
| CLI migration scope | Every command with human output. JSON output must stay byte-identical. |
| Function splitting | `PreviewSkillAdd`, `SubmitDistillRun`, `ReviewSkill`, `PreviewSourceWatch`. |

## Phases

| # | Phase | Status | Depends on | Human gate after |
|---|-------|--------|------------|------------------|
| 0.5 | Test speed and lint tooling (done by the planner; see below) | Completed | — | — |
| 0 | [Guard verification and baseline](./phase-00-guard-and-baseline.md) | Completed | 0.5 | No |
| 1 | [Skill detail read model in app](./phase-01-skill-detail-read-model.md) | Completed | 0.5 | No |
| 2 | [Shared error classification](./phase-02-shared-error-classification.md) | Completed | 1 | **Yes** |
| 3 | [Proposal confirmation helper and service splitting](./phase-03-proposal-helper-and-service-split.md) | Completed | 2 | No |
| 4 | [CLI terminal renderer package](./phase-04-cli-terminal-renderer.md) | Completed | 0 | **Yes (approve samples)** |
| 5 | [Migrate CLI commands to the renderer](./phase-05-migrate-cli-output.md) | Completed | 2, 4 | **Yes (approve before/after)** |
| 6 | [Docs sync](./phase-06-docs-sync.md) | Completed | 1–5 | No |

Run phases strictly in the numeric order 0 → 6, one at a time. Do not run phases in parallel.

### Phase 0.5 (completed by the planner before handover)

- Tests in `internal/app`, `delivery/mcpserver`, `delivery/cli`, `catalog`, `mutation`, and `skill` call `t.Parallel()`, except tests that use `Setenv`/`Chdir` or assert on the process-global MCP diagnostics logger. The full suite went from about 200 s to about 34 s. `-race -count=3` is clean.
- `golangci-lint v2.14.0` runs pinned through `go run` (`.golangci.yml`: standard linters + funlen 120 + revive argument-limit 6). `make lint LINT_BASE=<rev>` reports only issues introduced after `<rev>`. CI has a `Lint (new issues)` job.
- `Makefile`: `make check` (vet + lint + test), `make test-race`, `make test-perf`, `make lint-all`.

### Handover procedure (the user does this once, before giving the plan to the executor)

1. Commit the Phase 0.5 tooling changes.
2. With that commit checked out and a clean tree, run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/capture-cli-output.sh plans/261002-0936-pre-webui-architecture-and-cli-output/guard/baseline/cli-before`, then `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh baseline`.
3. Commit the plan directory.
4. Record both hashes **outside the repository**: the baseline commit (`guard/baseline/base_commit.txt`) and the guard commit (`git log -1 --format=%H -- plans/261002-0936-pre-webui-architecture-and-cli-output/guard`). Every guard run prints both; if either differs from your record, the guard was tampered with and the executor's results must be discarded.

## Executor hard rules (apply to every phase)

These rules exist because a previous executor satisfied checks without satisfying requirements. Breaking any rule fails the phase even if every test passes.

1. **Do not touch the guard or approvals.** Never edit, move, regenerate, or commit changes to anything under `plans/261002-0936-pre-webui-architecture-and-cli-output/guard/`. Never run `guard.sh baseline`, and never run `capture-cli-output.sh` into the baseline directory. Never create or edit `reports/approvals/*`: approvals are created only by the user, as described at each human gate.
2. **Tests are evidence, not obstacles.**
   - Never delete, rename, or skip a test.
   - Never add `t.Skip`.
   - Never weaken an assertion: no shorter substring, no removed check, no changed exit code, no `strings.Contains(x, "")`.
   - Before phase 5, never modify an existing `*_test.go` file; only add the new test files each phase names.
   - Add only the tests each phase names. Follow the Testing section of `AGENTS.md`: one owner test per contract, no near-duplicates, no test-only exports or seams.
   - New tests start with `t.Parallel()` unless they use `t.Setenv`/`t.Chdir` or assert on process-global state (then they must not).
   - In phase 5 you may update expected *human text* only in `internal/delivery/cli/*_test.go`, and each changed assertion must still check the same fact (same ID, state, or value).
3. **Public contracts are frozen.** JSON field names, MCP tool names, error code strings, CLI exit codes, and which stream (stdout or stderr) a command writes to must not change. The guard compares CLI `--json` output byte-for-byte against a planner-captured baseline.
4. **Lint stays clean.** `make lint LINT_BASE=$(cat plans/261002-0936-pre-webui-architecture-and-cli-output/guard/baseline/base_commit.txt)` must print `0 issues.` after every task. Never edit `.golangci.yml` or `Makefile`.
5. **No placeholders or test-only paths.**
   - No `TODO`, `FIXME`, `XXX`, `HACK`, `nolint`, `//go:build ignore`, or "not implemented".
   - No `testing.Testing()` or `testing.Short()` checks, and no `TestMain`.
   - No code that behaves differently under test.
6. **Satisfy intent, not grep.** Guard greps are a floor, not the definition of done. Examples of cheating that will be rejected in review:
   - renaming a helper so a grep stops matching while the logic stays in the adapter;
   - wrapping `fmt.Fprintf` in a local alias to dodge the print check;
   - splitting a long function into sequential `partOne`/`partTwo` chunks with no single responsibility;
   - passing an all-fields struct to every helper to dodge the parameter check.
7. **Stop instead of improvising.** If a step is ambiguous, a Verify fails, or a rule seems to block the requirement, follow the phase's Failure Protocol. Do not invent a workaround.
8. **Commit once per completed task** with a conventional message (`refactor(app): ...`, `feat(cli): ...`, `test(...): ...`). Never commit with failing tests. No AI attribution in commit messages.
9. **Report every phase.** Write `plans/261002-0936-pre-webui-architecture-and-cli-output/reports/phase-NN-report.md` containing:
   - the full output of `guard.sh check NN`, including the two hashes it prints first;
   - `git diff --stat <baseline-commit>` (commit id in `guard/baseline/base_commit.txt`);
   - every deviation from the plan, with its reason;
   - open questions.

## Guard

- Run: `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check <phase>`
- Pass condition: the last line is exactly `GUARD RESULT: PASS (phase <phase>)` and the exit code is 0.
- What the guard checks:
  - guard integrity (pinned checksums; guard directory committed and unchanged);
  - build, vet, full test suite, gofmt, and golangci-lint (new issues since the baseline commit);
  - no removed or renamed tests, unchanged skip count, no removed JSON tags, unchanged MCP tool set;
  - forbidden patterns in added code (skip, nolint, TODO, `testing.Testing`/`Short`, `TestMain`, vacuous `strings.Contains(x, "")`), and test-file edit rules;
  - required tests actually ran and passed (`go test -v`, `--- PASS:` lines), not merely exist;
  - user approval stamps for the human gates;
  - CLI assertion count and exit-code expectations;
  - CLI `--json` output identical to `guard/baseline/cli-before/`;
  - CLI human output with unchanged exit codes and streams that keeps every identifier and path;
  - the cumulative phase-specific structure checks.
- Guard self-check: every run prints the baseline commit and the guard commit first. The user compares them with the hashes recorded at handover (step 4 above).

## Non-goals

- Repository ports, a dependency-injection container, or moving SQL/filesystem access out of `internal/app`.
- Removing the `Result.Error` channel.
- Any JSON, MCP schema, or error-code change. One exception: the listed misclassified errors move from `internal_error` to `invalid_request` in phase 2.
- Color output, an interactive TUI, or the WebUI HTTP adapter.
- Fixing `skillhub status` printing "No skills yet" when only draft skills exist. That fix needs a new `home_summary` JSON field and is recorded as a follow-up.

## Success Criteria

- [x] `guard.sh check 6` prints `GUARD RESULT: PASS (phase 6)`.
- [x] The user approved the phase 2 error mapping, the phase 4 renderer samples, and the phase 5 before/after CLI output.
- [x] A final code review (outside the executor) finds no rule-5 violations. Reviewed 2026-10-08 over `2ad9018..df9e1d8`: no `TODO/FIXME/nolint/build ignore/testing.Testing/TestMain` or vacuous assertions added, no test removed, extracted helpers have single responsibilities, `verifyProposalPins` is shared by 8 call sites, `skill_get` delegates to `GetSkillDetail`. One finding: `buildAndStoreSourceWatchProposal` and `appendSourceSizeWarnings` were orphaned when a later feature made `PreviewSourceWatch` delegate to `PreviewAttach`; removed as dead code (`make lint` back to 0 issues, `internal/app` tests pass).

<!-- slug: pre-webui-architecture-and-cli-output -->
