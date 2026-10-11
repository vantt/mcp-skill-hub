---
phase: 4
title: "Installer: canonical folder, Claude wrapper, migration, removal, doctor hint"
status: pending
priority: P1
effort: "16h"
wave: 2
dependencies: [1]
---

# Phase 4: Installer: canonical folder, Claude wrapper, migration, removal, doctor hint

<!-- Red team: effort raised from 8h to 16h. The host-less shared kind touches Inspection, PlanResult, every
conflict pre-check in internal/app, the CLI change-line renderer, five test files and two goldens outside the
package, per-item conflicts, wrapper history, removal order and symlink handling. -->

## Context (traced on main `ef535de`, pre-5.0.0; re-verify every line number at start)

This phase starts after the onboarding plan is merged (plan.md "Sequencing"). That plan owns launch args, MCP
entry/config writing, permission rules (`cli-permissions.go`), option parsing and the role-based doctor/status
check set; this phase owns the skill-file path (`curator-source.go`, the system-skill writer in
`integration.go`, its removal in `remote.go`), every consumer of the inspection and plan shapes it changes (listed
under "Owned files"), and the doctor additions of plan decision 10.

- Adapters hard-code one skill file per host: Claude `.claude/skills/system-curator/SKILL.md`, Codex
  `.agents/skills/system-curator/SKILL.md`, Gemini `.gemini/skills/system-curator/SKILL.md`, same relative paths
  in user scope (`internal/hostintegration/integration.go:34-36`, `types.go:69-97`).
  <!-- Red team: tests outside Phase 4 -->The `SkillRelativePath` symbol is used only inside this package, but the
  three-copy layout is also pinned by tests and goldens outside it: `internal/app/workspace_test.go:337-346`
  (`hostArtifactPaths`, used at `:67,86`), `internal/delivery/cli/onboarding_test.go:293`,
  `internal/delivery/cli/connect_regression_test.go:96-100` with
  `internal/delivery/cli/testdata/connect-workspace/{project-files,global-files}.golden`, and
  `internal/delivery/cli/connect_remote_test.go:630` (exact item ID `host_claude-code_native-skill`).
- `prepare()` adds one native-skill file per host: local mode -> `prepareNativeCurator` (`integration.go:143-156`),
  otherwise `prepareCuratorRemoval` (`remote.go:483-498`). A read error from a skill file (for example a symlink
  component) is returned from `prepare` and fails the whole plan (`integration.go:150-152`).
- Local unverified host: `prepareExactFile` overwrites to bundled bytes (`curator-source.go:7-9`,
  `integration.go:390-396`). Verified skills-extension host: owned copy removed, edited copy conflicts
  (`curator-source.go:10-24`). No host is verified today (`docs/mcp-compatibility-matrix.json` skills_extension
  all `unverified`).
- <!-- Red team: ChangeSharedSkill does not fit the contract -->Inspection and plan are grouped by host:
  `HostInspection{Host, Level, Files}` and `Inspection{Workspace, Root, Binary, Hosts}`
  (`internal/hostintegration/types.go:141-154`); `buildPlan` loops `kinds x prepared.hosts` with a hard-coded kind
  list and returns `*ConflictError` on the first conflicting file of any kind (`integration.go:86-104`).
- Consumers that assume "every file belongs to a host" (all must change with this phase):
  - kind lists: `integration.go:87`, `internal/app/workspace.go:485` (`appendHostInspectionItems`);
  - conflict pre-checks over `inspection.Hosts[].Files`: `internal/app/connect.go:53-67`,
    `internal/app/connect_remote.go:196-211`, `internal/app/workspace.go:377-392`;
  - `levels[change.Host]` lookups: `internal/app/connect.go:79,93`, `internal/app/connect_remote.go:226`,
    `internal/app/workspace.go:403,450`;
  - item ID `"host_"+host+"_"+kind` (`internal/app/workspace.go:502`), parsed by `hostChangeLines` with
    `strings.Split(item.ID, "_")` (`internal/delivery/cli/connect_remote.go:118-146`) and named by
    `hostDisplayName` (`internal/delivery/cli/connect.go:161-172`);
  - `validateChangePath` looks the adapter up before it reads the kind, so an empty host fails with "unsupported
    host" (`files.go:304-339`);
  - `MissingFinding` treats any change with a missing preimage as "host integration missing" (`types.go:241-254`);
    status calls it through `CheckMissing` and drops its error (`internal/app/curation_home.go:151-157`); doctor
    calls it at `internal/app/workspace.go:397`.
- Apply re-derives desired bytes per path (`files.go:236-252`), writes changes one at a time in plan order and
  returns on the first error with no undo (`files.go:194-226`); writes go through `atomicWrite`, which creates safe
  parent directories (`files.go:413`); symlink components are refused (`files.go:19-35,51`).
- Doctor: a non-fixable workspace finding returns before any host repair (`internal/app/workspace.go:266-270`,
  again under the lock `:289-300`). `applyExternalConnections` returns `Result{}, err` on the first plan error, which
  discards the hub changes already applied (`internal/app/host-connections.go:66-73`); `checkProjectAndGlobalConnections`
  turns any plan error into "outdated or needs repair" and suggests `connect` again (`:91-100`).
- Hub vs project: init/doctor use `Request{Workspace: root}` (`internal/app/workspace.go:465`); project and user
  connections set `Root` to the project or home (`internal/app/host-connections.go:23,26`); remote and removal
  plans carry no workspace (`internal/app/connect_remote.go:159,441`). <!-- Red team: connect --hub canonical -->
  `scopeRegistration` returns any present registration, local or remote (`internal/hostintegration/remote.go:156-164`);
  `Registration.Remote()` tells them apart (`internal/hostintegration/connection.go:27`).
- MCP hides `system-curator` from every session identified as Claude, Codex or Gemini whose host is not verified
  for the skills extension, by client name only (`internal/delivery/mcpserver/server.go:365-366,399-411`).
- This code repo is itself a connected project (`.agents/skills/system-curator/` exists locally) and, after
  Phase 1, tracks `.agents/skills/distill-lab/` whose bytes equal the bundle. It has no `.skillhub/` folder.

## Owned files

<!-- Red team: list and own every consumer, test file and golden -->
- Modify (hostintegration): `internal/hostintegration/{integration.go,types.go,files.go,remote.go,preview.go,curator-source.go}`
  (rename the last to `system-skill-files.go` if the content no longer is curator-specific), and their tests
  `{curator-source_test.go,remote_test.go,doctor-skew_test.go,integration_test.go,scope_test.go}`.
- Create: `internal/hostintegration/skill-wrapper.go` (+ `skill-wrapper_test.go`),
  `internal/hostintegration/known-wrappers.go` (shipped wrapper digests),
  `internal/hostintegration/testdata/wrappers/{system-curator,distill-lab}.golden` (the current wrapper bytes),
  `internal/app/system_skill_doctor.go` (+ `system_skill_doctor_test.go`): the doctor adapter of plan decision 10
  (Node warning, Gemini notice; Phase 5 adds its notices there).
- Modify (app): `internal/app/connect.go` (conflict pre-check, `levels`), `internal/app/connect_remote.go`
  (conflict pre-check, `levels`), `internal/app/workspace.go` (`appendHostInspectionItems`, `hostChangeItem` ID and
  label, the doctor conflict branch, `MissingFinding` use), `internal/app/host-connections.go`
  (`applyExternalConnections`, `checkProjectAndGlobalConnections`), `internal/app/curation_home.go:150-157` only
  (status host-integration finding; Phase 6 owns the `review_lessons` block of the same file and runs after this
  phase).
- Modify (CLI): `internal/delivery/cli/connect_remote.go` (`hostChangeLines`), `internal/delivery/cli/connect.go`
  (`hostDisplayName`).
- Modify (tests and goldens outside the package, expected content: canonical files plus Claude wrapper, nothing
  under `.gemini/`): `internal/app/workspace_test.go` (`hostArtifactPaths`), `internal/app/connect_test.go` and
  `internal/app/connect_remote_test.go` if they pin the layout (grep at start),
  `internal/delivery/cli/onboarding_test.go`, `internal/delivery/cli/connect_regression_test.go`,
  `internal/delivery/cli/testdata/connect-workspace/{project-files,global-files}.golden`,
  `internal/delivery/cli/connect_remote_test.go`.
- Modify both copies (kept identical, `TestMatrixSynchronized`): `docs/mcp-compatibility-matrix.json`,
  `internal/hostintegration/matrix.json` (only the curator delivery note).
- Not owned: `cli-permissions.go`, MCP config writers, launch args, the onboarding plan's per-agent connection
  lines and `internal/delivery/cli/workspace.go:57` ("Agent connection for this workspace written: %s."),
  `skillhub version`.

## Design

1. **Adapter fields.** Replace `SkillRelativePath`/`UserSkillRelativePath` with `SkillsDirRelativePath`
   (`.claude/skills` for Claude, `.agents/skills` for Codex and Gemini; same in user scope), `Wrapper bool`
   (Claude true, Codex and Gemini false) and `LegacySkillsDirRelativePath` (Gemini only: `.gemini/skills`, used
   solely to clean up the old full copy). Keep `NativeSkill`.
2. **Shared files in the inspection and plan shapes.** <!-- Red team: ChangeSharedSkill contract -->
   - New kind `ChangeSharedSkill` ("shared-skill") for the canonical files `<root>/.agents/skills/<id>/<file>`.
     They belong to no host: `Inspection` gains `Shared []FileState` and `PlanResult` keeps them in `Changes` with
     an empty `Host`. `prepare` builds the shared set once per plan, not per adapter.
   - `buildPlan` kind order for writes: `ChangeMCP, ChangeHostPermissions, ChangeSharedSkill, ChangeNativeSkill,
     ChangeBootstrap` (canonical before wrappers). <!-- Red team: removal order --> For removal changes the order
     of the two skill kinds is reversed (wrappers and legacy copies first, canonical files last), so a failure
     part way never leaves a wrapper pointing at a deleted file.
   - `validateChangePath` handles `ChangeSharedSkill` before the adapter lookup: the path must be
     `<root>/.agents/skills/<id>/<file>` for an `<id>` and `<file>` of the bundle the plan covers (design 4),
     compared after `filepath.Clean`; then `rejectSymlinkComponents`.
   - `expectedChange` re-derives shared bytes from the bundle (create/update) or confirms ownership (removal).
3. **Per-item skill conflicts (decided).** <!-- Red team: conflict mid-fix aborts; tampered bundle blocks every doctor --fix -->
   A conflict on a system-skill file (shared, wrapper or legacy copy) no longer aborts the plan. `buildPlan` leaves
   that file out of `Changes` and records it in a new `PlanResult.Blocked []FileState` (path, reason); every
   other change, including MCP config and permissions, proceeds. Conflicts of the other kinds keep today's
   abort-the-plan contract (`integration.go:91-93`). When the canonical files of an id are blocked, the plan also
   removes that id's owned Claude wrapper (a normal removal change), so Claude Code never follows content Skill
   Hub did not write under a Skill Hub name. <!-- Red team: relative wrapper path trusts ~/.agents --> The block
   reason names the path and says: "not written by Skill Hub; Skill Hub left it in place and will not point
   agents at it. Review it, delete it, and run the command again." Consumers:
   - `connect`, `connect --hub` and doctor's pre-checks (`connect.go:53-67`, `connect_remote.go:196-211`,
     `workspace.go:377-392`) stop treating skill-file conflicts as fatal: they report each blocked file as its own
     item with that reason and continue; other-kind conflicts keep the existing "Nothing was written" result.
   - doctor: blocked items keep the result at `StatusActionRequired` and name the path; applied items are listed
     as "Remediated.".
   - `applyExternalConnections` collects the applied changes of every target and, on a plan or apply error,
     returns the items already applied plus one error item for that target instead of `Result{}, err`;
     `checkProjectAndGlobalConnections` reports blocked skill files as their own items, not as
     "outdated; run connect".
   - status (`curation_home.go:151-157`): blocked system-skill files surface as one host-integration item naming the
     first path (the `CheckMissing` error is no longer dropped silently for this case).
4. **Which skills a plan covers** (`plannedSystemSkills(mode, scope, root, workspace)`):
   - local mode: curator always; distill-lab only if `scope == ScopeProject && root == workspace`;
   - remote or remove mode: none to install; removal candidates: curator always, distill-lab only when `root`
     contains `.skillhub/schema-version` (so this code repo's tracked distill-lab source and wrapper, which match
     the shipped bytes, are never planned).
5. **When the canonical curator is kept** <!-- Red team: connect --hub keeps canonical; one shared curator folder -->
   (`needsCanonical(id, root)`, evaluated for remote and remove plans): the canonical files of an id stay only
   while, after the plan, at least one host in that root has a *local* registration (`Present && !Remote()`) and
   is not verified for the MCP skills extension. Local plans always install them. The hub-marker gate of design 4
   still decides whether distill-lab is considered at all. Consequences, each with a test:
   - `connect --hub` for all hosts in a project removes `.agents/skills/system-curator/` and the Claude wrapper.
   - Mixed project, `connect --hub <url> --host codex` while Claude Code stays local: the Claude wrapper stays, and
     the canonical folder stays because Claude still needs it, so Codex keeps reading the curator from
     `.agents/skills`. This is a stated limit of one shared folder: a curator file cannot be removed for one host
     while another host in the same folder still uses it. The `connect --hub` result for Codex says so in one line
     ("Codex still sees system-curator in .agents/skills while Claude Code in this folder uses the local hub").
   - removing every local registration (`--remove` or `--hub` for the last local host) removes canonical folders and
     empty parents.
6. **Wrapper rendering** (`skill-wrapper.go`): `renderWrapper(id, description) []byte`, Claude Code only.
   Frontmatter `name: <id>` and `description:` emitted by `yaml.v3` from the canonical frontmatter description;
   body (the first line carries a template version):
   ```text
   <!-- Written by Skill Hub (wrapper v1). Edit the skill in .agents/skills/<id>/, not here. -->

   This skill lives in the shared Agent Skills folder. Read
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/SKILL.md` now and follow it as this skill.
   Wherever it mentions this skill's folder or its scripts, use
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/`.
   ```
   The same bytes serve project scope (`<project>/.claude/skills/<id>/`) and user scope
   (`~/.claude/skills/<id>/` -> `~/.agents/skills/<id>/`).
7. **Wrapper ownership with history.** <!-- Red team: wrapper ownership has no history --> A wrapper is owned only
   when its bytes equal the current render or a previously shipped render: `known-wrappers.go` holds
   `id -> []sha256` of every wrapper a release wrote. `wrapperOwned` no longer re-renders with the description
   found in the file, so a wrapper whose description (or any other byte) the user edited is a conflict and is never
   overwritten. The current renders are committed as `testdata/wrappers/<id>.golden`; a test asserts current
   render == golden, and a tag-derived test built like Phase 1's (`git show <tag>:internal/hostintegration/testdata/wrappers/<id>.golden`
   for every `v*` tag that has the file, same `SKILLHUB_REQUIRE_TAGS` switch) asserts each shipped golden's digest is in `known-wrappers.go`. Changing
   the template or a canonical description therefore forces a golden update and, after a release, a digest entry.
8. **Ownership per path** (`systemSkillFileState`). Unknown content is never overwritten (owner-approved change,
   plan decision 4); a conflict is per item (design 3). <!-- Red team: Phase 5 ordering and ownership --> Export
   one read-only helper, `SystemSkillFileOwned(root, relPath string, raw []byte) bool`, which Phase 5's tracked
   and not-Skill-Hub notices call:
   - canonical path: missing -> create; bytes == current -> current; `KnownFileDigest` -> update; else blocked.
   - Claude wrapper path: missing -> create; current render -> current; known shipped render -> update; bytes are a
     known full copy of this skill's SKILL.md (old layout) -> replace with the wrapper; else blocked.
   - Gemini legacy path `.gemini/skills/system-curator/SKILL.md` (project and user scope): missing -> nothing;
     known full copy (current or previously shipped) -> delete, prune empty parents; else blocked, file left.
   - verified skills-extension host (`HostSupportsSkillsExtension`, `matrix.go:62-77`): Claude's curator wrapper
     is removed when owned (today's semantics kept); the distill-lab wrapper is still installed
     (`ServedOverMCP=false`); the canonical curator follows design 5.
9. **Symlinked skill folders are refused per item.** <!-- Red team: symlinked ~/.agents --> When `~/.agents`,
   `~/.claude`, `<root>/.agents`, `<root>/.claude` or any deeper component of a system-skill path is a symlink,
   Skill Hub does not write or delete through it: those skill files become blocked items ("Skill Hub does not write
   through the symlink <component>; system skill files were skipped"), and MCP config, permissions and
   instructions are still planned. The `ErrSymlink` from a skill-file read is caught in `prepare` for these paths
   only; other paths keep failing the plan as today.
10. **Removal** (remote and `--remove`): for the selected hosts, delete owned Claude wrappers and owned old full
    copies (Claude and Gemini paths) of the ids from design 4; delete the canonical files of an id only when
    `needsCanonical` (design 5) is false after the plan. Unowned files: left, plus the existing non-fatal behavior of
    `prepareCuratorRemoval` (returns an unchanged file, no conflict) is kept for removal, so `--remove` never fails
    on user content. `removeEmptyParents` already prunes empty folders (`files.go:222-224`).
11. **Dangling wrapper repair.** <!-- Red team: removal order / downgrade --> Doctor reports an owned Claude wrapper
    whose canonical `SKILL.md` is missing; in local mode the plan re-creates the canonical files (normal create
    change); in remote or remove mode it removes the wrapper.
12. **Item IDs and change lines.** <!-- Red team: shared kind breaks IDs and output --> `hostChangeItem` IDs
    become unique per file: `host_<host>_<kind>:<slash path relative to root>`; shared files use the display key
    `agents` (`host_agents_shared-skill:.agents/skills/distill-lab/SKILL.md`) and the level text `shared`.
    `hostChangeLines` parses with `strings.SplitN(id, "_", 3)` and cuts the `:` suffix; `hostDisplayName("agents")`
    returns "Agent Skills folder (.agents/skills)". So `connect` prints the canonical files on their own line next
    to the per-agent lines. The ID change is a JSON-visible change: Phase 7 adds a release-note line.
13. **"host integration missing" after an upgrade (explicit, accepted).** <!-- Red team: status after upgrade -->
    Every hub created before this release lacks the distill-lab files, and every project connected only to
    Claude Code lacks `.agents/skills/system-curator/`, so `MissingFinding` reports `host_integration_missing` in
    `status`, the web Home and `doctor` until `doctor --fix` (or `connect`) runs. This is accepted and documented:
    Phase 7 adds the release-note line, Phase 8 check 9 asserts that the item appears before and disappears after
    `doctor --fix`. The finding text stays owned by the onboarding plan.
14. **Doctor hint** (`internal/app/system_skill_doctor.go`): when the inspected root is the hub and distill-lab is
    installed, find `node` with `exec.LookPath` (the same name on every OS), run `node --version` with a 5 s
    timeout and parse the major version; when `node` is missing, fails to run or is older than 18, add one
    `Warning` ("distill-lab runs scripts/distill.mjs with Node 18 or newer; <not found | found vX.Y.Z>. Install
    Node from https://nodejs.org"). Never change `result.Status`. Inject the lookup and runner for tests
    (pattern: `internal/skillruntime/doctor.go:131`).
15. **Gemini notice (owner decision kept).** <!-- Red team: older Gemini CLI loses the curator --> No Gemini
    wrapper. When a plan removes an old `.gemini/skills/system-curator/SKILL.md`, the item's impact text and a
    doctor warning say: "Gemini CLI now reads system-curator from .agents/skills. A Gemini CLI older than
    <minimum version> reads only .gemini/skills, and Skill Hub does not serve system-curator to Gemini over MCP,
    so such a version no longer sees the curator; update Gemini CLI." The minimum version is filled in at start
    ("Re-verify at start"); MCP hiding (`server.go:399-411`) is unchanged.
16. **Doctor adapter (kept, agreed with the onboarding owner).** The Node warning, the Gemini notice and Phase 5's
    notices are cited here against today's `Finding`/`Warnings` and written against the extension point that
    exists at implementation start (the onboarding plan's phase 4 names it). One function in
    `system_skill_doctor.go` returns them for a root; the check set calls only that function.
    <!-- Red team: integrity reported twice --> Wrapper and canonical integrity are reported only through the
    host inspection items (design 3, 12), never through the adapter, so nothing is listed twice.

<!-- Red team: phantom connection string --> The former design item that rewrote "connection written:
claude-code, codex-cli, gemini-cli" is removed: that string exists neither on main nor in the onboarding plan
(main prints "Agent connection for this workspace written: %s." at `internal/delivery/cli/workspace.go:57` and
"Agent connection for <label> written." at `internal/app/connect.go:92`). The new layout reaches the output only
through design 12's shared line; the onboarding plan's per-agent lines are not edited.

## Tests (names describe behavior)

- Fresh hub plan writes canonical files for both ids and Claude wrappers for both; no Codex or Gemini wrapper; no
  path under `.gemini/`; canonical changes come before wrapper changes.
- Project-scope plan with `Root != Workspace` and user-scope plan: curator canonical + Claude wrapper only; no path
  contains `distill-lab`; the user-scope wrapper bytes equal the project-scope ones.
- Remote and remove plans never contain a create or update for any system skill; removal plans list wrappers and
  legacy copies before canonical files.
- Remove plan in a root without `.skillhub/schema-version` that holds owned distill-lab files (repo-shaped
  folder): no change touches `distill-lab`.
- `connect --hub` for all hosts in a locally connected project removes `.agents/skills/system-curator/` and the
  wrapper; `connect --hub --host codex` with Claude Code still local keeps both and prints the one-line limit.
- Old layout (three full current curator copies) -> one plan updates `.claude` to the wrapper, deletes the
  `.gemini` copy and its empty folders, keeps `.agents` current, and carries the Gemini notice; second plan is
  empty.
- Old layout with a previously shipped curator version (from known digests) -> migrated, no conflict.
- Edited full copy at `.claude/skills/system-curator/SKILL.md` or at `.gemini/skills/system-curator/SKILL.md` ->
  blocked item naming that path; file unchanged; MCP config changes in the same plan are still applied.
- Edited canonical file -> blocked item naming the canonical path; the owned wrapper of that id is removed;
  `connect` returns a result (not a Go error) listing the blocked path; `doctor` returns `StatusActionRequired`
  with an item naming the path; `doctor --fix` applies every other change.
  `doctor-skew_test.go:62-75` rewritten for the decided behaviour change: stale *unknown* content is blocked and
  stays unchanged instead of being restored; a stale *known* version is restored by `DoctorFix` without a
  conflict.
- `DoctorFix` with an edited user-scope `~/.gemini/skills/system-curator/SKILL.md`: the hub is migrated, the
  result lists the hub changes and one blocked item; `checkProjectAndGlobalConnections` does not suggest
  `connect -g` for it.
- Wrapper ownership: a shipped older wrapper (digest in `known-wrappers.go`) is updated; a wrapper with an edited
  description or extra text is blocked; current render equals the golden file.
- Symlinked `~/.agents` (and, separately, `<root>/.claude`) with `connect -g --host claude`: MCP config is written,
  the skill files are blocked items naming the symlink, nothing is written through it.
- Dangling wrapper (canonical folder deleted by hand): doctor reports it; `doctor --fix` re-creates the canonical
  files.
- Item IDs: two native-skill files of one host have distinct IDs; `hostChangeLines` prints an "Agent Skills folder
  (.agents/skills)" line for shared files and no line with an empty host name.
- Status after upgrade: an old-layout hub shows `host_integration_missing`; after `doctor --fix` it is gone.
- Windows path shape: wrapper relative path and `validateChangePath` work with `filepath` separators (table test
  with `filepath.FromSlash`); files written `0644`.
- Apply rejects a forged plan whose shared-skill change points outside `.agents/skills/<id>/` or whose native-skill
  change writes under `.gemini/`.
- Doctor hint: `node` missing -> one warning, status unchanged; `node --version` prints `v16.20.2` -> warning
  naming the version; `v18.0.0` and `v24.21.0` -> no warning; unparsable output or a failing run -> warning;
  non-hub root -> no check.
- Doctor adapter: in a hub, the check set receives the Node warning and the Gemini notice through the adapter only;
  a non-hub role gets none; no integrity item appears twice.
- `skillhub version` output is byte-unchanged.
- `TestMatrixSynchronized` after the note update.

## Steps

1. Run the plan's "Re-verify at start" items for this phase and update the citations above (including the
   onboarding plan's per-agent output and doctor extension point). Then adapter fields and
   `plannedSystemSkills`; port existing curator tests to the new paths.
2. Shared kind: `Inspection.Shared`, `PlanResult.Blocked`, prepare, buildPlan ordering (write and removal),
   expectedChange, validateChangePath, preview text.
3. Wrapper rendering, golden files, `known-wrappers.go`, ownership; migration cases including the Gemini legacy copy.
4. Per-item conflicts and symlink refusal; the app consumers (`connect.go`, `connect_remote.go`, `workspace.go`,
   `host-connections.go`, `curation_home.go:150-157`) and the CLI line renderer.
5. Removal rules (`needsCanonical`), the hub-marker gate, dangling-wrapper repair; remote tests.
6. Doctor hint, Gemini notice and the adapter.
7. Update the tests and goldens outside the package listed under "Owned files".
8. Matrix note: "Native delivery writes the skill once under `.agents/skills/<id>/`, which Codex and Gemini CLI
   read directly; Claude Code gets a generated wrapper SKILL.md that points to it."
9. `go test ./internal/hostintegration/... ./internal/app/... ./internal/delivery/...`, then `make check`.
10. Commits: `feat(hostintegration): install system skills once under .agents/skills with a Claude Code wrapper`,
    `feat(hostintegration): report a changed system skill file without blocking other repairs`,
    `feat(doctor): hint when distill-lab cannot find Node 18 or newer`.

## Validation

- All tests above pass; `make check` passes; `git grep -n "system-curator/SKILL.md" internal/hostintegration`
  shows only test fixtures for the old layout; `git grep -n "\.gemini/skills" internal/hostintegration` shows only
  the legacy cleanup path and its tests.
- <!-- Red team: tests outside Phase 4 --> `git grep -n "skills/system-curator\|\.gemini/skills" -- internal
  ':!internal/hostintegration'` shows only the new expectations (canonical plus wrapper) and legacy-cleanup tests.

## Risks

- Plan/Apply contract drift (plans are re-derived at apply time): every new path type goes through
  `expectedChange` and `validateChangePath`; the forged-plan test guards it.
- Per-item conflicts change a public result shape (blocked items, item IDs): owned tests and goldens are updated in
  this phase; the release note names the ID change.
- `doctor` from a project or home now inspects fewer files (no distill-lab): intended.
- One shared curator folder per root cannot serve "curator for Claude only": design 5 states the result and the
  connect output says it.
- Claude user-scope permission prompt for `~/.agents`: unverified (plan decision 2). If the owner-run check in
  Phase 8 sees one, the fix is a read-only allow rule agreed with the onboarding plan's permission owner;
  `~/.agents` is never added to `additionalDirectories` (that would also grant writes to a folder other tools
  share). <!-- Red team: relative wrapper path trusts ~/.agents -->
- The onboarding plan's check-set names differ from what this plan cites: absorbed by the adapter (design 16).

## Rollback

Revert the commits. <!-- Red team: downgrade path --> After a downgrade the old binary does not know the wrappers:
its `doctor --fix` writes the full curator copies back over `.claude/skills/system-curator/SKILL.md` after
confirmation, but it never touches `.claude/skills/distill-lab/SKILL.md`, and its `connect --remove` may delete the
canonical curator while leaving the curator wrapper. The rollback note (plan.md) tells the owner to delete
`.claude/skills/system-curator/` and `.claude/skills/distill-lab/` by hand if the old binary reports them.
