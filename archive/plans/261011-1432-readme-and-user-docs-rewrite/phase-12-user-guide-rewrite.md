---
phase: 12
title: "Rewrite docs/user-guide.md with stubs"
status: completed
priority: P2
effort: "2h"
wave: 2
dependencies: [2, 3, 5, 6, 8]
---

# Phase 12: Rewrite docs/user-guide.md with stubs

## Audience and goal

A user past the first win who wants to understand and use Skill Hub day to day. Title becomes "Using Skill Hub".
It keeps concepts, connecting agents, how the agent picks a skill, skills that need tools or secrets, and moving
machines; everything else has moved to the wave 1 pages and leaves a stub.

## Owned files

- `docs/user-guide.md` (<= 250 lines including stubs).

## Sources to read

- Current `docs/user-guide.md` (all), the move map and anchors in plan.md, `reports/wave0-frozen-map.md`.
- The wave 1 pages that received its content: `getting-started.md`, `installation.md`, `web-ui.md`,
  `troubleshooting.md`, `command-reference.md` (check each moved paragraph landed before cutting).
- Synthesis: "Doc set", "Hard constraints", "Voice and presentation".
- Style guide and glossary in plan.md.

## Required sections

1. `# Using Skill Hub` and a two-line intro linking `getting-started.md` for setup; `SKILLHUB_WORKSPACE`.
2. `## Concepts in plain words`: workspace, skill, skill states, source (upstream, learning reference), content
   approval, "Nothing commits for you". Drop the state-basis and proposal-pin internals (link `contracts/` in one
   line). Keep it short; terms per the glossary.
3. `## Set up and connect agents`: a two-line pointer for creating the workspace (`getting-started.md`);
   `### Connect one project (recommended)`, `### Connect every project at once` (TIP alert), `### What gets
   written` (heading text unchanged; table kept; permission rules in `<details>`; remove "this wave", the
   Claude Code version note and the plan link at old line 115; the "wave-4" migration paragraph becomes one
   plain sentence: "Older connections are upgraded when you run `connect --yes` or `doctor --fix --yes`."),
   CAUTION alert on committing connection files, `### Undo a connection`.
4. `## Curate skills`: stub, "Moved to [Curating skills](curating-skills.md)."
5. `## How the agent picks a skill`: "your agent is told to ask the hub"; recommendation or "no skill fits";
   only active skills; content approval for skills from someone else's repository; the resolve example fixed
   with `task.scope` (same JSON as getting-started); the `local.*` payload list in `<details>`.
6. `## Run skills that need tools or secrets`: keep the four cases, per-skill secrets, `skill doctor`; Windows
   caveat in `<details>`.
7. `## Local Web UI`: stub, "Moved to [Web UI](web-ui.md).", and under it the H3 `### Do common tasks in the Web
   UI` with "Moved to [Web UI: Do common tasks](web-ui.md#<frozen-anchor>)." (keeps the inbound anchor).
8. `## Move to another machine`: keep; IMPORTANT alert "Skill Hub never pushes for you"; link `installation.md`.
9. `## Troubleshooting`: stub to `troubleshooting.md`.
10. `## Command cheat sheet`: stub to `command-reference.md`.

## Commands to verify (`SB=/tmp/skillhub-docs-rewrite/user-guide`)

`skillhub connect --workspace ~/skillhub` (preview) and `--yes`; `skillhub connect -g --workspace ~/skillhub
--yes` (sandbox HOME only); compare the "What gets written" table with the files actually written in the sandbox
project and HOME (list them with `find`); the four permission rules in `.claude/settings.local.json`;
`skillhub resolve --request request.json` with `task.scope`; `skillhub skill env set|list|unset` (pipe a dummy
value); `skillhub skill doctor <skill-id>`; `git clone` of the sandbox workspace into a second folder then
`skillhub rebuild --workspace <clone>`.

## Images

None (web pictures live in `web-ui.md`).

## Anchors

Must survive: `#what-gets-written`, `#do-common-tasks-in-the-web-ui`, `#concepts-in-plain-words`,
`#how-the-agent-picks-a-skill`, `#run-skills-that-need-tools-or-secrets`, `#move-to-another-machine`, and the
stubs `#curate-skills`, `#local-web-ui`, `#troubleshooting`, `#command-cheat-sheet`.

## Acceptance criteria

- <= 250 lines; style guide met; no plan links, no Vietnamese anchors, no "wave", no host version numbers.
- Report lists every removed paragraph and where its content now lives (file and anchor); nothing lost.
- "What gets written" matches the sandbox run; the resolve example returns a recommendation.
- No banned claim; links resolve.

## Rollback

Revert the commit; the old guide returns and the wave 1 pages still stand alone.
