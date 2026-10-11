---
phase: 4
title: "Hub Git ignore entries and tracked system-skill finding"
status: pending
priority: P2
effort: "2h"
wave: 1
dependencies: []
---

# Phase 4: Hub Git ignore entries and tracked system-skill finding

## Context (traced)

- `requiredIgnoreEntries` (`internal/workspace/workspace.go:38-41`) are appended to the hub `.gitignore` by init
  (`:433-440`) and reported as fixable `gitignore_entry_missing` by doctor when absent (`:172-176`).
- `machineFiles` (`:36`) feed `trackedMachineFiles` (`:185-201`, `git ls-files -z -- <paths>`), reported as the
  non-fixable `machine_file_tracked` finding with a `git rm --cached` hint (`:177-179`). Tests:
  `internal/workspace/machine_files_test.go`.
- The suggested stage command lists canonical folders only (`internal/delivery/cli/workspace.go:71-73`), so it
  never stages `.agents`, `.claude` or `.gemini`.

## Owned files

- Modify: `internal/workspace/workspace.go`, `internal/workspace/machine_files_test.go`.

## Requirements

1. Add six ignore entries, anchored like the existing ones:
   `/.agents/skills/system-curator/`, `/.claude/skills/system-curator/`, `/.gemini/skills/system-curator/`,
   `/.agents/skills/distill-lab/`, `/.claude/skills/distill-lab/`, `/.gemini/skills/distill-lab/`.
   Write the ids as literals in this package (it must not import `systemskills`, which keeps the dependency
   direction) with a comment that they mirror the bundled system skill ids; a test asserts the exact list.
2. New finding `system_skill_tracked` (not fixable): "Git tracks a system skill file that Skill Hub regenerates
   from its version." Use the same `git ls-files` helper with the six folder paths; make the helper take the path
   list instead of duplicating it.
3. The fix text shown for `machine_file_tracked` is built in `internal/app/workspace.go:169`, which Phase 3 owns.
   Phase 3 adds the matching `git rm -r --cached <path>` text for `system_skill_tracked`; this phase only emits
   the finding.

## Tests

- New hub: `.gitignore` contains the six entries once; running init twice adds nothing.
- Old hub without the entries: doctor reports six `gitignore_entry_missing` findings; fix appends them.
- A hub with `git add -f .claude/skills/system-curator/SKILL.md` committed: doctor reports
  `system_skill_tracked` for that path; a user skill under `.claude/skills/my-skill/` is not reported.
- `git check-ignore` confirms `.agents/skills/distill-lab/scripts/distill.py` is ignored and
  `.claude/skills/my-skill/SKILL.md` is not.

## Steps

1. Edit lists and helper; add tests; `go test ./internal/workspace/...`.
2. Commit: `feat(workspace): keep generated system skill files out of Git`.

## Risks

- A hub owner deliberately committed customized system skills: they now see a tracked finding; the files stay
  tracked until they act. The finding text says why.

## Rollback

Revert the commit. Ignore lines already written to hubs are harmless.
