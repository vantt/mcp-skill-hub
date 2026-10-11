---
phase: 3
title: "distill-lab skill text: hub lookup, one script section, requirements"
status: pending
priority: P2
effort: "1.5h"
wave: 2
dependencies: [1, 2]
---

# Phase 3: distill-lab skill text: hub lookup, one script section, requirements

## Context (traced)

Line numbers below are in the dev SKILL.md as copied verbatim by Phase 1 into `.agents/skills/distill-lab/`.

- `SKILL.md:292`: "Hub root: `$SKILLHUB_WORKSPACE`, else `~/skill-hub`." A personal path in a shipped skill.
- `SKILL.md:341-354`: `S=<this skill's folder>/scripts/distill.py` then `python3 $S ...`. With a wrapper, "this
  skill's folder" is ambiguous (wrapper folder vs canonical folder), and the script is now `distill.mjs`
  (Phase 2).
- `SKILL.md:386-388`: mirrors at `<hub>/runtime/distill-lab/mirrors/<source-id>`; `/runtime/` is already ignored
  in every hub (`internal/workspace/workspace.go:39`).
- SKILL.md mentions no `skillhub` command or MCP tool today (`grep -n -i "skillhub\|skill_\|mcp"` finds only
  `$SKILLHUB_WORKSPACE` at `:292`).
- Hub marker: `.skillhub/schema-version` (`internal/workspace/workspace.go:75`).

## Owned files

- Modify (same edit in both trees, kept byte-identical): `.agents/skills/distill-lab/SKILL.md` (the editable
  source in this repo), `internal/systemskills/distill-lab/SKILL.md`.
- Modify: `internal/systemskills/embed_test.go` (version assertion and the self-containment grep test).

## Requirements

The skill must be self-contained (plan decision 12): it needs only Node 18 or newer and `git`, never calls the
`skillhub` binary, an MCP tool or the network, and never hard-codes a personal path.

1. Frontmatter: add `version: 0.1.0` (matches `DistillSkillVersion`). Keep `name` and `description` unchanged.
2. Hub root rule (replaces line 292), in this order: a hub folder the user names explicitly; then
   `$SKILLHUB_WORKSPACE` when set; then the nearest folder at or above the current working directory that
   contains `.skillhub/schema-version`; otherwise stop and ask the user for the hub folder. Never use a personal
   default path. Same order as the script's `--hub` lookup.
3. One "Running the script" section holds every script invocation: the script is `scripts/distill.mjs` in this
   skill's own folder; when Skill Hub installed it, that is `<hub>/.agents/skills/distill-lab/scripts/distill.mjs`.
   Set `S=` to it in the example block and show `node $S check|format|list|decide ...` plus `--hub`. Every other
   section refers to that section instead of repeating command lines (replaces `:341-354` and any other
   `python3` line). Mention that Skill Hub installs the skill only in the hub folder and that the copy under
   `.claude/skills/` is a pointer to `.agents/skills/distill-lab/`, but the skill does not depend on Skill Hub.
4. Requirements section (short): Node 18 or newer and Git, nothing else; no `npm install` (js-yaml is vendored
   in `scripts/vendor/`). The command is `node` on every OS. Say which commands need a hub (`check` and `format`
   without `--no-where`) and that the others work on any `distill.yaml`.
5. If Phase 2 took the parity fallback, add its one-line "Format changes" note.
6. Mirror the edits into `internal/systemskills/distill-lab/` and run the sync test.

## Tests

- `go test ./internal/systemskills/...`: sync, version assertion, and a self-containment test over every bundled
  distill-lab file except `vendor/`: no `skillhub ` command (the `SKILLHUB_WORKSPACE` variable name is allowed),
  no MCP tool names (`skill_`, `skills/get`), no `~/`, no `python`, no `distill.py`, no `npm install`.
- `grep -n -i "skill-hub\|python\|pyyaml" .agents/skills/distill-lab/SKILL.md` returns nothing.
- Every command shown in the "Running the script" section is executed once against a fixture in a scratch hub
  (Phase 8 repeats this end to end).

## Steps

1. Edit `.agents/skills/distill-lab/SKILL.md`, copy to the mirror, update the tests.
2. Run the tests above; commit `fix(distill-lab): find the hub without a personal path and run the Node script from one place`.

## Risks

- Agents started outside the hub folder without `SKILLHUB_WORKSPACE`: the skill now asks instead of guessing.
  Intended.
- Line-number references inside SKILL.md (if any) shift: grep for `:[0-9]` in the file after editing.

## Rollback

Revert the commit; the bundle returns to the verbatim dev SKILL.md.
