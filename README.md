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

You need Git and an agent such as Claude Code. Setup takes about two minutes.

### 1. Install

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

Open a new terminal and run `skillhub --version` to check it.

### 2. Set it up

Pick the case that fits you.

#### One computer

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init
skillhub connect -g
```

Restart your agent and say: **curate my Skill Hub**. Done: every project you open now uses your hub.

#### Hub on one computer, agent on another

Both computers must reach each other on the network (same Wi-Fi or LAN, or [Tailscale](https://tailscale.com)). Install Skill Hub on both, then:

**On the hub computer.** Create the hub as above (`init`), then start the server as a background service and make a one-time code:

```bash
cd ~/skill-hub && skillhub serve --install-service
skillhub token invite
```

The service restarts after a failure and when the computer starts (Linux and macOS; on Linux the command prints one more line if it needs `loginctl enable-linger` to run while you are logged out). On Windows, run `skillhub serve` in its own terminal and leave it open.

It prints a ready-to-paste line, for example:

```text
skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD
```

**On the agent computer.** Paste that line inside your project, then restart your agent:

```bash
cd <your-project> && skillhub connect --hub http://100.121.46.35:7420 --invite K7QF-9M2X-4TBD
```

The code works once and expires in 10 minutes; run `token invite` again for a new one. The agent computer keeps no skills, only a token file in its Skill Hub config folder. Add `-g` to the paste line to cover every project.

This runs over plain HTTP on your network: fine at home or in an office. On a network you do not trust, see `skillhub help serve` (`--strict-pairing`, SSH tunnel).

### 3. Check

```bash
skillhub doctor
```

It tells you what works and prints one fix for anything that does not. `skillhub doctor --fix` repairs what it can. The [troubleshooting guide](docs/troubleshooting.md) covers every message.

**Next:** [create your first skill](docs/curating-skills.md#create-your-first-skill).

More: [keep the hub on GitHub](docs/user-guide.md#keep-the-hub-on-github) · [run the server in the background, use SSH tunnels, rotate the token](docs/remote-hub-guide.md) · [installation details](docs/installation.md)

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

Start the web UI from your hub, on this machine only. It prints a link with a session token; open it in your browser. The [web UI guide](docs/web-ui.md) walks through each task.

```bash
cd ~/skill-hub && skillhub serve web --addr 127.0.0.1:7421
```

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
