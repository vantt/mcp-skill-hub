# Get started with Skill Hub

In about 10 minutes you will have your own skill library, connected to a project in Claude Code. You will write one skill and see the hub recommend it for a matching task.

You need Git, a terminal, and Claude Code, your **agent**: the AI coding tool you work with. Skill Hub is verified with Claude Code. It also writes setup files for Codex and Gemini CLI, which are not yet verified end to end.

## Install Skill Hub

Skill Hub is one program, `skillhub`. Run the one-liner for your platform. [Installation](installation.md) covers versions, signatures and building from source.

Linux and macOS:
```bash
curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh
```

Windows (PowerShell):
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex"
```

The installer places `skillhub` in `~/.local/bin` (`%LOCALAPPDATA%\skillhub\bin` on Windows) and adds it to your `PATH`. Open a new terminal, then check that the command works:

```bash
skillhub version
```

<details>
<summary>Notes for Windows</summary>

This guide writes paths such as `~/skillhub` and `~/myproject`. In PowerShell, use `$HOME\skillhub` and `$HOME\myproject` instead. Set the workspace variable with `$env:SKILLHUB_WORKSPACE = "$HOME\skillhub"`.

</details>

## Create your workspace

Your **workspace** is the Git repository that holds your skills. You own it, and every connected project reads from it. Create it with `init`:

```bash
skillhub init ~/skillhub --yes
```

```text
Workspace created at <home>/skillhub.
Agent connection for this workspace written: claude-code, codex-cli, gemini-cli.
```

`<home>` stands for your home folder. Skip the next steps that `init` prints; its commit hint stages files that must stay out of Git. Commit the workspace's own files, naming each path:

```bash
git -C ~/skillhub add .gitignore .skillhub/schema-version
git -C ~/skillhub commit -m "Initialize Skill Hub workspace"
```

```text
[main (root-commit) <sha>] Initialize Skill Hub workspace
 2 files changed, 3 insertions(+)
```

> [!CAUTION]
> Do not commit the connection files in your workspace: `.mcp.json`, `.claude/`, `.codex/`, `.gemini/`, `.agents/`, `AGENTS.md`, `CLAUDE.md` and `GEMINI.md`. `init` writes them for this machine, and some hold absolute paths. Always name the paths you commit; never stage the whole folder.

## Connect your agent

To **connect** means to register the hub with your agent, so it can ask the hub for skills. Go to a project you work on; this guide calls it `~/myproject`. Then connect it for Claude Code:

```bash
cd ~/myproject
skillhub connect --workspace ~/skillhub --host claude --yes
```

```text
Agent connection for project <home>/myproject written.
  - Claude Code: connected (.mcp.json, .claude/settings.local.json, curator skill, CLAUDE.md)
```

Output is trimmed. Leave out `--yes` to preview what `connect` would write. The [What gets written](user-guide.md#what-gets-written) table lists every connection file.

To connect every project on this machine at once, add `-g`: `skillhub connect -g --workspace ~/skillhub --host claude --yes`.

Now open a new Claude Code session in that folder, so it loads the hub. The first time, Claude Code shows a trust dialog that lists `Bash(skillhub:*)`. Check that the workspace and the connection are healthy:

```bash
skillhub doctor
```

```text
Workspace is healthy.
```

Inside a connected project, `skillhub` finds the workspace on its own. Elsewhere, set `SKILLHUB_WORKSPACE=~/skillhub` or pass `--workspace ~/skillhub`.

## Create your first skill

A **skill** is a folder with a `SKILL.md` your agent follows and a small file that says when it applies. This one teaches your team's commit style. Its last rule adds a marker line, so you can tell when the skill was used.

Save this text as `~/commit-style.md`:

```markdown
# Commit style

Use this skill when you write a Git commit message.

1. Start with a short summary line in the imperative mood, at most 50 characters.
2. Leave one blank line, then explain what changed and why in plain sentences.
3. End every commit message with this exact line:

   Checked-by: skillhub-demo
```

Create the skill from inside your connected project. The flags tell the hub when the skill applies (`--trigger`, `--example`) and when it does not (`--not-for`). At least 3 examples are recommended; one is enough to start:

```bash
skillhub skill create commit-style --collection core --name "Commit style" \
  --description "Write Git commit messages in our team style" --content-file ~/commit-style.md \
  --operation document --trigger "write a commit message" --not-for "writing release notes" \
  --min-scope single_step --example "write a commit message for my staged changes" --yes
```

```text
Draft skill commit-style saved.
```

New skills start as a **draft**: a skill agents are not offered yet. Make it **active**, so agents can be offered it:

```bash
skillhub skill activate commit-style --yes
```

```text
Skill commit-style is now active.
```

## See your first win

Your agent needs a staged change to write about. In a new project, run `git init`, create a `README.md`, then run `git add README.md`.

In your Claude Code session, ask:

> write a commit message for my staged changes

Your agent is told to ask the hub which skill fits. The hub sends back a **recommendation**: the skill that fits, or "no skill fits". If your agent used the skill, the commit message ends with this line:

```text
Checked-by: skillhub-demo
```

You can run the same lookup by hand. Save this request as `~/req.json`. Keep `scope` inside `task`; without it, the hub answers with a question instead of a skill.

```json
{"schema_version": "1", "request_id": "r1", "operation": "document",
 "task": {"description": "write a commit message for my staged changes",
          "scope": "single_step"}}
```

Ask the hub which skill fits the request:

```bash
skillhub resolve --request ~/req.json
```

```text
Resolved commit-style (medium confidence). Host approval is required before activation.
```

The second sentence means your agent still decides whether to load the skill. Confirm that the skill is active:

```bash
skillhub skill list
```

```text
ID            STATE   COLLECTION  NAME
commit-style  active  core        Commit style
```

> [!IMPORTANT]
> Skill Hub never commits or pushes for you. Commit the new skill yourself.

```bash
git -C ~/skillhub add skills history
git -C ~/skillhub commit -m "Add the commit-style skill"
```

```text
[main <sha>] Add the commit-style skill
 4 files changed, 66 insertions(+)
```

## See it in the web UI

This step is optional. The **web UI** is a set of local browser screens for your skills. Start it on this machine only:

```bash
skillhub serve web --loopback-only
```

```text
Skill Hub web UI: http://127.0.0.1:7421/#token=<token>
```

Your browser opens the address. Press Ctrl+C to stop the web UI. The [web UI guide](web-ui.md) tours every screen.

<img src="images/web/skills-list.png" alt="The skills list with a state filter and skills that are draft, active or deprecated." width="800"><br><em>Skills: every skill with its state, and filters to find one.</em>

## If it did not work

- "No Skill Hub workspace was found from the current directory." Run the command inside your connected project, or set `SKILLHUB_WORKSPACE` as shown above.
- Your agent does not use the hub: open a new Claude Code session in the connected folder. Then run `skillhub doctor` there.
- Activation is refused: the draft needs real instructions and routing fields. The error lists what is missing and suggests a `skillhub skill edit` command.
- `resolve` asks "What is the bounded scope of this task?": add `"scope":"single_step"` inside `task` in your request file.
- Anything else: see [Troubleshooting](troubleshooting.md), starting with [Setup and connection](troubleshooting.md#setup-and-connection) and [Skills](troubleshooting.md#skills).

## Next steps

- Add a skill from GitHub and review it: [Add a skill](curating-skills.md#add-a-skill-skillhub-skill-add).
- Keep copied skills up to date with `skillhub check`: [Sources and learning](sources-and-learning.md#keep-copied-skills-up-to-date).
- Let a **curator agent** (an agent with the distill-lab skill) propose **lessons**: improvements you decide on. See [Learn from other repositories](sources-and-learning.md#learn-from-other-repositories).
- Find your way around the screens: [The web UI](web-ui.md#do-common-tasks).
- Use Skill Hub every day: [Using Skill Hub](user-guide.md).
- Look up any command: [Command reference](command-reference.md).
