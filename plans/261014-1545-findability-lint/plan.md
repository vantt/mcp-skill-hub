---
title: "Self-resolution lint for skill examples"
description: "Warn when a skill's own examples do not resolve to it, or its counter-examples do, by reusing the leave-one-out logic of `skillhub eval routing` inside validate and review."
status: pending
priority: P2
effort: 1.5d
branch: worktree-agent-adf317bed349da81c
tags: [feature, backend, curation, routing]
blockedBy: []
blocks: []
created: 2026-10-14
---

# Self-resolution lint for skill examples

Adopts distill lesson `findability-lint` (docs/distillery/lessons/curation.yaml), and the draft case of
`existing-skill-and-mechanism-check`: resolve a draft's examples against the active library at review
time and say which existing skill they pick.

## Outcome

When a curator runs `skillhub maintain validate` or `skillhub skill review <id>` (CLI, MCP, web API),
each of the skill's own `routing.examples` is resolved with the current policy over the active library.
An example that does not pick the skill is a warning naming where it went (another skill, `no_skill`
or `needs_context`) and a fix. Each `routing.counter_examples` entry that still picks the skill is a
warning too. Warnings never fail validation or block activation.

## What already exists (verified 2026-10-14 at 4198bb7)

`internal/app/routing_eval.go` (`EvaluateRouting`) already holds the core logic:

- **What it measures.** For each active skill (not `system-curator`), sorted by id, it resolves every
  example with that one example removed from the skill (`leaveOneOutCatalog`, `removeKind "example"`),
  and every counter-example with that one removed (`"counter_example"`). Request: the phrase,
  `Scope = min_scope`, `Operation = operations[0]`, facts and capabilities that satisfy the skill's own
  requirements. A positive miss is any result other than `resolved` to the skill; a counter miss is
  `primary == skill` (`countersResolvedToOwnSkill`). An optional `--no-skill` suite adds more cases.
- **What it reports.** Aggregate precision, recall, no-skill recall/precision, FPR and per-skill recall,
  plus one `RoutingEvalCaseFailure` per miss: `skill_id, kind, index, phrase, got_status, got_primary`.
  The winner is reported, but there is no fix text.
- **How it runs.** Only on demand: CLI `skillhub eval routing [--json] [thresholds]`
  (internal/delivery/cli/evaluation_routing.go). No MCP tool runs it (MCP `routing_evaluate` resolves
  one request). It records an `evaluation_run_completed` telemetry event.
- **What it does not cover.** Validate and review never call it. It sees only active skills in the
  built catalog, so it cannot check a draft. It resolves through `resolveWithin`, which builds the
  distributed manifest of the winner. For a draft winner that build fails with an error, so the
  eval path cannot be reused unchanged for drafts.

Current lint wiring:

- `internal/resolver/lint.go`: `LintSkills` (validate: all active skills) and `LintTargetSkill` (review:
  one skill against active skills) with codes `missing_examples`, `generic_trigger`,
  `example_restates_trigger`, `trigger_collision`, `near_duplicate`, and `lint_pairs_skipped` above 2000 skills.
- `internal/app/metadata_lint.go` `workspaceLintWarnings`: called by `ValidateWorkspace` (CLI
  `maintain validate`, MCP workspace validate) once the canonical files are valid; maps findings to
  `Warning{code, summary: "<skill>: <message> Fix: <fix>"}`. `ValidateStaged` (pre-commit hook) does not lint.
- `internal/app/skill_review.go` `ReviewSkill`: appends `LintTargetSkill` findings to
  `activation_readiness.warnings` as `"<code>: <message> Fix: <fix>"`. CLI text prints those
  warnings only when the skill is not ready (internal/delivery/cli/skill_review.go:77-89). The web UI
  does not render them.

**Pre-existing defect found (blocks the draft case).** `ReviewSkill` passes the raw
`skill.meta.yaml` bytes to `resolverpkg.DecodeSkillDocument`, which calls `json.Unmarshal`. With YAML
input that always fails (probe: `invalid character 'a' in literal null`). The error is swallowed, so
review has never reported any routing lint finding. The catalog build turns the same file into JSON
in `catalog.parseEntity` (internal/catalog/input.go:193). Phase 1 fixes this first.

**Conclusion.** The eval does not deliver this behaviour at the right moment, so this plan is real
work. It is small, though: no new resolver loop. The plan extracts the eval's per-skill case loop and
reuses it. The new parts are a draft-injection catalog, finding mapping, a cost bound and wiring.

## Cost (measured, golden corpus, 42 active skills, 168 examples)

- `resolveWithin` with memoized skills: 1.17 ms per case. The whole `TestRoutingEvalGate` run (328
  cases, including workspace build) takes 0.83 s.
- Engine over a plain `SQLiteCatalog`: 2.14 ms per case, because `Skills()` decodes every row on each
  resolve. The lint path must decode skills once and apply leave-one-out in memory.
- The rule channel scans every active skill per request, so cost is about linear in
  `cases x active skills`: about 0.028 ms per skill-case. The live hub has 3 skills.
  An estimated 256 skills with 8 cases each would take about 14 s for workspace validate, which is
  too slow. A single-skill review (at most 20 cases with the 10-per-flag limit) stays under 1 s up to
  about 1,500 skills.
- Bound: skip self-resolution with a `self_resolution_skipped` warning when
  `cases x active skills` exceeds a constant. The starting value is 60,000 (about 2 s). Phase 2
  benchmarks this value before it is fixed.

## Phases

| # | Phase | Status | Depends on |
|---|-------|--------|------------|
| 1 | [Decode the reviewed skill like the catalog does](phase-01-decode-reviewed-skill.md) | pending | none |
| 2 | [Share the self-resolution loop with routing eval](phase-02-shared-self-resolution.md) | pending | none |
| 3 | [Emit self-resolution lint in validate and review](phase-03-lint-findings-and-wiring.md) | pending | 1, 2 |
| 4 | [Document the new warnings](phase-04-docs.md) | pending | 3 |

Phases 1 and 2 touch disjoint files and can run in parallel. Phase 3 integrates both.

## Finding contract (additive)

New `LintFinding` codes, same struct and same `Warning` / readiness string rendering as today:

| Code | Field | Value | OtherSkillID | Fix |
|------|-------|-------|--------------|-----|
| `example_resolves_elsewhere` | `routing.examples` | the example | winner, or empty | depends on status, see phase 3 |
| `counter_example_resolves_to_skill` | `routing.counter_examples` | the counter-example | empty | `Add a not_for entry that covers this request, or narrow the trigger it matches` |
| `self_resolution_skipped` | none | none | none | `Run skillhub eval routing to check examples` or `Fill in the required routing fields first` |

No field is removed or renamed. `skillhub eval routing` output, its metrics and its telemetry event
stay exactly the same.

## Acceptance criteria

- [ ] `skillhub skill review <draft>` reports each draft example that resolves elsewhere, naming the
      winning active skill or the `no_skill` / `needs_context` status, with a fix. It does this
      without activating the draft or rebuilding the catalog.
- [ ] `skillhub maintain validate` reports the same for every active skill against the built catalog.
- [ ] A counter-example that resolves to its own skill is reported in both places.
- [ ] Review also shows the existing routing lint (`trigger_collision`, `missing_examples`, ...), which
      it silently skipped before.
- [ ] Review text output shows routing lint warnings even when the skill is activation-ready.
- [ ] All new findings are warnings. Validate's exit code and `activation_readiness.ready` do not change.
- [ ] Output is identical across repeated runs on the same catalog snapshot (sorted findings, fixed policy).
- [ ] Above the work bound, one `self_resolution_skipped` warning appears and nothing is resolved.
- [ ] `skillhub eval routing --json` output on the golden corpus is byte-identical before and after.
- [ ] Lint records no telemetry.
- [ ] `make check` passes. Tests use temp workspaces only, never `/home/vantt/skill-hub`, the real HOME
      or real projects.

## Non-goals

No resolver policy or ranking change. No telemetry. No automatic edits of skills. No hard gate. No
other distill lessons. No web UI rendering of readiness warnings, which it does not show today.
No change to `ValidateStaged`.

## Risks

- **Draft retrieval is approximated.** A draft is not in the FTS index, so the injection catalog adds
  it as a search hit when a query token matches its indexed fields. A draft can then look slightly more
  findable than it will be. Validate after activation runs the exact check.
- **Golden fixtures may change.** Phase 1 makes review lint work for real, so web/CLI review goldens
  can gain warnings. Update them deliberately and review each diff.
- **Noise on small libraries.** Leave-one-out misses are common: the golden corpus has 83 failures in
  328 cases. Warnings are per example, so a weak skill lists several. This is accepted because the
  findings are warnings with a fix and are the same misses `eval routing` already reports.

## Open questions

1. Should `skill edit` / `skill create` previews also show these findings? The
   `skill.RoutingImpactHook` seam (internal/skill/lifecycle.go:138, `BasicRoutingImpactHook`) is the
   designed place for this ("until a later phase supplies a semantic routing evaluator"). It is not
   in scope here because the brief names lint, review and readiness.
2. `leaveOneOutCatalog` removes the example from `Skills()` but not from the FTS index, so the left-out
   phrase still makes its skill a candidate. Fixing this would change `eval routing` metrics and the
   gate thresholds in testdata/routing/gate-v1.json, so the plan keeps the eval's behaviour. Should a
   later change close that leak?
3. Is 60,000 skill-cases (about 2 s) the right budget for `maintain validate`, or should validate
   skip self-resolution above N active skills and point to `eval routing`?
4. Should the review JSON gain a structured `routing_findings` array (code, other_skill_id, value,
   fix) for agents, in addition to the existing strings? The plan keeps the strings only.
