# Skill Hub user guide

This guide explains how to use Skill Hub day to day. If you have not set it up yet, follow the Quickstart in the [README](../README.md) first.

For collecting, creating, reviewing, editing, and improving skills, use the dedicated [curating skills guide](curating-skills.md).

Run `skillhub help` for the command list and `skillhub help <command>` for the flags and an example of each command. Connected projects automatically resolve the workspace from their configuration. For standalone CLI use outside connected projects, you can export the workspace location once:

```bash
export SKILLHUB_WORKSPACE=~/skillhub
```

Without it, pass `--workspace ~/skillhub` to each command, or run the command from inside the workspace folder.

## Concepts in plain words

**Workspace.** A Git repository that holds your skills, your sources, and a history of changes. You create it with `skillhub init`. It is the only thing you need to back up.

**Skill.** A folder with a `SKILL.md` (the instructions your agent follows) and a small metadata file (when the skill applies and when it does not). Skills are grouped in collections such as `software`.

**Skill states.** A skill moves through four states:

- `draft`: being written or newly imported. The agent does not use it yet.
- `active`: ready. The agent may be recommended it.
- `deprecated`: still stored, being phased out.
- `archived`: retired, kept for history.

Transitions go in order: draft to active to deprecated to archived. You cannot jump from active straight to archived.

**Source.** A Git repository (for example on GitHub) that you watch for useful skills. You first save it as a candidate, then decide whether to watch it. Watching a source never changes your skills by itself.

**Source import.** You can import existing skills from a watched source directly into your workspace as drafts (`skillhub source import`). They are never auto-activated; you review and activate them when ready.

**Insight and inbox.** When a source changes, the agent reads the changes and proposes ideas for your skills. These proposals are called insights and wait in the inbox. Nothing is applied until you say so.

**Preview and confirmation.** Skill creation, edits, lifecycle transitions, imports, and insight application use preview followed by explicit confirmation. CLI commands that support `--yes` generate, validate, and apply a fresh proposal; not every mutation accepts `--yes`. Source capture and `triage` decisions to defer or reject apply immediately. See the [curation safety model](curating-skills.md#safety-model) before changing skills.

**Search index (catalog).** A search index Skill Hub builds from your workspace files so lookups are fast. It is rebuilt automatically when you run read commands if the index is stale and files are valid, or you can rebuild manually at any time (`skillhub rebuild`). Your files are the truth; the search index is disposable.

**Nothing commits for you.** Skill Hub writes files but never runs `git commit` or `git push`. You decide when to commit. `skillhub status` reminds you when there are uncommitted changes with the exact git commit command.

## Set up and connect agents

### Create the workspace

```bash
skillhub init ~/skillhub          # preview
skillhub init ~/skillhub --yes    # create
git -C ~/skillhub add -A
git -C ~/skillhub commit -m "Initialize Skill Hub workspace"
```

`init` requires an empty directory (or a non-existent path). If run on a non-empty directory that is not already a workspace, `init` refuses with a message suggesting a dedicated path (override with `--force` only if intended). `init` also writes agent connection files inside the workspace, so an agent opened in `~/skillhub` itself already works.

### Connect one project (recommended)

```bash
cd your-project
skillhub connect --workspace ~/skillhub          # preview
skillhub connect --workspace ~/skillhub --yes    # write files
```

By default `connect` acts on the current folder. Use `--project <dir>` to name another folder, and `--host claude`, `--host codex`, or `--host gemini` to limit it to one agent (repeat the flag or separate names with commas).

Then restart your agent so it loads the new server. Connected projects auto-resolve their workspace on subsequent commands without requiring `--workspace` or `SKILLHUB_WORKSPACE`.

### Connect every project at once

```bash
skillhub connect -g --workspace ~/skillhub --yes
```

`-g` (or `--global`) registers the hub in your user account, so every project sees it and you do not touch project folders. Use it when you want the hub everywhere. Use the per-project form when you want the hub in some projects only, or want the connection files visible in the project.

### What gets written

| Scope | Files |
|---|---|
| Project | `.mcp.json`, `.codex/config.toml`, `.gemini/settings.json` (server registration); `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` (a short block marked by `skillhub:bootstrap` comments); the `system-curator` skill under `.claude/skills/`, `.agents/skills/`, and `.gemini/skills/` |
| Global (`-g`) | `~/.claude.json`, `~/.codex/config.toml`, `~/.gemini/settings.json`; `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, `~/.gemini/GEMINI.md`; the `system-curator` skill under `~/.claude/skills/`, `~/.agents/skills/`, and `~/.gemini/skills/` |

The registration stores the absolute path of the `skillhub` binary and of your workspace. If you move either, run `skillhub connect` again.

Existing `CLAUDE.md`, `AGENTS.md`, and `GEMINI.md` files keep your own text. Skill Hub only manages the marked block.

> **Caution on committing connection files:** Project connection files (`.mcp.json`, `.codex/config.toml`, `.gemini/settings.json`) contain machine-specific absolute paths to your local binary and workspace. We recommend adding `.mcp.json`, `.codex/`, and `.gemini/` to your project's `.gitignore` rather than committing them to shared repositories. Similarly, do not commit connection files that embed machine-specific paths into your canonical skills repository.

### Undo a connection

Delete what `connect` wrote:

- the `skillhub` entry in the registration files above,
- the block between the `skillhub:bootstrap` start and end comments in the instruction files,
- the `system-curator` skill folders.

Your workspace and skills are untouched.

## Curate skills

Skill curation has its own task-oriented guide:

Turning source changes into insights (`skillhub distill ...`) is designed for the agent to run. The agent stops at proposals and states that active skills were not changed. To apply an insight, ask your agent: "Show me the top idea in my inbox," then approve it. The CLI form is `skillhub insight show|decide|apply|confirm` (see `skillhub help insight`).

## How the agent picks a skill

The connection block tells your agent to call the hub's `skill_resolve` tool at the start of each new substantial task. It does not do this for trivial edits. The hub compares the task with skill descriptions, triggers, and "not for" entries in the catalog and answers with a recommendation, or with "no skill fits".

The answer is only a recommendation. Your agent host still decides whether to load the skill. Only active skills are candidates; a draft is not recommended.

You can try the same lookup by hand. Write a request file and run it:

```bash
echo '{"schema_version":"1","request_id":"r1","task":{"description":"review the retry policy for reliability"},"operation":"review"}' > request.json
skillhub resolve --request request.json
```

The request format is [schemas/skill-resolve-request-v1.schema.json](../schemas/skill-resolve-request-v1.schema.json). Add `--json` for the full response. If the request is ambiguous, `resolve` provides guidance on how to refine it.

## Move to another machine

Your skills live in Git, so moving is a clone:

```bash
git clone <your-workspace-remote> ~/skillhub
skillhub rebuild --workspace ~/skillhub
cd your-project
skillhub connect --workspace ~/skillhub --yes    # or add -g
```

Push the workspace to a remote first. Skill Hub never pushes for you. Install the binary on the new machine as described in the README.

## Troubleshooting

**Something feels wrong.** Run:

```bash
skillhub doctor
```

`doctor` checks the workspace, the current project's connection, and global connections. If it lists repairs, preview and apply them:

```bash
skillhub doctor --fix
skillhub doctor --fix --yes
```

**Host approval and trust prompts after connect.**
When connecting Skill Hub to agent hosts, each host may prompt for permission on initial launch:
- **Claude Code:** May display an approval prompt when connecting to a new stdio MCP server command. Press `y` or accept to permit `skillhub mcp serve`.
- **Codex / Gemini CLI:** Look for host trust prompts regarding newly added MCP tools or instruction files. Approve the `skillhub` server to allow tool resolution.

**Windows SmartScreen or unsigned binary warnings.**
Skill Hub binaries are checksum-verified and Sigstore-attested via GitHub CI. On Windows, Windows SmartScreen or antivirus may flag newly downloaded unsigned executables. Choose "More info" -> "Run anyway" if prompted, or verify the file's SHA-256 against `checksums.txt` published with the release.

**`connect -g` refuses because of a symbolic link.**
The error states that a host integration path contains a symbolic link (for example, if `~/.claude` or `~/.agents` is symlinked). Skill Hub refuses to write through symlinks for security. Workarounds:
1. Connect per project instead: `cd your-project && skillhub connect --workspace ~/skillhub --yes`
2. Specify only unaffected hosts: `skillhub connect -g --host codex,gemini --workspace ~/skillhub --yes`
3. Replace the symlink with a real directory.

**The search index is stale.**
If you edit workspace files by hand, Skill Hub read commands (`skill list`, `skill show`, `status`) will automatically attempt an offline rebuild if canonical files validate. If validation fails or you want to rebuild manually, run:

```bash
skillhub validate    # shows exact file:line errors with fixes
skillhub rebuild
```

**Telemetry is degraded.**
`skillhub telemetry health` reports an error such as "file is not a database". Telemetry is a disposable local record of usage. Discard and recreate it:

```bash
skillhub telemetry purge --yes
```

**"no workspace found".**
Connected projects auto-resolve the workspace. Outside a connected project, pass `--workspace <path>`, set `SKILLHUB_WORKSPACE`, or run the command inside the workspace directory.

**The agent does not see the hub.**
Restart the agent after `connect`. Check that the `skillhub` binary still exists at the path recorded in the registration file. If you moved the binary or workspace, run `skillhub connect` again.

## Command cheat sheet

| Task | Command |
|---|---|
| Create a workspace | `skillhub init <path> [--force] --yes` |
| Connect a project | `skillhub connect [--workspace <path>] --yes` |
| Connect all projects | `skillhub connect -g --workspace <path> --yes` |
| Health and next step | `skillhub status` |
| Diagnose and repair | `skillhub doctor [--fix [--yes]]` |
| List skills | `skillhub skill list [--state <state>]` |
| Read a skill (any state) | `skillhub skill show <id>` |
| Create a draft | `skillhub skill create --id ... --collection ... --name ... --description ... [--yes]` |
| Edit | `skillhub skill edit <id> [--description ...] [--editor] [--yes]` |
| Change state | `skillhub skill activate\|deprecate\|archive <id> [--yes]` |
| Save a source | `skillhub source capture <url> --reason <text>` |
| List sources | `skillhub source list` |
| Inspect a source | `skillhub source triage <id> --decision accept --source-id <name> [--path <subdir>] --adapter git` |
| Confirm a proposal | `skillhub source confirm --proposal ... --proposal-digest ... --base-version ...` |
| Import draft skills from source | `skillhub source import <source-id> [--path <subdir>] [--skill <name>] [--yes]` |
| Check upstream | `skillhub check --all-due` or `--all` |
| Review the inbox | `skillhub inbox` |
| Act on an insight | `skillhub insight show\|decide\|apply\|confirm` |
| Update binary in place | `skillhub update [--yes]` |
| Ask for a skill | `skillhub resolve --request <file>` |
| See uncommitted changes | `skillhub diff` |
| Validate files | `skillhub validate` |
| Rebuild the search index | `skillhub rebuild` |
| Migrate the file format | `skillhub migrate [--to <n>] [--yes]` |
| Telemetry | `skillhub telemetry health\|export\|purge --yes` |
| Version | `skillhub version` |

Most commands accept `--workspace <path>` and `--json`.
