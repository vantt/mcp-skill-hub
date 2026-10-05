# Phase 6: Server-side activation tracking report

## Summary
- Implemented `activationTracker` (`internal/delivery/mcpserver/activation_tracker.go`) with per-session state mapping `*mcp.ServerSession -> *sessionState`, LRU eviction at 256 sessions, 32 resolutions cap, 2-hour TTL, and random 16-byte session hash generation.
- Added attribution logic: `recommended`, `supporting`, `override`, `after_no_skill`, `after_needs_context`, and `unsolicited`.
- Added deduplicated activation marking (`markActivation`) per `(session, resolution_id, skill_id)` for non-blocked entrypoint loads.
- Implemented `app.RecordSkillLoad` (`internal/app/skill_load_telemetry.go`) to build and record `skill.loaded` events with catalog snapshot and policy revision pins, strictly excluding task descriptions, URIs, and filesystem paths.
- Updated MCP server hooks (`server.go`, `resolver_tools.go`, `skill_tools.go`):
  - `skill_resolve` records resolution in tracker via `noteResolution`.
  - `skill_get`, `skills/get`, and `resources/read` record `skill.loaded` events with server-observed basis.
  - Active unapproved third-party skills return `review_required` and emit blocked loads with `status: review_required` and corresponding reason codes; activation is not marked.
  - Non-active skills and system curator emit no load events.
- Updated telemetry semantic validation and rollups (`events.go`, `rollup.go`, `feedback.go`, schema):
  - Server-observed `skill.loaded` events allow `status: review_required` and forbid `first_activation: true`.
  - Rollup keys count `blocked:review_required` for blocked loads and ignore them for activation counts.
  - Feedback calculation ignores blocked loads for `after_load` determination.
  - Schema enforces `status: review_required` when status is present on server-observed loads.

## Verification
- Unit & race tests:
  - `go test -race -count=1 -run Tracker ./internal/delivery/mcpserver/` (PASS)
  - `go test -count=1 ./internal/telemetry/ ./schemas/` (PASS)
  - `go test -count=1 -run SkillLoad ./internal/app/` (PASS)
  - `go test -race -count=1 -run 'Activation|Telemetry' ./internal/delivery/mcpserver/` (PASS)
  - `go test -race -count=1 ./internal/delivery/mcpserver/` (PASS)
- Privacy & acceptance checks in `TestActivationTelemetryEndToEnd`:
  - Verified `load:entrypoint`, `activation:recommended`, `load:reference`, `blocked:review_required` counts.
  - Verified draft skill produces no events or rollups.
  - Verified raw `telemetry.db` bytes contain no `skill://`, workspace paths, or task text.
- Full gate:
  - `make check` (vet, golangci-lint, go test) passes cleanly with 0 issues.
