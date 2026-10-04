---
phase: 2
title: "Telemetry events and daily rollups"
status: completed
priority: P1
effort: 8h
dependencies: []
---

# Phase 2: Telemetry events and daily rollups

## Context

- [plan.md](./plan.md) D1, D6; design `docs/design/04-telemetry-reproducibility-evaluation.md` §3.2–3.4, §4.2.
- Event allowlist: `internal/telemetry/events.go` (`eventPayloads`, `skillFields`, `commonRouting`); published schema `schemas/telemetry-event-v1.schema.json`.
- Three raw insert sites must all feed rollups: `internal/telemetry/store.go:463` (batched recorder writes, followed by the 14-day `DELETE` at `:467`), `internal/telemetry/feedback.go:304`, `internal/telemetry/curation_session.go:176`.
- Feedback events reuse the source resolution envelope (`feedbackResolutionSource`, `feedback.go:155`) and map outcomes to `skill.*` event types (`buildFeedbackEvents`, `:221`).

## Requirements

1. Payload allowlist additions (all `content_mode none`, tokens only):
   - `skillFields()`: `resource_kind` (entrypoint|reference|script|asset|resource), `surface` (skill_get|skills_get|resources_read), `attribution` (recommended|supporting|override|after_no_skill|after_needs_context|unsolicited), `first_activation` (bool), `after_load` (bool).
   - `commonRouting`: `setup_state` (ready|setup_required|unsupported_platform|unknown).
   - New `EventSkillDoctorChecked = "skill.doctor_checked"`: `skill_id`, `status` (ready|setup_required|unsupported_platform|failed), `reason_codes`, `duration_ms`.
   - New `EventTranscriptToolObserved = "transcript.tool_observed"`: `tool`, `skill_id`, `source` (claude-code), `basis` (transcript), `resolved_before` (bool).
2. Table, created at open with `CREATE TABLE IF NOT EXISTS`:
   ```sql
   CREATE TABLE telemetry_daily_rollups (
     day TEXT NOT NULL,          -- YYYY-MM-DD, UTC, from occurred_at
     skill_id TEXT NOT NULL,     -- '' for overall metrics
     metric TEXT NOT NULL,
     count INTEGER NOT NULL CHECK (count >= 0),
     PRIMARY KEY (day, skill_id, metric)
   ) STRICT;
   ```
3. One helper `insertEvent(ctx, tx, envelope storedEnvelope, insertSQL string) (inserted bool, err error)` used by all three sites. It inserts the raw row and, only when `RowsAffected()==1`, upserts `count = count + 1` for each key from `rollupKeys(envelope, tx)`.
4. `rollupKeys` mapping (pure function plus at most one indexed lookup):

   | Event | skill_id | metric |
   |---|---|---|
   | `resolution.completed` | '' | `resolution:<status>` |
   | `resolution.completed` with `setup_state` and `top_skill_id` | top skill | `setup:<state>` |
   | `resolution.failed` | '' | `resolution:failed` |
   | `resolution.recommended` | top skill | `recommended:primary` |
   | `resolution.recommended` | each other ID in `recommended_skill_ids` | `recommended:supporting` |
   | `skill.loaded`, `basis=server-observed` | skill | `load:<resource_kind>`; plus `activation:<attribution>` when `first_activation` |
   | `skill.*` from feedback | payload skill, else the source resolution's top skill | `feedback:<status>`; plus `feedback:negative` when status ∈ {failed, rejected, abandoned}; plus `feedback:negative_after_load` when `after_load` |
   | `skill.utility_reported` with `harmful` | skill | `feedback:negative` (and `_after_load` when flagged) |
   | `skill.doctor_checked` | skill | `doctor:<status>` |
   | `transcript.tool_observed` | skill or '' | `transcript:<tool>`; for `tool=Skill` also `native:resolved_before` or `native:no_resolve` |
5. Feedback: inside the feedback transaction, set `after_load=true` when a `skill.loaded` row with `basis=server-observed` exists for the same `resolution_id` (`json_extract(payload_json,'$.payload.basis')`).
6. Retention: in the batched write transaction, after the raw cutoff delete, `DELETE FROM telemetry_daily_rollups WHERE day < ?` with a 180-day cutoff (`defaultRollupRetention`, `Config.RollupRetention` override for tests). `Purge` already recreates the store, which clears rollups; add a test.
7. Rollups are excluded from `Export`/`Preview` (they are aggregates, not events). `trimLogicalSize` keeps measuring raw events only.
8. Read API: `func (r *Recorder) Rollups(ctx, from, to string) ([]RollupRow, error)` via a new admin op, mirroring `Preview`.

## Files

Create:
- `internal/telemetry/rollup.go` (table DDL, `insertEvent`, `rollupKeys`, retention, `RollupRow`, read query)
- `internal/telemetry/rollup_test.go`

Modify:
- `internal/telemetry/events.go` (constants, allowlist)
- `internal/telemetry/store.go` (schema init calls rollup DDL; use `insertEvent`; rollup retention)
- `internal/telemetry/feedback.go` (use `insertEvent`; `after_load` flag)
- `internal/telemetry/curation_session.go` (use `insertEvent`)
- `internal/telemetry/recorder.go` (`RollupRetention` config default, `opRollups`, `Rollups` method)
- `schemas/telemetry-event-v1.schema.json` (new event types and payload fields)
- `internal/telemetry/telemetry_test.go` only where existing assertions enumerate event types

## Steps

- [x] 1. Add constants and allowlist entries; update the JSON schema so the existing schema-parity test passes.
- [x] 2. Write `rollup.go`; replace the three raw `ExecContext` inserts with `insertEvent`.
- [x] 3. Add retention and the read op.
- [x] 4. Tests (below).

## Implementation status

Completed 2026-10-04. Notes:
- `after_load` is also allowlisted on `task.outcome_reported` and `skill.utility_reported`, because feedback outcomes `completed`/`failed` and utility reports map to those types and the rollup table needs the flag on them.
- `after_load` is server-derived, so feedback idempotency ignores it; a host retry after a later server-observed load stays deduplicated.
- Closed vocabularies (`setup_state`, `resource_kind`, `surface`, `attribution`, doctor `status`, transcript `source`/`basis`) are enforced in Go and in the schema so metric names stay bounded. A server-observed `skill.loaded` requires `skill_id` and `resource_kind`, plus `attribution` when `first_activation` is true.
- Rollup pruning also runs in the shared maintenance pass (flush, health, close, reads), not only the batched write.
- `Rollups` validates the day range before queueing, so a bad caller range does not degrade health.

## Tests and validation

- `go test ./internal/telemetry/ ./schemas/`
- Rollup tests: one inserted event increments once; a duplicate event ID (INSERT OR IGNORE) does not increment; events older than 14 days are pruned from raw but stay in rollups; rollups older than 180 days are pruned; purge clears rollups; feedback with and without a prior server-observed load sets `after_load` correctly; skill attribution falls back to the source resolution's top skill.
- Privacy test: a payload with an unknown key or a non-token value is still rejected for the new event types.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Rollup double counting on replays | Low × Medium | Increment only on `RowsAffected()==1`; deterministic IDs for transcript events (Phase 8). |
| Write-path latency | Low × Low | Upserts on a primary key inside the existing transaction; at most one indexed lookup per feedback event. |
| Schema drift between Go allowlist and JSON schema | Medium × Low | Existing parity test plus new cases. |

## Rollback

Revert. The leftover table is ignored by older binaries; `skillhub telemetry purge` removes it.
