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
- `distill.py:24` `import yaml` (unguarded); `:194-201` `_git` runs `git -C <hub>/runtime/distill-lab/mirrors/<src>`
  with no config limits; `:183` reads files with `git show`; `:273-278` canonical header "Written by
  distill-lab/scripts/distill.py ..."; `:288` `yaml.safe_load`; `:293-295` `hub_of` = `parents[4]` of the file
  (the script has no personal path; only SKILL.md:292 does); `:328-330` fixed temp name `<file>.yaml.tmp`, written
  through symlinks. PyYAML writes `1e23456` unquoted (YAML 1.1 floats need a dot); js-yaml `CORE_SCHEMA` reads it
  as a float (red team, 2026-10-11).
- `SKILL.md:386-388` mirrors are partial clones with a working tree inside the hub; the hub also holds secret env
  files under `runtime/config/` and `runtime/envs/` (`internal/hostintegration/runtime_access.go:12-23`).
- Header copies in fixtures: `internal/migration/testdata/test-audit-distill.yaml`, `phase0-distill.yaml`.
- `internal/distill/distill_model.go:39` says the Go model follows `.claude/skills/distill-lab/scripts/distill.py`.
- Local machine: Python 3 with PyYAML 6.0.3 available (used only for the differential run in Phase 2) and
  Node v24.21.0.
- Writer details that matter for a port (`distill.py:212-279`): custom `SafeDumper`, flow `decision` and
  `score.facts`, folded prose over 60 characters, `width=88`, `allow_unicode`; `final_score` is a Python float
  (`45.0` in the fixtures) from `round(..., 2)`; `decide` sets `at` to 12 characters of the cursor (`:366-397`);
  the evidence check counts lines with `splitlines()` (`:166-192`). Only `check`/`format` without `--no-where`
  use the hub (`:307-312`).
- The repo `.gitignore` ignores `.agents/`, `.claude/`, `.gemini/`; the dev copy is tracked because it was added
  explicitly. `.agents/skills/system-curator/` exists locally (written by `connect`, untracked). No `.skillhub/`
  in the repo root.

## Onboarding plan (read-only, checked 2026-10-11)

- `.claude/worktrees/agent-a5c101367df3589e1/plans/261011-2250-onboarding-ux-redesign/plan.md`: `DetectRole` (its
  phase 2) feeds status and doctor; its phase 5 deletes `docs/getting-started.md`. Everything in this file that
  cites pre-0.5.0 lines is re-verified at implementation start (plan.md "Re-verify at start").
- Contract change from that plan's owner (mcp-skill-hub-cb), 2026-10-11, superseding its earlier `.git/info/exclude`
  design: no local-exclude helper; the hub keeps the `.gitignore` mechanism on main since `b734bc9`
  (`requiredIgnoreEntries`, fixable `gitignore_entry_missing`, `machine_file_tracked`; `init` appends missing
  entries). The hidden stdio launcher keeps the name `mcp serve` (no rename, no registration migration).
  <!-- Validation: onboarding extension point --> Its extension point is named in its phase-04:33-63:
  `internal/app/health.go`, `CheckProvider`, `Finding{Code, Label, Severity ok|notice|problem|skipped, Message,
  Fix *FixAction, Manual}`, `RegisterProvider`; its plan.md:208 and phase-04:62-63 expect this plan to register
  integrity there too (answered in plan.md decision 10). The connection summary `connection written: claude-code,
  codex-cli, gemini-cli` comes from its `connectionSummaryLine` (phase-02:147-148, plan.md:210).
- <!-- Validation: release version --> The release that carries the onboarding redesign is 0.5.0 (tags are `v0.x`;
  latest stable `v0.4.1`, latest tag `v0.5.0-alpha.1`).

## Node and js-yaml (checked 2026-10-11)

- Prior art: `.claude/skills/distill/scripts/distill.mjs` imports a vendored js-yaml 4.3.2
  (its own `scripts/vendor/js-yaml.mjs`, an existing path of that other skill, 104205 bytes, header line added and source-map line removed, so not
  byte-identical to the published file; `js-yaml.LICENSE` equals the published LICENSE).
- npm registry: js-yaml 4.3.2 published 2026-08-26, MIT, `exports["."].import = ./dist/js-yaml.mjs`, no `engines`,
  no provenance attestation; tarball `https://registry.npmjs.org/js-yaml/-/js-yaml-4.3.2.tgz`, `dist.integrity`
  `sha512-SFNOvSJ+Dgf/9An904Yx+CgSlIPCkIpao4qo51lpee25TIRejdH3rhR4EZMGoNx3/TP3O+wzWuiTFl4sqbltzA==`, `dist.shasum`
  `8e44fb14a2643c59726bb15787b5f1512cb3d3fb` (`npm view`, 2026-10-11);
  `dist/js-yaml.mjs` 104118 bytes, sha256 `cea276c7e15f409a1adbe5d177aba7824398a474f7cf702bd57962c7d570636f`,
  no `import` statements; `LICENSE` 1084 bytes, sha256 `a07bc24468b9654ce76a547d47a2db282d07733b715db4c73a98bd63961f9550`.
  Latest 5.4.3 (2026-10-05) depends on `argparse`, and so does 4.3.2 (`^2.0.1`, for its command-line tool); 4.3.2
  is the `v4-legacy` dist-tag. <!-- Validation: js-yaml facts -->
- Node ESM docs (https://nodejs.org/api/esm.html): unflagged in v12.17.0/v13.2.0, stable in v14.17.0/v15.3.0,
  `node:` imports from v14.13.1/v12.20.0.
- <!-- Validation: runner images --> GitHub runner images (actions/runner-images README and image readmes, images
  dated 2026-10-04 / 2026-09-07):

  | Label | Image | Default Node | Cached Node |
  |---|---|---|---|
  | `ubuntu-latest` | Ubuntu 24.04 | 22.23.3 | 22.23.3, 24.21.0 |
  | `windows-latest` | Windows Server 2025 | 22.23.3 | 22.23.3, 24.21.0 |
  | `macos-latest` | macOS 26 arm64 | 24.20.0 | 22.23.2, 24.20.0 |

  None ships Node 18, so `actions/setup-node` with `node-version: 18` is required. CI `verify` job runs `go test ./...` on all three without a Node setup step
  (`.github/workflows/ci.yml:11-57`); `release.yml` "Verify release candidate" does the same
  (`.github/workflows/release.yml:100-168`, `go test -timeout 30m ./...` at `:150`); the web job uses
  `actions/setup-node` with `web/.nvmrc` (24) (`ci.yml:147`); the `lint` job checks out with `fetch-depth: 0`
  (`ci.yml:77-79`). Release tags today: `v0.1.0` to `v0.5.0-alpha.1`, each holding the curator file.
- Notices: `web/THIRD_PARTY_NOTICES.md` is generated by `web/scripts/notices.mjs` from the web lockfile and copied
  into releases (`.github/workflows/release.yml:225,407,473`). No other notices file.
- Go reader for `distill.yaml`: `internal/distill/distill_model.go:291` (`UnmarshalDocument`), `:120`
  (`ValidateDocument`); fixture readers `internal/distill/distill_model_test.go:13`, `internal/migration/v4_test.go:35`,
  `internal/delivery/web/routes_distill_test.go:34`.

## Host integration

- Adapters: `internal/hostintegration/integration.go:34-36` (Claude `.claude/skills/system-curator/SKILL.md`,
  Codex `.agents/skills/system-curator/SKILL.md`, Gemini `.gemini/skills/system-curator/SKILL.md`, same in user scope).
  `SkillRelativePath` used only in `types.go:69,72,95,97`, the adapter literals at `integration.go:34-36`, and
  three tests in the package (`curator-source_test.go:34`, `doctor-skew_test.go:23,37,69`, `remote_test.go:196,214`).
  <!-- Validation: SkillRelativePath callers -->
- `prepare()` native skill branch: `integration.go:143-156`; `prepareNativeCurator` `curator-source.go:7-24`;
  ownership `curator-source.go:27-29` (byte equality with the current bundle only); removal on remote/remove
  plans `remote.go:483-498`.
- Any conflict aborts the plan: `integration.go:91-93`. `prepareExactFile`: `integration.go:390-396`.
- Inspection is grouped by host (`types.go:141-154`). Consumers that assume a host per file: kind lists
  `integration.go:87`, `internal/app/workspace.go:485`; conflict pre-checks `internal/app/connect.go:53-67`,
  `internal/app/connect_remote.go:196-211`, `internal/app/workspace.go:377-392`; `levels[...]`
  `connect.go:79,94`, `connect_remote.go:226,244`, `workspace.go:403,450` (maps built at `connect.go:52`,
  `connect_remote.go:196`, `workspace.go:377,437`); item ID `workspace.go:501-503`, parsed by
  `internal/delivery/cli/connect_remote.go:123` (`hostChangeLines`), `internal/delivery/cli/connect.go:120` (`renderConnectResult`),
  `internal/delivery/cli/workspace.go:47` (init summary), compared exactly in `connect.go:104-105`
  (`connectCurationGuidance`); `formatChangeKind` hard-codes "curator skill" (`connect.go:194-195`). Apply's kind
  rank: `files.go:289-302`, checked at `:158`. `DoctorFix` gate `internal/app/workspace.go:228-230`;
  `applyHostIntegration` `:418-456`. Status schema `schemas/curation-home-v1.schema.json` (`actionItem`
  `additionalProperties: false`, `:59-65`), checked by `internal/delivery/cli/status_schema_test.go:27`.
  <!-- Validation: consumer citations --> Tests/goldens pinning the three copies outside the package:
  `internal/app/workspace_test.go:337-346`, `internal/delivery/cli/onboarding_test.go:293`,
  `connect_regression_test.go:96-100` + `testdata/connect-workspace/{project,global}-files.golden`,
  `connect_remote_test.go:630`. `scopeRegistration` returns remote registrations too (`remote.go:156-164`;
  `connection.go:27` `Remote()`). MCP hides the curator from identified Gemini sessions by name only
  (`internal/delivery/mcpserver/server.go:399-411`). `MissingFinding` (`types.go:241-254`) via `CheckMissing` in
  status (`internal/app/curation_home.go:151-157`). Doctor stops before host repair on a non-fixable finding
  (`internal/app/workspace.go:266-270`); `applyExternalConnections` drops applied results on error
  (`internal/app/host-connections.go:66-73`).
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
  or `.gemini/skills` entries); `:172-179` findings (`gitignore_entry_missing` fixable, `machine_file_tracked` not);
  `:185-201` `trackedMachineFiles`; `:425-440` `desiredGitignore` appends missing entries. Hub marker: `:75`
  `.skillhub/schema-version`. `git check-ignore` does not report tracked paths, so tracked files need `git ls-files`.
- Suggested stage command adds only canonical folders: `internal/delivery/cli/workspace.go:71-73`.
- `SKILLHUB_WORKSPACE`: `internal/delivery/cli/workspace_env.go:12`, resolution order `workspace_resolve.go:37-49`.

## Product text

- `internal/app/curation_home.go:479-488` (`review_lessons`, `distill.py list <path>`, path unquoted, from an
  unchecked glob at `:328`); `ActionItem` `:46-53`; test `curation_home_test.go:616-637`. The same home is the MCP
  tool `hub_status` (`internal/delivery/mcpserver/workspace_tools.go:30-32`). `shellArgument` quotes POSIX-style
  only (`internal/delivery/cli/next_commands.go:176-181`). Canonical id pattern `internal/canonical/skill.go:22`.
  `internal/distill/distill_model.go:39,119,184` name `distill.py`.
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
  `docs/command-reference.md` has no distill-lab mention (`:65` mentions lessons and "Distill with Curator Agent");
  `docs/troubleshooting.md` has no distill mention. <!-- Validation: command-reference citation -->

## Host skill loading (official docs, fetched 2026-10-11)

- Claude Code (https://code.claude.com/docs/en/skills): loads `.claude/skills/<name>/SKILL.md` (project, walking
  up to the repo root), `~/.claude/skills/`, enterprise, `--add-dir`, plugins. No mention of `.agents/skills`.
  Skill entries may be symlinks to directories. `${CLAUDE_SKILL_DIR}` is substituted in skill content and in
  `allowed-tools` Bash rules.
- <!-- Validation: host docs --> Claude Code permissions (https://code.claude.com/docs/en/permissions): read-only
  reads need no approval only "within the working directory and additional directories", so a user-scope wrapper
  reading `~/.agents/skills/` from another project is expected to prompt in Manual mode.
- Gemini CLI reads `.agents/skills` from v0.28.0 (PR #18151, merged 2026-02-03; v0.28.0 stable 2026-02-10).
  Git 2.45.0 first has `--no-lazy-fetch` and documents `GIT_NO_LAZY_FETCH`.
- Gemini CLI (https://geminicli.com/docs/cli/skills/): user `~/.gemini/skills/` or the `~/.agents/skills/` alias;
  workspace `.gemini/skills/` or the `.agents/skills/` alias; within a tier `.agents/skills/` takes precedence;
  higher-precedence location wins on name conflicts. Symlinks not documented.
- Codex (https://learn.chatgpt.com/docs/build-skills, redirected from developers.openai.com/codex/skills): reads
  `.agents/skills` from cwd up to the repo root and `$HOME/.agents/skills`; follows symlinked skill folders;
  same-name skills are not merged and both may appear.

## Verified vs not

- Verified in this project: Claude Code (MCP registration, native curator load, server toggle), per the matrix.
- Not verified: Gemini CLI and Codex skill loading in this project; any host following a wrapper's include
  instruction (Phase 8 checks Claude Code by hand).
