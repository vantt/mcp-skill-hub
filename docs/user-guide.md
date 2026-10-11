# Using Skill Hub

This guide is for day-to-day use once your hub works. To install Skill Hub and create your workspace, start with [Get started](getting-started.md).

Inside a connected project, `skillhub` finds your workspace on its own. Elsewhere, pass `--workspace ~/skillhub` to each command, or set `SKILLHUB_WORKSPACE=~/skillhub` in your shell.

## Concepts in plain words

**Workspace.** The Git repository that holds your skills; you create it with `skillhub init`. Git keeps all of it except the secrets you store per skill.

**Skill.** A folder with a `SKILL.md` your agent follows and a small file that says when it applies. Skills sit in collections, such as `core`.

**Skill states.** A skill moves through four states, in order: draft, active, deprecated, archived. A **draft** is a skill agents are not offered yet, and an **active** skill is one agents can be offered. A **deprecated** skill is being phased out; an **archived** one is retired and kept for history.

**Source.** A Git repository linked to one or more skills. An **upstream** is the source a skill was copied from; you can pull its updates. A **learning reference** is a source your curator agent reads for ideas; nothing is copied ([Sources and learning](sources-and-learning.md)).

**Content approval.** Agents receive nothing from a skill copied from someone else's repository until you approve its content. Only you can approve it, in your terminal.

**Preview first.** Commands that change skills show a preview. Nothing is written until you add `--yes` or confirm. How previews are pinned and how state is reported is in the [result envelope contract](contracts/result-envelope.md).

**Nothing commits for you.** Skill Hub writes files but never runs `git commit` or `git push`. `skillhub status` tells you when the workspace has uncommitted changes.

## Set up and connect agents

To create your workspace and connect a first project, follow [Get started](getting-started.md#create-your-workspace). This section covers the choices you have when you connect.

### Connect one project (recommended)

Connecting a project lets the agents you open in that folder ask the hub for skills. Go to the project and preview what `connect` would write:

```bash
cd ~/myproject
skillhub connect --workspace ~/skillhub --host claude
```

```text
Preview only: agent connection for project <home>/myproject was
not written without --yes.
  - Claude Code: will connect (.mcp.json, .claude/settings.local.json, curator skill, CLAUDE.md)
```

Output is trimmed; `<home>` is your home folder. Write the files:

```bash
skillhub connect --workspace ~/skillhub --host claude --yes
```

Without `--host`, `connect` also writes files for Codex and Gemini CLI, which are not yet verified end to end. You can name them too: `--host codex,gemini`. Use `--project <dir>` to connect another folder, then open a new Claude Code session in it.

### Connect every project at once

Connecting your user account makes the hub available in every project, and leaves project folders untouched:

```bash
skillhub connect -g --workspace ~/skillhub --host claude --yes
```

```text
Agent connection for your user account (all projects) written.
  - Claude Code: connected (.claude.json, .claude/settings.json, curator skill, CLAUDE.md)
```

Output is trimmed. `-g` (or `--global`) fits when you want the hub everywhere. Connect per project when you want it in some projects only.

### What gets written

`connect` writes these **connection files**, the files an agent reads to find the hub:

| Scope | Files |
|---|---|
| Project | `.mcp.json`, `.codex/config.toml`, `.gemini/settings.json` (hub registration); `.claude/settings.local.json` (Claude Code permissions); `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` (a short block between `skillhub:bootstrap` comments); the `system-curator` skill under `.claude/skills/`, `.agents/skills/` and `.gemini/skills/` |
| Global (`-g`) | `~/.claude.json`, `~/.codex/config.toml`, `~/.gemini/settings.json` (hub registration); `~/.claude/settings.json` (Claude Code permissions); `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, `~/.gemini/GEMINI.md` (the same block); the `system-curator` skill under `~/.claude/skills/`, `~/.agents/skills/` and `~/.gemini/skills/` |
| Folder access | Each agent may read and write the workspace's `runtime/cache/skills`, `runtime/envs` and `runtime/config`: Claude Code `permissions.additionalDirectories`, Codex `[sandbox_workspace_write] writable_roots`, Gemini CLI `context.includeDirectories` |

`connect` adds missing entries and never removes or reorders your own. Your own text in `CLAUDE.md`, `AGENTS.md` and `GEMINI.md` stays; Skill Hub manages only the marked block. Running `connect` again on a current connection changes nothing.

The registration holds the absolute paths of the `skillhub` program and of your workspace. If you move either, run `connect` again. Older connections are upgraded when you run `connect --yes` or `doctor --fix --yes`.

<details>
<summary>Claude Code permission rules</summary>

In the same permissions file, `connect` adds exactly these rules:

| Permission | Rule |
|---|---|
| allow | `Bash(skillhub:*)` |
| ask | `Bash(skillhub * --yes*)` |
| ask | `Bash(skillhub * confirm *)` |
| deny | `Bash(skillhub * --approve-content*)` |

Claude Code asks you before any `skillhub` command with `--yes` or `confirm`, and blocks `--approve-content`. The first time you open the folder, its trust dialog lists `Bash(skillhub:*)`. Permission-bypass modes do not enforce these rules.

</details>

> [!CAUTION]
> Do not commit connection files. `.mcp.json`, `.claude/settings.local.json`, `.codex/` and `.gemini/` hold absolute paths for this machine. `init` lists them in the workspace's `.gitignore`; add them to a project's `.gitignore` yourself. If your workspace is older, run `skillhub doctor --fix --yes` to add the entries, and `git rm --cached <file>` for any of them Git already tracks. `doctor` names each one it finds.

### Undo a connection

`connect --remove` takes out the Skill Hub entry, the marked block and the `system-curator` skill, unless you changed that skill. Other agent settings stay as they are. Preview it in the connected project:

```bash
skillhub connect --remove
```

Add `--yes` to remove it. Add `-g` for the user-account connection, or `--host claude` for one agent. Your workspace and your skills stay untouched.

## Curate skills

Moved to [Curating skills](curating-skills.md).

## How the agent picks a skill

Your agent is told to ask the hub which skill fits at the start of each new task. Trivial edits skip this; the connection block in `CLAUDE.md` carries that instruction. The hub compares the task with each skill's description, triggers, examples and "not for" entries.

The hub sends back a **recommendation**: the skill that fits, or "no skill fits". Your agent still decides whether to load it.

Only active skills are offered, never drafts. A skill from someone else's repository gives no content until you approve it in your terminal; see [Content approval](#concepts-in-plain-words).

You can run the same lookup by hand. Save this request, which matches the skill from [Get started](getting-started.md#create-your-first-skill), as `request.json`. Keep `scope` inside `task`; without it, the hub answers with a question instead of a skill.

```json
{"schema_version": "1", "request_id": "r1", "operation": "document",
 "task": {"description": "write a commit message for my staged changes",
          "scope": "single_step"}}
```

Ask the hub which skill fits:

```bash
skillhub resolve --request request.json
```

```text
Resolved commit-style (medium confidence). Host approval is required before activation.
```

The second sentence means your agent still decides whether to load the skill. Add `--json` for the full answer. The [request schema](../schemas/skill-resolve-request-v1.schema.json) and the [response schema](../schemas/skill-resolve-response-v1.schema.json) describe both formats.

<details>
<summary>What the agent receives when it loads a skill</summary>

When your agent loads a skill with the `skill_get` tool, the answer includes a `local` block:

- `local.path`: a read-only copy of the skill's files under the workspace's `runtime/cache/skills/<skill-id>@<hash>/`.
- `local.state_directory`: a writable folder under `runtime/envs/` for the skill's packages and caches. It survives edits to `SKILL.md`.
- `local.env`: the variables `SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR` and `SKILLHUB_CONFIG_DIR`.
- `local.status`: for example `review_required` when you have not approved the skill's content yet. The agent then gets no content and is told to ask you to run `skillhub skill review <skill-id>`.
- `local.preflight`: for a skill that declares tools, the check and setup commands to run first. Your agent is told to ask you before it runs setup.

If `local.path` disappears during a long session, your agent is told to call `skill_get` again.

</details>

## Run skills that need tools or secrets

Some skills only need their instructions. Others need programs, API keys or packages, declared in a runtime block (`skillhub skill edit <skill-id> --runtime-file <yaml>`). Skill Hub handles four cases:

| Case | What the skill needs | What happens |
|---|---|---|
| Instructions only | Nothing else | `SKILL.md` is all your agent needs. |
| Programs | Tools on this machine, such as `git` or `ffmpeg` | Your agent is told to run a check first and tell you what is missing. |
| Secrets | Private API keys | You store them per skill with `skillhub skill env set`. |
| Packages | Installed libraries or a virtual environment | Your agent is told to install them into the skill's own `$SKILLHUB_STATE_DIR`, never globally. |

### Store a skill's secrets

Each skill keeps its own secrets, outside Git. Store a value; the command reads it without showing it, or from a pipe:

```bash
skillhub skill env set <skill-id> <KEY>
```

```text
Stored <KEY> for skill <skill-id>.
```

`skillhub skill env list <skill-id>` lists the names you stored, never the values. `skillhub skill env unset <skill-id> <KEY>` removes one.

Values go to `runtime/config/<skill-id>/env` in your workspace, readable only by you (mode `0600`). The `runtime/` folder is ignored by Git. Names starting with `SKILLHUB_` are reserved.

<details>
<summary>Notes for Windows</summary>

The env file uses POSIX `KEY=value` lines with single-quoted values. In PowerShell, read the lines and set `$env:NAME` for each instead of dot-sourcing the file. Node ESM ignores `NODE_PATH`, so install ESM packages inside `$SKILLHUB_STATE_DIR`.

</details>

### Check a skill on this machine

`skill doctor` tells you whether this terminal has what an active skill needs:

```bash
skillhub skill doctor <skill-id>
```

```text
Skill doctor: commit-style
Note: no runtime requirements declared

State: ready
```

It checks the declared programs and their versions and whether each variable is set. For trusted content, it also runs the skill's setup check in its state folder; it never runs setup.

The result is saved as a hint for your agent. Exit codes: `0` ready, `1` setup required or unsupported platform, `2` invalid request or unknown skill.

## Local Web UI

Moved to [The web UI](web-ui.md).

### Do common tasks in the Web UI

Moved to [The web UI: Do common tasks](web-ui.md#do-common-tasks).

## Move to another machine

Your skills live in Git, so moving is a clone. Before you leave the old machine, push the workspace to a remote you own.

If you would rather keep one hub and let agents on other computers use it, see [Use one hub from several machines](remote-hub-guide.md).

> [!IMPORTANT]
> Skill Hub never commits or pushes for you. Commit and push the workspace yourself before you move.

On the new machine, [install Skill Hub](installation.md), then clone the workspace:

```bash
git clone <your-workspace-remote> ~/skillhub
```

A fresh clone has no search index yet. Build it once:

```bash
skillhub rebuild --workspace ~/skillhub
```

Connect your projects again, so the connection files point at the new paths:

```bash
cd ~/myproject
skillhub connect --workspace ~/skillhub --host claude --yes
```

Secrets stored with `skill env set` stay on the old machine. Store them again on the new one.

## Troubleshooting

Moved to [Troubleshooting](troubleshooting.md).

## Command cheat sheet

Moved to [Command reference](command-reference.md).
