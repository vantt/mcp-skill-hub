---
phase: 0
title: "Guard verification and baseline"
status: completed
priority: P1
effort: "0.5h"
dependencies: []
---

# Phase 0: Guard verification and baseline

## Goal

Confirm the planner-built guard and baseline are intact and green on the unmodified code before any change.

## Context

- The guard script, baseline metrics, CLI output snapshots (`guard/baseline/cli-before/`), and the error characterization fixture were produced by the planner and committed by the user (see "Handover procedure" in `plan.md`).
- You do not create or regenerate any baseline. You only verify.

## Files to Create / Modify

- Create: `plans/261002-0936-pre-webui-architecture-and-cli-output/reports/phase-00-report.md`
- Modify: nothing else.

## Tasks

### Task 0.1 — Verify the working tree is clean
- Goal: no uncommitted changes exist before phase 1.
- Steps:
  1. Run `git status --porcelain -- internal cmd schemas go.mod go.sum Makefile .golangci.yml .github plans/261002-0936-pre-webui-architecture-and-cli-output`. (Other untracked paths, such as `docs/use-cases/`, belong to the user; leave them alone.)
- Success criteria: empty output.
- Verify: the command prints nothing. If it prints anything, STOP (Failure Protocol). Do not stash, reset, or discard anything yourself.

### Task 0.2 — Run the guard for phase 0
- Goal: the guard passes on the unmodified code.
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 0`. It takes about 2 minutes.
- Success criteria: every line is `PASS`.
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 0)`.

### Task 0.3 — Write the phase report
- Goal: record the starting state, including the two hashes the guard prints first.
- Steps:
  1. Create `reports/phase-00-report.md` with the full output of Task 0.2.
  2. Commit it: `docs(plan): record phase 0 guard baseline`.
- Verify: no verification needed; the user compares the printed hashes with their own record.

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
