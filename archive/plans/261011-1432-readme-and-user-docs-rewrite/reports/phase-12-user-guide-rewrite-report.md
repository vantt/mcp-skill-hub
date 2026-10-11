# User guide rewrite report

`docs/user-guide.md` is now "Using Skill Hub", 249 lines (budget 250), with every required heading and stub.

## Sandbox runs (`SB=/tmp/skillhub-docs-rewrite/user-guide`, global run in `.../user-guide-global`)

- `connect --workspace ~/skillhub` preview (exit 0) and `--yes`: files written in the project, listed with `find`:
  `.mcp.json`, `.claude/settings.local.json`, `.claude/skills/system-curator/SKILL.md`, `.codex/config.toml`,
  `.gemini/settings.json`, `.gemini/skills/system-curator/SKILL.md`, `.agents/skills/system-curator/SKILL.md`,
  `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` (block `skillhub:bootstrap:v1:start/end`). Matches the "Project" row.
  Rerun prints "already current; nothing to change."
- Folder access: `.claude/settings.local.json` `permissions.additionalDirectories`, `.codex/config.toml`
  `[sandbox_workspace_write] writable_roots`, `.gemini/settings.json` `context.includeDirectories`, each with
  `runtime/cache/skills`, `runtime/envs`, `runtime/config`. The four Claude Code rules match the table exactly.
- `connect -g --yes` in a clean HOME: `~/.claude.json`, `~/.claude/settings.json` (same rules and directories),
  `~/.claude/CLAUDE.md`, `~/.claude/skills/system-curator/SKILL.md`, `~/.codex/config.toml`, `~/.codex/AGENTS.md`,
  `~/.agents/skills/system-curator/SKILL.md`, `~/.gemini/settings.json`, `~/.gemini/GEMINI.md`,
  `~/.gemini/skills/system-curator/SKILL.md`. Matches the "Global" row.
- `resolve --request request.json` with `task.scope` (same JSON as getting-started): `Resolved commit-style (medium confidence)`.
- `skill env set` (piped dummy value) / `list` / `unset`: outputs as shown; file `runtime/config/commit-style/env`
  mode `-rw-------`; `SKILLHUB_X` refused ("reserved").
- `skill doctor commit-style`: `State: ready`, exit 0; unknown skill exit 2.
- `git clone` of the workspace, `status` shows "search index missing", `rebuild --workspace <clone>` prints
  "Search index rebuilt.", `skill list` shows the skill; `connect` to the clone works.
- MCP `skill_get` `local` keys observed: `env`, `manifest_version`, `path`, `resources`, `state_directory`,
  `status`; snapshot folder `runtime/cache/skills/commit-style@<16 hex>/` with `SKILL.md` mode `0444`;
  `runtime/envs/commit-style@<16 hex>/` exists. `local.preflight` described from the bootstrap block (the test
  skill has no runtime block).

## Where removed content now lives

| Old paragraph | Now |
|---|---|
| Intro, `skillhub help` pointer | `command-reference.md` intro; `SKILLHUB_WORKSPACE` kept here |
| Concepts: intent-first addition | `curating-skills.md#add-a-skill-skillhub-skill-add` |
| Concepts: diagnostic review | `curating-skills.md` review section |
| Concepts: insight and inbox | removed (commands gone); verified flow in `sources-and-learning.md#learn-from-other-repositories` |
| Concepts: state basis; preview pins, recovery files | one line linking `contracts/result-envelope.md`; recovery detail in `curating-skills.md` |
| Create the workspace (with `git add -A`) | `getting-started.md#create-your-workspace` (safe add list) |
| Global bootstrap block reuse; `--json` `curation_guidance`; `skillhub-curation` entry removal | dropped as internals; replaced by the required sentence on upgrading older connections |
| "this wave", Claude Code version note, plan link (old line 115) | removed |
| Curate skills bullets (status, system-curator CLI preference, source import preview, eval routing) | `curating-skills.md` (start, add, routing sections); `command-reference.md` |
| How the agent picks: `setup_required`, approve command | `troubleshooting.md#skills` |
| Local Web UI, network binding | `web-ui.md#start-the-web-ui`, `#do-common-tasks`, `#keep-it-safe-on-your-network` |
| Troubleshooting (all items) | `troubleshooting.md` H2s; `validate --staged` and hook use in `command-reference.md#workspace-and-git` and `curating-skills.md`; degraded MCP startup dropped (see the troubleshooting report) |
| Command cheat sheet | `command-reference.md` (inbox and insight rows dropped) |

## Notes

- The "What gets written" table now names `.claude/settings.local.json` and `~/.claude/settings.json` explicitly
  (the old table only implied them) and uses "Folder access" instead of "Host Permissions".
- Added: secrets stay on the old machine when moving (verified: `runtime/` is gitignored).
- `docs/curating-skills.md` showed as modified in the worktree by another phase; not touched.

## Unverified

- An agent in a live Claude Code session following the bootstrap instructions (`local.preflight`, asking before
  setup, re-calling `skill_get`); text follows the instruction block `connect` writes.
- Codex and Gemini CLI behavior after `connect` (files written only).
- Windows env-file notes (kept from the old guide).
- `skill doctor` exit code 1 (from `skillhub help skill`; no skill with a runtime block was run).

Status: DONE
Summary: Rewrote docs/user-guide.md as "Using Skill Hub" within budget, with all kept anchors and stubs, and
verified every command and the "What gets written" table in a sandbox.
