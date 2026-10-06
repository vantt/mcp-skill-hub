---
title: "Phase 10: Distill handoff and runs"
status: in-progress
---

# Phase 10: Distill handoff and runs

<!-- Moved from plans/261003-1645-webui-v1-implementation phase 4 (handoff and run part) on 2026-10-05; adapted to the skill-centric Sources screen of phase 9 -->

## Context

- Plan: [plan.md](./plan.md). Depends on phase 9 (grouped `/sources` screen, `SourceSummary` distill fields from phase 5 Requirement 5).
- Origin: the Handoff/Runs part of the closed WebUI plan's phase 4 (`plans/261003-1645-webui-v1-implementation/phase-04-sources-handoff-runs.md`). Its standalone Sources table and Watch screen are not built: phase 9 owns `/sources`, and watching without a skill is refused (D11).
- Behavior authority: spec 04 §2.6 "Curator Agent Distill Handoff" and "Distill Run Return" (including "Resume payload" and the run-state table), §3.5, §6. Visual authority: `docs/design/webui-mockup/` lines 386–408 (Distill) and 409–446 (Run).
- Read first: `internal/app/distill.go` (`DistillService` fields, `readRun` near line 1102, `GetDistillRun`, `CancelDistillRun`), `internal/app/distill_test.go` (`revisionAdapter`, `newDistillWorkspace`, `writeDistillSource`, `prepareAndStart`, `validSubmission`), `internal/source/types.go` (`Adapter`), `schemas/distill-submission.schema.json`, `internal/delivery/web/server.go` (`registerRoutes`, `Server.distill`), `internal/delivery/web/routes_sources.go` (phase 8), `internal/delivery/web/fixtures_test.go`, `web/src/screens/sources/SourcesScreen.tsx` (phase 9), `web/src/state/local-store.ts`, `web/src/state/drafts.ts`, `web/e2e/support/server.ts`, `internal/delivery/cli/help.go` (`distill` usage).
- Facts verified on 2026-10-05:
  - `readRun` returns `errors.New("distill run not found")`; `app.ClassifyError` maps it to `invalid_request` through its "not found" substring rule.
  - Routes are registered per file through `registerRoutes((*Server).registerXRoutes)` and `mux.HandleFunc("<METHOD> <pattern>", ...)`; there is no route table value to inspect.
  - `web/src/domain/` does not exist yet; `web/src/state/` has `local-store.ts` and `drafts.ts`.
  - CLI: `skillhub distill prepare|start|submit|get|retry|cancel`, workspace from `SKILLHUB_WORKSPACE` or `--workspace`.

## Overview

Finish the Curator Agent loop in the WebUI: a handoff screen that produces a copyable brief for selected learning sources, and a Run Return screen that reads one run, polls while it is active, and can cancel it. The web adapter never starts, retries, or submits a run; only the Curator Agent does that through MCP.

## Requirements

1. **Run-not-found sentinel**: `internal/app/distill.go` gains `// ErrDistillRunNotFound reports a run ID with no stored run.` `var ErrDistillRunNotFound = errors.New("distill run not found")`; `readRun` returns it. The message is unchanged, so classification and CLI JSON stay identical.
2. **Run endpoints** in new `internal/delivery/web/routes_runs.go` (own `registerRoutes` call):
   - `GET /api/v1/runs/{id}` → `s.distill.GetDistillRun`; 404 when `errors.Is(err, app.ErrDistillRunNotFound)`; an invalid run ID (for example the escaped `..%2Fetc`; a literal `../etc` is redirected by `http.ServeMux` path cleaning before any handler runs) is 400.
   - `POST /api/v1/runs/{id}/cancel` (body `{}` through `decodeJSON`) → `s.distill.CancelDistillRun`, same 404 rule.
   - No other run route. Nothing in the web adapter calls `PrepareDistillRuns`, `StartDistillRun`, `RetryDistillRun`, or `SubmitDistillRun(s)`.
3. **Route safety test** `TestRoutesNeverMutateRuns` in new `internal/delivery/web/routes_runs_test.go`: parse every non-test `.go` file of the package with `go/parser`, collect the first argument of every `HandleFunc`/`Handle` call and fail when any of them is not a string literal (a pattern passed through a variable or constant would otherwise be skipped silently), assert at least 20 patterns were found and that both `GET /api/v1/runs/{id}` and `POST /api/v1/runs/{id}/cancel` are among them, no pattern contains `start`, `retry`, or `submit`, and the only non-`GET` pattern containing `/runs/` is `POST /api/v1/runs/{id}/cancel`. A second subtest sends authenticated `POST /api/v1/runs/RUN-X/start|retry|submit` and asserts 404 or 405. No production seam is added for the test.
4. **Distill fields in the Sources read model** come from phase 5 Requirement 5 (`Status`, `CurrentRevision`, `DistilledRevision`, `ReadyToDistill`). This phase only reads them, through their JSON tags `status`, `current_revision`, `distilled_revision`, `ready_to_distill`, and `upstream_only`.
5. **Frontend domain logic** (pure, tested first):
   - `web/src/domain/source-distill.ts`: `distillLabel(summary, sessionCheck?)` returns `{label, selectable}` in this priority: session check `unavailable` → `Unreachable (last check)`, not selectable; `!summary.ready_to_distill` → `Not a learning source` when `summary.upstream_only`, else `Up to date`, not selectable; `status === 'changed'` → `Changed`; `status === 'distill_pending'` → `Distill pending`; no `distilled_revision` → `Never distilled`; all three selectable.
   - `web/src/domain/handoff-brief.ts`: `buildHandoffBrief({sourceIds, idempotencyKey})` contains, in order: call `curation_run_start` with exactly those `source_ids` and that `idempotency_key`; read `items[].prepared.revision_package` and the target-revision resources; produce `coverage`, `findings`, `comparisons`, `insights`, `outstanding_decisions`; call `curation_run_submit` for each run; report per-source errors instead of skipping; return every run ID with its final state. `buildResumeBrief({runId, sourceId, state, idempotencyKey, decision?})` follows spec 04 "Resume payload" and mentions `curation_run_retry` only for `failed` or `awaiting_decision`. Neither brief ever contains `#token=`.
   - `web/src/state/recent-runs.ts` on `local-store` (key `recent-runs`, per workspace): `add({runId, sourceId, idempotencyKey?, state, openedAt})`, `update(runId, state)`, `remove(runId)`, `list()` newest first, at most 50 entries.
6. **Sources screen additions** (`SourcesScreen.tsx`, phase 9): each source row shows `distillLabel`; selectable rows get a checkbox; toolbar `Distill with Curator Agent` (disabled with the reason `Select at least one learning source` until a row is selected) navigates to `/sources/distill?source=<id>&source=<id>`; URL param `filter=ready` shows selectable rows only (Home's `distill_changed_sources` CTA already links there); a `Check due sources` toolbar action (`checkSources({due: true})`, phase 8) next to phase 9's `Check all`; an `Open a run` box navigates to `/sources/runs/<id>`, and the Run screen adds the run to recent runs once it loads successfully; a `Recent runs on this browser (not a full workspace history)` panel with `Remove` per entry. No other change to phase 9 behavior.
7. **Distill handoff screen** (`/sources/distill`, new `web/src/screens/distill/DistillHandoffScreen.tsx`): lists the selected sources with current and distilled revision (unknown or non-selectable IDs are shown as errors and excluded from the brief); a `handoff_request_id` from `crypto.randomUUID()` kept in a per-workspace draft (`drafts.ts`) whose draft ID is the sorted, comma-joined selected source IDs, so reopening the screen with the same selection copies the same brief and a different selection gets a new key (`curation_run_start` rejects one key reused for a different source set); the brief in a scrollable code block with `Copy handoff`; a `Paste run IDs returned by agent` box (one ID per line) that opens each with `GET /api/v1/runs/{id}` independently, shows per-ID success or error, and stores successes in recent runs with the `idempotency_key`. There is no Start button. Empty selection shows `Select learning sources on the Sources screen first.` with a link to `/sources?filter=ready`.
8. **Run Return screen** (`/sources/runs/:id`, new `web/src/screens/run/RunScreen.tsx`): key-value summary (run ID, source ID, state, attempt, from/to revision, package digest, `changed_resources` summary); the state panel per spec 04 (prepared, in_progress, awaiting_decision with a required decision field before `Copy resume handoff`, failed, finalized with an `Open Inbox →` CTA to `/inbox`, cancelled read-only, any other state read-only with the raw state); refetch every 5 s while `prepared` or `in_progress`; a polite live region announces only state changes; `Cancel run` (hidden for `finalized`/`cancelled`) opens `ConfirmDialog` with the spec's consequence text (the source cursor does not advance; an agent working on this run will fail to submit), then `POST /cancel`; `Copy resume handoff` only when the recent-runs record has an `idempotency_key`, otherwise hidden with the spec's explanation; 404 renders `NotFoundPage`. Successful cancel invalidates `['run', id]`, `['sources']`, and `['home']` (Home's `resume_run` action must not keep pointing at a cancelled run).
9. Routes: `web/src/routes.tsx` maps `/sources/distill` and `/sources/runs/:id` to the new screens; `/sources/watch` is removed (watching needs a skill; the Learning section's `Add learning reference` replaces it) and falls through to `NotFoundPage`; remove the `/sources/watch` title branch from `web/src/components/AppShell.tsx` so the page title matches.
10. **Seed recipe** `web/e2e/support/seed-workspace.ts` exporting `seedDistillWorkspace()`, built only through the real CLI (`web/.e2e/skillhub`, `SKILLHUB_WORKSPACE=<ws>` on every call):
    1. `init <ws> --yes`; create and activate `consumer-review` (`skill create consumer-review --collection core --name "Consumer Review" --description "Review consumer changes." --content-file <md> --trigger "review consumer changes" --not-for "write prose" --min-scope single_step --yes`, then `skill activate consumer-review --yes`).
    2. Write `<ws>/fixtures/source-a/SKILL.md` (two lines) and `<ws>/sources/catalog/source-a.yaml` with the fields `writeDistillSource` writes (schema_version 1, id, adapter `filesystem`, locator path `fixtures/source-a`, status `watching`, identity name and canonical, trust source `test` reviewed true, monitoring enabled weekly, limits timeout_seconds 20, max_bytes 8388608, max_files 100, max_file_bytes 2097152), no `purpose`, no revisions.
    3. `source attach source-a --skill-id consumer-review --yes` (phase 6), so the source is a learning reference and not an orphan.
    4. `source check source-a`; `distill prepare source-a --json` (read the run ID); `distill start <run> --json`; write a submission valid against `schemas/distill-submission.schema.json` with full coverage of the run's `changed_resources`, two observations, and two insights for `consumer-review` (phase 11 needs two); `distill submit <run> --submission <file> --json`.
    5. Repeat steps 2–4 for `source-b` but stop after `distill start` (left `in_progress`).
    6. Repeat steps 2–3 for `source-c`, then only `source check source-c`: it is checked, linked, and never distilled, so it is the one source with `ready_to_distill: true` (finalizing `source-a` set its distilled revision, and `source-b` already has an active run).
    7. Return `{ws, finalizedRunId, inProgressRunId}`.
11. **E2E server on a seeded workspace**: `web/e2e/support/server.ts` `startServer(options?: {workspace?: string})`: with `workspace`, it skips creating and initializing a temp workspace, serves the given one, and `stop()` leaves it on disk (the caller removes it); without it, behavior is unchanged. Journeys call `seedDistillWorkspace()` first, then `startServer({workspace: ws})`.

## Related code files

Create: `internal/delivery/web/routes_runs.go`, `internal/delivery/web/routes_runs_test.go`, `web/src/domain/{source-distill,handoff-brief}.ts` and their `.test.ts`, `web/src/state/recent-runs.ts`, `web/src/state/recent-runs.test.ts`, `web/src/screens/distill/{DistillHandoffScreen.tsx,distill.test.tsx}`, `web/src/screens/run/{RunScreen.tsx,run.test.tsx}`, `web/e2e/support/seed-workspace.ts`, `web/e2e/sources-runs.spec.ts`, golden files under `internal/delivery/web/testdata/golden/` (generated).

Modify: `internal/app/distill.go` (sentinel only), `internal/delivery/web/fixtures_test.go` (fake adapter and run seeding helpers), `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/routes.tsx` (the three `/sources/*` entries only), `web/src/components/AppShell.tsx` (the `/sources/watch` title branch only), `web/e2e/support/server.ts` (Requirement 11 only), `web/src/screens/sources/SourcesScreen.tsx`, `web/src/screens/sources/sources.test.tsx`, `web/src/i18n/en.ts`.

Do not modify any other file.

## Implementation steps

### Task 10.1 — Sentinel
- Steps: Requirement 1.
- Verify: `go test -count=1 ./internal/app/... ./internal/delivery/...` exits 0.

### Task 10.2 — Run endpoints and route safety
- Steps: Requirements 2–3. In `fixtures_test.go` add `fakeSourceAdapter` (in-memory revisions, modeled on `revisionAdapter`), `writeSourceRecord(t, root, id, adapter string, from, to *sourcepkg.Revision)` (via `sourcepkg.MarshalCanonical`), `seedRun(t, root, adapter) string` (build the catalog, then `app.DistillService{Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter}}` `PrepareDistillRuns` + `StartDistillRun`), and `seedFinalizedRun` (also `SubmitDistillRun` with a submission shaped like `validSubmission`; phase 11 reuses it). `TestRunEndpoints`: in-progress run → 200 with `state`; `RUN-DOESNOTEXIST0` → 404 with code `invalid_request`; `..%2Fetc` → 400 (use the escaped form; a literal `../etc` gets a 307 from `ServeMux`); cancel → 200 and `cancelled`; cancel again → the classified error (recorded in the golden file).
- Verify: `go test -count=1 -v -run '^(TestRunEndpoints|TestRoutesNeverMutateRuns)$' ./internal/delivery/web/` exits 0 and prints both `--- PASS:` lines.

### Task 10.3 — Domain logic
- Steps: Requirement 5 with table tests (one row per `distillLabel` branch plus a priority row: an unavailable check on a `changed` source is `Unreachable`), inline snapshots for both briefs, and recent-runs tests (add, update, remove, cap at 50, per-workspace isolation, storage unavailable).
- Verify: `cd web && npx vitest run src/domain src/state/recent-runs.test.ts` exits 0.

### Task 10.4 — Screens
- Steps: Requirements 6–9. Tests: `sources.test.tsx` gains selection (non-selectable rows have no checkbox; the button enables after a selection) and `?filter=ready`; `distill.test.tsx`: empty selection state, brief contains both source IDs and the key, pasted IDs show per-ID results; `run.test.tsx`: each state panel, cancel dialog text, resume hidden without a key, 404.
- Verify: `cd web && npx vitest run src/screens/sources src/screens/distill src/screens/run` exits 0.

### Task 10.5 — Seed and journey
- Steps: Requirements 10–11, then `web/e2e/sources-runs.spec.ts`: a test tagged `@seed` runs the seed and asserts with `distill get <id> --json` that the runs are `finalized` and `in_progress`; the journey (server on the seeded workspace): `/sources?filter=ready` lists `source-c` with a checkbox and does not list `source-a` (already distilled) → select `source-c` → `Distill with Curator Agent` → brief contains `curation_run_start` and the key → paste `finalizedRunId` (a run opened by paste, so its recent-runs record has no key from this brief) → Run page shows the finalized panel with `Open Inbox →` → back on `/sources` the recent-runs panel lists it → open `inProgressRunId` by URL → `Cancel run` → dialog shows the consequence → confirm → `cancelled`; resume is hidden for the run opened without a stored key; axe check on `/sources/distill` and the Run page.
- Verify: `make web-e2e` exits 0, its output contains ` passed` and not ` failed`.

### Task 10.6 — Gate and parity
- Steps: write a mockup-parity checklist for mockup lines 386–446 (Distill, Run) and the Sources additions, with screenshots under `reports/screenshots/phase-10/` (desktop and 360 px), into the phase report. Frontend tests that need API data use the Go golden files under `internal/delivery/web/testdata/golden/` as fixtures, never hand-written JSON shapes.
- Verify: `make check` exits 0 and `make web-check` exits 0; the phase report contains the checklist and links the screenshots.

## Todo

- [x] Task 10.1 sentinel
- [x] Task 10.2 run endpoints + route safety
- [x] Task 10.3 domain logic
- [x] Task 10.4 screens
- [ ] Task 10.5 seed + journey
- [ ] Task 10.6 gates

## Success criteria

A user selects learning sources on `/sources`, copies a handoff brief for the Curator Agent, pastes the returned run IDs, follows each run to `finalized` (and on to the Inbox) or cancels it; the web adapter has no route that can start, retry, or submit a run.

## UX acceptance

| Surface | Loading | Empty | Error | Key states |
|---|---|---|---|---|
| Sources (additions) | existing | existing | existing | distill label per row; checkbox only when selectable; `Distill with Curator Agent` disabled with reason |
| Distill handoff | Skeleton rows | `Select learning sources on the Sources screen first.` | per-ID error rows | same brief on reopen; no Start button |
| Run Return | Skeleton facts | — | 404 → Not found; banner + Retry | panel per state; polling only while active; cancel confirm; resume only with stored key |

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Upstream-only sources offered for distillation | M×M | `ready_to_distill` comes from the same rule as `skillhub status` (phase 3 Requirement 6, phase 5 Requirement 5); not selectable otherwise. |
| Seed depends on CLI commands from phase 6 | L×M | Phases run strictly in order; Task 10.5 fails fast with the CLI's own error text. |
| Polling floods the server | L×L | One run ID per screen, 5 s interval, only in active states. |

## Security considerations

The brief never contains the session token. The Run screen reads exactly one run ID; it never lists runs or loads source content. Cancel is a destructive action behind `ConfirmDialog`. The route safety test fails the build if a start/retry/submit route appears.

## Rollback

Revert the phase commits; `/sources/distill` and `/sources/runs/:id` return to `LaterPhasePage` (if phase 11 already deleted that component, revert phase 11 first). The sentinel is behavior-neutral and may stay.

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
