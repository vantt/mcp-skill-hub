---
phase: 3
title: "Affected-examples check"
status: pending
effort: 6h
depends_on: [1, 2]
---

# Phase 3: Affected-examples check

## Context

Given a served generation, a policy and a set of changes `[]skillChange{ID, Old *Skill, New *Skill}`
(Old is nil for an added skill, New is nil for a removed one), this phase decides which cases to
resolve and which results to report. plan.md, "The affected-examples rule", gives the reasoning and
the probe evidence: 84 mutations, 169 changed results, 0 outside the predicted set; median 40 of 294
cases predicted.

## Requirements

- **Influence predicate, in the resolver package, with no ranking change.** Add
  `resolver.Influences(skill Skill, request Request, policy Policy, linked bool) bool` next to the
  existing code. It reuses `NormalizeRequest`, the `scopeLevel` filter, `evaluateCompatibility` and
  `scoreSkill` exactly as `Resolve` applies them. It is true when:
  - the skill is not hard-excluded and its score is at least `ApplicabilityFloor`; or
  - `linked` is true and the score is at least `ApplicabilityFloor - 0.15`; or
  - the skill is hard-excluded and its excluded relevance (the expression in resolver.go:226) is at
    least `ApplicabilityFloor`.

  `linked` is true when the skill has `distinguish_from`, or another active skill's
  `distinguish_from` names it.

  The `0.15` and the excluded-relevance expression move into named helpers that `Resolve` and
  `Influences` both call, so the two cannot drift. Scores stay unchanged; a resolver test asserts it.
- **Case selection** for a change set C:
  1. Own cases of every `New` in C: all of them.
  2. Other active skills' examples and counter-examples: kept when any change in C has
     `Influences(New, req)`. Cases are sorted by (owner id, kind, index).
- **After-state:** resolve each selected case with `engineResolver` over
  `overlayCatalog(served, C.New)` composed with leave-one-out on the case owner.
- **Reporting:**
  - Own cases use the existing rules: an example misses unless it resolves to its own skill; a
    counter-example misses if it resolves to its own skill.
  - Another skill Y's example is reported as `example_taken_by` (SkillID X, OtherSkillID Y) when it
    misses and X in C takes part:
    - `primary == X`;
    - the status is not `resolved` and X is in `TopKSkillIDs[:2]`; or
    - the reason is `constraint_conflict` and X is hard-excluded with relevance at least the floor.
  - Other skills' counter-examples are resolved, but only reported when X is the winner, which is
    harmless for Y. An unmasked weakness of Y itself is left to `--full` (plan.md open question 2).
- **Before-state, only for reported `example_taken_by` cases:** resolve the same case over
  `overlayCatalog(served, C.Old)`, where Old is HEAD or served, chosen by the caller. The message says
  "now resolves to X" when the before-winner differs, else "already resolves to X". For ambiguity it
  says "now needs a choice between X and Y" or "already needs one".
- **Bound:** before resolving, compute `selected cases x active skills`. If it exceeds
  `maxSelfResolutionWork`, return one `self_resolution_skipped` and resolve nothing.
- **Full mode** is the same function with C = every active skill, Old = New = the served version. It
  reports only own-case misses, not `example_taken_by` duplicates.
- **Determinism:** the change set is sorted by id, cases are sorted, there is one policy and one
  served snapshot per run, and findings are sorted by (SkillID, Code, Field, Value, OtherSkillID)
  before they are returned.

## Files

- Modify `internal/resolver/resolver.go` and `internal/resolver/evidence.go`: export `Influences` and
  extract the two shared helpers. Scores and decisions stay the same.
- Modify `internal/resolver/lint.go`:
  - the finding builders for `example_resolves_elsewhere`, `counter_example_resolves_to_skill`,
    `example_taken_by` and `self_resolution_skipped`;
  - fix texts in the existing short, imperative style:
    - resolved to another skill: `Add a not_for entry on <winner> that covers this request, add
      distinguish_from, or sharpen this skill's triggers`;
    - no_skill: `Add a trigger that names this request's domain object, or rephrase the example`;
    - needs_context: `Narrow the triggers so one skill wins, or add distinguish_from with a
      discriminator question`;
    - counter-example: `Add a not_for entry that covers this request, or narrow the trigger it matches`;
    - `example_taken_by`: `Add a not_for entry on <X> that covers this request, narrow <X>'s
      triggers, or add distinguish_from between <X> and <Y>`.
- Create `internal/app/affected_examples.go`: `checkSkillChanges(ctx, view, policy, changes,
  mode) []resolverpkg.LintFinding`.
- Create `internal/resolver/influence_test.go`, and extend `internal/resolver/lint_test.go`.
- Create `internal/app/affected_examples_test.go`.

## Steps

1. Resolver test: refactoring the helpers does not change `Resolve` output on the golden corpus.
   Compare all 294 leave-one-out results before and after the refactor.
2. **Soundness test** (`TestInfluencePredicateCoversEveryChangedResult`):
   - load the golden corpus in memory;
   - for a deterministic set of skills, apply two mutations: append another skill's first example as
     a trigger, and replace all triggers with an unrelated phrase;
   - assert that every case whose status or primary changed is predicted by `Influences(old) ||
     Influences(new)`, and every reported `example_taken_by` case by `Influences(new)`.

   The full sweep of 42 skills x 2 mutations took about 40 s in the probe. Run 8 skills by default;
   skip the full sweep under `-short`, or gate it like the perf tests.
3. Lint-mapping tests: each status, the counter case, both `example_taken_by` wordings and the fix text.
4. App tests in temp workspaces:
   - own misses;
   - the steal case;
   - a removed skill;
   - an added draft;
   - the bound;
   - two runs give identical output;
   - no telemetry file is created.

## Validation

```bash
go test ./internal/resolver -run 'Influence|Lint' -count=1
go test ./internal/app -run 'AffectedExamples|SelfResolution|RoutingEval' -count=1
go test -race ./internal/app -run 'AffectedExamples' -count=1
make check
```

## Risks

- **Predicate drift if the resolver's decision rules change later.** Mitigation: shared helpers and
  the soundness test, which then fails loudly.
- **Retrieval displacement above `CandidateLimit`** is not modelled. Mitigation: documented, and
  `--full` covers it.

## Rollback

Revert the commit. Phases 1 and 2 stay valid on their own. The resolver refactor is
behaviour-neutral, as step 1 shows.
