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
| home | 2 | 2 | 1 | 2 | 2 | 2 | 2 | 2 | Overview still lists "pending insights" and "changed sources" as raw category names; see open question. |
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

## Tier 2 and Tier 3

Filled in by the later phases.
