<h1 align="center"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/images/brand/skill-hub-logo-dark.svg"><img src="docs/images/brand/skill-hub-logo-light.svg" alt="Skill Hub" width="280"></picture></h1>

<p align="center">Your skills in one place. Pulled by any agent, on demand.</p>

<p align="center"><a href="https://github.com/vantt/skillhub-mcp/releases/latest"><img src="https://img.shields.io/github/v/release/vantt/skillhub-mcp" alt="Latest release"></a> <a href="https://github.com/vantt/skillhub-mcp/actions/workflows/ci.yml"><img src="https://github.com/vantt/skillhub-mcp/actions/workflows/ci.yml/badge.svg" alt="CI"></a> <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License: MIT"></a></p>

<p align="center"><a href="README.vi.md">Tiếng Việt</a> · <a href="#quickstart">Get started</a> · <a href="docs/user-guide.md">Using Skill Hub</a> · <a href="docs/curating-skills.md">Curating skills</a></p>

Skill Hub serves your agent skills over MCP from one Git repository you own, so there is nothing to install or copy into each project: your agent asks the hub for the right skill when a task starts.
Because the library is central, you can collect skills from any repository, edit them, and tune them to your own criteria, with feedback from real use and an approval step before anything reaches an agent.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/web/dark-skills-list.png">
  <img src="docs/images/web/skills-list.png" alt="The skills list with a state filter and skills that are draft, active or deprecated." width="800">
</picture>

*Skills: every skill with its state, and filters to find one.*

## Quickstart

From nothing to an agent that talks to your own hub, on one computer, in about two minutes. You need Git and an agent such as Claude Code.

### 1. Install

This puts the `skillhub` command on your computer.

```bash
curl -fsSL https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh | sh
```

<details>
<summary>Windows (PowerShell)</summary>

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/skillhub-mcp/releases/latest/download/install.ps1 | iex"
```

PowerShell 5.1 has no `&&`: write each command below on its own line, or separate them with `;`.

</details>

Open a new terminal so it finds the command, then check it:

```bash
skillhub --version
```

The first line shows the version, such as 0.7.1.

### 2. Create your hub

The hub is a folder, in its own Git repository, that holds your skills. Git saves every change with your name and email. If `git config user.email` prints nothing, run:

```bash
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
```

Then create the hub:

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init
```

```text
Hub created at /home/you/skill-hub.
...
This is now the hub of this machine; `skillhub connect` uses it.
```

`init` may suggest connecting one project; use step 3 instead, which connects all of them.

### 3. Connect your agent

This tells the agents on this computer where your hub is, in every project you open.

```bash
skillhub connect -g
```

```text
Connected your user account (all projects) to the hub at
/home/you/skill-hub.
...
Next: restart your agent and ask it: "curate my Skill Hub"
```

The `WARNING` about host activation in the middle of it is expected. The first time, Claude Code asks you to trust `Bash(skillhub:*)`: allow it, so your agent can run `skillhub` commands.

### 4. Check

`doctor` checks the hub and the connection, and prints one fix for anything that is wrong.

```bash
skillhub doctor
```

```text
...
Claude Code       ok      user entry in ~/.claude.json; not used here, because this project's own
                          entry wins
...
Healthy.
```

Inside `~/skill-hub`, the hub folder has its own connection, so the lines for your user account say "not used here": that is normal. Lines marked `notice` are information, not errors. If the last line is not `Healthy.`, run `skillhub doctor --fix`, or look the message up in [Troubleshooting](docs/troubleshooting.md).

### 5. Ask your agent

Open your agent in any project folder (restart it if it was running, so it loads the connection), then say:

> curate my Skill Hub

Your agent checks your hub with `skillhub status` and suggests one next step. A new hub has no skills yet, so that step is your first skill: [create your first skill](docs/curating-skills.md#create-your-first-skill). Once you make it active, which turns it on for agents, your agent asks the hub for it on its own when a task fits.

### Next steps

- [Create your first skill](docs/curating-skills.md#create-your-first-skill), or [add skills from GitHub](docs/curating-skills.md#add-a-skill-from-github).
- [Browse and review skills in your browser](#see-it).
- [Keep the hub on GitHub](docs/user-guide.md#keep-the-hub-on-github), and [move it to another machine](docs/user-guide.md#move-to-another-machine).
- [Connect only some projects](docs/user-guide.md#connect-one-project-recommended) instead of all of them.
- [Install options, upgrade and uninstall](docs/installation.md).
- Every command and flag: `skillhub help`, `skillhub help <command>`, or the [command reference](docs/command-reference.md).

### Hub and agent on different computers

Keep the hub on one computer and use it from an agent on another, over the same Wi-Fi or LAN, or [Tailscale](https://tailscale.com). Do steps 1 and 2 on the hub computer, then start the server there as a background service and make a one-time code:

```bash
cd ~/skill-hub && skillhub serve --install-service
skillhub token invite
```

`token invite` prints one line per network of the hub computer, for example:

```text
Tailscale (100.121.46.35), on the other machine:
  $ skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD
```

On the agent computer, do step 1, then run the line for a network that computer can reach, with `-g` added so every project uses the hub:

```bash
skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD -g
```

Check it there with step 4, then restart your agent: it now asks the hub for skills. Curation stays on the hub computer: open your agent in `~/skill-hub` there and say "curate my Skill Hub". The code works once, for 10 minutes. `--install-service` covers Linux and macOS; on Windows, run `skillhub serve` in its own terminal and leave it open. The connection is plain HTTP, fine at home or in an office; for other networks, SSH tunnels and the token, read [One hub from several machines](docs/remote-hub-guide.md).

## How it works

1. Your skills live in a hub: a folder in a Git repository you own.
2. You connect your agent to the hub once.
3. When a task starts, your agent asks the hub which skill fits, and the hub answers.

```mermaid
flowchart LR
    agent["Your AI agent<br/>Claude Code"]
    hub{{"Skill Hub"}}
    repo[("Your skills<br/>in a Git repository you own")]
    you(["You"])
    gh["GitHub repos<br/>you import from"]

    agent -- "1 Which skill fits?" --> hub
    hub -- "2 This one" --> agent
    hub <--> repo
    you -- "Browser or terminal" --> hub
    gh -. "import, check updates" .-> hub

    classDef agentNode fill:#dbeafe,stroke:#1d4ed8,stroke-width:2px,color:#0f172a
    classDef hubNode fill:#fde68a,stroke:#b45309,stroke-width:3px,color:#0f172a
    classDef storeNode fill:#bbf7d0,stroke:#15803d,stroke-width:2px,color:#0f172a
    classDef personNode fill:#fbcfe8,stroke:#be185d,stroke-width:2px,color:#0f172a
    classDef externalNode fill:#e5e7eb,stroke:#4b5563,stroke-width:2px,color:#0f172a
    class agent agentNode
    class hub hubNode
    class repo storeNode
    class you personNode
    class gh externalNode
```


## Why Skill Hub

- **Nothing to install per project.** Connect once; no skill folders to copy around.
- **Make skills yours.** Collect skills from any repository, edit them, tune them to your own criteria. New skills start as drafts you approve.
- **One recommendation per task.** The full skill text loads only after your agent picks one.
- **Git is the database.** Plain files in your own repository: review, share and roll back with Git.
- **A local web UI** to browse, review and edit skills.

## See it

Start the web UI from your hub. It prints a link for this computer, your network and Tailscale, each with a session token; open one in your browser. The [web UI guide](docs/web-ui.md) walks through each task.

```bash
cd ~/skill-hub && skillhub serve web
```

Anyone with the link can change your skills; [keep it safe on your network](docs/web-ui.md#keep-it-safe-on-your-network).

<img src="docs/images/web/skill-review.png" alt="The Review tab of an active skill showing its content check and readiness." width="800">

*Review: whether agents can use this skill, and what to check first.*

## Works with

| Agent | Status |
|---|---|
| Claude Code | Verified ([compatibility matrix](docs/mcp-compatibility-matrix.json)) |
| Codex | `connect` writes connection files; not yet verified end to end |
| Gemini CLI | `connect` writes connection files; not yet verified end to end |

## Documentation

- **Start:** [Quickstart](#quickstart) · [Installation](docs/installation.md)
- **Guides:** [Using Skill Hub](docs/user-guide.md) · [Curating skills](docs/curating-skills.md) · [Sources and learning](docs/sources-and-learning.md) · [Web UI guide](docs/web-ui.md) · [One hub from several machines](docs/remote-hub-guide.md) · [Troubleshooting](docs/troubleshooting.md)
- **Reference:** [Command reference](docs/command-reference.md) · [All docs](docs/README.md)
- **Maintainers:** [Contributing](CONTRIBUTING.md) · [Release runbook](docs/release-runbook.md)

## Contributing and license

To build, test or release Skill Hub, see [CONTRIBUTING.md](CONTRIBUTING.md). Skill Hub is released under the MIT License; see [LICENSE](LICENSE).
