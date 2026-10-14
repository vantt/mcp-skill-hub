---
phase: 4
title: "Wire review and validate modes"
status: pending
effort: 6h
depends_on: [3]
---

# Phase 4: Wire review and validate modes

## Context

- `ReviewSkill` (internal/app/skill_review.go) appends lint strings to `activation_readiness.warnings`.
  Phase 1 makes it decode the canonical skill.
- `ValidateWorkspace` → `workspaceLintWarnings` (internal/app/metadata_lint.go) lints the served
  generation. It is used by CLI `maintain validate` (internal/delivery/cli/validate_staged.go) and by
  MCP workspace validate (internal/delivery/mcpserver/workspace_tools.go).
- `ValidateWorkspaceStaged` (internal/app/git_index_snapshot.go:235) validates an index snapshot from
  `CaptureGitIndexSnapshot` for the pre-commit hook. It has no lint today.
- CLI review text prints readiness warnings only when the skill is not ready
  (internal/delivery/cli/skill_review.go:77-89).

## Requirements

- **Review `<id>`** (lifecycle `draft` or `active` only): C = {id}.
  - New: the canonical skill (phase 1).
  - Old: the served version, or nil when id is not active in the served generation.
  - Before-state: the served generation.
  - Findings go to `activation_readiness.warnings` as `"<code>: <message> Fix: <fix>"`.
  - When the resolver rejects an incomplete draft, add one `self_resolution_skipped` with the fix
    `Fill in the required routing fields first`.
- **`maintain validate`, default changed mode:**
  - Changed ids: `git diff --name-only HEAD` plus `git ls-files --others --exclude-standard --
    skills/`, mapped from `skills/<collection>/<id>/...`. A change to SKILL.md counts, because the
    description affects scoring.
  - New: the working-tree file decoded with phase 1's function, or nil if deleted or not active.
  - Old: the HEAD blob (`git show HEAD:<path>`), or nil if absent or not active.
  - Before-state: HEAD versions overlaid on the served generation.
  - No Git, or no HEAD: every active skill counts as changed, under the bound.
  - `config/recommendation.yaml` changed: one `self_resolution_skipped` with the fix `Rebuild, then
    run skillhub maintain validate --full`.
  - A clean tree runs no self-resolution and prints no message.
- **`maintain validate --full`:** full mode over every active skill, bounded.
- **`maintain validate --staged`:** C = the staged ids (`git diff --cached --name-only HEAD`). New
  comes from the index snapshot `ValidateWorkspaceStaged` already captures, and Old from HEAD.
  - The hook's exit status still depends only on validation issues.
  - If the catalog generation is missing, skip without a warning, so a commit in a fresh hub is not
    noisy.
  - Lint runs only when the staged validation has no issues, as `ValidateWorkspace` does.
- **MCP workspace validate:** changed mode. No new input field (open question 4).
- **Pair lint is unchanged:** `LintSkills` still runs over all active skills in every mode.
- **CLI review text:** a "Routing lint" heading lists the lint warnings whenever there are any,
  whether or not the skill is ready. The readiness heading keeps missing fields, the scaffold note and
  `skill is marked active but has unmet activation requirements`.
- `--full` is parsed in `validateFlags`, appears in help (internal/delivery/cli/help.go) and is
  rejected together with `--staged`.

## Files

- Modify `internal/app/skill_review.go`
- Modify `internal/app/metadata_lint.go`: take a mode and a change set.
- Modify `internal/app/operations.go`: `ValidateWorkspace` gains the mode. Keep the exported signature
  compatible through a new options method or variant, and keep the old method for MCP.
- Modify `internal/app/git_index_snapshot.go`: lint after the staged validation.
- Create `internal/app/changed_skills.go`: the Git changed-set and HEAD-blob reader. Reuse the Git
  helpers `getSkillGitSummary` already uses; do not shell out in a new style.
- Modify `internal/delivery/cli/validate_staged.go` and `internal/delivery/cli/help.go`: `--full`.
- Modify `internal/delivery/cli/skill_review.go`: the "Routing lint" heading.
- Tests: `internal/app/skill_review_test.go`, `internal/app/metadata_lint_test.go`,
  `internal/app/changed_skills_test.go`, `internal/delivery/cli/validate_staged_test.go`,
  `internal/delivery/cli/skill_review_test.go`, and goldens if their fixtures miss.

## Steps

1. **`TestReviewReportsExampleTakenByNewTrigger`** (the required test):
   - set up a temp hub with active skills X and Y, built and committed;
   - edit X's canonical meta to add a trigger equal to one of Y's examples, without rebuilding;
   - review X alone;
   - expect `example_taken_by` with SkillID X, OtherSkillID Y and "now resolves";
   - confirm the catalog pointer and `git status` are otherwise unchanged.
2. Draft review: a draft whose example picks active Y reports `example_resolves_elsewhere` naming Y.
   A draft that takes Y's example reports `example_taken_by`.
3. Changed mode:
   - a clean tree gives no self-resolution findings;
   - editing X in the working tree reports X's cases only;
   - a staged edit is reported by `--staged`, and the hook exit code is 0 with only warnings;
   - a policy file change gives one skip warning.
4. `--full` reports own misses for every active skill. Over the bound it gives exactly one skip.
5. Determinism: run each mode twice and compare the JSON byte for byte.
6. CLI text: a ready draft with a miss shows "Routing lint". `--full --staged` is rejected.

## Validation

```bash
go test ./internal/app -run 'Review|Lint|ChangedSkills|AffectedExamples' -count=1
go test ./internal/delivery/cli -run 'Validate|Review|Docs' -count=1
go test ./internal/delivery/web ./internal/delivery/mcpserver -run 'Review|Validate' -count=1
make check
```

## Risks

- **The default validate output changes:** self-resolution warnings now depend on the diff. This is
  documented in phase 5.
- **Pre-commit time grows.** Measured at about 0.05-0.25 s per staged skill at 42 skills, estimated
  at about 1.4 s median at 256 skills, and capped by the bound.
- **A served generation that is stale against HEAD** gives approximate neighbours (plan.md risk).

## Rollback

Revert the commit. Phases 1-3 remain usable as library code. Validate and review return to the
phase 1 behaviour.
