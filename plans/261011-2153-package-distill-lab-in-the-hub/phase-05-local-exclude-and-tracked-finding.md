---
phase: 5
title: "Exclude generated system-skill folders locally and report tracked ones"
status: pending
priority: P2
effort: "2h"
wave: 1
dependencies: []
---

# Phase 5: Exclude generated system-skill folders locally and report tracked ones

## Context (traced; re-verify on the post-5.0.0 main)

- Today (pre-5.0.0) `requiredIgnoreEntries` (`internal/workspace/workspace.go:38-41`) are appended to the hub
  `.gitignore` by init (`:433-440`); `machineFiles` (`:36`) feed `trackedMachineFiles` (`:185-201`,
  `git ls-files -z -- <paths>`), reported as the non-fixable `machine_file_tracked` finding with a
  `git rm --cached` hint (`:177-179`). Tests: `internal/workspace/machine_files_test.go`.
- The onboarding plan (merged first, see plan.md "Sequencing") excludes the hub's own connection files through
  `.git/info/exclude` with a helper in `internal/workspace`, `ExcludeLocally(root string, patterns ...string) error`
  (idempotent; name may change), called by `init` before its first commit. It keeps `machine_file_tracked` and its
  tests. It moves doctor/status to a role-based check set (`DetectRole`).
- After Phase 4 Skill Hub writes system skills only under `.agents/skills/<id>/` and `.claude/skills/<id>/`;
  nothing goes to `.gemini/skills/` (plan decisions 1 and 7).

## Owned files

- Modify: `internal/workspace/workspace.go` (the new finding and the pattern list only).
- Create: `internal/workspace/system_skill_files_test.go`.
- Modify: the hub `init` and `doctor --fix` call sites of `ExcludeLocally` (file named at re-verify time), only to
  pass the four patterns.
- Not owned: `ExcludeLocally` itself, `machine_file_tracked`, `machine_files_test.go`, the hub `.gitignore` code.

## Requirements

1. Four patterns, anchored: `/.agents/skills/system-curator/`, `/.claude/skills/system-curator/`,
   `/.agents/skills/distill-lab/`, `/.claude/skills/distill-lab/`. No `.gemini/skills/` pattern. Write the ids as
   literals in `internal/workspace` (it must not import `systemskills`, which keeps the dependency direction)
   with a comment that they mirror the bundled system skill ids; a test asserts the exact list. If the onboarding
   plan's self-connection patterns already cover one of them, do not add it twice.
2. `init` passes the four patterns to `ExcludeLocally` for the hub before the first commit, so the generated files
   are neither committed nor shown as untracked. `doctor --fix` calls it too, so older hubs and fresh clones catch
   up (`info/exclude` is not shared by clones).
3. Doctor reports a fixable finding when any of the four paths is not ignored (`git check-ignore -q`), fixed by
   the same `ExcludeLocally` call.
4. New finding `system_skill_tracked` (not fixable): "Git tracks a system skill file that Skill Hub regenerates
   from its version." Use the existing `git ls-files` helper with the four folder paths; make the helper take the
   path list instead of duplicating it. Phase 4 adds the `git rm -r --cached <path>` fix text next to the
   `machine_file_tracked` case.
5. Both findings are built against the `Finding` type that exists at implementation start and go through the
   adapter of plan decision 10, so re-homing onto the role-based check set touches only that adapter.

## Tests

- New hub: `.git/info/exclude` contains the four patterns once; `init` twice adds nothing; `.gitignore` is not
  changed by this phase; `git status --porcelain --untracked-files=all` lists none of the four folders; the first
  commit contains none of their files.
- Old hub without the patterns: doctor reports the fixable finding; fix adds them; second doctor is clean.
- A hub with `git add -f .claude/skills/system-curator/SKILL.md` committed: doctor reports
  `system_skill_tracked` for that path; a user skill under `.claude/skills/my-skill/` is not reported.
- `git check-ignore` confirms `.agents/skills/distill-lab/scripts/distill.mjs` is ignored and
  `.claude/skills/my-skill/SKILL.md` is not.
- Windows: patterns use forward slashes in `info/exclude` (CI runs the tests on Windows).

## Steps

1. Pattern list, call sites, finding, helper parameter; tests; `go test ./internal/workspace/... ./internal/app/...`.
2. Commit: `feat(workspace): keep generated system skill files out of Git through the local exclude file`.

## Risks

- A hub owner deliberately committed customized system skills: they now see a tracked finding; the files stay
  tracked until they act. The finding text says why.
- A clone on another machine has no `info/exclude` entries until `init` or `doctor --fix` runs there: the
  generated files only appear after those commands, which add the patterns in the same step.

## Rollback

Revert the commit. Patterns already written to `info/exclude` are local and harmless.
