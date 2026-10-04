# WebUI mockup (Claude Design export)

Interactive prototype of the Skill Hub WebUI, kept in the repo as the visual reference for the WebUI build.
It is a design artifact, not production code: it runs on the Claude Design `x-dc` runtime with mock data embedded in the file.

| Item | Value |
|---|---|
| Source project | [Skill Hub WebUI prototype](https://claude.ai/design/p/786ffd79-c2b1-4b1c-a667-143c8423b263?file=Skill+Hub+WebUI.dc.html) (Claude Design project `786ffd79-c2b1-4b1c-a667-143c8423b263`) |
| Exported | 2026-10-03 |
| Behavior spec | [`../../use-cases/04-webui-user-flows-and-screen-specs.md`](../../use-cases/04-webui-user-flows-and-screen-specs.md) |
| Design brief | [`../../use-cases/05-webui-design-brief.md`](../../use-cases/05-webui-design-brief.md) |

When the mockup and the behavior spec disagree on fields, actions or error codes, the spec wins.

## Contents

| Path | Purpose |
|---|---|
| `Skill Hub WebUI.dc.html` | The prototype: all 13 routes, modals, drawer, and state toggles (loading / empty / degraded, run states, conflict, stale proposal, density, dark mode). Mock data lives in its script block. |
| `support.js` | Claude Design `x-dc` runtime that renders the `.dc.html` file (generated; do not edit). |
| `_ds/fgdesign-system-3658085c-…/` | Snapshot of the fgDesign System the prototype uses: tokens (`contract/`), roles and patterns, six themes (`themes/`), the compiled React bundle (`_ds_bundle.js`), and the system's own `readme.md`. The prototype loads the `precision` theme. |

## Viewing

React and the fonts load from CDNs, so open it through a local static server with network access:

```bash
cd docs/design/webui-mockup && python3 -m http.server 8801
# open http://localhost:8801/Skill%20Hub%20WebUI.dc.html
```

## Not exported

The export is limited to what the prototype imports. These parts of the source projects were not copied:

- Design system files the prototype does not load: `cards/`, `demo/`, `templates/`, `components/*.jsx` sources, `contract/SPEC.md`, `contract/CATALOG.md`, `contract/AGENT_RULES.md`, `_ds_manifest.json`, `_adherence.oxlintrc.json`.
- Project files `github.md` and `uploads/draw-*.png`.

The fgDesign System itself is maintained in its own Claude Design project (`65354ff3-365c-4390-8b04-29c8e76a7930`). Re-sync from there instead of editing the snapshot here.
