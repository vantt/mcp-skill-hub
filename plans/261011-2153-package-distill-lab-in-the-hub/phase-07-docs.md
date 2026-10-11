---
phase: 7
title: "User docs"
status: pending
priority: P2
effort: "3h"
wave: 3
dependencies: [2, 3, 4, 5, 6]
---

# Phase 7: User docs

## Context (traced on pre-5.0.0 main; read the files again at start)

This phase targets the docs as they are after the onboarding plan's 5.0.0 release (plan.md "Sequencing"):
`docs/getting-started.md` is deleted and the README rewritten by that plan, hub commands are documented as
`cd <hub> && skillhub <cmd>` with no `--workspace` in examples, and that plan leaves a slot for the distill
sentences in `docs/sources-and-learning.md` and `docs/web-ui.md`. Line numbers below are pre-5.0.0 and only
locate the old wording; the README is not edited here unless it gained a distill sentence (re-verify).

- `docs/sources-and-learning.md:8,176-184`: "it does not ship the distill-lab skill"; "paste to an agent that has
  the distill-lab skill".
- `docs/web-ui.md:140,161`: same wording.
- `docs/user-guide.md:66-72` ("What gets written": three full curator copies per scope) and `:100`
  (`connect --remove` removes the curator).
- `docs/remote-hub-guide.md:132,154`: "no system-curator skill is installed here".
- `docs/design/05-curation-lifecycle.md:611,620`: distill runs via distill-lab (true; add where it runs).
- `AGENTS.md:16`: "Bundled curator skill: `system-skills/curator/SKILL.md`."
- `docs/command-reference.md` and `docs/troubleshooting.md`: no distill mention today (grep).
- `docs/use-cases/02-core-curation-use-cases.md:271-273`, `03-cli-and-curator-mcp-mapping.md:276-277,383`
  (Vietnamese): statements stay true; verify only.
- `docs/design/web-ux-scorecard.md` mentions `distill.py`: a dated evaluation record, left unchanged.
- Release notes are written at release time (`docs/release-runbook.md`); this phase supplies the lines.

## Owned files

`docs/sources-and-learning.md`, `docs/web-ui.md`, `docs/user-guide.md`,
`docs/remote-hub-guide.md`, `docs/troubleshooting.md`, `docs/installation.md` (requirements line only),
`docs/design/05-curation-lifecycle.md`, `AGENTS.md`, and any other post-5.0.0 doc that names distill-lab
(list them at start with `git grep -l distill-lab docs README.md`). Read-only: `docs/command-reference.md` (change only if a
distill row is added elsewhere), `docs/use-cases/*`, `docs/design/web-ux-scorecard.md`.

## Requirements

1. sources-and-learning: Skill Hub installs distill-lab in the hub folder (at `init`, and on `doctor --fix` for
   older hubs); you run your agent in that folder; it needs Node 18 or newer and Git, no `npm install` and no
   Python (one line, with the nodejs.org link); the skill is self-contained and also works when copied on its own
   (hub found by `--hub`, `SKILLHUB_WORKSPACE`, or the current folder); it is not installed in projects or on
   machines connected with `--hub`.
2. web-ui: handoff step (in the slot the onboarding plan left) says "paste it to an agent you start in your hub
   folder".
3. Wherever the post-5.0.0 docs (for example `docs/curating-skills.md`, which received the old getting-started
   walkthrough) describe "an agent with the distill-lab skill", apply the same one-line correction; command
   examples use `cd <hub> && ...`, never `--workspace`.
4. user-guide "What gets written": project and global rows say the curator skill is written once under
   `.agents/skills/` (read directly by Codex and Gemini CLI) with a short pointer file under `.claude/skills/` for
   Claude Code; nothing under `.gemini/skills/`, and an old copy there from an earlier version is removed by
   `doctor --fix` unless you changed it. Add a sentence that in the hub folder itself `init` also writes
   `distill-lab` the same way, and that these generated folders are kept out of Git through the hub's local
   `.git/info/exclude`. Keep the heading text
   (anchor `#what-gets-written`). `--remove` line: "the Skill Hub skills, unless you changed them".
5. remote-hub-guide: "no system-curator or distill-lab skill".
6. troubleshooting: one entry "distill-lab says Node is missing or too old" -> install Node 18 or newer; one entry
   "connect or doctor reports that a system skill file differs" -> Skill Hub no longer overwrites a system skill
   file it does not recognize; review it, delete it, and run the command again (this covers an edited
   `.gemini/skills/system-curator/SKILL.md` too).
7. installation: requirements list gains "Node 18 or newer, only to run distill-lab in the hub".
8. 05-curation-lifecycle: add "(installed in the hub folder by init; it runs in the agent with Node, never in the
   binary)".
9. AGENTS.md: replace line 16 with the bundled skills and their sources: curator `system-skills/curator/SKILL.md`;
   distill-lab `.agents/skills/distill-lab/` (Node script with vendored js-yaml 4.3.2; `.claude/skills/distill-lab/`
   is a generated wrapper); both mirrored into `internal/systemskills/`, kept in sync by tests; the copy command
   for distill-lab.
10. Release-note lines, written into the phase report for the release author:
    - "Skill Hub installs the distill-lab skill in the hub folder. Its script is now Node (`distill.mjs`), so
      Python is no longer needed; run `node .agents/skills/distill-lab/scripts/distill.mjs` from the hub."
    - "System skills are written once under `.agents/skills/`; Claude Code gets a pointer file and the
      `.gemini/skills/system-curator` copy is removed."
    - "Behaviour change: a system skill file whose content Skill Hub does not recognize is now reported as a
      conflict and left unchanged; `doctor --fix` no longer overwrites it."
    - "`distill.yaml` header comments now name `distill.mjs` after the next `format`" (plus the parity-fallback
      note if Phase 2 took it).
11. Doc style follows the archived docs rewrite (plain sentences, no "(s)", no release-wave notes).

## Validation

- `make check` (the repo's doc link checks, if part of it) and a relative-link check over the edited files.
- `git grep -n "does not ship the distill-lab"` returns nothing; `git grep -n "~/skill-hub" docs` returns nothing
  new; `git grep -n -i "python\|pyyaml\|distill\.py\|py -3" -- docs AGENTS.md ':!docs/design/web-ux-scorecard.md' ':!docs/plans' ':!docs/references'`
  returns nothing about distill-lab.
- Every command shown was run in the Phase 8 sandbox (Phase 8 cross-checks).

## Risks

- Line budgets from the previous docs plan: keep additions to a few lines per file.

## Rollback

Revert the docs commit.
