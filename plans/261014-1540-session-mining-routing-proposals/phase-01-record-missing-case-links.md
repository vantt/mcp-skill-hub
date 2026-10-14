# Phase 1: Record the missing case links

## Context

Plan: [plan.md](plan.md), section "Linking a sequence". The case journal is the only store that holds task text.
It lacks three links the suggestions need:

- **A.** On `verified_reformulation` cases, the rejected skill X and the original phrasing are missing.
- **B.** On `override` load cases, the recommended primary X is missing.
- **C.** `after_needs_context` load cases are never written.

Everything added here lives in the case journal (`payload_json` of `telemetry_cases`). It is redacted by the same
`telemetry.Redactor`, under the same opt-in, retention and caps. Telemetry events and their schema do not change.

## Requirements

- When a case of kind `verified_reformulation` is recorded, add:
  - `resolver.prior_primary`: the primary skill ID of the prior resolution. It is empty when the prior was
    `no_skill`.
  - `task.prior_description`: the prior resolution's task description, already redacted by the tracker.
  Both come from the session tracker entry of the verified prior. They are added only when the prior is verified
  and still in the tracker (2-hour TTL). Otherwise the fields are omitted, never guessed.
- `rejected` cases (unverified prior) do not gain these fields. A prior that cannot be verified may name another
  session's resolution, so its data is never copied.
- Load cases (`override`, `after_no_skill`, and the new `after_needs_context`) add `resolver.primary`, the primary
  of the resolution the load follows. It is empty for `no_skill` and `needs_context`.
- Record a load case for attribution `after_needs_context` under exactly the same conditions as `after_no_skill`:
  the first activation, an entrypoint, not blocked, and a known resolution.
- Skill IDs pass `safeTelemetryToken` or are dropped. Descriptions are already redacted text, and must not be
  redacted twice in a way that changes the markers.
- No change to `CaseRecord`'s Go fields or the SQLite schema. The new keys go inside the existing `Task` and
  `Resolver` maps. Older readers ignore them.

## Files

| Path | Action | Change |
|---|---|---|
| `internal/app/caller_context.go` | modify | Add `PriorLookup func(resolutionID string) (PriorResolution, bool)` and a small `PriorResolution{PrimaryID, TaskDescription string}`, next to `PriorVerifier`. Add a nil-safe method `LookupPrior`. |
| `internal/delivery/mcpserver/session_telemetry.go` | modify | `callerContext` wires `PriorLookup` to `adapter.tracker.resolutionData(session, id)` (the same session-scoped tracker as the verifier). |
| `internal/app/resolver.go` | modify | In the case-recording block (around lines 80-111), add `prior_primary` and `prior_description` for `verified_reformulation`. |
| `internal/delivery/mcpserver/activation_tracker.go` | modify | `notedResolution.loadResolution()` carries `PrimaryID`. |
| `internal/app/skill_load_case.go` | modify | `LoadResolution.PrimaryID`, written as `resolver.primary`. |
| `internal/delivery/mcpserver/load_attribution.go` | modify | Include `after_needs_context` in the attribution check that fetches `resolutionData` and records the load case. |
| `internal/delivery/mcpserver/cases_matching_test.go` | modify | Extend the owner test (see Validation). |
| `docs/design/04-telemetry-reproducibility-evaluation.md` | modify | §4.1 case journal: list the case kinds and the fields, including the new ones, and state that they share the opt-in, redaction and caps. |

## Steps

1. Add `PriorResolution`, `PriorLookup` and `LookupPrior` to `CallerContext`. The CLI path leaves it nil, so it
   returns false.
2. Wire it in `callerContext`, only when `adapter.tracker != nil && session != nil` (mirror the verifier).
3. In `resolver.go`, when `kind == "verified_reformulation"`, call `LookupPrior(request.Prior.ResolutionID)` and set
   the two keys when found. Keep the block within the 120-line lint limit by extracting a helper that builds the
   case's `Task` and `Resolver` maps.
4. Add `PrimaryID` to `LoadResolution` and to `loadResolution()`, and write `resolver.primary` in
   `RecordLoadCase`.
5. Extend the attribution condition in `load_attribution.go` to `after_needs_context`.
6. Update the design doc paragraph.

## Validation

Narrowest first:

```bash
go test ./internal/delivery/mcpserver/ -run 'TestCasesMatchTelemetryEvents' -count=1
go test ./internal/app/ ./internal/telemetry/ ./internal/delivery/mcpserver/ ./internal/delivery/web/ -count=1
make check
```

Extend `TestCasesMatchTelemetryEvents` (the owner of "cases match the events they describe") with table rows,
not new near-duplicate tests:

- An override load case has `resolver.primary` equal to the recommended skill.
- A verified reformulation case has `resolver.prior_primary` equal to the first primary, and
  `task.prior_description` equal to the first phrasing. A `sk-...` token planted in the first phrasing shows up
  only as `[TOKEN]`.
- `needs_context` followed by a load writes an `after_needs_context` case.
- A `rejected` case whose prior belongs to another session carries neither new field.

`TestCasesIsolationWebAndExport` must stay green unchanged.

## Risks

- The tracker entry for the prior may have expired (2-hour TTL, 32 per session). The fields are then omitted, and
  phase 2 treats such a case as having no X phrasing. This is acceptable and is counted as a skip reason.
- More case rows (`after_needs_context`) count against the 50 per day cap. That is expected, and the cap is not
  changed.

## Rollback

Revert the commit. The new JSON keys in existing case rows are ignored by older code, and
`skillhub telemetry cases` or `telemetry purge` removes them if wanted. There is no migration.
