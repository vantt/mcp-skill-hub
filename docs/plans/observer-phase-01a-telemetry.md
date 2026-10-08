# Observer Phase 01a: Telemetry & Chain Metrics

**Date:** 2026-10-09  
**Branch:** `wave1/observer-telemetry`  
**Parent Plan:** [docs/plans/2026-10-08-observer-and-enrichment.md](2026-10-08-observer-and-enrichment.md) (Phase 1: O1, O2, O3 parts, O5, O6)  
**Related Design:** [docs/design/04-telemetry-reproducibility-evaluation.md](../design/04-telemetry-reproducibility-evaluation.md)

---

## 1. Goal

Implement the first wave of telemetry normalization and observable chain tracking:
1. Connect resolutions, re-resolutions, and loads via durable chains keyed by `(session_hash, resolution_id, event_id)` with verified prior attribution and normalized MCP client identity.
2. Enrich resolution events with `retrieval_candidate_count`, `channels`, and `stage_ms` without bumping `event_version` "1" or breaking backward compatibility.
3. Emit defined events: `clarification.requested`, `clarification.answered`, `evaluation.run_completed`, `catalog.changed`, and `index.rebuilt`.
4. Ensure `skill.loaded` events attribute to the resolution's catalog snapshot/policy and succeed even if catalog opening fails.
5. Standardize funnel metric definitions (chain unit, load attribution to newest resolution, separate `already_covered` bucket, offline `ignore_rate` and `true_no_skill`) with raw counts alongside every rate, support cuts (`--by client|operation|snapshot`), and introduce `skillhub telemetry chains` for disagreement triage.
6. Extend raw event retention to 30 days to cover baseline windows, and provide web UI visibility without leaking task text.

---

## 2. Owned Paths & Boundaries

### Owned paths
- `internal/telemetry/**`
- `internal/app/{resolver,usage,curation_telemetry,skill_load_telemetry,telemetry,feedback,routing_eval,transcript_import,catalog}.go`
- `internal/resolver` (emit observability metadata only, no ranking or scoring changes)
- `internal/delivery/mcpserver/{activation_tracker,resolver_tools}.go` and telemetry wiring in `server.go` (`recordLoad`, `callerContext`)
- `internal/delivery/cli/telemetry*.go`
- `internal/delivery/web/routes_usage*.go`, golden files under `internal/delivery/web/testdata/golden/`, `web/src/api/types.ts`, `web/src/screens/skill-detail/UsagePanel*.tsx`
- `schemas/telemetry-event-v1.schema.json`
- `docs/design/04-telemetry-reproducibility-evaluation.md`

### Forbidden paths (belonging to Worktree B or later waves)
- `internal/{workspace,canonical,catalog,skillruntime,migration,hostintegration,systemskills}`
- `system-skills/`
- `app/{distribution,skill_discovery,source_import,upstream_*,skill_snapshot,skill_review*,skill_add,insight,operations}.go`
- `docs/design/07`
- `mcpserver` `registerTools`, `insight_tools.go`

---

## 3. Detailed Steps

### Phase 1: Plan & Contracts
- Write this implementation plan.
- Update `schemas/telemetry-event-v1.schema.json`:
  - Add `retrieval_candidate_count` (`{"$ref": "#/$defs/count"}`)
  - Add `prior_resolution_id` (`{"$ref": "#/$defs/token"}`)
  - Add `prior_kind` (`{"$ref": "#/$defs/token"}`)
  - Add `prior_verified` (`{"type": "boolean"}`)
- Update `docs/design/04`:
  - Document actual meaning of `candidate_count` vs `retrieval_candidate_count` in §3.3.
  - Document the metric definition table in §9.1 (chain unit, offline ignore/true_no_skill, already_covered bucket).

### Phase 2: Event Allowlist & Retention
- In `internal/telemetry/events.go`:
  - Add `retrieval_candidate_count` (kindCount), `prior_resolution_id` (kindToken), `prior_kind` (kindToken), `prior_verified` (kindBool) to `commonRouting`.
- In `internal/telemetry/recorder.go`:
  - Raise `defaultRetention` from 14 days to 30 days (`30 * 24 * time.Hour`).
  - Add `RawEvents(ctx context.Context, since, until string)` via `opRawEvents` actor request.
- Ensure all previously stored events continue to pass `validateStored`.

### Phase 3: CallerContext & MCP Session Attribution
- In `internal/app/resolver.go` (or `internal/app/telemetry.go`):
  - Define `CallerContext{SessionHash string, Client telemetry.Client, PriorVerifier func(string) bool}`.
  - Provide `WithCallerContext(ctx, caller)` and `CallerFromContext(ctx)`.
- In `internal/delivery/mcpserver/activation_tracker.go`:
  - Store `catalogSnapshot`, `policyRevision`, `client` in `notedResolution`.
  - Add `hasResolution(session *mcp.ServerSession, resolutionID string) bool`.
  - Return resolution snapshot/policy/client from `attribute(...)`.
- In `internal/delivery/mcpserver/resolver_tools.go`:
  - Normalize MCP `clientInfo` to enum `claude-code | codex | cursor | gemini | other`, version at most `major.minor`. Unknown/odd name maps to `other`.
  - Populate `CallerContext` into `ctx` for `skill_resolve` and `skill_feedback`.
  - Verify `prior_verified` against `tracker.hasResolution`.

### Phase 4: Event Emission & Load Resilience
- In `internal/resolver`:
  - Track `Channels` (`fts`, `rules`), `StageMS` (`validation`, `retrieval`, `scoring`, `total`), and `RetrievalCandidateCount` on `Response` (`json:"-"`).
- In `internal/app/resolver.go`:
  - Set `SessionIDHash` and `Client` from `CallerContext`.
  - Populate `prior_resolution_id`, `prior_kind`, `prior_verified` in payload.
  - Emit `channels`, `stage_ms`, and `retrieval_candidate_count`.
  - Emit `clarification.requested` when `response.Status == StatusNeedsContext`.
  - Emit `clarification.answered` when `request.Prior.Kind == "clarification"`.
- In `internal/app/routing_eval.go`:
  - Emit `evaluation.run_completed` at the end of `EvaluateRouting`.
- In `internal/app/catalog.go`:
  - Emit `catalog.changed` and `index.rebuilt` when `BuildCatalogGenerationWithProgress` succeeds.
- In `internal/app/curation_telemetry.go` & `skill_load_telemetry.go`:
  - When `event.CatalogSnapshot` and `event.PolicyRevision` are already populated from resolution, emit directly without re-opening or failing if catalog is unopenable.

### Phase 5: Chain Metrics & Funnel Cuts
- In `internal/app/usage.go`:
  - Implement chain reconstruction: group by session hash, link resolutions via verified `prior_resolution_id`, attribute loads to newest resolution in chain.
  - Compute O5 metrics: `acceptance_rate`, `override_rate`, `false_no_skill_rate`, `true_no_skill`, `reformulation_rate`, `ignore_rate`, `needs_context_answer_rate`, `bypass_rate`, `negative_after_load`.
  - Compute `ignore_rate` and `true_no_skill` offline (no load within resolution TTL 2h; report "unknown" when indeterminate).
  - Add cuts: `--by client|operation|snapshot`.
  - State first-valid day per metric and show raw counts (`numerator`, `denominator`) beside every rate. All rates in [0, 1].
- In `internal/delivery/cli`:
  - Update `skillhub telemetry funnel` with `--by` cuts and O5 output.
  - Add `skillhub telemetry chains --since 7d [--kind override|after_no_skill|reformulation] [--workspace <path>] [--json]` listing disagreement chains.

### Phase 6: Web UI & Golden Tests
- Update `web/src/api/types.ts` and `UsagePanel.tsx` to handle new rate/count structures.
- Update web golden test `TestReadEndpointsGolden` using `-update`.
- Update CLI funnel tests.
- Run `make check`.

---

## 4. Required Tests

1. **Client Normalization:** Unknown or garbled `clientInfo` still records the event with client name `other` and does not reject the event.
2. **Prior Verification:** Forged `prior.kind: rejected` (ID not issued to this session) records `prior_verified=false` and does not count in `reformulation_rate`.
3. **Chain Isolation:** Two distinct sessions sending the identical task form two separate chains.
4. **Rate Bounds:** Every rate stays strictly within $[0, 1]$, or nil/N/A when denominator is 0.
5. **Storage Backward Compatibility:** Events stored before this change remain readable by export, feedback, and promotion; `event_version` remains "1"; old rows pass `validateStored`.
6. **Snapshot Attribution:** A skill load occurring after `catalog.changed` remains attributed to the resolution's catalog snapshot.
7. **Golden Tests:** Funnel CLI and web golden tests updated deliberately with `-update`, not deleted.
8. **Make Check:** Full `make check` (vet, lint, test) passes cleanly with 0 issues.
