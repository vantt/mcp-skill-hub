---
phase: 8
title: "Retire the dev copy (owner gate)"
status: pending
priority: P3
effort: "0.5h"
wave: 5
dependencies: [7]
---

# Phase 8: Retire the dev copy (owner gate)

## Gate

Do not start without the owner's explicit go-ahead in the session. The owner's standing rule is that the dev
copy in `.claude/skills/distill-lab/` is not edited in place without consent.

## Context

- After Phase 2 the shipped content lives in `system-skills/distill-lab/` (mirrored in
  `internal/systemskills/distill-lab/`). The dev copy at `.claude/skills/distill-lab/` (SKILL.md, scripts/distill.py,
  tracked in Git) is then an older, diverging third copy.
- The known-digest list (Phase 1) already includes every committed version of the dev copy, so hubs with a hand
  copy are migrated whatever is chosen here.

## Options (owner picks one)

A. Replace `.claude/skills/distill-lab/SKILL.md` with a thin wrapper (same template as Phase 3, Claude variant)
   pointing at `${CLAUDE_SKILL_DIR}/../../../system-skills/distill-lab/SKILL.md`, and delete `scripts/`. Developers
   of this repo keep a working `distill-lab` in Claude Code, aimed at a hub through `SKILLHUB_WORKSPACE`.
B. Delete `.claude/skills/distill-lab/` entirely; developers use the copy installed in their hub.

## Owned files

- `.claude/skills/distill-lab/**` only.

## Steps

1. Apply the chosen option.
2. A: open Claude Code in this repo with `SKILLHUB_WORKSPACE` pointing at a sandbox hub and check the skill loads
   and resolves the script. B: `git grep -n ".claude/skills/distill-lab"` shows no live reference outside
   `archive/` and `plans/`.
3. Commit: `chore(skills): point the repository distill-lab at the bundled copy` (A) or
   `chore(skills): remove the repository copy of distill-lab` (B).

## Rollback

Revert the commit; the dev copy comes back unchanged.
