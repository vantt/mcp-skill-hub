# Phase 2: Checkout, commit and create in the project

## Goal

A person or an agent edits and tries a skill in a real project, with every file including scripts, while the hub keeps
serving the released version. One `skill commit` brings the change back as one reviewed change and one hub commit.
A folder that was never checked out becomes a new skill through the same command. `skill create` scaffolds the folder
in the project. This phase covers a local hub only; phase 5 adds the remote hub.

## Decisions implemented

- **D1**: checkout copies the whole skill into the project, the hub stays unchanged until the commit is approved,
  the commit carries a full diff with a three-way merge when the hub changed, and drafts can be tried the same way.
- **D2**: the `checkout` and `commit` names, one hub Git commit with only the skill's files (phase 1 helper), a folder
  that was never checked out means a new skill, and the checkout folder added to the project's `.gitignore`.
- **D7 (folder part)**: every file listed, excluded files named (shared folder-import filter from the P0 plan),
  routing suggestions from the description with `--edit-routing`, and on a TTY the y/N chain (draft → approve only
  for third-party → activate) in one commit. Without a TTY it stops at draft. Committing the same folder again is an
  update with a diff.
- **D8**: `skill create <id>` writes `.claude/skills/<id>/SKILL.md` in the project after a 3-4 question form. Agents
  write the folder themselves and run `skill commit`. `status` does not report `FAILED` for a stale index after a
  direct edit.
- **D9 (commit part)**: committing a checkout of a third-party skill approves the new content in the same step, so the
  skill is never blocked. Every commit says who gets the new version and when (phase 1 line).

## Prerequisites

- Phase 1 (scoped commit, deferred chain commit, y/N helper).
- [findability-lint phase 1](../261014-1545-findability-lint/phase-01-decode-reviewed-skill.md):
  `catalog.NormalizeSkillDocument` and working review lint. The commit preview lints the proposed skill through it.
- P0 plan: the shared folder-import filter and exec-bit preservation.

## Context

- Multi-file proposals already work: `mutation.Change{Path, BeforeDigest, Contents, Delete}`. `skill_add`
  (`app/skill_add.go:547-656`) and `upstream_update` (`buildUpstreamFileChanges`, `:636`) build them.
- Merge: `classifyAllFiles(ctx, root, updateFileSets{Base, Local, Upstream}, resolutions)`
  (`app/upstream_update.go:494-501`) and `gitMergeFile` (`upstream_merge.go:76`). Statuses are upstream-centric
  (`upstream_only`, `local_only`, `both_changed`, …). `buildUpstreamFileChanges` also rewrites tracked-origin meta,
  which commit must not do.
- Base reconstruction: `reconstructSkillFilesAtCommit` (`app/upstream.go:295`). Served snapshots:
  `runtime/cache/skills/<id>@<digest16>`, with marker `.skillhub-snapshot.json` (`skill_snapshot.go:58, 268-279`).
  They are garbage-collected when not current (`:838`), so they are a fallback, not the source of truth.
- Hub meta lives in `.meta/` or `skill.meta.yaml` (`workspace.IsHubMeta`, `workspace.go:694`). It holds lifecycle,
  origin, routing and `quality.content_reviewed_digest`. `ContentDigest` excludes hub meta (`skillruntime/trust.go:53`).
- Trust: `IsThirdParty` (`trust.go:35-40`) by origin `github`/`git`, upstream or source id. A content change makes the
  approval stale automatically.
- Guided create: `skill_create_prompt.go:163-231` (TTY on both streams; writes to the hub with
  `ConfirmSkillMutation`). Default collection: `core` here (`:118-119`) and `default` in add (`skill_add.go:256`).
- Stale index after a direct edit: `health_hub.go:286-293` marks `hub_index_stale` as a problem. `severityWord` prints
  `FAILED` (`cli/status_doctor.go:270-273`).
- Status items come from `CurationHome.Actions` (`app/curation_home.go`) and are rendered in `renderHubStatus`
  (`status_doctor.go:196-248`). The schema is `schemas/curation-home-v1.schema.json`.

## Design

### Checkout (`skill checkout <id>`)

1. Resolve the project root (open question 7: the Git top level of the current directory, else the current
   directory). Refuse when `.claude/skills/<id>/` already exists. If it has a marker, the FIX says "already checked
   out: edit it, then `skillhub skill commit <id>`". If it has no marker, the FIX says
   "`skillhub skill commit .claude/skills/<id>` adds or updates it".
2. Read the skill's content files from the hub working tree. Hub meta is skipped (open question 9). File modes are
   kept with the P0 helper.
3. Write the files, then `.skillhub-checkout.json` (open question 8). `base_version` is the skill's manifest digest.
   `base_commit` is hub `HEAD` when `DirtyPaths` on the skill folder is empty, else empty.
4. Append `/.claude/skills/<id>/` to the project `.gitignore` under `# skillhub checkout`, unless a line already covers
   it (open question 10).
5. Checkout writes only in the project, so it needs no y/N prompt and makes no hub commit.

### Commit (`skill commit <id|folder|SUB-id>`)

Argument resolution: an existing directory is a folder. A `SUB-` prefix is a submission (phase 5). Anything else is
an id, read from `<project>/.claude/skills/<id>/`.

Read the folder with the shared folder-import filter. The marker and any hub meta are always excluded and listed.
Then pick one of three cases:

| Case | Base | Merge | Proposal |
|------|------|-------|----------|
| Marker, hub version == `base_version` | not needed | none | update: folder vs hub |
| Marker, hub changed | `base_commit` via `reconstructSkillFilesAtCommit`; else the snapshot dir `<id>@<digest16>`; else search the last 100 commits of the skill path for a matching manifest digest | three-way: base, hub (as "local"), folder (as "upstream") through `classifyAllFiles` | update with merged files. Conflicts are reported per file with `--accept <path>=hub\|project`, `--manual`, `--write-conflicts` (open question 13). If the base is not found, every file that differs is reported as a conflict |
| No marker, id unknown | — | — | new skill: draft in collection `default` (or `--collection`), origin `local` |
| No marker, id exists | the hub's current version | none | update that replaces the hub copy, with the warning in open question 12 |

- Build the proposal in a new `internal/app/skill_commit.go`. It reuses the change builder of `skill_add` for new
  skills and a new `buildSkillTreeChanges(current, proposed)` for updates (adds, modifications and deletes; hub meta
  untouched except routing changes from `--edit-routing` and flags). Lifecycle, origin, upstream and approval fields
  are never taken from the folder.
- Map the merge statuses to project words in the output: "changed in the project", "changed on the hub",
  "changed in both, merged", "needs a decision". Do not rename the upstream statuses inside `upstream_update.go`.
- Preview contents: every file with A/M/D and +/- lines (or size for a new skill), excluded files with their reason,
  routing lint for the proposed skill (via `NormalizeSkillDocument` and `LintTargetSkill`), routing suggestions for a
  new skill or one without routing (below), the trust line for third-party skills, and the `--diff` hint.
- Apply on a TTY uses the phase 1 prompt and the deferred chain commit:
  - **Update of a first-party skill:** `Apply this change? [y/N]`, then one commit.
  - **Update of a third-party skill:** `Apply this change? [y/N]`, then
    `Approve this content for agents? It is third-party (<origin>). [y/N]`. Both answers land in one commit. If the
    person answers "n" to the approval, nothing is written, and the output says why: applying without approval would
    stop agents from getting the skill. This keeps D9's "never blocked".
  - **New skill:** `Add <id> as a draft? [y/N]`, then `Activate <id> now? [y/N]` (asked only when activation readiness
    passes with the accepted routing). One commit.
- Without a TTY, `--yes` applies an update or adds a new skill as a draft (the chain stops at draft). For a
  third-party update it refuses unless `--approve-content <digest>` is given (open question 16). That flag is denied
  for agents by the existing rule.
- After apply, the marker is rewritten with the new `base_version` and `base_commit` (open question 11). A folder that
  was never checked out gets a marker and the `.skillhub-checkout.json` ignore pattern (open question 10).
- `--no-commit`, `--diff` and `--json` behave as in phase 1. The JSON preview adds `files[]`
  (`path, change, added, removed, size`), `excluded[]` (`path, reason`), `merge[]` (`path, status, action`),
  `routing_suggestions` and `checkout` (`path, base_version`). This is additive to `SkillProposal`.

### Routing suggestions (shared with phase 3)

New file `internal/skill/routing_suggest.go`: `SuggestRouting(description, skillMD string) RoutingInput`. It is
deterministic, with no model call (open question 14):

- Sentences starting "Use when/for …" become triggers. "Do not use / Not for …" becomes not_for.
- `## When to use`, `## When not to use` and `## Examples` list items become triggers, not_for and examples.
- `min_scope` defaults to `single_step`.

Existing flags override. `--edit-routing` (TTY only) opens `$EDITOR` on a YAML of the suggestions, reusing the editor
plumbing of `skill_editor.go`.

### Create (`skill create <id>`)

- On a TTY, ask 3-4 questions: what it does, when to use it (repeat), when not to use it (repeat), one or two examples.
  Then write `.claude/skills/<id>/SKILL.md`: frontmatter `name` and `description` (what it does plus "Use when …"),
  and a body with `## When to use`, `## When not to use`, `## Examples` and a starter instructions section.
- Without a TTY, the existing flags fill the same scaffold (open question 15).
- Nothing is written to the hub. The output ends with "Try it: ask your agent to use /<id>. When ready:
  skillhub skill commit <id>". The folder gets no marker, so its first commit is a new skill.
- Refuse when the folder exists. `skill_create_prompt.go` keeps its questions and drops the hub write.

### Status

- `GetCurationHome` gains `open_checkout` actions when run inside a project. It scans `.claude/skills/*/
  .skillhub-checkout.json` for the current hub and reports changed-file counts against `base_version`, and whether
  the hub changed since.
- `hub_index_stale` caused by a canonical digest change becomes a notice: "Search index is behind file edits; it
  rebuilds on the next read." A corrupt or missing generation stays a problem.
- `schemas/curation-home-v1.schema.json` gains the new action kind.

## Files

- Create `internal/app/skill_checkout.go`, `skill_checkout_test.go`, `skill_commit.go`, `skill_commit_test.go`, and
  `internal/app/checkout_marker.go` (read and write the marker; a small struct, also used by status and phase 5).
- Create `internal/skill/routing_suggest.go` and `routing_suggest_test.go`.
- Modify `internal/app/upstream_update.go` only to export a role-neutral wrapper around `classifyAllFiles`, with no
  behaviour change. Modify `internal/app/skill_add.go` only to export the new-skill change builder for reuse (phase 3
  owns the rest of this file).
- Modify `internal/app/curation_home.go`, `internal/app/health_hub.go:286-293`,
  `internal/delivery/cli/status_doctor.go`, and `schemas/curation-home-v1.schema.json`.
- Create `internal/delivery/cli/skill_checkout.go` and `skill_commit.go` with tests. Modify `internal/delivery/cli/skill.go`
  (dispatch, flags `--edit-routing`, `--collection` on commit, merge flags), `skill_create_prompt.go` (target the
  project), `skill_editor.go` (routing YAML), and `help.go`.
- Update `docs/contracts/error-codes.md` if new refusals need codes (proposed: reuse `skill_conflict` for "already
  checked out" and `content_review_required` for the non-TTY third-party refusal).

## Steps (tests first)

All tests use a temp hub (`skillhub init` in a temp dir), a temp project, and isolated `HOME`, `XDG_*` and
`GIT_CONFIG_GLOBAL`.

1. Marker tests: write, read, and reject an unknown `schema_version`.
2. Checkout tests: all content files copied with their modes, and no hub meta. The marker has `base_commit` = hub HEAD.
   `.gitignore` gets one line, and a second checkout of another skill adds a second line without duplicates. The
   hub tree and HEAD are unchanged.
3. Commit tests for each case in the table:
   - unchanged hub: modify `SKILL.md`, add `scripts/new.sh` with the exec bit, delete `references/old.md`. The
     preview lists A/M/D. Apply makes one hub commit with exactly those paths plus the receipt, and the exec bit
     is kept.
   - hub changed after checkout (a commit on a different line of `SKILL.md`): merged, and both edits kept.
   - same line changed: a conflict with `--accept SKILL.md=project` resolving it.
   - base unavailable (snapshot removed and `base_commit` empty): conflicts listed, and nothing written.
   - folder with `.env`, `node_modules/x` and a fake `.meta/skill.yaml` claiming `state: active` and a reviewed
     digest: all three excluded. The new skill is a draft and unapproved where that applies.
   - re-commit of the same folder: an update with a diff, never `skill_conflict`.
   - third-party skill (fixture with origin `github`): the TTY chain approves in the same commit, and `skill_get`
     on the runtime profile serves the new content right after. Non-TTY without `--approve-content` is refused, and
     the hub is unchanged.
   - runtime keeps serving the old version until confirm: open a runtime MCP session, checkout, edit, and assert
     `skill_get` content is unchanged until the commit.
4. Implement checkout, the marker, then commit (update, merge, new skill), then the chain.
5. Routing suggester table tests, then the `--edit-routing` test with a fake `$EDITOR` script.
6. Create tests: the TTY form writes the scaffold in the project only (the hub tree is byte-identical). The flag form
   writes the same. An existing folder is refused.
7. Status tests: `open_checkout` lists a checkout with "2 files changed". A direct edit in the hub gives a notice,
   not `FAILED`. The schema test passes.
8. CLI golden updates, reviewing each diff.

## Mock outputs

```text
$ skillhub skill checkout review-checklist
Copied review-checklist (active, 4 files) to .claude/skills/review-checklist/
Added .claude/skills/review-checklist/ to .gitignore.
The hub keeps serving the current version; agents in other projects are not affected.
Try it: ask your agent to use /review-checklist.
When ready: skillhub skill commit review-checklist
```

```text
$ skillhub skill commit review-checklist
review-checklist: 3 files changed since checkout (the hub did not change)
  M SKILL.md                    +3 -1
  A scripts/lint-diff.sh        +12
  D references/old-notes.md     -20
Left out: .env (looks like a secret)
Routing lint: no findings.
Show the diff: skillhub skill commit review-checklist --diff
Apply this change? [y/N] y
Committed 8c01d4e "Commit review-checklist from a project (3 files)".
Agents using this hub get it now; other clones after `git -C ~/skill-hub push`.
```

```text
$ skillhub skill commit pdf
pdf: 1 file changed in the project; the hub changed 1 file since checkout
  M SKILL.md            changed in both, merged          +2 -0
  M scripts/fill.py     changed on the hub, kept         (no project change)
pdf is third-party (github.com/anthropics/skills). The new content needs your approval;
without it agents could not use pdf.
Show the diff: skillhub skill commit pdf --diff
Apply this change? [y/N] y
Approve this content for agents? [y/N] y
Committed 51be7a2 "Commit pdf from a project (1 file), approve content".
Agents using this hub get it now; other clones after `git -C ~/skill-hub push`.
```

```text
$ skillhub skill commit .claude/skills/team-release
team-release is not on the hub: this adds a new skill (collection: default).
Files (2, 227 B):
  SKILL.md            181 B
  scripts/bump.sh      46 B
Left out: .env (looks like a secret)
Routing, suggested from the description (edit with --edit-routing):
  trigger   "cut a release", "bump the version"
  not for   "publish a package to npm"
  examples  "cut release 1.4 for the api"
  min scope single_step
Routing lint: 1 warning: only 1 example; 3 are recommended.
Add team-release as a draft? [y/N] y
Activate team-release now? [y/N] y
Committed 0f9e2b3 "Add team-release from a project, activate".
Agents using this hub get it now; other clones after `git -C ~/skill-hub push`.
```

```text
$ skillhub skill create deploy-notes
What does this skill do? Summarize a deploy for the team channel
When should an agent use it? (empty line to finish) after a production deploy
When should it not? (empty line to finish) for staging deploys
One or two example requests: write the deploy notes for today's release
Created .claude/skills/deploy-notes/SKILL.md (nothing changed on the hub).
Write the instructions there and try them: ask your agent to use /deploy-notes.
When ready: skillhub skill commit deploy-notes
```

```text
$ skillhub status            (inside the project)
…
Open checkouts in this project
  - review-checklist: 3 files changed since checkout · skillhub skill commit review-checklist
```

## Validation

```bash
go test ./internal/skill -run 'SuggestRouting' -count=1
go test ./internal/app -run 'Checkout|SkillCommit|CheckoutMarker' -count=1
go test ./internal/app -run 'Upstream|SkillAdd' -count=1        # no regression in the shared builders
go test ./internal/delivery/cli -run 'Checkout|Commit|Create|Status' -count=1
go test ./internal/delivery/cli -run 'TestStatusJSONValidatesAgainstVersionedSchema' -count=1
make check
```

## Risks

- **Content smuggled through hub meta or the marker.** Tests prove both are excluded, and lifecycle and approval come
  only from the hub.
- **The merge engine is upstream-centric.** Wrap it; do not rename its statuses. Upstream tests must stay green.
- **Losing the base** when the skill was dirty at checkout and the snapshot was GC'd. Then the commit reports every
  differing file as a conflict, so nothing is overwritten silently.
- **Two copies drift** (a forgotten checkout). `status` lists open checkouts.
- **Native skill loading differs by host.** Claude Code loads `.claude/skills/<id>`. Other hosts may not, which is
  out of scope (open question 7).

## Rollback

Revert the phase commits. `skill create` goes back to the hub. Markers and `.gitignore` lines in projects are
harmless leftovers.

## Effort

4 days.
