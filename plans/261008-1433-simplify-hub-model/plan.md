---
title: "Simplify the hub data model"
description: "Make Git the database, history, audit log and integrity mechanism of the hub; keep in the binary only what Git cannot give (locks, preview/confirm pins, human content approval). Move each skill's hub-side data into skills/<collection>/<id>/.meta/, reduce distillation to lessons + repo@commit pointers + cursor + decisions, and make a fresh clone of the hub work."
status: proposed
priority: P1
effort: TBD
branch: TBD
tags: [architecture, storage, distill, portability]
blockedBy: []
blocks: [docs/plans/2026-10-08-observer-and-enrichment.md (observer Phase 3 needs Phase 3 here; observer O7 baseline needs Phase 4 here)]
created: 2026-10-08
---

# Simplify the hub data model

## Overview

Three independent reviews of the hub data model (skills and mutation; sources,
distill and insights; runtime and distribution) found the same pattern: the
binary re-implements what Git and the upstream repositories already guarantee.
The visible cost is portability: a fresh `git clone` of a hub breaks on a new
machine. The hidden cost is about 21 files per learned idea and a large share of
`internal/distill`, `internal/insight`, `internal/mutation` and the runtime
catalog code.

Guiding principle (to be recorded in `docs/design/01-system-architecture.md`):
Git is the database, history, audit log and integrity mechanism. For a git
source, `repo@commit:path` is complete evidence. The binary adds only what Git
lacks: a workspace lock, preview/confirm pins against concurrent edits, and the
human content approval of third-party content.

## Verified starting facts (2026-10-08)

- A fresh clone lacks the empty layout directories that `init` creates; `skillhub
  status` failed with `statat sources/intake: no such file or directory` until
  they were created by hand. `internal/workspace/workspace.go:28-32` lists 12
  required directories.
- Workspace validation fails when any distill run exists but the gitignored
  package store is missing (`internal/distill/workspace_validation.go:44-49`);
  no code rebuilds packages from source plus revision.
- `ContentDigest` covers every file in a skill folder except the top-level
  `skill.meta.yaml` (`internal/skillruntime/trust.go:39-53`), plus the runtime spec
  (`trust.go:66-74`). Writing hub-side files into a skill folder today would
  invalidate its content approval; nested `.meta/*` files are hashed and distributed today.
- The live hub (`vantt/skill-hub`) already stopped tracking host-integration files
  (commit `a45b274`); a `git pull` of that commit on another clone deletes them there.
  The untracked `.mcp.json` still points at another machine's path. Local curator
  copies are v1.3.0 while the embedded curator is v1.5.1.
  <!-- Updated: Red Team 2026-10-08 - stale facts corrected -->
- Operation receipts embed before/after file bodies, capped at 64 KiB per change and
  512 KiB total (`internal/mutation/receipt.go:68-95`); one live receipt is 212 KB,
  contrary to `docs/design/07` §7. The bodies feed `GetOperationDiff`
  (`internal/app/insight.go:714-747`).
- `skill.meta.yaml` duplicates `SKILL.md` frontmatter (`name`, `description`),
  the directory name (`collection`) and `git log` (`history[]`, timestamps).
  `provenance.origin` is a source with a cursor, overlapping learning sources.
  It also carries trust inputs (`provenance`, read by `IsThirdParty`,
  `trust.go:28-30`), the runtime spec (setup/check commands, in the digest) and
  resolver fields (`aliases`, `topics`, `technologies`; `internal/resolver/evidence.go:247,270-293`).
- `skill.meta.yaml` is special-cased by exact name at ~20 production sites (see Phase 2).
- The binary never runs `git commit`; doc 07 §17.3 says V1 does not auto-commit.
- The curator's compatible-tools list omits registered tools its body uses
  (`skill_add_preview`, `skill_add_confirm`, `skill_review`, `source_watch_confirm`).

## Agreed decisions

| ID | Decision |
|---|---|
| D1 | Distillation is anchored to a skill: "what is worth learning" is judged against the target skill's current content. |
| D2 | Evidence for a git source is `repo@commit:path[#Lx-Ly]` with a **full 40-hex SHA** and a source that resolves to a repo URL; validation rejects changing a source's `repo` while lessons reference it. Only non-git documents need a digest and, when upstream keeps no history, a snapshot. Rebuild offline or with an unreachable commit reports "evidence unavailable" (warning), never a failure. <!-- Updated: Red Team 2026-10-08 - S10 --> |
| D3 | Every source belongs to at least one skill. A source saved for later is attached to an existing skill or to a new draft skill (draft skills never take part in routing or routing evals). Sources shared by several skills are deduplicated through the shared git mirror keyed by URL. `sources/intake/` and top-level `sources/` go away. <!-- Updated: Validation 2026-10-08 - D3 kept; S14 --> |
| D4 | Hub-side data of a skill lives in `skills/<collection>/<id>/.meta/`. One predicate `IsHubMeta(path)` (top-level `skill.meta.yaml` or first segment `.meta`) is used at every site: excluded from content digest, distribution (`resources/read`, snapshot copies), resource indexing, servable sizes, upstream comparison, and **stripped with an `upstream_meta_ignored` warning on every inbound path** (import, add, upstream update) so an upstream cannot ship its own `.meta/`. `.meta/distill.yaml` is also excluded from the catalog snapshot; `.meta/skill.yaml` (routing) still changes it. <!-- Updated: Red Team 2026-10-08 - S1, S3, S5 --> |
| D5 | `skill.meta.yaml` moves to `.meta/skill.yaml`. `name`/`description` come from `SKILL.md`; collection from the directory; history from Git. A field-by-field destination table (kept / moved under `routing` / derived / deleted) is part of the Phase 4 file; deletions need routing-eval evidence. Third-party status is derived from "has a source with role `upstream`" (the `learning` role never makes a skill third-party); `runtime` stays bound into the content digest. <!-- Updated: Red Team 2026-10-08 - S2, S13 --> |
| D6 | Upstream origin and learning references are one `sources` list in `.meta/skill.yaml`, each with roles `upstream` and/or `learning`. An `upstream` source keeps `kind`, `files_digest` and `transformations` for drift detection. The upstream cursor (`synced`) lives with the source in `skill.yaml`; distill cursors live with the lessons in `distill.yaml`. Until Phase 4 lands, `skill.meta.yaml` is authoritative and Phase 0 must not write `synced`. <!-- Updated: Red Team 2026-10-08 - S6 --> |
| D7 | Distillation is one file, `.meta/distill.yaml`: `goal`, `cursors`, `coverage` and `lessons`. A lesson has `key`, `what`, `notable`, `where` (one entry per source; several entries = convergence) and an inline `decision` (`candidate`, `planned`, `ported`, `rejected` with `reason`, `at`, and the `where` entries it saw — a new `where` entry reopens it). Usage-derived lessons (observer plan) use `where: usage:<case_id>` while candidate and point at the promoted Git eval case once decided. No separate decisions, comparisons, reports or snapshots; snapshots are added only when a non-git document source exists. `contrast` and R/E/F scores stay experimental until phase 0's scorecard decides. <!-- Updated: Red Team 2026-10-08 - S9, S15, cross-plan --> |
| D8 | Commits stay human: the binary approves locally and the user commits (doc 07 §3, §17.3). Receipts are kept but store digests only (id, idempotency key, request digest, per-path before/after digests); the diff for undo comes from Git when committed. (Former open decision 1.) <!-- Updated: Validation 2026-10-08 - S4 --> |
| D9 | Ported text from a learning source into skill content goes through a normal `skill_update` preview showing the source excerpt. <!-- Updated: Red Team 2026-10-08 - S13 --> |
| D10 | Never remove telemetry event types from `eventPayloads` or change `EventVersion`; stored events must stay readable (`internal/telemetry/events.go:206`). Keep a stable `catalog_snapshot` identity and a build-only temporary catalog (for observer replay) through any runtime trim. <!-- Updated: Red Team 2026-10-08 - B2, cross-plan --> |

Target layout:

```text
skills/<collection>/<id>/
├─ SKILL.md  references/  scripts/ …       content: distributed, approved, compared with upstream
└─ .meta/                                   hub-side: never distributed, never in the content digest
   ├─ skill.yaml                            id, status, routing, review, sources (roles, synced, upstream digests)
   └─ distill.yaml                          goal + cursors + coverage + lessons with inline decisions
runtime/                                    gitignored cache: catalog.db, shared git mirrors by URL, envs, secrets, locks
```

## Open decisions

1. ~~One operation = one Git commit versus "approved locally before commit".~~ Decided: D8.
2. Cross-skill comparisons: keep per skill only, or add a hub-level location.
3. Runtime trims (single `catalog.db` without generations and pins; serve skills
   from the checkout instead of `runtime/cache/skills` copies; cheaper staleness
   check; plain telemetry store). Each is independent and can be scheduled later,
   **but must keep D10** and is blocked by the observer plan's Phase 1 baseline and
   Phase 3 replay. <!-- Updated: Red Team 2026-10-08 - cross-plan -->

## Phases

| # | Phase | Depends on | Status |
|---|---|---|---|
| 0 | Discovery experiment: project-local `.claude/skills/distill-lab`, target `test-audit`, sources `openclaw/openclaw` and `obra/superpowers`; writes directly to `skills/default/test-audit/.meta/{skill.yaml,distill.yaml}` in the hub. `test-audit` content stays unapproved until phase 2 lands (it has never been approved). Phase 0 data is disposable until Phase 2 lands. The scorecard must state a keep/drop threshold per lesson field. | none | in progress |
| 1 | Hub portability: create layout directories lazily (read paths tolerate missing directories; do not add lazy creation for `sources/intake` and other directories later phases delete); `status`/`doctor` report "host integration missing" as fixable, with a note to re-run `skillhub integrate` after a pull that untracked those files; add the missing curator tools now and write the server-boundary test once against the final tool set in Phase 3 | none | done (merged d115fc7) |
| 2 | `IsHubMeta` predicate at every site (D4), inbound `.meta/` stripping, `.meta/distill.yaml` out of the catalog snapshot; bump the canonical schema version (`workspace.go:19`) so older binaries refuse rather than misjudge trust; record a minimum binary version | none | done (merged d115fc7) |
| 4 | `skill.meta.yaml` → `.meta/skill.yaml` (D5, D6) with a workspace migration through the WAL and a schema bump; field destination table; trust verdict and upstream status unchanged for every existing skill (test); approval-history walk follows both paths (`internal/app/skill_review_changes.go:24,73-90`) and leaves `.meta/` out of the blob sets | 2 | done (merged d6af65f; live hub migrated 2026-10-10, ebbb739) |
| 3 | Minimal distill model (D1-D3, D7): lessons, decisions, sources, cursor advanced in the same write as lessons; remove revision packages, run state machine, insight/incorporation entities and LINK files; keep the preview/confirm pin (`mutation.Proposal`, runtime pin store) and name exact types/paths kept and removed; migrate existing distill data (runs, insights, rejections, tombstones) into `distill.yaml` with a pre-Phase-3 fixture; contract table of every MCP tool, web route, JSON schema and telemetry event kept / renamed / removed / deprecated (AGENTS.md: preserve public contracts); keep event types (D10) | 4, findings of 0 | done (merged d6af65f, cleanup 81bbf1e; distill.yaml format = distill-lab) |
| 5 | Receipts store digests only (D8); `workspace_diff` uses Git for committed changes | 3 | done (9b6c986; schema v5, live hub not migrated yet) |

Phases run in the order 0 → 1 → 2 → 4 → 3 → 5. Phase files are written before each phase starts.
<!-- Updated: Red Team 2026-10-08 - S6 (4 before 3), S7, S8, S11, S12, S15; Validation - D8 -->

## Parallel execution with the observer plan (separate worktrees)

Each unit of work is done by **one agent in its own git worktree** (`isolation: worktree`).

| Wave | Worktree | Work |
|---|---|---|
| 1 | A | Observer Phase 1 telemetry (O1, O2, O5, O6) |
| 1 | B | **This plan Phase 1 → Phase 2.** Merge B first; A rebases on B |
| 2 | C | **This plan Phase 4 → Phase 3**, sequential in one worktree |
| 2 | D | Observer O3/O4 remainder → observer Phase 2 (must not touch `mcpserver/server.go` `registerTools`, `insight_tools.go`, `distribution.go`) |
| 3 | sequential | After C merges: observer §5.1 (re-measure tools/list) → observer baseline → observer Phase 3 → Phase 4; this plan Phase 5 |

Rules for every worktree agent:

1. Edit only the paths your phase owns. Shared files (curator `SKILL.md`, hostintegration
   templates, `mcpserver/server.go` `registerTools`, `telemetry/events.go`, `docs/design/04/06/07`)
   have one owner per wave; anyone else files a follow-up.
2. Never remove telemetry event types or change `EventVersion`.
3. Rebase on `main` right before merging; `make check` green after the rebase.
4. Merge one worktree at a time, in wave order. A schema or public-contract change lands in the
   same commit as its code.
5. Never commit in the live hub `/home/vantt/skill-hub` from a code worktree.

## Acceptance criteria

- `git clone` of a hub plus `skillhub rebuild` gives a valid, healthy workspace
  on a new machine, including a hub with distilled lessons (unreachable evidence is a warning).
- Writing or updating `.meta/` never changes a skill's content digest or review
  state; `.meta/` is never served to agents as a file (only `routing` is served, as data);
  writing `.meta/distill.yaml` never changes `CatalogSnapshot`.
- An upstream that ships `.meta/` cannot approve itself or change trust (test).
- Trust verdict and upstream status are unchanged for every existing skill after migration (test).
- One learned idea touches one hub file (`.meta/distill.yaml`) plus the skill
  edit when applied; receipts store digests only and the user's Git commit records the change.
- Human approval, reopen-only-on-new-evidence, tombstones and coverage gaps are
  preserved, including across the migration of existing distill data.
- Stored telemetry events stay readable.
- Phase 0 produces a scorecard with a keep/drop threshold per field that decides which
  lesson fields (`notable`, `contrast`, R/E/F) become schema.

## Red Team Review

### Session — 2026-10-08
**Reviewers:** Security Adversary (Fact Checker), Failure Mode Analyst (Flow Tracer), Assumption Destroyer (Scope Auditor), Scope & Complexity Critic (Contract Verifier); second adjudication by an Opus agent.
**Findings:** 15 + 1 cross-plan (16 accepted, 0 rejected; 6 accepted with smaller fixes per Opus)
**Severity breakdown:** 5 Critical, 9 High, 2 Medium

| # | Finding | Severity | Disposition | Applied To |
|---|---|---|---|---|
| X | Phase 3 removes the lifecycle the observer plan builds on; generations needed for replay | Critical | Accept — user chose "observer follows the simplified model" | D7, D10, Open decision 3, blocks |
| S1 | Upstream can ship `.meta/` and self-approve (import/update filter only `skill.meta.yaml`) | Critical | Accept | D4, Phase 2, criteria |
| S2 | D5 drops `provenance` (trust), `runtime` (setup commands), resolver fields | Critical | Accept (modified: derive third-party from upstream role, no sticky flag) | D5, D6, Phase 4 |
| S3 | `.meta/` under `skills/` changes `CatalogSnapshot` on every distill write | Critical | Accept (modified: exclude only `distill.yaml`) | D4, Phase 2, criteria |
| S4 | "The Git commit is the receipt" — binary never commits; receipts are idempotency + undo | Critical | Accept; open decision 1 decided | D8, Phase 5, criteria |
| S5 | ~20 sites special-case `skill.meta.yaml`; Phase 2 named 4 | High | Accept (modified: one predicate; no lab lock/review block) | D4, Phase 2 |
| S6 | Phase 3 needs Phase 4's file; cursors drift | High | Accept | D6, phase order |
| S7 | No canonical schema bump; binary skew | High | Accept | Phase 2, Phase 4 |
| S8 | Rename breaks approval-history walk | High | Accept | Phase 4 |
| S9 | D7 cannot keep reopen-on-new-evidence | High | Accept (modified: decision records seen `where` entries, no content digest) | D7 |
| S10 | Short SHAs, editable alias, unreachable commits | High | Accept | D2, criteria |
| S11 | No contract inventory (~20 tools, 12 routes, 8 schemas) | High | Accept | Phase 3 |
| S12 | Untracking commit deletes host files on other clones | High | Accept (modified: doctor finding + note, no hook) | Phase 1 |
| S13 | Ported learning lessons bypass trust review | High | Accept (modified: rule + normal preview, no new gate) | D5, D9 |
| S14 | D3 draft skills cost more; shared sources duplicated | Medium | Question → user kept D3 | D3 |
| S15 | Phase 0 already beyond D7; no scorecard thresholds; no data migration; Phase 1 invests in deleted dirs | Medium | Accept | D7, Phase 0, 1, 3 |
| B2 | Removing event types repeats the version-bump failure | — | Accept (Opus) | D10, Phase 3 |
| B3 | Phase 4 changes every snapshot, invalidating the observer baseline | — | Accept (Opus) | blocks, parallel schedule |

### Whole-Plan Consistency Sweep
- Files reread: plan.md (this plan has no phase files yet), observer plan §9
- Decision deltas checked: 12 (D2-D10, open decision 1 and 3, phase order)
- Reconciled stale references: "Git commit is the receipt", "four surfaces", "Phase 3 depends on 2", host-integration facts, curator version, "`proposal/incorporation/outcome entities`" (now: insight/incorporation removed, proposal pin kept)
- Unresolved contradictions: 0

## Validation Log

### Session 1 — 2026-10-08
**Trigger:** `/ak:plan validate` + `red-team` requested by the user; verification by the four red-team reviewers.
**Questions asked:** 3

#### Verification Results
- Tier: Full (6 phases)
- Claims checked: ~25 | Verified: ~18 | Failed: 4 | Unverified: 2
- Failures:
  1. "Git commit is the receipt" — the binary never commits; doc 07 §17.3. Fixed (D8).
  2. Phase 2 "four surfaces" — ~20 sites; native install copies only the curator. Fixed (D4).
  3. Host-integration files committed — already untracked by `a45b274`. Fixed.
  4. Curator at v1.5.0 / `source_watch_preview` referenced — embedded curator is v1.5.1; `source_watch_preview` not referenced. Fixed.

#### Questions & Answers
1. **[Architecture]** Resolve the cross-plan conflict (Phase 3 removes the lifecycle observer D7 builds on)?
   - Options: Observer follows the simplified model | Keep the insight lifecycle | Defer
   - **Answer:** Observer follows the simplified model
2. **[Tradeoffs]** Apply which version of the red-team fixes?
   - Options: Opus version | Original reviewer version | Opus version except D4
   - **Answer:** Opus version (includes deciding open decision 1 as D8)
3. **[Scope]** Keep or change D3?
   - Options: Keep D3 | Add a hub-level list
   - **Answer:** Keep D3

#### Confirmed Decisions
- Cross-plan: usage lessons live in `.meta/distill.yaml`; preview/confirm pin kept.
- D8: human commits; digest-only receipts.
- D3 kept, with mirror dedup and drafts out of routing.

#### Impact on Phases
- Phase order is now 0 → 1 → 2 → 4 → 3 → 5.
- Phase 2 grows (predicate at every site, schema bump); Phase 3 gains a contract table and data migration.
