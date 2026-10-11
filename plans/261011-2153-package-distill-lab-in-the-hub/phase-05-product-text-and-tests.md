---
phase: 5
title: "Product text and tests (Go, web, e2e, one screenshot)"
status: pending
priority: P2
effort: "4h"
wave: 1
dependencies: []
---

# Phase 5: Product text and tests (Go, web, e2e, one screenshot)

## Context (traced)

- `internal/app/curation_home.go:479-488`: `review_lessons` command `distill.py list <hub-relative path>`; the path
  comes from `countCandidateLessons` (`:299-303`). Test: `internal/app/curation_home_test.go:616-637`.
- Status renders action commands with `cliNextCommand` (`internal/delivery/cli/curation.go:97-101`); a command not
  starting with `skillhub ` and not a known token is returned as is (`internal/delivery/cli/next_commands.go:156-159`).
  Test file for status next steps: `internal/delivery/cli/workspace_next_steps_test.go`.
- Source watch confirmation: `internal/app/source.go:738` and `internal/delivery/cli/source.go:717` say "Distill
  with distill-lab to write .meta/distill.yaml ..."; test `internal/delivery/cli/source_test.go:214-215` checks
  for "distill-lab".
- Web: `web/src/domain/handoff-brief.ts:10-28` (brief text), test `handoff-brief.test.ts:5-17`;
  `web/src/screens/skill-detail/DistillTab.tsx:10-11` (`NOTE_DECIDE`);
  `web/src/screens/distill/DistillHandoffScreen.tsx:24-25` (`INTRO_BRIEF`), test `distill.test.tsx:110,121`;
  e2e `web/e2e/tier3.spec.ts:45,64`, `web/e2e/journeys.spec.ts:229` assert "Use the distill-lab skill".
- `internal/distill/distill_model.go:39` says the format follows `.claude/skills/distill-lab/scripts/distill.py`.
- Screenshot `docs/images/web/distill-handoff.png` shows `INTRO_BRIEF`; the harness re-shoots all images into a
  folder (`make web-shots SHOTS_OUT_DIR=...`, see `archive/plans/261011-1432-readme-and-user-docs-rewrite/phase-11-home-copy-fix.md`).

## Owned files

See plan.md wave 1, Phase 5 row. Read-only: `internal/hostintegration/**`, `internal/systemskills/**`, `web/e2e/ux/*`.

## Requirements

Script path (fixed by plan decision 1): `.agents/skills/distill-lab/scripts/distill.py`, relative to the hub.

1. `curation_home.go`: command becomes `python3 .agents/skills/distill-lab/scripts/distill.py list <path>` (path
   unchanged, hub-relative). Summary text: "%d candidate lesson(s) ready for review with the distill-lab skill in
   the hub folder" (+ the high-value variant). Keep `Kind`, `ID`, `Count`, `Priority`.
2. `curation.go`: for `action.Kind == "review_lessons"` print `cd <shellArgument(workspace)> && <command>`; all
   other kinds unchanged.
3. `source.go` and `cli/source.go` (same sentence in both): "Watching <id>. To learn from it, start an agent in
   this hub folder and ask it to distill with the distill-lab skill; it writes .meta/distill.yaml. Porting lessons
   to skill content uses skill_update.\nWatching does not auto-import skills."
4. `handoff-brief.ts`: keep the first line "Use the distill-lab skill ..." (e2e anchor). Add before the list:
   "Run this in an agent started in the Skill Hub folder; Skill Hub installs the distill-lab skill there." Step 2:
   "Record the lessons ... with the skill's script (python3 .agents/skills/distill-lab/scripts/distill.py)."
5. `DistillHandoffScreen.tsx` `INTRO_BRIEF`: "Copy this handoff and paste it to an agent you start in your Skill
   Hub folder, where Skill Hub installs the distill-lab skill. It records lessons in each skill; read them in the
   skill's Distill tab, where you decide what to keep."
6. `DistillTab.tsx` `NOTE_DECIDE`: replace the script sentence with "Or, in your hub folder, run python3
   .agents/skills/distill-lab/scripts/distill.py decide <file> <number> --state planned|ported|rejected."
7. `distill_model.go:39`: point the comment at `system-skills/distill-lab/scripts/distill.py`.
8. Screenshot: re-shoot into a scratch folder and copy only `distill-handoff.png` and its manifest entry (same
   recipe as the archived Phase 11); skip with a note if the harness fails, and report.

## Tests

- `curation_home_test.go`: exact new command and summary.
- `workspace_next_steps_test.go`: status output shows `cd <ws> && python3 .agents/...distill.py list ...` with a
  workspace path that contains a space (quoted).
- `source_test.go`: still contains "distill-lab"; add "hub folder".
- `handoff-brief.test.ts`: contains "Use the distill-lab skill", "Skill Hub folder", ".agents/skills/distill-lab/scripts/distill.py".
- `distill.test.tsx`: intro mentions the hub folder.
- e2e: existing anchors unchanged; add one assertion for "Skill Hub folder" in `tier3.spec.ts`.

## Steps

1. Go changes and tests; `go test ./internal/app/... ./internal/delivery/cli/... ./internal/distill/...`.
2. Web changes and tests; `make web-test`; `make web-e2e` (or the repo's e2e target) for the two specs.
3. Screenshot refresh.
4. Commits: `fix(curation): print a runnable distill-lab command from the hub folder`,
   `fix(web): tell the user to run the distill handoff in the hub folder`.

## Risks

- JSON/MCP consumers that parsed the old `distill.py list` command: the field is free text; documented in the
  release notes by the docs phase.

## Rollback

Revert the commits (code, tests and image together).
