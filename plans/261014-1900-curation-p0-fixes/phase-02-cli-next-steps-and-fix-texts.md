---
phase: 2
title: "CLI next steps, commit hints, FIX texts, default collection, edit trust warning, file counts"
status: pending
effort: 0.75d
depends_on: []
---

# Phase 2: next steps, FIX texts and counts outside `skill add`

## Context

Reproduced on 2026-10-14 (scratch hub):

```text
$ skillhub skill edit team-release --trigger "cut a release" --not-for deploying --min-scope single_step --yes
Draft skill team-release saved.
Next: edit the instructions with `skillhub skill edit team-release --editor`, then preview activation ...
$ skillhub skill activate team-release --yes
Skill team-release is now active.
Next: ask your agent to use it, or inspect with `skillhub skill show team-release`. Commit with
      `git -C /tmp/.../hub commit`.
$ skillhub status
Next: commit the changes of the hub (a Git command):
  $ git -C /tmp/.../hub add -- history skills && git -C /tmp/.../hub commit -m 'Update skills'
$ skillhub skill review team-release
Next: Stage the skill folder with `git -C <ws> add <skill folder>`, then commit it with `git -C <ws> commit`.
$ skillhub skill activate kafka-review --yes      # untouched scaffold
FIX: Preview the fix with `skillhub skill edit kafka-review `, review and confirm it, then preview ...
```

D3 will later commit automatically; until then every hint must be the command `status` prints.

## Items, evidence and fixes

### 2.1 Commit hints omit `git add` or print placeholders (report CLI UC1a pain 3, P0 #5; web UC4 pain 3; user item 4)

**Evidence.**
- `internal/delivery/cli/skill.go:755-767` (`nextStepForSkill`): "Commit with `git -C <path> commit`", no `add`, path
  not quoted. A new skill's files are untracked, so that commit records nothing.
- `internal/app/skill_review.go:618-620` (`computeNextAction`, active and Git dirty): the literal
  "`git -C <ws> add <skill folder>`" and "`git -C <ws> commit`". The web shows this text in the skill header.
- The correct command already exists: `commitCommand(root, files)` in `internal/app/health_hub.go:537-556`, used by
  `skillhub status`.

**Fix.**
1. In `health_hub.go`, split `commitCommand` into an exported `GitCommitCommand(root string, paths []string)
   string` (same output: changed top-level folders by name, quoted, `add -- ... && commit -m 'Update skills'`)
   and keep `commitCommand` as its `[]DiffFile` adapter, so `status` output is byte-identical.
2. `nextStepForSkill` uses `app.GitCommitCommand(workspacePath, result.ChangedPaths)` for the active, deprecated,
   archived and default branches ("Commit with `<command>`.").
3. `computeNextAction` receives `root` and the skill's dirty paths (`git.Staged`, `Unstaged`, `Untracked`) and
   returns "Commit the skill with `<GitCommitCommand(root, paths)>`."

**Tests first.**
- `internal/delivery/cli/skill_test.go`: after `create --yes` then `activate --yes` in a temp hub, stdout contains
  the command `skillhub status` prints for the same hub (`git -C <root> add -- history skills && git -C <root>
  commit -m 'Update skills'`, root as `shellquote.Arg` renders it), compared by calling `app.GitCommitCommand`.
- `internal/app/skill_review_test.go`: an active skill with an uncommitted file has a `NextAction` with no `<ws>`,
  no `<skill folder>`, and the root as `shellquote.Arg` renders it.
- `internal/app/health_test.go` (existing status tests): unchanged `status` output is the regression guard.

### 2.2 Draft "Next" always says `--editor`, also after approve or routing edits (report CLI UC1b pain 4, P0 #9)

**Evidence.** `nextStepForSkill` (`internal/delivery/cli/skill.go:758-759`) matches the summary text "Draft skill
... saved" and always prints "edit the instructions with `--editor`". Reproduced after a routing-only edit; the
report saw it after `--approve-content`. The review already computes the right step (`computeNextAction`,
`internal/app/skill_review.go:600-610`: scaffold, missing fields, or activate).

**Fix.** For a draft result, `writeSkillMutation` takes the next step from `service.ReviewSkill(ctx, workspace,
id).NextAction` (offline, read-only) instead of matching the summary. The approve flow itself is unchanged.

**Tests first.** CLI test: create with full routing and real content, then an edit `--yes` → Next names
`skillhub skill activate <id>`, not `--editor`; a scaffold draft still names `--editor`.

### 2.3 Activate FIX for untouched scaffold names no flag (report CLI UC2 pain 3, P0 #6; user item 7)

**Evidence.** `internal/delivery/cli/skill.go:788-808`: the switch maps trigger, not_for and min_scope to flags;
the `content (replace untouched scaffold instructions)` requirement maps to nothing, so the FIX reads
"`skillhub skill edit kafka-review `".

**Fix.** Add a case for `content`: `--editor` (and, in the same FIX, "or `--content-file <file>`"). When every
missing item maps to nothing, never print an edit command with no flag.

**Tests first.** CLI test: activate an untouched scaffold → stderr FIX contains `--editor` and `--content-file`.

### 2.4 Editing an approved third-party skill gives no warning (report CLI UC3 pain 2, P0 #4 first half)

**Evidence.** `skill edit` preview (`internal/app/skill_lifecycle.go:100-130`, printed by `writeSkillPreview`,
`internal/delivery/cli/skill.go:702-729`) says nothing about trust, while the update preview warns
(`internal/delivery/cli/skill_upstream.go:450-453`) and the web resource editor warns
(`web/src/screens/skill-detail/ResourceFileView.tsx:39`).

**Fix.** In `PreviewUpdate`, when the skill is third-party (`skillruntime.IsThirdParty`) and the proposal changes
any non-metadata file or the runtime block, add `Warning{Code: "content_review_required_after_apply", Summary:
"After applying, agents get no content from <id> until you approve it again: skillhub skill review <id>."}`.
`writeSkillPreview` prints `preview.Warnings`. JSON gains a warning entry (addition). The resolver is not changed.

**Tests first.** App test: a GitHub-origin approved skill, `--content-file` edit → warning present; a routing-only
edit → absent. CLI test: the warning text appears in the text preview.

### 2.5 Default collection differs: `add` uses `default`, `create` uses `core` (report CLI UC1a pain 5, web UC1 pain 4; user item 10)

**Evidence.** `default`: `internal/app/skill_add.go:255-258`, `:941`, `:986`, `internal/app/skill_discovery.go:394`.
`core`: `internal/delivery/cli/skill_create_prompt.go:136` (guided form when the hub has zero or several
collections), `internal/delivery/web/routes_skill_write.go:148-150`, `internal/delivery/mcpserver/skill_tools.go:33-36`,
`web/src/screens/skill-create/CreateSkillScreen.tsx:30,36,48` (list and initial value `core`, hint "Pick core if
unsure").

**Fix.** One constant `skill.DefaultCollection = "default"` in `internal/skill`; the three Go `core` defaults use
it; the web create form starts at `default`, lists it first, and the hint says "Pick default if unsure". The
non-TTY `skill create` still requires `--collection` (no flag change). Explicit `--collection core` keeps working.
MCP tool descriptions are not touched (no `tools/list` digest change). See plan open question 5.

**Tests first.** `skill_create_prompt_test.go`: guided create on a hub with two collections defaults to
`default`. `routes_skill_write_test.go`: a create preview without collection targets `skills/default/`. MCP test:
`skill_create_preview` without collection targets `skills/default/`. Web vitest: the create form's collection
starts at `default`.

### 2.6 Review counts hub metadata as a file (report CLI UC1a pain 4; web UC2 pain 4; user item 10)

**Evidence.** `inventorySkillResources` (`internal/app/skill_review.go:348-387`) counts every file, including
`.meta/skill.yaml`; CLI review prints it (`internal/delivery/cli/skill_review.go:41-44`): "4 files" for a skill whose
user files are `SKILL.md`, `scripts/bump.sh` and the copied `.env`.

**Fix.** Keep `resource_count` and `total_bytes` as they are (JSON contract). Add `file_count` and `file_bytes`
to `SkillResourceStatus` (additions): the same walk without `workspace.IsHubMeta` paths. CLI review prints
`file_count`/`file_bytes`. Phase 4 makes the web read them. This is the counting rule phase 1 uses in the add
preview.

**Tests first.** App test: a skill with `SKILL.md`, one script and `.meta/skill.yaml` has `file_count == 2` and
`resource_count == 3`. CLI test: review prints "2 files".

## Steps

1. Write the failing tests above; record failures.
2. Export `GitCommitCommand`; switch the CLI and review hints to it.
3. Draft next step from the review; scaffold FIX; edit trust warning.
4. Default collection constant and the four call sites; web create form.
5. `file_count`/`file_bytes` and the CLI review line.
6. After phase 1 merges: in `docs/curating-skills.md`, change `--collection core` to `--collection default` in the
   create examples (lines 78 and 196) and "uses the collection `core`" to `default` (line 191); update any sample
   output in that file that shows the commit hint. Check `docs/command-reference.md` for a stated default (none
   today; lines 38 and 167 show a placeholder).

## Validation

```bash
go test ./internal/app/ -run 'Review|HealthHub|Status|PreviewUpdate' -count=1
go test ./internal/delivery/cli/ -run 'Skill|Create' -count=1
go test ./internal/delivery/web/ -run 'Create' -count=1
go test ./internal/delivery/mcpserver/ -count=1
cd web && npx vitest run src/screens/skill-create
make check && make web-check
```

## Risks

- Exact-text tests in the CLI and web suites that assert today's wording or `core` will fail; update them only where
  they assert the defect, never weaken an unrelated assertion.
- `ReviewSkill` after a draft save adds one offline read per draft mutation (milliseconds); acceptable.
- Changing the create default puts new skills in a second collection on hubs whose skills are all in `core`; the
  guided form already uses the only existing collection, so this affects hubs with several collections and the
  web/MCP defaults only.

## Rollback

Revert the phase's commits. Only additive JSON fields and a warning were added; no stored data changes shape.
