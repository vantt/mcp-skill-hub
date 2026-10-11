---
phase: 4
title: "Installer: canonical folder, Claude wrapper, migration, removal, doctor hint"
status: pending
priority: P1
effort: "8h"
wave: 2
dependencies: [1]
---

# Phase 4: Installer: canonical folder, Claude wrapper, migration, removal, doctor hint

## Context (traced on pre-5.0.0 main; re-verify every line number at start)

This phase starts after the onboarding plan is merged (plan.md "Sequencing"). That plan owns launch args, MCP
entry/config writing, permission rules (`cli-permissions.go`), option parsing and the role-based doctor/status
check set; this phase owns only the skill-file path (`curator-source.go`, the system-skill writer in
`integration.go`, its removal in `remote.go`) and the doctor additions of plan decision 10.

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
- This code repo is itself a connected project (`.agents/skills/system-curator/` exists locally) and, after
  Phase 1, tracks `.agents/skills/distill-lab/` whose bytes equal the bundle. It has no `.skillhub/` folder.

## Owned files

- Modify: `internal/hostintegration/{integration.go,types.go,files.go,remote.go,preview.go,curator-source.go}`
  (rename the last to `system-skill-files.go` if the content no longer is curator-specific),
  `internal/hostintegration/{curator-source_test.go,remote_test.go,doctor-skew_test.go,integration_test.go,scope_test.go}`,
  `internal/app/workspace.go` (`hostChangeItem` label for the new kind, the Node hint call, and the
  `git rm -r --cached` fix text for the `system_skill_tracked` finding next to the `machine_file_tracked` case at
  `internal/app/workspace.go:169`).
- Create: `internal/hostintegration/skill-wrapper.go` (+ `skill-wrapper_test.go`),
  `internal/app/distill_requirements.go` (+ `distill_requirements_test.go`), which also holds the adapter that
  hands this plan's findings and warnings to the doctor check set.
- Modify: the one place that prints "connection written: claude-code, codex-cli, gemini-cli" (kept by the
  onboarding plan; locate it at start), wording only.
- Not owned: `cli-permissions.go`, MCP config writers, launch args, `skillhub version`.
- Modify both copies (kept identical, `TestMatrixSynchronized`): `docs/mcp-compatibility-matrix.json`,
  `internal/hostintegration/matrix.json` (only the curator delivery note).

## Design

1. **Adapter fields.** Replace `SkillRelativePath`/`UserSkillRelativePath` with `SkillsDirRelativePath`
   (`.claude/skills` for Claude, `.agents/skills` for Codex and Gemini; same in user scope), `Wrapper bool`
   (Claude true, Codex and Gemini false) and `LegacySkillsDirRelativePath` (Gemini only: `.gemini/skills`, used
   solely to clean up the old full copy). Keep `NativeSkill`.
2. **New change kind `ChangeSharedSkill` ("shared-skill").** One set of canonical files per plan, not per host,
   at `<root>/.agents/skills/<id>/<file>`. Host field: the empty host, rendered by `hostChangeItem` as
   "Agent Skills folder (.agents/skills)". `changeOrder`: after `ChangeHostPermissions`, before `ChangeNativeSkill`
   (canonical before wrappers). `validateChangePath` accepts it for any path in the planned canonical set.
3. **`ChangeNativeSkill`** now means either the Claude wrapper at `.claude/skills/<id>/SKILL.md` or the removal
   of an old full copy (Claude or Gemini legacy path). `validateChangePath` accepts exactly those paths for the
   host. No change ever creates or updates a file under `.gemini/`.
4. **Which skills a plan covers** (`plannedSystemSkills(mode, scope, root, workspace)`):
   - local mode: curator always; distill-lab only if `scope == ScopeProject && root == workspace`;
   - remote or remove mode: none to install; removal candidates: curator always, distill-lab only when `root`
     contains `.skillhub/schema-version` (so this code repo's tracked distill-lab source and wrapper, which match
     the shipped bytes, are never planned).
5. **Wrapper rendering** (`skill-wrapper.go`): `renderWrapper(id, description) []byte`, Claude Code only.
   Frontmatter `name: <id>` and `description:` emitted by `yaml.v3` from the canonical frontmatter description;
   body:
   ```text
   <!-- Written by Skill Hub. Edit the skill in .agents/skills/<id>/, not here. -->

   This skill lives in the shared Agent Skills folder. Read
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/SKILL.md` now and follow it as this skill.
   Wherever it mentions this skill's folder or its scripts, use
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/`.
   ```
   The same bytes serve project scope (`<project>/.claude/skills/<id>/`) and user scope
   (`~/.claude/skills/<id>/` -> `~/.agents/skills/<id>/`). `wrapperOwned(id, raw)`: parse the description from
   `raw`, re-render, compare bytes. Export what Phase 9's repo test needs (or keep it package-internal and put
   that test in this package, as planned).
6. **Ownership per path** (`systemSkillFileState`). Unknown content is never overwritten (owner-approved change,
   plan decision 4):
   - canonical path: missing -> create; bytes == current -> current; `KnownFileDigest` -> update; else conflict
     "system skill file differs from every version Skill Hub shipped; review it, delete it, and run the command
     again".
   - Claude wrapper path: missing -> create; `wrapperOwned` with current description -> current; `wrapperOwned`
     with an old description -> update; bytes are a known full copy of this skill's SKILL.md (old layout) ->
     replace with the wrapper; else conflict (same wording).
   - Gemini legacy path `.gemini/skills/system-curator/SKILL.md` (project and user scope): missing -> nothing;
     known full copy (current or previously shipped) -> delete, prune empty parents; else conflict (same
     wording), file left.
   - verified skills-extension host (`HostSupportsSkillsExtension`, `matrix.go:62-77`): Claude's curator wrapper
     is removed when owned (today's semantics kept); the distill-lab wrapper is still installed
     (`ServedOverMCP=false`); the canonical curator stays while another host in the root still needs it.
7. **Removal** (remote and `--remove`): for the selected hosts, delete owned Claude wrappers and owned old full
   copies (Claude and Gemini paths) of the ids from design 4; delete the canonical files of an id only when no
   Skill Hub registration remains in this root after the plan (check unselected adapters with
   `scopeRegistration`, `integration.go:127`). Unowned files: left, plus the existing non-fatal behavior of
   `prepareCuratorRemoval` (returns an unchanged file, no conflict) is kept for removal, so `--remove` never fails
   on user content. `removeEmptyParents` already prunes empty folders (`files.go:222-224`).
8. **Doctor hint** (`internal/app/distill_requirements.go`): when the inspected root is the hub and distill-lab is
   installed, find `node` with `exec.LookPath` (the same name on every OS), run `node --version` with a 5 s
   timeout and parse the major version; when `node` is missing, fails to run or is older than 18, add one
   `Warning` ("distill-lab runs scripts/distill.mjs with Node 18 or newer; <not found | found vX.Y.Z>. Install
   Node from https://nodejs.org"). Never change `result.Status`. Inject the lookup and runner for tests
   (pattern: `internal/skillruntime/doctor.go:131`).
9. **Doctor adapter.** The Node warning, Phase 5's exclude and tracked findings, and the wrapper/canonical
   integrity items with the `--fix` migration from the three-full-copy layout are cited here against today's
   `Finding` type and written against the finding type that exists at implementation start: the onboarding
   plan's role-based check set (`DetectRole`; code, message, optional fix). One function in
   `distill_requirements.go` returns them for a hub root; the check set calls only that function, so a later
   rename of the check-set types touches only the adapter.
10. **Connection summary wording.** Replace "connection written: claude-code, codex-cli, gemini-cli" in its one
    place with text that names the layout, for example "connection written: claude-code, codex-cli, gemini-cli;
    skills in .agents/skills (Codex and Gemini CLI read it directly; Claude Code gets a pointer)". Keep host ids
    unchanged; keep `skillhub version` untouched.

## Tests (names describe behavior)

- Fresh hub plan writes canonical files for both ids and Claude wrappers for both; no Codex or Gemini wrapper; no
  path under `.gemini/`.
- Project-scope plan with `Root != Workspace` and user-scope plan: curator canonical + Claude wrapper only; no path
  contains `distill-lab`; the user-scope wrapper bytes equal the project-scope ones.
- Remote and remove plans never contain a create or update for any system skill.
- Remove plan in a root without `.skillhub/schema-version` that holds owned distill-lab files (repo-shaped
  folder): no change touches `distill-lab`.
- Old layout (three full current curator copies) -> one plan updates `.claude` to the wrapper, deletes the
  `.gemini` copy and its empty folders, keeps `.agents` current; second plan is empty.
- Old layout with a previously shipped curator version (from known digests) -> migrated, no conflict.
- Edited full copy at `.claude/skills/system-curator/SKILL.md` or at `.gemini/skills/system-curator/SKILL.md` ->
  `ErrConflict` naming that path; file unchanged.
- Edited canonical file -> conflict. `doctor-skew_test.go:62-75` rewritten for the decided behaviour change:
  stale *unknown* content now reports a conflict and stays unchanged instead of being restored; a stale *known*
  version is restored by `DoctorFix` without a prompt-only conflict.
- Wrapper with an older description -> updated, no conflict; wrapper with extra user text -> conflict.
- `--remove` with only `--host claude` while Codex stays registered: Claude wrappers removed, canonical kept;
  removing all hosts removes canonical folders and empty parents.
- Windows path shape: wrapper relative path and `validateChangePath` work with `filepath` separators (table test
  with `filepath.FromSlash`); files written `0644`.
- Apply rejects a forged plan whose shared-skill change points outside `.agents/skills/<id>/` or whose native-skill
  change writes under `.gemini/`.
- Doctor hint: `node` missing -> one warning, status unchanged; `node --version` prints `v16.20.2` -> warning
  naming the version; `v18.0.0` and `v24.21.0` -> no warning; unparsable output or a failing run -> warning;
  non-hub root -> no check.
- Doctor adapter: in a hub, the role-based check set receives the Node warning and Phase 5's findings through the
  adapter only; a non-hub role gets none.
- Connection summary: the new wording appears once; `skillhub version` output is byte-unchanged.
- `TestMatrixSynchronized` after the note update.

## Steps

1. Run the plan's "Re-verify at start" items for this phase and update the citations above. Then adapter fields
   and `plannedSystemSkills`; port existing curator tests to the new paths.
2. Shared kind: prepare, buildPlan ordering, expectedChange, validateChangePath, preview text.
3. Wrapper rendering and ownership; migration cases including the Gemini legacy copy.
4. Removal rules and the hub-marker gate; remote tests.
5. Doctor hint, the adapter, `hostChangeItem` label and the connection summary wording.
6. Matrix note: "Native delivery writes the skill once under `.agents/skills/<id>/`, which Codex and Gemini CLI
   read directly; Claude Code gets a generated wrapper SKILL.md that points to it."
7. `go test ./internal/hostintegration/... ./internal/app/... ./internal/delivery/...`, then `make check`.
8. Commits: `feat(hostintegration): install system skills once under .agents/skills with a Claude Code wrapper`,
   `feat(doctor): hint when distill-lab cannot find Node 18 or newer`.

## Validation

- All tests above pass; `make check` passes; `git grep -n "system-curator/SKILL.md" internal/hostintegration`
  shows only test fixtures for the old layout; `git grep -n "\.gemini/skills" internal/hostintegration` shows only
  the legacy cleanup path and its tests.

## Risks

- Plan/Apply contract drift (plans are re-derived at apply time): every new path type goes through
  `expectedChange` and `validateChangePath`; the forged-plan test guards it.
- `doctor` from a project or home now inspects fewer files (no distill-lab): intended.
- An edited old Gemini copy now blocks `doctor --fix` until the owner deletes it: intended (decision 8); the
  conflict names the path.
- Claude user-scope permission prompt for `~/.agents`: if Phase 8 sees one, the canonical folder must join Claude
  user `additionalDirectories`. That code (`cli-permissions.go`) belongs to the onboarding plan's area; the lead
  raises it with the owner before this phase edits it.
- The onboarding plan's check-set names differ from what this plan cites: absorbed by the adapter (design 9).

## Rollback

Revert the commits. Hubs already migrated are restored to full copies by the old binary's confirmed fix (see
plan.md, Compatibility and rollback).
