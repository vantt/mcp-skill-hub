# Skill Hub

Skill Hub keeps one curated, Git-backed collection of agent skills and shares it with Claude Code, Codex, and Gemini CLI across all your projects. Your agent asks the hub which skill fits the task at hand; the hub answers with a recommendation, and you stay in control of what changes. It ships as a single Go binary. Your skills are plain files in a Git repository that you own.

## Quickstart

**1. Install Skill Hub.** Run the one-liner for your platform:

Linux / macOS:
```bash
curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh
```

Windows (PowerShell):
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex"
```

The installer places `skillhub` in `~/.local/bin` (or `%LOCALAPPDATA%\skillhub\bin` on Windows), verifies SHA-256 checksums, and configures your `PATH`. Open a new terminal and run `skillhub version` to confirm.

**2. Create your workspace.** This is the Git repository that holds your skills:

```bash
skillhub init ~/skillhub --yes
git -C ~/skillhub add -A
git -C ~/skillhub commit -m "Initialize Skill Hub workspace"
```

**3. Connect a project.** Connected projects auto-resolve the workspace:

```bash
cd your-project
skillhub connect --workspace ~/skillhub --yes
```

Tip: Connected projects find the workspace automatically from their local config. For standalone CLI use outside connected projects, run `export SKILLHUB_WORKSPACE=~/skillhub` or pass `--workspace ~/skillhub`. Add `-g` to connect every project at once: `skillhub connect -g --workspace ~/skillhub --yes`.

**4. Restart your agent** in that project and ask it:

> curate my Skill Hub

The agent shows the state of your hub and suggests one next step.

## Installation details and options

- **Install directory:** `$HOME/.local/bin` on Unix, `%LOCALAPPDATA%\skillhub\bin` on Windows.
- **PATH modification:** Enabled by default (updates shell profiles on Unix, User PATH on Windows). Opt out with `SKILLHUB_NO_MODIFY_PATH=1`.
- **Integrity and signatures:** Always verifies SHA-256 checksums. Verifies Cosign signatures when `cosign` is installed; enforce strict signature checking with `SKILLHUB_REQUIRE_SIGNATURE=1`.
- **Pin a version:** Set `SKILLHUB_VERSION=v0.1.0` before running the installer script.
- **Upgrade:** Run `skillhub update`, or re-run the installer one-liner.
- **Uninstall:**
  - Unix: `curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh -s -- --uninstall` (or set `SKILLHUB_UNINSTALL=1`).
  - Windows: `powershell -ExecutionPolicy ByPass -c "$env:SKILLHUB_UNINSTALL='1'; irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex"`.

## Other install options

**Build from source:** (requires Git and Go 1.26+)

```bash
git clone https://github.com/vantt/mcp-skill-hub.git
cd mcp-skill-hub
go build -o ~/.local/bin/skillhub ./cmd/skillhub
```

## What next

- **Curate your skills.** Use the [curating skills guide](docs/curating-skills.md) to add skills (`skillhub skill add`), create workflows (`skillhub skill create`), review diagnostic facts (`skillhub skill review`), edit instructions (`skillhub skill edit --editor`), and keep vendored skills current (`skillhub skill outdated`, `skillhub skill update`).
- **Check health and changes.** `skillhub status` shows what needs attention. `skillhub diff` shows uncommitted changes, and `skillhub validate --staged` validates commits before staging.
- **Diagnose issues.** `skillhub doctor` checks workspace and agent connections (`--fix` to repair); `skillhub skill doctor <id>` tests executable tools and environment requirements for an individual skill.
- **See every command.** `skillhub help`, or `skillhub help <command>`.

## Documentation

- [Curating skills](docs/curating-skills.md): adding, creating, reviewing, editing, and lifecycle management for skills.
- [User guide](docs/user-guide.md): setup, concepts, agent connections, moving machines, troubleshooting, and command reference.
- [Release runbook](docs/release-runbook.md): how releases are built, signed, and verified.
- [Design documents](docs/design/): architecture and decisions (written in Vietnamese).
- [MCP compatibility matrix](docs/mcp-compatibility-matrix.json): tested agent clients.

## Development

```bash
make check      # go vet + golangci-lint (new issues since origin/main) + full test suite
make test-race  # full suite with the race detector
make lint-all   # every lint finding, including existing debt
go run ./cmd/skillhub help
```

Tests run in parallel by default. A test that sets environment variables, changes the working directory, or asserts on process-global state must stay serial (no `t.Parallel()`).
