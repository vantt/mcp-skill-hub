---
phase: 5
title: "Docs: README onboarding and guides"
status: pending
priority: P1
effort: "1.5d"
dependencies: [2, 3, 4]
---

# Phase 5: Docs: README onboarding and guides

## Context

The README quickstart (`README.md:46-99`, heading `## Quickstart`) uses `init ~/skillhub --yes`, manual `git -C`
commits, `connect --workspace ... --yes` and `serve web --loopback-only`. The onboarding is split between the README and
`docs/getting-started.md`. `docs/remote-hub-guide.md` is SSH-tunnel first and uses `--trust-private-network`. Only
`docs/remote-hub-guide.md` is checked against the CLI help (`internal/delivery/cli/remote_guide_doc_test.go:18`).
Decisions 1, 2, 7 (revised security wording), 9; red-team items F8 (troubleshooting line), F11, F13.

The docs were rewritten on 2026-10-11 by another session. Follow its style guide and glossary
(`archive/plans/261011-1432-readme-and-user-docs-rewrite/plan.md:125` "Style guide (14 rules)" and `:149` "Glossary")
and its frozen transcripts approach: second person, present tense, one action per code block, expected output in a
`text` block, no `$` prompts, placeholders as `<name>`, GitHub alerts sparingly. Constraints from that session:
- keep the anchors `README.md#quickstart`, `docs/user-guide.md#what-gets-written` (`docs/user-guide.md:65`) and
  `docs/user-guide.md#do-common-tasks-in-the-web-ui` (`:212`);
- never tell readers to run `git add -A` in the hub (init commits; status prints a Git line that stages by name);
- the staging hint `stageWorkspaceCommand` is gone (Phase 2), so no doc shows a manual first commit.

All command output shown is captured from a binary installed with `go install` (`GOBIN` in a scratch folder) from the
branch after Phases 1-4, run in a sandbox with isolated HOME, XDG_* and `SKILLHUB_USER_CONFIG_DIR` (never
`/home/vantt/skill-hub`), trimmed, with paths and tokens replaced by placeholders.

## Requirements

**README.md** (keep header, badges, pitch, "Why", "How it works", "See it", "Works with", "Documentation",
"Contributing and license"; rewrite the middle, keeping the `## Quickstart` heading so `#quickstart` keeps working):
- `### Install`: the Linux/macOS one-liner, the Windows PowerShell one-liner in `<details>`, then
  `skillhub --version`. This is the only place the one-liner appears.
- `### Step 1: Install the hub`: `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`, its sample output, one
  sentence on what it did (hub, first commit, agents connected in that folder, path stored). Optional `<details>`
  "Keep the hub on GitHub": `cd ~/skill-hub && skillhub init <git-url>` with the three remote cases in one short list;
  "Use the hub on a second machine": `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <git-url>`.
- `### Step 2: Install the client project`: `cd <your-project> && skillhub connect`, sample output, then "Restart your
  agent and ask it: curate my Skill Hub". `-g` in a TIP. Remote hub in `<details>`: on the hub machine
  `cd ~/skill-hub && skillhub serve`, copy the printed `scp` and `connect` lines; one sentence on the two kinds of
  token linking to the remote guide section; link to the remote guide.
- `### Check`: `skillhub status` with real sample output for a healthy client project, then one line per common error
  ("No hub is set up on this machine", "This project uses the hub at <url>" / unreachable, "The hub at <path> is gone",
  "outdated connection or permission rules") each with its one fix command, and a link to troubleshooting.
- Windows note under each step: `cd <folder>; skillhub <command>` (PowerShell 5.1 has no `&&`).
- Replace the `docs/getting-started.md` links (`README.md:98,122`).

**No `docs/concepts.md`** (F13). The two token types go into a short section "Two kinds of token" in
`docs/remote-hub-guide.md` (hub token: file `runtime/config/.hub-token` in the hub, lasts until `token rotate`, used by
agents on other machines through `connect --hub ... --token-file`, never printed except `token show` on a terminal;
web session token: new on every `skillhub serve web`, in the URL fragment, used by your browser only, and anyone who has
the URL can change skills). `docs/user-guide.md` keeps "Concepts in plain words" as is.

**docs/remote-hub-guide.md**: restructure around LAN and Tailscale first (`skillhub serve` on the hub machine, `scp` +
`connect --hub` on the client with the per-hub token path), then "On an untrusted network"
(`skillhub serve --addr 127.0.0.1:7420` + `ssh -N -L`), the plain-HTTP warning as one WARNING alert that also names the
web UI write access and that the user-accepted default is all interfaces, `--addr` list example, token rotation (acts
directly, terminal only), check with `skillhub status` and `skillhub doctor` in the client project, and one sentence
that Codex and Gemini CLI do not prompt before `init`, `connect`, `doctor --fix`, `token rotate`, `cache clear` or
`serve` (Claude Code asks). Remove `--trust-private-network`, `serve mcp --addr` as the only form, every `--yes`, and
the `mcp serve` mention at `:26` (say "your agent starts Skill Hub over stdio").

**docs/troubleshooting.md** (F13): replace only the entries that became wrong and add the new ones; no reorganization.
New or replaced entries: non-empty folder, inside another repo, inside a hub subfolder, remote unreachable, remote not a
hub, unrelated history, push failed (re-run `skillhub init <url>`), Git identity missing, cloned hub refused for tracked
runtime files, no hub set up, hub moved (`cd <new-location> && skillhub init`, then `skillhub doctor --fix` in each
project), outdated connection or permission rules -> `skillhub doctor --fix`, remote unreachable -> what `doctor --fix`
cannot fix, token rejected -> re-run the `scp` line, "pre-commit hook fails with unknown command validate" ->
`skillhub doctor --fix` (F8). Replace "Start with doctor" with "Start with status".

**docs/installation.md**: no one-liner; its next-step link (`:25`) points to `../README.md#step-1-install-the-hub`.

**`docs/getting-started.md`**: delete, or leave a stub whose whole content is "Moved to [README Quickstart](../README.md#quickstart)"
if any external link must keep resolving (decide when writing; the stub is the default because the other session asked
for anchors or stubs). Its "Create your first skill" walkthrough moves to `docs/curating-skills.md` under the same
heading so `#create-your-first-skill` keeps working there; update `docs/curating-skills.md:5,115`,
`docs/troubleshooting.md:83`, `docs/user-guide.md:3,25,120`, `docs/README.md:7`.

**Other docs with old names** (plan.md inventory): `docs/command-reference.md` (no `integrate`, `check`, `mcp serve`
(`:89`), top-level maintenance, `--yes` on onboarding, `--loopback-only`, `--trust-private-network`; add `maintain`,
`--version`, `help --dev`), `docs/web-ui.md` and `docs/sources-and-learning.md` (mechanical command renames only;
their distill sentences belong to the distill-lab plan), `docs/release-runbook.md` (smoke steps; `mcp serve` at `:164`
becomes the stdio note), `docs/user-guide.md` (connect section without `--workspace`; keep both anchors),
`docs/curating-skills.md:291-296` (hook uses `skillhub maintain validate --staged`), `docs/README.md` (index),
`docs/contracts/web-api.md` (loopback flag mention), and a one-line "Superseded in part by the onboarding redesign of
2026-10" note at the top of `docs/design/remote-hub.md`.

**Release note** (in the release notes source the release runbook names): agent registrations are unchanged; run
`skillhub doctor --fix` in each hub and project once to get the new permission rules and to update a pre-commit hook
that calls `skillhub validate`.

**One docs-vs-help test (F13).** Generalize `TestRemoteHubGuideCommandsExist` into one table test (rename the file to
`docs_commands_test.go`) over `README.md`, `docs/remote-hub-guide.md`, `docs/troubleshooting.md`,
`docs/installation.md`, `docs/command-reference.md`, `docs/user-guide.md`, `docs/curating-skills.md`: it extracts
`skillhub` segments with the Phase 1 extractor (so `cd <x> && skillhub ...`, `mkdir ... && cd ... && skillhub ...` and
`cd <x>; skillhub ...` are covered) and every segment passes `checkGuideCommand`, with a minimum count per file. The
same test fails on a short list of non-command bans in those files: `getting-started.md` (except in the stub itself),
`--workspace` inside fenced code blocks, `git add -A`, `skillhub init ~`, `--trust-private-network`,
`--loopback-only`. No separate banned-strings test and no link checker.

## Files

- Modify: `README.md`, `docs/remote-hub-guide.md`, `docs/troubleshooting.md`, `docs/installation.md`,
  `docs/curating-skills.md`, `docs/user-guide.md`, `docs/command-reference.md`, `docs/web-ui.md`,
  `docs/sources-and-learning.md`, `docs/release-runbook.md`, `docs/README.md`, `docs/contracts/web-api.md`,
  `docs/design/remote-hub.md` (one line), release notes source
- Delete or stub: `docs/getting-started.md`
- Modify/rename: `internal/delivery/cli/remote_guide_doc_test.go` -> `docs_commands_test.go`

## Steps

1. Install the branch binary with `go install` into a scratch `GOBIN`; set up the sandbox; capture every output shown.
2. Write README, then remote guide, troubleshooting entries, installation; move the first-skill walkthrough; delete or
   stub getting-started and fix links.
3. Sweep the remaining docs from the inventory.
4. Generalize the docs test with the bans; run it.
5. Repo-wide grep gate (all names from the plan.md inventory) over `README.md` and `docs/*.md` (not `docs/design`,
   `docs/use-cases`, `docs/distillery`, `docs/plans`, `docs/brainstorm`, `docs/references`).

## Validation

- `go test ./internal/delivery/cli -run 'Docs|Guide'` passes; the README check finds at least 8 commands.
- `grep -rn getting-started README.md docs/*.md` returns only the stub (if kept).
- The three anchors resolve (checked by hand in the Markdown preview).
- A reader following README Install -> Step 1 -> Step 2 -> Check in a fresh sandbox reaches "curate my Skill Hub" with
  no command that is not printed in the README; outputs match the samples (paths aside).
- Render README and the touched guides in a Markdown preview; alerts and `<details>` render.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Docs describe output that changed after capture | Medium x Medium | Capture after Phase 4 merges; docs test checks commands and flags; samples marked as trimmed |
| External links to `docs/getting-started.md` break | Medium x Low | "Moved to" stub by default; `#create-your-first-skill` survives in curating-skills |
| Edits collide with the distill-lab plan's doc work | Low x Medium | Only mechanical renames in `docs/web-ui.md` and `docs/sources-and-learning.md`; this plan lands first |
| Windows readers copy `&&` lines into PowerShell 5.1 | Medium x Low | Windows note under each step |

## Rollback

Revert the docs commit; the test changes in this phase revert with it.
