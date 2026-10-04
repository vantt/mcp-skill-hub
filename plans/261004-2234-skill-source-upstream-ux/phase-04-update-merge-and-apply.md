---
title: "Phase 4: Update merge and apply"
status: todo
---

# Phase 4: Update merge and apply

## Context

- Plan: [plan.md](./plan.md) (D7, D8, D9, D10). Research: [merge and fetch](./reports/researcher-261004-2234-three-way-merge-and-fetch.md) sections 1, 3, Recommendations 1–8.
- Read first: `internal/app/upstream.go` (phase 3), `internal/app/upstream_origin.go` (phase 1), `internal/app/skill_add.go:688-760` (`planSkillAddProposal`, `storeSkillAddProposal` at line 1002, `ConfirmSkillAddProposal` at line 863), `internal/app/skill_lifecycle.go:440-500` (`DispatchConfirmProposal`, `RegisterProposalConfirmer`), `internal/skill/proposal_store.go:116-165`, `internal/mutation/mutation.go:31-38` and `:145-160` (BeforeDigest pinning), `internal/app/proposal_confirm.go`, `internal/app/skill_discovery.go:209` (`ensureImportedSkillFrontmatter`), `internal/app/skill_detail.go` (`ContentTrustFor`).

## Overview

Build the update: reconstruct base, upstream, and local versions of every file, classify and merge them, let the caller resolve what cannot be merged, and persist a pinned `upstream_update` proposal only when nothing is unresolved. Confirm applies files and the new origin pin atomically and leaves the content approval untouched, so a third-party skill returns to `review_required`.

## Requirements

1. **`internal/merge3`** (new package):
   - `Merge(base, local, upstream []byte) Result` with `Result{Content []byte; Conflicts int}`; line-based diff3 built on `github.com/epiclabs-io/diff3`'s structured merge API; conflict blocks serialized as `<<<<<<< local`, `||||||| base`, `=======`, `>>>>>>> upstream` (7 characters), final newline preserved exactly as the inputs agree.
   - `Unified(fromLabel, toLabel string, from, to []byte) string` using `github.com/aymanbagabas/go-udiff` (3 lines of context).
   - Conflict-marker detection reuses canonical validation's rule so preview and `PlanMutation` never disagree: export `canonical.HasConflictMarker(contents string) bool` (rename of `hasConflictMarker`, `internal/canonical/canonical.go:334-341`, which trims each line and prefix-matches `<<<<<<<`, `=======`, `>>>>>>>`) and call it from `merge3` and the preview.
   - `Mergeable(content []byte) bool` — valid UTF-8, no NUL byte, at most 256 KiB.
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
   | all three differ, all `Mergeable` | `both_changed` | `merged` if `Conflicts == 0`, else unresolved |
   | all three differ, not mergeable, or B absent with L ≠ U | `both_changed` | unresolved |
   | `BaseAvailable=false` and L ≠ U | `both_changed` | unresolved |

   Allowed actions per file: `upstream` (take U; delete when U absent), `local` (keep L; keep absent when L absent), `merged` (only when `Conflicts == 0`), `manual` (caller supplies `content`; must be valid UTF-8 and must not contain conflict markers). Invalid choices return `invalid_request` naming the path.
4. **Preview** `UpstreamService.PreviewUpdate(ctx, path, UpstreamUpdateInput{SkillID, TargetCommit, Resolutions []UpstreamResolution{Path, Action, Content}, IdempotencyKey})`:
   - Requires a tracked skill (`provenance.source_id` and `github`/`git` origin) whose state is `update_available` or `diverged`; otherwise `invalid_request` with a fix (`skillhub source check <source>` for `unknown`/`unavailable`; `Upstream is unchanged.` for `up_to_date`/`modified`; `The skill folder no longer exists upstream; keep your local copy or archive the skill.` for `upstream_removed`). `TargetCommit`, when given, must equal `state.CheckedCommit` (else `stale_proposal`: "Upstream moved since you reviewed it").
   - Returns `UpstreamUpdatePreview{Result; SkillID; SourceID; BaseCommit; TargetCommit; TargetCommittedAt; BaseAvailable; Files []UpstreamFile; UnchangedCount; Unresolved []string; Diff skill.DiffSummary; TrustImpact; Confirmation; Warnings}`. `UpstreamFile{Path, Status, DefaultAction, Action, Conflicts, Mergeable, UpstreamDiff (B→U), LocalDiff (B→L), ResultDiff (L→result), MergedWithMarkers}` (diff fields `omitempty`; `MergedWithMarkers` only when `Conflicts > 0`).
   - When `Unresolved` is empty and at least one file changes: plan the write set and persist the artifact; status `action_required`, pins filled. When `Unresolved` is non-empty: status `action_required`, summary `N file(s) need a decision before this update can be applied.`, `Confirmation.Confirmation.Required = true` with empty pins. When nothing changes (every file `local`/`unchanged`): still plan the meta-only write (re-pins the base to U) so the skill stops reporting the update.
   - `TrustImpact{ThirdParty bool; CurrentlyApproved bool; ReviewRequiredAfterApply bool; ReviewCommand string}`: `ReviewRequiredAfterApply = ThirdParty && any skill file changes`; `ReviewCommand = "skillhub skill review <id>"`.
   - `MetadataOnly()` returns a copy with every diff, `MergedWithMarkers`, and warning text that quotes content removed (used by MCP in phase 7).
5. **Write set** (`Command: "skill_upstream_update"`): one change per file whose final content differs from local, with `BeforeDigest` = digest of the local bytes the merge used (`""` when the file was absent); deletions use `Delete: true`. `PlanMutation` replaces an empty `BeforeDigest` with the digest it finds on disk (`internal/mutation/mutation.go:150-158`), so after planning compare each planned change's `BeforeDigest` with the expected value and return `stale_proposal` on any difference (a file created or edited between the merge read and planning is never overwritten). Plus `skill.meta.yaml`, edited through `yaml.Node` so untouched keys keep their order: `provenance.origin.commit = U`, `folder_digest = RevisionAt(U, origin.path).ContentDigest`, `files_digest` = digest of the reconstructed **upstream** files (the new base, not the merged result), `content_digest` = digest of the transformed upstream `SKILL.md`, `transformations` = transforms applied to it, top-level `updated_at = now`. `quality` is never modified. Request digest = sha256 over skill ID, B, U, and the sorted resolutions; idempotency key `skill_upstream_update:<id>:<first 32 hex of digest>` unless supplied.
6. **Persistence and confirm**: new `skill.ProposalKindUpstreamUpdate = "upstream_update"`; `LoadProposal` accepts it. **Kind guard:** `SkillService.LoadSkillProposal` (`internal/app/skill_lifecycle.go:186-199`) returns `invalid_request` for any kind other than `lifecycle` ("Proposal <id> is a <kind> proposal; confirm it with `skillhub skill confirm <id>`."). This closes the generic confirm paths that would otherwise apply it without `ConfirmUpdate`: MCP `skill_create_confirm` (`internal/delivery/mcpserver/skill_tools.go:61-65`), `skill_transition_confirm` (`:115-119`), `skill_update_confirm` (`internal/delivery/mcpserver/insight_tools.go:92-102`), and web `handleSkillConfirm` (`internal/delivery/web/routes_skill_write.go:244-248`); it also closes the same existing gap for `add` proposals, whose own confirm paths do not use `LoadSkillProposal`. artifact stored with `storeSkillAddProposal` (rename to `storeProposalArtifact` and update its one caller). `RegisterProposalConfirmer(skill.ProposalKindUpstreamUpdate, ...)` in an `init()` in `upstream_update.go` so `skillhub skill confirm <proposal>` works. `UpstreamService.ConfirmUpdate(ctx, path, proposalID, pins)` verifies pins with `verifyProposalPins` (expired → `stale_proposal`; mismatch → `stale_proposal`), applies through `confirmAndPublish`, maps `mutation.ErrConflict` to `stale_proposal` ("The skill changed after the preview; nothing was applied."), then best-effort records `skill_upstream_state` with `BaseCommit = U`, `Upstream = same`. The confirmer receives only the stored write set, so it re-derives `U` (from `provenance.origin.commit` in the `skill.meta.yaml` change), the source ID, and `TrustImpact` (third-party from the meta's provenance; review required when any non-meta file changes) from that write set. Returns `UpstreamUpdateResult{Result; SkillID; OperationID; ChangedPaths; CatalogSnapshot; Generation; GitDirty; TrustImpact}` with summary `Skill <id> updated to <U12>.` plus, when `ReviewRequiredAfterApply`, ` Agents cannot use it until you approve the new content: skillhub skill review <id>`.

## Related code files

Create: `internal/merge3/merge3.go`, `internal/merge3/merge3_test.go`, `internal/app/upstream_update.go`, `internal/app/upstream_update_test.go`.

Modify: `go.mod`, `go.sum`, `internal/skill/lifecycle.go` (constant at lines 57-60), `internal/skill/proposal_store.go` (line 151), `internal/app/skill_add.go` (rename `storeSkillAddProposal` → `storeProposalArtifact`, `storedProposalArtifact` unchanged), `internal/app/skill_lifecycle.go` (kind guard in `LoadSkillProposal`), `internal/canonical/canonical.go` (export `HasConflictMarker`; update its callers in that file), `internal/app/upstream.go` (shared helpers only, if needed), `internal/delivery/mcpserver/skill_tools_test.go` (one guard test).

Do not modify any other file.

## Implementation steps

### Task 4.1 — Dependencies and `merge3`
- Steps:
  1. `go get github.com/epiclabs-io/diff3@latest github.com/aymanbagabas/go-udiff@latest` (default pending user decision Q8). Run `go doc github.com/epiclabs-io/diff3` and use the structured merge function it exports (the research names `Diff3Merge`/`Diff3MergeWithOptions` [UNVERIFIED exact names]); do not use its text `Merge()` output.
  2. Implement Requirement 1. Tests in one table: clean non-overlapping edits; same-line conflict (1 conflict, all four marker lines present); adjacent-line edits; deletion vs edit; identical edits on both sides (0 conflicts); trailing newline preserved and absent cases. When `exec.LookPath("git")` succeeds, also run `git merge-file -p --diff3 -L local -L base -L upstream` on the same inputs and assert the same conflict count and identical output for clean cases; otherwise `t.Skip` only that comparison.
- Verify: `go test ./internal/merge3/ -count=1` exits 0 and prints `ok`.

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

- [ ] Task 4.1 dependencies + `merge3`
- [ ] Task 4.2 proposal kind
- [ ] Task 4.3 preview / write set / confirm
- [ ] Task 4.4 `make check`

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
| diff3 library edge cases (CRLF, whitespace) | M×M | Oracle comparison with `git merge-file`; non-mergeable files fall back to whole-file choice. |
| Transform drift makes the base differ from what was imported | M×M | `files_digest` comparison → `BaseAvailable=false` → explicit choices (never silent overwrite). |
| Upstream adds a huge or binary file | L×M | `Mergeable` false → whole-file choice; mutation limits (16 MiB per change) still apply. |
| Proposal kind unknown to an older binary's `skill confirm` | L×L | Same binary previews and confirms; older binaries report "unknown kind". |

## Security considerations

Upstream bytes are untrusted: they are written only after explicit human confirmation, and the third-party content gate keeps them away from agents until `--approve-content`. Canonical validation in `PlanMutation` (`validateVirtual`) also rejects conflict markers. Manual content is size-bounded by the mutation limits.

## Rollback

Revert the phase commit and run `go mod tidy`. Applied updates are ordinary canonical commits the user can revert with Git.

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
