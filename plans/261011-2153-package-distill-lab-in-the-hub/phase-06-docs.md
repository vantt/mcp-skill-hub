---
phase: 6
title: "User docs"
status: pending
priority: P2
effort: "3h"
wave: 3
dependencies: [2, 3, 4, 5]
---

# Phase 6: User docs

## Context (traced)

- `docs/sources-and-learning.md:8,176-184`: "it does not ship the distill-lab skill"; "paste to an agent that has
  the distill-lab skill".
- `docs/web-ui.md:140,161`: same wording.
- `docs/getting-started.md:217`: "a curator agent (an agent with the distill-lab skill)".
- `docs/user-guide.md:66-72` ("What gets written": three full curator copies per scope) and `:100`
  (`connect --remove` removes the curator).
- `docs/remote-hub-guide.md:132,154`: "no system-curator skill is installed here".
- `docs/design/05-curation-lifecycle.md:611,620`: distill runs via distill-lab (true; add where it runs).
- `AGENTS.md:16`: "Bundled curator skill: `system-skills/curator/SKILL.md`."
- `docs/command-reference.md` and `docs/troubleshooting.md`: no distill mention today (grep).
- `docs/use-cases/02-core-curation-use-cases.md:271-273`, `03-cli-and-curator-mcp-mapping.md:276-277,383`
  (Vietnamese): statements stay true; verify only.

## Owned files

`docs/sources-and-learning.md`, `docs/web-ui.md`, `docs/getting-started.md`, `docs/user-guide.md`,
`docs/remote-hub-guide.md`, `docs/troubleshooting.md`, `docs/installation.md` (requirements line only),
`docs/design/05-curation-lifecycle.md`, `AGENTS.md`. Read-only: `docs/command-reference.md` (change only if a
distill row is added elsewhere), `docs/use-cases/*`.

## Requirements

1. sources-and-learning: Skill Hub installs distill-lab in the hub folder (at `init`, and on `doctor --fix` for
   older hubs); you run your agent in that folder; it needs Python 3 and PyYAML (one line with the pip command and
   the Windows `py -3` note); it is not installed in projects or on machines connected with `--hub`.
2. web-ui: handoff step says "paste it to an agent you start in your hub folder".
3. getting-started: same one-line correction.
4. user-guide "What gets written": project and global rows say the curator skill is written once under
   `.agents/skills/` with short pointer files under `.claude/skills/` and `.gemini/skills/`; add a sentence that
   in the hub folder itself `init` also writes `distill-lab` the same way, and that these folders are listed in
   the hub `.gitignore`. Keep the heading text (anchor `#what-gets-written`). `--remove` line: "the Skill Hub
   skills, unless you changed them".
5. remote-hub-guide: "no system-curator or distill-lab skill".
6. troubleshooting: one entry "distill-lab says Python or PyYAML is missing" -> install command; and one entry
   "connect reports that a system skill file differs" -> review, delete, rerun.
7. installation: requirements list gains "Python 3 with PyYAML, only to run distill-lab in the hub".
8. 05-curation-lifecycle: add "(installed in the hub folder by init; it runs in the agent, never in the binary)".
9. AGENTS.md: list `system-skills/distill-lab/` next to the curator and say the two trees are mirrored into
   `internal/systemskills/`.
10. Doc style follows the archived docs rewrite (plain sentences, no "(s)", no release-wave notes).

## Validation

- `make check` (the repo's doc link checks, if part of it) and a relative-link check over the edited files.
- `git grep -n "does not ship the distill-lab"` returns nothing; `git grep -n "~/skill-hub" docs` returns nothing
  new.
- Every command shown was run in the Phase 7 sandbox (Phase 7 cross-checks).

## Risks

- Line budgets from the previous docs plan: keep additions to a few lines per file.

## Rollback

Revert the docs commit.
