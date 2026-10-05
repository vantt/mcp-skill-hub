---
title: "Skill execution, measurement funnel, and routing quality"
description: "Serve digest-pinned local skill snapshots with skill-level content trust and runtime setup support, measure the recommend-to-activate funnel server-side with long-lived rollups and transcript ground truth, show runtime state in the WebUI, and raise routing quality with examples, a generated eval gate, lint, and calibration."
status: in-progress
priority: P1
effort: 132h
branch: feat/skill-runtime-execution
tags: [feature, mcp, resolver, telemetry, security, web]
blockedBy: []
blocks: []
created: 2026-10-04
---

# Skill execution, measurement funnel, and routing quality

## Overview

Three user-accepted workstreams, executed as sequential phases (1–5, 5a, 5b are done and committed):

- **A. Skill execution (done).** Activation exports a read-only, digest-verified snapshot of a *trusted* skill into `runtime/cache/skills/<id>@<16hex>`, plus a writable state directory `runtime/envs/<id>@<deps16>` and a per-skill secret env file `runtime/config/<id>/env`. Third-party skills need a human content approval (CLI only) before agents get any content. The hub checks only the platform; `skillhub skill doctor` runs full checks in the user's terminal (`basis: terminal`). The hub never executes skill code in the MCP flow.
- **B. Measurement funnel (phases 6–8).** Server-side load events on `skill_get`, `skills/get`, and `resources/read`, attributed to the recommending resolution through a per-MCP-session tracker, with loads blocked by content review counted separately; 180-day daily rollups; `skillhub telemetry funnel` and a WebUI Usage tab; explicit Claude Code transcript import as ground truth.
- **C. Routing quality (phases 9–12).** `routing.examples` / `routing.counter_examples`, `technologies` / `topics` in ranking, a generated routing eval gated in `make check`, metadata lint (routing plus runtime hints), and evidence-based calibration.
- **WebUI runtime parity (phase 12a).** Skill Detail shows content trust, the exact CLI approve command (no web approval), runtime hints, the runtime block, the cached doctor state with its basis, and env key names.
- **Documentation (phase 13).**

## Executor notes (read before any phase)

- **Branch:** `feat/skill-runtime-execution`. Do not create other branches; do not push unless the user asks.
- **One phase at a time,** in table order. Read the whole phase file, then `plan.md` "Key decisions", before editing. Each phase file lists exactly which files it may touch; touching another file needs a one-line justification in the phase report.
- **Checks:**
  - Go: `make check` (= `go vet ./...`, `golangci-lint` new issues since `origin/main`, `go test -count=1 ./...`). Run focused `go test` commands from the phase first.
  - Race (phases touching MCP handlers): `go test -race -count=1 <pkg>`.
  - Web (phases 7, 12a): `make web-test` (typecheck, eslint, vitest) and `make web-build`. `make web-check` runs both after `npm ci`.
  - Web E2E (phase 12a): `make web-e2e` (builds UI and binary, installs Chromium, runs Playwright).
  - Web golden JSON: `go test ./internal/delivery/web/ -run <TestName> -update` rewrites `internal/delivery/web/testdata/golden/*.json`; rerun without `-update` to confirm. Frontend tests load these files through `web/src/test/golden.ts`.
- **Commits:** conventional commits (`feat(scope): …`, `fix(scope): …`, `test(scope): …`, `docs: …`). No AI references. No plan IDs, phase numbers, or decision labels (D1, R2, …) in code comments, test names, or commit messages; describe the behavior instead. One or more focused commits per phase.
- **Status updates when a phase is done:** run `ak plan phase close 261004-1547-skill-execution-measurement-routing <n>` for integer phases (it marks the phase file's checkboxes and syncs the plan store). For phase 12a (the CLI does not parse lettered phases) set `status: done` in its frontmatter by hand. Then set the phase's Status cell in the table below to `Done`, and add a one-paragraph implementation note at the end of the phase file.
- **Reports:** write `reports/<agent>-<YYMMDD-HHMM>-<slug>.md` in this plan directory for every phase (what changed, files, deviations, test commands and results, open questions).
- **Failure protocol (applies to every phase):** if a pass condition fails and you cannot fix the cause within the phase's file list, stop. Do not weaken, skip, or delete tests, do not add `t.Skip`, do not loosen thresholds, and do not edit golden files to match broken output. Write the report with the failing command, its output, and your diagnosis, set the phase status to `blocked` in the phase frontmatter, and report the blocker.

## Verified starting facts (re-checked 2026-10-04 against commit 996daa9)

- Agents activate through the `skill_get` tool (`internal/delivery/mcpserver/skill_tools.go:135`); `skills/get` and `resources/read` are the standards path (`internal/delivery/mcpserver/server.go:144-157`). None records load telemetry today.
- `skill_get` returns `skillGetResult{app.SkillDetail; Content; Local}` (`internal/delivery/mcpserver/types.go:184`). `local` comes from `adapter.localSkill` (`server.go:209`) which calls `SnapshotService.Ensure`. For a third-party skill whose content is not approved, `local.status` is `review_required` and `content` is omitted in any lifecycle state (`skill_tools.go:158-175`, `app.SkillService.ContentTrustFor` at `internal/app/skill_detail.go:39`). `resources/read` refuses every file of such a skill with `content_review_required` (`server.go:235-238`).
- Content trust: `skillruntime.ContentDigest` / `skillruntime.Evaluate` (`internal/skillruntime/trust.go`); manifest field `quality.content_reviewed_digest`; approval only through `skillhub skill edit <id> --approve-content <digest>`. `skill review` exposes `content_trust` (`app.ContentTrust`, `internal/app/skill_review.go:95`) with `changes_since_approval` (`app.ChangesSinceApproval`, `internal/app/skill_review_changes.go:30`) and `runtime_hints` (`skillruntime.Hints`, `internal/skillruntime/hints.go:21`, built by `skillruntime.AnalyzeHints` at `hints.go:64`).
- Setup annotation on resolver recommendations: `app.setupAnnotation` (`internal/app/skill_doctor.go:220`); states `review_required → unsupported_platform → cached doctor state → unknown`; doctor-derived states carry `basis: terminal`.
- Telemetry: rollups via `telemetry.Recorder.Rollups(ctx, from, to)` (`internal/telemetry/recorder.go:224`); keys from `rollupKeys` (`internal/telemetry/rollup.go`). A harmful utility report sent together with a negative outcome currently increments `feedback:negative` twice (one per stored event); phase 7 fixes it.
- Skill FTS ranking is `bm25(skill_fts,0.0,8.0,5.0,7.0)` over `(skill_id,name,aliases,description,triggers)` (`internal/resolver/sqlite_catalog.go:99`, `internal/catalog/schema.go:131`), so triggers rank at the default weight 1.0. Scoring is token overlap in `scoreSkill` (`internal/resolver/evidence.go:256`).
- Golden corpus `testdata/resolver/golden-v1.json` (42 skills, 150 cases) is used by `internal/resolver/resolver_test.go:767` and `internal/evaluation/evaluation_test.go:172`; calibration grid in `testdata/resolver/evaluation-policy-v1.json`.
- `system-skills/` contains only `curator` (skill ID `system-curator`), which the resolver excludes (`reservedSystemSkillID`, `sqlite_catalog.go:20`).
- Claude Code transcripts write one JSONL line per content block; dedupe by `tool_use.id`. Subagent transcripts live in `<project>/<session>/subagents/*.jsonl`.
- WebUI read API: `registerReadRoutes` (`internal/delivery/web/routes_read.go:19`) serves `/api/v1/skills/{id}` (`app.SkillDetail`) and `/api/v1/skills/{id}/review` (`app.SkillReviewResult`, already containing `content_trust` and `runtime_hints`). `web/src/api/types.ts` does not yet declare those fields.

## Key decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | **Correlation = per-MCP-session in-memory tracker** keyed by `*mcp.ServerSession` (go-sdk v1.8.0 `ServerRequest[P].Session`, `mcp/shared.go:624`). Remembers recent resolutions (2 h TTL, 32 per session, 256 sessions LRU). Each load is attributed at write time as `recommended`, `supporting`, `override`, `after_no_skill`, `after_needs_context`, or `unsolicited`. | URIs stay standard; hosts cannot add params to `ReadMcpResource`; write-time classification lets rollups report acceptance beyond the 14-day raw window. |
| D2 | **Local layout (done):** snapshot `runtime/cache/skills/<id>@<16 hex of manifest digest>/` (all files `0444`, marker `.skillhub-snapshot.json`, built in `.staging/` and renamed); writable state dir `runtime/envs/<id>@<deps16>/` with copies of dependency manifests; per-skill secret env `runtime/config/<id>/env` (`0600` in `0700`). `local.env` exports `SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR`, `SKILLHUB_CONFIG_DIR`. | Atomic and idempotent; never touches the project or Git tree; setup writes go to a separate writable directory that survives SKILL.md edits. |
| D3 | **Skill-level content trust (done):** third-party = origin `github`/`git` or `provenance.source_id`; it is trusted only when `quality.content_reviewed_digest` equals `skillruntime.ContentDigest` (all files except `skill.meta.yaml` plus the runtime block). Untrusted skills expose no content to agents. Approval is CLI-only (`skill edit --approve-content`), never MCP, never WebUI. Local-folder adds are trusted (user decision). The gate stops accidental execution by cooperative agents, not adversarial ones. | Agents and the WebUI cannot self-approve; the review is bound to exact content. |
| D4 | **No execution in the MCP flow; hub checks only the platform (done).** `skill doctor` probes bins, versions, env presence (process env or stored key) and runs `check` in the state dir; results are cached per (machine, skill, runtime fingerprint) and labelled `basis: terminal`. | Hub-side bin/env checks ran in the wrong environment. |
| D5 | **`setup` annotation is post-ranking** in `app.ResolverService` (done). Ranking never sees machine state. | Deterministic, replayable resolver. |
| D6 | **Rollups** in `telemetry.db` `telemetry_daily_rollups(day, skill_id, metric, count)`, written with the raw insert in one transaction only when the raw row is new (done). UTC days, 180-day retention. | Exactly-once per event ID; long-window funnel. |
| D7 | **Examples fold into existing features**: trigger feature = max(trigger overlap, 0.9 × example overlap); not-for feature = max(not_for overlap, counter-example overlap); new FTS columns `examples` and `keywords`. No new policy weight. | No policy schema change. |
| D8 | **Routing eval is leave-one-out.** | Otherwise the gate measures memorization. |
| D9 | **Named measurement cases are deferred.** Funnel supports `--since/--until` only. | Rollups are day-granular. |
| D10 | **Loads blocked by content review are counted, never as activations.** A `skill_get`/`skills/get`/`resources/read` of a `review_required` skill emits `skill.loaded` with `status: review_required`, `first_activation: false`; rollups count `blocked:review_required`; it does not set `after_load` on later feedback. | Shows demand for unapproved skills without inflating acceptance. |
| D11 | **WebUI runtime view is read-only.** It reuses `/review` for trust and hints and adds `GET /api/v1/skills/{id}/runtime` for the runtime block, the agent-facing setup annotation, the cached doctor result, and env key names. It never exports snapshots, creates directories, runs the doctor, or reads env values. | Parity without a second trust or execution path. |

## Phases

| # | Phase | Depends on | Effort | Status |
|---|---|---|---|---|
| 1 | [Manifest fields and routing-merge fix](./phase-01-manifest-fields-and-routing-merge.md) | — | 6h | Done |
| 2 | [Telemetry events and daily rollups](./phase-02-telemetry-events-and-daily-rollups.md) | — | 8h | Done |
| 3 | [skillruntime package: runtime, trust, doctor checks](./phase-03-skillruntime-package.md) | 1 | 8h | Done |
| 4 | [Local snapshot export and activation response](./phase-04-local-snapshot-and-activation-response.md) | 1, 3 | 10h | Done |
| 5 | [Doctor CLI, setup annotation, host instructions](./phase-05-doctor-setup-annotation-host-instructions.md) | 2, 3, 4 | 8h | Done |
| 5a | [Runtime revision: skill-level trust, state dir, platform-only checks, setup guidance](./phase-05a-runtime-revision.md) | 1–5 | 10h | Done |
| 5b | [Runtime hardening: host permissions, unapproved content, review diff, secret env](./phase-05b-runtime-hardening.md) | 5a | 8h | Done |
| 6 | [Server-side activation tracking](./phase-06-server-side-activation-tracking.md) | 2, 5b | 8h | Done |
| 7 | [Funnel aggregation, CLI, WebUI Usage tab](./phase-07-funnel-cli-and-web-usage.md) | 2, 6 | 12h | Pending |
| 8 | [Claude Code transcript import](./phase-08-transcript-import.md) | 2, 7 | 8h | Pending |
| 9 | [Resolver routing features](./phase-09-resolver-routing-features.md) | 1 | 8h | Pending |
| 10 | [Routing eval command, corpus, CI gate](./phase-10-routing-eval-and-gate.md) | 9 | 10h | Pending |
| 11 | [Metadata lint (routing and runtime hints)](./phase-11-metadata-lint.md) | 5b, 9 | 8h | Pending |
| 12 | [Calibration with recorded evidence](./phase-12-calibration.md) | 10 | 4h | Pending |
| 12a | [WebUI runtime parity](./phase-12a-webui-runtime-parity.md) | 5b (functional); after 7 for file ownership | 8h | Pending |
| 13 | [Documentation](./phase-13-documentation.md) | 1–12, 12a | 8h | Pending |

Phases run sequentially in the listed order. Shared files are owned by one phase at a time: `internal/delivery/mcpserver/server.go` (6), `internal/telemetry/{events.go,rollup.go,feedback.go}` and `schemas/telemetry-event-v1.schema.json` (6, then 7), `internal/delivery/cli/{telemetry.go,help.go}` (7, 8, 10), `internal/delivery/web/routes_read_test.go`, `web/src/api/{types.ts,queries.ts}`, `web/src/screens/skill-detail/SkillDetailScreen.tsx`, `web/src/i18n/en.ts` (7, then 12a), `internal/app/skill_review.go` (11).

## Data flow (end state)

```text
skill_resolve ──► ResolverService.Resolve ──► engine (ranking; examples/tech features)
      │                     └─► setupAnnotation (trust + platform + cached doctor, basis terminal)
      ├─► telemetry: resolution.* (+ setup_state)  ──► raw (14d) + rollups (180d)
      └─► session tracker: noteResolution
skill_get / skills/get / resources/read
      ├─► trusted: SnapshotService.Ensure ──► runtime/cache/skills/<id>@<d16>/ + runtime/envs/<id>@<deps16>/
      │      └─► response: local{path,state_directory,env,resources,preflight}
      ├─► review_required: no content, reads refused
      └─► tracker.attribute ──► skill.loaded(basis=server-observed, attribution | status=review_required)
skillhub skill doctor ──► checks + check cmd in state dir ──► doctor cache (basis terminal) + skill.doctor_checked
skillhub skill env set|unset|list ──► runtime/config/<id>/env (keys only ever shown)
skillhub telemetry import-transcripts ──► transcript.tool_observed (dedupe by tool_use.id)
skillhub telemetry funnel / GET /api/v1/skills/{id}/usage ──► rollups ⨝ active catalog
GET /api/v1/skills/{id}/review + /runtime ──► WebUI Skill Detail (trust, hints, runtime, doctor, env keys)
skillhub eval routing ──► cases from examples/counter_examples (+ no-skill file) ──► metrics, gate
skillhub validate / skill review ──► routing lint + runtime-hint lint warnings
```

## Backwards compatibility

- New manifest fields are optional. Older binaries reject unknown keys in `skill.meta.yaml`, so a workspace that adopts them needs a binary newer than `v0.2.0` (documented in phase 13).
- Phase 9 bumps catalog `DerivedSchemaVersion` 2 → 3; old generations report `StateIncompatible` and are rebuilt by `EnsureCatalog` at MCP startup or `skillhub rebuild`.
- `telemetry.db` changes are additive. Phases 6–7 add a `status` value on `skill.loaded` and new rollup metric names; nothing on `main` has written rollups yet (the feature branch is unreleased), so the `feedback:negative` fix needs no data migration.
- MCP and WebUI API output changes are additive (`schema_version` stays `"1"`). New WebUI endpoints: `/api/v1/skills/{id}/usage` (7), `/api/v1/skills/{id}/runtime` (12a).
- Third-party skills imported before approval exist lose agent access until approved (intended, D3).

## Rollback

Each phase is one or more focused commits; revert with `git revert`. Disposable state: `rm -rf runtime/cache/skills runtime/cache/doctor runtime/envs` (secret env files in `runtime/config` are user data; keep them). The rollup table is harmless to older binaries; reverting phase 9 brings back `DerivedSchemaVersion` 2 and the catalog rebuilds automatically.

## Acceptance criteria (whole plan)

- [x] `skill_get` / `skills/get` for an active, trusted skill return an absolute `local.path` inside `runtime/cache/skills/` matching manifest digests, plus `local.state_directory` and `local.env`; tampered snapshots are rebuilt.
- [x] An unapproved third-party skill returns `local.status: review_required` with no content and no paths; `resources/read` refuses every file with `content_review_required`; after `skill edit --approve-content <digest>` content and reads work.
- [x] `skillhub skill doctor <id>` reports platform, bins (with versions), env presence (process env or stored key), and `check`, exits 0/1/2, caches with `basis: terminal`; resolver recommendations carry `setup.state`.
- [x] Every entrypoint load emits one `skill.loaded` with `basis=server-observed` and a correct attribution; blocked loads count as `blocked:review_required`; `skillhub telemetry funnel --json` reports every metric in phase 7 for any window up to 180 days; `feedback:negative` counts once per report.
- [x] `skillhub telemetry import-transcripts --project <dir>` stores only tool, skill ID, timestamp, and session hash; a re-import adds nothing.
- [x] `skillhub eval routing` prints precision@1, recall, no-skill precision/recall, and false-positive rate, and exits non-zero below thresholds; the gate test runs in `make check`.
- [x] `skillhub validate` and `skillhub skill review` report routing lint warnings; `skillhub validate` also reports runtime-hint warnings (absolute install paths, missing runtime block, install prose without runtime block, missing lockfiles).
- [x] Any policy change is backed by a report in `reports/`; golden-v1 held-out gates still pass.
- [x] WebUI Skill Detail shows content trust (third-party, approved, digest, changes since approval with script/runtime/dependency flags) with the exact CLI approve command and no approve control, runtime hints, the runtime block, the cached doctor state with basis, and env key names only.
- [x] `make check`, `make web-check`, and `make web-e2e` pass; docs match shipped behavior.

## Deferred

- Named measurement cases (forgentX `case` windows): see D9.

## Open questions

1. `system-skills/` holds only the curator, which the resolver excludes, so it gets no routing examples. Phase 10 backfills examples into the golden-v1 corpus (overlay file) and the three `testdata/evaluation/workspace-overlay` skills. Examples for your own workspace's skills are content work outside this repository.

## Validation Log

### Verification Results

- **Tier:** Full (9 remaining phase files, 5+ phases).
- **Method:** for plan.md and each pending phase (6, 7, 8, 9, 10, 11, 12, 12a, 13), at least 15 claims (file paths, symbols, line citations, endpoints, commands) were sampled and checked by a scripted grep over a ±2-line window at the cited line, plus `go run ./cmd/skillhub help <cmd>` for command surfaces, against commit 996daa9 (script kept in the session scratchpad, not in the repo).
- **Claims checked:** 159 (plan.md 16, phases 6–12 and 13 15 each, phase 12a 20, plus 3 help-surface checks for `doctor`, `connect`, `validate`). **Verified:** 159 after correction. **Failed on first run:** 1 (`skill_get` registration cited at `skill_tools.go:133`; the `addTool` call is at `:135`; corrected in plan.md and phase 6). **Failed (uncorrectable):** 0. **Unverified:** 3 (tagged `[UNVERIFIED]` in place).
- **Stale citations from the previous revision, corrected while rewriting (verified by the same script):** listed below.
- **Corrections made (stale citation → current code):**
  - `excludingCatalog` is at `internal/app/resolver.go:173` (was `:150`).
  - `routing_evaluate` calls `app.ResolverService{}` at `internal/delivery/mcpserver/insight_tools.go:111` (was `:110`).
  - Help text: `telemetry` usage at `internal/delivery/cli/help.go:269`, `eval` usage at `:282` (were `:251`, `:264`).
  - MCP server key `skillhub` is written at `internal/hostintegration/integration.go:280,288` (was `:242`).
  - `DefaultPolicy` values at `internal/resolver/policy.go:32-38`, revision fingerprint at `:39-44` (were `:32-36`, `:37-43`).
  - `ActivationReadiness` is at `internal/app/skill_review.go:24-29` (was `:21-26`).
  - `ValidateWorkspace` lives in `internal/app/operations.go:19`, not `internal/app/workspace.go`.
  - `app.Warning` has only `code` and `summary` (`internal/app/result.go:64`; closed by `schemas/result-envelope.schema.json`), so lint fix text goes into `summary`; the old "code, message, fix" wording was wrong.
  - `reservedSystemSkillID` at `sqlite_catalog.go:20` (was `:21`); go-sdk `Session` field at `mcp/shared.go:624`.
  - Rule-channel trigger rank is at `internal/resolver/resolver.go:171`.
  - Resource kind classification is `catalog.resourceKind` (`internal/catalog/project.go:402`), returning `instructions|reference|script|asset|resource`; telemetry uses `entrypoint` for `instructions`.
  - Skill Detail tabs are at `web/src/screens/skill-detail/SkillDetailScreen.tsx:241-271`; strings in that screen are local constants, not `en.ts` keys (phases 7 and 12a follow the screen's existing pattern and add `en.ts` keys only where `useT` is already used).
- **Stale references removed:** `scripts_reviewed_digest`, `--approve-scripts`, withheld scripts, `.restricted` snapshots, `scripts_review_required`, `LiveCheck` bins/env in the MCP flow, executable detection (plan.md D2–D4, acceptance criteria, data flow, phases 6, 7, 13).
- **Unverified (tagged in place):** (1) phase 10 gate runtime under 10 s (measurable only after implementation); (2) phase 12 expected small effect of the FTS trigger weight (measured by the phase); (3) phase 13 per-host Windows handling of the env file (format documented, host behavior not tested). The forgentX transcript reference used by phase 8 was verified line by line (`encode_project_dir` `:117`, worktree prefix `:151`, valid cwds `:82`, mtime window `:202-207`).

### Whole-Plan Consistency Sweep

- **Status rows:** plan.md table now matches phase files: phases 1–5 Done (frontmatter `completed`/`done`), 5a and 5b frontmatter corrected from `pending` to `done` (they were done and committed in 317d084; the `ak plan` CLI cannot address lettered phases, so the frontmatter was edited directly), 6–13 and 12a `pending`.
- **Frontmatter:** `branch` corrected from `main` to `feat/skill-runtime-execution`; `status` `pending` → `in-progress`; `effort` 100h → 132h (sum of phases: 6+8+8+10+8+10+8 done = 58h; 8+12+8+8+10+8+4+8+8 pending = 74h).
- **Dependencies:** phase 6 now depends on 5b (it emits blocked loads for `review_required`); phase 11 depends on 5b (`AnalyzeHints`); phase 12a depends functionally on 5b only (it shows no usage data) and is sequenced after 7 because both edit `SkillDetailScreen.tsx`, `types.ts`, `queries.ts`, `routes_read_test.go`; phase 13 depends on 12a.
- **Terminology:** one vocabulary across files — "content trust", `content_reviewed_digest`, `--approve-content`, `review_required`, `content_review_required`, `basis: terminal`, `blocked:review_required`.
- **Done phases 1, 3, 4, 5** still describe the superseded per-file script trust; each now carries a one-line "Superseded by 5a" note at the top instead of a rewrite, since they are historical records.
- **File ownership:** no two phases edit the same file in parallel; shared files are listed under the phase table with their owning order.
- **Frontmatter of 5a/5b:** `effort` added (10h, 8h) to match the table.
- **Executability:** every pending phase now has checkbox steps with a mechanical pass condition, a file list, acceptance tests, risks, rollback, and a failure protocol; pending phases contain no stale runtime vocabulary (scripted scan for `scripts_reviewed|approve-scripts|.restricted|withheld|scripts_review_required|LiveCheck|IsExecutableResource` returned nothing outside superseded notes and this log).
- **`ak plan validate`:** exit 0, `[OK] plans/261004-1547-skill-execution-measurement-routing is a valid plan directory` (before and after the revision). `ak plan status` after the revision: `5/13 phases done, 36% complete (28/76 tasks)`. The CLI counts integer phases only; lettered phases (5a, 5b, 12a) are invisible to its parser, a tool limitation rather than a plan defect, so 12a status is tracked by hand (see Executor notes).

### Validation Interview (2026-10-04)
User accepted all recommended options; phase files already encode them:
1. Loads of review-required skills: `skill.loaded` with `status: review_required`, rolled up as `blocked:review_required`, never an activation (phase 6).
2. Curator `skill_get` calls: no contract change; counted as `unsolicited` and documented (phases 6, 13).
3. WebUI trust placement: content-trust card at top of Review tab + Runtime tab (phase 12a).
4. Phase 12a runs after phase 7, before docs.
5. `setup_failed_rate` denominator: all activations regardless of attribution (phase 7).
6. Runtime-hint lint warnings only in `validate`, not repeated in `skill review` (phase 11).
7. Phase 12a keeps the Runtime tab, reframed as a readiness checklist (requirement + doctor evidence + stored env + exact fix command per row). Rationale: the WebUI already has Add/Create/Editor flows; without runtime visibility a skill added or authored on the web dead-ends until an agent fails, and phase 7's `setup_failed` metric shows symptoms without causes. Cost is one read-only endpoint over existing functions.
