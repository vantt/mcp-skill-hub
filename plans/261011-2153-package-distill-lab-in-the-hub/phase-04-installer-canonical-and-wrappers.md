---
phase: 4
title: "Installer: canonical folder, Claude wrapper, migration, removal, doctor hint"
status: done
priority: P1
effort: "20h"
wave: 2
dependencies: [1]
---

# Phase 4: Installer: canonical folder, Claude wrapper, migration, removal, doctor hint

<!-- Red team: effort raised from 8h to 16h. The host-less shared kind touches Inspection, PlanResult, every
conflict pre-check in internal/app, the CLI change-line renderer, five test files and two goldens outside the
package, per-item conflicts, wrapper history, removal order and symlink handling. -->
<!-- Validation: effort raised from 16h to 20h for the Apply rank check, the DoctorFix gate, applyHostIntegration,
the other item-ID readers and formatChangeKind, the two output goldens, several ids per host in validateChangePath
and expectedChange, every adapter's registrations for needsCanonical, Blocked in MissingFinding, and the doctor
provider written against the named extension point. Citations re-checked on main 944cfbf. -->

## Context (traced on main `ef535de`, pre-0.5.0; re-verify every line number at start)

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
  - `levels` maps built at `internal/app/connect.go:52`, `internal/app/connect_remote.go:196`,
    `internal/app/workspace.go:377,437`, and `levels[change.Host]` lookups at `internal/app/connect.go:79,94`,
    `internal/app/connect_remote.go:226,244`, `internal/app/workspace.go:403,450`; <!-- Validation: levels citations -->
  - item ID `"host_"+host+"_"+kind` (`hostChangeItem`, `internal/app/workspace.go:501-503`), read by: <!-- Validation: item-ID parsers -->
    `hostChangeLines` with `strings.Split(item.ID, "_")` (`internal/delivery/cli/connect_remote.go:119-147`);
    `renderConnectResult` with the same split (`internal/delivery/cli/connect.go:120`); `connectCurationGuidance` by
    exact equality with the Claude MCP and permissions IDs (`internal/delivery/cli/connect.go:104-105`); the init
    summary host list, whose comment says "neither part contains _" and which requires exactly three parts
    (`internal/delivery/cli/workspace.go:46-47`); tests `internal/hostintegration/doctor-skew_test.go:40` and
    `internal/delivery/cli/connect_remote_test.go:628-630,691-692`. Hosts are named by `hostDisplayName`
    (`internal/delivery/cli/connect.go:161-172`) and files by `formatChangeKind` (`:174-207`), which hard-codes
    `native-skill` -> "curator skill" (`:194-195`), pinned on every host line of
    `internal/delivery/cli/testdata/connect-workspace/{project-output,global-output}.golden` (used at
    `internal/delivery/cli/connect_regression_test.go:231,248`);
  - `validateChangePath` looks the adapter up before it reads the kind, so an empty host fails with "unsupported
    host", and its native-skill branch accepts exactly one path per host (`files.go:304-339`, called at `:167`);
    `expectedChange` (`files.go:236-287`, called at `:180`) returns the full curator bytes for every local
    native-skill change (`:252`);
  - `MissingFinding` treats any change with a missing preimage as "host integration missing" (`types.go:241-254`);
    status calls it through `CheckMissing` (`types.go:257`) and drops its error (`internal/app/curation_home.go:151-157`,
    call at `:154`); doctor calls it at `internal/app/workspace.go:397`. The status action for it is built at
    `internal/app/curation_home.go:497-510` and labelled and summarized at `:536-563,573,585`.
- <!-- Validation: Apply rank check --> Apply checks a fixed dependency rank per kind: `changeOrder`
  (`files.go:289-302`: MCP 1, permissions 2, native skill 3, bootstrap 4, any other kind -1), called at `:158`,
  rejects a plan whose ranks are not monotonic, or that contains an unknown kind, with "plan changes are not
  dependency ordered". A new kind, or removals ordered differently from writes, fails there unless the rank changes.
- Apply (`apply`, `files.go:123-234`) re-derives desired bytes per path (`files.go:236-252`), writes changes one at a
  time in plan order and returns on the first error with no undo (`files.go:194-226`); writes go through
  `atomicWrite` (`files.go:407`), which creates safe parent directories; symlink components are refused
  (`files.go:19-35,51`).
- <!-- Validation: prepare reads selected adapters only --> `prepare` iterates only the selected adapters
  (`integration.go:120`), so it does not know the registrations of other hosts in the same root.
- Doctor: a non-fixable workspace finding returns before any host repair (`internal/app/workspace.go:266-270`,
  again under the lock `:289-300`). <!-- Validation: DoctorFix status gate and applyHostIntegration --> `DoctorFix`
  runs `applyExternalConnections` only when `apply` returned no error and a status of Applied, Ready or OK
  (`internal/app/workspace.go:228-230`). `applyHostIntegration` (`internal/app/workspace.go:418-456`, the init and
  `doctor --fix` path) has no conflict pre-check: a `ConflictError` from Plan returns `Result{}, err` (`:441-443`).
  `applyExternalConnections` returns `Result{}, err` on the first plan error, which discards the hub changes
  already applied (`internal/app/host-connections.go:66-73`); `checkProjectAndGlobalConnections` turns any plan
  error into "outdated or needs repair" and suggests `connect` again (`:91-100`).
- Hub vs project: init/doctor use `Request{Workspace: root}` (`internal/app/workspace.go:465`); `connect` sets
  `Root` to the project or home in `ConnectService.target` (`internal/app/connect.go:131-160`), and doctor's project
  and user checks do the same (`internal/app/host-connections.go:23,26`); remote and removal plans carry no
  workspace (`internal/app/connect_remote.go:159,441`). <!-- Validation: decision 6 citation --> <!-- Red team: connect --hub canonical -->
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
  `internal/app/system_skill_doctor.go` (+ `system_skill_doctor_test.go`): <!-- Validation: onboarding extension point -->
  `systemSkillProvider`, the `CheckProvider` of plan decision 10, registered with `RegisterProvider` from `init()`
  (Node check here; Phase 5 adds its two checks).
- Modify (app): `internal/app/connect.go` (conflict pre-check, `levels`), `internal/app/connect_remote.go`
  (conflict pre-check, `levels`), `internal/app/workspace.go` (`appendHostInspectionItems`, `hostChangeItem` ID and
  label, the doctor conflict branch, `MissingFinding` use, <!-- Validation: DoctorFix status gate -->the `DoctorFix`
  gate at `:228-230` and `applyHostIntegration` at `:418-456`), `internal/app/host-connections.go`
  (`applyExternalConnections`, `checkProjectAndGlobalConnections`), `internal/app/curation_home.go:150-157` only
  (status host-integration finding; Phase 6 owns the `review_lessons` block of the same file and runs after this
  phase; the host-integration action block `:497-510` and its labels `:536-563,573,585` are not edited, design 3).
- Modify (CLI): <!-- Validation: item-ID parsers --> new `internal/delivery/cli/host_item_id.go` (+ test) with the
  one ID parser; `internal/delivery/cli/connect_remote.go` (`hostChangeLines`), `internal/delivery/cli/connect.go`
  (`renderConnectResult`, `hostDisplayName`, `formatChangeKind`; `connectCurationGuidance` only gains a test), and
  the host-key collection that feeds the connection summary (today `internal/delivery/cli/workspace.go:46-50`;
  after the onboarding merge, the input of their `connectionSummaryLine`).
- Modify (tests and goldens outside the package, expected content: canonical files plus Claude wrapper, nothing
  under `.gemini/`): `internal/app/workspace_test.go` (`hostArtifactPaths`), `internal/app/connect_test.go` and
  `internal/app/connect_remote_test.go` if they pin the layout (grep at start),
  `internal/delivery/cli/onboarding_test.go`, `internal/delivery/cli/connect_regression_test.go`,
  `internal/delivery/cli/testdata/connect-workspace/{project-files,global-files}.golden`,
  <!-- Validation: output goldens --> `internal/delivery/cli/testdata/connect-workspace/{project-output,global-output}.golden`
  (no "curator skill" on the Codex and Gemini lines, `system-curator skill` on the Claude line, a new "Agent Skills
  folder (.agents/skills)" line), `internal/delivery/cli/connect_remote_test.go` (`:628-630`; `:691-692` unchanged),
  and in the package `internal/hostintegration/doctor-skew_test.go` (`:23,37,40,69`).
- Modify both copies (kept identical, `TestMatrixSynchronized`): `docs/mcp-compatibility-matrix.json`,
  `internal/hostintegration/matrix.json` (only the curator delivery note).
- Not owned: `cli-permissions.go`, MCP config writers, launch args, the onboarding plan's per-agent connection
  lines and the wording of the connection summary (`internal/delivery/cli/workspace.go:57` today, their
  `connectionSummaryLine` after the merge), `internal/app/health.go`, `skillhub version`.

## Design

1. **Adapter fields.** Replace `SkillRelativePath`/`UserSkillRelativePath` with `SkillsDirRelativePath`
   (`.claude/skills` for Claude, `.agents/skills` for Codex and Gemini; same in user scope), `Wrapper bool`
   (Claude true, Codex and Gemini false) and `LegacySkillsDirRelativePath` (Gemini only: `.gemini/skills`, used
   solely to clean up the old full copy). Keep `NativeSkill`.
2. **Shared files in the inspection and plan shapes.** <!-- Red team: ChangeSharedSkill contract -->
   - New kind `ChangeSharedSkill` ("shared-skill") for the canonical files `<root>/.agents/skills/<id>/<file>`.
     They belong to no host: `Inspection` gains `Shared []FileState` and `PlanResult` keeps them in `Changes` with
     an empty `Host`. `prepare` builds the shared set once per plan, not per adapter.
   - <!-- Validation: Apply rank check --> One rank per change, shared by `buildPlan` and Apply: replace
     `changeOrder(kind ChangeKind)` (`files.go:289-302`, called at `:158`) with `changeRank(change Change) int`:
     MCP 1, host permissions 2, shared-skill create/update 3, native skill (wrapper writes, wrapper removals,
     legacy-copy removals) 4, shared-skill removal 5, bootstrap 6; unknown kinds stay -1. Apply keeps its
     monotonic check over these ranks, so canonical writes come before wrappers <!-- Red team: removal order -->and
     canonical removals come after wrapper and legacy removals, and a failure part way never leaves a wrapper
     pointing at a deleted file. One plan never both writes and removes the same path (Apply's duplicate-path
     check, `files.go:163-166`). `buildPlan` emits changes sorted stably by `changeRank` instead of its hard-coded
     kind list (`integration.go:87`); the relative order of today's four kinds is unchanged.
   - `validateChangePath` handles `ChangeSharedSkill` before the adapter lookup: the path must be
     `<root>/.agents/skills/<id>/<file>` for an `<id>` and `<file>` of the bundle the plan covers (design 4),
     compared after `filepath.Clean`; then `rejectSymlinkComponents`. <!-- Validation: several ids per host --> Its
     native-skill branch, which today allows exactly one path per host, accepts for a wrapper host
     `<root>/<SkillsDirRelativePath>/<id>/SKILL.md` for every `<id>` the plan covers, and for Gemini only the
     legacy `<root>/.gemini/skills/system-curator/SKILL.md`, and that path only in a removal change.
   - `expectedChange` (`files.go:236-287`; today local native-skill always returns the full curator bytes, `:252`)
     re-derives per path: a shared file from the bundle bytes of that `<id>`/`<file>`; a wrapper from
     `renderWrapper` for the `<id>` in the path; the Gemini legacy path is valid only as a removal of owned bytes;
     removals of shared files and wrappers confirm ownership (design 7, 8).
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
   - doctor (no `--fix`): blocked items keep the result at `StatusActionRequired` and name the path.
   - <!-- Validation: applyHostIntegration --> `applyHostIntegration` (`internal/app/workspace.go:418-456`, init and
     `doctor --fix`): it applies the plan's changes as today, adds one item per `PlanResult.Blocked` entry with the
     block reason, and lists applied items as "Remediated."; a conflict of another kind still returns the error as
     today (`:441-443`). It does not set `StatusActionRequired` for blocked items itself.
   - <!-- Validation: DoctorFix status gate --> `DoctorFix` (`internal/app/workspace.go:228-230`): the gate that runs
     `applyExternalConnections` only for Applied, Ready or OK no longer sees a blocked hub item as a reason to stop,
     because `applyHostIntegration` leaves the status alone for blocked items. After `applyExternalConnections`
     returns, `DoctorFix` sets `StatusActionRequired` when any hub, project or user item is blocked. So a blocked
     hub file never skips the project and user repairs, which is what the acceptance criterion "every other repair
     in the same run is applied" requires. Errors and other non-OK statuses keep stopping there as today.
   - `applyExternalConnections` collects the applied changes of every target and, on a plan or apply error,
     returns the items already applied plus one error item for that target instead of `Result{}, err`;
     `checkProjectAndGlobalConnections` reports blocked skill files as their own items, not as
     "outdated; run connect".
   - <!-- Validation: MissingFinding reads Blocked --> status and doctor: with per-item blocking Plan returns no error
     for a skill conflict, so the blocked state must come from the plan itself. `MissingFinding(plan)`
     (`types.go:241-254`; used by `CheckMissing`, `types.go:257`, in status at `curation_home.go:154`, and directly
     by doctor at `internal/app/workspace.go:397`) returns, after the missing-file case, a Finding for
     `PlanResult.Blocked`: ID `host_integration_blocked`, summary "<first path> was not written by Skill Hub; review
     it or delete it, then run skillhub doctor --fix" (plus "and N more" when several), `Fixable: false`. Missing
     takes precedence when both occur; doctor lists every blocked item anyway. Status keeps using the existing
     host-integration action block (`curation_home.go:497-510`), which copies the Finding's ID and summary, so the
     block path reaches status, the web Home and `hub_status` without editing `:497-590`; that block, its kind
     `host_integration_missing` and its labels stay owned by the onboarding plan. `curation_home.go:150-157` only
     changes if `CheckMissing`'s signature does.
4. **Which skills a plan covers** (`plannedSystemSkills(mode, scope, root, workspace)`):
   - local mode: curator always; distill-lab only if `scope == ScopeProject && root == workspace`;
   - remote or remove mode: none to install; removal candidates: curator always, distill-lab only when `root`
     contains `.skillhub/schema-version` (so this code repo's tracked distill-lab source and wrapper, which match
     the shipped bytes, are never planned).
5. **When the canonical curator is kept** <!-- Red team: connect --hub keeps canonical; one shared curator folder -->
   (`needsCanonical(id, root)`, evaluated for remote and remove plans): the canonical files of an id stay only
   while, after the plan, at least one host in that root has a *local* registration (`Present && !Remote()`) and
   is not verified for the MCP skills extension. Local plans always install them. The hub-marker gate of design 4
   still decides whether distill-lab is considered at all. <!-- Validation: prepare reads every adapter -->
   `prepare` iterates only the selected adapters (`integration.go:120`), so for remote and remove plans it also
   reads the config of every supported adapter in that root (read-only, through `scopeRegistration` and
   `Registration.Remote()`) to evaluate `needsCanonical`; only the selected adapters get changes. Consequences,
   each with a test:
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
12. **Item IDs and change lines.** <!-- Red team: shared kind breaks IDs and output --> <!-- Validation: item-ID parsers and formatChangeKind -->
    Only skill-file IDs change, so every other ID stays parseable as today: native-skill and shared-skill items
    become unique per file, `host_<host>_<kind>:<slash path relative to root>`; shared files use the display key
    `agents` (`host_agents_shared-skill:.agents/skills/distill-lab/SKILL.md`) and the level text `shared`. MCP,
    permissions and bootstrap IDs keep the exact `host_<host>_<kind>` form, so `connectCurationGuidance`'s equality
    check (`internal/delivery/cli/connect.go:104-105`) and `connect_remote_test.go:691-692` need no change (a test
    pins it). Readers:
    - one parser, `parseHostItemID(id) (host, kind, relPath string, ok bool)` in the new
      `internal/delivery/cli/host_item_id.go`: `strings.SplitN(id, "_", 3)`, then cut the `:` suffix of the third part
      (kinds and hosts contain no `_` or `:`);
    - `hostChangeLines` (`connect_remote.go:123`), `renderConnectResult` (`connect.go:120`) and the host-key
      collection of the connection summary (today `internal/delivery/cli/workspace.go:46-47`, after the merge the
      input of `connectionSummaryLine`) use it; the summary skips the `agents` key, so the shared folder is never
      printed as an agent;
    - `hostDisplayName("agents")` returns "Agent Skills folder (.agents/skills)";
    - `formatChangeKind` no longer hard-codes `native-skill` -> "curator skill" (`connect.go:194-195`): for
      `native-skill` and `shared-skill` it returns "<id> skill", the `<id>` taken from the path after `skills/`
      (so distill-lab prints as "distill-lab skill"), and each line lists a description once;
    - tests: `doctor-skew_test.go:40` and `connect_remote_test.go:628-630` use the new skill IDs.
    So `connect` prints the canonical files on their own line next to the per-agent lines. The skill-ID change is
    JSON-visible: Phase 7 adds a release-note line.
13. **"host integration missing" after an upgrade (explicit, accepted).** <!-- Red team: status after upgrade -->
    Every hub created before this release lacks the distill-lab files, and every project connected only to
    Claude Code lacks `.agents/skills/system-curator/`, so `MissingFinding` reports `host_integration_missing` in
    `status`, the web Home and `doctor` until `doctor --fix` (or `connect`) runs. This is accepted and documented:
    Phase 7 adds the release-note line, Phase 8 check 9 asserts that the item appears before and disappears after
    `doctor --fix`. The finding text stays owned by the onboarding plan.
14. **Node check** <!-- Validation: onboarding extension point --> (`internal/app/system_skill_doctor.go`,
    `systemSkillProvider.Check` at `DepthFull` only, for the hub role): when distill-lab is installed, find `node`
    with `exec.LookPath` (the same name on every OS), run `node --version` with a 5 s timeout and parse the major
    version; when `node` is missing, fails to run or is older than 18, return one `Finding{Code:
    "distill_lab_node_missing", Severity: notice, Message: "distill-lab runs scripts/distill.mjs with Node 18 or
    newer; <not found | found vX.Y.Z>.", Fix: nil, Manual: "Install Node from https://nodejs.org"}`. A notice never
    changes the doctor status. Inject the lookup and runner for tests (pattern: `internal/skillruntime/doctor.go:131`).
15. **Gemini notice (owner decision kept).** <!-- Red team: older Gemini CLI loses the curator --> <!-- Validation: Gemini CLI minimum version -->
    No Gemini wrapper. When a plan removes an old `.gemini/skills/system-curator/SKILL.md`, the removal item's
    impact text and a result warning (the plan's own output, not a provider Finding, because it exists only while
    that removal is planned or applied) say: "Gemini CLI now reads system-curator from .agents/skills. A Gemini CLI
    older than v0.28.0 reads only .gemini/skills, and Skill Hub does not serve system-curator to Gemini over MCP,
    so such a version no longer sees the curator; update Gemini CLI." v0.28.0 is the first release with PR #18151
    (https://github.com/google-gemini/gemini-cli/pull/18151). MCP hiding (`server.go:399-411`) is unchanged.
16. **Doctor provider, and why integrity is not in it.** <!-- Validation: onboarding extension point -->
    `systemSkillProvider` implements the onboarding plan's `CheckProvider` (their phase-04:33-63: `Name`,
    `Applies(RoleReport)`, `Check(ctx, RoleReport, Depth) []Finding`) and is registered with `RegisterProvider` from
    the file's `init()`. `Applies` is true for the hub role only. It returns only `notice` Findings with `Fix` nil:
    the Node check (design 14) and Phase 5's `system_skill_tracked` and `system_skill_unowned_ignored`.
    <!-- Red team: integrity reported twice --> Wrapper and canonical integrity (missing, outdated, blocked, the
    old-layout migration, dangling wrappers, the Gemini notice) stays in the host inspection items (design 3, 12),
    which the onboarding plan's own hub-doctor and host-connection providers carry. This is the answer to their
    expectation (plan.md:208, phase-04:62-63) that this plan registers "wrapper/canonical integrity with a `--fix`
    migration" through `RegisterProvider`: those repairs need Plan/Apply semantics (preimage digests, desired bytes
    re-derived at apply time, the rank check of design 2, symlink refusal, per-item blocking); a
    `FixAction.Apply` closure would duplicate that engine or bypass its checks, and the files would be reported
    twice. The lead tells their owner before this phase starts (plan.md "Sequencing").

<!-- Red team: phantom connection string --> <!-- Validation: connection summary string comes from the onboarding plan -->
The former design item that rewrote "connection written: claude-code, codex-cli, gemini-cli" is removed. That
string is not on main (main prints "Agent connection for this workspace written: %s." at
`internal/delivery/cli/workspace.go:57` and "Agent connection for <label> written." at `internal/app/connect.go:92`);
it comes from the onboarding plan, whose `connectionSummaryLine` renders it for `init` and `connect`
(their plan.md:210, phase-02:147-148, "shared with the distill-lab plan"). This plan reuses that function
unchanged; it only makes sure the host keys it receives never include `agents` (design 12). The new layout reaches
the output through design 12's shared line; the onboarding plan's per-agent lines are not edited.

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
- <!-- Validation: Apply rank check --> Apply accepts the plans this phase builds and rejects unordered ones: a
  migration plan (shared create, wrapper update, Gemini legacy removal), a blocked-canonical plan (wrapper
  removal next to other shared writes), and a remote plan (wrapper removal before shared removal) apply without
  "not dependency ordered"; a forged plan with a shared removal before a wrapper removal, or a shared write after
  a wrapper write, is rejected.
- <!-- Validation: DoctorFix status gate --> `DoctorFix` in a hub with a blocked hub file while the current folder is
  a connected project and the user scope is connected: the project and user repairs are applied (the gate at
  `workspace.go:228-230` does not stop them), the result lists the blocked item, and the final status is
  `StatusActionRequired`.
- <!-- Validation: applyHostIntegration --> `init` over a folder that already holds an edited
  `.claude/skills/system-curator/SKILL.md`: init completes, lists one blocked item, and writes every other file.
- <!-- Validation: MissingFinding reads Blocked --> Status with a blocked canonical file and nothing missing: the
  `host_integration_missing` action carries ID `host_integration_blocked` and a summary naming the path;
  `hub_status` JSON carries the same; with a file also missing, the missing finding wins.
- <!-- Validation: prepare reads every adapter --> `connect --hub --host codex` in a project where only Claude Code
  is registered locally: `needsCanonical` sees the Claude registration although Claude is not selected, and the
  canonical folder stays.
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
- <!-- Validation: item-ID parsers and formatChangeKind --> `parseHostItemID` table test (old three-part IDs, skill IDs
  with a path suffix, the `agents` key, malformed IDs); `renderConnectResult` and the connection-summary host list
  never print `agents` as an agent; `connectCurationGuidance` still fires for the unchanged Claude MCP and
  permissions IDs; a hub `connect` lists "distill-lab skill" (never "curator skill") for the distill-lab wrapper
  and canonical files; the regenerated `project-output.golden` and `global-output.golden` show no skill on the
  Codex and Gemini lines.
- Status after upgrade: an old-layout hub shows `host_integration_missing`; after `doctor --fix` it is gone.
- Windows path shape: wrapper relative path and `validateChangePath` work with `filepath` separators (table test
  with `filepath.FromSlash`); files written `0644`.
- Apply rejects a forged plan whose shared-skill change points outside `.agents/skills/<id>/` or whose native-skill
  change writes under `.gemini/`.
- Node check: `node` missing -> one `notice` Finding, status unchanged; `node --version` prints `v16.20.2` ->
  notice naming the version; `v18.0.0` and `v24.21.0` -> none; unparsable output or a failing run -> notice;
  non-hub role -> `Applies` false; `DepthLight` -> no `node` run.
- Doctor provider: <!-- Validation: onboarding extension point --> `systemSkillProvider` is registered once through
  `RegisterProvider`; in a hub, `Run` at full depth returns its notices in the hub section; no integrity item
  (wrapper, canonical, Gemini notice) appears in its Findings, so nothing is listed twice.
- `skillhub version` output is byte-unchanged.
- `TestMatrixSynchronized` after the note update.

## Steps

1. Run the plan's "Re-verify at start" items for this phase and update the citations above (including the
   onboarding plan's per-agent output and doctor extension point). Then adapter fields and
   `plannedSystemSkills`; port existing curator tests to the new paths.
2. Shared kind: `Inspection.Shared`, `PlanResult.Blocked`, prepare, `changeRank` in `buildPlan` and Apply (write
   and removal), expectedChange and validateChangePath for several ids per host and the Gemini legacy removal,
   preview text. <!-- Validation: Apply rank check -->
3. Wrapper rendering, golden files, `known-wrappers.go`, ownership; migration cases including the Gemini legacy copy.
4. Per-item conflicts and symlink refusal; `MissingFinding` reading `Blocked`; the app consumers (`connect.go`,
   `connect_remote.go`, `workspace.go` including `applyHostIntegration` and the `DoctorFix` gate,
   `host-connections.go`, `curation_home.go:150-157`); the CLI ID parser, line renderers, `formatChangeKind` and the
   summary host list. <!-- Validation: consumers added -->
5. Removal rules (`needsCanonical` over every adapter's registrations), the hub-marker gate, dangling-wrapper
   repair; remote tests.
6. Node check, Gemini notice and `systemSkillProvider` registered through `RegisterProvider`.
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
- Claude user-scope permission prompt for `~/.agents`: <!-- Validation: Claude Code permissions docs --> expected to
  prompt for reads outside the working directory in Manual mode, per the permissions docs (plan decision 2); the
  project-scope wrapper reads inside the working directory without a prompt. If the prompt proves a problem, the
  fix is a read-only allow rule agreed with the onboarding plan's permission owner;
  `~/.agents` is never added to `additionalDirectories` (that would also grant writes to a folder other tools
  share). <!-- Red team: relative wrapper path trusts ~/.agents -->
- The onboarding plan's extension-point names differ on the merged main from their phase-04:33-63: only
  `system_skill_doctor.go` changes (design 16; "Re-verify at start").
- <!-- Validation: Apply rank check --> The new rank changes Apply's accepted order for every plan: today's four kinds
  keep their relative order, and the existing ordering tests stay green.

## Rollback

Revert the commits. <!-- Red team: downgrade path --> After a downgrade the old binary does not know the wrappers:
its `doctor --fix` writes the full curator copies back over `.claude/skills/system-curator/SKILL.md` after
confirmation, but it never touches `.claude/skills/distill-lab/SKILL.md`, and its `connect --remove` may delete the
canonical curator while leaving the curator wrapper. The rollback note (plan.md) tells the owner to delete
`.claude/skills/system-curator/` and `.claude/skills/distill-lab/` by hand if the old binary reports them.
