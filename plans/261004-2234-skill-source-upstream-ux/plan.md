---
title: "Skill-centric sources and WebUI v1 completion"
description: "Tie every source to skills: track the upstream repo of each vendored skill, report drift, review and apply updates with a 3-way merge through preview/confirm, attach learning references to skills, expose it consistently in CLI, MCP, and the WebUI, then finish the WebUI v1 (distill handoff and runs, Inbox, Patch Composer, hardening, release, docs)."
status: pending
priority: P1
effort: 168h
branch: feat/skill-source-upstream
tags: [feature, backend, api, frontend, cli, mcp, docs, security, release]
blockedBy: []
blocks: []
created: 2026-10-04
---

# Skill-centric sources and WebUI v1 completion

## Overview

A source becomes an attribute of a skill, in one of two roles:

- **Upstream**: the repository a skill was vendored from. Skill Hub records the base commit, checks the repository, reports per-skill drift (`update_available`, `modified`, `diverged`, `upstream_removed`, ...), and applies an upstream update through a 3-way merge that ends in the existing preview/confirm mutation flow. An update changes the content digest, so a third-party skill returns to `review_required` until a human approves it again; the flow never re-approves.
- **Learning reference**: a repository or document used to improve one of your own skills. Distillation insights attach to that skill.

`skill add` and `source import` from a repository attach the repository's source automatically (one repository and ref equals one source, many skills). Existing skills are attached by an explicit `skillhub source backfill`. Every new source has at least one linked skill or stays pending in the intake queue.

Since 2026-10-05 this plan also owns the rest of the WebUI v1. The WebUI plan `plans/261003-1645-webui-v1-implementation/` is closed: its phases 1–3 shipped; its phase 4 Sources/Watch work is replaced by phases 8–9 here; its remaining Handoff/Runs, Inbox/Insight/Composer, hardening/release, and documentation work moved to phases 10–13 (see "Plan amendment 2026-10-05").

## Verified starting facts (2026-10-04)

- `skill add` from GitHub writes `provenance.origin` but no `provenance.source_id` and no link (`internal/app/skill_add.go:547-578`). `source import` writes `provenance.source_id` plus a `role: origin` link and no `origin` block (`internal/app/source_import.go:220-260`). Triage accept with `--skill-id` writes a `role: learning-source` link (`internal/app/source.go:340-351`). The catalog accepts roles `learning-source`, `origin`, `inspiration` (`internal/catalog/input.go:443`).
- `origin.path` is written as `item.SkillDir`, which is relative to the URL's sub-path, not the repository root (`internal/app/skill_add.go:553-555`, `internal/app/skill_discovery.go:84`). For `skill add <repo>/tree/main/skills --all` the stored path loses the `skills/` prefix.
- `origin.folder_digest` for GitHub adds is the digest of the URL scope's tree object, shared by every skill of an `--all` add (`skill_add.go:428`, `internal/source/git_repository.go:143-162`).
- Git mirrors are depth-1 fetches of one ref (`git_repository.go:469-566`): commit history is not available, so commit counts cannot be computed locally, and the base commit may be missing on a fresh machine.
- `CheckSources` writes the canonical source record on every content change (`internal/app/source.go:537-552`), and `skillhub status` counts every source without `distilled_revision` as "ready to distill" (`internal/app/curation_home.go:215`). Auto-created upstream sources would flood both unless separated.
- The skill proposal store accepts only `lifecycle` and `add` kinds (`internal/skill/proposal_store.go:151`); new kinds register a confirmer through `RegisterProposalConfirmer` (`internal/app/skill_lifecycle.go:494`).
- Content trust: third-party = origin kind `github`/`git` or `provenance.source_id` set (`internal/skillruntime/trust.go:28-30`); `ContentDigest(files, Spec{}, false)` is a files-only digest (`trust.go:43-46`). Approval is CLI-only (`skillhub skill edit <id> --approve-content <digest>`).
- The WebUI has no source screens (`web/src/routes.tsx` maps `/sources*` to `LaterPhasePage`) and Skill Detail provenance is hard-coded (`web/src/screens/skill-detail/ReviewTab.tsx:174-198`).

## Key decisions

<!-- Updated: Validation Session 1 - D5, D6, D8, D10, D11 and data flow reflect the 2026-10-05 answers -->

| ID | Decision |
|---|---|
| D1 | **Roles.** Upstream = skill meta `provenance.source_id` + `provenance.origin` (authoritative, one per skill). Learning reference = link file `sources/skills/LINK-<skill>--<source>.yaml` with `role: learning-source` (legacy `inspiration` is read as a learning reference). `role: origin` links are legacy: still valid, no longer written, removed by backfill. A learning link never sets `provenance.source_id`, so it never changes a skill's trust class. |
| D2 | **One source per (repository, ref).** Upstream attachment reuses any source whose normalized repository URL and ref match; otherwise it creates one with ID `<owner>-<repo>` (sanitized), suffixed with the ref and then `-2`, `-3` on collision. New upstream sources are whole-repository (`locator.path: ""`), carry `purpose: upstream`, monitoring enabled, cadence `weekly`. Sources without `purpose` (created by watch/triage) keep their distill behavior even when skills are imported from them. |
| D3 | **Origin fields.** `origin.path` = repository-root-relative skill folder. `origin.folder_digest` = digest of that folder's tree object at `origin.commit`. New optional `origin.files_digest` = `skillruntime.ContentDigest(files, Spec{}, false)` over the files as written from that commit (after import transforms). Local edits = current files digest differs from `files_digest`. |
| D4 | **Volatile upstream state** lives in `runtime/operational.db` table `skill_upstream_state`. No derived catalog schema change: edges are read from `canonical_entities.content_json`. `DerivedSchemaVersion` is untouched (the runtime plan owns its bump). |
| D5 | **Checks are explicit and per source**: `skillhub check`, `skillhub source check`, `skill outdated --check`, `skill update`, WebUI "Check now", MCP `source_check`. There is no background daemon; the weekly cadence of auto-created sources only marks them due in `skillhub status`. Each check probes the ref with ls-remote (`RemoteRefCommit`, no mirror write) and syncs the mirror at most once per source, only when the ref moved. Upstream status compares reconstructed file sets (same discovery and transforms as `skill add`), not raw tree hashes. Upstream-only sources (`purpose: upstream`, no learning link) never write canonical files during a check. |
| D6 | **Status enum** per skill: `up_to_date`, `update_available`, `modified`, `diverged`, `upstream_removed`, `pinned`, `unavailable`, `unknown`, `untracked`. "Behind" is shown as the number of files changed within the skill folder, never a commit count, together with the committer date of the newest upstream commit read (`latest_committed_at`). |
| D7 | **Base reconstruction**: fetch `origin.commit` by SHA into the mirror on demand, re-apply the import transforms, and compare with `files_digest`. If the base is unavailable or does not reproduce `files_digest`, every file that differs between local and upstream needs an explicit choice. |
| D8 | **Merge engine = the system `git`**: `git merge-file -p --diff3` for the 3-way merge (exit `0` clean, `1`–`127` conflict count, anything else an error) and `git diff --no-index` for display, run through the existing `offlineGitCommand` helper (`internal/app/operations.go:156`) from a thin `internal/app/upstream_merge.go`; inputs go to `0600` temp files under the workspace `runtime/tmp/`. No Go merge or diff library is added. Binary, non-UTF-8, or files over 256 KiB are whole-file choices and never reach git. |
| D9 | **Apply** via a new proposal kind `upstream_update` (persisted, pinned, `BeforeDigest` on every file). Confirm never touches `quality.content_reviewed_digest`; the preview states whether agents lose access until re-approval and prints `skillhub skill review <id>`. |
| D10 | **Agents can report upstream updates but never apply them.** Updates are reviewed and applied only in the CLI (`skillhub skill update`) or the WebUI, where the diff is visible: taking an update is a review decision, and applying it flips a third-party skill to `review_required` mid-work. The only MCP upstream tool, `skill_upstream_status`, returns statuses, paths, counts, commits, and dates (no file content, diff, or commit message) plus `skillhub skill update <id>` and a WebUI hint. No MCP tool previews an update or returns pins for one; generic skill confirm paths refuse `upstream_update` proposals (defense in depth). |
| D11 | **No orphan sources** on every write path. `source watch` without a skill target is refused with guidance (confirmed 2026-10-05); triage `accept` needs `--skill-id` or `--new-skill`, or the new `import` decision. Pre-existing orphans are reported by `skillhub status`, never as validation errors. |
| D12 | **CLI exit codes**: 0 on success including "updates available"; `skill outdated --exit-code` exits 1 when any skill is `update_available`, `diverged`, or `upstream_removed`; 2 for invalid requests or application errors (`internal/delivery/cli/output.go:17-46`); `--json` never changes the exit code. |

## Phases

| # | Phase | Depends on | Effort | Status |
|---|---|---|---|---|
| 1 | [Upstream provenance model](./phase-01-upstream-provenance-model.md) | runtime plan | 8h | Done |
| 2 | [Source attachment on add and import](./phase-02-source-attachment-on-add-and-import.md) | 1 | 7h | Done |
| 3 | [Upstream check engine](./phase-03-upstream-check-engine.md) | 2 | 10h | Done |
| 4 | [Update merge and apply](./phase-04-update-merge-and-apply.md) | 3 | 14h | Done |
| 5 | [Source lifecycle and backfill](./phase-05-source-lifecycle-and-backfill.md) | 2, 3 | 10h | Done |
| 6 | [CLI: upstream and sources](./phase-06-cli-upstream-and-sources.md) | 3, 4, 5 | 9h | Done |
| 7 | [MCP tools and curator](./phase-07-mcp-tools-and-curator.md) | 3, 4, 5 | 6h | Done |
| 8 | [Web API](./phase-08-web-api.md) | 3, 4, 5 | 6h | Done |
| 9 | [Web UI](./phase-09-web-ui.md) | 8 | 8h | Pending |
| 10 | [Distill handoff and runs](./phase-10-distill-handoff-and-runs.md) | 9 | 18h | Pending |
| 11 | [Inbox, Insight, Patch Composer](./phase-11-inbox-insight-patch-composer.md) | 10 | 40h | Pending |
| 12 | [WebUI hardening and release](./phase-12-webui-hardening-and-release.md) | 9–11 | 22h | Pending |
| 13 | [Documentation and plan close](./phase-13-documentation.md) | 1–12 | 10h | Pending |

Phases run strictly in order, one at a time (user decision 2026-10-05: phases 10–13 start only after phase 9). Each phase file lists the only files it may modify.

## Data flow (end state)

```text
skill add <repo> / source import ──► origin{repo,ref,commit,path,folder_digest,files_digest}
                                     + provenance.source_id ──► sources/catalog/<repo-source>.yaml (reused per repo+ref)
skillhub check / source check / skill outdated --check / Web "Check now"
   └─► CheckSources ─► per source: RemoteRefCommit (ls-remote) ─► (moved) sync mirror once ─► per skill: reconstructed file-set digest at HEAD
         └─► operational.db skill_upstream_state{checked_commit, upstream_digest, changed_files, status, checked_at}
read paths (skill outdated, skill upstream, Skills list, Skill Detail, status)
   └─► state ⨝ working-tree origin ⨝ local files digest (working tree, same inventory as content trust) ─► status enum
skill update (CLI) / Web Review update   (no MCP path; MCP skill_upstream_status only reports)
   └─► base = transform(mirror@origin.commit) · upstream = transform(mirror@checked_commit) · local = working tree
   └─► per file: git merge-file / git diff --no-index (offlineGitCommand) ─► unresolved? return review model : persist upstream_update proposal (pins)
confirm ─► mutation (files + meta origin.commit/folder_digest/files_digest) ─► catalog publish
   └─► content digest changes ─► third-party skill becomes review_required ─► human: skill review + --approve-content
```

## Cross-plan dependency

- The runtime plan `plans/261004-1547-skill-execution-measurement-routing` is complete and merged into `main` (`338eff5`). Its WebUI contracts must be kept: Skill Detail reads trust from `GET /api/v1/skills/{id}/review`, runtime from `/runtime`, usage from `/usage`; `ContentTrustCard.tsx`, `RuntimeTab.tsx`, and the Usage tab stay as they are. The one deliberate change is phase 9's correction of the TypeScript `SkillProvenance` type to the flat Go JSON.
- Ownership kept from the runtime plan: resolver, telemetry, catalog schema version, `content_trust` types. This plan adds no telemetry event types and never edits `quality.content_reviewed_digest`.
- The WebUI plan `plans/261003-1645-webui-v1-implementation/` is closed and absorbed (phases 10–13). Its earlier user decisions still apply where the moved phases cite them; its guard script is retired (user decision 2026-10-05) and replaced by this plan's gates and executor rules.
- Evidence for the reconciliation: `plans/261003-1645-webui-v1-implementation/reports/261005-o-a-b-compatibility-review.md`.

## Backwards compatibility

- Canonical: two new optional fields (`provenance.origin.files_digest` in skill metadata, `purpose` in source records). Older binaries reject unknown keys in both (`strictYAML` with `KnownFields`), so a workspace that runs `skill add`, `source import`, `skill update`, or `source backfill` with this binary needs this binary or newer (documented in phase 13).
- `origin.path` semantics are corrected for new writes; backfill rewrites legacy paths after verifying them against the repository.
- JSON contracts are additive: `source list` keeps `candidates` and `sources` and adds `groups`; `skill list` entries gain optional `upstream_status`; `source_check` results gain optional `skills`.
- Behavior changes (accepted by the user's target UX): `skill add` from a repository creates or reuses a source; `source watch` without `--skill-id` is refused with guidance (vendor with `skill add`, link with `--skill-id`, or `source capture`); `source triage --decision accept` requires a skill target; `skillhub status` stops counting upstream-only sources as "ready to distill".

## Rollback

Each phase is one or more focused commits; revert with `git revert` in reverse order. Disposable state: `skill_upstream_state` is ignored by older binaries; deleting `runtime/operational.db` loses only check history. Canonical rollback: skills written with `files_digest` must have that key removed before an older binary can read them (`skillhub validate` names each file); source records created by attachment can stay (valid for older binaries).

## Acceptance criteria (whole plan)

- [ ] `skillhub skill add https://github.com/<o>/<r> --all --yes` creates one source and sets `provenance.source_id`, repository-relative `origin.path`, and `origin.files_digest` on every added skill; a second add from the same repository and ref reuses the source.
- [ ] `skillhub skill outdated --check` lists every tracked skill with status, current/latest commit, the latest upstream commit date, the number of files changed in the skill folder, and local state; `--exit-code` exits 1 only when something needs action; `--json` emits a stable `status` enum.
- [ ] `skillhub skill update <id>` previews per-file changes; a clean update applies through confirm; a conflicting update refuses to build pins until each conflict is resolved; after apply a third-party skill reports `review_required` and the output names `skillhub skill review <id>`.
- [ ] Local edits are never overwritten silently: every file changed both locally and upstream is merged or requires an explicit choice.
- [ ] `skillhub source list` groups by repository; `skillhub source backfill` attaches existing skills; no write path creates a source with zero linked skills.
- [ ] MCP can report upstream status but has no way to preview or apply an update (no such tool; generic confirm tools refuse `upstream_update` proposals); MCP responses never carry upstream file content or diffs; the curator copies stay identical.
- [ ] WebUI: Skills list badge and filter, one Skill Detail Sources tab (Upstream and Learning sections, indicator dot when an update is available), `/sources` grouped view, with loading, empty, and error states.
- [ ] WebUI provenance renders the flat Go fields, and an upstream update or Composer apply refreshes the Runtime tab and trust card.
- [ ] WebUI distill loop: select learning sources on `/sources`, copy a handoff brief, open returned runs, cancel an active run; no web route can start, retry, or submit a run (`TestRoutesNeverMutateRuns`).
- [ ] WebUI improvement loop: paged Inbox, Insight decisions, Patch Composer with full evidence mapping and conflict detection; MCP paging behavior unchanged.
- [ ] No route left on `LaterPhasePage`; every route passes the axe and 360 px sweep; release smoke proves `skillhub serve web` serves the authenticated UI; notices ship with the release.
- [ ] `make check`, `make web-check`, and `make web-e2e` pass; docs, including `docs/contracts/web-api.md`, match shipped behavior.

## Executor notes

- Branch: `feat/skill-source-upstream` already exists; on 2026-10-05 the controller rebased it onto `main` after the runtime plan merged. The executor does not create, rebase, or pull the branch; it verifies `git branch --show-current` prints `feat/skill-source-upstream` before phase 1 and never commits to `main`.
- One phase at a time, in table order. Before starting a phase, set its file frontmatter `status: in-progress` and its row in this table to `In progress`; after its gates pass, set `status: done` and `Done`. If `ak plan --help` is available, use its status commands instead of hand edits.
- Gates for every Go phase: `make check` exits 0. For web phases also: `cd web && npm run lint && npm run typecheck && npm test && npm run build` exit 0 (or `make web-check`).
- Commits: conventional commit format (`feat(upstream): ...`, `fix(source): ...`, `docs: ...`), no AI references, and no plan IDs, phase numbers, or finding codes in code comments, test names, or commit messages.
- Tests follow `AGENTS.md` "Testing": one owner test per contract at the strongest boundary; extend existing tables before adding near-duplicates.
- Every phase file carries a Failure Protocol. A failed Verify step means stop and report; never weaken a test to pass.
- Hard rules carried over from the closed WebUI plan (they replace its guard): never delete, rename, or skip a test, except the single move of `TestOpaqueCursorMultiPageAndIntegrity` from `internal/delivery/mcpserver/server_test.go` to `internal/delivery/paging/paging_test.go` in Task 11.1 (same name and assertions); never add `t.Skip`, `.skip`, `.only`, `xit`; never edit golden files by hand (regenerate with `-update` inside the owning task and read the diff); no `TODO`, `FIXME`, `XXX`, `HACK`, `nolint`, `@ts-ignore`, `@ts-expect-error`, `eslint-disable`, `testing.Testing()`, `TestMain`, or code that behaves differently under test; no exports, wrappers, or seams used only by tests; Go functions at most 120 lines and 6 parameters; never edit `.golangci.yml`; `make lint` reports no new issues; satisfy intent, not grep (renaming to dodge a check is a failure); start long-running processes only as a task says and stop them before the task ends.
- After each of phases 8–12, the controller runs a code review of the phase diff before the next phase starts; confirmed findings are fixed in that phase.
- Write a short completion report per phase to `reports/` named `executor-<YYMMDD-HHMM>-<phase-slug>.md` (what changed, commands run, deviations).

## Red Team Review

### Session — 2026-10-04
**Reviewers:** Security Adversary (Fact Checker), Failure Mode Analyst (Flow Tracer), Assumption Destroyer (Scope Auditor), Scope & Complexity Critic (Contract Verifier).
**Findings:** 37 raw, 24 after deduplication (20 accepted, 4 rejected). **Severity (deduplicated):** 3 Critical, 10 High, 11 Medium.

| # | Finding | Severity | Disposition | Applied to |
|---|---|---|---|---|
| 1 | Web route `/skills/{id}/upstream/confirm` conflicts with `/skills/proposals/{proposal_id}/confirm`; `ServeMux` panics | Critical | Accept | Phase 8 (route moved to `/api/v1/upstream/proposals/{id}/confirm`), Phase 9 |
| 2 | SHA-fetch test cannot pass (`git-upload-pack` refuses unadvertised SHAs); production path untested on fresh clones | Critical | Accept | Phase 1 (fixture config + negative fixture), Phases 3–4 (mirror-deleted subtest) |
| 3 | Generic confirm paths (MCP `skill_*_confirm`, web skill confirm) would apply `upstream_update` proposals | Critical | Accept | Phase 4 (kind guard in `LoadSkillProposal` + tests); kept after Validation Session 1 as defense in depth |
| 4 | MCP resolutions contradict D10 (agent could drop local edits or hide a fix) | High | Accept | Phase 7, D10. Superseded by Validation Session 1: agents cannot preview or apply updates at all |
| 5 | Base not reproducible (description fallback, nested skills, transforms) | High | Accept | Phase 1 (`importedSkillFiles`), Phases 3–4 (file-set reconstruction + tests) |
| 6 | `ResolveRefCommit` always fetches; double sync per check | High | Accept | Phase 1 (`RemoteRefCommit`), Phase 3, D5 |
| 7 | Mirror lock re-entrant deadlock, no deadline, readers unlocked | High | Accept | Phase 1 (`withMirrorLock`, 30 s, shared reads) |
| 8 | Watched sources reclassified as upstream-only stop distilling | High | Accept | Phase 2 (`purpose: upstream`), Phase 3 (regression test) |
| 9 | `source_security_test.go:112` breaks in Phase 5 outside its file list | High | Accept | Phase 5 |
| 10 | Backfill/status nag targets filesystem and HTTP imports | High | Accept | Phase 5 (adapter `git` only) |
| 11 | Curator embed test needs fenced list; contract version unchanged | High | Accept | Phase 7 (list + contract version 2 + pinned tests) |
| 12 | Two attach-by-URL paths with different matching; normalization spec contradicts its test | High | Accept | Phases 2, 5 (watch delegates to attach; `sameRepository` rule fixed), Phase 7 (`source_watch_confirm` widened instead of a new tool) |
| 13 | `Diff` digest mismatch; missing path indistinguishable from errors | High | Accept | Phase 1 (`ErrPathNotFound`), Phase 3 (set comparison, no `Diff`) |
| 14 | Upstream status on shared hot read paths (`ListSkills`, `ListSources`) | Medium | Accept | Phases 5, 8 (`ListSourceGroups`, `ListSkillsWithUpstream`) |
| 15 | State rows go stale, last-writer-wins | Medium | Accept | Phase 3 (repo/ref/path columns, guarded upsert, pruning) |
| 16 | Stub adapters silently get substitute revisions | Medium | Accept | Phases 1, 3 (`ErrHistoryUnavailable`) |
| 17 | Confirm through stored proposal loses `UpstreamSource`/`TrustImpact` | Medium | Accept | Phases 2, 4 (re-derive from write set) |
| 18 | Upstream path names unsanitized (terminal and agent injection); `--write-conflicts` symlink escape | Medium | Accept | Phases 4, 6, 7 |
| 19 | Case-fold collisions; empty `BeforeDigest` filled at plan time | Medium | Accept | Phase 4 |
| 20 | Marker check mismatch between the merge engine and canonical validation | Medium | Accept | Phase 4 (shared `canonical.HasConflictMarker`, `blocked` status) |
| 21 | Source record repository may differ from origin repository | Medium | Accept | Phase 3 (`source_origin_mismatch`), Phase 6 (repository shown) |
| 22 | Fetch by SHA accepts fork-network commits; no ancestry proof | Medium | Reject | Writing `origin.commit` requires canonical write access, which already allows editing the skill files directly; the content-approval gate still requires human review before agents use anything. Fetched refs are kept out of ref resolution (Phase 1). |
| 23 | `source import` should import the checked revision, not HEAD | Medium | Reject | The import preview itself is the inspection: it pins the exact commit and file set, and confirm applies only those bytes (`PlanMutation` pins). |
| 24 | Prune `refs/skillhub/commits/*`; exclude noise files from local digest; `skill_add_preview` absent from compatible tools | Medium | Reject | Mirror is a disposable cache (limit breach re-clones); excluding files would desynchronize local status from the content-trust digest (`trust.go:40-46`); the curator already routes to `skill_add_preview` today (`system-skills/curator/SKILL.md` intent table). |

Also applied without a finding number: dropped `CommitTime`/commit dates (unrequested), added runtime phase 12a ownership (`ContentTrustCard`, `RuntimeTab`, `SkillProvenance`), exported `SourceProposal.WriteCommand()`, broadened the Task 1.0 overlap check.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, phase-01 … phase-10.
- Decision deltas checked: 14 (RemoteRefCommit, purpose marker, file-set status, MCP no-resolution, kind guard, confirm route, widened `source_watch_confirm`, `ListSourceGroups`/`ListSkillsWithUpstream`, lock helper, path sanitization, contract version 2, dropped commit dates, branch rule, 12a ownership).
- Stale references searched (`ResolveRefCommit` as a probe, `CommitTime`, `committed_at`, `source_change_confirm`, per-source mutex, catalog-based local digest, "records an intake candidate", adapter `Diff` for upstream, `HasConflictMarkers`): 4 reconciled after the sweep, 0 remaining.
- Unresolved contradictions: 0.

## Validation Log

### Verification Results
- **Tier:** Full (10 phases; all four roles, through the planner pass and the four red-team reviewers).
- **Claims checked:** 143 by the planner (32 `file:line` citations, 89 existing file paths of 116 cited — the other 27 are files to create —, 22 helper symbols), plus about 95 by the Fact Checker and 26 traced flows by the Flow Tracer.
- **Verified:** all remaining claims after fixes. **Failed (all corrected in the plan):** 3 off-by-a-few line citations in plan.md; `ResolveRefCommit` described as a cheap probe; `sameRepository` spec vs its test; empty `BeforeDigest` semantics; generic confirm paths assumed kind-aware; `termui.Table` assumed to truncate; `source_security_test.go` missing from Phase 5. **Unverified:** 0 (the earlier unverified library API was removed by Validation Session 1; the merge engine is now `git merge-file`, whose exit-code semantics were confirmed by research).
- Baseline on 2026-10-04: `go build ./...` ok; `go test ./... -count=1` 1202 passed in 24 packages; `cd web && npm test` 36 passed.
- `ak plan validate plans/261004-2234-skill-source-upstream-ux`: valid.

### Whole-Plan Consistency Sweep
- Files reread: plan.md and all 10 phase files after red-team edits.
- Decision deltas checked: 14 (listed above). Reconciled stale references: 4 (`ResolveRefCommit` mention in phase 3 context, commit-date UX lines in phases 6 and 9, `source_change_confirm` in phase 5). Unresolved contradictions: 0.

### Validation Interview (2026-10-05)
The user answered the 8 decision questions:

| # | Question | Answer | Propagated to |
|---|---|---|---|
| 1 | How to show "behind" | Number of files changed within the skill folder, plus the commit date of the newest upstream commit read | D6; phase 3 (`checked_commit_at`, `CommitTime`, `latest_committed_at`); phase 6 (`UPDATED` column, detail and update lines); phase 9 (commit date); phase 10 |
| 2 | Merge base source | Fetched on demand by commit hash into the repository cache | D7 unchanged; phases 1, 4 unchanged |
| 3 | `source watch` without a skill | Refuse with guidance (`skill add`, `--skill-id`, `source capture`) | D11; phase 5 (marker resolved) |
| 4 | Schedule for auto-created upstream sources | Weekly; `status` shows them due until `skillhub check`; no background daemon | D5; phase 2 (marker resolved), phase 3, phase 10 |
| 5 | May agents apply upstream updates? | **No.** CLI or WebUI only, where the diff is visible. MCP reports status and returns `skillhub skill update <id>` plus a WebUI hint. Generic confirm refusal kept as defense in depth | D10; phase 4 (no MCP entry, no metadata-only variant; guard wording); phase 7 (removed `skill_upstream_update_preview`/`_confirm`; curator hands off; tests); phases 6, 8 (rollback/overview wording), 10; acceptance criteria |
| 6 | Skill Detail layout | One Sources tab with Upstream and Learning sections, plus an indicator dot on the tab label when an update is available | phase 9 |
| 7 | Triage `--new-skill` | Keep | phase 5 (unchanged) |
| 8 | Merge engine | Neither a library nor own diff3: `git merge-file -p` and `git diff --no-index` through `offlineGitCommand` | D8; phase 4 (Requirement 1, Task 4.1, files, risks, rollback; `go.mod` untouched); data flow |

Kept from the red-team session: `source_watch_confirm` is still widened, because attach, detach, and unwatch need it (it was never used for upstream updates).

### Whole-Plan Consistency Sweep (Validation Session 1)
- Files reread: plan.md and all 10 phase files.
- Decision deltas checked: 8.
- Terms searched across all plan files: `diff3` (only the git flag `--diff3` remains), `go-udiff`/`udiff`, `epiclabs`, `merge3`, `MetadataOnly`, `skill_upstream_update_preview`, `skill_upstream_update_confirm`, `pins` in MCP context, `commit count`/`commits behind`, `default pending user decision`, `Q3`/`Q4`/`Q5`, `go.mod`, `Mergeable`, `latest_committed_at`.
- Reconciled stale references: D6, D8, D10, D11, data flow, acceptance criteria, Verification Results, Open questions in plan.md; phases 2, 3, 4, 5, 6, 7, 8, 9, 10.
- Unresolved contradictions: 0.

## Plan amendment 2026-10-05

Trigger: the runtime plan shipped, and the user decided to close the WebUI plan and move all of its remaining work here ("move everything from A into B so A closes").

| Change | Where |
|---|---|
| Branch rebased onto `main` (two plan commits, no conflicts) | Executor notes |
| `SourceSummary` gains `Status`, `CurrentRevision`, `DistilledRevision`, `ReadyToDistill` (shared upstream-only rule) | Phase 5 Requirement 5 and its test |
| `SkillProvenance` TypeScript type corrected to the flat Go JSON; `ProvenanceCard` reads flat fields; test from the third-party golden | Phase 9 Requirements 1, 3; Task 9.2 |
| Upstream confirm invalidates `['skill-runtime', id]` | Phase 9 Requirement 2; Task 9.2 |
| New phase 10 from WebUI phase 4 (handoff, runs, run API, sentinel, route safety, seeds); the standalone Sources table, Watch screen, and `routes()` refactor are dropped (phase 9 owns `/sources`, D11 refuses skill-less watching, routes register per file) | phase-10 |
| New phase 11 from WebUI phase 5; fixtures now come from phase 10; Composer invalidates Runtime/trust and never edits metadata | phase-11 |
| New phase 12 from WebUI phase 6 tasks 6.1–6.4 and 6.6, adapted to shipped screens | phase-12 |
| Documentation renumbered 10 → 13 and absorbs WebUI phase 6 task 6.5 (WebUI shipped docs, `web-api.md`) plus plan close | phase-13 |
| WebUI guard retired; its hard rules move to Executor notes; code review after phases 8–12 | Executor notes |

Red-team of this amendment (2026-10-05, `reports/red-team-261005-1513-webui-absorption-review.md`): 3 High, 10 Medium, 4 Low, all accepted and applied. Highlights: the seed adds a never-distilled `source-c` so the journeys select a distillable source; `startServer({workspace})` serves the seeded workspace; the run test uses `..%2Fetc`; the handoff key is scoped to the selection; `SourceSummary` gets JSON tags and `upstream_only` with one shared Go helper; phase 8 defines `all` versus `due`; decision and cancel invalidations widened; mockup-parity checklists, screenshots, golden-only fixtures, `Check due sources`, and Open-a-run recording restored; the route test rejects non-literal patterns; the Task 11.1 test move is an explicit exception to the hard rules; phase 13 checks `serve` and `distill` help and drops `/sources/watch` from spec 04 §1 and §3.5.

Log entries above this section that say "phase 10" refer to the documentation phase, now phase 13.

User decisions (2026-10-05): retire the guard; the distill handoff opens only from the Sources screen; phases 10–13 run strictly after phase 9.

## Open questions

None. All 8 decision questions were answered on 2026-10-05 (see Validation Interview).
