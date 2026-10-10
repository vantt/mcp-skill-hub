# Web UI curation scorecard

Scores for each flow, judged with [the rubric](web-ux-rubric.md) from the capture harness output
(`make web-ux`). Scale: 0 fail, 1 weak, 2 good. Plan: `plans/261010-1454-web-curation-ux-evaluation/plan.md`.

Columns: Task, Next step, Wording, State, Safety, Copyable, Mobile, Access.

## Tier 1: daily flows

Scored on the capture of the hub clone, before the Tier 1 fixes and after them. "Before" is the
state at commit `7ec5811`; "after" is the working tree that carries the fixes below.

### Before

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Defects |
|---|---|---|---|---|---|---|---|---|---|
| home | 1 | 0 | 0 | 1 | 2 | 0 | 1 | 1 | D3, D9, D10, D11 |
| skills-list | 2 | 1 | 1 | 2 | 2 | 2 | 0 | 2 | D6, D8, D10 |
| skill-review | 1 | 2 | 1 | 2 | 2 | 1 | 0 | 2 | D2, D7, D9, D10, D12, D14, D15 |
| skill-review-draft | 1 | 1 | 1 | 1 | 2 | 1 | 0 | 2 | D7, D9, D10, D12, D14 |
| lifecycle-activate | 1 | 1 | 1 | 1 | 2 | 1 | 0 | 2 | D9, D10, D12 |
| lifecycle-deprecate | 1 | 2 | 0 | 1 | 1 | 2 | 1 | 2 | D13, D16 |

### After

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Notes |
|---|---|---|---|---|---|---|---|---|---|
| home | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Overview category names were reworded in Tier 3 (D30): "Sources ready to distill", "Candidate lessons to review". |
| skills-list | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Rows stack on a phone with the column names beside each value. |
| skill-review | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | |
| skill-review-draft | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | What is missing is said once next to the button, with a link to the editor. |
| lifecycle-activate | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | The harness captures only the disabled state; the confirm dialog of a skill that is ready to activate is covered by the lifecycle E2E. |
| lifecycle-deprecate | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Dialog says what changes and uses the action as the confirm label. |

No 0 remains on Task success, Next step or Copyable. Metrics after the fixes: no cut-off element and no
horizontal overflow at 390 px on these six flows, and no serious or critical axe finding at either width
(before: one at 390 px on home).

## Defects

D1 to D8 are the ids from the plan. D9 and up were found in this tier.

| Id | Severity | Defect | Status |
|---|---|---|---|
| D2 | High | A command could run off its box (no wrap, scroll inside the box) so it could not be read whole. | Fixed: every command wraps and the copy button copies the whole command. Covered by a unit test. |
| D3 | Medium | Home showed every count twice (Overview and Action categories). | Fixed: the second list is gone. Categories the product does not use yet read "Not set up", not 0. |
| D6 | Medium | At 390 px the Create skill button and the Routing column were pushed off screen. | Fixed: buttons wrap, rows stack with a label per value. |
| D7 | Low | Provenance title and button overlapped on a narrow card. | Fixed: the header row wraps. |
| D8 | Low | Three filters all showed "All". | Fixed: each has a visible label (Lifecycle, Collection, Upstream) and search has one too. |
| D9 | High | A missing spacing token (`--space-10`) voided the whole padding of the page area, so every page touched the header and the rail, and the title overlapped the header at 390 px. | Fixed in the shell. |
| D10 | High | At 390 px the left rail took 168 px and the header brand 200 px, leaving about 220 px for content. | Fixed: the rail is an icon strip and the brand name hides under 720 px. |
| D11 | High | Home's next action read "host integration missing" with no command, no reason, no way forward. | Fixed: plain title, a one-sentence reason and a copyable `skillhub doctor --fix`. |
| D12 | Medium | A draft that cannot be activated showed a long CLI sentence with raw backticks and a separate "Missing activation requirements"; a ready draft told the user to use the CLI next to an Activate button. | Fixed: one line lists what is missing with a link to the editor; a ready draft says it is ready. |
| D13 | Medium | The transition dialog was titled "Skill deprecated transition is ready for review.", showed an empty Diff and the button said "Confirm deprecated". | Fixed: "Deprecate ux-active?", a "What changes" line, no empty diff, button "Deprecate skill". |
| D14 | Medium | Review tab used internal words (canonical, served, validity) and always showed "Structurally valid". | Fixed: each card has a one-line explanation, the badge follows the review result. |
| D15 | Low | The header chip showed the skill's first operation as if it were its collection. | Fixed: it shows the collection. |
| D16 | Low | Dialog shadows used tokens that do not exist, so dialogs had no elevation. | Fixed: the tokens map to the design system's elevation. |

## Evidence

Before and after, 1280 px and 390 px (images in `web-ux-evidence/tier1/`):

| Flow | Before | After |
|---|---|---|
| home 1280 | ![](web-ux-evidence/tier1/before-home-1280.png) | ![](web-ux-evidence/tier1/after-home-1280.png) |
| home 390 | ![](web-ux-evidence/tier1/before-home-390.png) | ![](web-ux-evidence/tier1/after-home-390.png) |
| skills list 390 | ![](web-ux-evidence/tier1/before-skills-list-390.png) | ![](web-ux-evidence/tier1/after-skills-list-390.png) |
| draft review 1280 | ![](web-ux-evidence/tier1/before-skill-review-draft-1280.png) | ![](web-ux-evidence/tier1/after-skill-review-draft-1280.png) |
| draft review 390 | ![](web-ux-evidence/tier1/before-skill-review-draft-390.png) | ![](web-ux-evidence/tier1/after-skill-review-draft-390.png) |
| deprecate dialog 1280 | ![](web-ux-evidence/tier1/before-lifecycle-deprecate-1280.png) | ![](web-ux-evidence/tier1/after-lifecycle-deprecate-1280.png) |
| deprecate dialog 390 | ![](web-ux-evidence/tier1/before-lifecycle-deprecate-390.png) | ![](web-ux-evidence/tier1/after-lifecycle-deprecate-390.png) |

## Tier 2: weekly flows

Scored the same way on the capture of the hub clone, before and after the Tier 2 fixes. "Before" is the
state at commit `8d60581`. Upstream updates could not be captured with an update present: the hub clone has no
upstream-tracked skills and the CLI only adds skills from a real GitHub address, so the update review screen
was scored from its source and unit tests rather than from a screenshot.

### Before

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Defects |
|---|---|---|---|---|---|---|---|---|---|
| editor | 1 | 1 | 1 | 2 | 2 | 2 | 1 | 2 | D20, D21, D22 |
| editor-preview | 0 | 1 | 0 | 1 | 0 | 2 | 2 | 2 | D17, D18 |
| editor-conflict | 1 | 1 | 1 | 1 | 0 | 2 | 2 | 2 | D19, D23 |
| create | 1 | 1 | 1 | 2 | 2 | 2 | 1 | 2 | D20, D22, D24, D27 |
| create-preview | 0 | 1 | 0 | 1 | 1 | 2 | 2 | 2 | D17, D24 |
| add | 1 | 1 | 1 | 1 | 2 | 2 | 1 | 2 | D22, D25, D27 |
| add-advanced | 1 | 1 | 1 | 1 | 2 | 2 | 1 | 2 | D25 |
| upstream-updates | 1 | 0 | 1 | 1 | 2 | 2 | 2 | 2 | D26 |

### After

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Notes |
|---|---|---|---|---|---|---|---|---|---|
| editor | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Every routing field has a one-line explanation and an example; Preview changes stays off until something changed. |
| editor-preview | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Says in words what changes; the exact patch is one click away. |
| editor-conflict | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Shows what each side changed and keeps an edit made elsewhere. |
| create | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Says a draft is not routed yet; Preview draft is on the first screen at 1280 x 800. |
| create-preview | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | |
| add | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Not captured past Discover (needs the network). The Review step is covered by unit tests. |
| add-advanced | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | |
| upstream-updates | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Empty list explains why and how to check; the update review screen is scored from source and tests only. |

No 0 remains on Task success, Wording or Safety. Metrics after the fixes: no cut-off element, no horizontal
overflow and no serious or critical axe finding at 1280 or 390 px on these eight captures (same as before).

### Defects found in this tier

| Id | Severity | Defect | Status |
|---|---|---|---|
| D17 | High | The preview dialogs of Editor and Create were empty apart from a title and "Technical details". The web client read fields the API does not send (`impact`, `paths`, a text `diff`) and ignored the ones it does (`diff` as path lists, `full_diff`, `routing_impact`). | Fixed: one reader for the real fields; the dialog lists the files touched, what changes in words, and the exact patch behind a toggle. |
| D18 | High | Editor confirm said "Skill edit is ready for review." and the button said "Review"; the result was a toast with a raw operation id. | Fixed: "Save changes to <id>?", button "Save changes", toast "Saved 1 change to <id>."; after saving the form shows the saved text. |
| D19 | High | "Use latest as base" sent the old description back, silently undoing an edit made elsewhere; "Discard draft" reset only some fields. | Fixed: the person's edits are applied on top of the saved version and only for fields they changed; discard resets every field. Covered by unit and E2E tests. |
| D23 | High | The conflict drawer showed the same stale text on both sides ("Latest canonical" was the version the editor opened with) and the label "digest differs". | Fixed: the latest version is fetched; the drawer lists what you changed and what changed meanwhile, warns when both touched a field, and explains both ways forward. |
| D20 | Medium | Operations, Triggers, "Not for / Rationale" and "Min scope" had no explanation anywhere. | Fixed: one-line hint and an example on Create and Editor (shared text); scope choices say what they mean. |
| D21 | Medium | The readiness card's "Go to field" buttons opened the Editor without focusing the field. | Fixed: the cursor lands in the field that was named. |
| D24 | Medium | Create did not say that it makes a draft that is not routed yet, and left Description optional on the form while the server rejects an empty one. | Fixed: notes on the form and in the preview; Description is marked required and checked on the form. |
| D27 | Medium | A taken skill id or any other server-side validation failure came back as "The request conflicts with validation rules" with no reason. | Fixed in `routes_skill_write.go`: the validation message becomes the reason for create and edit previews. |
| D25 | Medium | Add from GitHub started with a made-up address that looked real, did not say what Discover does, and the Review step ignored the license, conflicts and files. | Fixed: empty field with an italic placeholder, a note that Discover only looks, hints under Advanced; the Review step shows the skills, revision, trust note, license warning, conflicts and files that would be written. |
| D26 | Low | The Skills list gave no help when no skill has an upstream update. | Fixed: the empty state says why and offers `skillhub skill outdated --check`; the filter option reads "Has an update". |
| D22 | Medium | Inputs were wider than their column (no border-box), so fields touched each other and crossed the card edge. | Fixed in the shared stylesheet; placeholders are italic so they do not pass for typed values. |

### Evidence

Before and after (images in `web-ux-evidence/tier2/`):

| Flow | Before | After |
|---|---|---|
| editor preview 1280 | ![](web-ux-evidence/tier2/before-editor-preview-1280.png) | ![](web-ux-evidence/tier2/after-editor-preview-1280.png) |
| editor conflict 1280 | ![](web-ux-evidence/tier2/before-editor-conflict-1280.png) | ![](web-ux-evidence/tier2/after-editor-conflict-1280.png) |
| create 1280 | ![](web-ux-evidence/tier2/before-create-1280.png) | ![](web-ux-evidence/tier2/after-create-1280.png) |
| create preview 390 | ![](web-ux-evidence/tier2/before-create-preview-390.png) | ![](web-ux-evidence/tier2/after-create-preview-390.png) |
| add, advanced open 1280 | ![](web-ux-evidence/tier2/before-add-advanced-1280.png) | ![](web-ux-evidence/tier2/after-add-advanced-1280.png) |

## Tier 3: occasional flows and dead screens

Scored on the capture of the hub clone, before and after the Tier 3 fixes. "Before" is the state at commit
`25a0250`; "after" is the working tree that carries the fixes below. Evidence images are in
`web-ux-evidence/tier3/`.

### Before

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Defects |
|---|---|---|---|---|---|---|---|---|---|
| home (Overview) | 2 | 2 | 0 | 2 | 2 | 2 | 2 | 2 | D30 |
| sources | 1 | 1 | 1 | 1 | 2 | 2 | 0 | 2 | D31 |
| distill-empty | 0 | 1 | 1 | 0 | 2 | 2 | 2 | 2 | D29 |
| distill-source | 0 | 1 | 0 | 2 | 2 | 1 | 1 | 2 | D28 |
| inbox (old address) | 1 | 0 | 1 | 1 | 2 | 2 | 2 | 2 | D32 |
| skill-tab-resources | 2 | 1 | 1 | 1 | 2 | 2 | 2 | 2 | D34 |
| skill-tab-usage | 2 | 1 | 1 | 1 | 2 | 2 | 1 | 2 | D34 |
| skill-tab-runtime | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | |
| skill-tab-sources | 1 | 1 | 1 | 1 | 2 | 2 | 2 | 1 | D34 |
| appearance-menu | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | |

### After

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Notes |
|---|---|---|---|---|---|---|---|---|---|
| home (Overview) | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Plain names; no run or lesson-inbox term left. |
| sources | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | A sentence says what a source is and what "ready" means; the empty state says how sources come to exist; row actions wrap at 390 px. |
| distill-empty | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Offers the ready sources to tick instead of a dead end; says why nothing is ready when so. |
| distill-source | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | The brief names each skill with its source and follows the distill-lab flow. |
| inbox (old address) | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Page not found with links to Skills, Sources and Home, and a pointer to where lessons live. |
| skill-tab-resources | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Empty state says what companion files are. |
| skill-tab-usage | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Empty state says when usage appears; the time-window buttons wrap at 390 px. |
| skill-tab-runtime | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Unchanged. |
| skill-tab-sources | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Says what a learning reference is; the address field has a label. |
| appearance-menu | 2 | 2 | 2 | 2 | 2 | 2 | 2 | 2 | Unchanged; the panel fits 390 px. |

No 0 remains. Metrics: before, `sources` had 2 clipped elements and `distill-source` and `skill-tab-usage` had 1 each at
390 px; after, no clipped element, no horizontal overflow and no serious or critical axe finding on any
Tier 3 capture at either width.

### Decisions

- **Inbox (D1, decided through the coordinator):** lessons are not triaged on a separate page. The app has no
  Inbox route or nav item (the old binary that showed "Unknown API path" and a badge of 39 predates that).
  `.meta/distill.yaml` does hold candidate lessons; the web already shows them per skill in the Distill tab
  (read-only) and Home links there. The Distill tab now says how to decide them, and `/inbox` lands on a page that
  points to Skills and Sources.
- **Public contract:** the Home JSON schema (`schemas/curation-home-v1.schema.json`, emitted by `skillhub status --json`
  and the status MCP tool) keeps its `home_summary` field names. Only the free-text category `kind` values
  changed.

### Defects found in this tier

| Id | Severity | Defect | Status |
|---|---|---|---|
| D1 | High | Inbox page failed with "Unknown API path" and a nav badge of 39. | Not present in the current app (route, nav item and API are gone); `/inbox` now says Page not found with links. Covered by an E2E test. |
| D4 | Medium | Sources page kept "Open a run" and "Recent runs". | Not reproducible: already gone in the current app. An E2E test asserts no "run" text on an empty Sources page. |
| D5 | Medium | The handoff brief told the agent to call `curation_run_start` and `curation_run_submit`, which no longer exist. | Fixed together with D28. |
| D28 | High | The brief could not be followed: no such tools, and it asked for "insights" and a run id back. | Fixed: it names each skill with its source and describes the distill-lab pass (`.meta/distill.yaml`, `distill.py`, do not edit the skill, do not commit, report lessons by decision state). Unit and E2E tests. |
| D29 | Medium | Distill with no source selected was a dead end ("Select learning sources on the Sources screen first."), and the Sources button was disabled with only a tooltip. | Fixed: the button always works; the Distill page lists the ready sources to tick, or says why none is ready. |
| D30 | Medium | Home Overview named categories after removed concepts, in raw snake case: "pending insights", "changed sources", "interrupted runs". | Fixed at the source: category kinds are `sources_ready_to_distill` and `candidate_lessons`, `interrupted_runs` is dropped (nothing writes a run), and the web shows plain names. |
| D31 | Medium | Sources said nothing about what a source is, "Changed" was unexplained, and at 390 px the Unwatch button ran off the card. | Fixed: intro sentence, richer empty state, wrapping row actions. |
| D32 | Medium | The old inbox address ended on "Page not found." with nowhere to go. | Fixed: links to Skills, Sources and Home. |
| D33 | Low | `var(--color-primary)` (Sources) and `var(--color-surface-subtle)` (Distill tab) are not tokens, so the skill links and lesson cards lost their colour. | Fixed: `--color-link` and `--color-surface-sunken`. |
| D34 | Low | Resources, Usage and the skill Sources tab gave empty states without saying what the thing is; the Usage time-window buttons ran off screen at 390 px; the address field had no label. | Fixed. |
| D35 | Low | The Distill tab showed lessons but not how to decide them. | Fixed: one note naming the curator agent and `distill.py decide`. |

### Evidence

Before and after (images in `web-ux-evidence/tier3/`):

| Flow | Before | After |
|---|---|---|
| home 1280 | ![](web-ux-evidence/tier3/before-home-1280.png) | ![](web-ux-evidence/tier3/after-home-1280.png) |
| sources 390 | ![](web-ux-evidence/tier3/before-sources-390.png) | ![](web-ux-evidence/tier3/after-sources-390.png) |
| distill, nothing selected 1280 | ![](web-ux-evidence/tier3/before-distill-empty-1280.png) | ![](web-ux-evidence/tier3/after-distill-empty-1280.png) |
| distill brief 1280 | ![](web-ux-evidence/tier3/before-distill-source-1280.png) | ![](web-ux-evidence/tier3/after-distill-source-1280.png) |
| usage 390 | ![](web-ux-evidence/tier3/before-skill-tab-usage-390.png) | ![](web-ux-evidence/tier3/after-skill-tab-usage-390.png) |

## Final scorecard

The whole harness was run again after the last fix (`make web-ux`, a clone of the live hub, 23 captures at 1280
and 390 px). Facts from that run: no capture error, no horizontal overflow, no clipped element and no serious or
critical axe finding on any flow at either width, and the visible text of every flow is identical to the Tier 3
"after" capture. Scores below are the sums of the per-tier tables above (8 criteria per flow, 2 is the best).

| Tier | Flows | Before: zeros / ones / points | After: zeros / ones / points |
|---|---|---|---|
| 1 daily | 6 | 8 / 23 / 57 of 96 | 0 / 0 / 96 of 96 |
| 2 weekly | 8 | 7 / 30 / 84 of 128 | 0 / 0 / 128 of 128 |
| 3 occasional | 10 | 7 / 24 / 122 of 160 | 0 / 0 / 160 of 160 |

No 0 remains in any tier. These are scores from reading screenshots and metrics; they say what is broken,
unclear or inconsistent, not what feels pleasant. That judgement belongs to the user's walk-through
([script and empty notes](web-ux-walkthrough.md)), which has **not happened yet**, so the plan's walk-through
criterion is still open.

### Regression guards

They run in the default E2E run (`make web-ux` is not needed) on a self-contained fixture hub, over every flow in
`web/e2e/ux/flows.ts`. Each was shown red by putting the old defect back and green again after restoring it.

| Guard | Spec | Put back to see it fail |
|---|---|---|
| No raw API error text, internal id (snake_case) or retired feature word (insight, inbox, run) on any flow or error route; each unknown address offers a way out | `web/e2e/ux/guards.spec.ts` | Home shows the raw category id; a label reads "Pending insights" |
| A taken skill id is refused with its reason | `web/e2e/ux/guards.spec.ts` | The server stops adding the validation message as the reason |
| No horizontal scroll and no clipped text on any flow at 390 px | `web/e2e/ux/guards.spec.ts` | Commands scroll instead of wrapping; the conflict drawer digests cannot wrap |
| Every copy button copies exactly the command beside it, and that command is fully visible | `web/e2e/ux/guards.spec.ts` | The copy button copies a truncated command |
| Axe (serious and critical) on every nav item and every Tier 1 and 2 route at 360, 390, 768, 1280 and 1440 px, light and dark | `web/e2e/a11y.spec.ts` | Pale muted text colour in the light theme |

Defect found by the guards: **D36 (Low)** the two sha256 digests in the editor conflict drawer's "Technical
details" had no wrapping, so they ran past the edge at 390 px. Fixed with the shared `app-wrap` class in
`web/src/components/ConflictDrawer.tsx`.

Not verified: the CI matrix (ubuntu, macOS, Windows). Nothing was pushed in this run. The guards use no shell
paths, only Node and Playwright APIs.
