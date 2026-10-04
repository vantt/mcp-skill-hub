---
phase: 11
title: "Metadata lint"
status: pending
priority: P2
effort: 6h
dependencies: [1, 9]
---

# Phase 11: Metadata lint

## Context

- Design rule: "Duplicate detection/evaluation creates curation warnings for frequently ambiguous pairs" (`docs/design/03-resolver-design.md` §3 validation rules).
- `skillhub validate` → `executeValidate` (`internal/delivery/cli/validate_staged.go:41`) → `app.WorkspaceService.ValidateWorkspace`; human output prints only the summary on success (`internal/delivery/cli/workspace.go:107-109`); `app.Result.Warnings` already exists (`internal/app/result.go`).
- `skillhub skill review` → `app.SkillService.ReviewSkill`; `SkillReviewResult.ActivationReadiness.Warnings` (`internal/app/skill_review.go:21-26`).
- Tokenizer and overlap helpers are unexported in `internal/resolver/evidence.go` (`tokenize`, `overlap`, `bestOverlap`, `stopWords`).

## Requirements

1. `resolver.LintSkills(skills []Skill) []LintFinding` in the resolver package (reuses its tokenizer, so lint and routing agree on what "overlap" means). `LintFinding{Code, SkillID, OtherSkillID, Field, Value, Message, Fix}`.
2. Rules (all warnings, never errors):

   | Code | Condition | Fix text |
   |---|---|---|
   | `trigger_collision` | A trigger of skill A and a trigger of skill B have token overlap ≥ 0.8 (or equal after normalization), and neither skill names the other in `distinguish_from`, `not_for`/`counter_examples` (token overlap ≥ 0.5 with the other's name or triggers), or `equivalent_to` | Add `distinguish_from` or narrow one trigger |
   | `generic_trigger` | Every token of a trigger is in a generic stoplist (`code, coding, help, fix, task, work, write, create, update, change, build, make, do, run, use, general, stuff, thing, project, app, file, review, test, debug, issue, problem`) or the trigger has fewer than 2 tokens after the resolver stopword filter | Make the trigger name the domain object |
   | `missing_examples` | Active skill with fewer than 3 `routing.examples` | Add 3–5 natural task phrasings |
   | `example_restates_trigger` | An example has token overlap ≥ 0.9 with one of the same skill's triggers | Rephrase as a real request |
   | `near_duplicate` | Two active skills whose combined description + triggers token sets overlap ≥ 0.6 with no `distinguish_from`, `equivalent_to`, or cross `not_for` between them | Add `distinguish_from` or `equivalent_to`, or merge |
3. Workspace lint input: active skills from the current catalog generation via `resolverpkg.NewSQLiteCatalog(...).Skills`. Pairwise rules are O(n²) over at most a few hundred skills; skip pair rules above 2,000 active skills with a single warning.
4. `ValidateWorkspace` appends findings to `Result.Warnings` (code, message, fix). When the catalog is unavailable, lint is skipped with one warning; canonical validation results are unchanged. `--staged` validation skips lint (staged content is not projected).
5. `ReviewSkill` runs the rules for the reviewed skill against all active skills (the reviewed skill may be a draft; it is projected from its canonical metadata with the same `decodeRouting` path) and appends findings to `ActivationReadiness.Warnings`.
6. Human output of `skillhub validate` prints warnings after the summary (`WARN <code> <skill>: <message>` + `FIX:`); exit code stays 0 when only warnings exist.

## Files

Create:
- `internal/resolver/lint.go`, `internal/resolver/lint_test.go`

Modify:
- `internal/resolver/sqlite_catalog.go` (export a small `DecodeSkillDocument(id string, contentJSON []byte) (Skill, error)` wrapper around `decodeRouting` for draft projection)
- `internal/app/workspace.go` (`ValidateWorkspace` appends lint warnings)
- `internal/app/skill_review.go` (review appends lint warnings)
- `internal/app/workspace_test.go`, `internal/app/skill_review_test.go`
- `internal/delivery/cli/workspace.go` (print warnings in human output)
- `internal/delivery/cli/ux_behavior_test.go` or a new focused test for the printed warnings

## Steps

1. Implement rules with table tests (one positive and one suppressed case per rule).
2. Wire into validate and review; JSON output carries warnings; human output prints them.
3. Run lint over the golden-v1 materialized workspace from Phase 10 and over the overlay fixture; fix fixture metadata only if a finding is a genuine defect, and note any accepted findings in the Phase 10 baseline report.

## Tests and validation

- `go test ./internal/resolver/ -run Lint ./internal/app/ -run 'Validate|Review' ./internal/delivery/cli/`
- `skillhub validate --json` on a workspace with a seeded collision shows `trigger_collision` and exits 0.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Noisy warnings erode trust | Medium × Low | Conservative thresholds; suppression through existing relationship fields; every warning has a concrete fix. |
| Validate slows on large catalogs | Low × Low | O(n²) only on active skills with token sets precomputed; cap at 2,000. |

## Rollback

Revert; warnings disappear, no data changes.
