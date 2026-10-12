# Install Skill Hub

The installer puts `skillhub` in `~/.local/bin` (Windows: `%LOCALAPPDATA%\skillhub\bin`), checks its SHA-256 checksum and adds that folder to your `PATH`. It runs on Linux, macOS and Windows, on amd64 and arm64.

## Install with one command

The install command for Linux, macOS and Windows is in the [README Quickstart](../README.md#install). After it runs, open a new terminal so the new `PATH` applies, then check that the command works:

```bash
skillhub --version
```

It prints the version, commit and build date. Next, [install the hub](../README.md#step-1-install-the-hub).

## Choose options

The installer reads a few environment variables. On Linux and macOS, put them in front of `sh`, for example `curl -fsSL <install.sh URL> | SKILLHUB_VERSION=<version> sh`. In PowerShell, set them first with `$env:SKILLHUB_VERSION = '<version>'`, then run `irm <install.ps1 URL> | iex` in the same window.

<details>
<summary>Environment variables</summary>

| Variable | What it does | Platforms |
|---|---|---|
| `SKILLHUB_VERSION=<version>` | Installs that release instead of the latest, such as `v0.1.0` or `0.1.0`. | all |
| `SKILLHUB_PREFIX=<dir>` | Installs into `<dir>/bin` instead of `~/.local/bin`. | Linux, macOS |
| `SKILLHUB_INSTALL_DIR=<dir>` | Installs straight into `<dir>`. On Linux and macOS it wins over `SKILLHUB_PREFIX`. | all |
| `SKILLHUB_NO_MODIFY_PATH=1` | Leaves your shell profile files (Windows: your user `PATH`) alone. | all |
| `SKILLHUB_REQUIRE_SIGNATURE=1` | Stops unless `cosign` is installed and the signature checks out. | all |

On Linux and macOS, each variable also has a flag. Run `sh install.sh --help` on a downloaded copy to list them.

</details>

## Check signatures

The installer always checks the SHA-256 checksum and stops on a mismatch.
With `cosign` installed, it also checks the release's Sigstore signature and stops if that fails. Without `cosign`, it prints a notice and goes on.

Set `SKILLHUB_REQUIRE_SIGNATURE=1` to require the signature check, or follow [Manual artifact verification](release-runbook.md#manual-artifact-verification) to check a download by hand.

## Upgrade

Update a binary that the installer put in place:

```bash
skillhub update
```

Add `--check` to only see whether a newer release exists, or `--version <version>` to pick one. Re-running the install command upgrades too. `skillhub update` refuses a binary you built yourself; rebuild it instead.

## Uninstall

On Linux or macOS, run the installer with `--uninstall`:

```bash
curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh -s -- --uninstall
```

On Windows, run this in PowerShell, then close that window so the variable cannot affect a later install:

```powershell
$env:SKILLHUB_UNINSTALL = '1'; irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex
```

Uninstall removes the binary, its install marker and the `PATH` lines the installer added. It never deletes your hubs, your skills or the connection files your agents use.

## Build from source

You need Git, Go 1.26.9 or newer, and Node.js 24. Node.js builds the web UI that is embedded in the binary.

```bash
git clone https://github.com/vantt/mcp-skill-hub.git
cd mcp-skill-hub
make web-build
go build -o skillhub ./cmd/skillhub
```

This leaves `skillhub` in the current folder. Move it to a folder on your `PATH`, then run `skillhub --version`.
