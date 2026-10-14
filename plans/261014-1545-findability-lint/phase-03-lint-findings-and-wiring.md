---
phase: 3
title: "Emit self-resolution lint in validate and review"
status: pending
effort: 4h
depends_on: [1, 2]
---

# Phase 3: Emit self-resolution lint in validate and review

## Context

Lint codes and their fix text live in `internal/resolver/lint.go`. They are rendered:

- by `workspaceLintWarnings` (internal/app/metadata_lint.go) into `Warning{code, summary: "<skill>: <message> Fix: <fix>"}`;
- by `ReviewSkill` into `activation_readiness.warnings` as `"<code>: <message> Fix: <fix>"`.

CLI review text prints readiness warnings only when the skill is not ready
(internal/delivery/cli/skill_review.go:77-89). That hides findings for an active or ready skill.

## Requirements

- Finding mapping and fix text sit next to the existing rules in `internal/resolver/lint.go`, as a
  pure function. The app layer only produces cases.
- Codes, fields and fix wording follow the existing style: short and imperative.
  - `example_resolves_elsewhere`, Field `routing.examples`, Value = the example, OtherSkillID = the winner.
    - Status `resolved` to another skill. Message `example %q on skill %q resolved to %q`.
      Fix `Add a not_for entry on <winner> that covers this request, add distinguish_from, or sharpen this skill's triggers`.
    - Status `no_skill`. Message `example %q on skill %q resolved to no skill`.
      Fix `Add a trigger that names this request's domain object, or rephrase the example`.
    - Status `needs_context`. Message `example %q on skill %q needs context (<reason codes>)`.
      Fix `Narrow the triggers so one skill wins, or add distinguish_from with a discriminator question`.
    - Any other status. Use the same message shape with the status name, and the `no_skill` fix.
  - `counter_example_resolves_to_skill`, Field `routing.counter_examples`, Value = the phrase.
    Message `counter-example %q still resolves to skill %q`.
    Fix `Add a not_for entry that covers this request, or narrow the trigger it matches`.
  - `self_resolution_skipped`, SkillID set in review and empty in validate.
    - Over the bound. Message `self-resolution lint skipped: <cases> examples x <skills> active skills exceeds <bound>`.
      Fix `Run skillhub eval routing to check examples`.
    - Draft not resolvable, because the resolver rejected the projected skill. Message carries the error.
      Fix `Fill in the required routing fields first`.
- Validate: all active skills except `system-curator`, checked against the catalog generation that
  lint already opens, with the policy from that generation. This needs a `catalog.View`
  (`ObserveCurrentLocked`, as the eval uses) or `resolverpkg.LoadPolicy(handle.DB)`. Pick one, keep
  one snapshot per run, and do not open the catalog twice.
- Review: run only for lifecycle `draft` or `active`. Target = the canonical skill decoded in phase 1,
  injected with `withTargetCatalog`, against active skills.
- Determinism: within the self-resolution block, sort findings by `(SkillID, Code, Field, Value,
  OtherSkillID)`, and append the block after the existing findings. The existing order is unchanged.
- CLI review text: print routing lint warnings under their own "Routing lint" heading whenever there
  are any, whether or not the skill is ready. Keep "Activation readiness" for missing fields and
  scaffold. JSON keeps the strings in `activation_readiness.warnings`, which is an additive change.
- Warnings only. Validate's exit status and `ready` are unchanged.

## Files

- Modify `internal/resolver/lint.go`: add `SelfResolutionMiss` (or reuse the case fields) and
  `SelfResolutionFindings(skillID string, misses []SelfResolutionMiss) []LintFinding`, plus the
  skipped-finding helper.
- Modify `internal/resolver/lint_test.go`: table tests for each status, the counter case and the fix text.
- Modify `internal/app/metadata_lint.go`: run self-resolution after `LintSkills` within the bound.
- Modify `internal/app/skill_review.go`: run self-resolution for the target within the bound.
- Modify `internal/delivery/cli/skill_review.go`: add the "Routing lint" heading. Tell routing lint
  apart from the readiness warning `skill is marked active but has unmet activation requirements`:
  keep that one under readiness.
- Modify tests: `internal/app/metadata_lint_test.go`, `internal/app/skill_review_test.go`,
  `internal/delivery/cli/skill_review_test.go`, and goldens if their fixtures miss.

## Steps

1. Test first in `lint_test.go` for the mapping and fix text, then implement.
2. Validate integration test in a temp workspace. Two active skills, where A's example matches B's
   trigger: expect `example_resolves_elsewhere` with `other_skill_id = B`. B gets a counter-example
   that matches its own trigger: expect `counter_example_resolves_to_skill`. Run twice and assert
   identical output.
3. Review integration test for a draft. A draft example that picks an active skill names it, and the
   draft is not activated. Check that `git status` in the temp hub and the catalog pointer are unchanged.
4. Bound test: set the bound low through a package variable that only tests override, or use a small
   bound with a test-only setter. Expect exactly one `self_resolution_skipped` and no resolve calls.
5. Incomplete draft, for example with no `min_scope`: expect `self_resolution_skipped` with the
   required-fields fix, and the readiness missing-fields list stays unchanged.
6. CLI text test: a ready draft with a miss shows the "Routing lint" heading.
7. Check that the eval snapshot from phase 2 still passes, and that lint writes no telemetry (no
   telemetry DB/file created in the temp workspace by validate or review).

## Validation

```bash
go test ./internal/resolver -run 'Lint' -count=1
go test ./internal/app -run 'Lint|Review|SelfResolution|RoutingEval' -count=1
go test ./internal/delivery/cli ./internal/delivery/web ./internal/delivery/mcpserver -run 'Review|Validate' -count=1
make check
```

## Risks

- **Validate gets slower on large hubs.** The bound and the skip finding cover this. The pre-commit
  `--staged` path does not lint.
- **Noise.** Many leave-one-out misses on weak skills. This is accepted: they are warnings, and they
  are the misses `eval routing` already reports.
- **MCP and web consumers parse `activation_readiness.warnings` strings.** The new strings use the
  same `"<code>: ..."` shape. No consumer branches on specific codes (the web UI does not render them).

## Rollback

Revert the commit. Phases 1 and 2 stay valid on their own.
