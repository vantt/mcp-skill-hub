---
phase: 4
title: "Sources, Handoff, Runs"
status: superseded
priority: P1
effort: 34h
dependencies: [3]
---

<!-- Updated: Validation Session 1 - rewritten as an executor handover; watch confirm checks ApplicationCommand; run-not-found sentinel; concrete test seeding recipes -->
> **Moved 2026-10-05:** Handoff, Run Return, run API, run sentinel, route safety, and seeds now live in `plans/261004-2234-skill-source-upstream-ux/phase-10-distill-handoff-and-runs.md` (adapted to the skill-centric Sources screen). Do not execute this file.
>
> **Archived / Superseded:** This phase implemented standalone source monitoring and orphan watch flows (`/sources/watch`, `/sources/distill`). The architecture has transitioned to a **Skill-Centric Sources model** defined in `plans/261004-2234-skill-source-upstream-ux/` (where sources are strictly attached to skills as Upstream or Learning References, with drift checking and 3-way merge updates). The UI and API work for sources is now owned by Phases 8 & 9 of `plans/261004-2234-skill-source-upstream-ux/`.


# Phase 4: Sources, Handoff, Runs

## Goal

Source monitoring and the Curator Agent handoff loop: Sources with checks, Watch Source, the Distill handoff brief, and Run Return with cancel and resume. The web adapter never starts, retries or submits a run.

## Before you start

- Read [plan.md](./plan.md) "Executor hard rules" and [decisions.md](./decisions.md) D2, D7, D8, D10, D11.
- Behavior authority: spec 04 §2.6–§2.7, §3.5, §6. Visual authority: mockup lines 322–366 (Sources), 367–385 (Watch), 386–408 (Distill), 409–446 (Run).
- Read once: `internal/delivery/mcpserver/source_watch_tools.go` lines 40–70; `internal/app/distill.go` lines 27–33 (`DistillService` fields), 1096–1110 (`readRun`); `internal/app/distill_test.go` lines 30–100 (`revisionAdapter`, the shape of a fake source adapter) and 489–545 (`newDistillWorkspace`, `writeDistillSource`, `prepareAndStart`); `internal/source/types.go` lines 136–142 (`Adapter` interface); `schemas/distill-submission.schema.json`.
- Facts verified on 2026-10-04:
  - MCP `source_watch_confirm` loads with `LoadSourceProposal` and refuses unless `preview.Confirmation.ApplicationCommand == "source_watch"`, using `app.NewInvalidRequestError(fmt.Sprintf("proposal %s is not a source_watch proposal", proposalID), "Supply a valid source_watch proposal ID.")`. The web adapter does the same.
  - `readRun` returns `errors.New("distill run not found")` for an unknown ID; the classifier turns it into `invalid_request` through its "not found" substring rule.
  - The app's own test helpers are unexported (package `app`), so web tests build their fixtures through public APIs plus a fake adapter in a `_test.go` file.
  - Production source adapters include `filesystem` (`sourcepkg.FilesystemAdapter{Root: <workspace>}`), which the end-to-end seed uses.
  - CLI: `skillhub source check <id>`, `skillhub distill prepare <source-id>`, `skillhub distill start <run-id>`, `skillhub distill submit <run-id> --submission <file>`, `skillhub distill get <run-id> --json`; the workspace comes from `SKILLHUB_WORKSPACE`.

## Tasks

### Task 4.1 — Run-not-found sentinel in the app
- Goal: the adapter can return 404 for unknown runs without string matching, while classification stays identical.
- Target files: modify `internal/app/distill.go`.
- Steps:
  1. Add `// ErrDistillRunNotFound reports a run ID with no stored run.` and `var ErrDistillRunNotFound = errors.New("distill run not found")` near the top of the file.
  2. In `readRun`, return `ErrDistillRunNotFound` in place of `errors.New("distill run not found")`. The message text is unchanged, so `app.ClassifyError` produces the same result.
- Success criteria: all existing tests pass; CLI JSON unchanged.
- Verify: `go test -count=1 ./internal/app/... ./internal/delivery/...` exits 0.

### Task 4.2 — Route-table safety
- Goal: a test that fails if any web route can mutate runs other than cancel.
- Target files: modify `internal/delivery/web/server.go`; create `internal/delivery/web/routes_test.go`.
- Steps:
  1. Collect route registrations in `func (s *Server) routes() []route` (`type route struct { method, pattern string; handler http.HandlerFunc }`) and build the mux from it in `Handler()`. This is used by production code, not only by tests.
  2. `TestRoutesNeverMutateRuns`: for every route, assert the pattern does not contain `start`, `retry` or `submit`, and that the only non-`GET` route whose pattern contains `/runs/` is `POST /api/v1/runs/{id}/cancel`.
- Success criteria: test passes.
- Verify: `go test -count=1 -v -run '^TestRoutesNeverMutateRuns$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestRoutesNeverMutateRuns`.

### Task 4.3 — Source and run endpoints
- Goal: six endpoints with golden files.
- Target files: create `internal/delivery/web/routes_sources.go`, `internal/delivery/web/routes_runs.go`, `internal/delivery/web/routes_sources_test.go`, `internal/delivery/web/routes_runs_test.go`; modify `internal/delivery/web/fixtures_test.go`, `internal/delivery/web/server.go`; add golden files.
- Steps:
  1. `GET /api/v1/sources?status=` → `s.sources.ListSources(ctx, ws, status)`.
  2. `POST /api/v1/sources/check`, body `{source_ids?: string[], all_due?: bool}`: exactly one of a non-empty `source_ids` or `all_due: true`, else `invalid_request`; → `s.sources.CheckSources(ctx, ws, ids, allDue)`.
  3. `POST /api/v1/sources/watch/preview`, body matching `app.SourceWatchInput` JSON tags: run `validateGitHubLocator(locator)` first, then `s.sources.PreviewSourceWatch`.
  4. `POST /api/v1/sources/watch/confirm`, body `{proposal_id, proposal_digest, base_version}`: pins required (same message as phase 3); `LoadSourceProposal`; the `"source_watch"` check above; `ConfirmSourceWatch` with the pins.
  5. `GET /api/v1/runs/{id}` → `s.distill.GetDistillRun`, `notFound = errors.Is(err, app.ErrDistillRunNotFound)`.
  6. `POST /api/v1/runs/{id}/cancel` (body `{}`) → `s.distill.CancelDistillRun`, same `notFound` rule.
  7. `fixtures_test.go`: add `type fakeSourceAdapter struct` implementing `sourcepkg.Adapter` with in-memory revisions (model it on `revisionAdapter` in `internal/app/distill_test.go`); `writeSourceRecord(t, root, id, adapter string, from, to *sourcepkg.Revision)` using `sourcepkg.MarshalCanonical` into `sources/catalog/<id>.yaml`; `seedRun(t, root, adapter) (runID string)` that builds the catalog (`catalog.BuildCatalogGeneration`), then calls `app.DistillService{Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter}}` `PrepareDistillRuns` and `StartDistillRun`; `seedFinalizedRun` that also calls `SubmitDistillRun` with a submission shaped like `validSubmission` in `internal/app/distill_test.go`.
  8. `TestSourceEndpoints`: list; check one source (the server's `s.sources.Adapters` set to the fake) → per-item status; check with neither IDs nor `all_due` → 400; watch preview with `/tmp/x` → 400 and the task 3.1 WHY text; watch confirm with missing pins → 400; watch confirm with an ID of a non-watch proposal → 400 with the "is not a source_watch proposal" text.
  9. `TestRunEndpoints`: get an in-progress run → 200 with `state`; get `RUN-DOESNOTEXIST0` → 404 with code `invalid_request`; get `../etc` → 400 (invalid run ID); cancel the in-progress run → 200 and state `cancelled`; cancel it again → the classified error (record code and status in the golden file).
- Success criteria: both tests pass.
- Verify: `go test -count=1 -v -run '^(TestSourceEndpoints|TestRunEndpoints)$' ./internal/delivery/web/` exits 0 and prints both `--- PASS:` lines.

### Task 4.4 — Pure frontend domain logic
- Goal: source status derivation, the handoff brief, and recent runs, each tested before any screen uses them.
- Target files: create `web/src/domain/source-status.ts`, `web/src/domain/source-status.test.ts`, `web/src/domain/handoff-brief.ts`, `web/src/domain/handoff-brief.test.ts`, `web/src/state/recent-runs.ts`, `web/src/state/recent-runs.test.ts`.
- Steps:
  1. `deriveSourceStatus(record, sessionCheck?)` returns `{ label, selectable }` in this priority order (spec 04 §2.6): session check `unavailable` → "Unreachable (last check)", not selectable; `status === 'changed'` → "Changed", selectable; `status === 'distill_pending'` → "Distill pending", selectable; no `distilled_revision` → "Never distilled", selectable; else "Up to date", not selectable. Table test with one row per branch plus a row proving priority (an unavailable check on a `changed` source is "Unreachable").
  2. `buildHandoffBrief({ sourceIds, idempotencyKey })` returns text that contains, in order: call `curation_run_start` with exactly those `source_ids` and that `idempotency_key`; read `items[].prepared.revision_package` and the target-revision resources; produce coverage, findings, comparisons, insights and outstanding decisions; call `curation_run_submit` for each run; report source errors instead of skipping them; return every run ID with its final state. `buildResumeBrief({ runId, sourceId, state, idempotencyKey, decision? })` follows spec 04 §2.6 "Resume payload" and mentions `curation_run_retry` only for `failed` or `awaiting_decision`. Snapshot tests for both (inline snapshots), plus an assertion that the brief never contains `#token=`.
  3. `recent-runs.ts` on `local-store` (key `recent-runs`, per workspace): `add({ runId, sourceId, idempotencyKey?, state, openedAt })`, `update(runId, state)`, `remove(runId)`, `list()` sorted by `openedAt` descending, at most 50 entries. Tests: add, update, remove, cap at 50, per-workspace isolation, storage unavailable.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/domain/source-status.test.ts src/domain/handoff-brief.test.ts src/state/recent-runs.test.ts` exits 0.

### Task 4.5 — Sources, Watch, Distill handoff, Run screens
- Goal: the four screens with every state in spec 04.
- Target files: create `web/src/screens/sources/`, `web/src/screens/watch/`, `web/src/screens/distill/`, `web/src/screens/run/`; modify `web/src/routes.tsx` (replace the `LaterPhasePage` entries for `/sources`, `/sources/watch`, `/sources/distill`, `/sources/runs/:id`), `web/src/api/queries.ts`, `web/src/api/types.ts`, `web/src/i18n/en.ts`.
- Steps:
  1. Sources (mockup 322–366): table and mobile cards with `deriveSourceStatus`; toolbar Watch new source, Check due sources (`all_due: true`), Check all sources (every monitored source ID), Distill with Curator Agent (selected IDs); per-row Check now; filter `?filter=ready` shows selectable rows only; check results kept in component state only (not storage) with a summary banner and per-item errors; Open-a-run box (navigates to `/sources/runs/<id>` and adds to recent runs on success); recent runs panel labeled "Recent runs on this browser (not a full workspace history)" with Remove per entry; empty states from spec 04 §5.
  2. Watch (mockup 367–385): GitHub URL required with client-side validation; source ID, ref, path, cadence (Daily, Weekly, Manual), monitoring switch that forces Manual when off, trust, license; Preview → `ProposalPreview` → confirm → navigate to `/sources` and focus the new row; handle `local_watch_unsupported`.
  3. Distill handoff (mockup 386–408): selected sources with revisions; a `handoff_request_id` generated once (`crypto.randomUUID()`) and kept in a per-workspace draft; the brief in a scrollable code block with Copy handoff; a paste box that accepts run IDs one per line, opens each with `GET /runs/{id}` independently, shows per-ID success or error, and stores successes in recent runs with the `idempotency_key`. There is no Start button.
  4. Run Return (mockup 409–446): key-value summary; the state panel per spec 04 §2.6 (prepared, in_progress, awaiting_decision with a required decision field before Copy resume handoff, failed, finalized with the Inbox CTA, cancelled read-only, any other state read-only with the raw state); refetch every 5 seconds while the state is `prepared` or `in_progress`; a polite live region announces only when the state changes; Cancel run opens `ConfirmDialog` with the spec's consequence text, then `POST /cancel`; Copy resume handoff is shown only when the recent-runs record has an `idempotency_key`, otherwise hidden with the explanation from spec 04; 404 renders `NotFoundPage`.
- Success criteria: typecheck, lint and unit tests pass.
- Verify: `make web-test` exits 0.

### Task 4.6 — End-to-end seed recipe
- Goal: a seeded real workspace with one source, one finalized run and one in-progress run, created only through the real CLI.
- Target files: create `web/e2e/support/seed-workspace.ts`.
- Steps:
  1. `seedDistillWorkspace()` (uses `SKILLHUB_WORKSPACE=<ws>` for every CLI call):
     a. `web/.e2e/skillhub init <ws> --yes`.
     b. Create and activate skill `consumer-review`: `skill create consumer-review --collection core --name "Consumer Review" --description "Review consumer changes." --content-file <md> --trigger "review consumer changes" --not-for "write prose" --min-scope single_step --yes`, then `skill activate consumer-review --yes`.
     c. Write `<ws>/fixtures/source-a/SKILL.md` with two lines of text.
     d. Write `<ws>/sources/catalog/source-a.yaml` with the same fields `writeDistillSource` writes (schema_version 1, id, adapter `filesystem`, locator path `fixtures/source-a`, status `watching`, identity name and canonical, trust source `test` reviewed true, monitoring enabled weekly, limits timeout_seconds 20, max_bytes 8388608, max_files 100, max_file_bytes 2097152) and no revisions.
     e. `skillhub source check source-a` (records the current revision through the production `filesystem` adapter).
     f. `skillhub distill prepare source-a --json`, read the run ID; `skillhub distill start <run> --json`; write a submission JSON valid against `schemas/distill-submission.schema.json` with full coverage of the run's `changed_resources` and one insight for `consumer-review`; `skillhub distill submit <run> --submission <file> --json`.
     g. Write `<ws>/fixtures/source-b/SKILL.md`, a second record `source-b` like step d, `source check source-b`, `distill prepare source-b`, `distill start <run2>` (left in progress).
     h. Return `{ ws, finalizedRunId, inProgressRunId }`.
  2. A spec-local smoke assertion: `skillhub distill get <finalizedRunId> --json` reports state `finalized`, and `<inProgressRunId>` reports `in_progress`.
- Success criteria: the seed produces both runs.
- Verify: `cd web && npx playwright test e2e/sources-runs.spec.ts --grep @seed` exits 0 (the `@seed` test in task 4.7 runs only the seed and the two state assertions).

### Task 4.7 — End-to-end sources and runs journeys
- Goal: the real binary drives the source and run screens.
- Target files: create `web/e2e/sources-runs.spec.ts`.
- Steps:
  1. A test tagged `@seed` runs `seedDistillWorkspace()` and the two state assertions.
  2. Journey (server started on the seeded workspace): Sources shows `source-a` and `source-b`; Check now on `source-a` shows a per-item result; open Distill with Curator Agent for a selected source → the brief contains `curation_run_start` and the `idempotency_key`; paste `finalizedRunId` → the Run page shows the finalized panel with the Inbox CTA; back on Sources, the recent-runs panel lists it; open `inProgressRunId` → Cancel run → the dialog states the consequence → confirm → state `cancelled`; the resume button is hidden for a run opened without a stored key.
- Success criteria: e2e passes.
- Verify: `make web-e2e` exits 0, and its output contains ` passed` and does not contain ` failed`.

### Task 4.8 — Phase close
- Goal: prove completion.
- Target files: create `plans/261003-1645-webui-v1-implementation/reports/phase-04-report.md` and screenshots under `reports/screenshots/phase-04/`.
- Steps:
  1. Mockup-parity checklist for lines 322–446.
  2. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 4`, paste its full output, commit.
- Success criteria: guard passes.
- Verify: the guard's last line is exactly `GUARD RESULT: PASS (phase 4)`.

## Progress

- [ ] Task 4.1 — Run-not-found sentinel in the app
- [ ] Task 4.2 — Route-table safety
- [ ] Task 4.3 — Source and run endpoints
- [ ] Task 4.4 — Pure frontend domain logic
- [ ] Task 4.5 — Sources, Watch, Distill handoff, Run screens
- [ ] Task 4.6 — End-to-end seed recipe
- [ ] Task 4.7 — End-to-end sources and runs journeys
- [ ] Task 4.8 — Phase close

## Failure Protocol
If any Verify step does not meet its stated pass condition:
1. You may make **one** fix attempt for that task. Change only the task's target files. Never edit a test's assertions to make it pass, never edit golden files by hand, never touch `plans/261003-1645-webui-v1-implementation/guard/`, `.golangci.yml`, or CI files unless the task lists them.
2. Re-run exactly the same Verify command.
3. If it still fails, STOP this phase. Do not try a second fix and do not reason around the failure.
4. If a `kongming` subagent can be spawned, give it: the phase and task id, the steps you ran, both Verify commands with their full output, and the pass condition. Apply its guidance, then re-run Verify once.
5. Otherwise, or if Verify still fails, report the same evidence to the user and wait.
Record every failure, the fix attempt and the outcome in the phase report.

## Rollback

Revert the phase's commits; the sentinel in task 4.1 is behavior-neutral and may stay.
