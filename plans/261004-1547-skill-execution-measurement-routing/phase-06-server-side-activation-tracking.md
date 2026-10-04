---
phase: 6
title: "Server-side activation tracking"
status: pending
priority: P1
effort: 8h
dependencies: [2, 5b]
---

# Phase 6: Server-side activation tracking

## Goal

Every time the MCP server hands skill content to an agent, it records one content-free `skill.loaded` event (`basis: server-observed`) attributed to the resolution that recommended the skill. Requests for a skill whose content awaits human review are recorded as blocked loads, never as activations.

## Context (read these first)

- `plan.md` → "Executor notes", decisions D1, D6, D10.
- MCP server: `internal/delivery/mcpserver/server.go`
  - `type Server struct` (`:38`), `New` (`:62`, must stay telemetry-free: `TestNewDoesNotOpenTelemetry` in `server_test.go:75`), `Serve` (`:95`, opens the recorder at `:109-118` and assigns it to `adapter.resolver.Telemetry`, `adapter.feedback.Recorder`, …).
  - `getSkill(ctx, _ *mcp.ServerSession, params)` (`:192`) serves `skills/get` (active skills only) and sets `result.Local = adapter.localSkill(...)`.
  - `localSkill` (`:209`), `readResource(ctx, *mcp.ReadResourceRequest)` (`:225`), `localResourceMeta` (`:252`, returns `refused=true` when the skill is `review_required`).
- `skill_get` tool handler: `internal/delivery/mcpserver/skill_tools.go:135-175`. It handles any lifecycle state; for an unapproved third-party skill it sets `local.status = review_required` and clears `content`.
- `skill_resolve` handler: `internal/delivery/mcpserver/resolver_tools.go:13-33` (`input resolveInput`, `response` is `resolverpkg.Response`).
- go-sdk v1.8.0: tool and resource handlers receive `req.Session *mcp.ServerSession` (`$(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.8.0/mcp/shared.go:624`); custom methods (`skills/get`) receive `*mcp.ServerSession` as their second argument.
- Telemetry vocabulary: `internal/telemetry/events.go` — `EventSkillLoaded`, `LoadBasisServerObserved` (`:137`), `skillFields()` (`:178`, already allows `skill_id`, `status`, `reason_codes`, `resource_kind`, `surface`, `attribution`, `first_activation`), enums `skillResourceKinds`, `skillLoadSurfaces`, `activationAttributes` (`:125-130`), `validateEventSemantics` (`:256`).
- Rollups: `internal/telemetry/rollup.go` `rollupKeys` (server-observed loads → `load:<kind>` and, when `first_activation`, `activation:<attribution>`).
- `after_load` derivation: `resolutionHasServerObservedLoad` (`internal/telemetry/feedback.go:230`).
- Event catalog snapshot and policy revision are filled by `recordCurationTelemetry` (`internal/app/curation_telemetry.go:17`) with `curationTelemetryEvent` (`:43`); reuse this pattern.
- Resource kinds: catalog kind from `catalog.resourceKind` (`internal/catalog/project.go:402`): `instructions|reference|script|asset|resource`; `LocalResource.Kind` and `skill.Resource.Kind` carry it. Telemetry maps `instructions` → `entrypoint`.
- Bundled curator: `systemskills.CuratorSkillID` gets no `local` and no load events (curator inspection is not an activation).
- Schema: `schemas/telemetry-event-v1.schema.json` (`skillPayload` at line 92).

## Requirements

1. **Tracker** (`internal/delivery/mcpserver/activation_tracker.go`):
   - `activationTracker` maps `*mcp.ServerSession → *sessionState`, at most 256 sessions (evict least recently seen), one mutex, injectable clock.
   - `sessionState{hash string; resolutions []notedResolution (max 32, newest last); activated map[string]bool}`. `hash` is 16 random bytes hex-encoded per session (never derived from client data); it becomes `telemetry.Event.SessionIDHash`.
   - `noteResolution(session, resolverpkg.Response)` records `{resolution_id, status, primary ID, supporting IDs, at}`; entries older than 2 h are dropped lazily.
   - `attribute(session, skillID) (resolutionID, attribution string)`, newest first within TTL: primary match → `recommended`; supporting match → `supporting`; otherwise the newest resolution decides: `resolved`/`already_covered` → `override`, `no_skill` → `after_no_skill`, `needs_context` → `after_needs_context`; none → `unsolicited` with empty resolution ID.
   - `markActivation(session, resolutionID, skillID) (first bool)`: true once per (session, resolution ID or `-`, skill).
   - A nil session (in-process tests without a session) is treated as one shared anonymous session.
2. **Emit points** (best-effort; a telemetry failure never fails or delays the response beyond the catalog open):
   - `skill_resolve`: `noteResolution` after a successful resolve.
   - `skill_get` for an **active** skill, `skills/get`, and `resources/read` of `SKILL.md` → one `skill.loaded` with `resource_kind: entrypoint`, `surface` (`skill_get`|`skills_get`|`resources_read`), `attribution`, `first_activation` from `markActivation`, `basis: server-observed`, `ResolutionID`, `SessionIDHash`.
   - `resources/read` of other files → `resource_kind` mapped from the catalog kind, `first_activation: false`, attribution still computed.
   - **Blocked loads:** when the skill is `review_required` (`skill_get`/`skills/get` returned `local.status == review_required`, or `resources/read` was refused with `content_review_required`) → `skill.loaded` with `status: review_required`, `reason_codes` from the content trust verdict (e.g. `content_review_required`, `content_review_stale`), `first_activation: false`, attribution computed, `markActivation` **not** called.
   - Non-active `skill_get` (draft/deprecated/archived; curator inspections) and the bundled curator emit nothing.
3. **Telemetry semantics** (`internal/telemetry`):
   - `validateEventSemantics`: for server-observed `skill.loaded`, `status`, when present, must be `review_required`, and then `first_activation` must not be true.
   - `rollupKeys`: a server-observed load with `status: review_required` yields only `{skill_id, "blocked:review_required"}`; other server-observed loads keep `load:<kind>` / `activation:<attribution>`.
   - `resolutionHasServerObservedLoad` ignores loads with `status = 'review_required'`, so feedback after a blocked load is not `after_load`.
   - `schemas/telemetry-event-v1.schema.json`: in `skillPayload`, when `basis` is `server-observed` and `status` is present, `status` must be `review_required` (add an `if/then`), and description text says blocked loads never carry `first_activation: true`.
4. **Wiring:** `Server` gains `telemetry app.TelemetrySink` and `tracker *activationTracker`. `New` creates the tracker (cheap, no I/O); `Serve` sets `adapter.telemetry = recorder` next to the existing assignments. With no recorder, the tracker still runs and events are skipped.
5. **App helper** (`internal/app/skill_load_telemetry.go`): `RecordSkillLoad(ctx context.Context, sink TelemetrySink, workspacePath string, load SkillLoad)` where `SkillLoad{SkillID, ResourceKind, Surface, Attribution, ResolutionID, SessionIDHash string; FirstActivation bool; Blocked bool; ReasonCodes []string}`. It builds the event with `curationTelemetryEvent(telemetry.EventSkillLoaded, payload)`, sets `ResolutionID`/`SessionIDHash`, and records through `recordCurationTelemetry` (which fills catalog snapshot and policy revision and recovers panics). It never includes a URI, path, or task text.

## Files

Create:
- `internal/delivery/mcpserver/activation_tracker.go`, `internal/delivery/mcpserver/activation_tracker_test.go`
- `internal/app/skill_load_telemetry.go`, `internal/app/skill_load_telemetry_test.go`

Modify:
- `internal/delivery/mcpserver/server.go` (fields, `New`/`Serve` wiring, emits in `getSkill` and `readResource`)
- `internal/delivery/mcpserver/resolver_tools.go` (`noteResolution`; the handler's `_ *mcp.CallToolRequest` becomes `req`)
- `internal/delivery/mcpserver/skill_tools.go` (`skill_get` emit)
- `internal/delivery/mcpserver/activation_telemetry_test.go` (create) or `internal/delivery/mcpserver/subprocess_test.go` (end-to-end assertion)
- `internal/telemetry/events.go`, `internal/telemetry/rollup.go`, `internal/telemetry/feedback.go`, `internal/telemetry/rollup_test.go`, `internal/telemetry/telemetry_test.go`
- `schemas/telemetry-event-v1.schema.json`, `schemas/embed_test.go`

## Steps

- [ ] **1. Tracker with tests.** Implement the tracker with an injectable `now func() time.Time`. Table tests: every attribution class, TTL expiry at 2 h, LRU eviction at 257 sessions, 32-resolution cap, `markActivation` dedupe per (session, resolution, skill), nil-session handling.
  Pass: `go test -race -count=1 -run Tracker ./internal/delivery/mcpserver/` → `ok`.
- [ ] **2. Telemetry semantics.** Add the `status: review_required` rule, the `blocked:review_required` rollup key, the `after_load` exclusion, and the schema `if/then`. Tests: blocked load stored and rolled up as `blocked:review_required` only; blocked load with `first_activation: true` rejected; a feedback after only a blocked load has no `after_load`; schema accepts the blocked payload and rejects `status: ready` on a server-observed load.
  Pass: `go test -count=1 ./internal/telemetry/ ./schemas/` → `ok`.
- [ ] **3. App helper.** `RecordSkillLoad` with a fake sink: payload keys are exactly the allowed set; no value contains `/`, `skill://`, or the task description.
  Pass: `go test -count=1 -run SkillLoad ./internal/app/` → `ok`.
- [ ] **4. Wire emit points** behind one adapter method `recordLoad(ctx, session, skillID, kind, surface string, blocked bool, reasons []string)` that calls `attribute`, `markActivation` (only when not blocked and kind is `entrypoint`), and `app.RecordSkillLoad`. `skill_resolve` calls `noteResolution`.
  Pass: `go test -race -count=1 ./internal/delivery/mcpserver/` → `ok` (including `TestNewDoesNotOpenTelemetry`).
- [ ] **5. End-to-end test** (new file `internal/delivery/mcpserver/activation_telemetry_test.go`, or next to `TestMCPResolveTelemetryLifecycle` in `subprocess_test.go:210`) with a temp workspace and a real recorder opened via `app.TelemetryService{}.Open(root)` and assigned to the adapter's `telemetry` field (same package). `connectDistributionSession` (`distribution_integrity_test.go:18`) discards the `*Server` returned by `New`, so add a small variant that keeps it and sets its `telemetry` field before connecting. Reuse `callSkillGet` (`distribution_integrity_test.go:249`), and `setSkillMeta` (`skill_get_content_gate_test.go:17`, used to add a `provenance.origin` of kind `github` so the skill becomes an unapproved third-party skill). Sequence: `skill_resolve` → `skill_get` of the primary → `skill_get` of another active skill → `resources/read` of a reference → `skill_get` of the unapproved third-party skill. Flush the recorder and assert rollups `activation:recommended` (primary), `activation:override` (other skill), `load:reference`, `blocked:review_required` (third-party) and no `activation:*` row for the third-party skill.
  Pass: `go test -race -count=1 -run 'Activation|Telemetry' ./internal/delivery/mcpserver/` → `ok`.
- [ ] **6. Gate.** Pass: `make check` exits 0.

## Acceptance tests

- Raw `telemetry.db` bytes after the end-to-end test contain no `skill://`, no workspace path, and no task text (scan the file like existing privacy tests in `internal/telemetry/telemetry_test.go`).
- Two `skill_get` calls for the same primary within one session/resolution produce one `activation:recommended` and two `load:entrypoint`.
- A draft skill `skill_get` produces no event.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Curator `skill_get` calls on active skills count as `unsolicited` activations | Medium × Low | Curator inspections are rarely preceded by a resolve, so they land in `unsolicited`, not `recommended`; acceptance rate is unaffected; documented in phase 13. Non-active `skill_get` is not counted. |
| One stdio process serves several tasks, so a stale resolution attributes a later manual pick | Medium × Low | 2 h TTL, newest-first scan; `override` stays distinguishable from `unsolicited`. |
| Catalog open per event adds latency to `resources/read` | Low × Low | `recordCurationTelemetry` opens the current generation read-only; measure with the existing `SKILLHUB_PERF=1` tests if they slow down. |
| Memory growth | Low × Low | 256 sessions × 32 resolutions hard caps. |
| Data race in handlers | Low × Medium | Single mutex; `-race` tests. |

## Rollback

Revert the phase commits. Events already recorded age out (raw 14 days, rollups 180 days) or `skillhub telemetry purge --yes`.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol": stop, do not weaken tests or thresholds, write `reports/<agent>-<YYMMDD-HHMM>-activation-tracking.md` with the failing command, output, and diagnosis, set `status: blocked`, report the blocker.
