---
phase: 1
title: "Freeze map, anchors, glossary; verify the first win"
status: completed
priority: P2
effort: "2h"
wave: 0
owner: lead
dependencies: []
---

# Phase 1: Freeze map, anchors, glossary; verify the first win

## Context

Every wave 1 writer needs three fixed inputs before starting: the file map and H2 anchors of every new file
(so cross-links do not break), the glossary (so all files use the same words), and a set of commands that were
actually run with a binary built from main (so nobody guesses output). The synthesis proved the first win once
(`plans/reports/docs-rewrite-discussion-synthesis.md`, "The first win"), but on a `version: dev` binary; this
phase repeats it on a fresh build and records the exact output.

## Owned files

- Create: `plans/261011-1432-readme-and-user-docs-rewrite/reports/wave0-frozen-map.md` (input for writers; a
  tool input, not a summary report).
- Outside the repo: `/tmp/skillhub-docs-rewrite/bin/skillhub` and `/tmp/skillhub-docs-rewrite/wave0/`.

## Steps

1. Build the binary once, before any other phase starts (no other phase builds until Phase 11):
   ```bash
   cd /home/vantt/projects/mcp-skill-hub-docs
   make web-build
   mkdir -p /tmp/skillhub-docs-rewrite/bin
   go build -o /tmp/skillhub-docs-rewrite/bin/skillhub ./cmd/skillhub
   ```
   Do this with the real HOME so Go reuses its caches; switch to the sandbox env only afterwards. Record
   `skillhub version` output (it must not say `dev` if the build stamps a version; if it does say `dev`, note it
   and move on: docs never print the version string).
2. Enter the sandbox env from plan.md (`SB=/tmp/skillhub-docs-rewrite/wave0`) and run the first win end to end:
   - `skillhub init ~/skillhub --yes`, then `git -C ~/skillhub status --porcelain` to list what `init` wrote.
     Decide the **safe add list**: everything except connection files with absolute paths (`.mcp.json`,
     `.claude/`, `.codex/`, `.gemini/`, and any other file containing the sandbox path; check with
     `grep -rl "$SB" ~/skillhub`). Freeze the exact `git -C ~/skillhub add <paths>` line and the commit.
   - In `~/myproject` (a `git init` folder): `skillhub connect --workspace ~/skillhub --host claude --yes`;
     record the files written and the output. Also run the `-g` form in a second sandbox HOME to record its
     output (never on the real HOME).
   - Write `commit-style.md` (the copy-paste skill file whose instructions end every commit message with the
     marker line `Checked-by: skillhub-demo`), then the complete `skill create` command from the synthesis,
     then `skillhub skill activate commit-style --yes`.
   - `skillhub doctor` (expect "Workspace is healthy."), `skillhub skill list` (expect `active`),
     `skillhub status`, and `skillhub resolve --request req.json` with the request JSON from the synthesis
     (expect "Resolved commit-style"). Also run the old request without `task.scope` and record that it does
     not match.
   - Start `skillhub mcp serve` over stdio in the sandbox project and confirm it lists 3 tools (skill_feedback,
     skill_get, skill_resolve), using the same method the synthesis used (a short JSON-RPC `initialize` +
     `tools/list` exchange from a script). Do not open a real Claude Code session.
3. Verify the trap fixes: the working form of adding a GitHub skill (`skillhub skill add
   https://github.com/anthropics/skills --path skills/pdf` preview only, no `--yes`; network allowed here),
   `skillhub version`, the "No Skill Hub workspace was found" message outside a workspace after `connect -g`,
   and `SKILLHUB_WORKSPACE=~/skillhub skillhub status` fixing it.
4. Capture `skillhub help` and `skillhub help <cmd>` for every top-level command and subcommand into
   `/tmp/skillhub-docs-rewrite/help/` (one file per command) so writers and Phase 17 compare against it.
5. Freeze the map in `reports/wave0-frozen-map.md`:
   - the doc set and owners (copy from plan.md, adjust only if a fact forces it, and say why);
   - the H2 list of every new file (start from the proposals in phases 2-8; confirm or adjust) and the
     survivor anchors and stubs from plan.md;
   - the glossary from plan.md, with any wording correction found while running commands;
   - the verified command transcript for the first win with trimmed outputs, the safe add list, the working
     GitHub add form, and every surprise (product bugs go to the follow-ups list, not into docs);
   - the brand file names for Phase 10 (`docs/images/brand/skill-hub-logo-light.svg`,
     `skill-hub-logo-dark.svg`, `skill-hub-mark-light.svg`, `skill-hub-mark-dark.svg`; "light" means "for light
     backgrounds").
6. Commit the frozen map (`docs(plans): freeze the docs map and the verified first-win commands`).

## Validation

- The transcript shows every first-win command with its real output; nothing is copied from the synthesis
  without being re-run.
- `grep -rl "$SB"` over the safe add list returns nothing (no absolute sandbox paths get committed).
- The real HOME has no new files: `ls -la ~/.claude.json ~/.claude ~/.codex ~/.gemini` timestamps are unchanged,
  and `/home/vantt/skill-hub` is untouched (`git -C /home/vantt/skill-hub status` unchanged).

## Risks

- The build differs from what release users install: docs show behavior, not version strings, so this is
  acceptable; note any `dev`-only behavior.
- MCP stdio check needs a client script: keep it in `/tmp/skillhub-docs-rewrite/wave0/`, not in the repo.

## Rollback

Delete the report commit; remove `/tmp/skillhub-docs-rewrite/`.
