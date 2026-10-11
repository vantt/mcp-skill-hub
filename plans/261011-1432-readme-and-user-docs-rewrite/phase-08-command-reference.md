---
phase: 8
title: "Write docs/command-reference.md"
status: pending
priority: P2
effort: "1.5h"
wave: 1
dependencies: [1]
---

# Phase 8: Write docs/command-reference.md

## Audience and goal

A user who knows what they want to do and needs the command. One merged table replaces the two tables that
disagree today (`docs/user-guide.md:486-536`, `docs/curating-skills.md:606-645`). It points to `skillhub help
<command>` as the authority for flags.

## Owned files

- Create: `docs/command-reference.md` (<= 90 lines).

## Sources to read

- Both existing tables (lines above).
- `/tmp/skillhub-docs-rewrite/help/` (Phase 1 captured `skillhub help` for every command): the authority.
- Style guide and glossary in plan.md.

## Required sections (proposed H2s)

1. Intro: one line; `skillhub help` and `skillhub help <command>`; most commands accept `--workspace <path>`
   and `--json`; mutating commands preview first and need `--yes` to write.
2. `## Set up`: init, connect (and `-g`; `integrate` alias), doctor, status, version, update.
3. `## Work with skills`: list, show, add, create, review, edit, approve content, env, doctor, activate,
   deprecate, archive, confirm.
4. `## Sources and learning`: outdated, upstream, update, source list/watch/unwatch/attach/detach/backfill/
   check/import/capture/triage, inbox, insight.
5. `## Workspace and Git`: diff, validate, validate --staged, rebuild, migrate.
6. `## Routing and usage`: resolve, eval routing, telemetry.

Table columns: Task | Command. Show only the main flags; never list a flag that `skillhub help` does not show.
Where the two old tables disagree, `skillhub help` wins; list each disagreement and its resolution in the report.
Leave out `connect --hub` and any remote hub command.

## Commands to verify

Every row against the Phase 1 help capture; for any command missing from the capture, run
`skillhub help <cmd>` with the Phase 1 binary in `SB=/tmp/skillhub-docs-rewrite/command-reference`.

## Acceptance criteria

- <= 90 lines; every row matches `skillhub help`; one table per H2.
- No remote hub; no hidden or internal commands unless `skillhub help` lists them.

## Rollback

Revert the commit.
