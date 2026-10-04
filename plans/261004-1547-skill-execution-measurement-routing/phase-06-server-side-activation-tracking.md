---
phase: 6
title: "Server-side activation tracking"
status: pending
priority: P1
effort: 8h
dependencies: [2, 4, 5]
---

# Phase 6: Server-side activation tracking

## Context

- [plan.md](./plan.md) D1. Event payload fields and rollup metrics from Phase 2.
- go-sdk v1.8.0: `ServerRequest[P].Session *ServerSession` (`mcp/shared.go:623-624`) is present on `CallToolRequest` and `ReadResourceRequest`; custom methods receive `*mcp.ServerSession` directly (`server.go:187` `getSkill`).
- The MCP `Server` struct is documented as stateless (`server.go:59`); the tracker is the first per-process state and must be bounded.
- Telemetry sink wiring: `Serve` sets `adapter.resolver.Telemetry` etc. (`server.go:103-111`); `app.TelemetrySink` (`internal/app/resolver.go:25`). Resolution events already carry `resolution_id` and the recommended IDs (`resolutionTelemetryPayload`, `resolver.go:180`).

## Requirements

1. `activationTracker` (delivery-level, in `mcpserver`):
   - Map `*mcp.ServerSession → *sessionState`, at most 256 sessions (evict least recently seen), guarded by one mutex.
   - `sessionState{hash string; resolutions []noted (max 32, newest last); activated map[string]bool}`. `hash` is a random 16-byte hex token generated per session (no derivation from client data) and used as `Event.SessionIDHash`.
   - `noteResolution(session, response)` records `{resolution_id, status, primary, supporting[], at}`; entries older than 2 h are dropped lazily.
   - `attribute(session, skillID) (resolutionID, attribution string)`, scanning newest first within TTL: primary match → `recommended`; supporting match → `supporting`; otherwise the newest resolution decides: `resolved`/`already_covered` → `override`, `no_skill` → `after_no_skill`, `needs_context` → `after_needs_context`; none → `unsolicited` (empty resolution ID). For attributed classes the resolution ID is the matching or newest resolution.
   - `markActivation(session, resolutionID, skillID) (first bool)`: true once per (session, resolution ID or `-`, skill).
2. Emit points (all best-effort; a telemetry failure never fails the request):
   - `skill_resolve` handler: `noteResolution` after a successful resolve.
   - `skill_get` (active skills only), `skills/get`, and `resources/read` of `SKILL.md`: `skill.loaded` with `resource_kind=entrypoint`, `surface`, `attribution`, `first_activation`, `basis=server-observed`, `ResolutionID`, `SessionIDHash`, plus the catalog snapshot.
   - `resources/read` of other files: `skill.loaded` with `resource_kind` from the existing path classification (`reference`, `script`, `asset`, `resource`), `first_activation=false`.
   - Repeated entrypoint loads in the same session and resolution emit events with `first_activation=false` (counted as loads, not activations).
3. `Server` gains `telemetry app.TelemetrySink` and `tracker *activationTracker`, both set in `New`/`Serve`; when no recorder could be opened, events are skipped and the tracker still runs (cheap).
4. Feedback correlation needs no MCP change: Phase 2 derives `after_load` from stored events by `resolution_id`.

## Files

Create:
- `internal/delivery/mcpserver/activation_tracker.go`
- `internal/delivery/mcpserver/activation_tracker_test.go`

Modify:
- `internal/delivery/mcpserver/server.go` (fields, wiring in `New`/`Serve`, emit in `getSkill`/`readResource`)
- `internal/delivery/mcpserver/resolver_tools.go` (`noteResolution`)
- `internal/delivery/mcpserver/skill_tools.go` (`skill_get` emit)
- `internal/delivery/mcpserver/server_test.go` or `subprocess_test.go` (end-to-end assertion over a real stdio session)

## Steps

1. Tracker with a fake clock; table tests for every attribution class, TTL expiry, LRU eviction, first-activation dedupe.
2. Wire emit points behind a small helper `recordLoad(ctx, session, skillID, kind, surface)`.
3. End-to-end test: start the server in-process with a temp workspace and a recorder; call `skill_resolve` then `skill_get` for the primary, `skill_get` for another skill, `resources/read` of a reference; flush the recorder and assert stored events and rollups (`activation:recommended`, `activation:override`, `load:reference`).

## Tests and validation

- `go test -race ./internal/delivery/mcpserver/`
- Assert no event payload contains a URI, path, or task text.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Curator `skill_get` calls on active skills inflate activations | Medium × Low | `surface` dimension separates `skill_get` from `skills_get`/`resources_read`; documented in Phase 13. |
| One stdio process serves several logical tasks, so a stale resolution attributes a later manual pick | Medium × Low | 2 h TTL and newest-first scan; `override` vs `unsolicited` remain distinguishable. |
| Memory growth | Low × Low | Hard caps (256 sessions × 32 resolutions). |
| Data race in handlers | Low × Medium | Single mutex; `-race` test. |

## Rollback

Revert; no persisted state beyond telemetry events.
