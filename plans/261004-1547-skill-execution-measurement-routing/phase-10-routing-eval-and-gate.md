---
phase: 10
title: "Routing eval command, corpus, CI gate"
status: done
priority: P1
effort: 10h
dependencies: [9]
---

# Phase 10: Routing eval command, corpus, CI gate

## Goal

`skillhub eval routing` generates test cases from every active skill's examples and counter-examples (leave-one-out), adds a hand-curated no-skill set, reports routing metrics, and a gate test in `make check` fails when quality drops below recorded thresholds.

## Context (read these first)

- `plan.md` → "Executor notes", D8, open question 1. Features from phase 9.
- `app.ResolverService.Resolve` (`internal/app/resolver.go:35`) discovers the workspace, opens the catalog per call, wraps the view with `excludingCatalog` (`:173`, used at `:107`), and records telemetry through `service.Telemetry`. `routing_evaluate` calls `app.ResolverService{}` with no sink (`internal/delivery/mcpserver/insight_tools.go:111`); the routing eval must also run with no sink so it never pollutes the funnel.
- Post-ranking `setup` annotation (`ResolverService.setupStatus`, `resolver.go:160`) does not affect ranking; the eval compares only status and primary ID.
- Evaluation suite format and strict loader: `evaluation.Suite` (`internal/evaluation/types.go:64`), `evaluation.LoadSuite(path)` (`internal/evaluation/parse.go:48`), `Expected.NoSkill` (`types.go:46,61`).
- Golden corpus `testdata/resolver/golden-v1.json` (`skills[]` with `id, collection_id, name, description, status, digest, aliases, operations, triggers, not_for, min_scope, reviewed, equivalent_to, supporting, …`; 42 skills, 150 cases). Workspace fixture `testdata/evaluation/workspace-overlay/skills/software/{code-review,database-design,deploy-service}` used by `internal/delivery/cli/evaluation_test.go:38`.
- Eval CLI: `runEvaluation` (`internal/delivery/cli/evaluation.go:50`) dispatches `manifest|run|promote` (`:59-63`); help `internal/delivery/cli/help.go:282`.
- Policy parsing: `resolver.LoadPolicy(ctx, *sql.DB)` (`internal/resolver/sqlite_catalog.go:29`) reads `config/recommendation.yaml` from `routing_documents` and validates it inline; schema `schemas/recommendation-policy-v1.schema.json`.
- Compatibility: a skill with `min_scope` returns `missing_scope` when the request has no scope (`internal/resolver/evidence.go:44-49`), so generated requests must carry the skill's declared context.

## Requirements

1. **One catalog open per run.** Extract from `Resolve` an unexported `resolveWithin(ctx, root string, handle *catalog.Handle, request resolverpkg.Request, decorate func(resolverpkg.Catalog) resolverpkg.Catalog) (resolverpkg.Response, error)`; `Resolve` keeps its behavior and telemetry. The eval calls `resolveWithin` with `Telemetry == nil` and a fresh `resolverpkg.NewCache` per case.
2. **Case generation** from every active skill except `system-curator`:
   - Positive: one case per `routing.examples[i]`, expecting status `resolved` with primary = that skill.
   - Counter: one case per `routing.counter_examples[i]`, expecting primary ≠ that skill (`no_skill`, `needs_context`, or another skill all pass).
   - Request context reflects the skill's preconditions: `task.scope = min_scope`, `operation = operations[0]` when present, required facts (`facts.all` and the first `facts.any`) with `basis: user`, required capabilities in `execution.capabilities`. Request IDs `routing-eval-<skill>-<ex|cx>-<i>`.
   - **Leave-one-out:** the decorator removes example *i* (or counter-example *i*) from that skill's `Examples` (`CounterExamples`) before scoring. FTS candidacy may still see it; scoring is decisive, so this leak is accepted and documented in the report.
3. **No-skill cases** from an optional file in the existing suite format (`schema_version: 1`, `cases[]` with full `request` and `expected.no_skill: true`), loaded with `evaluation.LoadSuite`. Passes only with status `no_skill`.
4. **Metrics** (P positives, N counters, Z no-skill cases): precision@1 = correct resolved primaries in P ÷ all `resolved` in P ∪ Z; recall = correct resolved primaries in P ÷ |P|; no-skill recall = `no_skill` in Z ÷ |Z|; no-skill precision = `no_skill` in Z ÷ all `no_skill` in P ∪ Z; false-positive rate = (counters resolved to their own skill + Z resolved to any skill) ÷ (|N| + |Z|). Zero denominators → `null`, which never passes a gate. Report lists every failing case (skill, kind, index, phrase, got status, got primary) and per-skill recall.
5. **CLI** `skillhub eval routing [--workspace <path>] [--no-skill <file>] [--policy <file>] [--min-precision F] [--min-recall F] [--min-no-skill-recall F] [--max-fpr F] [--json]`. `--policy` loads a recommendation-policy YAML for calibration (phase 12) without touching the workspace. Exit 0 pass (or no thresholds), 1 any threshold failed, 2 invalid request.
6. **Corpus and backfill:**
   - `testdata/routing/examples-v1.json`: for each of the 42 golden-v1 skills, 3–5 `examples` and 1–3 `counter_examples`, synthetic and sanitized like golden-v1. Header rule: examples are natural task phrasings, not trigger restatements; counter-examples phrase near-misses belonging to a sibling skill (e.g. `code-review` vs `pull-request-review`) or to no skill.
   - `testdata/routing/no-skill-v1.yaml`: at least 30 requests no golden skill should take (chit-chat, trivial edits, out-of-catalog domains, requests excluded by `not_for`).
   - `testdata/routing/gate-v1.json`: thresholds for the four gated metrics.
   - Backfill `routing.examples` (3–5) and `routing.counter_examples` (1–2) into the three `testdata/evaluation/workspace-overlay` skills.
7. **CI gate** (`go test`, so it runs in `make check`): materialize a workspace from golden-v1 skills plus the examples overlay (one `skill.meta.yaml` and `SKILL.md` per skill, status active), rebuild the catalog, run the routing eval with `no-skill-v1.yaml`, fail when any metric is outside `gate-v1.json`. Thresholds come from the first measured run after phase 9: each gated value floored to two decimals minus 0.02 (FPR: ceiling plus 0.02). Record the measurement in `reports/routing-eval-baseline-report.md` (per-metric values, failing cases, command, commit hash).

## Files

Create:
- `internal/app/routing_eval.go`, `internal/app/routing_eval_test.go`
- `internal/app/routing_eval_gate_test.go` (fixture materialization helper lives here)
- `internal/delivery/cli/evaluation_routing.go`, `internal/delivery/cli/evaluation_routing_test.go`
- `testdata/routing/examples-v1.json`, `testdata/routing/no-skill-v1.yaml`, `testdata/routing/gate-v1.json`
- `plans/261004-1547-skill-execution-measurement-routing/reports/routing-eval-baseline-report.md`

Modify:
- `internal/app/resolver.go` (extract `resolveWithin`; no behavior change)
- `internal/resolver/sqlite_catalog.go` (extract the body of `LoadPolicy` into exported `ParsePolicy(digest string, contentJSON []byte) (Policy, error)`; YAML from `--policy` is converted to JSON with the same stable encoder the catalog uses)
- `internal/delivery/cli/evaluation.go` (dispatch `routing`, usage), `internal/delivery/cli/help.go` (`eval` usage)
- `testdata/evaluation/workspace-overlay/skills/software/*/skill.meta.yaml` (examples backfill)

## Steps

- [x] **1. Extract `resolveWithin`.**
  Pass: `go test -count=1 -run 'Resolve|Continuity' ./internal/app/` → `ok` with no test edits.
- [x] **2. Case generation, leave-one-out decorator, metrics** with a tiny two-skill workspace and hand-checked expected metrics.
  Pass: `go test -count=1 -run RoutingEval ./internal/app/` → `ok`.
- [x] **3. CLI** with exit codes 0/1/2, tested on the backfilled overlay workspace.
  Pass: `go test -count=1 -run Routing ./internal/delivery/cli/` → `ok`; `go run ./cmd/skillhub help eval` lists `routing`.
- [x] **4. Corpus, baseline, thresholds.** Author the corpus files; run the eval on the materialized workspace; write the baseline report; write `gate-v1.json` from the margin rule.
  Pass: `reports/routing-eval-baseline-report.md` exists with all five metrics and the commit hash.
- [x] **5. Gate test.**
  Pass: `go test -count=1 -run RoutingEvalGate -v ./internal/app/` → `ok`; record its duration in the report. [UNVERIFIED: target under 10 s on CI; measure, and if it exceeds 10 s, note it in the report rather than raising the budget silently.]
- [x] **6. Telemetry isolation:** after a full eval run, `telemetry.db` has no new `resolution.*` events.
  Pass: covered by a test in step 2 → `ok`.
- [x] **7. Gate.** Pass: `make check` exits 0.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Self-generated cases overstate quality | High × Medium | Leave-one-out, counter-examples, independent no-skill set. |
| Flaky gate after unrelated resolver changes | Medium × Medium | Deterministic resolver; 0.02 margin; failures list exact cases. |
| Corpus bias (examples copy triggers) | Medium × Low | Header authoring rule; phase 11 lint flags examples that restate triggers. |
| Slow gate under `-race` | Medium × Low | One catalog open per run; per-case work in memory. |

## Rollback

Revert; drop `testdata/routing`. No runtime state.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol". Never set thresholds from a run that has not been recorded in the baseline report. Write `reports/<agent>-<YYMMDD-HHMM>-routing-eval.md`, set `status: blocked`, report the blocker.

## Implementation Note
Extracted `resolveWithin` in `internal/app/resolver.go` and `ParsePolicy` in `internal/resolver/sqlite_catalog.go`. Implemented `RoutingEvalService` supporting leave-one-out case generation from skill examples and counter-examples alongside external no-skill suites. Created `testdata/routing/` corpus files (`examples-v1.json`, `no-skill-v1.yaml`, `gate-v1.json`) and backfilled workspace-overlay skills. Added CLI subcommand `skillhub eval routing` with threshold evaluation and exit codes 0/1/2. Implemented `TestRoutingEvalGate` running in under 2.5s, verified baseline thresholds, and documented results in `reports/routing-eval-baseline-report.md`.
