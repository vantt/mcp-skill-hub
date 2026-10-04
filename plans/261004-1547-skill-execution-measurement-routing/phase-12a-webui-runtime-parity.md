---
phase: 12a
title: "WebUI runtime parity"
status: pending
priority: P2
effort: 8h
dependencies: [5b]
---

# Phase 12a: WebUI runtime parity

<!-- Updated: Validation Session 1 - Runtime tab reframed as readiness checklist; tab kept (decision recorded in plan.md) -->
## Goal

The WebUI Skill Detail screen shows what the CLI already knows about a skill's content trust and runtime: whether it is third-party and approved, its content digest, what changed since the last approval (with script, runtime, and dependency flags), the **exact CLI command** to approve it, runtime hints, the declared runtime block, the setup state agents see, the cached doctor result with its basis, and the names (never values) of stored env keys.

**Approval stays CLI-only.** The WebUI has no approve button, no approve endpoint, and no request field that can set `quality.content_reviewed_digest`. Reason: approval is a human act bound to reviewed content; anything an agent can drive (MCP tools, or a browser session an agent controls) must not be able to approve. The UI shows the command and tells the user to run it in a terminal.

## Context (read these first)

- `plan.md` → "Executor notes", D3, D4, D11.
- **Dependencies:** functionally only phase 5b (all data exists today). Sequenced after phase 7 because both edit `internal/delivery/web/routes_read_test.go`, `web/src/api/types.ts`, `web/src/api/queries.ts`, and `web/src/screens/skill-detail/SkillDetailScreen.tsx`. If phase 7 is not done yet, the steps still apply; only the tab list differs.
- **Existing read API** (`internal/delivery/web/routes_read.go`): `registerReadRoutes` (`:19`) serves `GET /api/v1/skills/{id}` → `app.SkillDetail` (`internal/app/skill_detail.go:15`) and `GET /api/v1/skills/{id}/review` → `app.SkillReviewResult` (`internal/app/skill_review.go:68`). The review JSON **already contains**:
  - `content_trust` → `app.ContentTrust{third_party, approved, content_digest, reason_codes?, approve_command?, changes_since_approval?}` (`skill_review.go:95-104`);
  - `changes_since_approval` → `app.ChangesSinceApproval{found, commit?, diff_command?, added, removed, modified, runtime_changed, scripts_changed, dependencies_changed, history_truncated}` (`internal/app/skill_review_changes.go:30-41`);
  - `runtime_hints` → `skillruntime.Hints{interpreters, dependency_manifests, missing_lockfiles, absolute_install_paths, missing_runtime_block, install_prose_detected, install_cues}` (`internal/skillruntime/hints.go:21-29`);
  - `provenance` → `app.SkillProvenance` (`skill_review.go:48`).
  See `internal/delivery/web/testdata/golden/skill-review.json` for the current shape.
- **Not yet exposed to the web:** runtime block, setup annotation, cached doctor result, env key names. Sources:
  - Canonical runtime block: `reviewRuntimeSpec(skillMetaBytes) (skillruntime.Spec, bool)` (`skill_review.go:409`); `skillruntime.Spec{Requires{Bins[]{Name,Version}, Env[], Platforms[]}, Setup{Command, Check}}` (`internal/skillruntime/spec.go:19-44`).
  - Agent-facing setup annotation: `ResolverService.setupStatus(ctx, root, handle, entry)` (`internal/app/resolver.go:160`) → `setupAnnotation` (`internal/app/skill_doctor.go:220`) → `*resolverpkg.SetupStatus{State, ReasonCodes, Basis, CheckedAt}` (`internal/resolver/types.go:155`). Needs the served `DistributedSkill` from `buildDistributedSkill(ctx, root, handle, id)` (`internal/app/distribution.go:274`) inside `openDistributionGeneration` (`:256`) / `closeDistributionHandle` (`:268`).
  - Cached doctor result: `skillruntime.ReadCache(root, skillID, fingerprint) (Result, bool, error)` (`internal/skillruntime/cache.go:121`, never creates directories), fingerprint `doctorFingerprint(entry.Version, spec)` (`skill_doctor.go:31`), spec from `evaluateSkillTrust(skillID, contentJSON, entry.digests)` (`internal/app/skill_snapshot.go:312`). `skillruntime.Result{State, Basis, Checks[]{Kind,Name,Status,Detail}, CheckedAt}` (`internal/skillruntime/doctor.go:158`; `CheckOutput` is `json:"-"`).
  - Env key names: `app.SkillEnvService{}.List(ctx, workspace, id)` (`internal/app/skill_env.go:98`) → `SkillEnvResult.Keys` (sorted names only; it does not create the directory).
- **Read-only guarantee:** do not call `SnapshotService.Ensure`, `SkillDoctorService.Run`, `SkillEnvService.Set/Unset`, or anything that writes under `runtime/`.
- **Web patterns:** route registration via `init()` + `registerRoutes` (`routes_read.go:15-17`); `writeJSON`, `writeError(w, err, notFound bool)`, `writeAppError` (`internal/delivery/web/errors.go:43,51,63`); write routes decode with `DisallowUnknownFields` (`routes_skill_write.go:27`), and `skillUpdatePreviewRequest` (`routes_skill_write.go:156`) has no trust field. Fixture `newWebWorkspace`, `newTestServer`, `get` (`internal/delivery/web/fixtures_test.go`); golden test `TestReadEndpointsGolden` and `normalizeGolden` (`routes_read_test.go:17,28`). Skill ID pattern used by the app: `^[a-z0-9][a-z0-9-]{0,62}$` (`snapshotSkillIDPattern`, `skill_snapshot.go:67`).
- **Frontend patterns:** `web/src/api/types.ts` (interfaces), `web/src/api/queries.ts` (`useSkillReview` → `['skill-review', id]`), `SkillDetailScreen.tsx` (tabs `:241-271`, panels `:274-288`, `ReviewTab` receives `review`), `ReviewTab.tsx` (cards: `section.fg-card`, `.fg-card__title`, `.fg-facts/.fg-fact`, local `TITLE_*`/`LABEL_*` constants), components `CommandBlock` (command + copy), `CopyButton`, `StatusBadge` (`tone: success|warning|danger|info|neutral`), `EmptyState`, `Skeleton`. Golden loader `web/src/test/golden.ts`; unit-test pattern `web/src/screens/skills/skills.test.tsx` (QueryClient with `setQueryData`, `MemoryRouter`). E2E: `web/e2e/skill-lifecycle.spec.ts`, server helper `web/e2e/support/server.ts` (`startServer()` returns `{url, origin, ws, stop}`, runs the built binary from `web/.e2e/skillhub` with `SKILLHUB_WORKSPACE`).
- Web scripts (`web/package.json`): `typecheck`, `lint`, `test` (vitest), `e2e` (playwright); Make targets `web-test`, `web-build`, `web-check`, `web-e2e`.
- Human review in the WebUI is intended to see full content (`GET /api/v1/skills/{id}` keeps `content` for unapproved skills); do not gate it.

## Requirements

1. **App read model** `internal/app/skill_runtime_status.go`:
   ```go
   type SkillRuntimeStatus struct {
       Result
       SkillID       string                   `json:"skill_id"`
       Runtime       *skillruntime.Spec       `json:"runtime"`          // canonical block; null when absent
       Served        bool                     `json:"served"`           // active and servable in the current catalog
       Setup         *resolverpkg.SetupStatus `json:"setup,omitempty"`  // exactly what skill_resolve attaches for this skill
       Doctor        *SkillRuntimeDoctor      `json:"doctor,omitempty"` // cached terminal result, if any
       DoctorCommand string                   `json:"doctor_command"`   // "skillhub skill doctor <id>"
       EnvKeys       []string                 `json:"env_keys"`         // names only, sorted, never values
       EnvSetCommand string                   `json:"env_set_command"`  // "skillhub skill env set <id> <KEY>"
   }
   type SkillRuntimeDoctor struct {
       State     string               `json:"state"`
       Basis     string               `json:"basis"`      // "terminal"
       CheckedAt string               `json:"checked_at"` // RFC 3339 UTC
       Checks    []skillruntime.Check `json:"checks"`
   }
   func (SkillService) RuntimeStatus(ctx context.Context, path, id string) (SkillRuntimeStatus, error)
   ```
   - Unknown skill → `skill.ErrNotFound` (from `locateSkillDir`). Invalid ID → `NewInvalidRequestError`.
   - `Setup` and `Doctor` are filled only when the skill is active and servable; reuse `ResolverService{}.setupStatus` for `Setup` (do not reimplement the state order) and extract one helper `cachedDoctorResult(root string, entry DistributedSkill, contentJSON []byte) (*skillruntime.Result, error)` used by both `setupAnnotation` and `RuntimeStatus`, so the two always read the same cache entry. An untrusted skill has no cached result by design (doctor does not cache it).
   - An unusable env file (malformed or symlinked) does not fail the request: `EnvKeys` is empty and `Result.Warnings` gets `{code: "env_file_unusable", summary: "Fix or recreate it with skillhub skill env set <id> <KEY>."}`.
   - `Result` status `ok`, summary one sentence.
2. **Web endpoint** `GET /api/v1/skills/{id}/runtime` in `internal/delivery/web/routes_runtime.go` → `SkillRuntimeStatus`; 404 for unknown, 400 for an ID that fails the skill ID pattern. No other new routes.
3. **No approval path through the web (tested):** `POST /api/v1/skills/{id}/approve` is not routed (404/405), and `POST /api/v1/skills/{id}/update/preview` with a body containing `content_reviewed_digest` or `approve_content` is rejected with 400 by `DisallowUnknownFields`.
4. **Types and queries** (`web/src/api/types.ts`, `queries.ts`): add `ContentTrust`, `ChangesSinceApproval`, `RuntimeHints`, `SkillProvenance`; extend `SkillReviewResult` with `provenance?`, `content_trust`, `runtime_hints`; add `RuntimeSpec`, `SetupStatus`, `DoctorCheck`, `SkillRuntimeDoctor`, `SkillRuntimeStatus`; add `useSkillRuntime(id)` with key `['skill-runtime', id]`.
5. **Content trust card** `web/src/screens/skill-detail/ContentTrustCard.tsx`, rendered first in `ReviewTab` when `review.content_trust.third_party` is true (local skills show nothing new):
   - Badge: `Approved` (success) when `approved`; `Changed since approval` (warning) when `changes_since_approval` is present; otherwise `Review required` (warning).
   - Content digest in monospace with `CopyButton`.
   - When `changes_since_approval.found`: added/removed/modified path lists; highlighted flags for `scripts_changed`, `runtime_changed`, `dependencies_changed`; `diff_command` in a `CommandBlock`; a note when `history_truncated`. When present but `found` is false: "The approved content could not be found in Git history; review the full skill." When absent and not approved: "Never approved: review the full skill."
   - When `approve_command` is present: a `CommandBlock` with it and the text "Approval is CLI-only. Review the content, then run this command in your terminal. The WebUI and agents cannot approve content." No button, link, or form submits anything.
   - Agent impact line: approved → "Agents receive this skill's content."; otherwise → "Agents get no content and no files from this skill until it is approved."
6. **Runtime tab** `web/src/screens/skill-detail/RuntimeTab.tsx` (`?tab=runtime`, tab label `Runtime`), using `useSkillRuntime` and the review query's `runtime_hints`. It answers one question — *can this skill run on this machine, and if not, what exactly do I do?* — as a **readiness checklist**, not a dump of separate data sections. Requirement, evidence and fix sit on the same row.
   - **Header verdict** from `setup.state`: `Ready` (success), `Setup required` (warning), `Needs review` (warning, `review_required`), `Unsupported platform` (danger), `Not checked yet` (neutral, `unknown`), plus reason codes. Under it: "Hub checks only the platform. Binary/env results come from your terminal (basis: terminal) at <checked_at>, and may differ from your agent's shell." When `served` is false: "Agents receive this skill only while it is active."
   - **Checklist rows**, one per requirement, merging the runtime block with doctor evidence and stored env:
     - Platform: required platforms → pass/fail for this machine.
     - Each bin: `name` + version constraint → doctor result (`found 3.12, satisfies >=3.10` / `not found` / `not checked`).
     - Each env name: status `stored in skill env` (key present in `env_keys`) / `present in terminal env` (doctor pass) / `missing` → inline `CommandBlock` with `env_set_command` for that key. Values are never shown; render "Values are never shown." once under the env rows.
     - Setup check: `setup.check` as code → last doctor result; `setup.command` as code with "Ask before running; installs into SKILLHUB_STATE_DIR". Never a run button.
     - Footer: `doctor_command` in a `CommandBlock` ("Re-check from your terminal"); "Doctor has not run for this version." when no cached result.
   - **No runtime block:** empty state "This skill declares no runtime requirements." When `runtime_hints.missing_runtime_block` or `install_prose_detected`: a warning "This skill has scripts or install instructions but no declared runtime, so agents can't preflight it" with the detected interpreters / dependency manifests / install cue names and the suggestion `skillhub skill edit <id> --runtime-file <yaml>` (or ask the curator to propose one).
   - **Authoring warnings** (from `runtime_hints`, names/paths only): absolute install paths ("breaks when run from the hub's copy"), missing lockfiles ("installs are not reproducible"). Each with a one-line why.
   - Every value shown must match `skillhub skill doctor <id> --json` (cached), `skillhub skill env list <id>`, and `skillhub skill review <id>` for the same workspace.
7. Copy strings follow the screen's existing local-constant pattern (`LABEL_*`, `TITLE_*`, `MSG_*`).

## Files

Create:
- `internal/app/skill_runtime_status.go`, `internal/app/skill_runtime_status_test.go`
- `internal/delivery/web/routes_runtime.go`, `internal/delivery/web/routes_runtime_test.go`
- `internal/delivery/web/testdata/golden/skill-runtime.json`, `skill-runtime-third-party.json`, `skill-runtime-approved.json`, `skill-review-third-party.json` (generated)
- `web/src/screens/skill-detail/ContentTrustCard.tsx`, `web/src/screens/skill-detail/ContentTrustCard.test.tsx`
- `web/src/screens/skill-detail/RuntimeTab.tsx`, `web/src/screens/skill-detail/RuntimeTab.test.tsx`
- `web/e2e/skill-runtime.spec.ts`

Modify:
- `internal/app/skill_doctor.go` (extract `cachedDoctorResult`; `setupAnnotation` behavior unchanged)
- `internal/delivery/web/routes_read_test.go` (add the `skill-runtime` case for the local fixture skill)
- `internal/delivery/web/fixtures_test.go` (helper that builds a runtime workspace, see step 3)
- `web/src/api/types.ts`, `web/src/api/queries.ts`
- `web/src/screens/skill-detail/ReviewTab.tsx` (render `ContentTrustCard`), `web/src/screens/skill-detail/SkillDetailScreen.tsx` (Runtime tab button and panel)

## Steps

- [ ] **1. Extract `cachedDoctorResult`** from `setupAnnotation` without behavior change.
  Pass: `go test -count=1 -run 'Doctor|Resolve|Setup' ./internal/app/` → `ok` with no test edits.
- [ ] **2. `RuntimeStatus` app tests** (`skill_runtime_status_test.go`), reusing the app test helpers already in `internal/app/skill_snapshot_test.go` (`updateSkillMeta` `:22`, `writeSkillFile` `:43`, `markThirdParty` `:53`, `withRuntime` `:60`, `setReviewedDigest` `:67`):
  - local skill without runtime → `runtime: null`, `served: true`, no `setup`, no `doctor`, `env_keys: []`;
  - unapproved third-party with runtime → `setup.state == "review_required"`, no `doctor`;
  - approved third-party with runtime after a real `SkillDoctorService{}.Run` (runtime with only `requires.platforms: [linux, darwin, windows, freebsd]`, no bins, no check, so the result is `ready` everywhere) → `doctor.state == "ready"`, `doctor.basis == "terminal"`, `setup.basis == "terminal"`;
  - draft skill → `served: false`, no `setup`;
  - stored key via `SkillEnvService{}.Set(..., "RUNTIME_TEST_TOKEN", "SENTINEL-ENV-VALUE")` → `env_keys == ["RUNTIME_TEST_TOKEN"]` and the JSON encoding never contains `SENTINEL-ENV-VALUE`;
  - read-only: a recursive listing of `<workspace>/runtime` is identical before and after `RuntimeStatus` for a fresh skill (no snapshot, state dir, or config dir created);
  - malformed env file → `env_file_unusable` warning, no error.
  Pass: `go test -count=1 -run RuntimeStatus ./internal/app/` → `ok`.
- [ ] **3. Web route, goldens, and no-approval tests.**
  - Add the `skill-runtime` case (`/api/v1/skills/review-skill/runtime`, 200) and a `skill-runtime-unknown` case (404) to `TestReadEndpointsGolden`.
  - Add `newRuntimeWebWorkspace(t)` in `fixtures_test.go` that creates and activates `vendor-skill` (third-party via `provenance.origin {kind: github, repository, commit}`, runtime block with `bins: [python3]`, `env: [VENDOR_TOKEN]`, `setup.check`, a `scripts/run.py`, a `package.json` without lockfile, and a SKILL.md line mentioning `pip install` and `~/.claude/skills/`) and `approved-skill` (third-party, platforms-only runtime, `content_reviewed_digest` set from `app.SkillService{}.ContentTrustFor(...).ContentDigest` — write the runtime block and all files first, because the digest covers every file except `skill.meta.yaml` plus the runtime block; then rebuild the catalog and run `app.SkillDoctorService{}.Run` once), stores `VENDOR_TOKEN` with a sentinel value, then rebuilds the catalog. Edit `skill.meta.yaml` with `gopkg.in/yaml.v3` the way `updateSkillMeta` in `internal/app/skill_snapshot_test.go:22-40` does (those helpers are in package `app` and not importable from package `web`).
  - `TestRuntimeEndpointsGolden` writes `skill-review-third-party.json` (review of `vendor-skill`), `skill-runtime-third-party.json` (runtime of `vendor-skill`), and `skill-runtime-approved.json` (runtime of `approved-skill`) through `normalizeGolden`; it also asserts no response body contains the sentinel env value.
  - `TestWebHasNoContentApprovalPath`: `POST /api/v1/skills/vendor-skill/approve` → 404 or 405; `POST /api/v1/skills/vendor-skill/update/preview` with `{"expected_content_digest":"…","content_reviewed_digest":"sha256:…"}` → 400.
  - Generate goldens: `go test ./internal/delivery/web/ -run 'TestReadEndpointsGolden|TestRuntimeEndpointsGolden' -update`, then rerun without `-update`.
  Pass: `go test -count=1 ./internal/delivery/web/` → `ok`; the four new golden files exist; `grep -c SENTINEL internal/delivery/web/testdata/golden/*.json` prints 0 for every file.
- [ ] **4. Types, query, `ContentTrustCard`, `RuntimeTab`, tab wiring.**
  Pass: `cd web && npm run typecheck` exits 0.
- [ ] **5. Vitest.**
  - `ContentTrustCard.test.tsx`: with `loadGolden('skill-review-third-party')` shows "Review required", the digest, and the approve command text; `queryAllByRole('button', { name: /approve/i })` is empty and no `form` element exists; with a hand-built `changes_since_approval` (`found: true`, `scripts_changed: true`, `runtime_changed: true`, `dependencies_changed: false`, `modified: ['scripts/run.py']`) shows "Changed since approval", the modified path, both flags, and the diff command; with `history_truncated` shows the truncation note; with `loadGolden('skill-review')` (local skill) renders nothing.
  - `RuntimeTab.test.tsx`: `skill-runtime-third-party` → runtime block (python3, VENDOR_TOKEN as a name), `review_required` badge, hints (`missing_lockfiles`, absolute install path, install cue `pip install`), env key `VENDOR_TOKEN`, "Values are never shown."; `skill-runtime-approved` → checklist rows with doctor evidence and basis "terminal", a missing env row showing its `skillhub skill env set` command; `skill-runtime` (no runtime) → "This skill declares no runtime requirements."; the rendered text never contains `SENTINEL`.
  Pass: `make web-test` exits 0.
- [ ] **6. Build.** Pass: `make web-build` exits 0.
- [ ] **7. Playwright** `web/e2e/skill-runtime.spec.ts`: using `startServer()`, mark `smoke-skill` third-party by editing `<ws>/skills/core/smoke-skill/skill.meta.yaml` (add `provenance.origin`), run `skillhub rebuild` with `SKILLHUB_WORKSPACE=<ws>`, store a key with `skillhub skill env set smoke-skill E2E_TOKEN` (value piped on stdin); open `/skills/smoke-skill`: Review tab shows "Review required" and `skillhub skill edit smoke-skill --approve-content sha256:`; no button named /approve/i; Runtime tab shows `E2E_TOKEN` and not the piped value.
  Pass: `make web-e2e` exits 0. If Chromium cannot be downloaded in the execution environment, record the exact error in the phase report and continue; steps 1–6 remain the gate.
- [ ] **8. Gate.** Pass: `make check` exits 0 and `make web-check` exits 0.

## Acceptance tests

- Opening Skill Detail for an unapproved third-party skill shows the approve command and the agent-impact line; there is no control that approves.
- Runtime tab values match `skillhub skill review <id> --verbose`, `skillhub skill doctor <id> --json` (cached state), and `skillhub skill env list <id>` for the same workspace.
- No golden file, rendered DOM, or API response contains a stored env value.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| A future contributor adds a web approve button "for convenience" | Medium × High | Requirement text, `TestWebHasNoContentApprovalPath`, and the vitest "no approve button" assertion; phase 13 documents why. |
| Web and agent-facing setup state diverge | Low × Medium | `Setup` reuses `ResolverService.setupStatus`; doctor cache read through one shared helper. |
| Endpoint accidentally writes runtime state (snapshot/state dir) | Low × Medium | Read-only listing test in step 2; no call to `Ensure`/`Run`/`Set`. |
| Env value leakage | Low × High | Keys come from `SkillEnvService.List` (names only); sentinel assertions in Go, golden, vitest, and Playwright. |
| Golden churn in existing files | Low × Low | Existing `skill-review.json` is untouched by this phase (types only); new goldens live in new files. |

## Rollback

Revert the phase commits; the endpoint, components, and goldens disappear. No persisted state.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol": stop, do not weaken tests or edit goldens to match broken output, write `reports/<agent>-<YYMMDD-HHMM>-webui-runtime-parity.md`, set `status: blocked`, report the blocker.
