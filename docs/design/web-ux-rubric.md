# Web UI curation rubric

How the web UI is judged, one flow at a time. The capture harness produces the evidence; this page says
how to score it. Plan: `plans/261010-1454-web-curation-ux-evaluation/plan.md`.

## Run the capture

```bash
make web-ux
```

The command rebuilds the UI and the binary, clones a hub into a temp directory with an isolated `HOME`
and `XDG_*`, adds one skill per lifecycle state through the CLI, serves the web UI and captures every
flow at 1280 px and 390 px. It prints the output directory (default `web/test-results/ux`, not committed).

| Variable | Default | Meaning |
|---|---|---|
| `UX_HUB_SOURCE` | `~/skill-hub` when it is a Git workspace | Hub to clone. The source is only read; nothing is written to it. Set it empty to use a small fixture workspace instead. |
| `UX_OUT_DIR` | `web/test-results/ux` | Where screenshots, text and reports go. It is emptied at the start of a run. |

Per flow the output holds:

- `<flow>-1280.png` and `<flow>-390.png`: the whole page, not only the first screen.
- `<flow>.txt`: the visible text at 1280 px, for the wording check.
- `<flow>-<width>.axe.json`: axe findings (WCAG 2 A and AA).
- `<flow>-<width>.metrics.json`: horizontal overflow, primary actions below the first screen, and elements
  whose text is cut off or pushed past the edge.
- `index.json`: every flow with its row in the Tier table, the task, the per-width summary, and the states
  the harness could not create.

The flow list lives in `web/e2e/ux/flows.ts`, in the Tier order of the plan (most used first). A flow
can capture a second state of the same screen, such as an open dialog; both carry the same Tier row.

## States the harness cannot create

The CLI cannot write a skill that fails strict validation, and it only tracks upstream repositories over
https GitHub. Those two states are listed under `skipped` in `index.json` instead of being faked. A clone of
a hub that already has upstream-tracked skills shows real upstream states.

## Scores

Each flow gets 0, 1 or 2 for each item. A flow passes only if a first-time user can finish its task
without leaving the page to read a document.

| Score | Meaning |
|---|---|
| 0 | Fail: the user is stuck, misled or sees a raw error |
| 1 | Weak: the user finishes only after guessing or scrolling around |
| 2 | Good: the user finishes without hesitation |

| Item | Question |
|---|---|
| Task success | Can a first-time user finish the flow from this screen alone? |
| Next step | Is the next action obvious and the primary action visible without scrolling at 1280 and 390 px? |
| Wording | Would the user understand every label without reading a doc? Any internal word (digest, proposal, canonical, served)? |
| State | Are empty, loading, error and disabled states explained, with a way forward? |
| Safety | Is the preview vs apply boundary clear? Does it say what will change before it changes? |
| Copyable | Can every command shown be read and copied whole? |
| Mobile | Is nothing clipped or overlapping at 390 px? |
| Access | Does axe report no serious or critical issue? Is it keyboard reachable? |

Where the evidence comes from:

- **Next step** and **Mobile**: the screenshots, plus `primaryActions[].belowFold` and `clipped` in the metrics.
- **Wording** and **State**: the `.txt` dump, then the screenshot to see how the text is shown.
- **Copyable**: any command in the text dump that appears in `clipped`, or cut off in the screenshot, scores 0.
- **Access**: `axe.json` for serious and critical findings. Keyboard reach is checked by hand or in a later phase.
- **Safety** and **Task success**: read the flow's task in `index.json` and try to finish it from the screenshot alone.

## Scorecard template

One row per flow, in Tier order. Defect ids come from the defect list in the plan (D1, D2, ...); add new
ids as new defects are found. Keep the scored copy next to the phase report, not in this file.

| Flow | Task | Next | Wording | State | Safety | Copyable | Mobile | Access | Notes | Defects |
|---|---|---|---|---|---|---|---|---|---|---|
| home | | | | | | | | | | |
| skills-list | | | | | | | | | | |
| skill-review | | | | | | | | | | |
| skill-review-draft | | | | | | | | | | |
| lifecycle-activate | | | | | | | | | | |
| lifecycle-deprecate | | | | | | | | | | |
| editor | | | | | | | | | | |
| editor-preview | | | | | | | | | | |
| editor-conflict | | | | | | | | | | |
| create | | | | | | | | | | |
| create-preview | | | | | | | | | | |
| add | | | | | | | | | | |
| add-advanced | | | | | | | | | | |
| upstream-updates | | | | | | | | | | |
| sources | | | | | | | | | | |
| distill-empty | | | | | | | | | | |
| distill-source | | | | | | | | | | |
| inbox | | | | | | | | | | |
| skill-tab-resources | | | | | | | | | | |
| skill-tab-usage | | | | | | | | | | |
| skill-tab-runtime | | | | | | | | | | |
| skill-tab-sources | | | | | | | | | | |
| appearance-menu | | | | | | | | | | |

Honest limit: reading screenshots finds broken, truncated, unclear and inconsistent screens well. It cannot
say what feels pleasant; the final phase of the plan ends with a walk-through by the user.
