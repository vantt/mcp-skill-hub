# Phases 6 to 9 implementation report: product text, docs, sandbox verification, dev copy

Branch `feat/distill-lab-bundle-and-script`, worktree `/home/vantt/projects/mcp-skill-hub-distill`, on top of
`3c148d0` (phases 1 to 5). Sandbox results are in [verification.md](verification.md).

## Commits

1. `7a82821 fix(systemskills): recognize system skill files checked out with CRLF line endings`
2. `369e122 fix(curation): print a runnable distill-lab command only for an intact skill` (phase 6, Go)
3. `7a91021 fix(web): tell the user to run the distill handoff in the hub folder` (phase 6, web and screenshot)
4. `6d5f192 fix(doctor): print the Node install advice as text, not as a command` (found in phase 8)
5. `816a738 fix(doctor): show removed files and system skill notes after doctor --fix` (found in phase 8)
6. `ff71084 fix(doctor): list a skill file left in place once in the doctor --fix report` (follow-up of 5)
7. `3e991a1 fix(connect): say removed for a skill file a remote connection took away` (found in phase 8)
8. `6bc95b7 fix(init): name a system skill file that init left in place` (found in phase 8)
9. `9b0a3e3 docs: describe distill-lab in the hub and system skills in .agents/skills` (phase 7)
10. `42a798a chore(skills): point the repository distill-lab at the shared Agent Skills folder` (phase 9)
11. this report, `verification.md` and the plan status.

## CRLF line endings

`systemskills.KnownFileContent(id, path, raw)` accepts the bytes or, when they hold CRLF, their LF form;
`SameContent` and `NormalizeLineEndings` back it. Every ownership check in `system-skill-files.go`
(shared writes and removals, full copies, legacy Gemini copies, `expectedSkillChange`, `SystemSkillFileOwned`)
and `wrapperOwned` use it, so a hub cloned with `core.autocrlf=true` is upgraded or cleaned up like LF bytes
(the planned write then stores LF). The distill-lab integrity gate compares the same way. Tests:
`TestKnownFileContentReadsCRLFAsLF` (an edited CRLF copy, a lone CR and another skill's bytes stay unknown)
and `TestCRLFCheckoutOfSystemSkillFilesIsStillOwned` (hub, old layout, ownership by path).

## Phase 6 as built

- `internal/app/distill_lab_intact.go`: `distillLabIntact(root)` reads every bundled distill-lab file with
  `Lstat` on each component (no symlink) and compares with the bundle.
- `review_lessons` (`reviewLessonsAction` in `curation_home.go`): summary "N candidate lessons ready for review
  with the distill-lab skill in the hub folder" (+ "; N is/are high-value"), no "(s)"; when intact,
  `Script`, `File`, `HubRelative` and `Command = "node <script> list <file>"`; otherwise an empty command, no
  structured fields and "; distill-lab in this hub differs from Skill Hub's copy, run `skillhub doctor`".
  `countCandidateLessons` picks the top file only under folders that pass `canonical.ValidID` (new one-line
  export of the existing id pattern). The recommended-summary "lesson(s)" text was made plural-aware too.
- `schemas/curation-home-v1.schema.json`: `script`, `file`, `hub_relative` added to `actionItem` with
  descriptions; `additionalProperties: false`, `required` and `$id` unchanged. Field descriptions live in the
  schema and the Go doc comments; the MCP output schema strips descriptions, so they are not in MCP discovery.
- CLI: `reviewLessonsCommand(action, hub, goos)` in `curation.go`; `status` prints the summary as a bullet and
  the command on its own unwrapped `$` line (wrapping would break copy-paste). The status renderer lives in
  `status_doctor.go` after the onboarding merge, not `curation.go` as the plan said.
- Source watch sentence updated in `internal/app/source.go` and `internal/delivery/cli/source.go`;
  `distill_model.go` comments point at `distill.mjs`; web `HomeAction` type gains the three optional fields.
- Web: handoff brief, `INTRO_BRIEF`, `NOTE_DECIDE` as specified; tests updated; e2e anchor plus "Skill Hub
  folder" in `tier3.spec.ts`. `docs/images/web/distill-handoff.png` re-shot with `make web-shots
  SHOTS_OUT_DIR=<scratch>`; only that image and its manifest height (693 -> 774) were copied.
- Tests: `curation_home_test.go` (command, fields, summary, tampered and missing script, CRLF script, unsafe
  folder names), `distill_lab_intact_test.go` (symlinked `.agents`), `review_lessons_test.go` (status output
  for a hub path with a space, JSON fields, schema with and without the fields, unknown property rejected,
  per-shell table), `hub_status_lessons_test.go` (MCP `hub_status`), `source_test.go` ("hub folder").
- `git grep -n "distill.py" -- internal web/src web/e2e ':!internal/migration/testdata'` still lists two
  places, on purpose: `known-digests.go` explains why no digest of the old dev script exists, and
  `system_skill_git_test.go` uses a hand-copied `scripts/distill.py` as the user's file in an ignored folder.

## Phase 7 as built

Edited: `docs/sources-and-learning.md` (where distill-lab runs, Node 18 and Git 2.45.0, self-contained copy,
network sentence, untrusted-source warning with the residual risk, the `status` command), `docs/web-ui.md`
(two sentences), `docs/user-guide.md` (connect output, "What gets written" table and paragraph, pointer-file
evidence, `--remove` line; anchor kept), `docs/remote-hub-guide.md` (table row), `docs/troubleshooting.md`
(five entries: symlink skip, unknown file, Gemini CLI v0.28.0, Node notice, tracked file),
`docs/installation.md` (requirements sentence), `docs/design/05-curation-lifecycle.md`, `AGENTS.md`
(sources, copy command, js-yaml update pointer, release rule for known digests). `README.md` was touched only
to make its `connect` output example match the real output (the per-agent lines lost "curator skill" and
gained the Agent Skills folder line); no distill sentence was added there.

Deviation: the user guide does not say the wrapper was "checked by hand", because check 12 was not run; it
cites the Claude Code skills documentation instead. Change it once the owner runs check 12.

Validation: relative links and anchors of every edited file resolve; the "does not ship", Python and
"no network" greps return nothing about distill-lab.

## Release-note lines (for the release author)

- Skill Hub installs the distill-lab skill in the hub folder. Its script is now Node (`distill.mjs`, Node 18 or
  newer, Git 2.45.0 or newer), so Python is no longer needed; `skillhub status` prints the exact command.
- System skills are written once under `.agents/skills/`; Claude Code gets a pointer file and the
  `.gemini/skills/system-curator` copy is removed. A Gemini CLI older than v0.28.0 reads only `.gemini/skills`
  and, because Skill Hub does not serve system-curator to Gemini over MCP, no longer sees the curator; update
  Gemini CLI.
- Behaviour change: a system skill file whose content Skill Hub does not recognize is now reported and left
  unchanged; `connect`, `init` and `doctor --fix` skip only that file and apply every other repair. Copies with
  CRLF line endings (a Windows clone with `core.autocrlf=true`) are recognized as Skill Hub's.
- The hub `.gitignore` gains four entries for the generated system skill folders. Existing hubs show
  `gitignore_entry_missing` until `doctor --fix` adds them; commit the `.gitignore` change. If Git already
  tracks a generated file, doctor prints the `git rm --cached` command for it; Skill Hub never runs it.
- After upgrading, `status`, the web Home and `doctor` report host integration missing until `doctor --fix`
  installs distill-lab (and, in projects connected only to Claude Code, the shared curator folder).
- JSON items for skill files now carry the file path in their ID (`host_<host>_<kind>:<path>`), and the shared
  `.agents/skills` files appear under `agents`.
- The `review_lessons` action carries `script`, `file` and `hub_relative`; its command is printed only when the
  hub's distill-lab matches Skill Hub's copy. `schemas/curation-home-v1.schema.json` gains these three optional
  `actionItem` properties (public contract change; existing documents stay valid). A blocked system skill file
  appears in `status` as the host-integration action with ID `host_integration_blocked`.
- `doctor --fix --json` items in `fixed` may carry `notes` (for example the Gemini CLI note); removed files are
  printed as "removed".
- distill.mjs differs from distill.py on purpose: `--hub` only checks the hub found from the file; the evidence
  check refuses symlinked or Git-tracked mirrors, never fetches (a missing blob is reported as not fetched), and
  times out after 30 s per Git call; writes refuse symlinks and use a random temporary file.
- `distill.yaml` header comments now name `distill.mjs` after the next `format` (only the first comment line
  changes; verified on the fixture).
- Rollback: after downgrading, delete `.claude/skills/system-curator/` and `.claude/skills/distill-lab/` if the
  older binary reports them; it does not know the pointer files.

## Phase 9 as built

`.claude/skills/distill-lab/SKILL.md` is the wrapper a fresh sandbox `init` wrote (byte-equal to
`testdata/wrappers/distill-lab.golden`); `scripts/distill.py` removed; `git ls-files .claude/skills/distill-lab`
lists only `SKILL.md`. `internal/hostintegration/repo_wrapper_test.go` renders the wrapper from
`.agents/skills/distill-lab/SKILL.md` and compares it with the repo file and the golden; its failure message
says how to regenerate both and when to add a known-wrapper digest. No live reference to
`.claude/skills/distill-lab/scripts` remains outside `archive/` and `plans/`. Step 4 (Claude Code in this repo)
was not run, for the same reason as check 12.

For the lead: `docs/plans/handoff-2026-10-10-lead.md:88` ("Never edit distill-lab
(`.claude/skills/distill-lab`)") now names a generated wrapper; the editable source is
`.agents/skills/distill-lab/`. That handoff is the lead's record and was not edited.

## Checks run

`gofmt -l` clean; `go vet ./...` and `GOOS=windows go vet ./...` and `GOOS=windows go build ./...` pass;
`GIT_CONFIG_NOSYSTEM=1 SKILLHUB_REQUIRE_TAGS=1 SKILLHUB_REQUIRE_NODE=1 go test -count=1 ./...` pass;
`make lint LINT_BASE=main` 0 issues; `make web-check` pass (178 unit tests); `make web-e2e` 33 passed, 2 skipped
(screenshot specs); `make web-shots` into a scratch folder pass.

## Open points

- Phase 8 check 12 and phase 9 step 4 (Claude Code following the wrapper) need the owner or lead.
- Fixes 5 and 6 add an optional `Notes` field to the onboarding plan's `FixAction`/`FixedItem`
  (`internal/app/health.go`) and pass a `*FixAction` into `reconnectLocal`; tell the onboarding owner.
- Seen, not changed (onboarding scope): `connect --remove` in the hub deletes its committed `CLAUDE.md`,
  `AGENTS.md` and `GEMINI.md`, and a following `init`/`doctor` neither restores nor reports them; an old hub
  shows "Workspace invalid" in `status` while only `.gitignore` entries are missing, and its "Agent connection"
  check is skipped until that repair; `connect -g` on a symlinked `~/.claude` still stops as before.
