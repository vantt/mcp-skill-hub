---
title: "Phase 11: Inbox, Insight, Patch Composer"
status: in-progress
---

# Phase 11: Inbox, Insight, Patch Composer

<!-- Moved from plans/261003-1645-webui-v1-implementation phase 5 on 2026-10-05; fixtures come from phase 10; guard checks replaced by this plan's gates -->

## Context

- Plan: [plan.md](./plan.md). Depends on phase 10 (`seedFinalizedRun` in `internal/delivery/web/fixtures_test.go`, `seedDistillWorkspace()` in `web/e2e/support/seed-workspace.ts`).
- Origin: the closed WebUI plan's phase 5 (`plans/261003-1645-webui-v1-implementation/phase-05-inbox-insight-patch-composer.md`). Earlier user decisions kept: paging helpers move fully to `internal/delivery/paging` with the two MCP test files updated; decisions send no idempotency key; cursors are compared by decoded content, never bytes.
- Behavior authority: spec 04 §2.8–§2.9 (including Patch Composer), §3.6, §4.3, §6. Visual authority: `docs/design/webui-mockup/` lines 447–475 (Inbox), 476–509 (Insight), 510–552 (Composer).
- Read first: `internal/delivery/mcpserver/server.go` (`cursorMACKey`, `pageDigest`, `pageOwner` near line 671, `encodeCursor`, `decodeCursor`, `normalizeLimit`, `makePage`, `cursorValue`), `internal/delivery/mcpserver/types.go` (`defaultLimit`, `maximumLimit`, `page[T]`, `pageInput`), `internal/delivery/mcpserver/insight_tools.go`, `internal/app/insight.go` (`GetInsightInbox` line 171, `GetInsightDetail` 274, `DecideInsight` 306 and its derived idempotency key, `PreviewInsightApplication` 391, `ConfirmInsightApplication` 506, `errors.New("insight not found")` line 757), `internal/app/skill_detail.go` (`Path`, `ContentDigest`), `web/src/components/{ProposalPreview,ConflictDrawer,ConfirmDialog}.tsx`, `web/src/state/drafts.ts`.
- Facts verified on 2026-10-05: the paging helpers are still in `mcpserver` (no `internal/delivery/paging` package); `PreviewInsightApplication` takes `changes[]` (`path`, `contents`) and `mappings[]` (`observation_id`, `artifact_path`, `concept`) and no expected digest; `cursorMACKey` is random per process.

## Overview

The improvement loop in the WebUI: the paged Inbox, Insight Detail with decisions, and the Patch Composer where the user writes a `SKILL.md` replacement and maps every piece of evidence before previewing and confirming.

## Requirements

1. **Shared paging package** `internal/delivery/paging` (`// Package paging provides snapshot-bound, integrity-checked page cursors shared by delivery adapters.`): moved code with exported `DefaultLimit`, `MaximumLimit`, `Page[T]` (JSON tags `items`, `has_more`, `next_cursor,omitempty`, `total,omitempty`), `Owner` (was `pageOwner`), `EncodeCursor`, `DecodeCursor`, `NormalizeLimit`, `Make` (was `makePage`); `pageDigest`, `cursorChecksum`, `cursorValue`, `cursorMACKey` stay unexported. Bodies are copied unchanged. `pageInput` stays in `mcpserver`. MCP behavior and the fuzz target `FuzzMCPFrameAndInputHelpers` (name and package) are unchanged, so CI needs no edit.
2. **Insight-not-found sentinel**: `// ErrInsightNotFound reports an insight ID with no stored insight.` `var ErrInsightNotFound = errors.New("insight not found")`, used at the current `errors.New("insight not found")`.
3. **Endpoints** in new `internal/delivery/web/routes_insights.go`:
   - `GET /api/v1/inbox?limit=&cursor=`: `paging.NormalizeLimit`; `GetInsightInbox`; `owner := paging.Owner("inbox", result.Groups)`; `paging.DecodeCursor(cursor, owner, "inbox")`, on error 410 with `app.Error{Code: app.ErrorSnapshotExpired, Render: {Error: "The inbox changed since this page was loaded.", Why: "The cursor no longer matches the current inbox.", Fix: "Reload the inbox from the first page."}}`; `paging.Make` with key `group.SkillID + "\x00" + group.Category` (same key as MCP); respond `{groups, has_more, next_cursor, total}` plus the result's `schema_version` and `status`.
   - `GET /api/v1/insights/{id}` → `GetInsightDetail`; 404 when `errors.Is(err, app.ErrInsightNotFound)`.
   - `POST /api/v1/insights/{id}/decision`, body `{decision, rationale}` (an `idempotency_key` field is rejected by `decodeJSON`) → `DecideInsight` with no key, so a retry deduplicates through the derived key.
   - `POST /api/v1/insights/{id}/apply/preview`, body `{contents, mappings: [{observation_id, concept}]}`: the target path is `GetSkillDetail(insight.SkillID).Path`, the only allowed path; the client never sends a path → `PreviewInsightApplication`.
   - `POST /api/v1/insights/apply/confirm`, body = `confirmationRequest` (pins required, same message as the skill confirm route) → `ConfirmInsightApplication`.
4. **Frontend domain logic** (pure, tested first): `web/src/domain/evidence-set.ts` (`requiredObservations(detail)` = union of `insight.observation_ids` and every `comparisons[].observation_ids`, de-duplicated, tagged `direct` with its finding or `comparison` with `subject`/`verdict`; `coverage(mappings, required)` → `{mapped, required, unmapped[]}`, a mapping counts only with a non-empty trimmed `concept`; `addMapping` rejects duplicate `(observation, concept)` pairs); `web/src/domain/insight-staleness.ts` (`isInsightStale` true when a direct finding is missing or not `active`, or a comparison is `stale`); `web/src/state/composer-draft.ts` on `drafts.ts` (keyed by insight ID, stores `evidenceDigest` and `contentDigest`, `loadComposerDraft` reports `needsRebase` when either differs).
5. **Screens** (`/inbox`, `/inbox/:id`, `/inbox/:id/apply` replace their `LaterPhasePage` entries):
   - Inbox: groups with recommendation, priority, status, rank score, impact, evidence counts, stale marker; filters for status, category, and priority over loaded pages only, with `Filtering N loaded groups`; `Load more` while `has_more`; the URL keeps filters, never the cursor; on `snapshot_expired` a notice and reload from page 1; two distinct empty states (no insights at all; none matching filters).
   - Insight Detail: recommendation, rationale, status, decision history, findings, comparisons with stale flags; actions per status exactly as spec 04 §2.9; rationale required in the decision modal; `Obsolete` behind `ConfirmDialog`; `Compose patch` disabled with a visible reason when `isInsightStale`; decisions retried with the same body after a network error.
   - Composer: fixed target path; full-replacement editor seeded with current content; `Preview` disabled with a visible reason until the content differs (after normalizing one trailing newline) and coverage is complete; mapping list with add/remove concept; `mapped/required` counter in a polite live region; before each preview refetch `GET /api/v1/skills/{id}` and compare `content_digest` with the draft's, opening `ConflictDrawer` on mismatch; `ProposalPreview` then confirm with the three pins; receipt with a CTA to the skill's Review tab; narrow screens use tabs `Content` and `Mapping (mapped/required)` with the counter in the sticky action bar.
   - After a successful decision, invalidate `['inbox']`, `['insight', id]`, `['home']` (Home and the nav badge count pending insights), `['sources']`, and `['skill-sources', skillId]` (pending-insight counts); after apply also `['skill', skillId]`, `['skill-review', skillId]`, `['skill-runtime', skillId]`, `['skill-sources', skillId]`, `['skills']`, `['home']`: a content change moves content trust to stale, and the Runtime tab must not show the old state.
6. The Composer changes `SKILL.md` content only. It never edits skill metadata, so `runtime`, routing examples/counter-examples, quality fields, and `quality.content_reviewed_digest` are preserved; an approval that no longer matches shows as stale on the Review tab and is never re-approved by the WebUI.

## Related code files

Create: `internal/delivery/paging/paging.go`, `internal/delivery/paging/paging_test.go`, `internal/delivery/web/routes_insights.go`, `internal/delivery/web/routes_insights_test.go`, `web/src/domain/{evidence-set,insight-staleness}.ts` and their `.test.ts`, `web/src/state/composer-draft.ts`, `web/src/state/composer-draft.test.ts`, `web/src/screens/{inbox,insight,composer}/` (screen plus one `.test.tsx` each), `web/e2e/insight-apply.spec.ts`, golden files (generated).

Modify: `internal/delivery/mcpserver/{server,types,curation_tools,insight_tools,source_tools,workspace_tools}.go`, `internal/delivery/mcpserver/server_test.go` and `hardening_test.go` (only as Task 11.1 says), `internal/app/insight.go` (sentinel only), `internal/delivery/web/fixtures_test.go` (`seedPendingInsight`), `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/routes.tsx` (the three `/inbox*` entries and the `LaterPhasePage` import only), `web/src/i18n/en.ts`.

Delete: `web/src/components/LaterPhasePage.tsx` once `grep -rn LaterPhasePage web/src` shows no other import (every route is real after this phase).

Do not modify any other file.

## Implementation steps

### Task 11.1 — Paging package
- Steps: (1) run `go test -count=1 ./internal/delivery/mcpserver/` and save the output for the phase report; (2) `grep -n "normalizeLimit\|decodeCursor\|encodeCursor\|makePage\|pageOwner\|maximumLimit\|defaultLimit\|page\[" internal/delivery/mcpserver/*.go` and update every hit; (3) implement Requirement 1, replacing `page[X]` with `paging.Page[X]`; (4) move `TestOpaqueCursorMultiPageAndIntegrity` into `paging_test.go` with the same name, assertions, and `t.Parallel()`; delete it from `server_test.go`; (5) in `server_test.go` use `paging.MaximumLimit` where `maximumLimit` was used, and in `hardening_test.go` use `paging.NormalizeLimit` and `paging.DecodeCursor`; change nothing else in those two files; (6) rerun and compare with step 1.
- Verify: `go test -count=1 ./internal/delivery/mcpserver/ ./internal/delivery/paging/` exits 0; `go test -count=1 -v -run '^TestOpaqueCursorMultiPageAndIntegrity$' ./internal/delivery/paging/` prints `--- PASS`; `grep -nE '^func (decodeCursor|encodeCursor|normalizeLimit|makePage|pageOwner)' internal/delivery/mcpserver/*.go | wc -l` prints `0`; `go test -count=1 -run '^$' -fuzz '^FuzzMCPFrameAndInputHelpers$' -fuzztime=10s ./internal/delivery/mcpserver/` exits 0.

### Task 11.2 — Sentinel
- Steps: Requirement 2.
- Verify: `go test -count=1 ./internal/app/... ./internal/delivery/...` exits 0.

### Task 11.3 — Endpoints
- Steps: Requirement 3; `seedPendingInsight(t, root)` uses phase 10's `seedFinalizedRun` with one insight for an activated test skill. `TestInboxPaging`: three insights in three groups and `limit=2` → page 1 has 2 groups and a `next_cursor`, page 2 has 1 and `has_more: false`; a tampered cursor → 410 `snapshot_expired`; adding an insight between pages, then page 2 with the old cursor → 410. `TestInsightEndpoints`: detail 200; unknown ID → 404 `invalid_request`; decision `plan` with rationale → applied; same request again → same `operation_id`; no rationale → 400; body with `idempotency_key` → 400; apply preview with unchanged content → 400; with a missing mapping → 400; complete → `action_required` with a diff; confirm with a wrong digest → 409 `stale_proposal`; right pins → applied; replay → same receipt; the skill's metadata file is byte-identical before and after apply (Requirement 6); an apply whose content changes the `SKILL.md` frontmatter `description` → record in the golden file whether the service rejects it or applies it, and if it applies, note the resulting `SKILL.md`/meta mismatch as an open question in the phase report (do not add client-side rewriting).
- Verify: `go test -count=1 -v -run '^(TestInboxPaging|TestInsightEndpoints)$' ./internal/delivery/web/` exits 0 and prints both `--- PASS:` lines.

### Task 11.4 — Domain logic
- Steps: Requirement 4; tests cover every branch, including an observation present directly and in a comparison (counted once) and two concepts for one observation (allowed).
- Verify: `cd web && npx vitest run src/domain/evidence-set.test.ts src/domain/insight-staleness.test.ts src/state/composer-draft.test.ts` exits 0.

### Task 11.5 — Screens
- Steps: Requirement 5 with component tests per screen (states, disabled reasons, conflict drawer on digest mismatch, invalidation list after apply).
- Verify: `cd web && npx vitest run src/screens/inbox src/screens/insight src/screens/composer` exits 0.

### Task 11.6 — End-to-end insight journey
- Steps: `web/e2e/insight-apply.spec.ts` uses `seedDistillWorkspace()`: Inbox shows the insights → open one → Reject with rationale → Rejected → Reopen with rationale (if refused for lack of new evidence, assert the refusal text and continue) → Compose patch (Plan it first if needed) → edit → map every observation → counter complete → Preview → Confirm → receipt → the skill's Review loads. Conflict: open the Composer for the second insight, run `skillhub skill edit consumer-review --description "Changed outside" --yes` (`SKILLHUB_WORKSPACE=<ws>`), press Preview → Conflict Drawer opens. Axe check on all three screens.
- Verify: `make web-e2e` exits 0, its output contains ` passed` and not ` failed`.

### Task 11.7 — Gate and parity
- Steps: mockup-parity checklist for mockup lines 447–552 with screenshots under `reports/screenshots/phase-11/` (desktop and 360 px); frontend tests use the Go golden files as API fixtures, never hand-written JSON shapes.
- Verify: `make check` exits 0 and `make web-check` exits 0. The phase report includes the before/after MCP test output from Task 11.1 and the parity checklist.

## Todo

- [x] Task 11.1 paging package
- [ ] Task 11.2 sentinel
- [ ] Task 11.3 endpoints
- [ ] Task 11.4 domain logic
- [ ] Task 11.5 screens
- [ ] Task 11.6 e2e
- [ ] Task 11.7 gates

## Success criteria

From a finalized run, a user can page through the Inbox, decide an insight with a rationale, compose a `SKILL.md` replacement with every observation mapped, preview, and confirm; a concurrent edit is caught before preview; MCP paging behavior is unchanged.

## UX acceptance

| Surface | Loading | Empty | Error | Key states |
|---|---|---|---|---|
| Inbox | Skeleton groups | two distinct empty states | banner + Retry; `snapshot_expired` notice | Load more; filters over loaded pages |
| Insight Detail | Skeleton | — | 404 → Not found | actions per status; stale disables Compose with reason |
| Composer | Skeleton editor | — | banner; 409 → Review again | Preview disabled with reason; counter; Conflict Drawer |

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Paging move changes MCP behavior | L×H | Bodies copied unchanged; before/after test output compared; fuzz target rerun. |
| Composer overwrites a concurrent edit | M×M | Digest refetch before preview plus confirm pins. |
| Stale Runtime/trust view after apply | M×M | Invalidation list in Requirement 5 includes `skill-runtime` and `skill-review`. |

## Security considerations

The client never sends a target path. Skill content renders through the existing Markdown component and `DiffView`; no `dangerouslySetInnerHTML`. The WebUI never approves content.

## Rollback

Revert the phase commits. Task 11.1 is independent; revert it alone if it causes any MCP difference.

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
