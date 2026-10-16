# Using Skill Hub

This guide is for day-to-day use once your hub works. To install Skill Hub, your hub and a first project, start with the [Quickstart](../README.md#quickstart).

Every command acts on the folder you run it in. Inside the hub or a connected project, `skillhub` finds your hub on its own, so this guide writes commands as `cd <folder> && skillhub <command>`.

## Concepts in plain words

**Hub.** The folder that holds your skills, kept in its own Git repository; you create it with `skillhub init`. Git keeps all of it except the secrets you store per skill. The command help also calls it the workspace.

**Skill.** A folder with a `SKILL.md` your agent follows and a small file that says when it applies. Skills sit in collections, such as `core`.

**Skill states.** A skill moves through four states, in order: draft, active, deprecated, archived. A **draft** is a skill agents are not offered yet, and an **active** skill is one agents can be offered. A **deprecated** skill is being phased out; an **archived** one is retired and kept for history.

**Source.** A Git repository linked to one or more skills. An **upstream** is the source a skill was copied from; you can pull its updates. A **learning reference** is a source your curator agent reads for ideas; nothing is copied ([Sources and learning](sources-and-learning.md)).

**Content approval.** Agents receive nothing from a skill copied from someone else's repository, or sent from another machine, until you approve its content. Only a person can approve it: with a `y/N` question in a terminal, or on the web page **Waiting for review**. Your agent cannot.

**Preview first.** Commands that change skills show a preview. In a terminal the command asks `Apply this change? [y/N]` under it; without a terminal (an agent, a script) nothing is written until you add `--yes` or confirm. `--diff` adds the full diff. Setup commands such as `init`, `connect` and `doctor --fix` act at once and print what they changed; add `--dry-run` to see the changes first. How previews are pinned and how state is reported is in the [result envelope contract](contracts/result-envelope.md).

**One change, one commit.** Each change you apply becomes one Git commit in the hub with only its own files, using your hooks and signing. Skill Hub never pushes, apart from `skillhub init` pushing the first commit to an empty repository when you give it one; other machines get a change after you push. `--no-commit` writes the files and skips the commit. `skillhub status` tells you when the hub has uncommitted changes, such as files you edited by hand.

## Set up and connect agents

To install your hub and connect every project, follow the [Quickstart](../README.md#quickstart). This section covers the choices you have when you connect: one project only, or every project at once.

### Connect one project

Connecting a project lets the agents you open in that folder ask the hub for skills. Go to the project and see what `connect` would write:

```bash
cd <your-project> && skillhub connect --dry-run
```

```text
Dry run: agent connection for project
<home>/<your-project>
was not written.
Would change:
  - Claude Code: will connect (.mcp.json, .claude/settings.local.json, system-curator skill)
  - Codex: will connect (.codex/config.toml)
  - Gemini CLI: will connect (.gemini/settings.json)
  - Agent Skills folder (.agents/skills): will connect (system-curator skill)
```

Output is trimmed; `<home>` is your home folder. Write the files:

```bash
cd <your-project> && skillhub connect
```

`connect` uses the hub that `skillhub init` set up last on this machine. To use another hub on this machine, name its folder: `skillhub connect <path>`. Codex and Gemini CLI are not yet verified end to end; to connect only Claude Code, add `--host claude`. Then open a new Claude Code session in the project.

### Connect every project at once

Connecting your user account makes the hub available in every project, and leaves project folders untouched:

```bash
skillhub connect -g --host claude
```

```text
Connected your user account (all projects) to the hub at
<home>/skill-hub.
Agent connection written: claude-code.
  - Claude Code: connected (.claude.json, .claude/settings.json, system-curator skill)
  - Agent Skills folder (.agents/skills): connected (system-curator skill)
```

Output is trimmed. `-g` (or `--global`) fits when you want the hub everywhere. Connect per project when you want it in some projects only.

### What gets written

`connect` writes these **connection files**, the files an agent reads to find the hub:

| Scope | Files |
|---|---|
| Project | `.mcp.json`, `.codex/config.toml`, `.gemini/settings.json` (hub registration); `.claude/settings.local.json` (Claude Code permissions); the `system-curator` skill in `.agents/skills/system-curator/` and its pointer file `.claude/skills/system-curator/SKILL.md` |
| Global (`-g`) | `~/.claude.json`, `~/.codex/config.toml`, `~/.gemini/settings.json` (hub registration); `~/.claude/settings.json` (Claude Code permissions); the `system-curator` skill in `~/.agents/skills/system-curator/` and its pointer file `~/.claude/skills/system-curator/SKILL.md` |
| Folder access | Each agent may read and write the hub's `runtime/cache/skills`, `runtime/envs` and `runtime/config`: Claude Code `permissions.additionalDirectories`, Codex `[sandbox_workspace_write] writable_roots`, Gemini CLI `context.includeDirectories` |

Skill Hub writes its skills once, in the shared Agent Skills folder `.agents/skills/`, which Codex and Gemini CLI read directly (Gemini CLI from v0.28.0). Claude Code reads only `.claude/skills/`, so it gets a short pointer file there that tells it to read the skill in `.agents/skills/`. Nothing is written under `.gemini/skills/`; `doctor --fix` removes an old copy there from an earlier version, unless you changed it. In the hub folder itself, `init` also writes the `distill-lab` skill the same way ([Sources and learning](sources-and-learning.md#hand-changes-to-your-curator-agent)). These generated skill folders stay out of Git through entries in the hub's `.gitignore`, which `init` adds, or `doctor --fix` in an older hub. A skill file you changed is never overwritten or deleted: Skill Hub skips that one file and names it.

Claude Code follows the pointer file through `${CLAUDE_SKILL_DIR}`, as its [skills documentation](https://code.claude.com/docs/en/skills) describes; Codex and Gemini CLI reading `.agents/skills/` rests on their own documentation. With the user-account pointer (`~/.claude/skills/` to `~/.agents/skills/`), Claude Code is expected to ask for permission the first time it reads `~/.agents/skills/` from another project in its default permission mode, because that folder lies outside the project; see its [permissions documentation](https://code.claude.com/docs/en/permissions).

`connect` adds missing entries and never removes or reorders your own. `connect` writes no instruction files: the protocol reaches your agent through the MCP server itself (see [How the agent picks a skill](#how-the-agent-picks-a-skill)). If an earlier version left a Skill Hub block in `CLAUDE.md`, `AGENTS.md` or `GEMINI.md`, `doctor --fix` removes it; Skill Hub only removes its old marked block, and your own text stays byte-for-byte. Running `connect` again on a current connection changes nothing.

The registration holds the absolute paths of the `skillhub` program and of your hub. If you move the program, run `connect` again. If you move the hub, run `cd <new-location> && skillhub init` there, then `skillhub doctor --fix` in each project. Older connections are upgraded when you run `connect` or `doctor --fix`.

<details>
<summary>Claude Code permission rules</summary>

In the same permissions file, `connect` adds exactly these rules:

| Permission | Rule |
|---|---|
| allow | `Bash(skillhub:*)` |
| ask | `Bash(skillhub * --yes*)` |
| ask | `Bash(skillhub * confirm *)` |
| ask | `Bash(skillhub *init*)` |
| ask | `Bash(skillhub *connect*)` |
| ask | `Bash(skillhub *--fix*)` |
| ask | `Bash(skillhub *token rotate*)` |
| ask | `Bash(skillhub *cache clear*)` |
| ask | `Bash(skillhub *serve*)` |
| deny | `Bash(skillhub * --approve-content*)` |

Claude Code asks you before any `skillhub` command with `--yes` or `confirm`, and before `init`, `connect`, `doctor --fix`, `token rotate`, `cache clear` and `serve`, wherever their flags stand. So an agent can preview a `skill commit`, and you see the prompt when it applies the commit with `--yes`. It blocks `--approve-content`, so an agent never approves content. Codex and Gemini CLI have no such rules, so they do not ask first. Connections made by an earlier release lack the newer rules; `skillhub doctor` reports them and `skillhub doctor --fix` adds them. The first time you open the folder, its trust dialog lists `Bash(skillhub:*)`. Permission-bypass modes do not enforce these rules.

`connect` notes which of these rules it added itself, in `permission-records.json` in your Skill Hub user configuration folder (not in Claude's settings file). `connect --remove` takes out only those, so a rule you already had, such as your own `Bash(skillhub:*)`, stays. `doctor --fix` notes the rules it adds the same way. A connection made before this record existed has none, and `connect --remove` then takes out every rule in the table.

</details>

> [!CAUTION]
> Do not commit connection files. `.mcp.json`, `.claude/settings.local.json`, `.codex/` and `.gemini/` hold absolute paths for this machine. `init` lists them in the hub's `.gitignore`; add them to a project's `.gitignore` yourself. If your hub is older, run `skillhub doctor --fix` to add the entries, and the one `git rm --cached -- <files>` command that `doctor` prints for the files Git already tracks.

### Undo a connection

`connect --remove` takes out the Skill Hub entry, an old marked block if one is left, and the Skill Hub skills, unless you changed them. Other agent settings stay as they are. In a project, text you wrote in `CLAUDE.md`, `AGENTS.md` or `GEMINI.md` stays, and a file that held only an old block is deleted. Run in the hub folder itself, it leaves those three files alone, because the hub commits them as its own content; `doctor --fix` restores the rest of the hub's connection. Run it in the connected project:

```bash
cd <your-project> && skillhub connect --remove
```

It also removes the Claude Code permission rules that `connect` adds (listed above) and the hub's folders it allowed. It cannot tell a rule `connect` added from an identical one you wrote yourself first, so a rule you had added by hand, such as `Bash(skillhub:*)`, goes with them: the dry-run names every rule, and you can add yours back afterwards. Every other rule stays.

Add `--dry-run` to see what it would remove first. Add `-g` for the user-account connection, or `--host claude` for one agent. Your hub and your skills stay untouched.

## Curate skills

Moved to [Curating skills](curating-skills.md).

## How the agent picks a skill

Your agent is told to ask the hub which skill fits at the start of each new task. Trivial edits skip this; the Skill Hub MCP server sends that instruction when your agent connects (MCP server instructions). Claude Code, Codex CLI and Gemini CLI read it (Gemini CLI loads MCP servers, and so their instructions, only in trusted folders); Cursor's behaviour is not documented. A client that ignores server instructions gets the tools without the protocol, and there is no fallback. The hub compares the task with each skill's description, triggers, examples and "not for" entries.

The hub sends back a **recommendation**: the skill that fits, or "no skill fits". Your agent still decides whether to load it.

Only active skills are offered, never drafts. A skill from someone else's repository gives no content until you approve it in your terminal; see [Content approval](#concepts-in-plain-words).

You can run the same lookup by hand. Save this request, which matches the skill from [Create your first skill](curating-skills.md#create-your-first-skill), as `request.json`. Keep `scope` inside `task`; without it, the hub answers with a question instead of a skill.

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

- `local.path`: a read-only copy of the skill's files under the hub's `runtime/cache/skills/<skill-id>@<hash>/`.
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

Values go to `runtime/config/<skill-id>/env` in your hub, readable only by you (mode `0600`). The `runtime/` folder is ignored by Git. Names starting with `SKILLHUB_` are reserved.

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

## Keep the hub on GitHub

Create an empty repository on GitHub, without a README or license file. Then give its URL to `init` in your hub:

```bash
cd ~/skill-hub && skillhub init <git-url>
```

What `init` does depends on the repository:

- **It is empty:** `init` pushes the hub there and sets it as `origin`. This is the only push Skill Hub ever makes for you.
- **It already holds this hub:** `init` sets it as `origin` and pushes nothing.
- **It holds anything else:** `init` refuses and changes nothing.

After that, Skill Hub commits each change you confirm; you push with Git yourself when you want a backup or want other machines to get it.

## Move to another machine

Your skills live in Git, so moving is a fetch. Before you leave the old machine, push the hub to a remote you own. [Keep the hub on GitHub](#keep-the-hub-on-github) shows how `init` sets one up.

If you would rather keep one hub and let agents on other computers use it, see [Use one hub from several machines](remote-hub-guide.md).

> [!IMPORTANT]
> Skill Hub commits each confirmed change but never pushes. Push the hub yourself before you move, and commit any file you edited by hand (`skillhub status` lists them).

On the new machine, [install Skill Hub](installation.md), then fetch the hub into an empty folder. `init` builds its search index and makes it the hub of this machine:

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <git-url>
```

Connect your projects again, so the connection files point at the new paths:

```bash
cd <your-project> && skillhub connect
```

Secrets stored with `skill env set` stay on the old machine. Store them again on the new one.

## Troubleshooting

Moved to [Troubleshooting](troubleshooting.md).

## Command cheat sheet

Moved to [Command reference](command-reference.md).
