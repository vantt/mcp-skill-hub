# Phase 3: Preview a suggestion with the routing check

## Context

Plan: [plan.md](plan.md), sections "Promotion and the mutation flow" and "Findability check". This phase turns
one suggestion into an ordinary skill edit proposal, through `PreviewSkillUpdate`, so that confirm, pins, expiry,
receipts and Git behaviour are the existing ones. It also fills `RoutingImpact` through the existing
`skill.Manager.RoutingHook` extension point with a what-if routing check.

## Requirements

### Preview

- `SkillService.PreviewRoutingSuggestion(ctx, path, suggestionID string, fullDiff bool) (RoutingSuggestionProposal, error)`:
  1. Validate the ID format.
  2. Re-run `RoutingSuggestionService.Suggest` with the default query and find the ID. If it is not found, return
     an `invalid_request` app error. Why: "Suggestion <id> is not current: its evidence expired, it was applied or
     dismissed, or the skill changed." Fix: "Run `skillhub telemetry cases suggest`."
  3. Read the current routing (`ReadSkillRouting`) and append the text to the field, so the existing lists are
     kept and `mergeRouting` replaces only that list.
  4. Call `PreviewSkillUpdate` with `fullDiff` forced to true (the human must see the exact text in the diff),
     using `SkillService{Manager: skill.Manager{RoutingHook: routingWhatIfHook{...}}}`.
  5. Return `RoutingSuggestionProposal{SkillProposal, Suggestion RoutingSuggestion}`. The suggestion carries the
     reason and the evidence for display. The stored proposal file is the normal one.
- Nothing is written to the skill. The only writes are the stored proposal under `runtime/proposals/`, as for any
  edit preview.
- Confirming is unchanged: `ConfirmProposal` / `ConfirmSkillMutation` with pins. No new confirm path and no new
  proposal kind.
- `Rationale` is not set. Telemetry evidence is not copied into Git; only the routing entry is.

### What-if routing check (`routingWhatIfHook`)

- It implements `skill.RoutingImpactHook`. It is read-only and deterministic for the base snapshot.
- If the current catalog pointer's snapshot differs from `baseSnapshot`, it returns a summary saying the check was
  skipped because the catalog changed, with no warnings. The preview itself is still valid, and its pins catch
  staleness at confirm.
- It parses the before and after routing of the target skill and computes the added entries per field.
- It runs the positive and counter cases of every active skill twice: once on the baseline catalog, and once with
  an overlay decorator that:
  - replaces the target's `Examples`, `CounterExamples` and `NotFor` in `Skills()`;
  - in `Search()`, appends the target as a hit when the query overlaps an **added example** by at least the same
    0.5 threshold and the target is not already a hit. This over-reports candidacy, which is the conservative
    direction for detecting a skill that steals other skills' examples.
- It also resolves the suggestion's own phrase on both catalogs (the same request shape as a positive case of the
  target skill).
- Output:
  - `Summary`: "Routing check (approximate): N regressions across M cases; the phrase resolves to <before> → <after>.
    Run `skillhub eval routing` after confirming for the exact result."
  - `Warnings`: one line per case that passed on the baseline and fails with the overlay ("<skill> example #i now
    resolves to <primary|status>"), at most 10, then "and K more".
  - When the added entry is an `example`, a warning if the phrase still does not resolve to the target. When it is
    a `counter_example` or `not_for`, a warning if the phrase still resolves to the target.
- **Budget:** if the active skills hold more than 600 positive and counter cases in total, skip with a summary that
  names `skillhub eval routing`. Measure the real time once by hand, on a throwaway temp workspace with 50 active
  skills of 10 examples each (not committed), and record it in the commit message body. If it exceeds 3 s, lower
  the budget and say so in the commit message.

### Shared code (no duplication)

- Refactor `evaluatePositiveCases` and `evaluateCounterCases` (`internal/app/routing_eval.go`) to take a base
  decorator that composes under the leave-one-out decorator, so `EvaluateRouting` and the hook share one
  implementation. `EvaluateRouting` passes the identity, so its behaviour is unchanged.
- Expose from those functions a per-case pass/fail record (skill, kind, index, got status and primary), which is
  the existing `RoutingEvalCaseFailure` plus a pass set, so the hook can diff baseline against overlay.
- Keep functions within 120 lines and 6 arguments: bundle the inputs in a small struct if needed.

## Files

| Path | Action | Change |
|---|---|---|
| `internal/app/routing_eval.go` | modify | Add the base decorator parameter and per-case results; behaviour unchanged |
| `internal/app/routing_whatif.go` | create | `routingWhatIfHook`, the overlay catalog, and the diffing |
| `internal/app/routing_suggestion_preview.go` | create | `PreviewRoutingSuggestion` and `RoutingSuggestionProposal` |
| `internal/app/routing_suggestion_preview_test.go` | create | The owner test of the preview and confirm path |
| `internal/app/routing_eval_test.go` | unchanged | Must stay green (`TestRoutingEvalGate` too) |

## Steps

1. Refactor `routing_eval.go` (composition plus results). Run the existing routing eval tests before going on.
2. Write the overlay catalog and the hook, and the summary and warning rendering.
3. Write `PreviewRoutingSuggestion`.
4. Write the tests.

## Validation

```bash
go test ./internal/app/ -run 'TestRoutingEval' -count=1
go test ./internal/app/ -run 'TestPreviewRoutingSuggestion' -count=1
go test ./internal/app/ -count=1
make check
```

`TestPreviewRoutingSuggestion` is one table over a temp workspace seeded as in phase 2:

- The preview of an `examples` suggestion returns pins, a full diff containing exactly the added line, and a
  `RoutingImpact.Summary` that starts with "Routing check (approximate)". The skill's files are byte-identical
  before confirm.
- `ConfirmProposal` with those pins applies the edit. `.meta/skill.yaml` now holds the entry, `git status` is
  dirty, and the next `Suggest` no longer lists the ID (`already_present`).
- A constructed suggestion whose phrase equals another skill's example yields a regression warning naming that
  skill.
- An unknown or dismissed ID returns the `invalid_request` error, and nothing is stored.
- If the catalog changed between preview and hook (simulate with a pointer snapshot mismatch through a real
  rebuild in the test), the check is skipped with that summary.

## Risks

- **The overlay approximation misses or invents flips.** It is labelled approximate, the conservative direction
  is chosen for stealing, and exact checking is open question 3.
- **The `routing_eval.go` refactor changes gate numbers.** The existing tests and `TestRoutingEvalGate` must pass
  unchanged. The refactor must not alter case order or requests.
- **Latency of the preview.** The budget and skip rule are above. The check runs only for suggestion previews, not
  for every `skill edit`.

## Rollback

Revert the commit. `routing_eval.go` returns to its single-decorator form, and stored proposals made by the
feature expire within 24 hours or can be confirmed as normal edits.
