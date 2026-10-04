---
phase: 3
title: "Add, Create, Skill Detail, proposals"
status: complete
priority: P1
effort: 46h
dependencies: [2]
---

<!-- Updated: Validation Session 1 - rewritten as an executor handover; skill confirm mirrors MCP (LoadSkillProposal + ConfirmSkillMutation, all three pins required) -->

# Phase 3: Add, Create, Skill Detail, proposals

## Goal

The skill authoring and lifecycle screens (Add from GitHub, Create, Skill Detail with Review, Editor and Resources) and the shared Preview → Confirm components that later phases reuse.

## Before you start

- Read [plan.md](./plan.md) "Executor hard rules" and [decisions.md](./decisions.md) D2, D7, D8, D10, D11.
- Behavior authority: spec 04 §2.3–§2.5, §3.1–§3.4, §3.7, §4, §6. Visual authority: mockup lines 179–265 (Skill Detail), 266–300 (Add), 301–321 (Create), 553–596 (Proposal Preview and destructive confirm), 597–614 (Conflict drawer).
- Read once: `internal/delivery/mcpserver/skill_tools.go` lines 20–125 (create and transition preview/confirm), `internal/delivery/mcpserver/insight_tools.go` lines 85–103 (`skill_update_preview` / `skill_update_confirm`), `internal/delivery/mcpserver/skill_add_tools.go` lines 60–92, `internal/skill/lifecycle.go` lines 83–104 (`CreateInput`, `UpdateInput`).
- Facts verified on 2026-10-04:
  - MCP confirms create, update and transition proposals with `SkillService.LoadSkillProposal(ctx, ws, proposalID)` then `ConfirmSkillMutation(ctx, ws, preview, app.ConfirmationPins{...})`, and refuses when any of the three pins is empty with `app.NewInvalidRequestError("proposal_id, proposal_digest, and base_version pins are required", "Supply all confirmation pins.")`. The web adapter does exactly the same. `SkillService.ConfirmProposal` and `DispatchConfirmProposal` must not be used (they fall back to stored pins when none are supplied; the guard forbids them).
  - Add proposals confirm with `SkillAddService.LoadSkillAddProposal` then `ConfirmSkillAdd`.
  - `SkillAddInput.Locator` also accepts local directories (CLI use). The web endpoint must reject anything that is not a public GitHub URL before calling the app.
  - `skill.RoutingInput` fields are `Operations`, `Triggers`, `NotFor`, `MinScope`.

## Tasks

### Task 3.1 — GitHub-only locator validation
- Goal: the transport boundary refuses every locator except public GitHub URLs.
- Target files: create `internal/delivery/web/locator.go`, `internal/delivery/web/locator_test.go`.
- Steps:
  1. `func validateGitHubLocator(raw string) *app.Error` returns `nil` when all hold: length ≤ 2048 after trimming; `url.Parse` succeeds; scheme is `https`; host is exactly `github.com` (case-insensitive, no port); no userinfo, no query, no fragment; path has at least two non-empty segments (`/owner/repo`), and if a third segment exists it is `tree` or `blob` followed by at least a ref segment.
  2. Otherwise return `app.NewInvalidRequestError("The web UI accepts only public GitHub URLs.", "Use https://github.com/<owner>/<repo>[/tree/<ref>/<path>]. Import local folders with the CLI.")`.
  3. `TestLocatorValidation` table: accept `https://github.com/acme/agent-skills`, `https://github.com/acme/agent-skills/tree/main/skills`, `https://GitHub.com/acme/repo/blob/main/SKILL.md`; reject `http://github.com/a/b`, `https://github.com:8443/a/b`, `https://user@github.com/a/b`, `https://github.com/a`, `https://github.com/a/b?x=1`, `https://gitlab.com/a/b`, `file:///etc`, `/etc/passwd`, `./skills`, `git@github.com:a/b.git`, `ssh://github.com/a/b`, a 3,000-character URL, and the empty string.
- Success criteria: test passes.
- Verify: `go test -count=1 -v -run '^TestLocatorValidation$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestLocatorValidation`.

### Task 3.2 — Skill write endpoints
- Goal: six endpoints that mirror the MCP tools.
- Target files: create `internal/delivery/web/routes_skill_write.go`, `internal/delivery/web/routes_skill_write_test.go`, new golden files under `internal/delivery/web/testdata/golden/`; modify `internal/delivery/web/server.go` (register routes).
- Steps:
  1. Decode bodies with `json.Decoder` and `DisallowUnknownFields`; a decode error returns `invalid_request` ("The request body is not valid JSON for this endpoint.").
  2. `POST /api/v1/skills/add/preview`, body `{locator, selection?, all?, target_id?, collection?, idempotency_key?}`: run `validateGitHubLocator` first; then `s.skillAdd.PreviewSkillAdd(ctx, ws, app.SkillAddInput{..., FullDiff: true})`.
  3. `POST /api/v1/skills/add/confirm`, body `{proposal_id, proposal_digest, base_version}`: all three non-empty after trimming, else the pins error above; `LoadSkillAddProposal`, then `ConfirmSkillAdd` with the pins.
  4. `POST /api/v1/skills/create/preview`, body `{id, collection, name, description, content, routing: {operations, triggers, not_for, min_scope}, rationale, idempotency_key?}`: build `skill.CreateInput{ID, Collection, Name, Description, Content: []byte(content), Routing: skill.RoutingInput{...}, Rationale, IdempotencyKey}`; `PreviewCreate(ctx, ws, input, true)`.
  5. `POST /api/v1/skills/{id}/update/preview`, body with optional fields `name *string`, `description *string`, `content *string`, `routing *{...}`, `rationale *string`, plus `expected_content_digest` and `idempotency_key?`: map to `skill.UpdateInput` (set `Content` and `SetContent: true` only when `content` is present; leave pointers nil when absent); `PreviewSkillUpdate(ctx, ws, id, input, true)`; `notFound = errors.Is(err, skill.ErrNotFound)`.
  6. `POST /api/v1/skills/{id}/transitions/preview`, body `{target, idempotency_key?}`: `target` must be `active`, `deprecated` or `archived`, else `invalid_request`; `PreviewTransitionWithKey(ctx, ws, id, target, true, key)`; `notFound` as above.
  7. `POST /api/v1/skills/proposals/{proposal_id}/confirm`, body `{proposal_digest, base_version}`: pins required as in step 3 (the path supplies `proposal_id`); `LoadSkillProposal`, then `ConfirmSkillMutation`.
  8. `TestSkillWriteEndpoints` (one journey test using `newWebWorkspace`): create preview for `drafted` → hash every file under the workspace (excluding `runtime/`) before and after; the hashes match. Confirm → status `applied`. Confirm again with the same pins → the same `operation_id`. Update preview with a wrong `expected_content_digest` → status 409 and code `edit_conflict`. Transition `drafted` to `archived` → classified error (record the code and status in the golden file). Transition to `active` while required routing fields are missing → status 400, code `invalid_request`. Add preview with locator `/etc` → status 400 and the WHY text from task 3.1 (proves the app was not reached). Create preview A, then create and confirm a second skill, then confirm A → status 409 and code `stale_proposal` or `stale_context` (record which in the report). Write a golden file per distinct response with the phase 1 normalizer.
  9. `TestSkillConfirmRequiresAllPins`: for both confirm endpoints, each of the three pins missing → status 400 with the exact pins message, and the workspace file hashes are unchanged.
- Success criteria: both tests pass.
- Verify: `go test -count=1 -v -run '^(TestSkillWriteEndpoints|TestSkillConfirmRequiresAllPins)$' ./internal/delivery/web/` exits 0 and prints both `--- PASS:` lines.

### Task 3.3 — Safe Markdown renderer
- Goal: untrusted skill Markdown can never run script, load remote images, or open unsafe links.
- Target files: create `web/src/components/Markdown.tsx`, `web/src/components/markdown.test.tsx`; modify `web/package.json` (add `react-markdown` with an exact version from `npm view react-markdown version`).
- Steps:
  1. Render with `react-markdown`, `skipHtml`, a `urlTransform` that returns the URL only when its scheme is `http:`, `https:` or `mailto:` (else empty string), and an `img` component override that renders only the `alt` text in a `<span>`.
  2. Links open with `rel="noreferrer noopener"`.
  3. Tests feed: `<script>alert(1)</script>`, `<img src=x onerror=alert(1)>`, `[x](javascript:alert(1))`, `![t](https://tracker.example/p.gif)`, `<iframe src=https://e.x>`, `[ok](https://example.com)`. Assert: no `script`, `iframe` or `img` element in the output; no `href` starting with `javascript:`; the `https://example.com` link is present.
- Success criteria: test passes.
- Verify: `cd web && npx vitest run src/components/markdown.test.tsx` exits 0.

### Task 3.4 — Shared proposal and recovery components
- Goal: reusable Proposal Preview, Diff, destructive confirm, Conflict Drawer, toast and unsaved-changes guard.
- Target files: create `web/src/components/ProposalPreview.tsx`, `DiffView.tsx`, `ConfirmDialog.tsx`, `ConflictDrawer.tsx`, `Toast.tsx`, `UnsavedGuard.tsx`, `web/src/components/proposal-preview.test.tsx`, `web/src/components/conflict-drawer.test.tsx`; types in `web/src/api/types.ts` read from the new golden files.
- Steps:
  1. `ProposalPreview` (port mockup lines 553–583): header with operation, target and from → to state; affected paths, routing impact, warnings; `DiffView`; collapsed "Technical details" with proposal ID, digest and base version, each copyable; Cancel and Confirm; Confirm disabled while the request is in flight; on `stale_proposal` show the danger banner, disable Confirm and show "Create new preview"; on a network error keep the same pins and allow Retry.
  2. `DiffView`: render unified diff lines with visible prefixes `+`, `−` and a space, plus an accessible label (Added, Removed, Unchanged) per line; a Split toggle only when the viewport is at least 1280 px wide.
  3. `ConfirmDialog` (mockup 584–596): names the entity and consequence; the danger button repeats the verb.
  4. `ConflictDrawer` (mockup 597–614): shows the user's draft and the latest content, the two digests under technical details, and four actions: download draft as `.md`, copy draft, use latest as base, discard draft and reload (asks for confirmation). There is no overwrite action.
  5. Modal and drawer: focus trap, `aria-labelledby` on the title, Escape closes, focus returns to the opener.
  6. `UnsavedGuard`: blocks route changes with a confirmation when the form is dirty (`useBlocker`) and sets `beforeunload`.
  7. Tests: preview renders pins and diff prefixes; Confirm calls the handler once even on double click; stale state disables Confirm; network-error retry sends the same pins; Escape closes and returns focus; the drawer has exactly the four actions and "discard" asks for confirmation.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/components/proposal-preview.test.tsx src/components/conflict-drawer.test.tsx` exits 0.

### Task 3.5 — Editor drafts
- Goal: browser-only drafts keyed by skill and digest.
- Target files: create `web/src/state/drafts.ts`, `web/src/state/drafts.test.ts`.
- Steps:
  1. Built on `local-store`: `saveDraft(workspaceId, kind, id, baseDigest, value)`, `loadDraft(...)`, `clearDraft(...)`; key name `draft.<kind>.<id>`; stored value includes `baseDigest`.
  2. `loadDraft` reports `{ value, stale: storedDigest !== currentDigest }`.
  3. Tests: round trip; stale detection; clear.
- Success criteria: tests pass.
- Verify: `cd web && npx vitest run src/state/drafts.test.ts` exits 0.

### Task 3.6 — Add, Create and Skill Detail screens
- Goal: the three screens with all spec 04 states.
- Target files: create `web/src/screens/skill-add/`, `web/src/screens/skill-create/`, `web/src/screens/skill-detail/` (`ReviewTab.tsx`, `EditorTab.tsx`, `ResourcesTab.tsx`, `SkillDetailScreen.tsx`); modify `web/src/routes.tsx` (replace the `LaterPhasePage` entries for `/skills/add`, `/skills/create`, `/skills/:id`), `web/src/api/queries.ts`, `web/src/i18n/en.ts`, `web/src/screens/skills/SkillsScreen.tsx` (Deprecate and Archive now open the transition preview).
- Steps:
  1. Add Skill (mockup 266–300, spec 04 §2.3): step 1 URL field with client-side validation that mirrors task 3.1, advanced options (collection, target ID); request cancel with `AbortController` while discovering; on `skill_selection_required` show the server WHY verbatim, a "Skill name or path" field and "Import all"; step 2 full-page review (identity, origin, license with warning when unknown or proprietary, resources with count and size, diff, technical details) with Back, Cancel, Confirm import; receipt with CTA to Review (one skill) or to `/skills?state=draft` (several).
  2. Create Skill (mockup 301–321, spec 04 §2.4): Identity, Routing, Instructions cards; instructions as scaffold, uploaded Markdown (read with `File.text()`, reject files over 1 MB or not `.md`), or written text; blur and submit validation; Preview draft; after confirm, if the scaffold was untouched, show the "not yet activatable" notice.
  3. Skill Detail (mockup 179–265, spec 04 §2.5): header with lifecycle badge and next action; tabs synced to `?tab=review|editor|resources`; Review cards; Activation readiness lists missing fields, each linking to the Editor control (`content` → Markdown editor, `trigger` → Triggers, `not_for or rationale` → Not-for/Rationale, `min_scope` → Min scope); lifecycle matrix exactly as spec 04 §2.5 (Activate disabled with reasons when not ready; Archive uses `ConfirmDialog` before the preview); Editor with split view on wide screens and Edit/Preview tabs on narrow ones, `Markdown` for preview, routing panel, draft autosave via `drafts.ts`, `expected_content_digest` from the loaded detail, conflict on `edit_conflict` opens `ConflictDrawer`; Resources tab shows metadata only, with Changed and Missing tags and the recovery guidance text; `resource_content_unavailable` shows the divergence panel with copy-command `skillhub rebuild`.
  4. After every confirm: invalidate `['skills']`, `['skill', id]`, `['skill-review', id]`, `['home']`; clear the draft; show the receipt toast.
  5. A 404 from a read renders `NotFoundPage`.
- Success criteria: typecheck, lint and unit tests pass.
- Verify: `make web-test` exits 0.

### Task 3.7 — End-to-end lifecycle and conflict journeys
- Goal: the real binary completes the lifecycle and the conflict recovery.
- Target files: create `web/e2e/skill-lifecycle.spec.ts`.
- Steps:
  1. Start a server with `startServer()` from phase 2.
  2. Journey: Create skill `e2e-skill` through the UI → preview → confirm → Review shows it is not ready → Editor: add a trigger, a not-for entry and a min scope → preview → confirm → Review shows ready → Activate → preview → confirm → badge Active → Deprecate → confirm → Archive → destructive confirm → preview → confirm → badge Archived and no lifecycle action remains.
  3. Conflict: open the Editor of a draft skill, edit the content in the UI, then change the description outside the UI by running `web/.e2e/skillhub skill edit <id> --description "Changed outside" --yes` with `SKILLHUB_WORKSPACE=<ws>`, then press Preview changes → the Conflict Drawer is visible and has no overwrite action → choose "Use latest as base" → preview succeeds.
  4. Reload while the Proposal Preview is open → the modal is gone and the Editor still shows the draft text.
  5. Add Skill: the URL `/etc/passwd` shows the inline error and sends no request (assert no request to `/api/v1/skills/add/preview`).
- Success criteria: e2e passes.
- Verify: `make web-e2e` exits 0, and its output contains ` passed` and does not contain ` failed`.

### Task 3.8 — Phase close
- Goal: prove completion.
- Target files: create `plans/261003-1645-webui-v1-implementation/reports/phase-03-report.md` and screenshots under `reports/screenshots/phase-03/` (each new screen and the modal and drawer, at 1440, 1024 and 390 in light and dark).
- Steps:
  1. Write the mockup-parity checklist for mockup lines 179–321 and 553–614 (done or dropped with reason).
  2. Record the code returned for the stale-confirm case of task 3.2.
  3. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 3`, paste its full output, commit.
- Success criteria: guard passes.
- Verify: the guard's last line is exactly `GUARD RESULT: PASS (phase 3)`.

## Progress

- [x] Task 3.1 — GitHub-only locator validation
- [x] Task 3.2 — Skill write endpoints
- [x] Task 3.3 — Safe Markdown renderer
- [x] Task 3.4 — Shared proposal and recovery components
- [x] Task 3.5 — Editor drafts
- [x] Task 3.6 — Add, Create and Skill Detail screens
- [x] Task 3.7 — End-to-end lifecycle and conflict journeys
- [x] Task 3.8 — Phase close

## Failure Protocol
If any Verify step does not meet its stated pass condition:
1. You may make **one** fix attempt for that task. Change only the task's target files. Never edit a test's assertions to make it pass, never edit golden files by hand, never touch `plans/261003-1645-webui-v1-implementation/guard/`, `.golangci.yml`, or CI files unless the task lists them.
2. Re-run exactly the same Verify command.
3. If it still fails, STOP this phase. Do not try a second fix and do not reason around the failure.
4. If a `kongming` subagent can be spawned, give it: the phase and task id, the steps you ran, both Verify commands with their full output, and the pass condition. Apply its guidance, then re-run Verify once.
5. Otherwise, or if Verify still fails, report the same evidence to the user and wait.
Record every failure, the fix attempt and the outcome in the phase report.

## Rollback

Revert the phase's commits; phase 2 screens keep working and the routes fall back to `LaterPhasePage`.
