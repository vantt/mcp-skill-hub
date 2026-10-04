# fgDesign System v1.0

A token-driven, single-skeleton design system for data-dense product UI (CRM,
task management, financial reporting, editorial reading, media playback).
One markup, six orthogonal axes reskin it via CSS alone:
`data-theme × data-scheme × data-accent × data-density × data-num-font × data-typeface-set`.

**Sources:** imported from Claude Design project https://claude.ai/design/p/65354ff3-365c-4390-8b04-29c8e76a7930, originally authored natively there (formerly `design-contract/`
inside the fgCRM project). Inducted donors, all read-only: `retailCRM Prototype`
(rule builder, divergence bar, cohort grid), `dhamma-player` / "Autumn Atelier"
(→ `atelier` theme, tree, sync, shell, media domain), and the open-source Vibe
design system (MIT, github.com/mondaycom/vibe → `moday` theme). Theme names
describe a *character*, not an affiliation; there is no brand logo in this system.

## Quick start

```html
<link rel="stylesheet" href="styles.css">
<html data-theme="precision" data-scheme="dark" data-accent="moss"
      data-density="compact" data-num-font="mono" data-typeface-set="grotesk">
<body class="fg-root">
  <button class="fg-btn fg-btn--primary">Get started</button>
```

Three equivalent ways to author the same DOM (never mix values, only syntax):
1. **Raw markup** — `.fg-*` classes (canonical; all 125 roles). See `cards/`.
2. **Web Components** — `<fg-button variant="primary">` via `contract/elements.js` (13 atomic roles).
3. **React** — `components/**/*.jsx`, exposed in the compiled bundle (13 atomic roles + Input/Select + composite roles: NavRail, Table, Kanban/TaskCard, Ledger, Profile/Facts/Pipeline).

## Index

- `styles.css` — the ONLY file consumers link (`@import` barrel).
- `contract/AGENT_RULES.md` — binding rules for any agent touching this project (source project's CLAUDE.md).
- `thumbnail.html` — homepage tile.
- `SKILL.md` — Agent-Skill entry point.
- `contract/SPEC.md` — **the binding contract.** Read first.
- `contract/contract.css` — Tier 2 + 3 tokens with neutral defaults.
- `contract/components.css` — Tier 4 core roles (50).
- `contract/patterns/*.css` — domains: editorial · task · crm · financial · media.
- `contract/CATALOG.md` / `catalog.json` — every role + aliases ("I have an AlertBanner → `.fg-banner`").
- `contract/elements.js` — optional Web Component companion.
- `themes/` — precision · berich · clickup · terminal · atelier · moday.
- `components/` — React companion: `actions/` Button · `forms/` Field, Input, Select, Switch · `display/` Card, Chip, Badge, Status, Avatar, AvatarCluster, Kpi · `feedback/` Banner, Caveat · `navigation/` Tabs (+Tab), NavRail (+NavItem) · `data/` Table · `task/` Kanban, KanbanColumn, TaskCard · `financial/` Ledger · `crm/` Profile, Facts, Pipeline.

**Intentional additions:** the React layer adds nothing visual — it emits the
exact markup of existing roles (`Input`/`Select` are the existing `.fg-input` /
`.fg-select` roles; `Field` already wraps them in elements.js). The composite components (NavRail, Table, Kanban/TaskCard, Ledger, Profile/Facts/Pipeline) likewise emit only existing `.fg-nav`, `.fg-table`/`.fg-bulkbar`, `.fg-kanban`/`.fg-task`, `.fg-ledger`, `.fg-profile`/`.fg-facts`/`.fg-pipeline` markup — added so React consumers don't hand-write the most-used domain roles.

**Templates** set `data-theme` (etc.) on `<html>` from their Tweaks props and add `fg-root` to `<body>`; in a consumer, point `ds-base.js`'s `base` at the bound DS folder.

---

## Content fundamentals

- **Language:** English for UI copy and all code identifiers; prose commentary may be Vietnamese. Sample data is Vietnam-flavoured (Mai Lan Nguyen, Ho Chi Minh City, `₫ 4,820,000`) alongside USD.
- **Casing:** sentence case for buttons, titles, menu items ("Log a call outcome", "Confirm export"). ALL-CAPS only via `.t-label` furniture (eyebrows, field labels, table heads) — and themes can turn that off with `--type-label-case`.
- **Voice:** terse, operational, second person implied, no "I/we". State the fact, then the action: "Payment sync failed for this account." → **Retry**. "3 items are below reorder threshold." → **Review**.
- **Numbers carry meaning:** always exact, formatted, tabular (`1,284 rows`, `$284.5k`, `▲ 12.4%`). Deltas pair a glyph with the figure; money is direction-colored via `.fg-money`.
- **Length:** buttons 1–2 words ("Save", "Get started"), banners one sentence, hints one clause ("Shown on invoices.").
- **No emoji.** No exclamation marks. No marketing fluff inside product chrome.

## Visual foundations

- **Architecture:** Tier 1 private theme primitives (`--_*`) → Tier 2 semantic tokens (`--color-*`, `--type-*`, `--space-*`, `--radius-*`, `--elevation-*`, `--motion-*`) → Tier 3 character tokens (`--btn-radius`, `--card-elevation`, `--chip-radius`…) → Tier 4 `.fg-*` roles that read only Tier 2+3.
- **Color:** `action ≠ brand ≠ link` are separate tokens; a theme may unify (precision, berich, atelier, terminal), split fully (clickup: ink action / violet brand / blue link) or partially (moday: action = brand, link split). Status colors are AA-checked as text on their own tints. Value direction (`--color-positive/-negative`) is distinct from status. Optional 5-color `--color-accent-alt-*` signature palette drives tags and lanes. Tints are derived with `color-mix`, never new hexes.
- **Scheme:** every theme ships light + dark; `data-scheme` swaps color tokens only — radius, weight, type and character never change.
- **Type:** four family slots (`display`, `body`, `mono`, `accent`) and 15 roles (`display`, `title`→`heading-sm`, `body`, `body-sm`, `label`, `caption`, `micro`, `ui`, `tag`, `figure-sm/md/lg`), each with size/leading/tracking/weight tokens and a `.t-<role>` helper. Semantic weight scale (regular/medium/semibold/bold). Figures use `--num-variant` (tabular, slashed zero, lining). All faces from Google Fonts: Fraunces, Newsreader, Geist, Geist Mono, Space Grotesk, Be Vietnam Pro, Spectral, IBM Plex Mono, Plus Jakarta Sans, Inter, Sometype Mono, JetBrains Mono, Manrope, Figtree, Poppins.
- **Spacing:** `--space-0…8` = 0, 4, 8, 12, 16, 24, 36, 56, 80. Density axis (`compact`) tightens row padding, card padding, input height.
- **Radius:** scale `none · xs · sm · md · lg · pill`; which bar a component uses is per-theme character (precision sharp 4px, clickup/atelier pill, terminal none).
- **Cards:** surface fill + hairline border (`--border-width-hairline`) + theme-owned radius and elevation (none in precision/terminal, warm shadow in berich, `elevation-sm` in clickup/atelier, Vibe xs in moday). `--rule` variant adds a strong left strip in action color; `--sunken` sits on the sunken surface.
- **Elevation:** four steps `none/sm/md/lg`; mostly border-first, shadows soft and low. Modals use `--color-scrim` backdrop.
- **Backgrounds:** flat solid canvases. No photography, no illustrations, no repeating textures. Only signature effects: faint page vignette (precision/berich/terminal scanlines), conic signature border (clickup/moday), sunken section bands, dark feature panel.
- **Motion:** `--motion-fast` 120ms (hover/press), `--motion-settle` 200ms, `--motion-open` 260ms (overlays), `--ease-standard` / `--ease-decelerate`. Fades and short slides; no bounces. Sync spinner is the only loop.
- **States:** hover = `--color-action-hover` / `--color-surface-hover` tint; press = `--color-action-press`; focus = outline `--focus-width` in `--focus-color` (soft ring in atelier/moday); disabled = reduced opacity + `--color-text-disabled`. Status dots glow via `--status-glow` (off in clickup/moday).
- **Transparency & blur:** only for scrims and `color-mix` tints; no glassmorphism.
- **Layout:** `.fg-page` (centered column, `--layout-page-max`) for documents; `.fg-shell` (header / left rail · main · right rail / footer) for app chrome; `.fg-reading-shell` for long-form. Tables are the workhorse — sortable heads, row selection + bulk bar, sticky wrappers.

## Iconography

- **No icon font, no sprite, no icon package.** The system uses **unicode glyphs** as icons, styled by color/size tokens: `▾` select chevron, `✓` check/step done, `✕` clear/close, `▲ ▼ —` deltas, `◧ ☰ ◷ ✉` nav items, `⧉` copy, `⌘L` shortcuts (in `.fg-cmdk__kbd` / `.fg-kbd`).
- One **inline SVG** recurs: the 13px stroke magnifier in `.fg-search__glyph` and `.fg-cmdk__head` (1.3px stroke, `currentColor`). Copy it from `demo/core.html`.
- Icon-only actions use `.fg-icon-btn` (square, theme radius). Avatars are **initials**, not photos.
- **No emoji. No logo** — the system has no brand mark; render the product name in `.t-heading` where a mark would go.
- If a consumer needs a richer set, pick a thin-stroke outline set (e.g. Lucide from CDN) at `currentColor` — this is a consumer substitution, not part of the system.

## Consuming this system elsewhere

Copy `styles.css` + `contract/` + `themes/` (self-contained, relative paths).
Copies are snapshots. Every change here goes through the Change Protocol in
`contract/SPEC.md` with `demo/contract-tests.html` green.
