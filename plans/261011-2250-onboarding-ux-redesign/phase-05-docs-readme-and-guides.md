---
phase: 5
title: "Docs: README onboarding and guides"
status: pending
priority: P1
effort: "1d"
dependencies: [2, 3, 4]
---

# Phase 5: Docs: README onboarding and guides

## Context

The README quickstart (`README.md:58-100`) uses `init ~/skillhub --yes`, manual `git -C` commits,
`connect --workspace ... --yes` and `serve web --loopback-only`. The onboarding is split between the README and
`docs/getting-started.md` (220 lines). `docs/remote-hub-guide.md` (497 lines) is SSH-tunnel first and uses
`--trust-private-network`. Only `docs/remote-hub-guide.md` is checked against the CLI help
(`internal/delivery/cli/remote_guide_doc_test.go:18`). Decisions 1, 2, 7 (security wording), 9. Docs follow the style
of `docs/user-guide.md` (second person, present tense, one action per code block, expected output in a `text` block,
no `$` prompts, placeholders as `<name>`, GitHub alerts sparingly).

All command output shown is captured from a binary built from the branch after Phases 1-4, run in a sandbox with
isolated HOME and XDG_* (never `/home/vantt/skill-hub`), trimmed, with paths and tokens replaced by placeholders.

## Requirements

**README.md** (keep header, badges, pitch, "Why", "How it works", "See it", "Works with", "Documentation",
"Contributing and license"; rewrite the middle):
- `## Install`: the Linux/macOS one-liner, the Windows PowerShell one-liner in `<details>`, then
  `skillhub --version`. This is the only place the one-liner appears.
- `## Step 1: Install the hub`: `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`, its sample output, one
  sentence on what it did (hub, first commit, agents connected in that folder, path stored). Optional
  `<details>` "Keep the hub on GitHub": `cd ~/skill-hub && skillhub init <git-url>` with the three remote cases in one
  short list; "Use the hub on a second machine": `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <git-url>`.
- `## Step 2: Install the client project`: `cd <your-project> && skillhub connect`, sample output, then
  "Restart your agent and ask it: curate my Skill Hub". `-g` in a TIP. Remote hub in `<details>`: on the hub machine
  `cd ~/skill-hub && skillhub serve`, copy the printed `scp` and `connect` lines; link to the remote guide.
- `## Check`: `skillhub status` with real sample output for a healthy client project, then one line per common error
  ("No hub is set up on this machine", "This project uses the hub at <url>" / unreachable, "The hub at <path> is
  gone", "outdated connection") each with its one fix command, and a link to troubleshooting.
- Windows note under each step: `cd <folder>; skillhub <command>` (PowerShell 5.1 has no `&&`).
- Remove the `docs/getting-started.md` links (`README.md:98,122`); "Documentation" links Concepts instead.

**docs/concepts.md** (new): hub; client project; connect (project vs `-g`); agent; the two kinds of token with a
comparison table (hub token: file `runtime/config/.hub-token` in the hub, lasts until `token rotate`, used by agents on
other machines through `connect --hub ... --token-file`, never printed except `token show` on a terminal; web session
token: new on every `skillhub serve web`, in the URL fragment, used by your browser only); skill states (draft,
active, deprecated, archived) in one table. Moves `docs/user-guide.md` "Concepts in plain words" (`:7-21`) here and
leaves a one-line pointer.

**docs/remote-hub-guide.md**: restructure around LAN and Tailscale first (`skillhub serve` on the hub machine,
`scp` + `connect --hub` on the client), then "On an untrusted network" (`skillhub serve --addr 127.0.0.1:7420` +
`ssh -N -L`), the plain-HTTP warning as one WARNING alert, `--addr` list example, token rotation (acts directly),
check with `skillhub status` and `skillhub doctor` in the client project. Remove `--trust-private-network`,
`serve mcp --addr` as the only form, and every `--yes`.

**docs/troubleshooting.md**: reorganized by step. "Installing the hub" (non-empty folder, inside another repo,
inside a hub subfolder, remote unreachable, remote not a hub, unrelated history, push failed, Git identity missing).
"Installing the client project" (no hub set up, hub moved, outdated connection after an upgrade (old `mcp serve`
entries) -> `skillhub doctor --fix`, conflicting managed block, symlink refusal, remote unreachable -> what
`doctor --fix` cannot fix and what to do on the hub machine, token rejected -> re-run the `scp` line, token file mode).
Keep the existing Skills and Windows sections; replace "Start with doctor" with "Start with status".

**docs/installation.md**: details only (options, pin a version, signatures, upgrade, uninstall, build from source); no
one-liner; its next-step link (`:25`) points to `../README.md#step-1-install-the-hub`.

**Delete `docs/getting-started.md`.** Its "Create your first skill" walkthrough moves to `docs/curating-skills.md`
under the same heading so the anchor `#create-your-first-skill` keeps working there; update
`docs/curating-skills.md:5,115`, `docs/troubleshooting.md:83`, `docs/user-guide.md:3,25,120`, `docs/README.md:7`.

**Other docs with old names** (from the plan.md inventory): `docs/command-reference.md` (rewrite tables: no
`integrate`, `check`, `mcp serve`, top-level maintenance, `--yes` on onboarding, `--loopback-only`,
`--trust-private-network`; add `maintain`, `--version`, `help --dev`), `docs/web-ui.md`,
`docs/sources-and-learning.md` (`source check`), `docs/release-runbook.md` (smoke steps), `docs/user-guide.md`
(connect section without `--workspace`), `docs/README.md` (index), `docs/contracts/web-api.md` (loopback flag
mention), and a one-line "Superseded in part by the onboarding redesign of 2026-10" note at the top of
`docs/design/remote-hub.md`.

**Docs-vs-help test.** Generalize `TestRemoteHubGuideCommandsExist` into a table test over `README.md`,
`docs/concepts.md`, `docs/remote-hub-guide.md`, `docs/troubleshooting.md`, `docs/installation.md`,
`docs/command-reference.md`, `docs/user-guide.md`, `docs/curating-skills.md`: the command regex also matches
`cd <x> && skillhub ...`, `mkdir ... && cd ... && skillhub ...` and `cd <x>; skillhub ...`, and every command found
passes `checkGuideCommand` (`remote_guide_doc_test.go:52`), with a minimum count per file. Add a banned-strings test
over `README.md` and `docs/*.md` (not `docs/design`, `docs/use-cases`, `docs/distillery`, `docs/plans`,
`docs/brainstorm`, `docs/references`): `skillhub integrate`, `skillhub web`, `mcp serve`, `skillhub check`,
`--trust-private-network`, `--loopback-only`, `getting-started.md`, `--workspace` in code blocks, `--yes` after
`init|connect|doctor|cache clear|token rotate`, `skillhub init ~`. Add a link check for relative links in those files
(no CI link check exists today).

## Files

- Modify: `README.md`, `docs/concepts.md` (new), `docs/remote-hub-guide.md`, `docs/troubleshooting.md`,
  `docs/installation.md`, `docs/curating-skills.md`, `docs/user-guide.md`, `docs/command-reference.md`,
  `docs/web-ui.md`, `docs/sources-and-learning.md`, `docs/release-runbook.md`, `docs/README.md`,
  `docs/contracts/web-api.md`, `docs/design/remote-hub.md` (one line)
- Delete: `docs/getting-started.md`
- Modify: `internal/delivery/cli/remote_guide_doc_test.go` (generalize; consider renaming to `docs_commands_test.go`)
- Create: banned-strings and link-check tests next to it

## Steps

1. Build the branch binary with `go install` and `GOBIN` into a scratch folder; set up the sandbox; capture every
   output shown in the docs.
2. Write README, then concepts, troubleshooting, remote guide, installation; move the first-skill walkthrough; delete
   getting-started and fix links.
3. Sweep the remaining docs from the inventory.
4. Generalize the docs test; add banned-strings and link tests; run them.

## Validation

- `go test ./internal/delivery/cli -run 'Docs|Guide|Banned|Links'` passes; the README check finds at least 8
  commands.
- `grep -rn getting-started README.md docs/*.md` returns nothing.
- A reader following README Install -> Step 1 -> Step 2 -> Check in a fresh sandbox reaches "curate my Skill Hub"
  with no command that is not printed in the README; outputs match the samples (paths aside).
- Render README and the touched guides in a Markdown preview; alerts and `<details>` render.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Docs describe output that changed after capture | Medium x Medium | Capture after Phase 4 merges; docs test checks commands and flags; samples marked as trimmed |
| External links to `docs/getting-started.md` break | High x Low | Accepted (no backward compatibility); README is the entry point; the `#create-your-first-skill` anchor survives in curating-skills |
| Windows readers copy `&&` lines into PowerShell 5.1 | Medium x Low | Windows note under each step |

## Rollback

Revert the docs commit; the tests added in this phase revert with it.
