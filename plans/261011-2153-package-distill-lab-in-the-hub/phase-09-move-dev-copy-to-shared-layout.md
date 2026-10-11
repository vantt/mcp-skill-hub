---
phase: 9
title: "Move the dev copy to the shared layout"
status: pending
priority: P3
effort: "1h"
wave: 6
dependencies: [8]
---

# Phase 9: Move the dev copy to the shared layout

## Approval and order

The owner approved keeping the dev copy in this repo and moving it to the shipped layout (2026-10-11). The phase
stays last so the wrapper text is the one Phase 4 renders and Phase 8 verified with Claude Code.

## Context

- After Phases 1-3 the editable source is `.agents/skills/distill-lab/` (SKILL.md, `scripts/distill.mjs`,
  `scripts/vendor/js-yaml.*`), mirrored in `internal/systemskills/distill-lab/` (plan decision 3).
- `.claude/skills/distill-lab/` still holds the old full dev copy: `SKILL.md` and `scripts/distill.py`, both
  tracked although `.claude/` is ignored (`/.gitignore`). It is a third, diverging copy and the only remaining
  Python file.
- The known-digest list (Phase 1) keys old dev SKILL.md versions by `SKILL.md`, so they stay valid after this
  move; it never listed `distill.py`, which Skill Hub never installed.
- `docs/plans/handoff-2026-10-10-lead.md:88` tells the lead "Never edit distill-lab (`.claude/skills/distill-lab`)".

## Owned files

- `.claude/skills/distill-lab/**` of this code repo.
- Create: `internal/hostintegration/repo_wrapper_test.go`.

## Steps

1. Replace `.claude/skills/distill-lab/SKILL.md` with the exact wrapper the installer renders: take it from a
   sandbox hub (`new init`, then copy `$S/hub/.claude/skills/distill-lab/SKILL.md`). It points at
   `${CLAUDE_SKILL_DIR}/../../../.agents/skills/distill-lab/`, which in this repo is the source folder.
2. `git rm .claude/skills/distill-lab/scripts/distill.py` (single script language; the folder `scripts/` goes
   with it).
3. Add `repo_wrapper_test.go`: `../../.claude/skills/distill-lab/SKILL.md` equals `renderWrapper("distill-lab",
   <description from ../../.agents/skills/distill-lab/SKILL.md>)` and Phase 4's
   `testdata/wrappers/distill-lab.golden`, so a description change in the source fails until the wrapper and the
   golden are regenerated (and, after a release, the shipped wrapper digest is added to `known-wrappers.go`); the
   failure message says how. <!-- Red team: wrapper ownership has no history -->
4. Check: open Claude Code in this repo with `SKILLHUB_WORKSPACE` pointing at a sandbox hub; the skill loads and
   runs `node .agents/skills/distill-lab/scripts/distill.mjs list ...`. `git grep -n "\.claude/skills/distill-lab/scripts"`
   shows no live reference outside `archive/` and `plans/`; `git ls-files .claude/skills/distill-lab` lists only
   `SKILL.md`.
5. Report to the lead that `docs/plans/handoff-2026-10-10-lead.md:88` now names a wrapper; the canonical
   path is `.agents/skills/distill-lab/`. That handoff is the lead's record, so this phase does not edit it.
6. `go test ./internal/hostintegration/... ./internal/systemskills/...`; commit
   `chore(skills): point the repository distill-lab at the shared Agent Skills folder`.

## Rollback

Revert the commit; the old full dev copy (SKILL.md and `distill.py`) comes back unchanged and the canonical source
is unaffected.
