---
title: "Curation UX redesign: checkout, commit, auto-commit and review"
description: "Implement design decisions D1-D9: every confirmed change becomes one hub Git commit, terminals confirm with y/N, skills are edited and tried in a project checkout and brought back with skill commit, the web edits whole skills and approves content, and remote machines can submit skills to a review queue."
status: pending
priority: P1
effort: 15d
branch: (each phase gets its own worktree)
tags: [feature, backend, frontend, cli, curation, contract]
blockedBy: [261014-1900-curation-p0-fixes, 261014-1545-findability-lint] # findability-lint: its phase 1 only
blocks: []
created: 2026-10-14
---

# Curation UX redesign: checkout, commit, auto-commit and review

The binding decisions are in [design-decisions.md](design-decisions.md) (D1-D9 and "Còn lại sau vòng thiết kế").
This plan does not change any command name, flag, flow or behaviour decided there. Where a decision leaves an
implementation detail open, a phase proposes the smallest option and the detail is listed under
[Open questions](#open-questions). Treat every proposed answer as provisional until the user confirms it.

Evidence: [CLI/agent UX review](../reports/ux-review-261014-1651-curation-cli-agent.md),
[web UX review](../reports/ux-review-261014-1651-curation-web.md), plus a code scout on `main` at `4de8625`.

## Outcome

1. Every confirmed change to the hub creates one Git commit in the hub repository. The commit holds only the files of
   that change, uses a message written by skillhub, and is never pushed. `--no-commit` keeps today's
   behaviour (D3).
2. Every hub-changing command prints a preview. In a terminal with a person, it asks `[y/N]` under the preview. Without
   a terminal, it keeps two steps (preview, then `--yes`). `--diff` shows the full diff, and the hidden
   `--full-diff` is removed (D5).
3. `skillhub skill checkout <id>` copies a whole skill into the project. `skillhub skill commit <id|folder>` brings the
   changes back as one reviewed change, with a three-way merge when the hub changed in the meantime and the
   third-party approval in the same step. A folder that was never checked out becomes a new skill. `skill create <id>`
   scaffolds the folder in the project (D1, D2, D7, D8, D9).
4. `skill add` takes only GitHub URLs, shows every file, the excluded files, the license and routing suggestions, and
   chains add, approve and activate into one commit. Git host parsing sits behind a `HostProvider` port, with GitHub
   as the only provider and unchanged behaviour (D7, "Còn lại").
5. The web edits a skill as a file tree with pending changes, reviews them, and commits them as one change. It also has
   a "Waiting for review" page, a full-skill review screen with Approve, and a "Checkout to a project" command
   (D6, D9).
6. A machine that uses a remote hub can run `skill commit`. The files travel over a plain HTTP endpoint on
   `skillhub serve` into a queue on the hub. A person on the hub machine reviews them. No MCP tool is added (D4).
7. Docs, help and the curator skill tell one story. Direct edits of hub files are not the official path. Editing
   goes through checkout, and quick edits go through the web (D8).

## Constraints

- Git is the database. The only new file outside the hub is the checkout marker in the project. Submissions wait
  under the hub's untracked `runtime/` directory.
- Mutations keep their pins (`proposal_id`, `proposal_digest`, `base_version`) and the single mutation choke point
  (`internal/mutation`, `CommitWithOptions`).
- An agent never approves third-party or submitted content. The `deny` rule on `--approve-content` stays.
- The runtime MCP `tools/list` does not grow. `internal/delivery/mcpserver/tools_size_test.go` budgets stay as they are.
- Tests use temp dirs with isolated `HOME`, `XDG_*` and `GIT_CONFIG_GLOBAL`. They never touch `/home/vantt/skill-hub`,
  the real HOME or real projects.
- No plan IDs or phase numbers in code comments, test names or commit messages.

## Non-goals

- Per-file conflict prompts in the terminal, `--accept-all`, pager output, or merging `outdated`, `upstream` and
  `update` (report flows 4B, 5, 6). Upstream update keeps its flow and only gains D3 and D5.
- Trial scope (report option C), cache garbage collection changes, and receipt de-noising.
- Per-invite submit permission (`token invite --can-submit`). D4 defers it.
- Hardening against an agent that drives a browser or fakes a TTY. D9 accepts this for now; the plan records it as a
  risk.
- GitLab, private repositories, or LAN hosts. The port only makes them possible later.
- The items of `261014-1900-curation-p0-fixes`: the secret and junk file filter, the exec bit, the wrong messages,
  the `workspace_diff` copy button, the `<ws>` placeholder and the `OP-…` toast. This plan builds on that filter.

## What exists (verified at 4de8625)

| Need | Exists | Where |
|------|--------|-------|
| One write path for every managed mutation | `mutation.CommitWithOptions`, exclusive lock, receipt last, catalog published under the lock | `internal/mutation/mutation.go:207`, `transaction.go:37`, `internal/skill/lifecycle.go:494-520` |
| Callers outside skills | sources, workspace remediation, migration | `internal/app/source.go:975`, `workspace.go:292`, `migration.go:90` |
| Git through exec | `offlineGitCommand`, porcelain parsing, `show HEAD:path` | `internal/app/operations.go:156-704` |
| A `git commit` today | only `commitFirst` in `init`, with hooks and signing off | `internal/app/workspace_init.go:445-487` |
| Multi-file proposals (add, modify, delete) | `mutation.Change{Path, BeforeDigest, Contents, Delete}` | `internal/mutation/mutation.go:33-38` |
| Three-way merge | `gitMergeFile`, `classifyAllFiles` over base, local and upstream maps | `internal/app/upstream_merge.go:76`, `upstream_update.go:501` |
| Base files from history | `reconstructSkillFilesAtCommit` | `internal/app/upstream.go:295` |
| Content digest and trust | `ContentDigest` (excludes hub meta), `IsThirdParty`, `Evaluate` | `internal/skillruntime/trust.go:35-111` |
| Served snapshots | `runtime/cache/skills/<id>@<digest16>`, read-only, GC'd when not current | `internal/app/skill_snapshot.go:543-646, 838` |
| Bundle over HTTP | `GET /mcp/bundle` behind the hub bearer token | `internal/delivery/mcpserver/distribution.go:23,123`, `http.go:226-291` |
| Remote-project detection | `FindRemoteEntries`, guard "Skills are curated on the hub machine" | `internal/app/doctor_remote.go:360`, `internal/delivery/cli/workspace_resolve.go:94-108` |
| TTY check | `term.IsTerminal` on stdin and stdout | `internal/delivery/cli/skill_create_prompt.go:27-29` |
| Preview printer | `writeSkillPreview` ("No files changed. Confirm with:") | `internal/delivery/cli/skill.go:702-732` |
| `--full-diff` | parsed at `skill.go:398`, used by create, edit, transitions and add | `skill.go:193, 255, 296`, `skill_add.go:42, 234` |
| Claude Code rules from `connect` | allow `skillhub:*`; ask `* --yes*`, `* confirm *`, ...; deny `* --approve-content*` | `internal/hostintegration/cli-permissions.go:15-26` |
| Web servers | web UI on its own listener (7421), MCP plus bundle on 7420 | `internal/delivery/cli/serve.go:25, 181-270`, `internal/delivery/web/listen.go:14` |
| Runtime draft gate | done: runtime serves only active, servable skills | commit `383b684` |

Missing: a name-based ignore list for folders (the P0 plan adds it), a project-side marker, any y/N prompt, any
routing suggestion from a description, a web approve route, and any submission path.

## Phases

| # | Phase | Decisions | Effort | Depends on |
|---|-------|-----------|--------|------------|
| 1 | [Auto-commit and terminal confirm](phase-01-auto-commit-and-confirm.md) | D3, D5 (and the "who gets it" line of D6/D9) | 2.5d | P0 plan merged |
| 2 | [Checkout, commit and create in the project](phase-02-checkout-commit-create.md) | D1, D2, D7 (folder), D8, D9 (approve in commit) | 4d | 1; findability-lint phase 1; P0 folder filter |
| 3 | [GitHub add flow and the Git host port](phase-03-github-add-and-host-port.md) | D7 (URL), "Còn lại" (port) | 1.5d | 1, 2 (routing suggester) |
| 4 | [Web file tree, commit and review](phase-04-web-files-and-review.md) | D6, D9 | 4d | 1, 2 |
| 5 | [Remote submission](phase-05-remote-submission.md) | D4, D9 (queue items) | 2.5d | 2, 4 |
| 6 | [Docs, help and curator skill](phase-06-docs-and-curator.md) | D8 (one story), all | 1d | 1-5 |

**Order and parallel work.** 1 → 2 → (3 ∥ 4) → 5 → 6.

- Phases 3 and 4 own disjoint files. Phase 3 owns `internal/source/*`, `internal/app/skill_add.go`,
  `source_watch.go`, `source.go`, `source_import.go`, `source_backfill.go` and `internal/delivery/cli/skill_add.go`.
  Phase 4 owns `internal/delivery/web/*` and `web/src/*`. Phase 3 does not change web wording (the URL error text
  stays identical), so it does not touch `web/src`.
- Contract docs (`docs/contracts/*.md`, `schemas/*.json`) are updated in the phase that changes the contract. Phase 6
  owns only narrative docs, help text and the curator skill. Phases 3 and 4 must not both add error codes. Phase 4
  adds none, so `docs/contracts/error-codes.md` has one owner at a time.
- [findability-lint phase 1](../261014-1545-findability-lint/phase-01-decode-reviewed-skill.md) runs unchanged from
  its own phase file, before phase 2 of this plan. It can run in parallel with phase 1. Both touch
  `internal/app/skill_review.go`, but in different functions: lint decode at `:146`, commit hint text at `:613-619`.
  The lead merges them.

## Related plans

- **`261014-1900-curation-p0-fixes`** (being written). It is a prerequisite. Phase 1 here replaces commit hints
  that the P0 plan corrects. Phase 2 reuses the P0 folder-import filter (`.env*`, `*.pem`, `id_*`, `node_modules/`,
  …) and its exec-bit preservation for checkout, commit and submission. This plan calls that filter
  "the shared folder-import filter" and adds no second list.
- **`261014-1545-findability-lint`** stays on hold except phase 1, which this plan pulls in as a prerequisite
  because the `commit` preview must show routing lint (design-decisions "Còn lại"). The preview calls the same review
  lint function, so the later self-resolution warnings (phases 2-4 of that plan) appear in the `commit` preview
  without more work here.
- **`261014-1650-runtime-skill-get-gate`** is completed (`383b684`). Drafts are no longer served at runtime, so
  checkout (phase 2) is now the only way to try a draft in a real agent. That makes phase 2 the most valuable phase
  after the foundation.
- **`261014-1540-session-mining-routing-proposals`** stays on hold. Its edit-preview output gains `--diff` and
  auto-commit from phase 1 for free.
- **`261014-1600-tools-list-budget`** is completed. Phase 5 must keep its budget test green with no change to the
  budgets.

## Contract changes (all listed; each phase repeats its own)

| Surface | Change | Phase |
|---------|--------|-------|
| Mutation result JSON (`SkillMutationResult`, source and migration results) | Adds a `commit` object (`status`, `hash`, `message`, `reason`, `manual_command`, `reach`). Additive | 1 |
| Result text "Git commit remains user-controlled." | Replaced by the commit line | 1 |
| CLI flags | Adds `--no-commit` and `--diff` to every hub-changing command. Removes `--full-diff` | 1 |
| CLI behaviour on a TTY | Asks `[y/N]` after the preview instead of exiting. Exit 0 after "n" with "Nothing written" | 1 |
| Error codes | Refusing when the change's files hold uncommitted edits: proposed reuse of `edit_conflict` (OQ 3) | 1 |
| `skill create` | Writes `.claude/skills/<id>/` in the project, not the hub. Hub-only flags (`--collection`, `--runtime-file`) move to `skill commit` | 2 |
| New commands | `skill checkout <id>`, `skill commit <id\|folder>` | 2 |
| `skill add <folder>` | Refused with a FIX that points to `skill commit <folder>` | 3 |
| `status` JSON (`curation-home-v1`) | Adds action kinds `open_checkout` and `submission_waiting`. Stale index after a direct edit becomes a notice | 2, 5 |
| Web API | Adds file-tree, changes preview, approve, review-queue and submission routes. Removes the single-file resource preview route once the SPA no longer uses it. The proposal confirm body gains `commit_message` | 4, 5 |
| HTTP on 7420 | Adds `POST /mcp/submissions` behind the hub bearer token | 5 |
| Threat model | A stolen hub token can now fill the submission queue. It still cannot change a served skill | 5 |
| MCP | No change. The curation tools commit through the shared mutation path, with no new input field (OQ 4) | 1 |
| Claude Code rules from `connect` | No change. A test proves `skill commit --yes` asks and `--approve-content` stays denied on every subcommand | 1 |

## Acceptance criteria

- [ ] After any confirmed change from the CLI, MCP or web, `git -C <hub> log -1 --name-only` shows one commit with
      exactly the change's files and its receipt, and the rest of the working tree is untouched (other dirty and
      staged files stay as they were).
- [ ] The commit runs the hub's hooks and signing config. When the commit fails (hook, signing without a TTY, no
      identity), the files stay written and the output gives the reason and the exact manual command.
- [ ] A change whose files hold uncommitted edits is refused before anything is written, unless `--no-commit` is set.
- [ ] On a TTY, every hub-changing command asks `[y/N]` under the preview and applies exactly the previewed proposal.
      Without a TTY, or with `--json`, it never prompts. `--diff` prints the full diff. `--full-diff` is an unknown
      flag.
- [ ] `skill checkout` → edit several files, including a script → `skill commit` gives one preview with A/M/D per
      file, line counts, routing lint and the excluded files. The hub keeps serving the old version until confirm.
- [ ] When the hub changed after checkout, `skill commit` merges three ways. A conflict is reported per file with the
      existing `--accept`, `--manual` and `--write-conflicts` choices.
- [ ] Committing a checkout of a third-party skill on a TTY asks to approve the new content in the same step. The
      skill is never left unserved because of this commit. Without a TTY, it is refused unless the person passes
      `--approve-content <digest>`.
- [ ] `skill commit <folder>` without a marker creates a new skill: every file, routing suggestions, then y/N for
      draft and activate. All of it is one commit. Committing the same folder again is an update with a diff, not a
      conflict.
- [ ] `skill create <id>` writes `.claude/skills/<id>/SKILL.md` in the project and nothing in the hub.
- [ ] `skill add <github-url>` lists every file and the license in text mode, suggests routing, and chains draft →
      approve → activate into one commit on a TTY. Without a TTY it stops at draft.
- [ ] `internal/source` resolves GitHub through a `HostProvider` registry. Existing locator tests pass unchanged.
- [ ] The web edits, adds, renames, deletes and uploads files in a tree, reviews all pending changes, and commits them
      once. The bar says "N changes not committed · agents still get the current version".
- [ ] The web "Waiting for review" page lists drafts, skills whose content needs approval, and submissions. Approve
      becomes enabled only after every changed file was opened.
- [ ] From a project connected with `connect --hub`, `skill commit` uploads to the hub queue. Nothing on the hub
      changes until a person approves. `status` on the hub shows the waiting submission.
- [ ] The runtime `tools/list` budget test passes with unchanged budgets.
- [ ] `make check` passes. `make web-check` and `make web-e2e` pass for phases 4 and 5.

## Test matrix

| Area | Unit | Integration (temp hub + temp project) | E2E |
|------|------|---------------------------------------|-----|
| Scoped commit, refusal, hooks, signing failure | `internal/workspace` git commit helper | CLI, MCP and web confirm paths | — |
| y/N prompt | prompt helper with fake TTY streams | every hub-changing command in a table test | — |
| Checkout and commit | marker, merge mapping, hub-meta exclusion | checkout → edit → commit; hub changed → merge; third-party approve | — |
| New skill from a folder, create | routing suggester | create → commit chain → one commit | — |
| GitHub add | host registry | `file://` test fixture route (as today) | — |
| Web files and review | Vitest for tree and pending bar | Go route tests | Playwright: edit, add, rename, delete, review, commit, approve |
| Submission | archive limits and queue caps | serve on a random port, remote project commit, hub review | Playwright waiting page |

## Risks

- **Committing under the mutation lock would deadlock with a pre-commit hook that runs skillhub.** Mitigation: the
  dirty-path check runs under the lock, and `git commit` runs after the lock is released.
- **Signing that needs a passphrase can hang without a TTY.** Mitigation: no stdin, `GIT_TERMINAL_PROMPT=0`, and a
  timeout (proposed 60 s, OQ 2). A failed commit leaves the files written and prints the manual command.
- **Existing hubs have uncommitted skill edits.** The first change to such a skill is refused. Mitigation: the
  refusal names the files and gives the manual commit and the `--no-commit` escape. The release note says so.
- **Checkout marker or hub meta in the folder could smuggle lifecycle, origin or approval fields.** Mitigation:
  `commit` never reads hub meta or the marker as content. Lifecycle, origin, upstream and approval always come from
  the hub.
- **Fake TTY or browser automation lets an agent approve** (D9 accepts this for now). Default Claude Code rules ask
  before `script …` because it does not match `skillhub:*`. This is recorded for the later security work.
- **The web UI binds 0.0.0.0 by default**, so Approve on the web is reachable from the network behind the per-start
  session token. D9 accepts this. It is recorded.
- **The submission queue widens what a stolen token can do** (D4 accepts this). Mitigation: size and count caps,
  the existing rate limit, nothing served before review, and a documented threat-model change.

## Rollback

Each phase is its own set of commits and reverts on its own, in reverse order (6 → 1). Reverting phase 1 restores
"files only" (identical to `--no-commit`). Commits that skillhub already made stay in the hub's Git history and are
ordinary commits. Checkout markers left in projects are inert files once phase 2 is reverted. Pending submissions
under `runtime/submissions/` can be deleted.

## Open questions

Each has the smallest proposed answer. The implementing phase uses the proposed answer only after the user confirms
it.

1. **Commit message format.** Proposed: subject `<Verb> <skill-id>[: <detail>]` (for example
   `Activate review-checklist`, `Commit review-checklist from a project (2 files)`, `Add pdf from github.com/anthropics/skills`),
   with a body trailer `Skillhub-Operation: OP-…`. Web Review can edit the message (D6). The CLI has no message flag.
2. **Signing and hook timeout without a TTY.** Proposed: 60 s, then report "commit did not finish: <reason>", leave
   the files written, and print the manual command.
3. **Error code for "the change's files have uncommitted edits".** Proposed: reuse `edit_conflict` with a new message
   and FIX, instead of a new code.
4. **`--no-commit` outside the CLI.** Proposed: CLI only. MCP curation tools and the web always commit (D3: same
   behaviour everywhere). No MCP schema change.
5. **Prompt wording and default.** Proposed: `Apply this change? [y/N]`. Empty or anything other than `y`/`yes`
   prints "Nothing written." and exits 0. The prompt needs a TTY on both stdin and stdout, and `--json` never prompts.
6. **`--diff` in JSON.** Proposed: same as `--full-diff` today. `full_diff` is filled only with `--diff`, so the JSON
   contract stays the same. MCP keeps its `full_diff` input.
7. **Checkout location and project root.** Proposed: `<project>/.claude/skills/<id>/`, where the project is the Git
   top level of the current directory, or the current directory outside Git. No `--dir` flag for now. Codex
   `.agents/skills` is not written.
8. **Checkout marker.** Proposed: `.skillhub-checkout.json` inside the checked-out folder:
   `{schema_version: 1, skill_id, hub (workspace path or hub URL), base_version (manifest digest), base_commit
   (hub HEAD when the skill was clean, else empty), checked_out_at}`. `commit` never sends it as content.
   `status` finds markers under `.claude/skills/*/`.
9. **What a checkout copies.** Proposed: content files only (`SKILL.md` and resources, as served). Hub meta
   (`.meta/`, `skill.meta.yaml`) stays in the hub. Routing is edited at commit time with `--edit-routing` or the
   existing routing flags. Any hub meta found in a folder is listed as excluded ("managed by the hub").
10. **`.gitignore` lines.** Proposed: checkout appends `/.claude/skills/<id>/` under a `# skillhub checkout` comment
    (creating `.gitignore` if needed) and never removes it. Committing a folder that the project tracks (not a
    checkout) writes the marker and appends only `.skillhub-checkout.json` as a pattern, so the project's own
    source stays tracked.
11. **After a successful commit.** Proposed: the checkout stays, and its marker moves to the new base, so the next
    edit and commit continue from there. There is no "discard checkout" command; deleting the folder ends it, and
    `status` stops listing it.
12. **Folder without a marker whose id already exists on the hub.** Proposed: an update. The base is the hub's
    current version, the preview says "this folder was not checked out; it replaces the hub copy" and shows the full
    diff. It is never a conflict error (D7).
13. **Merge choice names for `commit`.** Proposed: reuse the update flags with role names that fit:
    `--accept <path>=hub|project`, `--manual <path>=<file>`, `--write-conflicts <dir>`.
14. **Where routing suggestions come from.** Proposed: a deterministic suggester (no model call) over the
    frontmatter `description` and, when present, the `## When to use`, `## When not to use` and `## Examples`
    sections that `skill create` writes into `SKILL.md`. `min_scope` defaults to `single_step`, and the operation
    is left empty unless the description names one. On a TTY, `--edit-routing` opens `$EDITOR` on a small YAML of
    the suggestions. Without a TTY, the existing `--trigger`, `--not-for`, `--example`, `--counter-example`,
    `--min-scope` and `--operation` flags override suggestions.
15. **`skill create` without a TTY.** Proposed: the existing flags (`--name`, `--description`, `--trigger`,
    `--not-for`, `--example`) fill the same scaffold. `--collection` and `--runtime-file` move to `skill commit`,
    whose collection default becomes `default` for both add and commit, ending the `core`/`default` split.
16. **Commit of a third-party skill's checkout without approval.** Proposed: without a TTY, refused (exit 2) with a
    FIX telling a person to run it in a terminal, or to pass `--approve-content <digest>` (already denied for
    agents). It is not queued. On a TTY, answering "n" to the approval writes nothing, because applying unapproved
    content would stop agents from getting the skill (D9 "never blocked").
17. **The MCP `skill_create_*` curation tools and the web Create screen.** Proposed: unchanged (they create a draft in
    the hub through the managed path, which D8 does not forbid). Only the CLI `skill create` moves to the project.
18. **Web pending changes.** Proposed: kept in the browser (`localStorage`, as drafts are today) and sent as one
    changeset on Review. They are lost if the browser storage is cleared. The routing form moves to its own
    "Routing" tab, and each save there is one reviewed commit.
19. **Web upload and change size.** Proposed: keep the 4 MiB request cap and the 1 MiB editable-text cap. An uploaded
    file is at most 2 MiB, sent as base64.
20. **Submission endpoint and payload.** Proposed: `POST /mcp/submissions` on 7420, next to `/mcp/bundle`. The body is
    the bundle tar format with a first entry `submission.json` (`skill_id`, `base_version`, `client_label`,
    `excluded`). Limits are the bundle limits (512 files, 16 MiB). The reply is `202 {submission_id, status:
    "waiting"}`.
21. **Submission queue storage and caps.** Proposed: `runtime/submissions/SUB-<ulid>/` (untracked, machine-local).
    At most 50 waiting submissions and 256 MiB in total. A new submission for the same skill id from the same client
    label replaces the older waiting one.
22. **Reviewing a submission in the CLI.** Proposed: `skillhub skill commit <SUB-id>` previews it like a folder (D7)
    and asks on a TTY: apply → approve (always, it is untrusted) → activate (new skill only). The same command with
    `--discard` drops it. Without a TTY, it refuses to apply. `skillhub skill review <id>` and `status` name waiting
    submissions.
23. **Provenance of a submitted skill.** Proposed: origin stays `local` once a person approves it. "Submitted from
    <client label>" is recorded in the commit message and the receipt, not in the canonical schema.
24. **Checkout from a remote-connected project.** Proposed: allowed for active, servable skills through the existing
    `GET /mcp/bundle` (no new endpoint). The marker records the hub URL and the bundle digest as `base_version`.
    Drafts cannot be checked out remotely.
