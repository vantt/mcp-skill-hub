# Phase 2: Mine routing suggestions

## Context

Plan: [plan.md](plan.md), sections "Outcome" (the signals) and "Design in one page". This phase adds a read-only
app service that joins case-journal rows (redacted text) with raw events (loads, rejections) and the current
catalog. It returns suggestions with evidence. It writes nothing except the dismissal file, and only on an
explicit dismiss.

## Requirements

### Inputs

- `Cases(since, "")` from the recorder, for case kinds `after_no_skill`, `after_needs_context`, `override` and
  `verified_reformulation`. Kinds `rejected`, `needs_context_resolved` and `repeated_gap` are ignored. A
  `repeated_gap` has no target skill, and a new skill would be a non-goal.
- `RawEvents(since, today)` gives the loads per `resolution_id`: server-observed `skill.loaded`, not blocked
  (reuse `parseChainLoad` from `usage_chains.go`). It also gives `activation.rejected` skill IDs per resolution,
  as extra evidence.
- The current catalog view gives the active skills and their routing (`examples`, `counter_examples`, `not_for`).
- The default window is 30 days (the raw-event default retention), set with `Since`.

### Signals

| Signal | Case | Target, field, text | Needs |
|---|---|---|---|
| `missed_then_chosen` | `after_no_skill`, `after_needs_context` | `chosen`, `examples`, `task.description` | — |
| `rejected_then_other` (X side) | `verified_reformulation` with `resolver.prior_primary` = X | X, `counter_examples`, `task.prior_description` | the phase 1 fields |
| `rejected_then_other` (Y side) | same case, Y = `chosen` | Y, `examples`, `task.prior_description` | raw `skill.loaded` of Y on `case.resolution_id`, or a load case of Y on it |
| `rejected_then_other` | `override` with `resolver.primary` = X, `chosen` = Y | X `counter_examples` and Y `examples`, `task.description` | phase 1 `resolver.primary` |
| `repeated_rejection` | at least 3 X-side events for X from at least 2 sessions whose phrasings overlap by at least 0.5 (single-link) | X, `not_for`, the shared terms of the cluster in medoid order (at least 2 tokens), else the medoid phrasing | — |

- X and Y must differ. If X equals Y, there is no suggestion.
- Overlap uses the resolver's own `tokenize` and `overlap` (`internal/resolver/evidence.go:164,200`, both
  unexported today). Export one small production function, `PhraseOverlap(a, b string) float64`, that wraps them.
  Phase 3's overlay uses it too. Do not reimplement tokenization.
- A `repeated_rejection` suggestion **replaces** the `counter_examples` suggestions of its cluster members, and
  lists them as its evidence.

### Filters

Each filter has a named skip reason that is counted in the report:

- `no_text`: the phrasing is empty or the phase 1 field is missing.
- `redacted`: the text contains any redaction marker. Add `telemetry.ContainsRedaction(text) bool` next to the
  redactor, driven by the same placeholder list the redactor writes: `[PRIVATE KEY]`, `[CREDS]`, `[REDACTED]`,
  `[JWT]`, `[AWS KEY]`, `[GITHUB TOKEN]`, `[SLACK TOKEN]`, `[TOKEN]`, `[EMAIL]`, `<repo>`, and a `~`-prefixed home
  path. The list is defined once, in `redactor.go` (DRY), and the redactor test asserts that every marker it emits
  is in the list.
- `unsafe_characters`: after whitespace collapse, any Unicode control character or bidi/format character (Cc, Cf)
  remains.
- `too_long`: more than 300 runes (the canonical limit, which the redactor's "..." truncation always exceeds).
- `skill_unavailable`: the target is missing or not `active`.
- `already_present`: the normalized text is already in the target field.
- `conflict`: the text is in a contradicting field of the same skill (`examples` against `counter_examples` or
  `not_for`). The text is listed in the report, not suggested.
- `field_full`: the target `examples` or `counter_examples` list already has 10 entries.
- `dismissed`: the ID is in the dismissal file.
- `below_threshold`: fewer events than `MinEvents` (default 1 for `examples`/`counter_examples`; `not_for` uses its
  own fixed rule above).

### Grouping and evidence

- Group by suggestion ID (skill, field, normalized text). Per suggestion, the evidence holds: `events` (count),
  `sessions` (distinct session hashes, count only), `first_seen` and `last_seen` (UTC dates), `kinds` (count per
  case kind), `other_skill` (Y for an X-side suggestion, X for a Y-side one), `rejected_feedback` (count of
  matching `activation.rejected`), `clients` (distinct client names), `resolution_ids` and `case_ids` (at most 10
  each, newest first, for `eval promote` and `telemetry cases`), and the `reason` (a fixed sentence per signal,
  with skill IDs and counts filled in, and never the text).
- Session hashes are never output.
- Order: events descending, then sessions descending, then `last_seen` descending, then ID.
- The report holds `case_journal_enabled`, `window` (from and to), `raw_events_since` (the oldest raw event, so
  missing Y evidence can be explained), `suggestions`, and `skipped` (count per reason).

### Dismissals

- `runtime/routing_suggestions.json` holds `{"version":1,"dismissed":{"<id>":{"at":"<RFC3339>","skill_id":"..","field":".."}}}`.
  It is written through an atomic temp-plus-rename with mode 0600, under `os.OpenRoot` (as `proposal_store.go`
  does), rejecting symlinks. It is capped at 1000 entries, dropping the oldest.
- `Dismiss(ctx, path, id)` accepts only IDs that match `^rs_[0-9a-f]{20}$`. Dismissing an ID that is not currently
  suggested is allowed and idempotent, so a curator can dismiss from a saved listing.
- `telemetry purge` does not clear dismissals, because it purges telemetry only. Deleting the file restores
  every dismissed suggestion.

## Files

| Path | Action | Change |
|---|---|---|
| `internal/app/routing_suggestions.go` | create | `RoutingSuggestionService` (fields `Telemetry TelemetryService`, `Clock`), `RoutingSuggestionQuery{Since time.Time; SkillID string; MinEvents int}`, `RoutingSuggestion`, `RoutingSuggestionEvidence`, `RoutingSuggestionReport`, `Suggest`, `Dismiss`, and small functions per signal and filter |
| `internal/app/routing_suggestion_dismissals.go` | create | Read and write the dismissal file (kept separate because it is the only writer) |
| `internal/telemetry/redactor.go` | modify | The marker list and `ContainsRedaction` |
| `internal/telemetry/redactor_test.go` | modify | Every emitted marker is detected (extend the existing table) |
| `internal/resolver/evidence.go` | modify | Export `PhraseOverlap` over the existing `tokenize` and `overlap` |
| `internal/app/routing_suggestions_test.go` | create | The owner table test (see Validation) |

## Steps

1. Add the redaction marker list and `ContainsRedaction`, and extend the redactor test.
2. Export `PhraseOverlap`.
3. Write the service: load cases, then raw-event indexes, then catalog skills, then signal extraction, then
   filters, then grouping, then clustering for `not_for`, then ordering.
4. Write the dismissal store.
5. Write the table test. Seed through `TelemetryService.Open(root)`, using `RecordCase` with the case journal
   enabled through `telemetry.SetCaseJournalEnabled` on the temp root, and `Record` for `skill.loaded` and
   `activation.rejected` events. Use a temp workspace with 3 or more active skills (reuse the helpers that
   `routing_eval_test.go` uses).

## Validation

```bash
go test ./internal/telemetry/ -run 'TestRedact' -count=1
go test ./internal/app/ -run 'TestRoutingSuggestions' -count=1
go test ./internal/app/ ./internal/telemetry/ ./internal/resolver/ -count=1
make check
```

`TestRoutingSuggestions` is one table, and each row is one contract:

- An after-no-skill load gives an X `examples` suggestion with full evidence fields.
- A verified reformulation plus a Y load gives an X `counter_examples` and a Y `examples` suggestion with the
  original phrasing. Without the Y load, there is only X.
- An override gives both sides.
- Three overlapping X-side events from 2 sessions give one `not_for` suggestion and no member `counter_examples`
  suggestions. The same events from one session give no `not_for` suggestion.
- One row per skip reason, asserting the `skipped` count and the absence of the suggestion.
- The case journal disabled gives `case_journal_enabled: false` and no suggestions.
- Dismiss, then suggest, means the ID is gone. A second dismiss is idempotent. A malformed ID is refused.
- The suggestion ID is stable for the same (skill, field, text) across runs.

## Risks

- **Clustering quality.** Single-link clustering at 0.5 overlap is crude. A human confirms the result, and the
  text and members are shown. Thresholds are open question 5.
- **Cost.** Cases are at most 500 and raw events cover 30 days, so this is linear and cheap. No caching is added.
- **Catalog view staleness.** The current published generation is used. Suggestions for skills edited since the
  case was recorded are filtered by `already_present` or `conflict`.

## Rollback

Revert the commit. The service is read-only apart from the dismissal file, and
`runtime/routing_suggestions.json` can be deleted safely.
