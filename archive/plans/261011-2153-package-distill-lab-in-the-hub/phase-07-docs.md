---
phase: 7
title: "User docs"
status: done
priority: P2
effort: "3.5h"
wave: 4
dependencies: [2, 3, 4, 5, 6]
---

# Phase 7: User docs

<!-- Validation: release is 0.5.0 (was written 5.0.0) throughout this file -->

## Context (traced on pre-0.5.0 main; read the files again at start)

This phase targets the docs as they are after the onboarding plan's 0.5.0 release (plan.md "Sequencing"):
`docs/getting-started.md` is deleted and the README rewritten by that plan, hub commands are documented as
`cd <hub> && skillhub <cmd>` with no `--workspace` in examples, and that plan leaves a slot for the distill
sentences in `docs/sources-and-learning.md` and `docs/web-ui.md`. Line numbers below are pre-0.5.0 and only
locate the old wording; the README is not edited here unless it gained a distill sentence (re-verify).

- `docs/sources-and-learning.md:8,176-184`: "it does not ship the distill-lab skill"; "paste to an agent that has
  the distill-lab skill".
- `docs/web-ui.md:140,161`: same wording.
- `docs/user-guide.md:66-72` ("What gets written": three full curator copies per scope) and `:100`
  (`connect --remove` removes the curator).
- `docs/remote-hub-guide.md:132,154`: "no system-curator skill is installed here".
- `docs/design/05-curation-lifecycle.md:611,620`: distill runs via distill-lab (true; add where it runs).
- `AGENTS.md:16`: "Bundled curator skill: `system-skills/curator/SKILL.md`."
- <!-- Validation: command-reference citation --> `docs/command-reference.md`: no distill-lab mention today;
  `:65` mentions lessons, "Distill with Curator Agent" and the Distill tab, which stay true.
  `docs/troubleshooting.md`: no distill mention today (grep).
- `docs/use-cases/02-core-curation-use-cases.md:271-273`, `03-cli-and-curator-mcp-mapping.md:276-277,383`
  (Vietnamese): statements stay true; verify only.
- `docs/design/web-ux-scorecard.md` mentions `distill.py`: a dated evaluation record, left unchanged.
- Release notes are written at release time (`docs/release-runbook.md`); this phase supplies the lines.

## Owned files

`docs/sources-and-learning.md`, `docs/web-ui.md`, `docs/user-guide.md`,
`docs/remote-hub-guide.md`, `docs/troubleshooting.md`, `docs/installation.md` (requirements line only),
`docs/design/05-curation-lifecycle.md`, `AGENTS.md`, and any other post-0.5.0 doc that names distill-lab
(list them at start with `git grep -l distill-lab docs README.md`). Read-only: `docs/command-reference.md` (change only if a
distill row is added elsewhere), `docs/use-cases/*`, `docs/design/web-ux-scorecard.md`.

## Requirements

1. sources-and-learning: Skill Hub installs distill-lab in the hub folder (at `init`, and on `doctor --fix` for
   older hubs); you run your agent in that folder; it needs Node 18 or newer and Git 2.45.0 or newer and nothing
   else, no `npm install` (one line, with the nodejs.org link; <!-- Validation: Phase 7 self-contradiction -->the
   docs do not name the old script language at all, so the validation grep below stays empty; the release note
   says Python is no longer needed); the skill is self-contained and also works when copied on its own
   (<!-- Red team: hub lookup -->the script finds the hub from the `distill.yaml` path it is given; outside a hub
   only `check`/`format` with the evidence check need one); it is not installed in projects or on machines
   connected with `--hub`. <!-- Red team: network claim; untrusted upstream sources --> Add two sentences: the
   script makes no network call and never fetches a missing file into a partial-clone mirror (it says "not
   fetched"); the agent's mirror clone and fetch steps do use the network. Distill reads untrusted upstream text:
   mirrors are bare clones so no upstream agent files land in the hub, and the agent must never read
   `runtime/config/` or `runtime/envs/` while distilling. <!-- Validation: deny-read dropped --> Add one honest
   sentence: an agent started in the hub folder can read those secret env files, because skills run there need
   them, so only distill sources you are willing to show to that agent.
2. web-ui: handoff step (in the slot the onboarding plan left) says "paste it to an agent you start in your hub
   folder".
3. Wherever the post-0.5.0 docs (for example `docs/curating-skills.md`, which received the old getting-started
   walkthrough) describe "an agent with the distill-lab skill", apply the same one-line correction; command
   examples use `cd <hub> && ...`, never `--workspace`.
4. user-guide "What gets written": project and global rows say the curator skill is written once under
   `.agents/skills/` (read directly by Codex and Gemini CLI) with a short pointer file under `.claude/skills/` for
   Claude Code; nothing under `.gemini/skills/`, and an old copy there from an earlier version is removed by
   `doctor --fix` unless you changed it. Add a sentence that in the hub folder itself `init` also writes
   `distill-lab` the same way, and that these generated folders are kept out of Git through entries in the hub's
   `.gitignore` (added by `init`, or by `doctor --fix` in an older hub). <!-- Red team: contract change, .gitignore instead of info/exclude -->
   <!-- Red team: user-scope wrapper unverified --> <!-- Validation: Claude Code permissions docs --> Say that
   Claude Code following the pointer file was checked by hand in a project folder; with the user-scope pointer
   (`~/.claude/skills/` to `~/.agents/skills/`) Claude Code is expected to ask for permission the first time it
   reads `~/.agents/skills/` from another project in Manual mode, per its permissions docs.
   Keep the heading text
   (anchor `#what-gets-written`). `--remove` line: "the Skill Hub skills, unless you changed them".
5. remote-hub-guide: "no system-curator or distill-lab skill".
6. troubleshooting: one entry "doctor notes that distill-lab cannot find Node 18 or newer" (a notice, not a
   problem) -> install Node 18 or newer; <!-- Validation: notice severity --> one entry
   "connect or doctor reports that a system skill file was not written by Skill Hub" -> Skill Hub no longer
   overwrites a system skill file it does not recognize; it skips only that file, applies every other repair, and
   stops pointing Claude Code at it; review it, delete it, and run the command again (this covers an edited
   `.gemini/skills/system-curator/SKILL.md` too). <!-- Red team: symlinked ~/.agents; older Gemini CLI --> One entry
   "system skill files were skipped because a folder is a symlink" -> Skill Hub does not write through a symlinked
   `.agents` or `.claude`; copy the skills by hand or replace the link with a folder. One entry "Gemini CLI no longer
   shows system-curator" -> update Gemini CLI to v0.28.0 or newer, which reads `.agents/skills`
   (<!-- Validation: Gemini CLI minimum version -->PR #18151).
7. installation: requirements list gains "Node 18 or newer, only to run distill-lab in the hub".
8. 05-curation-lifecycle: add "(installed in the hub folder by init; it runs in the agent with Node, never in the
   binary)".
9. AGENTS.md: replace line 16 with the bundled skills and their sources: curator `system-skills/curator/SKILL.md`;
   distill-lab `.agents/skills/distill-lab/` (Node script with vendored js-yaml 4.3.2; `.claude/skills/distill-lab/`
   is a generated wrapper); both mirrored into `internal/systemskills/`, kept in sync by tests; the copy command
   for distill-lab. <!-- Red team: vendored js-yaml provenance; known digests --> Add the js-yaml update procedure
   (Phase 2 requirement 1) and the release rule: a bundled file or wrapper template that changes after a release
   needs the shipped digest in `known-digests.go`/`known-wrappers.go`, which the tag-derived tests enforce.
10. Release-note lines, written into the phase report for the release author:
    - "Skill Hub installs the distill-lab skill in the hub folder. Its script is now Node (`distill.mjs`), so
      Python is no longer needed; `skillhub status` prints the exact command."
    - "System skills are written once under `.agents/skills/`; Claude Code gets a pointer file and the
      `.gemini/skills/system-curator` copy is removed." <!-- Red team: older Gemini CLI loses the curator -->
      "A Gemini CLI older than v0.28.0 reads only `.gemini/skills` and, because Skill Hub does not serve
      system-curator to Gemini over MCP, no longer sees the curator; update Gemini CLI."
    - "Behaviour change: a system skill file whose content Skill Hub does not recognize is now reported and left
      unchanged; `connect` and `doctor --fix` skip only that file and apply every other repair."
    - <!-- Red team: contract change, .gitignore --> "The hub `.gitignore` gains four entries for the generated
      system skill folders. Existing hubs show `gitignore_entry_missing` until `doctor --fix` adds them; commit the
      `.gitignore` change. If Git already tracks a generated file, doctor prints the `git rm --cached` command for
      it; Skill Hub never runs it."
    - <!-- Red team: status after upgrade --> "After upgrading, `status`, the web Home and `doctor` report host
      integration missing until `doctor --fix` installs distill-lab (and, in projects connected only to Claude Code,
      the shared curator folder)."
    - <!-- Red team: item IDs --> "JSON items for skill files now carry the file path in their ID
      (`host_<host>_<kind>:<path>`), and the shared `.agents/skills` files appear under `agents`."
    - <!-- Red team: review_lessons over MCP --> "The `review_lessons` action carries `script`, `file` and
      `hub_relative`; its command is printed only when the hub's distill-lab matches Skill Hub's copy."
      <!-- Validation: status JSON schema --> "`schemas/curation-home-v1.schema.json` gains these three optional
      `actionItem` properties (public contract change; existing documents stay valid). A blocked system skill file
      appears in `status` as the host-integration action with ID `host_integration_blocked`."
    - <!-- Red team: deliberate script differences --> "distill.mjs differs from distill.py on purpose: `--hub` only
      checks the hub found from the file; the evidence check refuses symlinked or Git-tracked mirrors, never
      fetches (a missing blob is reported as not fetched), and times out after 30 s per Git call; writes refuse
      symlinks and use a random temporary file."
    - "`distill.yaml` header comments now name `distill.mjs` after the next `format`" (plus the parity-fallback
      note if Phase 2 took it).
    - <!-- Red team: downgrade path --> Rollback note: "After downgrading, delete `.claude/skills/system-curator/`
      and `.claude/skills/distill-lab/` if the older binary reports them; it does not know the pointer files." 
11. Doc style follows the archived docs rewrite (plain sentences, no "(s)", no release-wave notes).

## Validation

- `make check` (the repo's doc link checks, if part of it) and a relative-link check over the edited files.
- <!-- Red team: network claim --> `git grep -n -i "no network\|never calls the network"` over the edited docs
  returns nothing that describes distill-lab as fully offline.
- `git grep -n "does not ship the distill-lab"` returns nothing; `git grep -n "~/skill-hub" docs` returns nothing
  new; `git grep -n -i "python\|pyyaml\|distill\.py\|py -3" -- docs AGENTS.md ':!docs/design/web-ux-scorecard.md' ':!docs/plans' ':!docs/references'`
  returns nothing about distill-lab. <!-- Validation: Phase 7 self-contradiction --> Requirement 1 no longer writes
  "no Python", so this grep and the requirements agree: docs describe what distill-lab needs, never what it no
  longer needs; the release note (not a doc) carries "Python is no longer needed".
- Every command shown was run in the Phase 8 sandbox (Phase 8 cross-checks).

## Risks

- Line budgets from the previous docs plan: keep additions to a few lines per file.

## Rollback

Revert the docs commit.
