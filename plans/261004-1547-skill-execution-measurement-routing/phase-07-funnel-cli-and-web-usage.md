---
phase: 7
title: "Funnel aggregation, CLI, WebUI Usage tab"
status: pending
priority: P1
effort: 12h
dependencies: [2, 6]
---

# Phase 7: Funnel aggregation, CLI, WebUI Usage tab

## Goal

Turn the daily rollups into a funnel report (recommend → activate → outcome, plus setup and review blockers) available from `skillhub telemetry funnel`, a read-only WebUI endpoint, and a Usage tab in Skill Detail. Fix the `feedback:negative` double count first so the report is correct.

## Context (read these first)

- `plan.md` → "Executor notes", D6, D9, D10.
- Rollup metric names written today (`internal/telemetry/rollup.go` `rollupKeys`):
  - overall (`skill_id = ""`): `resolution:<resolved|no_skill|needs_context|already_covered|failed>`
  - per skill: `recommended:primary`, `recommended:supporting`, `setup:<ready|setup_required|unsupported_platform|review_required|unknown>` (top skill of a completed resolution), `doctor:<ready|setup_required|unsupported_platform|failed>`, `load:<entrypoint|reference|script|asset|resource>`, `activation:<attribution>`, `blocked:review_required` (phase 6), `feedback:<status>`, `feedback:setup_failed`, `feedback:negative`, `feedback:negative_after_load`, `transcript:<tool>`, `native:no_resolve`, `native:resolved_before` (phase 8 writes the last three).
- **Known defect:** `buildFeedbackEvents` (`internal/telemetry/feedback.go:245`) stores a primary event (`task.outcome_reported`, `skill.activated`, …) and, when `utility` is supplied, a second `skill.utility_reported` event with ID `<event_id>:utility`, inserted after the primary in the same transaction (`storeFeedbackEvents`, `feedback.go:292`). `rollupKeys` adds `feedback:negative` for a negative primary status **and** for a harmful utility, so one report with `outcome: failed` + `utility: harmful` counts twice.
- Rollup read API: `(*telemetry.Recorder).Rollups(ctx, from, to string) ([]telemetry.RollupRow, error)` (`internal/telemetry/recorder.go:224`), days `YYYY-MM-DD` inclusive; `RollupRow{Day, SkillID, Metric, Count}` (`rollup.go:28`).
- App telemetry boundary: `app.TelemetryService.Open(path)` (`internal/app/telemetry.go:42`) and `closeTelemetryRecorder` (`:114`).
- Active skills: `app.SkillService.ListSkills(ctx, path, "active")` (`internal/app/skill_list.go:32`) → `SkillListResult.Skills[].ID`. The bundled `system-curator` is not a workspace skill.
- CLI: `internal/delivery/cli/telemetry.go` — `runTelemetry` (`:34`), subcommand switch (`:52-90`), `telemetryFlags` (`:129`); help text `internal/delivery/cli/help.go:269` (`"telemetry"` entry).
- Web: routes register through `registerRoutes` in an `init()` (pattern: `internal/delivery/web/routes_read.go:15-25`); errors via `writeError(w, err, notFound bool)` / `writeAppError` (`internal/delivery/web/errors.go:51,63`); golden tests `TestReadEndpointsGolden` (`internal/delivery/web/routes_read_test.go:28`) with fixture `newWebWorkspace` (`fixtures_test.go`); goldens in `internal/delivery/web/testdata/golden/`.
- Frontend: `web/src/api/types.ts`, `web/src/api/queries.ts` (TanStack Query hooks, `apiFetch`), `web/src/screens/skill-detail/SkillDetailScreen.tsx` (tab buttons `:241-271`, panels `:274-288`; labels are local `LABEL_*` constants), reusable `StatusBadge`, `EmptyState`, `Skeleton` in `web/src/components/`; golden loader `web/src/test/golden.ts`; unit test pattern `web/src/screens/skills/skills.test.tsx`.

## Requirements

1. **Rollup fix** (`internal/telemetry/rollup.go`): a harmful `skill.utility_reported` adds `feedback:negative` (and `feedback:negative_after_load`) only when its companion primary event (ID = utility ID without the `:utility` suffix, looked up in the same transaction) does **not** have a negative status (`failed`, `rejected`, `abandoned`). Result: one feedback report counts `feedback:negative` at most once. No data migration (feature branch unreleased).
2. **`app.UsageService.Funnel(ctx, workspace string, q FunnelQuery) (FunnelReport, error)`** in `internal/app/usage.go`, `FunnelQuery{Since, Until time.Time; SkillID string}`:
   - Window defaults to the last 30 days ending today (UTC), clamped to 180 days; day granularity.
   - **Overall:** resolutions by status; `recommended_primary`, `recommended_supporting`; activations by attribution; `acceptance_rate = activation:recommended / recommended:primary`; `overrides` (`activation:override`); `misses` (`activation:after_no_skill`); `unsolicited`; `blocked_by_review` (`blocked:review_required`); `resolutions_review_required` (`setup:review_required`); `resolutions_setup_required` (`setup:setup_required`); doctor runs by state and `doctor_failure_rate = (setup_required + unsupported_platform + failed) / all doctor runs`; `setup_failed` (`feedback:setup_failed`) and `setup_failed_rate = feedback:setup_failed / sum(activation:*)`; `negative_feedback`, `negative_after_load`; transcript counts (`transcript:*`, `native:no_resolve` labelled "native skill use without a hub resolve").
   - **Per skill:** the same counters plus loads by resource kind.
   - **Lists:** `dead_skills` (active, zero recommendations, zero loads, zero blocked loads), `recommended_never_activated` (`recommended:primary > 0`, `activation:recommended = 0`, `blocked:review_required = 0`), `blocked_by_review` (skills with `blocked:review_required > 0` or `setup:review_required > 0`, sorted by count desc — "approve these to unlock demand"), `negative_after_load`, `setup_failures` (`feedback:setup_failed > 0`).
   - Rates are `null` (Go `*float64`) when the denominator is 0. The report states `window{since,until,days}`, `raw_retention_days: 14`, `rollup_retention_days: 180`, and `basis` per metric (`server-observed`, `host-reported`, `terminal`, `transcript`).
3. **CLI** `skillhub telemetry funnel [--since <Nd|YYYY-MM-DD>] [--until <YYYY-MM-DD>] [--skill <id>] [--workspace <path>] [--json]`. Human output: overall block, per-skill table sorted by `recommended_primary` desc (top 20; `--skill` shows one), then the lists. Exit 0; unknown `--skill` or malformed dates → exit 2 with the existing invalid-request renderer.
4. **Web API** `GET /api/v1/skills/{id}/usage?since=<7d|30d|90d|180d>` → per-skill `FunnelReport` (overall block omitted). Unknown skill → 404 through `writeError(w, err, true)`; bad `since` → 400 through `writeAppError(app.NewInvalidRequestError(...))`. Read-only; same token auth as other routes.
5. **WebUI Usage tab** (`?tab=usage`) rendering `UsagePanel`: window selector (7/30/90/180 days), counters (recommended, activated after recommendation, acceptance rate, overrides, misses, blocked by review, loads by kind, setup failures, negative after load, doctor states with basis "terminal"), a basis caption per group, and an empty state when every count is zero. Follow the screen's existing local-constant label pattern.

## Files

Create:
- `internal/app/usage.go`, `internal/app/usage_test.go`
- `internal/delivery/cli/telemetry_funnel.go`, `internal/delivery/cli/telemetry_funnel_test.go`
- `internal/delivery/web/routes_usage.go`, `internal/delivery/web/routes_usage_test.go`
- `internal/delivery/web/testdata/golden/skill-usage.json` (generated)
- `web/src/screens/skill-detail/UsagePanel.tsx`, `web/src/screens/skill-detail/UsagePanel.test.tsx`

Modify:
- `internal/telemetry/rollup.go`, `internal/telemetry/rollup_test.go` (double-count fix)
- `internal/delivery/cli/telemetry.go` (dispatch `funnel`, flags), `internal/delivery/cli/help.go` (`telemetry` usage)
- `internal/delivery/web/routes_read_test.go` (add the `skill-usage` golden case) and, if needed, `internal/delivery/web/fixtures_test.go` (seed rollups)
- `web/src/api/types.ts`, `web/src/api/queries.ts` (`useSkillUsage(id, since)`), `web/src/screens/skill-detail/SkillDetailScreen.tsx` (tab + panel)

## Steps

- [ ] **1. Double-count fix.** Test: one report `outcome: failed` + `utility: harmful` → `feedback:negative` = 1; `outcome: completed` + `utility: harmful` → 1; `outcome: failed` alone → 1; retry of the same report → still 1.
  Pass: `go test -count=1 -run Rollup ./internal/telemetry/` → `ok`.
- [ ] **2. UsageService.** Seed events through a real recorder in a temp workspace (`TelemetryService{}.Open`, record, `Flush`), never by writing SQL. Tests: acceptance rate; null rates at zero denominators; dead-skill detection using a rollup day 60 days old (inject the recorder clock as `rollup_test.go:79` does); window clamp at 180 days; `--skill` filter; `blocked_by_review` list; `setup_failed_rate`.
  Pass: `go test -count=1 -run Usage ./internal/app/` → `ok`.
- [ ] **3. CLI.** Human and JSON output tests; exit codes 0 and 2; help text lists `funnel`.
  Pass: `go test -count=1 -run 'Funnel|Help' ./internal/delivery/cli/` → `ok`; `go run ./cmd/skillhub help telemetry` shows `funnel`.
- [ ] **4. Web route + golden.** Add the route and a `skill-usage` case (`/api/v1/skills/review-skill/usage?since=30d`) plus a 404 case. Generate with `go test ./internal/delivery/web/ -run TestReadEndpointsGolden -update`, then rerun without `-update`.
  Pass: `go test -count=1 ./internal/delivery/web/` → `ok`; `internal/delivery/web/testdata/golden/skill-usage.json` exists.
- [ ] **5. Frontend.** Types, `useSkillUsage`, tab button, `UsagePanel`. Vitest renders `loadGolden('skill-usage')` and an all-zero report (empty state) and the window selector changes the query key.
  Pass: `make web-test` exits 0; `make web-build` exits 0.
- [ ] **6. Gate.** Pass: `make check` exits 0.

## Acceptance tests

- `skillhub telemetry funnel --json --since 7d` in a workspace with no telemetry prints a report with all counts 0 and all rates `null`, exit 0.
- After phase 6's end-to-end sequence, the funnel shows the third-party skill under `blocked_by_review` and not under `recommended_never_activated`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Readers mix host-reported and server-observed numbers | Medium × Medium | `basis` per metric in JSON and captions in human/UI output. |
| Opening a recorder per web request contends with the MCP writer | Low × Low | Read-only admin op; SQLite handles multi-process access; local UI traffic is light. |
| Day-granular windows surprise same-day comparisons | Low × Low | Documented; named cases deferred (D9). |
| The companion lookup in `rollupKeys` misses because insert order changes | Low × Medium | Test step 1 pins the behavior; the lookup is in the same transaction as the insert. |

## Rollback

Revert the phase commits; no persisted schema is added.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol": stop, do not weaken tests or edit goldens to match broken output, write `reports/<agent>-<YYMMDD-HHMM>-funnel-usage.md`, set `status: blocked`, report the blocker.
