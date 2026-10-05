---
title: "Skill Hub WebUI v1 implementation"
description: "Local HTTP adapter in Go plus a React/TypeScript frontend, embedded in the skillhub binary, delivering the 13 screens of the approved mockup in six verifiable phases."
status: pending
priority: P1
effort: 218h
branch: main
tags: [feature, frontend, backend, api, security]
blockedBy: []
blocks: []
created: 2026-10-03
---

# Skill Hub WebUI v1 implementation

## Overview

Deliver the WebUI described by the behavior spec ([04](../../docs/use-cases/04-webui-user-flows-and-screen-specs.md)) and the visual brief ([05](../../docs/use-cases/05-webui-design-brief.md)), matching the Claude Design mockup kept at [`docs/design/webui-mockup/`](../../docs/design/webui-mockup/README.md). The command is `skillhub serve web` (alias `skillhub web`). The WebUI calls the same `internal/app` services as the CLI and MCP server and adds no business logic of its own.

**This plan is written for a separate executor model (Google Gemini).** Every phase file is a sequence of tasks, each with exact target files, ordered steps, success criteria and a mechanical Verify command, followed by a literal Failure Protocol. A guard script checks every phase. There are no human approval gates between phases (user decision, 2026-10-04).

Supporting records: [decisions.md](./decisions.md) (all architecture decisions and their status), [reports/red-team-261003-1645-webui-v1-plan-review.md](./reports/red-team-261003-1645-webui-v1-plan-review.md).

## Phases

| # | Phase | Depends on | Effort | Status |
|---|---|---|---|---|
| 1 | [Web adapter foundation (Go)](./phase-01-web-adapter-foundation.md) | — | 32h | Complete |
| 2 | [Frontend foundation, Home, Skills catalog](./phase-02-frontend-foundation-home-skills.md) | 1 | 38h | Complete |
| 3 | [Add, Create, Skill Detail, proposals](./phase-03-skill-add-create-detail-proposals.md) | 2 | 46h | Complete |
| 4 | [Sources, Handoff, Runs](./phase-04-sources-handoff-runs.md) | 3 | 34h | Superseded by plans/261004-2234-skill-source-upstream-ux/ |
| 5 | [Inbox, Insight, Patch Composer](./phase-05-inbox-insight-patch-composer.md) | 3 | 40h | Pending |
| 6 | [Hardening, docs, release](./phase-06-hardening-docs-release.md) | 5 | 28h | Pending |

Phase 4 has been superseded by the skill-centric source architecture in `plans/261004-2234-skill-source-upstream-ux/` (Phases 8 & 9). Phase 5 is decoupled from Phase 4 and remains pending for the Inbox and Patch Composer workflow.

## Executor hard rules (apply to every task)

A previous executor in this repository satisfied checks without satisfying requirements. Breaking any rule below fails the phase even if every test passes.

1. **Never touch the guard.** Do not edit, move, regenerate or commit changes under `plans/261003-1645-webui-v1-implementation/guard/`. Never run `guard.sh baseline`.
2. **Tests are evidence.** Never delete, rename or skip a test; never add `t.Skip`, `.skip`, `.only`, `xit`; never weaken an assertion; never edit golden files by hand (regenerate them only with `-update` inside the task that owns them, and read the result). Do not modify existing `*_test.go` files, except `internal/delivery/mcpserver/server_test.go` and `hardening_test.go` in task 5.1, exactly as that task says.
3. **Public contracts are frozen.** CLI `--json` output, CLI human output, MCP tool names and schemas, JSON field names and error code strings must not change. The guard compares CLI output with a baseline byte for byte.
4. **No placeholders or test-only paths.** No `TODO`, `FIXME`, `XXX`, `HACK`, `nolint`, `@ts-ignore`, `@ts-expect-error`, `eslint-disable`, `testing.Testing()`, `TestMain`, or code that behaves differently under test. Do not create exports, wrappers or seams that only tests use (AGENTS.md, Testing). Test doubles live in `_test.go` files.
5. **Lint limits.** Go functions at most 120 lines, at most 6 parameters (`.golangci.yml`). `make lint LINT_BASE=$(cat plans/261003-1645-webui-v1-implementation/guard/baseline/base_commit.txt)` must print no new issues. Never edit `.golangci.yml`.
6. **Satisfy intent, not grep.** Guard greps are a floor. Renaming to dodge a grep, splitting a function into meaningless parts, or copying logic the plan says to reuse will be rejected in review.
7. **Stay in scope.** Implement exactly the tasks. Do not add features, dependencies or files that no task names. If a step is ambiguous or seems wrong, follow the Failure Protocol; do not improvise.
8. **Processes.** Start long-running processes only as the tasks say (Playwright starts and stops its own server on port 0; `scripts/web-dev.sh` refuses busy ports). Stop every process you start before the task ends.
9. **Commits.** Commit at least once per completed task with a conventional message (`feat(web): ...`, `test(web): ...`, `docs: ...`). Never commit with failing checks. No AI attribution.
10. **Reports.** Each phase ends by writing `reports/phase-NN-report.md`: the full guard output (including the two hashes it prints first), `git diff --stat <baseline commit>`, every deviation with its reason, every Failure Protocol event, and open questions.

## Guard

- Run: `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check <phase>` at the end of each phase.
- Pass: the last line is exactly `GUARD RESULT: PASS (phase <phase>)` and the exit code is 0.
- Checks: guard integrity; Go build, vet, tests, gofmt, lint (new issues only); `web/` excluded from Go packages; no removed tests, no new skips, no removed JSON tags, unchanged MCP tool set; forbidden patterns; test-file edit rules; CLI `--json` and human output identical to the baseline; from phase 2, `make web-check`, `make web-e2e` and frontend forbidden patterns; per-phase required files, greps and named tests.
- Baseline: commit `37b61880fc9cf33a0fd9428368adfe4a9f1264b4`, captured by the planner on 2026-10-04. `guard.sh check 0` on that tree passed every check except "guard directory is committed", which passes once step 1 of the handover is done.

### Handover (the user does this once, before giving the plan to the executor)

1. Commit the plan directory (including `guard/`) and `docs/design/webui-mockup/`.
2. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 0`; it must end with `GUARD RESULT: PASS (phase 0)`.
3. Record outside the repository the two hashes it prints first (baseline commit and guard commit). Every later guard run prints them again; if either differs, the guard was tampered with and the executor's results must be discarded.

## Acceptance criteria (whole plan)

- [ ] `skillhub serve web` (alias `skillhub web`) serves all 13 mockup routes against a real workspace, with loading, empty, error and success states per spec 04 §5.
- [ ] Every mutation goes Preview → Confirm with all three pins and handles `stale_proposal`; no force overwrite exists.
- [ ] No API request is served without the session token, an allowlisted `Host`, and (for non-GET) a same-origin `Origin`; failed authentication is throttled.
- [ ] The server listens on one `0.0.0.0` socket when the host has two or more non-loopback IPv4 addresses, otherwise on loopback; `--addr` and `--loopback-only` override as in D14; a warning lists every reachable URL when not on loopback.
- [ ] The web adapter never calls run start, retry or submit.
- [ ] Golden JSON produced by Go tests is the only API fixture used by frontend tests.
- [ ] Light and dark, desktop/tablet/mobile, keyboard and axe checks pass on every route.
- [ ] `make check`, `make web-check`, `make web-e2e` and the guard pass for phase 6; a release build embeds the UI.
- [ ] Docs no longer claim "no Web UI in V1"; README, user guide, architecture, error-code and release docs match shipped behavior.
- [ ] CLI `--json` output and MCP tool schemas are unchanged.

## Validation Log

### Session 1 — 2026-10-04
**Trigger:** `/ak-plan validate` before handing the plan to a Gemini executor.
**Questions asked:** 7

#### Questions & Answers

1. **[Scope]** Gemini will cook this plan. Convert all six phases to the handover format for a weaker executor (task-level Goal, exact files and symbols, steps, comparable Verify, Failure Protocol)?
   - Options: Convert everything (Recommended) | Only phase-level Verify and Failure Protocol | Keep as is
   - **Answer:** Convert everything
   - **Rationale:** The executor under-infers; every step it would otherwise guess is now written out.
2. **[Risks]** When a Verify fails, what should Gemini do (it may not be able to spawn `kongming`)?
   - Options: Stop and report with evidence (Recommended) | Allow one self-fix, then stop
   - **Answer:** Allow one self-fix, then stop
   - **Rationale:** Faster on trivial errors; bounded to one attempt on the task's own non-test files, then STOP (written into every phase's Failure Protocol).
3. **[Risks]** Add an anti-workaround guard script like the pre-WebUI plan?
   - Options: Yes, compact guard (Recommended) | No, rely on final review
   - **Answer:** Yes, compact guard
   - **Rationale:** With no human gates, the guard and Verify commands are the only protection.
4. **[Scope]** Human approval gates between phases?
   - Options: After P1 and each UI phase (Recommended) | One gate at the end | No gates
   - **Answer:** No gates
   - **Rationale:** Screenshots and parity checklists are still produced per phase for later review, but nothing blocks on them.
5. **[Architecture]** Inbox paging helpers live in `mcpserver/server.go` and are used by five tool files, the CI fuzz target and `server_test.go`. How to share them?
   - Options: Shared package with unexported wrappers in mcpserver (Recommended) | Move fully and update MCP tests | Client-side paging only (spec change)
   - **Answer:** Move fully and update MCP tests
   - **Rationale:** One implementation; task 5.1 lists every caller and the exact test edits; the guard allows only those two test files to change, and only in phase 5.
6. **[Assumptions]** How to seed runs and insights for tests (the app's helpers are unexported)?
   - Options: Public `app.DistillService` plus a test-local fake adapter; e2e through the real CLI with a `filesystem` source (Recommended) | Exported shared fixture package | Go tests only
   - **Answer:** Public service plus fake adapter; e2e through the real CLI
   - **Rationale:** Follows the AGENTS.md rule against test-only exports; the e2e seed is a verified task with a STOP condition.
7. **[Assumptions]** Apply the verified corrections (schema read by path; `app.SkillStates` validation; dedicated help test; MCP-identical skill confirm with three pins; `source_watch` check on confirm; no idempotency key for decisions)?
   - Options: Apply all (Recommended) | Review each
   - **Answer:** Apply all

#### Confirmed Decisions
- Executor format: full handover, Failure Protocol with one self-fix then STOP.
- Guard: yes; human gates: none.
- D13: paging helpers move to `internal/delivery/paging`; MCP tests updated in place (only `server_test.go` line 355, `hardening_test.go` lines 86–87, and the moved cursor test).
- Test seeding: public APIs plus fake adapter in Go tests; real CLI for e2e.
- Decisions D2, D6–D12 had no objection and are treated as accepted.

#### Impact on Phases
- Phase 1: task-level rewrite; state validation, schema path, help test, listen rule.
- Phase 3: skill confirm via `LoadSkillProposal` + `ConfirmSkillMutation`, pins required.
- Phase 4: run-not-found sentinel, `source_watch` check, fake adapter and CLI seed.
- Phase 5: full paging move; insight-not-found sentinel; no idempotency key on decisions; cursors compared by content.
- Phase 6: explicit doc edits matching guard greps.

### Verification Results
- **Tier:** Full (6 phases; fact, flow, scope and contract roles)
- **Claims checked:** 60 — **Verified:** 49 | **Failed:** 8 (all fixed above) | **Unverified:** 3 (external library behavior: `react/jsx-no-literals` options, Fontsource package names, nested `go.mod` exclusion — each now has a Verify step that proves it on the executor's machine)

#### Failures (fixed)
1. [Fact] `schemas/embed.go` does not embed `error-envelope.schema.json` → test reads the file by path.
2. [Flow] `ListSkills` unknown state becomes `internal_error` (no classifier rule) → adapter validates with `app.SkillStates`.
3. [Fact] No existing help test enumerates commands → `TestServeWebCommand` covers help.
4. [Contract] `ConfirmProposal(pins *)` falls back to stored pins; MCP requires all three → mirror MCP.
5. [Contract] MCP `source_watch_confirm` checks `ApplicationCommand` → mirrored.
6. [Scope] App test fixtures are unexported (`distill_test.go:489`, `insight_test.go:282`) → fake adapter in web tests; CLI seed for e2e.
7. [Contract] Paging helpers have 5 tool callers, `server.go` itself, the CI fuzz target and `server_test.go` → listed with line numbers in task 5.1; cursor MAC key is per process, so cursors are compared by content.
8. [Flow] A client idempotency key on decisions defeats the server's deterministic key (`insight.go:334`) → decisions send none; guard forbids it.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, decisions.md, phase-01 … phase-06, red-team report, guard.sh.
- Decision deltas checked: 9 (handover format, Failure Protocol variant, guard, no gates, D13 full move, seeding, confirm path, decision keys, sentinels).
- Reconciled stale references: 11 (D13 text, phase 5 dependency on 4, efforts and total, `--ignore-scripts` removed, `skill create --workspace` replaced by `SKILLHUB_WORKSPACE`, architecture doc lines, guard file and test names aligned with tasks).
- Unresolved contradictions: 0.
