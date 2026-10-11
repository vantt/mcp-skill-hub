---
phase: 3
title: "Installer: canonical folder, wrappers, migration, removal, doctor hint"
status: pending
priority: P1
effort: "8h"
wave: 2
dependencies: [1]
---

# Phase 3: Installer: canonical folder, wrappers, migration, removal, doctor hint

## Context (traced)

- Adapters hard-code one skill file per host: Claude `.claude/skills/system-curator/SKILL.md`, Codex
  `.agents/skills/system-curator/SKILL.md`, Gemini `.gemini/skills/system-curator/SKILL.md`, same relative paths
  in user scope (`internal/hostintegration/integration.go:34-36`, `types.go:69-97`). All `SkillRelativePath`
  users are inside this package (`types.go:95,97`; tests `curator-source_test.go`, `remote_test.go`, `doctor-skew_test.go`).
- `prepare()` adds one native-skill file per host: local mode -> `prepareNativeCurator` (`integration.go:143-156`),
  otherwise `prepareCuratorRemoval` (`remote.go:483-498`).
- Local unverified host: `prepareExactFile` overwrites to bundled bytes (`curator-source.go:7-9`,
  `integration.go:390-396`). Verified skills-extension host: owned copy removed, edited copy conflicts
  (`curator-source.go:10-24`). No host is verified today (`docs/mcp-compatibility-matrix.json` skills_extension
  all `unverified`).
- Apply re-derives desired bytes per path (`files.go:236-252`) and checks each path against the host contract
  (`files.go:310-339`, one `skillRel` per host). Writes go through `atomicWrite`, which creates safe parent
  directories (`files.go:413`); symlink components are refused (`files.go:19-35,51`).
- Any conflicting file aborts the plan (`integration.go:91-93`).
- Doctor item rendering: `hostChangeItem` (`internal/app/workspace.go:501`); warnings: `result.Warnings`
  (`internal/app/workspace.go:453`), printed by the CLI (`internal/delivery/cli/workspace.go:116,351`).
- Hub vs project: init/doctor use `Request{Workspace: root}` (`internal/app/workspace.go:465`); project and user
  connections set `Root` to the project or home (`internal/app/host-connections.go:23,26`); remote and removal
  plans carry no workspace (`internal/app/connect_remote.go:159,441`).

## Owned files

- Modify: `internal/hostintegration/{integration.go,types.go,files.go,remote.go,preview.go,curator-source.go}`
  (rename the last to `system-skill-files.go` if the content no longer is curator-specific),
  `internal/hostintegration/{curator-source_test.go,remote_test.go,doctor-skew_test.go,integration_test.go,scope_test.go}`,
  `internal/app/workspace.go` (`hostChangeItem` label for the new kind, the python hint call, and the
  `git rm -r --cached` fix text for the `system_skill_tracked` finding next to the `machine_file_tracked` case at
  `internal/app/workspace.go:169`).
- Create: `internal/hostintegration/skill-wrapper.go` (+ `skill-wrapper_test.go`),
  `internal/app/distill_requirements.go` (+ `distill_requirements_test.go`).
- Modify both copies (kept identical, `TestMatrixSynchronized`): `docs/mcp-compatibility-matrix.json`,
  `internal/hostintegration/matrix.json` (only the curator delivery note).

## Design

1. **Adapter fields.** Replace `SkillRelativePath`/`UserSkillRelativePath` with `SkillsDirRelativePath`
   (`.claude/skills`, `.agents/skills`, `.gemini/skills`; same for user scope) and `Wrapper bool` (Claude and Gemini
   true, Codex false). Keep `NativeSkill`.
2. **New change kind `ChangeSharedSkill` ("shared-skill").** One set of canonical files per plan, not per host,
   at `<root>/.agents/skills/<id>/<file>`. Host field: the empty host, rendered by `hostChangeItem` as
   "Agent Skills folder (.agents/skills)". `changeOrder`: after `ChangeHostPermissions`, before `ChangeNativeSkill`
   (canonical before wrappers). `validateChangePath` accepts it for any path in the planned canonical set.
3. **`ChangeNativeSkill`** now means a wrapper at `<host skills dir>/<id>/SKILL.md`. `validateChangePath` accepts
   the wrapper path of any system skill id for that host.
4. **Which skills a plan covers** (`plannedSystemSkills(mode, scope, root, workspace)`):
   - local mode: curator always; distill-lab only if `scope == ScopeProject && root == workspace`;
   - remote or remove mode: none to install; removal candidates for both ids.
5. **Wrapper rendering** (`skill-wrapper.go`): `renderWrapper(host, id, description) []byte`. Frontmatter
   `name: <id>` and `description:` emitted by `yaml.v3` from the canonical frontmatter description; body for
   Claude:
   ```text
   <!-- Written by Skill Hub. Edit the skill in .agents/skills/<id>/, not here. -->

   This skill lives in the shared Agent Skills folder. Read
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/SKILL.md` now and follow it as this skill.
   Wherever it mentions this skill's folder or its scripts, use
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/`.
   ```
   Gemini: the same text with "`.agents/skills/<id>/SKILL.md` in the folder that contains this `.gemini` folder"
   instead of the variable. `wrapperOwned(host, id, raw)`: parse the description from `raw`, re-render, compare
   bytes.
6. **Ownership per path** (`systemSkillFileState`):
   - canonical path: missing -> create; bytes == current -> current; `KnownFileDigest` -> update; else conflict
     "system skill file differs from every version Skill Hub shipped; review it, delete it, and run the command
     again".
   - wrapper path: missing -> create; `wrapperOwned` with current description -> current; `wrapperOwned` with an
     old description -> update; bytes are a known full copy of this skill's SKILL.md (old layout) -> replace with
     the wrapper; else conflict (same wording).
   - verified skills-extension host (`HostSupportsSkillsExtension`, `matrix.go:62-77`): curator wrapper is
     removed when owned (today's semantics kept); distill-lab wrapper is still installed (`ServedOverMCP=false`).
7. **Removal** (remote and `--remove`): for the selected hosts, delete owned wrappers and owned old full copies of
   both ids; delete the canonical files of an id only when no Skill Hub registration remains in this root after
   the plan (check unselected adapters with `scopeRegistration`, `integration.go:127`). Unowned files: left, plus
   the existing non-fatal behavior of `prepareCuratorRemoval` (returns an unchanged file, no conflict) is kept for
   removal, so `--remove` never fails on user content. `removeEmptyParents` already prunes empty folders
   (`files.go:222-224`).
8. **Doctor hint** (`internal/app/distill_requirements.go`): when the inspected root is the hub and distill-lab is
   installed, find an interpreter (`python3`, then `py -3` on Windows, then `python`) with `exec.LookPath`, run
   `<py> -c "import yaml"` with a 5 s timeout; on failure add one `Warning` ("distill-lab runs
   scripts/distill.py with Python 3 and PyYAML; not found: <what>. Install: python3 -m pip install --user pyyaml").
   Never change `result.Status`. Inject the lookup and runner for tests (pattern: `internal/skillruntime/doctor.go:131`).

## Tests (names describe behavior)

- Fresh hub plan writes canonical files for both ids, Claude and Gemini wrappers for both, no Codex wrapper.
- Project-scope plan with `Root != Workspace` and user-scope plan: curator only; no path contains `distill-lab`.
- Remote and remove plans never contain a create or update for any system skill.
- Old layout (three full current curator copies) -> one plan updates `.claude` and `.gemini` to wrappers, keeps
  `.agents` current; second plan is empty.
- Old layout with a previously shipped curator version (from known digests) -> migrated, no conflict.
- Edited full copy at `.claude/skills/system-curator/SKILL.md` -> `ErrConflict` naming that path; file unchanged.
- Edited canonical file -> conflict; `doctor-skew_test.go` updated: stale *unknown* content now reports a
  conflict instead of being restored; a stale *known* version is restored by `DoctorFix`.
- Wrapper with an older description -> updated, no conflict; wrapper with extra user text -> conflict.
- `--remove` with only `--host claude` while Codex stays registered: Claude wrappers removed, canonical kept;
  removing all hosts removes canonical folders and empty parents.
- Windows path shape: wrapper relative path and `validateChangePath` work with `filepath` separators (table test
  with `filepath.FromSlash`); files written `0644`.
- Apply rejects a forged plan whose shared-skill change points outside `.agents/skills/<id>/`.
- Doctor hint: interpreter missing -> one warning, status unchanged; yaml import fails -> warning; all present
  -> no warning; non-hub root -> no check.
- `TestMatrixSynchronized` after the note update.

## Steps

1. Adapter fields and `plannedSystemSkills`; port existing curator tests to the new paths.
2. Shared kind: prepare, buildPlan ordering, expectedChange, validateChangePath, preview text.
3. Wrapper rendering and ownership; migration cases.
4. Removal rules; remote tests.
5. Doctor hint and `hostChangeItem` label.
6. Matrix note: "Native delivery writes the skill once under `.agents/skills/<id>/`; Claude Code and Gemini CLI get
   a generated wrapper SKILL.md that points to it."
7. `go test ./internal/hostintegration/... ./internal/app/... ./internal/delivery/...`, then `make check`.
8. Commits: `feat(hostintegration): install system skills once under .agents/skills with host wrappers`,
   `feat(doctor): hint when distill-lab cannot find Python 3 or PyYAML`.

## Validation

- All tests above pass; `make check` passes; `git grep -n "system-curator/SKILL.md" internal/hostintegration`
  shows only test fixtures for the old layout.

## Risks

- Plan/Apply contract drift (plans are re-derived at apply time): every new path type goes through
  `expectedChange` and `validateChangePath`; the forged-plan test guards it.
- `doctor` from a project or home now inspects fewer files (no distill-lab): intended.
- Claude user-scope permission prompt for `~/.agents`: if Phase 7 sees one, add the canonical folder to Claude
  user `additionalDirectories` here.

## Rollback

Revert the commits. Hubs already migrated are restored to full copies by the old binary's confirmed fix (see
plan.md, Compatibility and rollback).
