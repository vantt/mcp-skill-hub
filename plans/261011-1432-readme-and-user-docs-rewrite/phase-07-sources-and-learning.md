---
phase: 7
title: "Write docs/sources-and-learning.md"
status: pending
priority: P2
effort: "1.5h"
wave: 1
dependencies: [1]
---

# Phase 7: Write docs/sources-and-learning.md

## Audience and goal

A user whose skills came from other repositories, or who wants their skills to improve from what others publish.
The page explains two jobs in plain words: keep copied skills up to date with their upstream, and learn from
learning references through your curator agent. It states that nothing runs in the background.

## Owned files

- Create: `docs/sources-and-learning.md` (<= 280 lines).

## Sources to read

- `docs/curating-skills.md:274-449` (Keep vendored skills up to date; Learn from references),
  `:555-605` (Import repository skills as drafts; Advanced intake and recovery).
- `docs/user-guide.md:30` (Source concept), `:36` (Insight and inbox).
- Synthesis "Claims that must NOT appear" (watching and distilling).
- Style guide and glossary in plan.md.

## Required sections (proposed H2s)

1. Intro: the two jobs, what a source, an upstream and a learning reference are (glossary lines), and "nothing
   runs in the background; checks happen when you run them".
2. `## Keep copied skills up to date`: check for updates (`skillhub skill outdated`, `--check`), inspect one
   (`skillhub skill upstream <skill-id>`), apply an update (`skillhub skill update <skill-id>`; explain the
   three-way choice per file in plain words: take upstream, keep mine, edit manually), approve the content again
   (updates never approve content), track an older skill (`skillhub source backfill`).
3. `## Learn from other repositories`: attach and detach a learning reference, watch a repository for a skill
   (`source watch` records an interval only), check for updates (`skillhub source check --all-due` or `--all`),
   hand changes to your curator agent (distill-lab skill), review lessons in the inbox (`skillhub inbox`,
   `skillhub insight ...`).
4. `## Import many skills from a repository`: `skillhub source import <locator> [--skill <name> | --all]`; drafts
   only.
5. `## Triage candidate repositories`: `<details>` with `source capture`, `source list`, `source triage`, the
   confirm and import steps (advanced intake).

## Commands to verify (`SB=/tmp/skillhub-docs-rewrite/sources`)

Help forms for every command shown (`skillhub help skill outdated`, `skill upstream`, `skill update`,
`source backfill`, `source attach`, `source detach`, `source watch`, `source unwatch`, `source check`,
`source import`, `source capture`, `source triage`, `source list`, `inbox`, `insight`). Run offline what can
run offline: `skillhub source capture <url> --reason <text>` and `skillhub source list` in a sandbox workspace,
`skillhub inbox` on an empty hub. Network is allowed for one preview: `skillhub source import
https://github.com/anthropics/skills --path skills/pdf` without `--yes` (confirm the flag exists first). Do not
run `source check` against the network more than once.

## Images

Optional, at most one: `sources.png` (alt "The sources list with one source and the result of its last check.",
caption "Sources: where skills learn from, and when each was last checked."). `distill-handoff.png` belongs to
`web-ui.md`; link there instead of repeating it.

## Anchors

`curating-skills.md` stubs will point to this page's H2s; keep the frozen heading text.

## Acceptance criteria

- <= 280 lines; style guide met; "3-way merge" and "vendored" appear at most once each, explained.
- No claim of automatic watching or of the hub distilling by itself.
- Every flag exists in `skillhub help`; links resolve.

## Rollback

Revert the commit.
