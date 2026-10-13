---
phase: 1
title: "Baseline, budgets and decisions"
status: completed
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 1: Baseline, budgets and decisions

## Goal
Measure how fast and how complete a skill fetch is today over stdio, turn the user's two criteria into numbers, and settle the decisions Q1 to Q5 before any code.

## Files to Create / Modify
- Create: `internal/delivery/mcpserver/remote_bench_test.go` (benchmarks only; skipped unless `SKILLHUB_PERF=1`, like the other performance budgets)
- Create: `docs/design/remote-hub.md` (threat model, decisions, budgets; short)
- Modify: `Makefile` (`make remote-baseline`)

## Tasks & Steps
- [x] 1. Measure on a clone of the hub (isolated `HOME` and `XDG_*`, never the live hub): time of `skill_resolve`, `skill_get`, and `resources/read` for every file, over stdio, for `test-audit` (4 files) and `herdr-cook-plan` (14 files, about 186 KB). Count round trips a client needs to get the whole folder today.
- [x] 2. Record the numbers in `docs/design/remote-hub.md` and confirm or adjust the budgets in `plan.md` (they are proposals).
- [x] 3. Write the threat model: who can reach the listener, what a stolen token allows (runtime tools only, read-only), what travels (task text, skill content), what must never travel (curation, write paths, secrets in `$SKILLHUB_CONFIG_DIR/env`).
- [x] 3a. The fan-out of 20 stdio processes is already measured in `plan.md` ("Measured today"); copy it into the design note rather than measuring again.
- [x] 4. Q1 to Q5 are answered in `plan.md` (2026-10-11); copy them and the command syntax into the design note.
- [x] 5. Go or stop is settled: go.

## Verification
- [x] A table of baseline timings exists with the command that produced it (`make remote-baseline REMOTE_HUB=/home/vantt/skill-hub`, run twice, in `docs/design/remote-hub.md`).
- [x] Each success criterion in `plan.md` has a number or a yes/no test.
- [x] Q1 to Q5 are answered in writing (done in `plan.md`).

## Result (2026-10-11)
- Today a remote-style client needs 16 sequential round trips for the `herdr-cook-plan` folder (5 for `test-audit`); on stdio the folder takes 248 to 266 ms at p50, and 100 ms of RTT would add 1.6 s.
- `skills/get` serves the files outside `.meta/`: 13 of 14 files for `herdr-cook-plan`, 2 of 4 for `test-audit`.
- Each `resources/read` costs about 15 ms on the hub because it rebuilds the skill manifest and checks the snapshot; the bundle in Phase 3 should be built in one pass.
- The three numeric budgets are confirmed and the yes/no budget is unchanged; no number changed.
