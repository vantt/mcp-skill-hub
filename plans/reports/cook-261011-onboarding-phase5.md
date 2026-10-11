# Onboarding UX redesign, phase 5 (docs: README onboarding and guides): implementation report

Branch `worktree-agent-a5c101367df3589e1`, 2026-10-12. Plan:
[plan.md](../261011-2250-onboarding-ux-redesign/plan.md),
[phase-05](../261011-2250-onboarding-ux-redesign/phase-05-docs-readme-and-guides.md). Status: completed.

## What changed

Commits on the branch (nothing pushed):

| Commit | Change |
|---|---|
| docs (README) | README Quickstart rewritten: Install (one-liner, Windows in `<details>`, `skillhub --version`), Step 1 (`mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`, sample output, "Keep the hub on GitHub" and "Use the hub on a second machine" with `init <git-url>`), Step 2 (`cd <your-project> && skillhub connect`, "curate my Skill Hub", `-g` TIP, remote hub in `<details>` linking "two kinds of token"), Check (`status` sample, five common messages with their fix, link to troubleshooting). PowerShell blocks under both steps. "See it" uses `serve web --addr 127.0.0.1:7421`. `docs/getting-started.md` deleted; its first-skill walkthrough is now `docs/curating-skills.md#create-your-first-skill` (with "See your first win" and "Commit the new skill" through the Git line status prints). `docs/installation.md` keeps details only. `docs/user-guide.md`: connect without `--workspace`/`--yes`/`--project`, the nine permission rules, moved-hub advice, move to another machine through `init <git-url>`, corrected "nothing commits for you". `docs/README.md` index. |
| docs (remote guide) | `docs/remote-hub-guide.md` restructured: LAN and Tailscale first (`cd ~/skill-hub && skillhub serve`, banner, `scp -p` to `~/.skillhub-token-<host>-<port>`, `connect --hub ... --token-file ...`, PowerShell block), `--addr` lists, `--allow-host`, "Prefer -g", "On an untrusted network: use an SSH tunnel" (`serve --addr 127.0.0.1:7420`, tunnel block, `ssh -N -L`), systemd unit with `WorkingDirectory` instead of `--workspace`, "Two kinds of token" table, check with `status` and `doctor` (what `doctor --fix` cannot fix), one WARNING alert naming the 0.0.0.0 default and the web write access, the Codex/Gemini CLI prompt sentence, `token rotate` (acts directly, terminal only). No `mcp serve` mention ("your agent starts Skill Hub over stdio"). |
| docs (troubleshooting) | "Start with status"; new sections "Installing the hub" (folder with files, inside another repo, inside a hub, Git could not read the URL, not a hub, unrelated history, push failed, no Git identity, cloned hub tracking runtime files), "Installing the client project" (no hub set up, hub folder gone, outdated connection files or permission rules), "Using a hub on another machine" (nothing answers, token refused). Replaced the outdated entries (no workspace found, missing permission entries, stale index, fresh clone, Windows firewall) and added the pre-commit hook entry and a Windows `&&` note. |
| docs (renames) | `docs/command-reference.md` (setup table, `doctor --fix [--dry-run]`, exit codes, `maintain` group, "Serve other machines", `--version`, `help --dev`, no `mcp serve`), `docs/web-ui.md` (start command, 0.0.0.0 default, WARNING with write access, new banner), `docs/sources-and-learning.md` (command renames only: `source check`, `serve web --addr`, the folder sentence), `docs/release-runbook.md`, `docs/contracts/web-api.md` (binding and Host rules), `docs/contracts/error-codes.md`, one-line note on `docs/design/remote-hub.md`. |
| test | `remote_guide_doc_test.go` renamed to `docs_commands_test.go`. `TestDocsCommandsExist` checks every `skillhub` segment (code blocks and inline code, chains with `&&` and `;`) in README and six guides against the help, with a minimum count each (README 15, remote guide 45, troubleshooting 30, installation 3, command reference 55, user guide 20, curating skills 40; found 22/55/41/4/65/25/46) and fails on `--workspace` inside a code block. `TestDocsAvoidRemovedNames` scans README and every `docs/*.md` for `getting-started.md`, `--trust-private-network`, `--loopback-only`, `skillhub init ~`, `git add -A`. `TestCheckGuideCommandReadsArgumentsAndChoices` covers the checker changes below. |
| docs(plans) | Phase 5 status, todo, plan row, this report. |

### Checker changes (`guideCommandProblems`)

- A word after the subcommand that the help does not name starts the arguments, so `skillhub skill activate commit-style --yes`
  passes; the first subcommand word must still be named (whole word, so `activat` fails).
- `--json\|--quiet` (a choice inside a Markdown table) is read as two flags.
- `skillhub help`, `skillhub help <command>` and `skillhub help --dev` are valid when they run.
- `skillhub <command>` (only placeholders) is the general form and is skipped.
- Inside code blocks, a sample-output line that quotes a command in backticks is read like prose (only the quoted command counts).

## Verification

- Binary from this branch installed with `go install` (`GOBIN` in the session scratchpad); sandbox with its own HOME,
  XDG_*, `SKILLHUB_USER_CONFIG_DIR` and `GIT_CONFIG_GLOBAL=/dev/null`. Captured: `--version`, `help`, `help --dev`, help
  of init/connect/status/doctor/serve/token/cache/maintain/skill; init in an empty folder, re-run, folder with files,
  inside another repo, inside a hub, unreachable URL, non-hub remote, empty remote (push), second machine (fetch),
  existing hub bound to an empty remote, unrelated history, `--dry-run` with a URL; connect, `--dry-run`, `-g`,
  `--remove --dry-run`, connect with no stored hub; status/doctor healthy, moved hub, `doctor --fix` re-point, outdated
  permission rules; `serve --addr 127.0.0.1:<port>` banner, remote connect, status/doctor while serving and after stop,
  `token rotate --dry-run` and off a terminal; the first-skill walkthrough (create, activate, resolve, list, status Git
  line); `init` in a hub cloned with Git. Paths in the docs are replaced by `<home>`, `/home/you` or placeholders.
- The 0.0.0.0 banners (MCP and web) were not started on this machine (they would listen on every interface); their text
  comes from `serve_banner.go`, `web/listen.go` and the Phase 3 golden.
- `go test -count=1 ./internal/delivery/cli -run 'Docs|Guide|CommandSegments'` passes; a mutation (an `init --force`,
  `doctor --fix --yes`, `--workspace` and `getting-started.md` line appended to README) fails both docs tests as intended.
- `gofmt -l .` empty; `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=darwin go vet ./...` clean; full
  `go test -count=1 ./...` passes (isolated HOME, XDG_*, `SKILLHUB_USER_CONFIG_DIR`, `GIT_CONFIG_GLOBAL=/dev/null`).
- No Markdown lint or link tooling runs in the Makefile or CI (`ci.yml` lints Go only), so nothing else to run.

## Grep gate

Over README, `docs/*.md`, `docs/contracts`, `CONTRIBUTING.md`, `AGENTS.md`, `system-skills`, `scripts`, `web/src`,
`web/e2e`, `internal`, `cmd`, `.github`, `Makefile`, `schemas`: no `integrate`, `skillhub web`, `skillhub check`,
`--trust-private-network`, `--loopback-only`, `SKILLHUB_WEB_ADDR`, `init --force`, top-level
`validate|rebuild|migrate|diff`, `getting-started`, onboarding `--yes`, or `mcp serve`/`serve mcp` in user docs. Remaining
hits, all intentional:

- `internal/app/health_hub.go` (the pre-commit hook finding names the removed `skillhub validate` on purpose),
  `internal/delivery/cli/connect.go` ("commands with --yes or confirm" = curation commands), `web/e2e/smoke.spec.ts`
  ("loopback-only requests" describes browser traffic, not the flag), `scripts/mcp-fanout-bench.sh` (a comment about the
  hidden stdio entry).
- `skillhub version` in the curator `SKILL.md` and `skillhub version --json` in the release runbook: the command stays (P2).
- `telemetry import-transcripts --project <dir>` and `remote-timing.sh --project`: their own flags, not `connect --project`.
- `docs/PRD.md` lines 835, 1551, 1563, 1760 (`skillhub rebuild`, `skillhub validate`): left as is, see Decisions.

The installers already print `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init` (PowerShell:
`mkdir -Force ~/skill-hub; cd ~/skill-hub; skillhub init`), the same commands as README Step 1; no change needed.

## Decisions where the plan was silent

1. **getting-started.md deleted, not stubbed**: the task said to delete it; both anchors other docs need survive
   (`curating-skills.md#create-your-first-skill`, README `#quickstart`), and the ban test covers README and `docs/*.md`.
2. **Removed commands in prose are not in inline code** (troubleshooting and curating-skills say "the old top-level
   validate command"), so the docs test can check every inline `skillhub` command without an exception list.
3. **Text bans apply to README and every top-level `docs/*.md`**; the command check stays on the seven onboarding files
   the plan names (not `web-ui.md` or `sources-and-learning.md`, which the distill-lab session edits next).
4. **`docs/PRD.md` is left as is**: it is the original Vietnamese product requirements draft with commands that never
   existed (`skillhub distill`, `skillhub ui`), a historical record like `docs/design/*`.
5. **"Nothing commits for you" corrected** in user-guide, curating-skills and command reference: Skill Hub never commits
   or pushes skill changes; only `init` makes the first commit and pushes to an empty repository you name.
6. **Remote guide systemd unit** uses `WorkingDirectory=` instead of `--workspace`, keeping code blocks free of `--workspace`.
7. **Windows `scp`**: phase 3 left `~` expansion in PowerShell unverified, so the PowerShell block shows `$HOME\...`.
8. **README Check list** names five messages exactly as the CLI prints them; "This project uses the hub at <url>" is
   covered by its unreachable form ("Nothing answers at <url>").

## Release note (for the release owner)

The repository has no changelog or release notes source: `.github/workflows/release.yml` runs `gh release create
--generate-notes`. Suggested text for 0.5.0:

> Agent registrations are unchanged. Run `skillhub doctor --fix` once in each hub and in each connected project: it adds
> the new Claude Code permission rules and updates a pre-commit hook that still calls the old top-level validate command
> (now `skillhub maintain validate --staged`).

## Not done / for later

- Phase 6 (guided `skill create`) stays pending.
- Markdown preview rendering of alerts and `<details>` was not checked in a browser; the syntax matches the existing docs.
- Windows and macOS not run; only docs and a Go test changed.

## Unresolved questions

- None blocking.
