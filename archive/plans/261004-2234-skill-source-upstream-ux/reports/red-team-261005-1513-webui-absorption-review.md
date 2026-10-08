# Red-team review: WebUI plan absorption (phases 10–13)

Date: 2026-10-05. Scope: the uncommitted amendment on `feat/skill-source-upstream` (`git diff HEAD`), plus new phases 10–12, checked against `internal/`, `web/`, `Makefile`, and `.github/workflows/release.yml`. This is a report only: no plan or code file was edited.

## Findings

### High

**H1. The seeded journey selects a source that cannot be selected.**
- Evidence: `phase-10-distill-handoff-and-runs.md:47` finalizes `source-a`. On finalize, `internal/app/distill.go:864-865` sets `DistilledRevision = ToRevision` and `Status = "watching"`. A later check of unchanged content keeps it `up_to_date` (`internal/app/source.go:521-525`). Under `phase-05:32`, `ReadyToDistill` is therefore false, so `distillLabel` makes the row non-selectable (`phase-10:36`).
- Two steps fail as a result:
  - `phase-10:78`: "`/sources?filter=ready` lists `source-a` with a checkbox → select".
  - `phase-12:23` (§3.5 journey): "check a learning source, select it".
- Only `source-b` (checked, never distilled, status `changed`) is selectable.
- Fix: either select `source-b` in both journeys, or add a third learning source `source-c` to the seed (checked, not prepared). The second option avoids handing off a source that already has an in-progress run. Assert the label `Never distilled` / `Changed`.

**H2. Playwright cannot start the server on the seeded workspace, and the phase file lists forbid the fix.**
- Evidence:
  - `web/e2e/support/server.ts:16-25`: `startServer()` takes no argument. It always runs `mkdtemp` and `init` on a new workspace, creates `smoke-skill`, and `stop()` deletes that workspace with `rm -rf`.
  - `phase-10:43-49`: `seedDistillWorkspace()` runs its own `init <ws>` and returns `ws`.
  - `phase-10:78`: the journey needs a "server on the seeded workspace".
  - `phase-10:53-57` and `phase-12:32-36` do not list `web/e2e/support/server.ts` and say "Do not modify any other file".
- Fix (pick one): add `web/e2e/support/server.ts` to phase 10 Modify with `startServer({ workspace?: string })`, which skips `init` and the smoke skill when a workspace is given. Or change the seed to `seedDistillWorkspace(ws)`, which seeds into `server.ws` after `startServer()` and does not run `init`.

**H3. The `../etc` assertion in TestRunEndpoints cannot pass as written.**
- Evidence: `phase-10:66` expects "`../etc` → 400".
- `http.ServeMux` cleans the path first. I tested this with Go's `net/http` mux and the same patterns:
  - `GET /api/v1/runs/../etc` returns **307** to `/api/v1/etc`.
  - `GET /api/v1/runs/..%2Fetc` matches `{id}` with value `../etc`, which reaches `readRun`. `safeOpaqueRecordID` then returns `invalid_request` (`distill.go:1097`), so the result is 400.
- Under the Failure Protocol, the literal form stops the phase.
- Fix: send `/api/v1/runs/..%2Fetc` (or another ID that fails `safeOpaqueRecordID`), and say so in Task 10.2.

### Medium

**M1. The handoff idempotency key is not scoped to the selected sources.**
- Evidence: `phase-10:40` says "a `handoff_request_id` ... kept in a per-workspace draft so reopening the screen copies the same brief". `drafts.ts` keys drafts by `(workspace, kind, id)`. No `id` is specified.
- If the same key is reused for a different source set, `curation_run_start` sees the same `idempotency_key` with a different request, which produces an idempotency conflict.
- Fix: draft `kind = "handoff"` and `id` = the sorted source IDs joined (also used as `baseDigest`). Add a test: a different selection gets a different key, and the same selection keeps its key.

**M2. Phase 11 breaks an executor hard rule, with no exception written down.**
- Evidence: `plan.md:135` says "never delete, rename, or skip a test". `phase-11:53` step 4 deletes `TestOpaqueCursorMultiPageAndIntegrity` from `server_test.go` (it is at line 566) and moves it to `paging_test.go`.
- The closed plan stated this exception explicitly (`plans/261003-1645-webui-v1-implementation/plan.md:44`). B's executor notes do not, so a strict executor will hit the Failure Protocol.
- Fix: add "except the move of `TestOpaqueCursorMultiPageAndIntegrity` in Task 11.1 (same name, same assertions)" to `plan.md:135`.

**M3. Several mutations leave related views showing stale data.**
- Phase 11 (`phase-11:37`): decisions invalidate only `['inbox']` and `['insight', id]`. But:
  - The Inbox nav badge comes from `useHome()` (`web/src/components/NavRail.tsx:16,73`).
  - Home has `review_insights`.
  - Pending-insight counts appear in `['sources']` (`SourceSummary.PendingInsights`) and in `['skill-sources', skillId]` (`phase-08:46`).
- Phase 10 (`phase-10:41`): cancel invalidates `['run', id]` and `['sources']`, but not `['home']`. Home's `resume_run` action (`web/src/screens/home/action-cta.ts`) can keep pointing at the cancelled run.
- Fix:
  - Decision: add `['home']`, `['sources']`, and `['skill-sources', skillId]`.
  - Apply: add `['sources']`.
  - Cancel: add `['home']`.
  - Assert each list in the component tests.

**M4. The new `SourceSummary` fields are not fully specified for the client.**
- Evidence: `phase-05:32` adds `Status`, `CurrentRevision`, `DistilledRevision`, and `ReadyToDistill`, but gives no JSON tags. `phase-10:36` reads `ready_to_distill`, `status`, and `distilled_revision`.
- `phase-10:36` also needs to tell "upstream-only" apart (to show `Not a learning source`), but the summary has no `purpose` or upstream-only flag. `Roles` comes from linked skills, not from `purpose: upstream` (phase 3:70). A `purpose: upstream` source with no remaining skills would show "Up to date".
- `CurrentRevision string` does not say which part of `sourcepkg.Revision` (`internal/source/types.go:106-111`) it holds.
- Fix: give explicit tags (`status`, `current_revision`, `distilled_revision`, `ready_to_distill`), state that `*_revision` holds `Revision.Value`, and add `UpstreamOnly bool \`json:"upstream_only,omitempty"\``, computed by the same shared predicate.

**M5. "Implement the upstream-only test once" has no defined mechanism.**
- Evidence: phase 3 Requirement 6 puts the rule into the SQL `ChangedSources` count (`phase-03:75`, `curation_home.go:215`). Phase 5 builds `ReadyToDistill` in a Go read model. `phase-05:32` says "never copy the rule", but one predicate cannot serve both a SQL query and a Go loop.
- An executor will either copy the rule (which breaks the instruction) or improvise.
- Fix: in phase 5, name one Go helper (for example `isUpstreamOnly(record, learningLinked bool)`). Either change `curation_home.go` to compute `ChangedSources` from records in Go with that helper, or keep a single SQL predicate constant and derive `ReadyToDistill` through it. Keep the `curation_home_test` fixture from phase 3 as the regression test.

**M6. Scope dropped from plan A with no stated reason.**
- (a) **Mockup-parity checklists and screenshots per phase** (A `phase-04:121`, A `phase-05:109`). In plan A's validation, Q4 named these as the compensating evidence for the user's "no gates" decision (`plans/261003-1645-webui-v1-implementation/plan.md:105`). Phases 10–12 and the executor report format omit them. Restore them in the phase reports, or record the user's acceptance of dropping them.
- (b) **Plan A acceptance "Golden JSON produced by Go tests is the only API fixture used by frontend tests"** (A `plan.md:76`) is not in B's acceptance criteria. `run.test.tsx`, `distill.test.tsx`, and the inbox, insight, and composer tests are not told to load goldens. Phase 10 only goldens the cancel-again error. Carry the criterion over, and add run, inbox, and insight goldens that the frontend tests load through `web/src/test/golden.ts`.
- (c) **Plan A's "Open a run" box added the run to recent runs on success** (A `phase-04:84`). `phase-10:39` only navigates. Restore the behavior, or state why it was dropped.
- (d) **A "Check due sources" button** (A `phase-04:84`). Phase 9 has only `Check all`, but Home's `check_due_sources` CTA is still labelled "Check due sources →" and links to `/sources` (`action-cta.ts`). See M7.

**M7. "Check all" in phases 8–9 maps to a parameter that only checks due sources.**
- Evidence: `phase-09:31` calls `checkSources({all: true})`, and `phase-08:33` accepts `{source_ids?, all?}`. But `CheckSources(ctx, path, ids, allDue)` skips sources with monitoring off or manual cadence when `allDue` is true (`internal/app/source.go:443,473-475`). Phase 8 never says how `all` maps to `allDue`.
- Result: "Check all" either checks only due sources, or the handler must build the full ID list itself. The plan specifies neither.
- Fix: define it. For example, `all: true` means the handler lists every record ID and passes `ids`. Optionally add `all_due` to match the Home CTA.

**M8. TestRoutesNeverMutateRuns can miss routes without failing.**
- Today there are 15 literal patterns: 13 `METHOD /path` routes plus `/api/` and `/` in `assets.go:32-33`. Phase 8 adds 12 and phase 10 adds 2, for 29. So `>= 20` is not a vacuous pass today, but up to 9 routes could disappear unnoticed.
- The bigger gap: "collect the string-literal first argument" silently skips a pattern passed as a variable or constant. A future `const p = "POST /api/v1/runs/{id}/start"` would escape the check.
- Fix: fail on any `HandleFunc`/`Handle` call whose first argument is not a `*ast.BasicLit`. Assert that both run patterns are in the collected set, and that every non-test file containing `registerRoutes(` contributes at least one pattern. Keep the threshold as a floor.
- (The HTTP subtest is fine: a probe returns 404 through the `/api/` catch-all, which I confirmed empirically.)

**M9. Phase 13's claim check cannot reach zero mismatches.**
- Evidence: Task 13.1 (`phase-13:49`) captures only `skill --help`, `source --help`, and `check --help`. Task 13.5 (`phase-13:65`) requires every quoted command to appear in that output.
- But Requirements 9–12 quote `skillhub serve web`, `--loopback-only`, and `--allow-host`, and the §2.6 handoff text quotes `distill` commands.
- Fix: add `/tmp/skillhub serve --help` (or `help serve`) and `/tmp/skillhub distill --help` to Task 13.1.

**M10. References that are stale or left out of scope.**
- `plan.md:103`: "documented in phase 10" should say phase 13. This is outside the log sections that `plan.md:232` excuses.
- `phase-13:33` (Requirement 7) updates §2.6, §2.7, and §0. It does not update spec 04's §1 navigation diagram and route table, which still list `/sources/watch` (`docs/use-cases/04-webui-user-flows-and-screen-specs.md:51,68`), or the §3.5 title "Watch, Check và Distill Source" (line 495).
- `web/src/components/AppShell.tsx:51-54` still titles `/sources/watch` "Watch source" with a Sources breadcrumb, while phase 10 makes that route render `NotFoundPage`. `AppShell.tsx` is not in phase 10's file list.
- Fix: correct `plan.md:103`; extend Requirement 7 to cover §1 and §3.5; add `AppShell.tsx` (the watch branch only) to phase 10 Modify.

### Low

- **L1.** `phase-10:119` says a rollback returns routes to `LaterPhasePage`, but phase 11 deletes `LaterPhasePage.tsx` (`phase-11:46`). Reverting phase 10 after phase 11 needs that file restored. Note this in the rollback text.
- **L2.** `phase-12:26` does not name the job that attaches `THIRD_PARTY_NOTICES.md`. Assets are uploaded from `draft` (around `release.yml:364`), while `web` (line 187) builds. Name the job and the artifact handoff. In `phase-12:27`, "after install" is ambiguous because the existing Unix smoke step ends with uninstall.
- **L3.** `phase-13:20` says "close this plan and the WebUI plan it absorbed", but plan A is already `status: completed`. Task 13.6 Verify (`phase-13:70`) never checks `plan.md` `status: completed`.
- **L4.** The Composer replaces the whole `SKILL.md`, including the frontmatter `description`. Requirement 6 preserves the meta file, but a changed frontmatter description silently moves away from `skill.meta.yaml`. Either keep the frontmatter read-only in the editor, or state the expected behavior.

## Claims verified OK

- `readRun` returns `errors.New("distill run not found")` (`internal/app/distill.go:1102`), and an invalid ID returns `invalid_request` (line 1097). `Server.distill` exists (`internal/delivery/web/server.go:47`). `GetDistillRun` is a value method with no adapter dependency (line 991). `CancelDistillRun` is at line 312.
- Routes are registered per file through `registerRoutes` (`server.go:60-64`) with `mux.HandleFunc("METHOD pattern")`. There is no route-table value, so dropping the `routes()` refactor is justified. Neither phase 8 nor phase 10 patterns conflict with the existing mux. A POST to `/runs/X/start` returns 404 through the `/api/` catch-all (`assets.go:32,36-41`).
- The `distill_test.go` helpers exist with the cited names: `revisionAdapter`:34, `newDistillWorkspace`:489, `writeDistillSource`:505 (its fields match seed step 2), `prepareAndStart`:529, `validSubmission`:545.
- The CLI used by the seed exists:
  - `skill create` flags, `skill activate`, `--min-scope` values (`internal/delivery/cli/help.go:92-135`).
  - The production `filesystem` adapter is `FilesystemAdapter{Root: root}` (`source.go:101`).
- Seed step 3 works for a filesystem source:
  - `source attach <source-id|locator> --skill-id` (`phase-06:38`) goes to `PreviewAttach`. The public-GitHub restriction there applies only to `Locator`, not `SourceID` (`phase-05:23`).
- Under phase 3, `source check` still writes revisions for a learning-linked source with no `purpose` (`phase-03:70-71`).
- The `ReadyToDistill` predicate matches `curation_home.go:215` plus phase 3's upstream-only exclusion. Home's `distill_changed_sources` CTA already links to `/sources?filter=ready`, and `resume_run` links to `/sources/runs/:id` (`action-cta.ts`).
- The Composer conflict test is valid: `skill edit --description` rewrites the `SKILL.md` frontmatter (`internal/skill/lifecycle.go:319-335`), and `content_digest` is the sha256 of that content (`internal/app/skill_detail.go:83`).
- `PreviewInsightApplication` writes only the requested paths under the skill directory (`internal/app/insight.go:415-435`), so asserting a byte-identical meta file is feasible. The path from `GetSkillDetail().Path` is workspace-relative (golden `skill-detail.json:6`), and that matches `skillDirectoryForID`.
- Cited symbols and lines are accurate:
  - `insight.go`: 171, 274, 306, 391, 506, 757.
  - Paging helpers in `mcpserver/server.go`: 56, 665-724.
  - `TestOpaqueCursorMultiPageAndIntegrity` at `server_test.go:566`.
  - The fuzz target name in `ci.yml:106` and `release.yml:185`.
- Query keys `home`, `skills`, `skill`, `skill-review`, and `skill-runtime` exist (`web/src/api/queries.ts:28-68`). Phase 9 adds `skill-sources` and `sources`. Upstream confirm now invalidates `['skill-runtime', id]` (`phase-09:27`).
- The flat provenance shape is pinned by `testdata/golden/skill-review-third-party.json:101-105`. The frontend can load it through `web/src/test/golden.ts`.
- Skill Detail tab values `review`, `usage`, and `runtime` exist (`SkillDetailScreen.tsx:48,275,284`).
- The `serve web` flags `--loopback-only`, `--allow-host`, `--addr`, and `--no-open` exist, and a bad Host gets 421 (`security.go:101-102`).
- `release.yml` has job `web` at line 187 and `smoke` at line 635. It has no `serve web` step and no notices step yet.
- The Makefile has `web-test`, `web-check`, and `web-e2e`. `lint` uses `LINT_BASE ?= origin/main`.
- The doc anchors exist: `01-system-architecture.md` lines 125, 486, 561; `error-codes.md:14` "adds no web UI"; `README.md:67` `source watch`.
- Font licenses are covered by the npm notices, because the fonts are `@fontsource/*` runtime dependencies (`web/package.json`).
- Only `routes.tsx` imports `LaterPhasePage`, so phase 11 can delete it safely.
- Phase efforts add up to `plan.md`'s 168h. `ak plan validate` passes for both plans. The "moved" banners in plan A's phases 4–6 point to the correct B phases.
- Plan A's acceptance item "the web adapter never calls run start/retry/submit" is kept (`phase-10:32-33`, `plan.md:122`).

Status: DONE_WITH_CONCERNS
Summary: The absorption is mostly sound and its references check out, but three High defects would stop phase 10 or 12 at Verify: the seeded journey selects a source that is no longer distillable, the e2e server cannot run on a seeded workspace, and the `../etc` request is redirected by `ServeMux`. Ten Medium gaps cover idempotency-key scoping, the test-move rule, missing cache invalidations, under-specified fields and predicates, and dropped parity and golden-fixture evidence.
