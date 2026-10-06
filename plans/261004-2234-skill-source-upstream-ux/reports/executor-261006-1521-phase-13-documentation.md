# Executor Report: Phase 13 — Documentation and Plan Close

**Phase:** Phase 13: Documentation and Plan Close  
**Status:** Complete  
**Plan Status:** Completed (13 of 13 phases complete, 100%)  
**Branch:** `feat/skill-source-upstream`  
**Timestamp:** 2026-10-06 15:21 +0700 (Asia/Saigon)  

---

## 1. What Changed in Phase 13

### Task 13.1 — Read Shipped Behavior
- Built `./cmd/skillhub` binary and verified shipped commands, flags, and help texts for `skill`, `source`, `check`, `serve`, and `distill`.
- Verified commands `skill outdated` and `skill update <id>` are present.

### Task 13.2 — User Documentation
- **`README.md`:** Updated "Curate your skills" to reference `skillhub skill outdated` and `skillhub skill update` instead of `source watch`.
- **`docs/user-guide.md`:**
  - Expanded "Concepts in plain words" with a concise 3-sentence definition of **Source** covering its two explicit roles (upstream source and learning reference) and the no-orphan invariant.
  - Added new commands to the Command Cheat Sheet: `skill outdated`, `skill upstream`, `skill update`, `source list`, `source attach`, `source detach`, `source unwatch`, `source backfill`, and updated `source watch` to require `--skill-id`.
- **`docs/curating-skills.md`:**
  - Replaced legacy "Watch and learn from upstream sources" with two distinct, comprehensive sections:
    - **"Keep vendored skills up to date":** Automatic source attachment on `skill add`; drift inspection via `skillhub skill outdated` (columns, statuses, `--check`, `--exit-code`); detail inspection via `skillhub skill upstream`; 3-way merge via `skillhub skill update` (file table, `--accept`, `--manual`, `--write-conflicts`, confirmation); explicit explanation that updates never approve content (re-approval step using `skill review` and `skill edit --approve-content`); backfilling legacy skills with `skillhub source backfill`; and binary compatibility notice for workspaces with `origin.files_digest`.
    - **"Learn from references":** Managing learning sources via `source attach` and `source detach`; watching repositories with `source watch --skill-id` under the no-orphan invariant; triaging candidates with `source triage` (`accept --skill-id|--new-skill`, `import`, `defer`, `reject`); unwatching with `source unwatch`; checking revisions via `source check` (`check`); and distilling changes to review inbox insights.
  - Updated "Advanced intake and recovery" example to include `--skill-id my-skill` on `source triage --decision accept`.
  - Updated bottom Command Map table with all new and updated commands.

### Task 13.3 — Design and Use-Case Documentation
- **`docs/design/06-source-learning-and-distillation.md`:**
  - Invariants: Added Invariant 13 (no-orphan invariant: every source must link to at least one skill) and Invariant 14 (agent safety: MCP only reads metadata via `skill_upstream_status`; no update preview/apply via MCP).
  - Conceptual model (§3.2): Detailed the two distinct roles and their canonical encoding: Upstream (`provenance.source_id` + `provenance.origin` with `files_digest`) and Learning references (`sources/skills/LINK-*.yaml` with `role: learning-source`). Documented one source per (repository, ref) (D2), `purpose: upstream`, and routine check behavior.
  - Revision check (§6.1): Documented that `CheckSources` refreshes per-skill upstream drift in SQLite `runtime/operational.db` table `skill_upstream_state` without creating canonical Git churn for upstream-only sources.
- **`docs/design/07-storage-and-mutation-model.md`:**
  - Layout (§4): Updated canonical link format to `sources/skills/LINK-<skill-id>--<source-id>.yaml`.
  - Operational DB (§14.1): Documented volatile `skill_upstream_state` table.
  - Canonical projection (§16): Documented `provenance.origin.files_digest` in `skill.meta.yaml` and learning links in `LINK-*.yaml`.
  - Application services (§21): Listed domain-specific mutation commands: `skill_upstream_update`, `source_backfill`, `source_attach`, `source_detach`, `source_unwatch`, and `insight_apply`.
- **`docs/use-cases/03-cli-and-curator-mcp-mapping.md`:**
  - Added UC-08: "Kiểm tra và cập nhật skill từ upstream" with CLI commands (`skill outdated`, `skill upstream`, `skill update`, `source backfill`) and MCP tool `skill_upstream_status` (read-only; D10 safety boundary).
  - Added UC-09: "Gắn nguồn học cho skill" with CLI commands (`source attach`, `source detach`, `source watch`, `source unwatch`, `source triage`) and MCP tools (`source_watch_preview`/`confirm`, `source_triage`).
  - Updated UC-07A/B and Section 3 Parity Matrix table to reflect `--skill-id` and new use cases.
- **`docs/use-cases/04-webui-user-flows-and-screen-specs.md`:**
  - §0 item 8: Documented delivery adapter `internal/delivery/web` and command `skillhub serve web`.
  - §1: Removed `/sources/watch` route and node from navigation diagram and route table.
  - §2.2 Skills Catalog: Documented Upstream status chips and filter options.
  - §2.5 Skill Detail: Documented Tab Runtime (system requirements, setup check, CLI approve codebox), Tab Usage (telemetry metrics), Tab Sources (Upstream section, 3-way update review modal, Learning section with distill toggle/detach), and real Provenance card.
  - §2.6 Sources: Documented grouped layout by repository and ref, role chips (`upstream`, `learning-source`, `orphan`), linked skills, distill checkboxes, and actions (`Check now`, `Import more`, `Unwatch`, `Distill with Curator Agent`).
  - §2.7: Clarified that source monitoring requires a skill target and no standalone Watch page exists.
  - §3.5: Updated flow title to "Check và Distill Source".
- **`docs/use-cases/05-webui-design-brief.md`:**
  - §4.6: Replaced ASCII sketch with grouped layout showing repository heading, role chips, linked skills, and per-source actions.
  - §4.7: Noted that watching requires a skill target and no standalone Watch route exists.

### Task 13.4 — WebUI Shipped Documentation
- **`docs/design/01-system-architecture.md`:**
  - Replaced `Web UI không thuộc V1.` with delivery adapter description (`internal/delivery/web`, `skillhub serve web`).
  - Added `WEB[Web adapter]` to Delivery subgraph in mermaid diagram.
  - Updated Delivery module table row with package path `internal/delivery/web`.
  - Updated Defaults to `- Web UI local qua \`skillhub serve web\`, chỉ chạy khi người dùng khởi động.`
  - Removed Web UI from out-of-scope list.
- **`docs/design/05-curation-lifecycle.md`:**
  - Removed Web UI from "Không bao gồm" (line 5) and documented Web UI as the visual management surface alongside Curator Skill and CLI.
- **`docs/PRD.md`:** Marked Simple Web UI in architecture diagram as delivered via `skillhub serve web`.
- **`docs/contracts/error-codes.md`:** Replaced `adds no web UI` with reference to `docs/contracts/web-api.md`.
- **`docs/contracts/web-api.md` (New):**
  - Published comprehensive contract for `internal/delivery/web` (`/api/v1`).
  - Details token-based authentication via URL fragment and Bearer header.
  - Details Host header validation (HTTP 421) and Origin/Referrer protection.
  - Details listen rules (automatic multi-IP binding to `0.0.0.0`, loopback default, `--loopback-only`, `--allow-host`).
  - Complete endpoint mapping table for all 34 `mux.HandleFunc` route patterns in code with application service methods.
  - Documents safety boundaries: no distill mutations via web, and no content approvals via web.
  - Status code mapping table linking to `internal/delivery/web/errors.go`.
  - Links to `schemas/` and `internal/delivery/web/testdata/golden/`.
- **`README.md`:** Added "Web UI" section explaining `skillhub serve web`, URL fragment token, multi-IP detection, `--loopback-only`, `--allow-host`, and plain-HTTP warning. Updated "Build from source" to specify `make web-build`.
- **`docs/user-guide.md`:** Added task-oriented "Local Web UI" section and troubleshooting note for Windows Defender Firewall prompts on wildcard binding.
- **`docs/release-runbook.md`:** Updated automated release pipeline to detail `web` job (`make web-build`, `npm run notices`), `web-dist` artifact, packaging of `THIRD_PARTY_NOTICES.md`, and Unix/Windows `serve web` smoke tests.

### Task 13.5 — Link and Claim Check
- Verified all 55 relative markdown links across the 14 touched documents resolve with zero broken links.
- Verified all 34 web route patterns in `internal/delivery/web/*.go` appear in `docs/contracts/web-api.md`.
- Verified all quoted CLI commands and flags match shipped help output.
- Passed full verification gates: `make web-check` and `make check`.

### Task 13.6 — Plan Close
- Marked all 13 phases `status: done` across all phase markdown files.
- Marked all 13 rows `Done` and plan frontmatter `status: completed` in `plans/261004-2234-skill-source-upstream-ux/plan.md`.
- Checked all 12 whole-plan acceptance criteria in `plan.md`.

---

## 2. Commits by Phase (Full Plan History)

| Phase | Title | Key Commits |
|---|---|---|
| **Phase 1** | Upstream provenance model | `f3234b2` (Task 1.0 start & overlap check)<br/>`fb34133` (Task 1.1 canonical files_digest)<br/>`73cdf16` (Task 1.2 RevisionAt, RemoteRefCommit, mirror lock)<br/>`18abb83` (Task 1.3 importedSkillFiles helper & skill add provenance)<br/>`94d1b9f` (Task 1.4 make check gate)<br/>`e053b17` (Phase 1 completion report) |
| **Phase 2** | Source attachment on add and import | `fec8fba` (Task 2.1 source matching & purpose field)<br/>`b7dcee4` (Task 2.2 attach upstream source on skill add)<br/>`1786ae3` (Task 2.3 write origin & drop links on import)<br/>`f270337` (Task 2.4 CLI render upstream in skill add)<br/>`590a786` (Task 2.5 make check gate)<br/>`034362d` (Phase 2 completion report) |
| **Phase 3** | Upstream check engine | `673e268` (Task 3.1 skill_upstream_state table & store methods)<br/>`42cd2cf` (Task 3.2 upstream status derivation & local digest helper)<br/>`f71c80f` (Task 3.3 CheckSources integration & read model)<br/>`b7993db` (Task 3.4 curation home & status integration)<br/>`dc73d6d` (Task 3.4 coalesce fix)<br/>`195f382` (Task 3.5 make check gate)<br/>`0b69b1b` (Phase 3 completion report) |
| **Phase 4** | Update merge and apply | `ec0e457` (Task 4.1 git-backed merge & diff helpers)<br/>`6550a2d` (Task 4.2 upstream_update proposal kind & kind guard)<br/>`08942cc` (Task 4.3 preview, write set planning, confirm)<br/>`17b4fe8` (Task 4.4 make check gate)<br/>`6429f13` (Phase 4 completion report) |
| **Phase 5** | Source lifecycle and backfill | `4703615` (Task 5.1 attach, detach, unwatch operations)<br/>`7f4eefa` (Task 5.2 require skill on watch & triage decisions)<br/>`d9173fb` (Task 5.3 grouped source list & import more)<br/>`baf40a7` (Task 5.4 source backfill for legacy skills)<br/>`2d2c9a2` (Task 5.4 upstream status next-actions)<br/>`f9630c5` (Task 5.5 lint & style refactors)<br/>`43d9ff6` (Phase 5 completion)<br/>`3cf302d` (Phase 5 completion report) |
| **Phase 6** | CLI: upstream and sources | `dda97fd` (Task 6.1 start)<br/>`d430c8a` (Task 6.2 skill outdated & skill upstream)<br/>`903deeb` (Task 6.3 skill update command & confirm handler)<br/>`3be4214` (Task 6.4 source attach, detach, unwatch, backfill, grouped list)<br/>`48082c6` (Task 6.5 help text & escaping tests)<br/>`760c561` (Task 6.5 lint cleanup)<br/>`fff29b6` (Phase 6 completion)<br/>`1a9fb7a` (Phase 6 completion report) |
| **Phase 7** | MCP tools and curator | `dd28376` (Task 7.1 start)<br/>`333fe14` (Task 7.2 skill_upstream_status tool & registration)<br/>`2cb92bf` (Task 7.3 source tools & watch confirm)<br/>`6202aaa` (Task 7.4 tool annotations table)<br/>`cfbbebc` (Task 7.5 curator guidance v2)<br/>`71ebf73` (Phase 7 completion)<br/>`9e312c6` (Phase 7 completion report) |
| **Phase 8** | Web API | `8a4ec2a` (Task 8.1 start)<br/>`2676891` (Task 8.2 SkillSources read model & ListSkillsWithUpstream)<br/>`61c83b5` (Task 8.3 sources & upstream update API routes)<br/>`ef03444` (Task 8.4 golden test cases)<br/>`c118526` (Task 8.5 refactors & countPendingInsights)<br/>`7028356` (Phase 8 completion)<br/>`cf7d267` (Phase 8 completion report) |
| **Phase 9** | Web UI | `9ca1d2b` (Task 9.1 API types & query hooks)<br/>`f3354b4` (Task 9.2 Sources tab, upstream review, provenance card)<br/>`6d6c0dd` (Task 9.3 upstream filter, chips, home CTAs)<br/>`e0005db` (Task 9.4 Sources screen & route mapping)<br/>`300292f` (Task 9.5 smoke test suite updates)<br/>`de20433` (Task 9.5 refactors & constants)<br/>`56a13ff` (Phase 9 completion)<br/>`b79e170` (Phase 9 completion report) |
| **Phase 10** | Distill handoff and runs | `02aa74d` (Task 10.1 start)<br/>`27953a8` (Task 10.2 ErrDistillRunNotFound sentinel)<br/>`33469ce` (Task 10.3 run inspection & cancel routes with safety test)<br/>`63bda81` (Task 10.4 distill labels, handoff briefs, recent runs)<br/>`c7ab9f5` (Task 10.5 Distill handoff & Run return screens)<br/>`f7d29df` (Task 10.6 distill workspace seed recipe & e2e journey)<br/>`3d25b9b` (Phase 10 completion & report) |
| **Phase 11** | Inbox, Insight, Patch Composer | `971a682` (Task 11.1 shared integrity-checked paging package)<br/>`75294d8` (Task 11.2 ErrInsightNotFound sentinel)<br/>`f32cb46` (Task 11.3 inbox paging & insight lifecycle endpoints)<br/>`c3c0c51` (Task 11.4 evidence set, insight staleness, composer draft)<br/>`c37d2b1` (Task 11.5 Inbox, Insight detail, Patch Composer screens)<br/>`ed81a8f` (Task 11.6 insight & patch composer e2e journey test)<br/>`97a664d` (Phase 11 completion & report) |
| **Phase 12** | WebUI hardening and release | `7cc5bfe` (Task 12.1 end-to-end journey tests for shipped flows)<br/>`fb5a8c4` (Task 12.2 accessibility and responsive sweep test suite)<br/>`fc3416f` (Task 12.3 Vietnamese glyph coverage & contrast patches)<br/>`81da4f7` (Task 12.4 third-party notices generator & bundle)<br/>`29cbf4b` (Task 12.5 web UI release smoke testing in release workflow)<br/>`9904ba4` (Task 12.6 notices generator lint fix)<br/>`6750b1f` (Phase 12 completion & report) |
| **Phase 13** | Documentation and plan close | `a5de828` (Task 13.1 start & shipped command verification)<br/>`ac79c4d` (Task 13.2 user guide, curation guide, readme updates)<br/>`e0d8652` (Task 13.3 design and use case specifications)<br/>`ba442a2` (Task 13.4 Web UI delivery adapter & web api contract)<br/>*(This commit)* (Tasks 13.5 & 13.6 links check, plan close, final report) |

---

## 3. Acceptance Criteria Verification Matrix

| # | Whole-Plan Acceptance Criterion | Status | Evidence / Verification Source |
|---|---|---|---|
| 1 | `skillhub skill add ... --all --yes` creates source, sets `provenance.source_id`, `origin.path`, and `origin.files_digest`; second add reuses source | **PASS** | `TestSkillAddAttachesSource`, `TestSkillAddReusesSource` in `internal/app/skill_add_test.go` |
| 2 | `skillhub skill outdated --check` lists tracked skills with status, commit, date, changed files count, local state; `--exit-code` exits 1 on attention; `--json` emits stable enum | **PASS** | `TestSkillOutdatedCommand`, `TestSkillOutdatedExitCode` in `internal/delivery/cli/skill_outdated_test.go` |
| 3 | `skillhub skill update <id>` previews per-file changes; clean update applies; conflicting update refuses pins until resolved; third-party reports `review_required` | **PASS** | `TestSkillUpdateCommand`, `TestConfirmUpstreamUpdateKindGuard` in `internal/delivery/cli/skill_update_test.go` |
| 4 | Local edits are never overwritten silently: files changed both locally and upstream are merged or require explicit choice | **PASS** | `TestUpstreamMergeThreeWay`, `TestUpstreamUpdatePlanning` in `internal/app/upstream_merge_test.go` |
| 5 | `skillhub source list` groups by repository; `skillhub source backfill` attaches existing skills; no write path creates zero-linked source | **PASS** | `TestGroupedSourceList`, `TestSourceBackfill`, `TestSourceWatchRequiresSkill` in `internal/delivery/cli/source_test.go` |
| 6 | MCP reports upstream status but cannot preview/apply update (no such tool, generic confirm refuses `upstream_update`); responses omit file diffs; curator copies stay identical | **PASS** | `TestSkillUpstreamStatusTool`, `TestGenericConfirmRejectsUpstreamUpdate` in `internal/delivery/mcpserver/upstream_tools_test.go` |
| 7 | WebUI: Skills list badge and filter, Skill Detail Sources tab (Upstream/Learning sections, update indicator), `/sources` grouped view with loading, empty, and error states | **PASS** | `web/e2e/smoke.spec.ts`, `web/e2e/journeys.spec.ts` (flows 3.1, 3.4, 3.5) |
| 8 | WebUI provenance renders flat Go fields; upstream update or Composer apply refreshes Runtime tab and trust card | **PASS** | `web/e2e/journeys.spec.ts` (flow 3.1, 3.6), `web/e2e/insight-apply.spec.ts` |
| 9 | WebUI distill loop: select learning sources on `/sources`, copy handoff brief, open returned runs, cancel active run; no web route mutates runs | **PASS** | `TestRoutesNeverMutateRuns` in `internal/delivery/web/routes_runs_test.go`, `web/e2e/sources-runs.spec.ts` |
| 10 | WebUI improvement loop: paged Inbox, Insight decisions, Patch Composer with full evidence mapping and conflict detection; MCP paging unchanged | **PASS** | `web/e2e/insight-apply.spec.ts`, `internal/delivery/paging/paging_test.go` |
| 11 | No route left on `LaterPhasePage`; every route passes axe and 360px sweep; release smoke proves authenticated UI; notices ship with release | **PASS** | `web/e2e/a11y.spec.ts` (14 routes, 0 serious/critical violations), `.github/workflows/release.yml` smoke tests, `THIRD_PARTY_NOTICES.md` |
| 12 | `make check`, `make web-check`, and `make web-e2e` pass; docs, including `docs/contracts/web-api.md`, match shipped behavior | **PASS** | All gates exit 0; 55/55 relative links resolve; 34/34 web routes documented |

---

## 4. Final Quality Gates Output

- **`make check`:** Exits `0`
  - `go vet ./...`: 0 issues
  - `golangci-lint`: 0 issues
  - `go test -count=1 ./...`: All 25 packages passed (`app`, `canonical`, `catalog`, `cli`, `termui`, `mcpserver`, `paging`, `web`, `distill`, `evaluation`, `hostintegration`, `insight`, `migration`, `mutation`, `resolver`, `selfupdate`, `skill`, `skillruntime`, `source`, `systemskills`, `telemetry`, `transcripts`, `workspace`, `schemas`).
- **`make web-check`:** Exits `0`
  - TypeScript: `tsc --noEmit` passed cleanly
  - ESLint: `eslint .` passed with 0 errors/warnings
  - Vitest: 25 test files passed, 109/109 tests passed
  - Production build: `vite build` succeeded with assets bundled
- **`make web-e2e`:** Exits `0`
  - Playwright: 19/19 tests passed across all journeys, accessibility sweeps, and responsive viewports.

---

## 5. Deviations & Rationale

None. All Phase 13 requirements and tasks executed strictly according to plan specifications.

---

## 6. Open Questions & Next Steps

None. The implementation plan `plans/261004-2234-skill-source-upstream-ux` is 100% complete.
