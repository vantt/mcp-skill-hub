---
phase: 1
title: "Baseline, budgets and decisions"
status: pending
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

## Tasks & Steps
1. Measure on a clone of the hub (isolated `HOME` and `XDG_*`, never the live hub): time of `skill_resolve`, `skill_get`, and `resources/read` for every file, over stdio, for `test-audit` (4 files) and `herdr-cook-plan` (14 files, about 186 KB). Count round trips a client needs to get the whole folder today.
2. Record the numbers in `docs/design/remote-hub.md` and confirm or adjust the budgets in `plan.md` (they are proposals).
3. Write the threat model: who can reach the listener, what a stolen token allows (runtime tools only, read-only), what travels (task text, skill content), what must never travel (curation, write paths, secrets in `$SKILLHUB_CONFIG_DIR/env`).
3a. The fan-out of 20 stdio processes is already measured in `plan.md` ("Measured today"); copy it into the design note rather than measuring again.
3b. Check whether Claude Code, Codex and Gemini CLI can register an MCP server by HTTP URL directly. The answer decides whether a shared local server can replace the per-session stdio processes, or only the remote case needs the proxy.
4. Ask the user Q1 to Q5 in `plan.md`; record the answers.
5. Decide go or stop: if the need is only "same skills on several machines", write that the Git route is enough and mark the plan cancelled.

## Verification
- A table of baseline timings exists with the command that produced it.
- Each success criterion in `plan.md` has a number or a yes/no test.
- Q1 to Q5 are answered in writing.
