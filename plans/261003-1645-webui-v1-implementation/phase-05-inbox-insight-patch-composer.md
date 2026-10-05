---
phase: 5
title: "Inbox, Insight, Patch Composer"
status: moved
priority: P1
effort: 40h
dependencies: [3]
---

<!-- Updated: Validation Session 1 - rewritten as an executor handover; paging helpers move fully to internal/delivery/paging with MCP tests updated (user decision); decisions send no idempotency key; cursors compared by content, not bytes -->
> **Moved 2026-10-05** to `plans/261004-2234-skill-source-upstream-ux/phase-11-inbox-insight-patch-composer.md`. Do not execute this file.


# Phase 5: Inbox, Insight, Patch Composer

## Goal

The improvement loop: the paged Inbox, Insight Detail with decisions, and the Patch Composer where the user writes a `SKILL.md` replacement and maps every piece of evidence before previewing and confirming.

## Before you start

- Read [plan.md](./plan.md) "Executor hard rules" and [decisions.md](./decisions.md) D2, D7, D8, D10, D13.
- Behavior authority: spec 04 §2.8–§2.9 (including Patch Composer), §3.6, §4.3, §6. Visual authority: mockup lines 447–475 (Inbox), 476–509 (Insight), 510–552 (Composer).
- Read once: `internal/delivery/mcpserver/server.go` lines 51–57 (`cursorMACKey`), 528–620 (cursor and page helpers), 160–180 (resource listing that pages); `internal/delivery/mcpserver/types.go` lines 12–22 and 250–255; `internal/delivery/mcpserver/insight_tools.go` lines 14–40; `internal/app/insight.go` lines 70–110 (inputs), 330–340 (decision idempotency key), 480–500 (apply preview).
- Facts verified on 2026-10-04:
  - Paging helpers live in `internal/delivery/mcpserver/server.go`: `pageDigest`, `pageOwner`, `encodeCursor`, `cursorChecksum`, `decodeCursor`, `normalizeLimit`, `makePage`, type `cursorValue`, variable `cursorMACKey`; `types.go` holds `defaultLimit = 25`, `maximumLimit = 100`, `type page[T]`, `type pageInput`.
  - Callers: `curation_tools.go` (lines 78, 87, 88, 92), `insight_tools.go` (18, 27, 28, 32), `source_tools.go` (29, 47, 48, 52), `workspace_tools.go` (43, 53, 54, 58, 72, 76, 80), `server.go` (78, 168, 172, 176, 394, 399, 612). Tests: `server_test.go` (355 uses `maximumLimit`; `TestOpaqueCursorMultiPageAndIntegrity` at 554–592 uses the helpers) and `hardening_test.go` (lines 86–87 inside the CI fuzz target `FuzzMCPFrameAndInputHelpers`). Re-run `grep -n "normalizeLimit\|decodeCursor\|encodeCursor\|makePage\|pageOwner\|maximumLimit\|defaultLimit\|page\[" internal/delivery/mcpserver/*.go` before editing and update every hit.
  - `cursorMACKey` is random per process, so cursor strings differ between runs. Compare decoded page content (`items`, `has_more`, `total`), never cursor bytes.
  - `DecideInsight` derives a deterministic idempotency key from insight, decision and request digest when the caller sends none (`insight.go:334`). The web adapter must not send one, so a retry after a network error deduplicates.
  - `PreviewInsightApplication` takes `changes[]` (`path`, `contents`) and `mappings[]` (`observation_id`, `artifact_path`, `concept`) and accepts no expected digest.
  - `GetInsightDetail` returns `errors.New("insight not found")` for an unknown ID (`insight.go:757`).

## Tasks

### Task 5.1 — Move the paging helpers to a shared package
- Goal: one paging implementation used by both adapters, with MCP behavior unchanged.
- Target files: create `internal/delivery/paging/paging.go`, `internal/delivery/paging/paging_test.go`; modify `internal/delivery/mcpserver/server.go`, `types.go`, `curation_tools.go`, `insight_tools.go`, `source_tools.go`, `workspace_tools.go`, `server_test.go`, `hardening_test.go`.
- Steps:
  1. Before editing, run `go test -count=1 ./internal/delivery/mcpserver/` and save the output in the report.
  2. Create package `paging` (comment: `// Package paging provides snapshot-bound, integrity-checked page cursors shared by delivery adapters.`) containing the moved code with these exported names: `DefaultLimit`, `MaximumLimit`, `Page[T]` (same JSON tags: `items`, `has_more`, `next_cursor,omitempty`, `total,omitempty`), `Owner` (was `pageOwner`), `EncodeCursor`, `DecodeCursor`, `NormalizeLimit`, `Make` (was `makePage`). Keep `pageDigest`, `cursorChecksum`, `cursorValue` and `cursorMACKey` unexported in `paging`. Copy the bodies unchanged.
  3. Delete the moved definitions from `mcpserver/server.go` and `mcpserver/types.go` (`pageInput` stays in `mcpserver`; it is the MCP input schema).
  4. Update every caller found by the grep above to the qualified names. In type positions replace `page[X]` with `paging.Page[X]`.
  5. Move `TestOpaqueCursorMultiPageAndIntegrity` from `server_test.go` into `paging_test.go` (package `paging`) with the same name, the same assertions and the same `t.Parallel()`; only the helper names change to the exported ones. Delete it from `server_test.go`.
  6. In `server_test.go` line 355 use `paging.MaximumLimit`; in `hardening_test.go` lines 86–87 use `paging.NormalizeLimit` and `paging.DecodeCursor`. Change nothing else in those two files. The fuzz target keeps its name and package, so `.github/workflows/ci.yml` needs no change.
  7. Run the MCP and paging tests and compare the result list with step 1: the same tests pass (plus the moved one now reported under `internal/delivery/paging`).
- Success criteria: MCP behavior and test results unchanged; the helpers exist only in `paging`.
- Verify: `go test -count=1 ./internal/delivery/mcpserver/ ./internal/delivery/paging/` exits 0; `go test -count=1 -v -run '^TestOpaqueCursorMultiPageAndIntegrity$' ./internal/delivery/paging/` prints `--- PASS: TestOpaqueCursorMultiPageAndIntegrity`; `grep -nE '^func (decodeCursor|encodeCursor|normalizeLimit|makePage|pageOwner)' internal/delivery/mcpserver/*.go | wc -l` prints `0`; `go test -count=1 -run '^$' -fuzz '^FuzzMCPFrameAndInputHelpers$' -fuzztime=10s ./internal/delivery/mcpserver/` exits 0.

### Task 5.2 — Insight-not-found sentinel in the app
- Goal: 404 for unknown insights without string matching; classification unchanged.
- Target files: modify `internal/app/insight.go`.
- Steps:
  1. Add `// ErrInsightNotFound reports an insight ID with no stored insight.` and `var ErrInsightNotFound = errors.New("insight not found")`.
  2. Replace `errors.New("insight not found")` at line 757 with `ErrInsightNotFound`.
- Success criteria: existing tests pass.
- Verify: `go test -count=1 ./internal/app/... ./internal/delivery/...` exits 0.

### Task 5.3 — Inbox and insight endpoints
- Goal: five endpoints with golden files.
- Target files: create `internal/delivery/web/routes_insights.go`, `internal/delivery/web/routes_insights_test.go`; modify `internal/delivery/web/server.go`, `internal/delivery/web/fixtures_test.go`; add golden files.
- Steps:
  1. `GET /api/v1/inbox?limit=&cursor=`: `limit` parsed as an integer (missing → 0); `paging.NormalizeLimit`; `s.insights.GetInsightInbox(ctx, ws)`; `owner := paging.Owner("inbox", result.Groups)`; `lastKey, err := paging.DecodeCursor(cursor, owner, "inbox")`; on error respond with status 410 and `app.Error{Code: app.ErrorSnapshotExpired, Render: app.ErrorRender{Error: "The inbox changed since this page was loaded.", Why: "The cursor no longer matches the current inbox.", Fix: "Reload the inbox from the first page."}}`; `paging.Make(result.Groups, limit, lastKey, owner, "inbox", key)` with `key = group.SkillID + "\x00" + group.Category` (same key as MCP); respond with `{ "groups": page.items, "has_more", "next_cursor", "total" }` plus the result's `schema_version` and `status`.
  2. `GET /api/v1/insights/{id}` → `GetInsightDetail`; `notFound = errors.Is(err, app.ErrInsightNotFound)`.
  3. `POST /api/v1/insights/{id}/decision`, body `{decision, rationale}` (no idempotency key field; `DisallowUnknownFields` rejects one) → `DecideInsight(ctx, ws, id, app.InsightDecisionInput{Decision, Rationale})`.
  4. `POST /api/v1/insights/{id}/apply/preview`, body `{contents, mappings: [{observation_id, concept}]}`: load the target with `s.skills.GetSkillDetail(ctx, ws, insight.SkillID)` (insight from `GetInsightDetail`), take `detail.Path` as the only allowed path, and build `changes = [{path: detail.Path, contents}]` and `mappings[i].artifact_path = detail.Path`. The client never sends a path. → `PreviewInsightApplication(ctx, ws, id, app.PreviewInsightInput{Changes, Mappings})`.
  5. `POST /api/v1/insights/apply/confirm`, body `{proposal_id, proposal_digest, base_version}`: pins required (same message as phase 3) → `ConfirmInsightApplication(ctx, ws, proposalID, digest, base)`.
  6. Fixtures: `seedPendingInsight(t, root)` using `seedFinalizedRun` from phase 4 with one insight for the activated test skill.
  7. `TestInboxPaging`: with three insights in three groups and `limit=2`, page 1 has 2 groups and a `next_cursor`, page 2 has 1 group and `has_more: false`; a tampered cursor → 410 and code `snapshot_expired`; adding an insight between pages, then requesting page 2 with the old cursor → 410.
  8. `TestInsightEndpoints`: detail 200; unknown ID → 404 with code `invalid_request`; decision `plan` with rationale → applied; the same request again → the same `operation_id` (deterministic key); decision without rationale → 400; a body with `idempotency_key` → 400 (unknown field); apply preview with unchanged content → 400; with a missing mapping → 400; with complete mappings → `action_required` and a diff; confirm with wrong digest → 409 and code `stale_proposal`; confirm with the right pins → applied; replay → same receipt.
- Success criteria: both tests pass.
- Verify: `go test -count=1 -v -run '^(TestInboxPaging|TestInsightEndpoints)$' ./internal/delivery/web/` exits 0 and prints both `--- PASS:` lines.

### Task 5.4 — Pure frontend logic
- Goal: evidence set, staleness and composer drafts, tested first.
- Target files: create `web/src/domain/evidence-set.ts`, `web/src/domain/evidence-set.test.ts`, `web/src/domain/insight-staleness.ts`, `web/src/domain/insight-staleness.test.ts`, `web/src/state/composer-draft.ts`, `web/src/state/composer-draft.test.ts`.
- Steps:
  1. `requiredObservations(detail)` = the union of `insight.observation_ids` and every `comparisons[].observation_ids`, de-duplicated, each tagged `direct` (with the matching `findings[]` entry) or `comparison` (with the comparison's `subject` and `verdict`). `coverage(mappings, required)` returns `{ mapped, required, unmapped[] }`; a mapping counts only with a non-empty trimmed `concept`; duplicate `(observation, concept)` pairs are rejected by `addMapping`.
  2. `isInsightStale(detail)` is true when any direct finding has `status !== 'active'` or is missing, or any comparison has `stale === true`.
  3. `composer-draft.ts` on `drafts.ts`: key by insight ID; store `evidenceDigest` and `contentDigest`; `loadComposerDraft` reports `needsRebase` when either digest differs from the current ones.
  4. Tests cover every branch above, including an observation present both directly and in a comparison (counted once) and two concepts for one observation (allowed).
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/domain/evidence-set.test.ts src/domain/insight-staleness.test.ts src/state/composer-draft.test.ts` exits 0.

### Task 5.5 — Inbox, Insight Detail and Composer screens
- Goal: the three screens with every state in spec 04.
- Target files: create `web/src/screens/inbox/`, `web/src/screens/insight/`, `web/src/screens/composer/`; modify `web/src/routes.tsx` (replace the remaining `LaterPhasePage` entries), `web/src/api/queries.ts`, `web/src/api/types.ts`, `web/src/i18n/en.ts`.
- Steps:
  1. Inbox (mockup 447–475): groups with recommendation, priority, status, rank score, impact, evidence counts and a stale marker; filters for status, category and priority applied to loaded pages only, with the text "Filtering N loaded groups"; Load more while `has_more`; the URL keeps filters only, never the cursor; on `snapshot_expired` show the notice and reload from page 1; two distinct empty states.
  2. Insight Detail (mockup 476–509): recommendation, rationale, status, decision history, findings, comparisons with stale flags; actions per status exactly as spec 04 §2.9; rationale required in the decision modal; Obsolete uses `ConfirmDialog`; Compose patch disabled with a visible reason when `isInsightStale`; decision requests are retried with the same body after a network error.
  3. Composer (mockup 510–552): fixed target path from the skill detail; full-replacement editor seeded with current content; Preview disabled until content differs from the original after normalizing a single trailing newline, and until coverage is complete, each with a visible reason; mapping list with add/remove concept; counter `mapped/required` in a polite live region and "Unmapped" text for unmapped items; immediately before each preview, refetch `GET /skills/{id}` and compare `content_digest` with the one the draft was based on, and on mismatch open `ConflictDrawer` instead of previewing; `ProposalPreview` then confirm with the three pins; receipt with CTA to the skill's Review; narrow screens use tabs "Content" and "Mapping (mapped/required)" with the counter in the sticky action bar.
- Success criteria: typecheck, lint and unit tests pass.
- Verify: `make web-test` exits 0.

### Task 5.6 — End-to-end insight journey
- Goal: the real binary completes decide and apply.
- Target files: create `web/e2e/insight-apply.spec.ts`.
- Steps:
  1. Use `seedDistillWorkspace()` from phase 4 (it creates one pending insight for `consumer-review`).
  2. Inbox shows the insight → open it → Reject with rationale → status Rejected → Reopen with rationale (if the server refuses for lack of new evidence, assert the refusal text is shown and continue) → Compose patch (if the insight is not pending or planned after the previous step, Plan it first) → edit content → map every observation → counter reaches complete → Preview → Confirm → receipt → the skill's Review loads.
  3. Conflict: open the Composer again for a second seeded insight or the same skill, change the skill outside the UI with `skillhub skill edit consumer-review --description "Changed outside" --yes` (`SKILLHUB_WORKSPACE=<ws>`), press Preview → the Conflict Drawer opens.
- Success criteria: e2e passes.
- Verify: `make web-e2e` exits 0, and its output contains ` passed` and does not contain ` failed`.

### Task 5.7 — Phase close
- Goal: prove completion.
- Target files: create `plans/261003-1645-webui-v1-implementation/reports/phase-05-report.md` and screenshots under `reports/screenshots/phase-05/`.
- Steps:
  1. Include the before/after MCP test outputs from task 5.1.
  2. Mockup-parity checklist for lines 447–552.
  3. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 5`, paste its full output, commit.
- Success criteria: guard passes.
- Verify: the guard's last line is exactly `GUARD RESULT: PASS (phase 5)`.

## Progress

- [ ] Task 5.1 — Move the paging helpers to a shared package
- [ ] Task 5.2 — Insight-not-found sentinel in the app
- [ ] Task 5.3 — Inbox and insight endpoints
- [ ] Task 5.4 — Pure frontend logic
- [ ] Task 5.5 — Inbox, Insight Detail and Composer screens
- [ ] Task 5.6 — End-to-end insight journey
- [ ] Task 5.7 — Phase close

## Failure Protocol
If any Verify step does not meet its stated pass condition:
1. You may make **one** fix attempt for that task. Change only the task's target files. Never edit a test's assertions to make it pass, never edit golden files by hand, never touch `plans/261003-1645-webui-v1-implementation/guard/`, `.golangci.yml`, or CI files unless the task lists them.
2. Re-run exactly the same Verify command.
3. If it still fails, STOP this phase. Do not try a second fix and do not reason around the failure.
4. If a `kongming` subagent can be spawned, give it: the phase and task id, the steps you ran, both Verify commands with their full output, and the pass condition. Apply its guidance, then re-run Verify once.
5. Otherwise, or if Verify still fails, report the same evidence to the user and wait.
Record every failure, the fix attempt and the outcome in the phase report.

## Rollback

Revert the phase's commits. Task 5.1 is independent; revert it on its own if it causes any MCP difference.
