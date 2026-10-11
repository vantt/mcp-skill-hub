---
phase: 2
title: "Write docs/getting-started.md"
status: pending
priority: P2
effort: "2h"
wave: 1
dependencies: [1]
---

# Phase 2: Write docs/getting-started.md

## Audience and goal

A developer who read the README pitch and wants to feel the value in about 10 minutes. At the end they have a
workspace, a connected project, one active local skill, and visible proof that the hub recommends it. This is
the most important page of the branch: every command on it is copy-pasted by a newcomer.

## Owned files

- Create: `docs/getting-started.md` (150-220 lines).

## Sources to read

- Synthesis: "Value story", "The first win", "Traps newcomers hit", "Get started outline", "Voice and
  presentation".
- `reports/wave0-frozen-map.md`: verified transcript, safe add list, frozen anchors and glossary.
- Old text: `README.md:5-42`, `docs/user-guide.md:45-89` (workspace and connect), `:156-179` (resolve).
- Style guide and glossary in plan.md (all 14 rules apply).

## Required sections (proposed H2s; Phase 1 freezes the final list)

0. Intro (no heading): what you will have at the end, about 10 minutes, prerequisites (Git, Claude Code).
1. `## Install Skill Hub`: the two one-liners, check with `skillhub version`, link `installation.md`.
2. `## Create your workspace`: one sentence on what a workspace is; `skillhub init ~/skillhub --yes`; commit
   only the safe add list from Phase 1 (never `git add -A`); the CAUTION alert about connection files.
3. `## Connect your agent`: per project with `--host claude`; TIP alert for `-g`; "open a new Claude Code
   session in that folder"; check with `skillhub doctor`; one line on what gets written linking
   `user-guide.md#what-gets-written`; one line on `SKILLHUB_WORKSPACE` for CLI use outside connected projects.
4. `## Create your first skill`: the copy-paste `commit-style.md` block with the `Checked-by: skillhub-demo`
   marker; the complete `skill create` command in one block (with `--trigger`, `--not-for`, `--min-scope`,
   `--example`, `--operation`); `skillhub skill activate commit-style --yes`; one line each on draft and active.
5. `## See your first win`: ask the agent "write a commit message for my staged changes"; what you should see
   (the marker line); manual check with `skillhub resolve --request req.json` and the exact JSON with
   `task.scope`; sanity checks (`skillhub skill list` shows `active`, `/mcp` in Claude Code lists `skillhub`
   with 3 tools). IMPORTANT alert: Skill Hub never commits or pushes for you; commit the new skill yourself with
   the exact `git -C ~/skillhub add skills/... && git commit` line from Phase 1.
6. `## See it in the web UI` (optional): `skillhub serve web`, one screenshot (`skills-list.png`), link
   `web-ui.md`.
7. `## If it did not work`: four quick fixes (no workspace found -> `SKILLHUB_WORKSPACE`; agent does not see
   the hub -> new session, `skillhub doctor`; activation refused -> missing routing fields; resolve asks a
   question -> add `task.scope`), link `troubleshooting.md`.
8. `## Next steps`: add a skill from GitHub (`curating-skills.md`), sources and learning
   (`sources-and-learning.md`), web UI tour (`web-ui.md`), everyday use (`user-guide.md`).

## Commands to verify in the sandbox (`SB=/tmp/skillhub-docs-rewrite/getting-started`)

`skillhub version`; `skillhub init ~/skillhub --yes`; the safe `git -C ~/skillhub add ...` and commit;
`skillhub connect --workspace ~/skillhub --host claude --yes`; `skillhub doctor`; the full `skill create`;
`skillhub skill activate commit-style --yes`; `skillhub skill list`; `skillhub resolve --request req.json`;
the post-activation `git add` and commit; `skillhub serve web --no-open` (start, read the printed URL format,
stop it). Do not run the installer one-liners (they write to `~/.local/bin` of the real user if HOME leaks);
copy them verbatim from the current README.

## Images

- `docs/images/web/skills-list.png`, alt "The skills list with a state filter and skills that are draft, active
  or deprecated.", caption "Skills: every skill with its state, and filters to find one.", width 800.

## Anchors

Link only to frozen anchors. Other files will link to this page's H2s, so keep the frozen heading text.

## Acceptance criteria

- 150-220 lines; every rule of the style guide met; one callout per screen.
- Every command block was run in the sandbox and its shown output is real, trimmed output.
- No banned claim; no avoid-term on the first screen; agent support stated honestly (Claude Code verified).
- The create command is complete in one block; the resolve JSON carries `task.scope`.
- "What you should see" does not promise more than the stdio proof and `skillhub resolve` show.
- All links resolve to frozen anchors; unverifiable items are listed in the report, not written.

## Risks

- Readers on Windows: the one-liner and paths differ; one `<details>` block for Windows notes, verified only by
  reading `scripts/install.ps1` usage in the current README (say so in the report).

## Rollback

Delete the file (revert the commit); nothing links to it until wave 2.
