---
phase: 2
title: "distill-lab content: hub path, runnable steps, requirements"
status: pending
priority: P2
effort: "2h"
wave: 2
dependencies: [1]
---

# Phase 2: distill-lab content: hub path, runnable steps, requirements

## Context (traced)

- `SKILL.md:292`: "Hub root: `$SKILLHUB_WORKSPACE`, else `~/skill-hub`." A personal path in a shipped skill.
- `SKILL.md:341-354`: `S=<this skill's folder>/scripts/distill.py` then `python3 $S ...`. With a wrapper, "this
  skill's folder" is ambiguous (wrapper folder vs canonical folder).
- `SKILL.md:386-388`: mirrors at `<hub>/runtime/distill-lab/mirrors/<source-id>`; `/runtime/` is already ignored
  in every hub (`internal/workspace/workspace.go:39`).
- `distill.py:24` imports `yaml` unguarded; `hub_of()` derives the hub as `parents[4]` of the `distill.yaml` path
  (`distill.py:293-295`) and `_git` reads mirrors under it (`:194-201`). The canonical header text
  (`distill.py:273-278`) is copied into fixtures `internal/migration/testdata/*-distill.yaml`; keep it unchanged.
- Hub marker: `.skillhub/schema-version` (`internal/workspace/workspace.go:75`).

## Owned files

- Modify (same edit in both trees, kept byte-identical): `system-skills/distill-lab/SKILL.md`,
  `system-skills/distill-lab/scripts/distill.py`, `internal/systemskills/distill-lab/SKILL.md`,
  `internal/systemskills/distill-lab/scripts/distill.py`.
- Modify: `internal/systemskills/embed_test.go` (only the version assertion).

## Requirements

1. Frontmatter: add `version: 0.1.0` (matches `DistillSkillVersion`). Keep `name` and `description` unchanged.
2. Hub root rule (replaces line 292): use `$SKILLHUB_WORKSPACE` when set; otherwise the nearest folder at or above
   the current working directory that contains `.skillhub/schema-version` (the hub this skill is installed in);
   otherwise stop and ask the user for the hub folder. Never use a personal default path.
3. Script location: state it as `<hub>/.agents/skills/distill-lab/scripts/distill.py` and set
   `S=<hub>/.agents/skills/distill-lab/scripts/distill.py` in the example block. Mention that the skill is
   installed by Skill Hub only in the hub folder and that copies under `.claude/skills/` or `.gemini/skills/` are
   pointers.
4. Requirements section (short): Python 3, PyYAML, Git. On Windows use `py -3` (or `python`) instead of
   `python3`. Install hint: `python3 -m pip install --user pyyaml`.
5. `distill.py`:
   - Wrap `import yaml` so a missing PyYAML exits with the script's existing `ERROR/WHY/FIX` style and the pip
     hint, exit code 2.
   - In `hub_of()`, check that the derived folder contains `.skillhub/schema-version`; if not, exit with
     `ERROR: <file> is not inside a Skill Hub folder` / `FIX: pass the path <hub>/skills/<collection>/<skill-id>/.meta/distill.yaml`.
   - No other behavior change; the canonical header string stays byte-identical.
6. Mirror the edits into `internal/systemskills/distill-lab/` and run the sync test.

## Tests

- `go test ./internal/systemskills/...` (sync + version assertion).
- Manual in a scratch folder: copy `internal/migration/testdata/test-audit-distill.yaml` into
  `<tmp>/hub/skills/default/test-audit/.meta/distill.yaml`, create `<tmp>/hub/.skillhub/schema-version`, run
  `python3 -I internal/systemskills/distill-lab/scripts/distill.py list <that file> --top 5` -> exit 0, numbered
  output; without the marker -> the new error; with `python3 -I -c` simulating no yaml is not needed (read the
  guard).
- `go test ./internal/migration/...` still passes (fixtures unchanged).
- `grep -n "skill-hub" system-skills/distill-lab/SKILL.md` returns nothing.

## Steps

1. Edit `system-skills/distill-lab/*`, copy to the mirror, update the version assertion.
2. Run the tests above; commit `fix(distill-lab): find the hub from the workspace marker and name the script path`.

## Risks

- Agents started outside the hub folder without `SKILLHUB_WORKSPACE`: the skill now asks instead of guessing.
  Intended.
- Line-number references inside SKILL.md (if any) shift: grep for `:[0-9]` in the file after editing.

## Rollback

Revert the commit; the bundle returns to the verbatim dev copy.
