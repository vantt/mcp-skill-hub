---
title: "Routing suggestions mined from telemetry"
description: "The hub reads its own case journal and usage events for routing misses and suggests examples, counter_examples and not_for entries for existing skills; every suggestion becomes an ordinary skill edit preview that a human confirms or dismisses."
status: pending
priority: P2
effort: 13h
branch: TBD (own worktree, see Execution)
tags: [feature, backend, telemetry, curation, routing]
blockedBy: []
blocks: []
created: 2026-10-14
---

# Routing suggestions mined from telemetry

Adopts distill lesson `session-mining-drafts-only` (`docs/distillery/lessons/skill-recommendation.yaml`, decision
`candidate`, final score 3). The source (meta_skill) mines sessions only into new draft skills. The host takes the
step the source lacks: it turns tasks that missed into routing-metadata suggestions for skills that already exist.

## Outcome, constraints, non-goals

- **Outcome.** A curator runs `skillhub telemetry cases suggest` and sees, per active skill, suggested additions
  to `routing.examples`, `routing.counter_examples` or `routing.not_for`. Each suggestion shows the text, the
  reason, and the supporting evidence (event counts, distinct sessions, first and last date, the kinds of event,
  the other skill involved, and resolution IDs). `skillhub skill edit <skill-id> --from-suggestion <id>` turns one
  suggestion into the normal skill edit preview (diff, pins, routing check). The curator then confirms it with
  `skillhub skill confirm <proposal-id>` (or `--yes`), or discards it with `skillhub telemetry cases dismiss <id>`.
  Nothing changes in a skill until that confirm.
- **Signals** (refined against the code, see "Verified context"):
  1. *Missed, then chosen.* A resolution ended `no_skill` or `needs_context` and the host then loaded skill X
     (load cases `after_no_skill`, `after_needs_context`). Suggest the task phrasing as an `examples` entry on X.
  2. *Rejected, then another skill used.* X was recommended, and either the host re-resolved with
     `prior.kind: rejected` (verified for that session) and then loaded Y, or the host loaded Y instead of X with
     no re-resolve (load case `override`). Suggest the original phrasing as a `counter_examples` entry on X and as
     an `examples` entry on Y. The Y half needs evidence that Y was loaded.
  3. *Repeatedly rejected for one kind of task.* At least 3 signal-2 events for X, from at least 2 sessions, whose
     phrasings overlap. Suggest one `not_for` entry on X. It replaces those events' `counter_examples` suggestions.
- **Constraints.**
  - No automatic edits. The only write path is the existing `PreviewSkillUpdate` → stored proposal →
    `ConfirmSkillMutation` flow (`internal/app/skill_lifecycle.go`). There is no new approval path.
  - Task phrasings come from agents and are untrusted data. They are taken only from the case journal, which
    already redacts them. Phrasings that still carry redaction markers are dropped, as are phrasings over the
    canonical 300-rune limit and phrasings that hold control or bidi characters. Text is shown quoted and is
    never interpreted. Only re-resolutions whose prior was verified for that session count.
  - Phrasings stay on the CLI, the only surface that shows case text today. There is no MCP and no web exposure
    (`TestCasesIsolationWebAndExport` guards export and web).
  - Recording changes are limited to the minimum missing links and stay inside the case journal, which is opt-in,
    redacted, kept 90 days, and capped at 50 per day and 500 in total. Event payloads keep `content_mode: none`.
  - Tests use temp workspaces and the package `TestMain` HOME/XDG isolation. They never touch the live hub
    (`/home/vantt/skill-hub`), the real HOME or a real `telemetry.db`.
  - No plan IDs, phase numbers or lesson keys go in code comments, test names or commit messages.
  - Follow `AGENTS.md` "Testing" (use the `test-audit` skill, one owner test per contract, no test-only exports)
    and the lint limits (functions of at most 120 lines and at most 6 arguments).
- **Non-goals.** No automatic edits to skills. No new skill drafts from sessions. No LLM in the loop. No cloud
  service. No other distill lessons: `findability-lint` stays a separate lint, `prune-proposals-human-approved`
  and `user-skill-preferences` are context only. No MCP tool and no web route for suggestions (open questions 1
  and 2). No change to `eval promote`.

## Verified context (2026-10-14, at `4198bb7`)

### What telemetry records today

| Store | What it keeps | Task text? | Retention | Source |
|---|---|---|---|---|
| `telemetry_events` (`runtime/telemetry.db`) | Envelope with `session_id_hash`, `request_id`, `resolution_id`, snapshot/policy pins, client, and an allowlisted payload: `status`, `top_skill_id`, `reason_codes`, `prior_resolution_id`, `prior_kind`, `prior_verified`, and for `skill.loaded` the `skill_id`, `attribution` and top-k | **No.** Every event has `content_mode: none` (`internal/app/telemetry.go` `Open`) | 30 days by default (`internal/telemetry/recorder.go:18` `defaultRetention`). The design doc says 14 days (`docs/design/04-...md` §4.2), which is a doc/code mismatch noted here and not fixed in this plan | `events.go`, `store.go`, `raw_events.go` |
| `telemetry_daily_rollups` | Counters only | No | 180 days | `rollup.go` |
| `telemetry_cases` (case journal) | `CaseRecord` (`internal/telemetry/cases.go`): kind, session hash, resolution ID, event ID, client, snapshot, `prior_verified`, `task.description`, operation, the redacted request (including `prior.resolution_id`/`kind`), resolver top-k, and `chosen` | **Yes, redacted** by `telemetry.Redactor` (secrets, tokens, emails, workspace/home paths, 2048-byte cap) | 90 days, 50 per day, 500 in total (`recorder.go:23-25`) | `cases.go`, `internal/app/resolver.go:58-112`, `internal/app/skill_load_case.go` |

The case journal is **opt-in** (`skillhub telemetry cases enable`, `runtime/case_journal.json` or
`SKILLHUB_CASE_JOURNAL=1`). Without it there is no task text, so there can be no phrasing suggestions. The feature
reports this instead of guessing.

Case kinds written today:

- From `skill_resolve` (`internal/app/resolver.go:60-78`): `rejected` (prior not verified), `verified_reformulation`
  (prior `rejected`, verified for this session), `needs_context_resolved`, and `repeated_gap`. `task.description`
  is the **new** phrasing, and `chosen` is the new primary.
- From the first activation of a load (`internal/delivery/mcpserver/load_attribution.go:58-69`): `override` and
  `after_no_skill`. `task.description` is the phrasing of the resolution the load follows, and `chosen` is the
  loaded skill.

### Linking a sequence

The links exist, but three of them are missing from the case journal:

- Events link by `resolution_id`, and re-resolutions link to their prior through `prior_resolution_id` with
  `prior_verified` (server-side session tracker, `internal/delivery/mcpserver/activation_tracker.go`). Loads are
  attributed to the newest matching resolution of the session (`attributeDetails`). `UsageService.Chains`
  (`internal/app/usage_chains.go`) already builds `override`, `after_no_skill` and `reformulation` chains from
  raw events, without text.
- **Gap A.** A `verified_reformulation` case holds the new phrasing and the new primary (Y). It does not hold the
  rejected skill X or the original phrasing, which is the phrasing that wrongly hit X. Both live in the in-memory
  tracker (`notedResolution.PrimaryID`, `TaskDescription`, already redacted) for 2 hours, and X is also in raw
  events for 30 days. The original phrasing is nowhere durable.
- **Gap B.** An `override` load case holds the loaded skill Y. It does not hold the recommended primary X, only
  top-k, whose first entry is not guaranteed to be the primary. The tracker has `PrimaryID`.
- **Gap C.** `after_needs_context` loads are attributed (`activation_tracker.go:233`), but no case is written,
  because `load_attribution.go:58` checks only `override` and `after_no_skill`.
- "Y was then used" after a verified reformulation is a `recommended` load on the new resolution, so no case is
  written. It is found in raw events (`skill.loaded` with that `resolution_id`), so it is visible for 30 days.
  That is enough because the mining window defaults to 30 days. After raw events expire, the Y half of the
  suggestion is withheld rather than guessed.
- Explicit `skill_feedback outcome: rejected` produces `activation.rejected` with `skill_id` (`feedback.go`) but no
  text. It is counted as extra evidence when it shares a resolution with a case, not as a trigger
  (open question 4).

Phase 1 closes gaps A to C inside the case journal with the same redactor, caps and retention, and nothing else.

### Promotion and the mutation flow

- `internal/telemetry/promotion.go` (`skillhub eval promote --resolution-id`) builds a sanitized,
  `review_required`, deliberately incomplete eval-case draft from one resolution's events. It never fills task
  text, because events have none. It is a precedent, not a reusable pipeline: it is per resolution, its output is
  an eval case rather than a skill edit, and it reads only events. This plan reuses its stance (allowlist,
  `review_required`, evidence IDs), and lists resolution IDs in the evidence so a curator can still promote one.
  `promotion.go` stays unchanged.
- Skill edits: `SkillService.PreviewSkillUpdate` with `skill.UpdateInput.Routing` (`internal/skill/lifecycle.go`
  `RoutingInput`, merged by `mergeRouting`, which replaces a list that is supplied and keeps keys that are not)
  stores a pinned proposal in `runtime/proposals/<id>.json` for 24 hours (`proposal_store.go`). It is confirmed
  by `skillhub skill confirm <proposal-id>`, by `--yes`, by MCP `skill_update_confirm`, or by web
  `POST /api/v1/skills/proposals/{id}/confirm`, all through `ConfirmSkillMutation` with `ConfirmationPins`.
- `skill.Manager.RoutingHook` (`RoutingImpactHook`, which runs during preview only and must be read-only and
  deterministic) exists for "a later evaluator" that attaches measured routing deltas. Today only
  `BasicRoutingImpactHook` runs. The preview shows `RoutingImpact.Summary` and `Warnings` (`cli/skill.go:294,712`).
  This plan plugs the routing check in there.
- Canonical limits (`internal/canonical/skill.go:670-697`): at most 10 `examples` and 10 `counter_examples`,
  each at most 300 runes. In the resolver, `not_for` and `counter_examples` feed the same penalty
  (`internal/resolver/evidence.go:336`). `counter_examples` also become counter cases in `skillhub eval routing`.

### Where a curator sees it

- CLI: `skillhub telemetry cases` already lists case text (CLI only), `telemetry chains` lists text-free chains,
  `telemetry funnel` gives rates, and `skillhub skill edit` previews routing edits.
- MCP curation profile: `skill_update_preview`/`confirm` and `routing_evaluate`. No tool exposes case text.
- Web: `GET /api/v1/skills/{id}/usage` (funnel only). `TestCasesIsolationWebAndExport` asserts that case text never
  appears in web JSON or in `telemetry export`.
- Chosen surface: extend `telemetry cases` with `suggest` and `dismiss` (it already owns case text and its
  isolation), and extend `skill edit` with `--from-suggestion` (it already owns routing previews). No new command
  group is added. A flag on `telemetry chains` was rejected because that command reads text-free raw events and
  has a JSON contract, `DisagreementChain`.

### Findability check

There is no self-resolution lint (the `findability-lint` lesson is a candidate). The cheap equivalent exists:
`EvaluateRouting` (`internal/app/routing_eval.go`) runs every active skill's examples and counter-examples through
the real resolver, leave-one-out, using a catalog decorator passed to `resolveWithin`. A second decorator that
overlays the suggested routing on one skill makes a what-if check. It compares baseline and suggested failures
and reports cases that flip. It is approximate in one known way. FTS candidacy comes from the built generation, so
the overlay also injects the target skill as a search hit when a query overlaps an added example. That
over-reports candidacy, which is the conservative direction for spotting a skill that "steals" another skill's
examples. The preview labels the check approximate and points to `skillhub eval routing` after confirm. An exact
check (projecting the proposed write set into a scratch generation) is open question 3, not scope.

## Design in one page

```text
case journal (redacted text) ─┐
                              ├─► RoutingSuggestionService.Suggest ──► telemetry cases suggest (CLI, read-only)
raw events (loads, rejects) ──┘        filters, grouping, evidence         │
current catalog (active skills)                                             ▼
runtime/routing_suggestions.json (dismissed IDs) ◄── telemetry cases dismiss <id>
                                                                            │
skill edit <skill> --from-suggestion <id> ──► re-mine, find id ──► PreviewSkillUpdate(Routing: current + entry)
                                                 with Manager.RoutingHook = routing what-if check
                                                                            ▼
                                     stored proposal (24 h) ──► skill confirm <proposal-id> / --yes ──► Git working tree
```

- A suggestion ID is `rs_` followed by the first 20 hex characters of
  sha256(skill ID, NUL, field, NUL, normalized text). It is stable across runs, so `dismiss` and `--from-suggestion`
  work without storing suggestions.
- Normalization trims the text and collapses whitespace. Equality ignores case.
- Dismissals are machine-local runtime state, like the telemetry they come from. They are not written to Git.
  Losing the file only brings suggestions back.

## Phases

| # | Phase | Depends on | Status |
|---|---|---|---|
| 1 | [Record the missing case links](phase-01-record-missing-case-links.md): rejected primary and original phrasing on reformulation cases, primary on load cases, `after_needs_context` load cases | none | pending |
| 2 | [Mine routing suggestions](phase-02-mine-routing-suggestions.md): signals, filters, grouping, evidence, dismissals (app layer, read-only) | 1 | pending |
| 3 | [Preview a suggestion with the routing check](phase-03-preview-suggestion-with-routing-check.md): reuse `PreviewSkillUpdate`, what-if `RoutingHook` | 2 | pending |
| 4 | [CLI surface and docs](phase-04-cli-surface-and-docs.md): `telemetry cases suggest/dismiss`, `skill edit --from-suggestion`, help and docs | 3 | pending |

The phases run in order, and each one ends with a green `make check` and its own commit.

## Execution

- Implement in a dedicated git worktree branched from `main` (at `4198bb7` or later). Do not edit the main
  checkout.
- Before writing tests, load the `test-audit` skill (AGENTS.md "Testing").
- Commits use conventional format, for example `feat(telemetry): record the rejected skill and its phrasing in
  reformulation cases`, `feat(curation): suggest routing entries from usage`, and
  `feat(cli): preview a routing suggestion as a skill edit`. They name no plan, phase or lesson.
- Do not push or merge. The lead reviews and merges.

## Acceptance criteria

1. With the case journal enabled, a scripted MCP session (temp workspace) that resolves `no_skill` and then loads
   X yields an `examples` suggestion for X carrying the redacted phrasing, count 1, sessions 1, the date, and the
   resolution ID.
2. A verified `prior.kind: rejected` re-resolution from X to Y, followed by a load of Y, yields a
   `counter_examples` suggestion on X and an `examples` suggestion on Y, both with the **original** phrasing. If the
   load of Y is absent, only the X suggestion appears. An unverified `rejected` case yields nothing.
3. Three or more such events for X from at least 2 sessions with overlapping phrasings yield one `not_for`
   suggestion on X, which replaces those `counter_examples` suggestions.
4. Phrasings with a redaction marker, more than 300 runes, or control or bidi characters, phrasings already in the
   target or a conflicting field, a full list (10 entries), a skill that is inactive or missing, and dismissed IDs
   are never suggested. Each is counted under its own skip reason.
5. `skill edit <id> --from-suggestion <sid>` shows the reason, the evidence, the routing check (regressions and
   whether the phrase now routes as intended) and the normal diff and pins, and writes nothing to the skill until
   `skill confirm` or `--yes`. Confirming goes through `ConfirmSkillMutation`, and the result is a normal Git-dirty
   routing edit.
6. Case text still never appears in `telemetry export` or web JSON (`TestCasesIsolationWebAndExport` stays green),
   and event payloads are unchanged (`schemas/telemetry-event-v1.schema.json` is untouched).
7. `make check` passes. No test reads the live hub, the real HOME or a real `telemetry.db`.

## Risks

| Risk | Mitigation |
|---|---|
| The case journal is off by default, so the feature is empty for most hubs | `suggest` says so and names `skillhub telemetry cases enable`, and the docs state it. Off by default is a privacy decision and is not changed |
| An agent floods phrasings to push a suggestion | The case journal caps of 50 per day and 500 in total, `not_for` needs at least 2 sessions, verified priors only, and a human confirms each diff with its evidence shown |
| A suggested example makes another skill's examples resolve elsewhere | The what-if check lists those flips as warnings, labelled approximate, and the preview points to `skillhub eval routing` |
| Sensitive text moves from the runtime DB into Git through a confirmed example | Only redacted text is used, phrasings with any redaction marker are dropped, and the human sees the exact text in the diff before confirming |
| Terminal escape sequences or bidi tricks in phrasings | Mining strips and rejects control and bidi characters, and the CLI prints the text quoted |
| The what-if check is slow on a large catalog | It runs two routing evals, and is measured in phase 3. Above a case budget it is skipped with a clear summary, and the curator runs `eval routing` instead |
| Raw events (30 days) expire before cases (90 days) | The default window is 30 days. The Y half of signal 2 is withheld once its load evidence has expired |
| Lint function limits | Split mining into small functions per signal, as `usage_chains.go` does |

## Open questions

1. Should a read-only MCP curation tool expose suggestions (for the `system-curator` path without a shell)?
   It would put agent-written phrasings in front of an agent (a prompt-injection channel) and widen the case-text
   boundary, which today is CLI only. The default answer is no.
2. Should the web skill page show suggestions? That would need an explicit change to the
   `TestCasesIsolationWebAndExport` invariant. The default answer is no.
3. Should the routing check build an exact scratch generation from the proposed write set instead of the overlay
   approximation, and should it also run for ordinary `skill edit --example` previews (which overlaps
   `findability-lint`)?
4. Should an explicit `skill_feedback outcome: rejected` with no re-resolve and no override write its own case kind
   (with the resolution's redacted phrasing), so that it can trigger suggestions? Today it is only extra evidence.
5. Thresholds: the default here is 1 event for examples and counter-examples (a human confirms each) and 3 events
   from 2 sessions with overlap of at least 0.5 for `not_for`. Should they be stricter, or configurable beyond
   `--min-events`?
6. The `not_for` text is derived deterministically (shared terms of the cluster, else its medoid phrasing).
   Should `--from-suggestion` accept an edited text, or should the curator dismiss the suggestion and use
   `skill edit --not-for`?
7. Should the doc/code mismatch on raw-event retention (14 days in the design doc, 30 in code) be fixed in the
   doc as part of phase 4, or separately?
