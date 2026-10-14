---
title: "Self-resolution lint for skill examples"
description: "Warn when a skill's own examples do not resolve to it, its counter-examples do, or a change to it takes other skills' examples. Review and validate check only the examples a change can affect; a bounded full scan stays as the safety net."
status: in-progress
priority: P2
effort: 3d
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

When skill X changes, a curator learns, as warnings, about every routing result the change can break,
and nothing else:

1. **X's own cases.** Each example of X that does not resolve to X is reported, with where it went
   (another skill, `no_skill` or `needs_context`) and a fix. Each counter-example of X that still
   resolves to X is reported too.
2. **Other skills' examples that X now takes.** These are reported as "your change makes `<Y>`'s
   example '…' resolve to `<X>`" (or "makes it ambiguous between `<X>` and `<Y>`"), naming both
   skills, with a fix on X.

This runs in:

- `skillhub skill review <id>` (CLI, MCP, web API), for a draft or an active skill;
- `skillhub maintain validate`, for the skills changed against HEAD;
- `skillhub maintain validate --staged`, for the skills staged against HEAD.

`skillhub maintain validate --full` checks every active skill within a work bound. It is the safety
net for policy changes and periodic runs. Warnings never fail validation, the pre-commit hook or
activation.

## The affected-examples rule (user decision, refined by evidence)

When skill X changes from X_old to X_new, a routing result can change only for:

1. X's own examples and counter-examples. These are always checked.
2. Other skills' cases where X takes part in the decision, before or after the change.

The resolver (internal/resolver/resolver.go:134-361) decides from:

- the top two scored candidates;
- the highest hard-excluded relevance (the `constraint_conflict` check);
- which skills are retrieved, with each channel capped at `CandidateLimit` = 40.

`scoreSkill(skill, request, policy)` is a pure function of one skill and the request. X therefore
takes part only when:

- its own score reaches `ApplicabilityFloor`; or
- it reaches `ApplicabilityFloor - 0.15` and X has a discriminator link; or
- X is hard-excluded with a relevance at or above the floor.

Below that, X can neither win nor be the applicable runner-up (resolver.go:327-331). This is the
**influence predicate**. It needs no resolve, only one `scoreSkill` call per case and version.

Why not plain candidate membership ("X matches lexically")? FTS is an OR of query tokens, and every
skill that shares the operation is a rule candidate. On the golden corpus, X is a lexical candidate
for 113-261 of the 294 cases (median 230). The influence predicate cuts that to a median of 40.

**Soundness, measured.** A probe ran on the golden corpus: 42 skills, 294 cases, the in-memory
catalog, default policy. It applied 84 mutations, two per skill: steal another skill's example as a
trigger, and drop all triggers. Those mutations changed 169 case results, and the predicate
(old ∪ new) predicted all 169, with 0 misses. One known gap: above 40 active skills, X entering a
capped retrieval list can push out a top-two skill. The predicate does not model that.
`validate --full` catches it.

**What is reported, and why "before" is needed only for labels.** A new problem in another skill's
case always has new X in the after-state's top two, or X causing the constraint conflict. If old X
was the winner and new X is not, the case either became correct or still misses as it did before.
So the after-state is resolved only for cases where **new X** influences. A case is reported when it
misses and the decision involves X. "Before" is resolved only for those reported cases, to word them
"now resolves to X" (new) or "already resolves to X". The "before" source is:

- **review:** the served catalog generation, which is what agents get now;
- **validate:** HEAD versions of the changed skills (`git show HEAD:<path>`), overlaid on the served
  generation, because a rebuilt generation may already contain the edit.

Another skill's counter-example that starts resolving to its own skill only after X stops taking it
shows a weakness that skill already had. It is left to `--full`; see open question 2.

## What already exists (verified 2026-10-14 at 4198bb7)

`internal/app/routing_eval.go` (`EvaluateRouting`) already holds the core logic:

- **What it measures.** For each active skill (not `system-curator`), sorted by id, it resolves every
  example with that one example removed from the skill (`leaveOneOutCatalog`, `removeKind "example"`),
  and every counter-example with that one removed (`"counter_example"`). Request: the phrase,
  `Scope = min_scope`, `Operation = operations[0]`, facts and capabilities that satisfy the skill's own
  requirements. A positive miss is any result other than `resolved` to the skill; a counter miss is
  `primary == skill`.
- **What it reports.** Aggregate metrics plus one `RoutingEvalCaseFailure` per miss
  (`skill_id, kind, index, phrase, got_status, got_primary`). The winner is reported, but there is no
  fix text.
- **How it runs.** Only on demand: CLI `skillhub eval routing`. No MCP tool runs it (MCP
  `routing_evaluate` resolves one request). It records an `evaluation_run_completed` telemetry event.
- **What it does not cover.** Validate and review never call it. It sees only active skills in the
  built catalog, so it cannot check a draft. It resolves through `resolveWithin`, which builds the
  distributed manifest of the winner. For a draft winner that build fails, so the eval path cannot be
  reused unchanged for drafts.

Current lint wiring:

- `internal/resolver/lint.go`: `LintSkills` (validate) and `LintTargetSkill` (review), codes
  `missing_examples`, `generic_trigger`, `example_restates_trigger`, `trigger_collision`,
  `near_duplicate`, `lint_pairs_skipped`.
- `internal/app/metadata_lint.go` `workspaceLintWarnings`: runs from `ValidateWorkspace` (CLI and MCP)
  on the served generation. `ValidateWorkspaceStaged` (internal/app/git_index_snapshot.go:235), used
  by the pre-commit hook, does not lint.
- `internal/app/skill_review.go`: findings go to `activation_readiness.warnings`. CLI text prints them
  only when the skill is not ready. The web UI does not render them.

**Pre-existing defect (blocks the draft case).** `ReviewSkill` passes YAML to
`DecodeSkillDocument`, which only parses JSON (probe: `invalid character 'a' in literal null`). The
error is swallowed, so review has never reported any routing lint. Phase 1 fixes this.

## Cost

Measured on the golden corpus (42 skills, 294 cases):

| Step | Cost |
|------|------|
| One resolve, skills decoded once (memoized) | 0.90 ms |
| One resolve, plain `SQLiteCatalog` (re-reads every skill row on each resolve) | 2.14 ms |
| Influence prefilter over all 294 cases, both versions, one changed skill | about 9 ms |
| Cases to resolve after prefilter (own + influenced; old ∪ new, an upper bound) | min 10, median 40, p90 98, max 272 |
| **One changed skill, end to end** (prefilter + after-resolves + before-resolves for findings) | **about 0.05 s median, 0.1 s p90, 0.25 s worst** |
| Full scan (`--full`, 294 resolves) | about 0.27 s |

Estimate for 256 skills, assuming about 7 cases per skill (about 1,800 cases):

- A resolve grows with the number of skills, to about 5.5 ms.
- The prefilter is about 55 ms per changed skill.
- If the influenced fraction stays at about 14 % (median), one changed skill costs about 1.4 s; at
  the p90 fraction it is about 3.3 s.
- A full scan is about 10 s.

On the small corpus the incremental check is about as cheap as a full scan. The gain shows on larger
libraries, and in reports that list only what the change caused.

Bound: when `resolves x active skills` would exceed `maxSelfResolutionWork`, report one
`self_resolution_skipped` warning and resolve nothing. The starting value is 60,000 (about 2 s), and
phase 2 benchmarks it before it is fixed. The same bound applies to every mode.

## Validate modes (decision and justification)

Accepted by the user on 2026-10-14: `maintain validate` checks only changed skills by default, and
`maintain validate --staged` (pre-commit) runs the same check, warnings only.

- **`maintain validate` defaults to the changed skills. `--full` scans every active skill.**
  - Justification: the warnings a curator needs after an edit are the ones the edit causes. The cost
    then scales with the change, not the library, and a validate on a clean tree costs nothing.
  - The full scan answers a different question ("is the whole library healthy?"). It belongs to
    policy changes, periodic runs and `eval routing` totals.
  - The pair lint (`trigger_collision`, ...) still runs over all active skills as today. Only
    self-resolution is scoped.
  - Changed set: `git diff --name-only HEAD` (staged and unstaged) plus untracked files
    (`git ls-files --others --exclude-standard -- skills/`), mapped from `skills/<collection>/<id>/...`
    to `<id>`.
  - Outside Git, or with no HEAD commit, every active skill counts as changed, under the bound.
  - If `config/recommendation.yaml` changed, the mode emits one `self_resolution_skipped` warning
    with the fix "Rebuild, then run `skillhub maintain validate --full`", because a policy change
    affects every example.
- **`maintain validate --staged` (pre-commit) runs the same check for the staged skills.** After-state:
  the index versions from the existing `CaptureGitIndexSnapshot`. Before-state: HEAD.
  - Measured cost per changed skill: about 0.05 s median and 0.25 s worst on the 42-skill corpus.
  - Estimated cost: about 1.4 s median at 256 skills, capped by the bound.
  - Failure semantics are unchanged: these are warnings only, the hook's exit code depends only on
    validation errors, and a missing catalog generation skips the check silently.
- **MCP workspace validate** uses the default (changed) mode. Full mode is CLI-only (open question 4).

## Reconciliation with the curation redesign (2026-10-14)

Recorded by the coordinator before the remaining phases run, against
`plans/261014-1740-curation-ux-redesign/design-decisions.md` (D1-D9) and the redesign as implemented on branch
`feat/curation-wave`.

- **Phase 1 is done** (commit `565976b`): review decodes the skill like the catalog and lints it through
  `lintReviewedSkill` (`internal/app/skill_review.go`). Skip it.
- **One lint entry point, three consumers.** `lintReviewedSkill` is now called by `skill review`
  (`skill_review.go`), the `skill commit` preview (`internal/app/skill_commit_plan.go`) and the web changes
  preview (`internal/app/skill_tree_edit.go`). Phase 4 wires the affected-examples check into that shared path,
  not into `ReviewSkill` alone, so its warnings show in the `skill commit` preview (CLI, checkout or folder, new
  skill or update) and in the web Review screen as well as in `skill review`. The after-state is the proposed
  skill (`plan.final` files); the before-state for review and both previews is the served generation, as the
  plan already says for review.
- **Drafts come from a project folder now** (D7, D8): a new skill reaches review first as a `skill commit
  <folder>` preview, before any hub draft exists. The draft case of phase 3 must therefore accept the proposed
  bytes directly (the bytes input phase 1 already added), not only a hub draft.
- **Validate modes are unchanged.** Every confirmed change is auto-committed through Git (D3), so the pre-commit
  hook's `maintain validate --staged` runs on each skillhub commit. It stays warnings-only and must stay inside the
  same work bound; it must not take the commit lock (`runtime/locks/commit.lock`).
- **Open question 1 is answered by the redesign:** `skill edit` and `skill create` previews are no longer the
  editing path; the `skill commit` and web previews above are, and they get the findings through the shared path.
- **No command, flag or flow from D1-D9 changes.** `--full` and `--staged` on `maintain validate` are this
  plan's own flags.

## Phases

| # | Phase | Status | Depends on |
|---|-------|--------|------------|
| 1 | [Decode the reviewed skill like the catalog does](phase-01-decode-reviewed-skill.md) | pending | none |
| 2 | [Share the self-resolution loop and add the overlay catalog](phase-02-shared-self-resolution.md) | pending | none |
| 3 | [Affected-examples check](phase-03-affected-examples-check.md) | pending | 1, 2 |
| 4 | [Wire review and validate modes](phase-04-wire-review-and-validate.md) | pending | 3 |
| 5 | [Document the new warnings and modes](phase-05-docs.md) | pending | 4 |

Phases 1 and 2 touch disjoint files and can run in parallel.

## Finding contract (additive)

These use the same `LintFinding` struct and the same `Warning` / readiness-string rendering as today:

| Code | SkillID | OtherSkillID | Field / Value | When |
|------|---------|--------------|---------------|------|
| `example_resolves_elsewhere` | owner | winner or empty | `routing.examples` / phrase | An own example misses |
| `counter_example_resolves_to_skill` | owner | none | `routing.counter_examples` / phrase | An own counter-example resolves to its owner |
| `example_taken_by` | changed skill X | owner Y | `routing.examples` (of Y) / phrase | Y's example now resolves to X, or is ambiguous between X and Y (message says which, and "now" or "already") |
| `self_resolution_skipped` | target or empty | none | none | Over the bound, policy changed, or the resolver rejected an incomplete draft |

No field is removed or renamed. `skillhub eval routing` output, its metrics and its telemetry event
stay the same. New CLI flag: `maintain validate --full`.

## Acceptance criteria

- [ ] Review of skill X alone reports the case where a trigger added to X steals Y's example:
      `example_taken_by`, SkillID X, OtherSkillID Y, "now resolves". This is a named test in phase 4.
- [ ] `skillhub skill review <draft>` reports the draft's own misses with the winning skill, and the
      active skills' examples it would take. It does not activate or rebuild anything.
- [ ] `maintain validate` checks only the skills changed against HEAD. A clean tree runs no
      self-resolution.
- [ ] `maintain validate --staged` checks the staged skills, and the hook's exit status is unchanged.
- [ ] `maintain validate --full` checks every active skill under the bound.
- [ ] Over the bound, or when the policy changed, exactly one `self_resolution_skipped` appears.
- [ ] Review also shows the existing routing lint, which it silently skipped before. Review text shows
      routing lint even when the skill is ready.
- [ ] The influence predicate passes the mutation soundness test (no changed result outside the
      predicted set) on the golden corpus.
- [ ] Output is identical across repeated runs on the same snapshot (sorted inputs and findings, one
      policy).
- [ ] `skillhub eval routing --json` on the golden corpus is byte-identical before and after.
      Lint records no telemetry. All new findings are warnings.
- [ ] `make check` passes. Tests use temp workspaces only, never `/home/vantt/skill-hub`, the real HOME
      or real projects.

## Non-goals

No resolver policy or ranking change: the resolver only exposes its existing pure scoring as a
predicate. No telemetry. No automatic edits of skills. No hard gate. No other distill lessons. No web
UI rendering of readiness warnings. No persisted cache of earlier results.

## Risks

- **Retrieval displacement above 40 skills** is not modelled by the predicate. Mitigation: `--full`
  catches it; this is documented.
- **A stale served generation.** Unchanged skills are read from the served generation. If it lags the
  working tree, the overlay is checked against old neighbours. Validate already lints the served
  generation; review reports divergence.
- **Draft retrieval is approximated** in the overlay's search, and slightly over-approximates FTS.
- **Golden fixtures may change** once review lint works; review each diff.
- **Noise.** Leave-one-out misses on X's own cases are common. They are per example, with a fix.

## Open questions

1. Should `skill edit` / `skill create` previews show these findings through
   `skill.RoutingImpactHook` (internal/skill/lifecycle.go:138)? This is the natural next consumer of
   the phase 3 check. It is not in scope.
2. Should another skill's counter-example that starts resolving to its own skill after X changes be
   reported on review of X? It is left to `--full`, because it shows that skill's old weakness, not a
   new problem caused by X.
3. `leaveOneOutCatalog` removes the example from scoring but not from FTS. Closing that would change
   `eval routing` metrics and testdata/routing/gate-v1.json. Should a later change close it?
   Decided 2026-10-14: yes, as a separate change after this plan; tracked as F16 in
   docs/plans/2026-10-10-observation-backlog.md.
4. Should MCP workspace validate accept a `full` option, or stay with the changed-skills default?
5. Should the review JSON gain a structured `routing_findings` array (code, other_skill_id, value,
   fix) in addition to the strings? The plan keeps strings only.
