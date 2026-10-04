# Phase 2 report: telemetry events and daily rollups

Status: completed. Focused tests pass. Nothing is committed.

## Files
- Created `internal/telemetry/rollup.go`. It holds the table DDL, `insertEvent`, `rollupKeys`, `resolutionTopSkill`, `pruneRollups`, `RollupRow`, and the range read.
- Created `internal/telemetry/rollup_test.go`.
- Modified `internal/telemetry/events.go`. It adds the two new event types, the new allowlist fields, enum enforcement, and server-observed load semantics.
- Modified `internal/telemetry/store.go`. Rollup DDL now runs at open. `writeEvents` uses `insertEvent`. Rollup pruning runs in the write and maintenance passes.
- Modified `internal/telemetry/feedback.go`. It uses `insertEvent` and derives `after_load`. Dedupe comparison now ignores `after_load`.
- Modified `internal/telemetry/curation_session.go`. It uses `insertEvent`.
- Modified `internal/telemetry/recorder.go`. It adds `RollupRetention` (default 180 days, negative values rejected), `opRollups`, and `Recorder.Rollups`.
- Modified `schemas/telemetry-event-v1.schema.json`. It adds the new event types, the new fields, and their enums.
- Modified `schemas/embed_test.go`. The phase file names no schema test file, but this is the test file for the schema it does name. I appended one new test there.
- Updated the phase-02 file: status, checklist, and implementation notes.

`telemetry_test.go` did not need any changes.

## Deviations (also recorded in the phase file)
- `after_load` is allowlisted on `task.outcome_reported` and `skill.utility_reported`. The completed/failed outcomes and harmful utility reports map to those types, and the rollup table requires the flag on them.
- Feedback idempotency ignores the server-derived `after_load` field. Without that, a host retry made after a server-observed load would return `ErrFeedbackConflict`.
- Rollup pruning also runs in `maintainStore` (flush, health, close, and rollup reads), not only in the batched write.

## Tests
- `go vet ./internal/telemetry/ ./schemas/` is clean.
- `go test ./internal/telemetry/... ./schemas/...` passes, including with `-race` for telemetry.
- The dependent packages `internal/app`, `internal/delivery/mcpserver`, `internal/delivery/cli`, and `internal/catalog` pass.
- I did not run `make check`, as instructed.

## Notes for later phases
- A `skill.loaded` with `basis=server-observed` must carry `skill_id` and `resource_kind`, and also `attribution` when `first_activation=true`. Otherwise the event is rejected.
- Transcript events require `source=claude-code` and `basis=transcript`.
- The exported constants are `LoadBasisServerObserved`, `TranscriptSourceClaude`, and `TranscriptBasis`.
- When a harmful utility report comes with a failed, rejected, or abandoned outcome, `feedback:negative` is incremented twice, once per event. This follows the phase's mapping table. The funnel phase may want to account for it.
