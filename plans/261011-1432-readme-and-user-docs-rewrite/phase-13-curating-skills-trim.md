---
phase: 13
title: "Trim docs/curating-skills.md with stubs"
status: pending
priority: P2
effort: "1.5h"
wave: 2
dependencies: [6, 7, 8]
---

# Phase 13: Trim docs/curating-skills.md with stubs

## Audience and goal

A user building their collection: add, create, review, edit, change lifecycle, commit, and check routing
quality. Source tracking and learning moved to `sources-and-learning.md`; the command map moved to
`command-reference.md`.

## Owned files

- `docs/curating-skills.md` (<= 350 lines including stubs).

## Sources to read

- Current `docs/curating-skills.md` (all), the move map in plan.md, `reports/wave0-frozen-map.md`.
- `sources-and-learning.md`, `command-reference.md`, `troubleshooting.md` (check moved content landed).
- Synthesis "Traps newcomers hit" (bare create, examples wording, the working GitHub add form).
- Style guide and glossary in plan.md.

## Required changes

1. Intro: link `getting-started.md` instead of `README.md#quickstart` (keeping the README link is also fine;
   the anchor survives).
2. `## Start with the hub, not a command`: keep; cut the "in this wave" and MCP/CLI curator internals to one
   plain sentence.
3. `## Safety model`: keep, rewritten in plain words; "drafts by default", content approval in the terminal only,
   preview before confirm, never commits or pushes (IMPORTANT alert). Move the cooperative-scope and pin details
   into `<details>`.
4. `## Add a skill`: the GitHub example uses the working form verified by Phase 1 (`--path skills/pdf` or a
   `/tree/main/skills/pdf` URL), never the failing `--skill pdf` form; the four steps after add (review, content
   approval, routing fields, activate) stated plainly.
5. `## Create a skill from your own workflow`: always the complete command (with `--trigger`, `--not-for`,
   `--min-scope`); link the getting-started example.
6. `## Review`, `## Edit and improve`, `## Activate, deprecate, and archive`: keep, trimmed; "at least 3
   examples are recommended".
7. `## Review and commit workspace changes` (with `validate --staged` and a hook example): keep; commit with
   explicit paths, never `git add -A`.
8. `## Measure usage` and `## Check routing quality`: keep, short.
9. Stubs (exact old heading text, one "Moved to" line each): "Keep vendored skills up to date", "Learn from
   references", "Resource-verified fallback and state basis" (to `troubleshooting.md` and `contracts/`), "Import
   repository skills as drafts (`skillhub source import`)", "Advanced intake and recovery", "Command map".

## Commands to verify (`SB=/tmp/skillhub-docs-rewrite/curating`)

`skillhub status`; `skillhub skill add <local-folder>` (preview and `--yes`); the working GitHub add form
(preview only, network once); full `skill create`; `skillhub skill review <skill-id>` and `--verbose`;
`skillhub skill edit <skill-id> --description ...` (preview and `--yes`), `--example`, `--counter-example`,
`--runtime-file` (help only if no spec file is at hand); `skillhub skill confirm <proposal-id>` (help);
`skillhub skill activate|deprecate|archive` in order; `skillhub diff`; `skillhub validate --staged`;
`skillhub telemetry funnel`; `skillhub eval routing --json`.

## Anchors

Keep every surviving H2 text unchanged where possible (other docs link them); stubs keep their old text. Report
any heading renamed and update no other file (Phase 17 checks inbound links).

## Acceptance criteria

- <= 350 lines; style guide met; no "wave", no remote hub, no failing example.
- Report lists every removed paragraph and its new home.
- Commands verified; links resolve.

## Rollback

Revert the commit.
