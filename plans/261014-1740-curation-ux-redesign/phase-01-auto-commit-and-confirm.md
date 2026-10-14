# Phase 1: Auto-commit and terminal confirm

## Goal

Every confirmed change to the hub becomes one scoped Git commit, from the CLI, MCP and the web alike. Every
hub-changing CLI command asks `[y/N]` under its preview when a person is at a terminal, and keeps the two-step
`--yes` shape otherwise. `--diff` replaces the hidden `--full-diff`.

## Decisions implemented

- **D3**: auto-commit, `--no-commit`, a scoped pathspec, hooks and signing respected, refusal when the same files
  hold manual edits, and files kept written with a manual-commit hint when the commit cannot finish. CLI, web and
  agents behave the same.
- **D5**: the y/N prompt on a TTY, two steps without one, a `--diff` hint line in every preview, and `--full-diff`
  dropped. This applies to every hub-changing command.
- **D6/D9 (shared part)**: every commit reports who gets the new version and when.

## Context

- The single write path is `mutation.CommitWithOptions` (`internal/mutation/mutation.go:207`). It takes the exclusive
  lock, runs `commitWhileLocked`, and publishes the catalog under the lock (`internal/skill/lifecycle.go:494-520`).
  Callers: `skill.Manager.Confirm` (`lifecycle.go:504`), `app/source.go:975`, `app/workspace.go:292`,
  `app/migration.go:90`. Every confirm surface reaches one of them: CLI `skill.go:300` and `:147`,
  `skill_create_prompt.go:225`, MCP `tool_confirmation.go:40`, web `routes_skill_write.go:263`.
- `Receipt{OperationID, ChangedPaths, …, GitDirty}` (`mutation.go:72`). The receipt path is
  `history/operations/YYYY/MM/<id>.yaml` (`receipt.go:45`).
- The only commit in the code is `commitFirst` (`app/workspace_init.go:445-487`), which turns hooks and signing off.
  That stays as it is for `init`. The new helper does the opposite: it respects both.
- `offlineGitCommand` (`app/operations.go:165`) already sets `GIT_TERMINAL_PROMPT=0` and turns off askpass and the
  pager. `mutation.gitDirty` (`filesystem.go:507`) reports dirty status for the whole tree.
- There is no shared CLI confirm helper. `writeSkillPreview` (`cli/skill.go:702-732`) prints
  "No files changed. Confirm with:". Source flows have `writeSourceProposal` (`source.go:602`) and
  `writeSourceImportProposal` (`:675`).
- Hub-changing commands with preview-unless-`--yes` today: `skill add`, `skill create` (flag form), `skill edit`,
  `skill activate|deprecate|archive`, `skill update`, `source watch|import|attach|detach|unwatch|backfill`,
  `eval promote`, `maintain migrate`. `skill confirm --proposal …` always applies. `skill env`, `maintain rebuild`,
  `source capture` and `triage` write without a preview. `skill env` writes untracked `runtime/config`, so it is
  out of scope for D3.
- `next_commands.go:55-80` appends `--yes` to `skill confirm`, which ignores it. The P0 plan owns that wrong hint.

## Design

### Scoped commit (internal/workspace)

New file `internal/workspace/git_commit.go`:

```go
// CommitPolicy says whether a confirmed change becomes a Git commit.
type CommitPolicy struct {
    Disabled    bool   // --no-commit
    Message     string // empty: the caller's default message
    Interactive bool   // a person is at a terminal: signing may prompt
    Defer       bool   // a chain collects paths and commits once at the end
}

// CommitOutcome is reported on every mutation result.
type CommitOutcome struct {
    Status        string   // committed | skipped | failed | deferred
    Hash          string
    Message       string
    Reason        string   // why skipped or failed, in plain words
    ManualCommand string   // exact git command to finish by hand
    Reach         Reach
}
type Reach struct{ ThisHubNow bool; PushNeeded bool; Remote string }

func DirtyPaths(ctx context.Context, root string, paths []string) ([]string, error)
func CommitPaths(ctx context.Context, root string, paths []string, message string, interactive bool) CommitOutcome
```

- `DirtyPaths` runs `git status --porcelain=v1 -z --untracked-files=all -- <paths>` and returns every path with a
  staged, unstaged or untracked change. This includes a path that does not exist yet but has an untracked file.
- `CommitPaths` runs `git add -A -- <paths>`, then `git commit -m <message> -- <paths>`. The pathspec form commits only
  those paths and leaves everything else in the index and working tree as it was. It does not pass `--no-verify`,
  does not override `commit.gpgsign`, and does not set an identity. When `interactive` is false, it has no stdin and
  a timeout (proposed 60 s, open question 2). Non-Git hubs or a missing `git` binary give `skipped` with a reason.
  A non-zero exit gives `failed`, with the first lines of stderr as the reason and
  `git -C <hub> add -- <paths> && git -C <hub> commit -- <paths>` as the manual command.
- `Reach.PushNeeded` is true when the hub has a Git remote (reuse `app/git_remote.go` detection, moved or wrapped if
  `workspace` cannot import `app`).

### Wiring into the mutation path

- `mutation.Options` gains `Commit workspace.CommitPolicy` and a default message. `Receipt` gains
  `Commit workspace.CommitOutcome`.
- In `commitWhileLocked`, before anything is staged: unless `Disabled`, call `DirtyPaths` on the write set's paths
  and deletes. A non-empty result returns the "uncommitted edits" error (proposed `edit_conflict`, open question 3)
  naming the files, with FIX "commit or discard these edits first, or re-run with `--no-commit`". The receipt path is
  new and never dirty.
- In `CommitWithOptions`, after `lock.Unlock()`: when the commit is enabled, not deferred, and the result is not a
  replayed operation, call `CommitPaths` with the changed paths plus the receipt path. This must run after the
  unlock, because the hub's pre-commit hook runs `skillhub maintain validate --staged`.
- The policy reaches the mutation layer through a context value (`workspace.WithCommitPolicy(ctx, policy)`), read in
  `CommitWithOptions`. This avoids threading a flag through about 15 app input types. Each confirm surface sets it:
  the CLI from `--no-commit` and TTY state, and MCP and the web with the defaults (commit on, not interactive).
- Chains (phase 2 and 3: add → approve → activate) set `Defer`, collect `Receipt.ChangedPaths` and receipt paths,
  and call `CommitPaths` once. A helper in `internal/app` (`commitChain`) owns that.
- Default messages: open question 1 (proposed `<Verb> <skill-id>[: <detail>]` plus a `Skillhub-Operation: OP-…`
  trailer). Each service passes its verb.
- `SkillMutationResult` and the source and migration results expose `commit` (additive JSON). The text
  "Git commit remains user-controlled." (`app/skill_lifecycle.go:244`) is replaced by the commit line.

### Terminal confirm (internal/delivery/cli)

New file `internal/delivery/cli/confirm_prompt.go`:

```go
// confirmPreview prints the preview; on a TTY it asks and, on "y", applies exactly the previewed proposal.
func confirmPreview(env cliEnv, preview previewView, apply func(pins) error) (applied bool, err error)
func interactiveTerminal(stdin io.Reader, stdout io.Writer) bool // both are terminals
```

- The prompt runs only when stdin and stdout are terminals, `--yes` is not set and `--json` is not set.
- On "y", it calls the same confirm function as `skill confirm --proposal`, with the pins from the preview it just
  printed. The person therefore applies exactly what they saw.
- Anything other than `y` or `yes` prints `Nothing written.` and exits 0.
- Without a TTY, output stays as today, but the last lines become:
  `Show the diff: <same command> --diff` and `Apply: <same command> --yes`.
- `writeSkillPreview` changes "No files changed. Confirm with:" to "Nothing written yet." The `--diff` hint line is
  added. `writeSourceProposal` and `writeSourceImportProposal` get the same helper.
- `--diff` replaces `--full-diff` in `parseSkillFlags` (`skill.go:398`) and in the add, create, edit, transition,
  update and source paths. `--editor` keeps forcing the full diff. `--full-diff` becomes an unknown flag.
- `--no-commit` is accepted by every hub-changing command and by `skill confirm`.
- After apply, the commit line prints, for example
  `Committed 3f2a91c "Activate review-checklist". Agents using this hub get it now; other clones after `git -C ~/skill-hub push`.`

### Claude Code rules

The agent path does not change shape: no TTY, preview, then `--yes`. `cli-permissions.go:15-26` stays as it is. A new
test asserts that the installed rules ask for `skillhub skill commit x --yes` and `skillhub skill update x --yes`,
and deny `--approve-content` on `skill commit`, `skill add` and `skill edit`. The rule patterns are already generic,
so this test guards against regressions.

## Files

- Create `internal/workspace/git_commit.go` and `git_commit_test.go`.
- Modify `internal/mutation/mutation.go` (Options, Receipt, refusal under the lock, commit after unlock),
  `internal/mutation/transaction.go` and `receipt.go` (expose the receipt path), and `mutation_test.go`.
- Modify `internal/skill/lifecycle.go` (pass the policy and verb), `internal/app/skill_lifecycle.go` (result field,
  text), `internal/app/source.go`, `workspace.go`, `migration.go` (verbs), `internal/app/result.go` if the result
  printer needs it. Create `internal/app/commit_chain.go`.
- Create `internal/delivery/cli/confirm_prompt.go` and `confirm_prompt_test.go`. Modify `skill.go`, `skill_add.go`,
  `skill_upstream.go`, `source.go`, `source_watch.go`, `source_import.go`, `evaluation.go`, `migration.go`,
  `next_commands.go`, `help.go`.
- Modify `internal/delivery/mcpserver/tool_confirmation.go` and `internal/delivery/web/routes_skill_write.go`
  (context policy; commit outcome passes through).
- Modify `internal/app/skill_review.go:613-619` and `internal/delivery/cli/skill.go:755-767` (`nextStepForSkill`):
  remove "commit it yourself" hints after a successful commit. Keep them only for `--no-commit` and failed commits.
- Create `internal/hostintegration/cli_permissions_commit_test.go`.
- Update `docs/contracts/result-envelope.md`, `schemas/result-envelope.schema.json` (the `commit` object) and
  `docs/contracts/error-codes.md` (the refusal message, or a new code if open question 3 says so).

## Steps (tests first)

1. Write `git_commit_test.go` against a temp repo with isolated `GIT_CONFIG_GLOBAL` and an identity:
   - Only the given paths are committed. An unrelated staged file stays staged and uncommitted. An unrelated modified
     file stays modified.
   - Deletes are committed.
   - A pre-commit hook that exits 1 gives `failed`, the files are still on disk, and `ManualCommand` is exact.
   - `commit.gpgsign=true` with `gpg.program` set to a script that sleeps past the timeout gives `failed` with a
     timeout reason when not interactive.
   - No identity gives `failed` with a reason that names `user.email`.
   - A non-Git root gives `skipped`.
   - `DirtyPaths` reports modified, staged and untracked files under the given paths only.
2. Write mutation tests: a change to a skill with an uncommitted manual edit is refused before any write and no
   receipt is written. The same change with `Disabled` applies. A replayed operation does not commit again. A
   deferred policy returns `deferred` and no commit.
3. Implement the helper and the mutation wiring until steps 1-2 pass.
4. Write CLI tests (table over every hub-changing command) with fake TTY streams:
   - "y" applies, and the hub HEAD advances by exactly one commit.
   - "n" and an empty answer write nothing and exit 0.
   - No TTY prints the preview with the `--diff` and `--yes` hint lines and writes nothing.
   - `--json` never prompts.
   - `--diff` prints the unified diff. `--full-diff` fails as an unknown flag.
   - `--no-commit` writes without committing.
5. Implement `confirm_prompt.go` and switch each command to it.
6. Write MCP and web confirm tests: after `skill_transition_confirm` and the web proposal confirm, the hub has one new
   commit and the JSON carries `commit.status = "committed"`.
7. Add the permissions test. Update help text and contract docs. Update the goldens that print the old commit hints,
   reviewing each diff.

## Mock outputs

Terminal, person present:

```text
$ skillhub skill activate review-checklist
Activate review-checklist: draft → active
  M skills/default/review-checklist/.meta/skill.yaml   +1 -1
Routing: ready (3 triggers, 2 not-for, 3 examples).
Show the diff: skillhub skill activate review-checklist --diff
Apply this change? [y/N] y
review-checklist is now active.
Committed 3f2a91c "Activate review-checklist".
Agents using this hub get it now; other clones after `git -C ~/skill-hub push`.
```

Agent, no TTY:

```text
$ skillhub skill activate review-checklist
Activate review-checklist: draft → active
  M skills/default/review-checklist/.meta/skill.yaml   +1 -1
Nothing written yet.
Show the diff: skillhub skill activate review-checklist --diff
Apply: skillhub skill activate review-checklist --yes
```

Refusal:

```text
$ skillhub skill edit pdf --trigger "fill a pdf form" --yes
ERROR: skills/default/pdf/SKILL.md has edits that were not committed.
WHY: Skill Hub commits each change with only its own files, and these files already differ from Git.
FIX: Commit them (git -C ~/skill-hub add -- skills/default/pdf && git -C ~/skill-hub commit), discard them,
     or re-run with --no-commit.
```

Commit failure:

```text
Applied: pdf trigger added.
Not committed: the commit needs your signing key passphrase and there is no terminal.
Finish it with: git -C ~/skill-hub add -- skills/default/pdf history/operations/2026/10/OP-01J….yaml && git -C ~/skill-hub commit -- skills/default/pdf history/operations/2026/10/OP-01J….yaml
```

## Validation

```bash
go test ./internal/workspace -run 'Commit|Dirty' -count=1
go test ./internal/mutation -count=1
go test ./internal/delivery/cli -run 'Confirm|Prompt|Diff|NoCommit' -count=1
go test ./internal/delivery/mcpserver ./internal/delivery/web -run 'Confirm' -count=1
go test ./internal/hostintegration -count=1
make check
```

## Risks

- **Deadlock with the pre-commit hook.** The commit runs after unlock. A test installs a hook that runs
  `skillhub maintain validate --staged` against the temp hub.
- **Existing hubs with uncommitted skill edits are refused.** The message gives the commit command and
  `--no-commit`. The release note says so (phase 6).
- **Context-carried policy is easy to drop** on a path that builds a fresh context. The MCP and web tests assert the
  commit on the real handlers. A test also covers the source and migration callers.
- **Goldens and UX fixtures change** (`docs/contracts/ux-fixtures.md`, CLI goldens). Review every diff; do not
  regenerate blindly.

## Rollback

Revert the phase commits. Mutations go back to writing files only. Commits already made stay in the hub's history.

## Effort

2.5 days.
