---
title: "Phase 5: Source lifecycle and backfill"
status: in-progress
---

# Phase 5: Source lifecycle and backfill

<!-- Updated: Validation Session 1 - watch without skill refused (confirmed); triage --new-skill kept (confirmed) -->

## Context

- Plan: [plan.md](./plan.md) (D1, D2, D11). Depends on phases 2–3.
- Read first: `internal/app/source.go` (`TriageSourceCandidate` at line 201, `previewOnboarding` at line 250, `ConfirmSourceProposal` at line 404, proposal storage at lines 852-945), `internal/app/source_watch.go`, `internal/app/source_import.go`, `internal/app/source_links.go` and `internal/app/upstream_source.go` (phases 2–3), `internal/app/skill_add.go` (`PreviewSkillAdd`), `internal/skill/lifecycle.go:196-251` (`PreviewCreate`), `internal/canonical/canonical.go:146-160` (broken-reference validation), `internal/app/curation_home.go`.

## Overview

Close every path that could leave a source without a skill, give sources an explicit lifecycle (attach, detach, unwatch), make triage end in a skill outcome, add a grouped read model for the Sources view, and attach skills that were added before upstream tracking existed.

## Requirements

0. **Definition.** A skill is *linked* to a source when its `skill.meta.yaml` has `provenance.source_id: <source>` (upstream, or a non-Git `source import`) or a `sources/skills/*.yaml` link names both (any role). A source with no linked skill is an *orphan*. Implement `linkedSkills(root, sourceID)` in `source_links.go` and use it everywhere below.
1. **Learning references** (`internal/app/source_links.go`):
   - `PreviewAttach(ctx, path, SourceAttachInput{SkillID, SourceID, Locator, Ref, Path, Cadence, IdempotencyKey}) (SourceProposal, error)`: exactly one of `SourceID` or `Locator`. The skill must exist. With `Locator` (public GitHub only, validated like `validateSourceWatchLocator`): reuse a source whose repository (compared with `sameRepository`, phase 2), ref, and path all match, else build one with the `source_watch.go` helpers (`resolveSourceWatchRoute`, `deriveSourceWatchConfig`, `buildSourceWatchRecord`). Write `sources/skills/LINK-<skill>--<source>.yaml` (`role: learning-source`). An existing identical link returns `StatusOK` "already linked" with no pins. Never write `provenance.source_id`.
   - `PreviewDetach(ctx, path, skillID, sourceID)`: delete the link. If the source is left with no learning link and no upstream skill, also delete `sources/catalog/<id>.yaml` **only when** no other canonical entity references it (search `canonical_entities.content_json` for `"source_id":"<id>"` outside skills and links); otherwise set `monitoring: {enabled: false, cadence: manual}` and add warning `source_kept_referenced`.
   - Both persist through `storeSourceProposal` and confirm through the existing `ConfirmSourceProposal`, whose success summary switches on `preview.planned.WriteSet.Command`: `source_attach` → `Linked <source> to <skill> as a learning reference.`, `source_detach` → `Unlinked <source> from <skill>.`, `source_unwatch` → see 2, existing commands keep their text. Add the exported accessor `func (p SourceProposal) WriteCommand() string { return p.planned.WriteSet.Command }` so delivery adapters (phase 7 widened `source_watch_confirm`) can check the command without touching unexported fields.
2. **Unwatch** `PreviewUnwatch(ctx, path, sourceID)`: a source with no linked skill and no referencing entity is deleted; otherwise its monitoring becomes `{enabled: false, cadence: manual}` (explicit checks still work; due checks and status nags stop). Summary: `Stopped watching <id>.` or `Removed <id>.`
3. **`source watch` requires a skill and is the same operation as attach**: `SourceWatchInput` gains `SkillID string`. Empty → `invalid_request`: why `A watched source must belong to a skill.`, fix ``Pass --skill-id <id> to use it as a learning reference, run `skillhub skill add <locator>` to vendor its skills, or `skillhub source capture <locator> --reason <text>` to save it for later.`` (confirmed in Validation Session 1). Non-empty → `PreviewSourceWatch` delegates to `PreviewAttach` (one implementation of "create or reuse a source from a URL and link it"; write-set command `source_attach`). `checkExistingSourceCollisions` (`internal/app/source_watch.go:217-221`) switches from exact string comparison to `sameRepository`, so `https://github.com/o/r.git` and `https://github.com/o/r` never create two sources. New learning-source IDs keep `deriveSourceID`.
4. **Triage outcomes**: `SourceTriageInput` gains `NewSkillID string`. Decisions:
   - `accept` requires exactly one of `SkillID` (existing skill; learning link, as today) or `NewSkillID` (the write set also contains a scaffold draft skill from `skill.Manager{}.PreviewCreate(ctx, root, skill.CreateInput{ID, Collection: "default", Name: <id>, Description: "Skill that learns from <identity name>."}, false).WriteSet().Changes` plus the learning link). Neither → `invalid_request` listing both flags and the `import` decision.
   - New `import`: delegates to `SkillAddService.PreviewSkillAdd` with the candidate's locator and a new `SkillAddInput.CandidateID` (`json:"-"`); `PreviewSkillAdd` then appends the candidate file change (`status: accepted`) to its write set. Confirmation is `skillhub skill confirm <proposal>`.
   - `defer` and `reject` unchanged.
5. **Grouped read model**: new method `SourceService.ListSourceGroups(ctx, path) (SourceListResult, error)` fills a new `Groups []SourceRepositoryGroup \`json:"groups,omitempty"\`` in addition to `candidates` and `sources`. `ListSources` stays a cheap file read and leaves `Groups` empty, so `source show` and MCP `source_intake_list` pay no hashing cost; only CLI `source list` and web `GET /api/v1/sources` call `ListSourceGroups`. `SourceRepositoryGroup{Repository string; Sources []SourceSummary}` sorted by repository; `SourceSummary{SourceID, Adapter, Ref, Path, Roles []string ("upstream", "learning"), Monitoring, UpstreamSkills []SourceSkillStatus{SkillID, Status}, LearningSkills []string, PendingInsights int, LastCheckedAt, Availability, LastError, Orphan bool, Status string `json:"status"` (the source record status, e.g. `watching`, `changed`, `distill_pending`), CurrentRevision string `json:"current_revision,omitempty"` and DistilledRevision string `json:"distilled_revision,omitempty"` (each the `Revision.Value` of that record revision, empty when absent), UpstreamOnly bool `json:"upstream_only"`, ReadyToDistill bool `json:"ready_to_distill"`}`. `UpstreamOnly` = `purpose: upstream` and no `learning-source`/`inspiration` link (phase 3 Requirement 6). Put that test in one Go helper, `sourceIsUpstreamOnly(record, links)`, next to `ListSourceGroups`; phase 3's SQL in `ChangedSources` stays as written, and the Task 5.3 test asserts both agree on the same fixture. `ReadyToDistill` = `!UpstreamOnly` and (status `changed` or `distill_pending` or no `distilled_revision`). Phase 10 reads these four fields for the distill handoff. Non-Git sources group under their URL. Statuses come from `ListSkillUpstream` (phase 3); check times from `OperationalStore.List`.
6. **"Import more"** reuses `PreviewSourceImport`. For a whole-repository source (`locator.path: ""`) called without `Path`, the scope defaults to the longest common parent directory of the `origin.path` values of skills already imported from it (empty when none), so a monorepo source does not walk the whole tree against the 8 MiB / 2048-file limits; the existing limit error and its `--path` fix stay for the remaining cases. `DiscoveredSkill` gains `Imported bool \`json:"imported,omitempty"\`` set when an existing skill has `provenance.source_id == source` and the same repository-relative `origin.path`; its skip reason becomes `Already imported as <id>.`
7. **Backfill** (`internal/app/source_backfill.go`): `PreviewBackfill(ctx, path, BackfillInput{SkillID, RepoPath string})` and `ApplyBackfill(ctx, path, BackfillPreview)`:
   - Candidates: (A) origin kind `github`/`git` without `provenance.source_id`; (B) `provenance.source_id` set, `created_by: source_import`, no `origin` block, **and** the source record has `adapter: git` (filesystem and HTTP imports are linked but never upstream-tracked, phase 2). `SkillID` limits to one skill; `RepoPath` (only with `SkillID`) overrides path discovery.
   - Path: (A) try `origin.path` at `origin.commit`; if it holds no `SKILL.md`, search the tree at that commit for folders ending in `/<origin.path>` that contain `SKILL.md`; exactly one match wins; zero or several → skip with reason `path_ambiguous` and fix `skillhub source backfill --skill <id> --path <repo-path>`. (B) `repoRelativeSkillPath(source.Locator.Path, provenance.path)`, then `provenance.path` alone.
   - Commit: (A) `origin.commit`; (B) `provenance.revision`. Missing commit upstream → attach without `files_digest` and warning `base_unavailable_local_edits_unknown`.
   - Writes: origin rebuilt with `buildGitOrigin` (path, folder_digest, files_digest from the reconstructed base), `provenance.source_id` from `ensureUpstreamSource` (deduplicated per repository and ref in the batch), removal of legacy `provenance.revision`/`provenance.path` for (B), and deletion of `sources/skills/LINK-<skill>--<source>.yaml` files with `role: origin`. One write set (`Command: "source_backfill"`), applied with `confirmAndPublish`. Preview lists every skill as `attach`, `skip` (with reason), or `already_tracked`.
   - No persisted proposal: `ApplyBackfill` re-runs the preview and applies its fresh pins (same pattern as `skill add --yes`).
8. **Status actions** (`curation_home.go`): `track_upstream_skills` (count of candidates A+B as defined above, so filesystem/HTTP imports never count, priority 30, `Command: "skill_upstream_status"` because action commands name MCP tools an agent can call and backfill is CLI-only, label `Track upstream updates: skillhub source backfill`) and `link_orphan_sources` (sources with no linked skill, priority 20, `Command: "source_intake_list"`, label `Link or remove sources without skills: skillhub source list`). Both add to `OptionalItems` and `AttentionItems`.

## Related code files

Create: `internal/app/source_backfill.go`, `internal/app/source_backfill_test.go`, `internal/app/source_links_test.go`.

Modify: `internal/app/source_links.go`, `internal/app/source.go`, `internal/app/source_watch.go`, `internal/app/source_import.go`, `internal/app/skill_add.go` (`CandidateID` only), `internal/app/curation_home.go`, `internal/app/source_test.go`, `internal/app/source_security_test.go` (line 112: pass `SkillID` of a skill created at test start, keeping the expiry assertions), `internal/app/source_watch_test.go`, `internal/app/source_import_test.go`, `internal/app/curation_home_test.go`, and these test files whose fixtures watch or accept without a skill (test changes only; their production code is owned by phases 6–7): `internal/delivery/cli/source_test.go` (lines 143 and 195: add `--skill-id <id>` of a skill created at test start), `internal/delivery/cli/source_watch_test.go` (case 2 at line 24: expect `A watched source must belong to a skill.`; phase 6 restores a network-error case with `--skill-id`), `internal/delivery/mcpserver/source_import_test.go` (line 49: add `"skill_id"` of a skill created at test start).

Do not modify any other file.

## Implementation steps

### Task 5.1 — Attach, detach, unwatch
- Steps: implement Requirements 1–2 and the summary switch. Tests (`TestSourceLinks`): attach by locator creates source + link and leaves the skill's meta byte-identical (no `source_id`); attach again → "already linked"; detach of the only link deletes the source; detach when an observation references the source keeps it with monitoring off and warning `source_kept_referenced`; unwatch of a source with upstream skills turns monitoring off and keeps the record; `canonical.Validate(root)` returns no issues after each confirm.
- Verify: `go test ./internal/app/ -run TestSourceLinks -count=1` exits 0 and prints `ok`.

### Task 5.2 — Watch and triage outcomes
- Steps: implement Requirements 3–4. The skill requirement in `PreviewSourceWatch` runs after `validateSourceWatchLocator` (so local-folder and empty-locator errors keep precedence) and before any network call. Update `internal/app/source_watch_test.go` and `internal/app/source_test.go` cases that watch or accept without a skill so they pass `SkillID` (keep their original assertions), and add cases: watch without skill → `invalid_request`; accept with `NewSkillID` creates `skills/default/<id>/SKILL.md` (draft scaffold) and the link in one operation; decision `import` produces a skill-add proposal whose write set includes `sources/intake/<candidate>.yaml`; `source watch` of `https://github.com/o/r.git` when a source for `https://github.com/o/r` with the same ref and path exists reuses it.
- Verify: `go test ./internal/app/ -run 'TestSourceWatch|TestSourceTriage|TestSourceChecks|TestSourceCapture|TestLocalWatch|TestMonitoringDisabled|TestSourceProposalExpiry' -count=1 && go test ./internal/delivery/cli/ -run 'TestSourceWatch|TestSourceCapture|TestSourceImportCLI' -count=1 && go test ./internal/delivery/mcpserver/ -run 'TestSourceWatch|TestSourceImport' -count=1` exits 0 and each prints `ok`.

### Task 5.3 — Grouped list and import-more flag
- Steps: implement Requirements 5–6. Test (through `ListSourceGroups`): two sources from the same repository (different refs) appear in one group; a source with no links has `Orphan: true`; an upstream-only source has `UpstreamOnly: true`, `ReadyToDistill: false`, and is excluded from `ChangedSources` (both rules agree); a never-distilled source with a learning link has `ReadyToDistill: true`; import preview marks an already-imported skill `Imported: true`.
- Verify: `go test ./internal/app/ -run 'TestSourceList|TestSourceImport' -count=1` exits 0 and prints `ok`.

### Task 5.4 — Backfill
- Steps: implement Requirement 7. Test (`TestSourceBackfill`, real file:// repository): write a legacy GitHub-add meta by hand (origin with scope-relative `path: a`, no `source_id`, no `files_digest`) for a repo containing `skills/a/SKILL.md`; preview → `attach` with path `skills/a`; apply → meta has `source_id`, `origin.path: skills/a`, `files_digest`; a legacy `source_import` skill from a Git source with a `role: origin` link gets an origin block and the link is deleted; a skill imported from a `filesystem` source is neither a candidate nor counted by `track_upstream_skills`; an ambiguous path (two `*/a` folders) is skipped with `path_ambiguous`; a second preview reports `already_tracked`.
- Verify: `go test ./internal/app/ -run TestSourceBackfill -count=1` exits 0 and prints `ok`.

### Task 5.5 — Status actions
- Steps: implement Requirement 8 with one test case each in `curation_home_test.go`.
- Verify: `go test ./internal/app/ -run 'TestCurationHome|TestGetCurationHome' -count=1` exits 0 and prints `ok`.

### Task 5.6 — Gate
- Verify: `make check` exits 0.

## Todo

- [x] Task 5.1 attach / detach / unwatch
- [x] Task 5.2 watch + triage outcomes
- [x] Task 5.3 grouped list + import-more
- [x] Task 5.4 backfill
- [x] Task 5.5 status actions
- [x] Task 5.6 `make check`

## Success criteria

- No service in `internal/app` can confirm a write set that leaves a source with zero linked skills, except when it deletes that source in the same write set.
- Learning links never alter `skill.meta.yaml`.
- Backfill is repeatable (second run is a no-op) and never guesses an ambiguous path.

## UX acceptance

Service-level; phase 6 renders. Required error text (watch without skill):

```text
Error: A watched source must belong to a skill.
Fix:   Pass --skill-id <id> to use it as a learning reference, run `skillhub skill add <locator>` to vendor its skills, or `skillhub source capture <locator> --reason <text>` to save it for later.
```

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Scripts or agents calling `source watch` without a skill break | M×M | Clear fix text; MCP description updated in phase 7; documented in phase 13. Refusal confirmed in Validation Session 1. |
| Deleting a source that distilled records reference breaks validation | M×H | Reference check before delete; fallback to monitoring off; `canonical.Validate` asserted in tests. |
| Backfill on a workspace with many legacy skills hits the network per repository | M×L | One `ensureUpstreamSource` and one mirror sync per repository and ref; per-skill failures become skips, not aborts. |
| Legacy `origin.path` cannot be recovered | M×L | Explicit `--path` override; skip with reason; never guess. |

## Security considerations

Attach by locator accepts only public GitHub URLs (same validation as watch). Backfill reads upstream through the existing safe transport and never changes trust class (all candidates are already third-party).

## Rollback

Revert the phase commit. Backfilled metas remain valid for this binary; an older binary needs `files_digest` removed. Deleted `role: origin` links can be restored from Git history.

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
