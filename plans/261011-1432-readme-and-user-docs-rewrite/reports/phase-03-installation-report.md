# Installation page report

Wrote `docs/installation.md` (89 lines, budget 90). H2s match the frozen list: Install with one command, Choose
options, Check signatures, Upgrade, Uninstall, Build from source. Supported platforms is one line in the intro.

## Verification of every variable, flag and claim

| Claim in the page | Source |
|---|---|
| Default dir `~/.local/bin` | `scripts/install.sh:6` (`SKILLHUB_PREFIX` default `$HOME/.local`), `:47-50` (`<prefix>/bin`) |
| Default dir `%LOCALAPPDATA%\skillhub\bin` | `scripts/install.ps1:6`, `:601-602` |
| `SKILLHUB_VERSION` (accepts `v1.2.3` or `1.2.3`) | `install.sh:9`, `:83-88`; `install.ps1:623-624`; usage text `install.sh:572-573` |
| Default version is the release the script came from | stamped by `.github/workflows/release.yml:481-483` |
| `SKILLHUB_PREFIX` (Unix only) | `install.sh:6`; absent from `install.ps1` |
| `SKILLHUB_INSTALL_DIR` overrides prefix | `install.sh:12`, `:47-48`; `install.ps1:593-594` |
| `SKILLHUB_NO_MODIFY_PATH` | `install.sh:19`; `install.ps1:39` (user PATH in registry, `:7`) |
| `SKILLHUB_REQUIRE_SIGNATURE` | `install.sh:20`, `:177-178`; `install.ps1:38` |
| Each Unix variable has a flag; `--help` lists them | `install.sh:565-592` (usage), `:594-706` (parsing); `sh scripts/install.sh --help` ran without side effects |
| SHA-256 always checked; cosign optional; notice without it | `install.sh:288-302`; `install.ps1:430-450`; runbook "Installer policy" |
| Re-running upgrades; no-op on same version | `install.sh:589-590` (usage), `:724-752` |
| `--uninstall` via `sh -s -- --uninstall` | `install.sh:609-611`, `:707-709` |
| Windows uninstall `$env:SKILLHUB_UNINSTALL = '1'; irm ... \| iex` | `install.ps1:37`; same form as `release.yml:909-910` |
| Uninstall removes binary, marker, PATH lines; keeps workspaces and connection files | `install.sh:519-558` ("Canonical workspaces and host configuration were preserved."); `install.ps1:495-586` |
| `skillhub update`, `--check`, `--version <v>` | `/tmp/skillhub-docs-rewrite/help/help-update.txt`; re-run in sandbox `SB=/tmp/skillhub-docs-rewrite/installation` |
| `skillhub update` refuses a self-built binary | ran `skillhub update --check` in the sandbox: "Refusing to update an unmanaged binary ... missing .skillhub-managed marker" |
| `skillhub version` prints version, commit, build date | `help-version.txt`; ran in the sandbox |
| Go 1.26 | `go.mod:3` (`go 1.26.0`) |
| `make web-build` then `go build -o skillhub ./cmd/skillhub` | wave 0 frozen map section 1 (not re-run, per the phase) |

## Decisions

- The Windows uninstall uses the PowerShell form CI uses, not the README's `powershell -c "$env:...='1'; ..."`.
  Inside double quotes an outer PowerShell expands `$env:SKILLHUB_UNINSTALL` to an empty string before the inner
  shell runs, so the old form only works from `cmd.exe`. The page tells readers to close the window afterwards.
- Added `SKILLHUB_INSTALL_DIR` to the options table: it exists in both scripts and is the only way to change the
  folder on Windows. `--purge-config`, `--dry-run`, `--os`, `--arch` and the fixture options are left to
  `install.sh --help`.
- The manual signature steps are not copied; the page links `release-runbook.md#manual-artifact-verification`
  (an H3 in the current runbook). The runbook trim must keep that heading.

## Not verified

- The installers themselves and a real `skillhub update` were not run (no real HOME; the sandbox binary is
  unmanaged). No release version is quoted; the page uses `<version>`.
