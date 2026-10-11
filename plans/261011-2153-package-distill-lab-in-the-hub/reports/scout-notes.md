# Scout notes: package distill-lab in the hub

Evidence gathered on 2026-10-11 at `c86b295` (main, clean). Paths are relative to the repo root.

## System skill packaging today

- `internal/systemskills/embed.go:23-27` embeds one file, `curator/SKILL.md`, as `CuratorSkill`; `:94-102`
  `CuratorBundle()` returns it with a sha256 digest. IDs and versions at `:10-21`.
- `internal/systemskills/embed_test.go:22-28` requires the embedded file to equal `system-skills/curator/SKILL.md`
  (both trees identical, `cmp` clean). `AGENTS.md:16` names `system-skills/curator/SKILL.md`.
- Curator consumers: `internal/hostintegration/bootstrap.go:19`; `internal/app/distribution.go:131-137,169-170,196-197,224-225,249-250,516-517`;
  `internal/delivery/mcpserver/server.go:329-331,365,385,419,499,570`; `internal/delivery/mcpserver/skill_tools.go:214,229`;
  `internal/app/skill_bundle.go:54,130`; `internal/app/skill_snapshot.go:393`.
- Curator history: 12 commits touch the curator SKILL.md (`git log --oneline -- system-skills/curator/SKILL.md internal/systemskills/curator/SKILL.md`).

## distill-lab dev copy

- `.claude/skills/distill-lab/SKILL.md` (493 lines) and `scripts/distill.py` (421 lines), both 100644 in Git.
- `SKILL.md:292` "Hub root: `$SKILLHUB_WORKSPACE`, else `~/skill-hub`." `SKILL.md:341-354` script usage with
  `S=<this skill's folder>/scripts/distill.py`. `SKILL.md:386-388` mirrors at `<hub>/runtime/distill-lab/mirrors/<source-id>`.
- `distill.py:24` `import yaml` (unguarded); `:194-201` `_git` uses `<hub>/runtime/distill-lab/mirrors/<src>`;
  `:273-278` canonical header "Written by distill-lab/scripts/distill.py ..."; `:293-295` `hub_of` = `parents[4]`.
- Header copies in fixtures: `internal/migration/testdata/test-audit-distill.yaml`, `phase0-distill.yaml`.
- `internal/distill/distill_model.go:39` says the Go model follows `.claude/skills/distill-lab/scripts/distill.py`.
- Local machine: Python 3 with PyYAML 6.0.3 available (for Phase 7).

## Host integration

- Adapters: `internal/hostintegration/integration.go:34-36` (Claude `.claude/skills/system-curator/SKILL.md`,
  Codex `.agents/skills/system-curator/SKILL.md`, Gemini `.gemini/skills/system-curator/SKILL.md`, same in user scope).
  `SkillRelativePath` used only in `types.go:69,72,95,97` and three tests in the package.
- `prepare()` native skill branch: `integration.go:143-156`; `prepareNativeCurator` `curator-source.go:7-24`;
  ownership `curator-source.go:27-29` (byte equality with the current bundle only); removal on remote/remove
  plans `remote.go:483-498`.
- Any conflict aborts the plan: `integration.go:91-93`. `prepareExactFile`: `integration.go:390-396`.
- Apply re-derives native-skill bytes: `files.go:236-252`; `validateChangePath` one skill path per host:
  `files.go:310-339`; symlinks refused: `files.go:19-35,51`; empty parents pruned: `files.go:222-224`; safe parent
  dirs on write: `files.go:413`.
- Verified MCP-skills delivery: `matrix.go:62-77`; every client is `unverified` for `skills_extension`
  (`docs/mcp-compatibility-matrix.json:72-135`), matrix notes `:153-156`; `internal/hostintegration/matrix.json`
  is identical.
- Current local behavior overwrites unknown curator content after confirmation:
  `internal/hostintegration/doctor-skew_test.go:15-75`.
- Hub vs project vs user roots: `internal/app/workspace.go:465` (`Request{Workspace: root}`),
  `internal/app/host-connections.go:23,26` (project `Root: cwd`, user `Root: home`, cwd==workspace skipped),
  `internal/app/connect_remote.go:159,441` (remote and remove carry no workspace).
- Doctor rendering: `hostChangeItem` `internal/app/workspace.go:501`; warnings `:453`, CLI prints them
  `internal/delivery/cli/workspace.go:116,351`; `machine_file_tracked` fix text `internal/app/workspace.go:169`.

## Workspace Git hygiene

- `internal/workspace/workspace.go:36` `machineFiles`; `:38-41` `requiredIgnoreEntries` (no `.agents`, `.claude/skills`
  or `.gemini/skills` entries); `:172-179` findings; `:185-201` `trackedMachineFiles`; `:433-440` append on init.
  Hub marker: `:75` `.skillhub/schema-version`.
- Suggested stage command adds only canonical folders: `internal/delivery/cli/workspace.go:71-73`.
- `SKILLHUB_WORKSPACE`: `internal/delivery/cli/workspace_env.go:12`, resolution order `workspace_resolve.go:37-49`.

## Product text

- `internal/app/curation_home.go:479-488` (`review_lessons`, `distill.py list <path>`); test `curation_home_test.go:616-637`.
- Status rendering: `internal/delivery/cli/curation.go:97-101`; unknown commands pass through
  `internal/delivery/cli/next_commands.go:156-159`; quoting `shellArgument` `:176-181`.
- `internal/app/source.go:738`, `internal/delivery/cli/source.go:717`; test `internal/delivery/cli/source_test.go:214-215`.
- Web: `web/src/domain/handoff-brief.ts:10-28` (+ `handoff-brief.test.ts:5-17`); `web/src/screens/skill-detail/DistillTab.tsx:10-11`;
  `web/src/screens/distill/DistillHandoffScreen.tsx:24-25` (+ `distill.test.tsx:110,121`); e2e `web/e2e/tier3.spec.ts:45,64`,
  `web/e2e/journeys.spec.ts:229`. Screenshot `docs/images/web/distill-handoff.png` (manifest entry exists).
- Web status type has no workspace path (`web/src/api/types.ts:8-14`), so the handoff names "your Skill Hub folder"
  instead of a path.

## Docs

- `docs/sources-and-learning.md:8,178,183`; `docs/web-ui.md:140,161`; `docs/getting-started.md:217`;
  `docs/user-guide.md:66-72,100`; `docs/remote-hub-guide.md:132,154`; `docs/design/05-curation-lifecycle.md:611,620`;
  `docs/use-cases/02-core-curation-use-cases.md:271-273`, `03-cli-and-curator-mcp-mapping.md:276-277,383` (still true).
  `docs/command-reference.md` and `docs/troubleshooting.md` have no distill mention.

## Host skill loading (official docs, fetched 2026-10-11)

- Claude Code (https://code.claude.com/docs/en/skills): loads `.claude/skills/<name>/SKILL.md` (project, walking
  up to the repo root), `~/.claude/skills/`, enterprise, `--add-dir`, plugins. No mention of `.agents/skills`.
  Skill entries may be symlinks to directories. `${CLAUDE_SKILL_DIR}` is substituted in skill content and in
  `allowed-tools` Bash rules.
- Gemini CLI (https://geminicli.com/docs/cli/skills/): user `~/.gemini/skills/` or the `~/.agents/skills/` alias;
  workspace `.gemini/skills/` or the `.agents/skills/` alias; within a tier `.agents/skills/` takes precedence;
  higher-precedence location wins on name conflicts. Symlinks not documented.
- Codex (https://learn.chatgpt.com/docs/build-skills, redirected from developers.openai.com/codex/skills): reads
  `.agents/skills` from cwd up to the repo root and `$HOME/.agents/skills`; follows symlinked skill folders;
  same-name skills are not merged and both may appear.

## Verified vs not

- Verified in this project: Claude Code (MCP registration, native curator load, server toggle), per the matrix.
- Not verified: Gemini CLI and Codex skill loading in this project; any host following a wrapper's include
  instruction (Phase 7 checks Claude Code by hand).
