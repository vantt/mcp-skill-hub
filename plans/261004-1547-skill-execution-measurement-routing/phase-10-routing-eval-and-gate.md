---
phase: 10
title: "Routing eval command, corpus, CI gate"
status: pending
priority: P1
effort: 10h
dependencies: [9]
---

# Phase 10: Routing eval command, corpus, CI gate

## Context

- [plan.md](./plan.md) D8, open question 1. Features from Phase 9.
- `app.ResolverService.Resolve` (`internal/app/resolver.go:31`) opens the catalog per call and wraps the view with `excludingCatalog` (`:150`); `routing_evaluate` calls `app.ResolverService{}` with no telemetry sink (`internal/delivery/mcpserver/insight_tools.go:110`), the same must hold here so eval runs never pollute the funnel.
- Evaluation suite format and strict loader: `evaluation.Suite`, `evaluation.LoadSuite` (`internal/evaluation/types.go:66`, `parse.go:48`); `Expected.NoSkill`.
- Golden corpus: `testdata/resolver/golden-v1.json` (42 skills, 150 cases); workspace fixture `testdata/evaluation/workspace-overlay/skills/software/{code-review,database-design,deploy-service}` used by `internal/delivery/cli/evaluation_test.go:38`.
- Eval CLI dispatch: `internal/delivery/cli/evaluation.go:59-67`; help `help.go:264`.
- Compatibility logic: a skill with `min_scope` returns `missing_scope` when the request has no scope (`internal/resolver/evidence.go:47-53`), so generated requests must carry the skill's declared context.

## Requirements

1. **Refactor for one catalog open per run.** Extract from `Resolve` an unexported `resolveWithin(ctx, root, handle, request, decorate func(resolverpkg.Catalog) resolverpkg.Catalog)`; `Resolve` keeps its behavior and telemetry; eval calls `resolveWithin` with `Telemetry == nil` and a fresh `resolverpkg.NewCache` per case.
2. **Case generation** from every active skill except `system-curator`:
   - Positive: one case per `routing.examples[i]`, expecting status `resolved` with primary = that skill.
   - Counter: one case per `routing.counter_examples[i]`, expecting primary ≠ that skill (`no_skill`, `needs_context`, or another skill are all acceptable).
   - Request context reflects the skill's declared preconditions: `task.scope = min_scope`, `operation = operations[0]` when present, required facts (`facts.all` and the first `facts.any`) added with `basis: user`, required capabilities added to `execution.capabilities`. Request IDs are `routing-eval-<skill>-<ex|cx>-<i>`.
   - **Leave-one-out:** the decorator removes example *i* (or counter-example *i*) from that skill's `Examples` (`CounterExamples`) before scoring. FTS candidacy may still see it; scoring is the decisive stage, so this leak is accepted and documented.
3. **No-skill cases** from an optional file in the existing suite format (`schema_version: 1`, `cases[]` with full `request` and `expected.no_skill: true`), loaded with `evaluation.LoadSuite`. A case passes only with status `no_skill`.
4. **Metrics** (P = positives, N = counters, Z = no-skill cases):
   - precision@1 = correct resolved primaries in P ÷ all `resolved` responses in P ∪ Z
   - recall = correct resolved primaries in P ÷ |P|
   - no-skill recall = `no_skill` in Z ÷ |Z|; no-skill precision = `no_skill` in Z ÷ all `no_skill` in P ∪ Z
   - false-positive rate = (counters resolved to their own skill + Z resolved to any skill) ÷ (|N| + |Z|)
   - Undefined metrics (zero denominator) are reported as `null` and never pass a gate.
   - Report also lists every failing case (skill, kind, index, phrase, got status, got primary) and per-skill recall.
5. **CLI** `skillhub eval routing [--workspace <path>] [--no-skill <file>] [--policy <file>] [--min-precision F] [--min-recall F] [--min-no-skill-recall F] [--max-fpr F] [--json]`. `--policy` loads a recommendation-policy YAML for calibration runs (Phase 12) without touching the workspace. Exit 0 pass (or no thresholds given), 1 any threshold failed, 2 invalid request.
6. **Corpus and backfill** (open question 1):
   - `testdata/routing/examples-v1.json`: for each of the 42 golden-v1 skills, 3–5 `examples` and 1–3 `counter_examples`, synthetic and sanitized like golden-v1. Counter-examples should phrase near-miss tasks that belong to a sibling skill (for example `code-review` vs `pull-request-review`) or to no skill.
   - `testdata/routing/no-skill-v1.yaml`: at least 30 hand-curated requests that no golden skill should take (chit-chat, trivial edits, out-of-catalog domains, requests excluded by `not_for`).
   - `testdata/routing/gate-v1.json`: thresholds for the four gated metrics.
   - Backfill `routing.examples` (3–5) and `routing.counter_examples` (1–2) into the three `testdata/evaluation/workspace-overlay` skills.
7. **CI gate** (runs in `make check` through `go test`): a test materializes a workspace from golden-v1 skills plus the examples overlay (one `skill.meta.yaml` and `SKILL.md` per skill, status active), rebuilds the catalog, runs the routing eval with `no-skill-v1.yaml`, and fails when any metric is below `gate-v1.json`. Thresholds are set from the first measured run after Phase 9: each gated value floored to two decimals minus 0.02 (FPR: ceiling plus 0.02). The measurement goes into `reports/routing-eval-baseline-report.md`.

## Files

Create:
- `internal/app/routing_eval.go`, `internal/app/routing_eval_test.go`
- `internal/app/routing_eval_gate_test.go` (fixture materialization helper lives in this test file)
- `internal/delivery/cli/evaluation_routing.go`, `internal/delivery/cli/evaluation_routing_test.go`
- `testdata/routing/examples-v1.json`, `testdata/routing/no-skill-v1.yaml`, `testdata/routing/gate-v1.json`
- `plans/261004-1547-skill-execution-measurement-routing/reports/routing-eval-baseline-report.md`

Modify:
- `internal/app/resolver.go` (extract `resolveWithin`, no behavior change)
- `internal/resolver/sqlite_catalog.go` (extract the body of `LoadPolicy` into exported `ParsePolicy(digest string, contentJSON []byte) (Policy, error)` so `--policy` files and the catalog share one validator; YAML is converted to JSON with the same stable encoder the catalog uses)
- `internal/delivery/cli/evaluation.go` (dispatch `routing`, usage text)
- `internal/delivery/cli/help.go` (`eval` usage)
- `testdata/evaluation/workspace-overlay/skills/software/*/skill.meta.yaml` (examples backfill)

## Steps

1. Extract `resolveWithin`; the existing resolver and continuity tests prove no behavior change.
2. Case generation, leave-one-out decorator, metrics; unit tests with a tiny workspace (2 skills, hand-checked expected metrics).
3. CLI subcommand with exit codes; test on the backfilled overlay workspace.
4. Author the corpus files; run the eval; record the baseline report (per-metric values, failing cases, run command, binary commit); write `gate-v1.json`.
5. Gate test.

## Tests and validation

- `go test ./internal/app/ -run RoutingEval ./internal/delivery/cli/ -run Routing`
- Telemetry isolation: after a full eval run, `telemetry.db` has no new `resolution.*` events.
- Gate test runtime stays under 10 s on CI (`go test -run RoutingEvalGate -v ./internal/app/` reports duration).
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Self-generated cases overstate quality | High × Medium | Leave-one-out scoring, counter-examples, and an independent hand-curated no-skill set. |
| Flaky gate after unrelated resolver changes | Medium × Medium | Deterministic resolver; thresholds include a 0.02 margin; failures list exact cases. |
| Corpus authoring bias (examples copy triggers) | Medium × Low | Authoring rule in the corpus header: examples must be natural task phrasings, not trigger restatements; lint (Phase 11) flags examples identical to triggers. |
| Slow gate under `-race` | Medium × Low | One catalog open per run; per-case work is in-memory. |

## Rollback

Revert; drop the `testdata/routing` files. No runtime state.
