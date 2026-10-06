---
title: "Phase 4: Update merge and apply"
status: done
---

# Phase 4: Update merge and apply

<!-- Updated: Validation Session 1 - merge engine is git merge-file / git diff --no-index via offlineGitCommand; no MCP entry -->

## Context

- Plan: [plan.md](./plan.md) (D7, D8, D9, D10). Research: [merge and fetch](./reports/researcher-261004-2234-three-way-merge-and-fetch.md) sections 3–4 (UX and fetching). Its section 1 library recommendation is superseded: the user chose the system `git` as the merge engine (Validation Session 1, decision 8); its `git merge-file` exit-code findings still apply.
- Read first: `internal/app/upstream.go` (phase 3), `internal/app/upstream_origin.go` (phase 1), `internal/app/skill_add.go:688-760` (`planSkillAddProposal`, `storeSkillAddProposal` at line 1002, `ConfirmSkillAddProposal` at line 863), `internal/app/skill_lifecycle.go:440-500` (`DispatchConfirmProposal`, `RegisterProposalConfirmer`), `internal/skill/proposal_store.go:116-165`, `internal/mutation/mutation.go:31-38` and `:145-160` (BeforeDigest pinning), `internal/app/proposal_confirm.go`, `internal/app/skill_discovery.go:209` (`ensureImportedSkillFrontmatter`), `internal/app/skill_detail.go` (`ContentTrustFor`), `internal/app/operations.go:156-172` (`offlineGitCommand`: `-C <root>`, fsmonitor/untracked-cache/preload off, `GIT_TERMINAL_PROMPT=0`, `GIT_OPTIONAL_LOCKS=0`, `GIT_PAGER=cat`), `internal/app/skill_review_changes.go:205-215` (`gitOutput`, `gitLines`).

## Overview

Build the update: reconstruct base, upstream, and local versions of every file, classify and merge them, let the caller resolve what cannot be merged, and persist a pinned `upstream_update` proposal only when nothing is unresolved. Confirm applies files and the new origin pin atomically and leaves the content approval untouched, so a third-party skill returns to `review_required`.

## Requirements

1. **Merge engine = the system `git`** (already a hard dependency: `offlineGitCommand` in `internal/app/operations.go:156` and the file:// Git tests). No Go merge or diff library is added; `go.mod` is unchanged. A thin file `internal/app/upstream_merge.go` (package `app`, so it reuses the unexported `offlineGitCommand` and its offline env/config conventions) provides:
   - `mergeableText(content []byte) bool` — valid UTF-8, no NUL byte, at most 256 KiB. Files that fail it are never handed to git; they get a whole-file choice (`upstream` or `local`).
   - `gitMergeFile(ctx, root string, base, local, upstream []byte) (merged []byte, conflicts int, err error)`: writes the three inputs to a fresh directory `runtime/tmp/upstream-merge-<random>/` under the workspace (directory `0700`, files `0600`, created through `os.OpenRoot(root)`, refusing symlinked parents, removed with `defer` even on error), then runs `offlineGitCommand(ctx, root, "merge-file", "-p", "--diff3", "-L", "local", "-L", "base", "-L", "upstream", <local>, <base>, <upstream>)` under a 30 s `context.WithTimeout`. Exit-code semantics of `git merge-file`: `0` = clean merge (stdout is the result); `1`–`127` = number of conflicts (capped at 127; stdout holds the result with standard 7-character markers including the `||||||| base` section); any other status (git's `-1` surfaces as `255`), a signal, a timeout, or `exec.ErrNotFound` = error `upstream_merge_failed` naming the file (never treated as a conflict count). The conflict count comes only from the exit code, never from scanning the output, so it always matches git.
   - `gitDiffNoIndex(ctx, root, path string, from, to []byte) (string, error)`: same temp-file handling, runs `offlineGitCommand(ctx, root, "diff", "--no-index", "--no-color", "--no-ext-diff", "-U3", <from>, <to>)`; exit `0` = no difference (empty string), `1` = difference (stdout), anything else = error. Header lines (`diff --git`, `---`, `+++`) are rewritten from the temp paths to `a/<path>` / `b/<path>`; an absent side is written as an empty temp file and shown as `/dev/null`. Only called for `mergeableText` content; binary files get the summary `binary file changed`.
   - CRLF: inputs are passed to git byte-for-byte (no normalization, no `core.autocrlf` effect because `merge-file` and `diff --no-index` operate on the given files); `SKILL.md` is already LF-normalized by `ensureImportedSkillFrontmatter`. Tests pin git's behavior on CRLF companions.
   - Marker detection for **user-supplied** (`manual`) content and for upstream content uses one rule shared with canonical validation, so preview and `PlanMutation` never disagree: export `canonical.HasConflictMarker(contents string) bool` (rename of `hasConflictMarker`, `internal/canonical/canonical.go:334-341`; it trims each line and prefix-matches `<<<<<<<`, `=======`, `>>>>>>>`, a superset of what `git merge-file` emits).
2. **File set.** For skill `S` with origin `{repository, ref, commit B, path}` and state `checked_commit U`:
   - Upstream files at a commit = the reconstruction from phase 3: `RevisionAt(Locator{record repository, ref, origin.path}, commit)` → `List` → `DiscoverSkillsFromResources` → root item (`SkillDir == ""`) → `importedSkillFiles(item, S)`. This applies the same description fallback and nested-skill exclusion as `skill add`. Always use the source record's repository URL.
   - Rejected upstream paths: any path containing a control or non-printable character (`unicode.IsPrint` false, including newline and ESC) fails the preview with `invalid_request` `upstream_path_rejected` naming the escaped path; any case variant of `skill.meta.yaml` is ignored with warning `upstream_meta_ignored`; two upstream paths that collide under case folding fail with `upstream_path_rejected`.
   - An upstream file whose content trips `canonical.HasConflictMarker` (for example a Markdown setext `=======` heading) cannot be written into the workspace (canonical validation rejects it): its status is `blocked`, the only allowed action is `local`, and the summary explains why.
   - Local files = working-tree files of the skill folder except `skill.meta.yaml`.
   - Base is **available** when `RevisionAt(B)` succeeds and, if `origin.files_digest` is set, the reconstructed base files digest equals it; otherwise `BaseAvailable=false` and warning `upstream_base_unavailable` (or `upstream_base_mismatch`).
3. **Classification and defaults** (B = base, L = local, U = upstream; byte equality):

   | Case | `status` | default `action` |
   |---|---|---|
   | L = U (any B) | `unchanged` (omitted from `files`, counted) | — |
   | L = B, U ≠ B, U present | `upstream_only` | `upstream` |
   | U = B, L ≠ B, L present | `local_only` | `local` |
   | B absent, U only | `added_upstream` | `upstream` |
   | B absent, L only | `added_local` | `local` |
   | B present, L = B, U absent | `removed_upstream` | `upstream` (delete) |
   | B present, L ≠ B, U absent | `removed_upstream` + conflict | unresolved |
   | B present, U = B, L absent | `removed_local` | `local` (stay deleted) |
   | B present, U ≠ B, L absent | `removed_local` + conflict | unresolved |
   | all three differ, all `mergeableText` | `both_changed` (merged by `gitMergeFile`) | `merged` if conflicts `== 0`, else unresolved |
   | all three differ, not mergeable, or B absent with L ≠ U | `both_changed` | unresolved |
   | `BaseAvailable=false` and L ≠ U | `both_changed` | unresolved |

   Allowed actions per file: `upstream` (take U; delete when U absent), `local` (keep L; keep absent when L absent), `merged` (only when `Conflicts == 0`), `manual` (caller supplies `content`; must be valid UTF-8 and must not contain conflict markers). Invalid choices return `invalid_request` naming the path.
4. **Preview** `UpstreamService.PreviewUpdate(ctx, path, UpstreamUpdateInput{SkillID, TargetCommit, Resolutions []UpstreamResolution{Path, Action, Content}, IdempotencyKey})`:
   - Requires a tracked skill (`provenance.source_id` and `github`/`git` origin) whose state is `update_available` or `diverged`; otherwise `invalid_request` with a fix (`skillhub source check <source>` for `unknown`/`unavailable`; `Upstream is unchanged.` for `up_to_date`/`modified`; `The skill folder no longer exists upstream; keep your local copy or archive the skill.` for `upstream_removed`). `TargetCommit`, when given, must equal `state.CheckedCommit` (else `stale_proposal`: "Upstream moved since you reviewed it").
   - Returns `UpstreamUpdatePreview{Result; SkillID; SourceID; BaseCommit; TargetCommit; TargetCommittedAt; BaseAvailable; Files []UpstreamFile; UnchangedCount; Unresolved []string; Diff skill.DiffSummary; TrustImpact; Confirmation; Warnings}`. `UpstreamFile{Path, Status, DefaultAction, Action, Conflicts, Mergeable, UpstreamDiff (B→U), LocalDiff (B→L), ResultDiff (L→result), MergedWithMarkers}` (diff fields `omitempty`; `MergedWithMarkers` only when `Conflicts > 0`).
   - When `Unresolved` is empty and at least one file changes: plan the write set and persist the artifact; status `action_required`, pins filled. When `Unresolved` is non-empty: status `action_required`, summary `N file(s) need a decision before this update can be applied.`, `Confirmation.Confirmation.Required = true` with empty pins. When nothing changes (every file `local`/`unchanged`): still plan the meta-only write (re-pins the base to U) so the skill stops reporting the update.
   - `TrustImpact{ThirdParty bool; CurrentlyApproved bool; ReviewRequiredAfterApply bool; ReviewCommand string}`: `ReviewRequiredAfterApply = ThirdParty && any skill file changes`; `ReviewCommand = "skillhub skill review <id>"`.
   - There is no MCP entry point and no metadata-only variant: agents cannot preview or apply upstream updates (Validation Session 1, decision 5). `TargetCommittedAt` is the committer date of `U` (phase 3 `CommitTime`).
5. **Write set** (`Command: "skill_upstream_update"`): one change per file whose final content differs from local, with `BeforeDigest` = digest of the local bytes the merge used (`""` when the file was absent); deletions use `Delete: true`. `PlanMutation` replaces an empty `BeforeDigest` with the digest it finds on disk (`internal/mutation/mutation.go:150-158`), so after planning compare each planned change's `BeforeDigest` with the expected value and return `stale_proposal` on any difference (a file created or edited between the merge read and planning is never overwritten). Plus `skill.meta.yaml`, edited through `yaml.Node` so untouched keys keep their order: `provenance.origin.commit = U`, `folder_digest = RevisionAt(U, origin.path).ContentDigest`, `files_digest` = digest of the reconstructed **upstream** files (the new base, not the merged result), `content_digest` = digest of the transformed upstream `SKILL.md`, `transformations` = transforms applied to it, top-level `updated_at = now`. `quality` is never modified. Request digest = sha256 over skill ID, B, U, and the sorted resolutions; idempotency key `skill_upstream_update:<id>:<first 32 hex of digest>` unless supplied.
6. **Persistence and confirm**: new `skill.ProposalKindUpstreamUpdate = "upstream_update"`; `LoadProposal` accepts it. **Kind guard (defense in depth; no MCP tool previews or applies upstream updates, so this guard is what keeps an agent from applying one through a generic confirm tool with pins obtained elsewhere):** `SkillService.LoadSkillProposal` (`internal/app/skill_lifecycle.go:186-199`) returns `invalid_request` for any kind other than `lifecycle` ("Proposal <id> is a <kind> proposal; confirm it with `skillhub skill confirm <id>`."). This closes the generic confirm paths that would otherwise apply it without `ConfirmUpdate`: MCP `skill_create_confirm` (`internal/delivery/mcpserver/skill_tools.go:61-65`), `skill_transition_confirm` (`:115-119`), `skill_update_confirm` (`internal/delivery/mcpserver/insight_tools.go:92-102`), and web `handleSkillConfirm` (`internal/delivery/web/routes_skill_write.go:244-248`); it also closes the same existing gap for `add` proposals, whose own confirm paths do not use `LoadSkillProposal`. artifact stored with `storeSkillAddProposal` (rename to `storeProposalArtifact` and update its one caller). `RegisterProposalConfirmer(skill.ProposalKindUpstreamUpdate, ...)` in an `init()` in `upstream_update.go` so `skillhub skill confirm <proposal>` works. `UpstreamService.ConfirmUpdate(ctx, path, proposalID, pins)` verifies pins with `verifyProposalPins` (expired → `stale_proposal`; mismatch → `stale_proposal`), applies through `confirmAndPublish`, maps `mutation.ErrConflict` to `stale_proposal` ("The skill changed after the preview; nothing was applied."), then best-effort records `skill_upstream_state` with `BaseCommit = U`, `Upstream = same`. The confirmer receives only the stored write set, so it re-derives `U` (from `provenance.origin.commit` in the `skill.meta.yaml` change), the source ID, and `TrustImpact` (third-party from the meta's provenance; review required when any non-meta file changes) from that write set. Returns `UpstreamUpdateResult{Result; SkillID; OperationID; ChangedPaths; CatalogSnapshot; Generation; GitDirty; TrustImpact}` with summary `Skill <id> updated to <U12>.` plus, when `ReviewRequiredAfterApply`, ` Agents cannot use it until you approve the new content: skillhub skill review <id>`.

## Related code files

Create: `internal/app/upstream_merge.go`, `internal/app/upstream_merge_test.go`, `internal/app/upstream_update.go`, `internal/app/upstream_update_test.go`.

Modify: `internal/skill/lifecycle.go` (constant at lines 57-60), `internal/skill/proposal_store.go` (line 151), `internal/app/skill_add.go` (rename `storeSkillAddProposal` → `storeProposalArtifact`, `storedProposalArtifact` unchanged), `internal/app/skill_lifecycle.go` (kind guard in `LoadSkillProposal`), `internal/canonical/canonical.go` (export `HasConflictMarker`; update its callers in that file), `internal/app/upstream.go` (shared helpers only, if needed), `internal/delivery/mcpserver/skill_tools_test.go` (one guard test).

Do not modify any other file.

## Implementation steps

### Task 4.1 — Git-backed merge and diff
- Steps:
  1. Implement Requirement 1 in `internal/app/upstream_merge.go` and export `canonical.HasConflictMarker` (update its callers in `canonical.go`).
  2. Tests (`TestUpstreamGitMerge`, table-driven, real `git` — no skip, git is a hard dependency): clean non-overlapping edits (conflicts 0, output equals the expected bytes); same-line conflict (conflicts 1; output contains `<<<<<<< local`, `||||||| base`, `=======`, `>>>>>>> upstream`); two separate conflicts (conflicts 2); adjacent-line edits (expected count = the exit code of `git merge-file -p --diff3` run directly with `exec.Command` on the same three files inside the test, so the wrapper is compared with git itself); identical edits on both sides (0); missing trailing newline on one side; CRLF companion file with non-overlapping edits (clean, CRLF preserved); a NUL-containing input is rejected by `mergeableText` and `gitMergeFile` is not called; a canceled context returns `upstream_merge_failed`; after success and after error `runtime/tmp/` contains no `upstream-merge-*` entry; the unexported temp-writer helper, called directly, creates a `0700` directory and `0600` files. `gitDiffNoIndex`: identical inputs → `""`; one-line change → output starts with `diff --git a/SKILL.md b/SKILL.md` and contains no temp path; added file shows `--- /dev/null`.
- Verify: `go test ./internal/app/ -run TestUpstreamGitMerge -count=1` exits 0 and prints `ok`.

### Task 4.2 — Proposal kind
- Steps: add the constant; accept it in `LoadProposal`; rename the store helper. Extend the existing proposal-store test table with an `upstream_update` artifact round-trip.
- Verify: `go test ./internal/skill/ -count=1` exits 0 and prints `ok`.

### Task 4.3 — Preview, write set, confirm
- Steps: implement Requirements 2–6 in `internal/app/upstream_update.go`.
- Tests (`TestUpstreamUpdate` with subtests; real file:// repository; skill added and checked through phases 1–3; the skill's content approved first via `SkillService` update with `ContentReviewedDigest` set to `ContentTrustFor(...).ContentDigest`):
  1. `clean`: upstream edits `SKILL.md`; preview has pins and no unresolved; confirm; local `SKILL.md` equals transformed upstream; meta `origin.commit == U`; `quality.content_reviewed_digest` unchanged; `ContentTrustFor` reports `content_review_stale`; `GetSkillUpstream` reports `up_to_date`; result summary contains `skillhub skill review`.
  2. `merged`: local and upstream edit different lines; default `merged`; result contains both edits.
  3. `conflict`: both edit the same line; `Unresolved == ["SKILL.md"]`, pins empty; resolution `manual` with marker text → `invalid_request`; `manual` with clean text → pins; confirm; status afterwards `modified`.
  4. `removed-upstream-with-local-edit`: unresolved; resolution `local` keeps the file.
  5. `stale`: local edit after preview → confirm returns error code `stale_proposal`, file untouched.
  6. `base-mismatch`: overwrite `origin.files_digest` with a different valid digest; preview has `BaseAvailable == false` and every differing file unresolved.
  7. `confirm-via-dispatch`: `SkillService.DispatchConfirmProposal` with the proposal ID returns an `UpstreamUpdateResult` whose `TrustImpact.ReviewRequiredAfterApply` is true and whose state row has `BaseCommit == U`.
  8. `kind-guard`: `SkillService.LoadSkillProposal` with an `upstream_update` proposal ID returns `invalid_request`; in `internal/delivery/mcpserver/skill_tools_test.go`, `skill_transition_confirm` with that ID returns an error result and the files are unchanged.
  9. `base-fetched-by-sha`: `os.RemoveAll(<root>/runtime/sources/git)` before the preview; the fixture repository has `uploadpack.allowReachableSHA1InWant true`; preview still has `BaseAvailable == true`.
  10. `no-description-and-nested`: upstream `SKILL.md` without a `description` and a nested `sub/SKILL.md` skill; after add and an upstream edit to the parent's `SKILL.md`, preview has `BaseAvailable == true`, no file under `sub/` appears in `files`, and the clean update applies.
  11. `rejected-paths`: an upstream file named `bad\nname.md` fails with `upstream_path_rejected`; `SKILL.META.YAML` is ignored with `upstream_meta_ignored`; an upstream file containing a line `=======` is `blocked`.
  12. `planned-before-digest`: create a local file between preview computation and planning (inject through a test hook or by calling the planning helper directly) → `stale_proposal`.
- Verify: `go test ./internal/app/ -run TestUpstreamUpdate -count=1` exits 0 and prints `ok`.

### Task 4.4 — Gate
- Verify: `make check` exits 0.

## Todo

- [x] Task 4.1 git-backed merge and diff
- [x] Task 4.2 proposal kind
- [x] Task 4.3 preview / write set / confirm
- [x] Task 4.4 `make check`

## Success criteria

- No path is written without either a default action from the table or an explicit resolution.
- Confirm is atomic and refuses when any touched file changed after preview.
- The content approval is never written by this flow.

## UX acceptance

Service-level only; phases 6–9 render it. JSON shape of a conflicting preview (abridged):

```json
{"status":"action_required","summary":"1 file(s) need a decision before this update can be applied.",
 "skill_id":"pdf","source_id":"anthropics-skills","base_commit":"3f9c2a1…","target_commit":"81d04be…",
 "base_available":true,"unchanged_count":5,"unresolved":["SKILL.md"],
 "files":[{"path":"SKILL.md","status":"both_changed","default_action":"","action":"","conflicts":1,"mergeable":true,"upstream_diff":"@@ …","local_diff":"@@ …","merged_with_markers":"…"},
          {"path":"scripts/extract.py","status":"upstream_only","default_action":"upstream","action":"upstream","conflicts":0,"mergeable":true,"result_diff":"@@ …"}],
 "trust_impact":{"third_party":true,"currently_approved":true,"review_required_after_apply":true,"review_command":"skillhub skill review pdf"},
 "confirmation":{"confirmation":{"required":true,"pins":{"proposal_id":"","proposal_digest":"","base_version":""}}}}
```

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Upstream content that canonical validation rejects (setext `=======` lines) | M×L | `blocked` status with `local` only, explained in the summary; the skill keeps reporting the update until upstream changes. |
| `git` missing or too old on a user machine | L×M | Same requirement as existing Git features; error `upstream_merge_failed` says git is required; `--diff3` is supported by every git still in support (`--zdiff3` is not used). |
| Temp files leak upstream content | L×M | Under the gitignored workspace `runtime/tmp/`, `0600`/`0700`, removed by `defer`; leftovers from a crash are removed at the start of the next merge (delete `runtime/tmp/upstream-merge-*` older than 1 h). |
| Transform drift makes the base differ from what was imported | M×M | `files_digest` comparison → `BaseAvailable=false` → explicit choices (never silent overwrite). |
| Upstream adds a huge or binary file | L×M | `mergeableText` false → whole-file choice, never sent to git; mutation limits (16 MiB per change) still apply. |
| Proposal kind unknown to an older binary's `skill confirm` | L×L | Same binary previews and confirms; older binaries report "unknown kind". |

## Security considerations

Upstream bytes are untrusted: they are written only after explicit human confirmation, and the third-party content gate keeps them away from agents until `--approve-content`. Canonical validation in `PlanMutation` (`validateVirtual`) also rejects conflict markers. Manual content is size-bounded by the mutation limits.

## Rollback

Revert the phase commit. Applied updates are ordinary canonical commits the user can revert with Git.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
