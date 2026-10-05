---
title: "Phase 3: Upstream check engine"
status: in-progress
---

# Phase 3: Upstream check engine

<!-- Updated: Validation Session 1 - behind = changed files in the skill folder plus the upstream commit date; weekly schedule, explicit checks only -->

## Context

- Plan: [plan.md](./plan.md) (D4, D5, D6). Depends on phases 1–2.
- Read first: `internal/app/source.go:443-613` (`CheckSources`), `internal/source/operational_store.go`, `internal/source/git_repository.go:143-162` (`CurrentRevision`) and the phase 1 additions (`RevisionAt`, `RemoteRefCommit`, `ErrPathNotFound`), `internal/app/upstream_origin.go` (`importedSkillFiles`), `internal/app/curation_home.go:194-290` and `:296-470`, `internal/app/skill_review.go:221` (`locateSkillDir`) and `:308` (`inventorySkillResources`), `internal/app/skill_detail.go` (`ContentTrustFor` shows how both are combined), `schemas/curation-home-v1.schema.json`.

## Overview

Compute a per-skill upstream status from one fetch per source, store the observation in `runtime/operational.db`, and expose a read model that joins it with the skill's origin and its current local files. `CheckSources` drives it, so `skillhub check`, `skillhub source check`, and MCP `source_check` all refresh upstream state. Upstream-only sources stop writing canonical files during checks and stop counting as "ready to distill".

## Requirements

1. Operational table (additive, created in `openOnce`):
   ```sql
   CREATE TABLE IF NOT EXISTS skill_upstream_state (
    skill_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    repository TEXT NOT NULL,
    ref TEXT NOT NULL,
    path TEXT NOT NULL,
    base_commit TEXT NOT NULL,
    checked_commit TEXT NOT NULL,
    checked_commit_at TEXT NOT NULL,
    upstream TEXT NOT NULL CHECK(upstream IN ('same','changed','removed','pinned','unavailable')),
    upstream_digest TEXT NOT NULL,
    changed_files_json TEXT NOT NULL,
    checked_at TEXT NOT NULL,
    last_error TEXT NOT NULL
   ) STRICT;
   ```
   `OperationalStore` methods: `RecordUpstream(ctx, []UpstreamState) error` (one transaction; upsert guarded by `ON CONFLICT(skill_id) DO UPDATE SET ... WHERE excluded.checked_at >= skill_upstream_state.checked_at`, so an older check never overwrites a newer confirm), `ListUpstream(ctx) ([]UpstreamState, error)`, `DeleteUpstreamExcept(ctx, keep []string) error` (removes rows for skills no longer tracked; called at the end of every `CheckSources`). `ChangedFiles` is `[]Change` (`path`, `status`) or `nil` when unknown (JSON `null`).
2. Status derivation is a pure function `deriveUpstreamStatus(origin upstreamOrigin, sourceID string, state *sourcepkg.UpstreamState, localDigest string) (status, local string)`:

   | Condition (first match) | status | local |
   |---|---|---|
   | origin kind `github`/`git`, `sourceID == ""` | `untracked` | from digests |
   | `origin.ref` is 40 lowercase hex | `pinned` | from digests |
   | source record missing, or its normalized repository/ref differ from the origin's (`sameRepository`) | `unavailable` with error `source_origin_mismatch` | from digests |
   | no state, or `state.BaseCommit != origin.commit`, or the state's repository/ref/path differ from the origin's | `unknown` | from digests |
   | `state.Upstream == "unavailable"` | `unavailable` | from digests |
   | `state.Upstream == "removed"` | `upstream_removed` | from digests |
   | `same` + local `clean` or `unknown` | `up_to_date` | |
   | `same` + local `modified` | `modified` | |
   | `changed` + local `clean` or `unknown` | `update_available` | |
   | `changed` + local `modified` | `diverged` | |

   Local (always from the **working tree**, the same inventory `ContentTrustFor` uses, so local status and the content-approval digest agree): `unknown` when `origin.files_digest == ""`; `clean` when the current files digest (`skillruntime.ContentDigest(files, Spec{}, false)` over the working-tree files of the skill folder, `skill.meta.yaml` excluded) equals `files_digest`; otherwise `modified`.
3. Per-source check `checkSourceUpstream(ctx, record, skills)`:
   - Skip network when every skill is pinned (record `pinned`).
   - All adapter calls use the **source record's** repository URL (mirrors are keyed by the raw URL, `git_repository.go:427-446`).
   - `head := RemoteRefCommit(repo, ref)` (ls-remote, phase 1); on error record `unavailable` with a sanitized error (`sanitizeOperationalError`).
   - If every skill already has `checked_commit == head` and `base_commit == origin.commit`, only refresh `checked_at` (no mirror sync).
   - Else sync the mirror once (`CurrentRevision(Locator{repo, ref, ""})`), then per skill:
     - `head == origin.commit` → `same`.
     - Reconstruct the upstream file set at `head`: `RevisionAt(Locator{repo, ref, origin.path}, head)` → `List` → `DiscoverSkillsFromResources` → the item with `SkillDir == ""` → `importedSkillFiles(item, skillID)` (phase 1). `ErrPathNotFound`, or no root `SKILL.md` → `removed`; `ErrHistoryUnavailable`/other errors → `unavailable`.
     - `upstream_digest` = `ContentDigest(files, Spec{}, false)` of that set. Equal to `origin.files_digest` → `same`; else `changed`. When `files_digest` is empty, compare the tree digest with `origin.folder_digest` instead.
     - `ChangedFiles`: reconstruct the base set the same way at `origin.commit` and compare path → digest maps; if the base cannot be read, `ChangedFiles = nil`.
   - Comparing reconstructed file sets (not the raw tree hash) keeps nested skills and import transforms from producing false updates.
   - "Behind" is reported as the number of changed files within the skill folder (`ChangedFiles`), never as a commit count (depth-1 mirrors have no history). Alongside it, `checked_commit_at` records the committer date of `head`, the newest upstream commit read, via a new adapter method `CommitTime(ctx context.Context, repository, commit string) (time.Time, error)` in `internal/source/git_repository.go` (reads the commit object from the mirror under the shared mirror lock; zero time and no error-state change when unreadable). Checks never run in the background: they run only from `skillhub check`, `source check`, `skill outdated --check`, `skill update`, WebUI Check now, and MCP `source_check`; the weekly cadence only makes the source show as due in `skillhub status`.
4. `CheckSources` integration:
   - Load tracked skills (skills whose meta has `provenance.source_id` and a `github`/`git` origin) grouped by source, and the set of source IDs that have at least one learning link (`role` `learning-source` or `inspiration`) via a new `readSkillSourceLinks(root)`.
   - A source with `purpose: upstream` (phase 2) and no learning link is **upstream-only**. Its branch runs **instead of** the existing `adapter.CurrentRevision` call (`internal/app/source.go:494`): run the upstream check, record the existing `CheckState` (availability, next check), set `item.Status` to `updates_available` when any skill is `changed` or `removed`, `up_to_date` otherwise, or `unavailable`; never call the canonical revision update (`internal/app/source.go:537-552`).
   - Any other source (watched or triaged, `purpose` absent, or with a learning link) keeps the current behavior, including canonical revision writes and distill status; if it also has tracked skills, the upstream check runs after it and reuses the synced mirror.
   - `SourceCheckItem` gains `Skills []SkillUpstream \`json:"skills,omitempty"\``. `updates_available` counts toward `Changed`.
5. Read model in `internal/app/upstream.go`: tracked skills are loaded from **working-tree** metas (skill IDs from the catalog `skills` table, meta bytes via `locateSkillDir`). `SkillUpstream` JSON fields `skill_id, source_id, repository, ref, path, base_commit, latest_commit, latest_committed_at (RFC 3339, omitted when unknown), changed_files (int, -1 unknown), files ([]{path,status}, detail only), local, status, checked_at, error, next_action`. `next_action` is a CLI command string: `skillhub skill update <id>` for `update_available`/`diverged`, `skillhub skill upstream <id>` for `upstream_removed`, `skillhub source check <source>` for `unknown`/`unavailable`, `skillhub source backfill` for `untracked`, empty otherwise. Service methods: `ListSkillUpstream(ctx, path) (UpstreamListResult, error)` and `GetSkillUpstream(ctx, path, id) (SkillUpstreamResult, error)`; skills without a `github`/`git` origin are omitted from the list and return `invalid_request` ("Skill <id> was not added from a repository.") from Get.
6. `skillhub status`:
   - The `ChangedSources` query (`curation_home.go:215`) excludes upstream-only sources: `json_extract(content_json,'$.purpose')='upstream'` and no `skill_source_link` entity with role `learning-source`/`inspiration` for that source.
   - New `CurationSummary.UpstreamUpdates int \`json:"upstream_updates"\`` = skills with status `update_available` or `diverged` (an `upstream_removed` skill has nothing to apply, so it shows in `skill outdated` and the WebUI but does not nag in `status`). When > 0, add `ActionItem{Kind: "review_upstream_updates", Count, Priority: 75, Summary: "%d skill(s) have upstream changes to review", Command: "skill_upstream_status"}`, add it to `AttentionItems` and `OptionalItems`, a `recommendationLabel` of `Review upstream updates with skillhub skill outdated`, and a summary case `%d skill(s) have upstream changes to review.`. A failure to read upstream state never fails `status`; it leaves the count at 0.
   - `schemas/curation-home-v1.schema.json`: add `upstream_updates` (integer) to `homeSummary.properties` and `required`.

## Related code files

Create: `internal/app/upstream.go`, `internal/app/upstream_test.go`, `internal/app/source_links.go` (read helpers `readSkillSourceLinks`, `learningSourceIDs`).

Modify: `internal/source/operational_store.go`, `internal/source/git_repository.go` (`CommitTime` only), `internal/app/source.go` (`CheckSources`, `SourceCheckItem`), `internal/app/curation_home.go`, `schemas/curation-home-v1.schema.json`, `internal/delivery/web/testdata/golden/home.json` (regenerate), `internal/app/curation_home_test.go`, `internal/source/operational_store_test.go`.

Do not modify any other file.

## Implementation steps

### Task 3.1 — Operational table
- Steps: add the DDL to the init `ExecContext` in `openOnce`; add `UpstreamState` and the three methods; test round-trip including `ChangedFiles == nil`, upsert replacing a row, and delete.
- Verify: `go test ./internal/source/ -run TestOperationalUpstreamState -count=1` exits 0 and prints `ok`.

### Task 3.2 — Pure status derivation
- Steps: implement `deriveUpstreamStatus` and the local-digest helper; one table test covering every row of the Requirement 2 table plus `files_digest` empty.
- Verify: `go test ./internal/app/ -run TestDeriveUpstreamStatus -count=1` exits 0 and prints `ok`.

### Task 3.3 — Per-source check and `CheckSources` integration
- Steps:
  1. Implement `checkSourceUpstream`, `readSkillSourceLinks`, tracked-skill loading, and the `CheckSources` branch (Requirement 4). Use optional interfaces for `RemoteRefCommit` and `RevisionAt`; a stub adapter without them yields `unavailable` (never a substitute revision).
  2. Integration test with a real file:// repository (configure `uploadpack.allowReachableSHA1InWant true` as in phase 1) (harness of `TestSkillAddRemoteGitRealAdapter`): add `skills/a` (phases 1–2 path), commit an upstream change to `skills/a/SKILL.md`, call `CheckSources(ctx, root, []string{sourceID}, false)`; assert `Results[0].Status == "updates_available"`, `Results[0].Skills[0].Status == "update_available"`, `ChangedFiles == 1`, `LatestCommittedAt` equals the fixture commit's committer time, and `sources/catalog/<id>.yaml` bytes unchanged. Then write a local edit to `skills/default/a/SKILL.md` and assert `GetSkillUpstream` returns `diverged` / `modified`. Then delete `skills/a` upstream, check again, assert `upstream_removed`. Separately, a commit that only adds a nested `skills/a/sub/SKILL.md` leaves `a` `up_to_date`. Regression for learning sources: a source created by `PreviewSourceWatch` (no `purpose`) from which a skill is then imported still gets its canonical `current_revision` updated by `CheckSources` and still counts in `distill_changed_sources`.
- Verify: `go test ./internal/app/ -run 'TestUpstreamCheck|TestSourceChecks' -count=1` exits 0 and prints `ok`.

### Task 3.4 — Status integration
- Steps: implement Requirement 6; extend `internal/app/curation_home_test.go` with one case (fixtures under `testdata/ux/curation-home/*.yaml` stay unchanged): an upstream-only source without `distilled_revision` is not counted in `distill_changed_sources`, and a recorded `changed` state yields `review_upstream_updates` with count 1. Update the schema, then regenerate goldens with `go test ./internal/delivery/web/ -run TestReadEndpointsGolden -update -count=1` and inspect the diff: only `"upstream_updates": 0` may be added to `home.json`.
- Verify: `go test ./internal/app/ -run 'TestCurationHome|TestGetCurationHome' -count=1 && go test ./internal/delivery/cli/ -run TestStatusJSON -count=1 && git diff --stat internal/delivery/web/testdata/golden/` — tests print `ok` and the diff lists only `home.json`.

### Task 3.5 — Gate
- Verify: `make check` exits 0.

## Todo

- [x] Task 3.1 operational table
- [ ] Task 3.2 status derivation
- [ ] Task 3.3 check engine + `CheckSources`
- [ ] Task 3.4 status integration
- [ ] Task 3.5 `make check`

## Success criteria

- No mirror sync when the ref has not moved (one ls-remote per source); exactly one mirror sync per source when it did.
- Upstream-only sources never dirty the canonical tree during checks.
- Every status in the enum is reachable and covered by the table test.

## UX acceptance

`skillhub check` human output (rendered by the existing source check printer) now shows `updates_available` for upstream-only sources; phase 6 adds per-skill lines. `skillhub status` with one pending update:

```text
Status     1 skill(s) have upstream changes to review.
...
Next: Review upstream updates with skillhub skill outdated  skill_upstream_status
```

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Large monorepo mirror sync is slow or exceeds 1 GiB | M×M | Existing mirror limits and timeouts; the source is marked `unavailable` with the limit error; `RemoteRefCommit` avoids syncs when nothing moved. |
| `status` becomes slow with many tracked skills (working-tree hashing) | L×M | Hashing reads only skill folders; failures leave the count at 0; measure in the integration test that a 50-skill workspace status completes (no assertion on time, report it). |
| A forced push removes the base commit | M×M | `ChangedFiles = nil` (shown as "unknown"); status still `update_available`; phase 4 falls back to explicit choices. |
| Existing workspaces relying on canonical `current_revision` updates for upstream-only sources | L×L | Only sources with tracked skills and no learning link change behavior; they had no distill purpose. |

## Security considerations

Errors written to the operational DB pass through `sanitizeOperationalError`. No upstream content is stored, only digests, paths, statuses, and commit IDs.

## Rollback

Revert the phase commit. The extra table is ignored by older binaries; remove `upstream_updates` from the schema and golden in the revert.

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
