---
phase: 5
title: "Ignore generated system-skill folders in the hub .gitignore and report tracked ones"
status: pending
priority: P2
effort: "2.5h"
wave: 3
dependencies: [4]
---

# Phase 5: Ignore generated system-skill folders in the hub .gitignore and report tracked ones

<!-- Red team: contract change from the onboarding plan's owner (mcp-skill-hub-cb). The local-exclude helper and
`.git/info/exclude` are dropped; the hub keeps the `.gitignore` mechanism already on main. This phase therefore
extends `requiredIgnoreEntries`. File renamed from phase-05-local-exclude-and-tracked-finding.md. -->
<!-- Red team: Phase 5 ordering and ownership. Moved from wave 1 to wave 3 after Phase 4, because its notices go
through the adapter Phase 4 creates (`internal/app/system_skill_doctor.go`). -->
<!-- Validation: the adapter is now `systemSkillProvider`, a `CheckProvider` registered through the onboarding
plan's `RegisterProvider`; its checks return `notice` Findings. Release is 0.5.0 (was written 5.0.0). -->

## Context (traced on main `ef535de`; re-verify on the post-0.5.0 main)

- `requiredIgnoreEntries` (`internal/workspace/workspace.go:38-41`) are the hub `.gitignore` lines Skill Hub
  requires. `Inspect` reports a fixable `gitignore_entry_missing` finding for each missing line
  (`:172-176`), and the workspace mutation pipeline repairs it by appending the line (`desiredGitignore`,
  `:425-440`); `init` appends them for a new hub. `machineFiles` (`:36`) feed `trackedMachineFiles`
  (`:185-201`, `git ls-files -z -- <paths>`), reported as the non-fixable `machine_file_tracked` finding
  (`:177-179`). Tests: `internal/workspace/machine_files_test.go`. All of this came with commit `b734bc9`.
- The onboarding plan keeps this mechanism (contract from its owner, 2026-10-11): no local-exclude helper, no
  `.git/info/exclude`; `init` appends missing required entries; `gitignore_entry_missing` stays fixable by
  `doctor --fix`; `machine_file_tracked` stays theirs.
- A non-fixable workspace finding makes `doctor --fix` return before any host repair
  (`internal/app/workspace.go:266-270`, again under the lock `:289-300`). <!-- Red team: system_skill_tracked blocks doctor --fix -->
  So a new tracked-file report must not be a non-fixable `workspace.Finding`.
- <!-- Red team: check-ignore never reports tracked paths --> `git check-ignore` does not report a tracked path as
  ignored, so it cannot tell whether a tracked system-skill file exists. Tracked files are detected with
  `git ls-files`; missing ignore lines are detected by the existing `.gitignore` content check.
- After Phase 4 Skill Hub writes system skills only under `.agents/skills/<id>/` and `.claude/skills/<id>/`;
  nothing goes to `.gemini/skills/` (plan decisions 1 and 7).

## Owned files

- Modify: `internal/workspace/workspace.go` (`requiredIgnoreEntries` only).
- Modify: `internal/app/system_skill_doctor.go` and its test (created by Phase 4): the tracked-file and
  not-Skill-Hub-file checks of `systemSkillProvider`. <!-- Validation: onboarding extension point -->
- Modify: `internal/workspace/machine_files_test.go` only if it asserts the exact `requiredIgnoreEntries` list
  (grep at start; otherwise create `internal/workspace/system_skill_ignore_test.go`).
- Not owned: `machine_file_tracked`, `trackedMachineFiles`, `init`, the onboarding plan's doctor/status check set.

## Requirements

1. Append four anchored entries to `requiredIgnoreEntries`: `/.agents/skills/system-curator/`,
   `/.claude/skills/system-curator/`, `/.agents/skills/distill-lab/`, `/.claude/skills/distill-lab/`. No
   `.gemini/skills/` entry. Write the ids as literals in `internal/workspace` (it must not import `systemskills`,
   which keeps the dependency direction) with a comment that they mirror the bundled system skill ids; a test
   asserts the exact list. Because `.gitignore` is committed, the entries also hold for every clone of the hub.
2. Nothing new is needed for `init` or `doctor --fix`: a new hub gets the entries from `init`, an older hub gets a
   fixable `gitignore_entry_missing` finding per new entry until `doctor --fix` appends them (the fix shows as a
   `.gitignore` change the owner commits). Phase 7 adds this to the release-note lines.
3. <!-- Red team: system_skill_tracked blocks doctor --fix; tracked fix untracks user files --> Tracked notice
   `system_skill_tracked`, non-blocking: in `systemSkillProvider` (not a `workspace.Finding`), list with
   `git ls-files -z --` the tracked files under the four folders. For each tracked file whose bytes Skill Hub owns
   (current or known bundle bytes, or an owned wrapper; ownership from Phase 4), return one
   <!-- Validation: onboarding extension point -->`Finding{Code: "system_skill_tracked", Severity: notice, Message:
   "Git tracks <path>, which Skill Hub regenerates from its version.", Fix: nil, Manual: "git rm --cached -- <path>"
   (then commit)}`. The command is printed, never run, names exactly that file (never `-r`, never a folder), and a
   notice never changes the doctor status or blocks `--fix`. A tracked file Skill Hub does not own is the user's: it is
   reported by Phase 4 as a blocked item if it sits at a Skill Hub path, otherwise left alone, and no untracking
   command is ever suggested for it.
4. <!-- Red team: exclude hides user-owned files --> User files are never hidden silently: the folder entries make
   Git ignore any *untracked* file in those four folders, so the provider returns one `notice` Finding (code
   `system_skill_unowned_ignored`, `Fix` nil) per untracked file there that Skill Hub does not own ("<path> is
   ignored by .gitignore and was not written by Skill Hub"; `Manual`: "move it to another skill folder or run
   `git add -f -- <path>`"). A hand-copied `scripts/distill.py` next to a migrated wrapper is the typical case.
5. <!-- Validation: onboarding extension point --> Both checks run inside `systemSkillProvider`, registered through
   the onboarding plan's `RegisterProvider` (`internal/app/health.go`, their phase-04:33-63; plan decision 10,
   Phase 4 design 16), for the hub role at full depth only. Neither touches the renderers or `health.go`.

## Tests

- New hub: `.gitignore` contains the four entries once; `init` twice adds nothing; `git status --porcelain
  --untracked-files=all` lists none of the four folders; init's first commit contains none of their files.
- Old hub without the entries: doctor reports `gitignore_entry_missing` for each; `doctor --fix` appends them;
  second doctor reports none.
- A hub with `git add -f .claude/skills/system-curator/SKILL.md` committed (the 0.5.0 init case): doctor shows one
  `system_skill_tracked` notice (severity `notice`, `Manual` holding the command) with `git rm --cached -- .claude/skills/system-curator/SKILL.md`; status is
  unchanged; `doctor --fix` still runs the host repairs (wrapper migration, distill-lab install).
- A tracked file under `.claude/skills/distill-lab/` that Skill Hub does not own: no untracking command anywhere
  in the output.
- An untracked `.claude/skills/distill-lab/scripts/distill.py`: one `system_skill_unowned_ignored` notice naming it;
  a user skill under `.claude/skills/my-skill/` gets no warning and is not ignored.
- Windows: entries use forward slashes (CI runs the tests on Windows).

## Steps

1. Entries and their test; the two checks in `systemSkillProvider` with tests; `go test ./internal/workspace/... ./internal/app/...`.
2. Commit: `feat(workspace): keep generated system skill files out of Git through the hub .gitignore`.

## Risks

- A hub owner deliberately committed customized system skills: they now see a tracked warning or a blocked item;
  the files stay tracked until they act. The text says why.
- A clone on another machine has the ignore entries (committed) but no generated files until `init` or
  `doctor --fix` runs there: intended; the generated files depend on that machine's binary.
- Existing hubs see `gitignore_entry_missing` after the upgrade until `doctor --fix`: documented in the release note.

## Rollback

Revert the commit. Lines already appended to a hub `.gitignore` stay there and are harmless; the owner may delete
them.
