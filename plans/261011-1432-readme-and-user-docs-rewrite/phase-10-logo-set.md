---
phase: 10
title: "Design the logo set"
status: pending
priority: P2
effort: "2h"
wave: 1
dependencies: [1]
---

# Phase 10: Design the logo set

## Goal

A real, small, valid SVG logo set that matches the product look: calm and editorial, a warm off-white
background, serif headings, a sage-green accent, and a green diamond mark next to a serif "Skill Hub" wordmark
(today the app draws a plain `◆` glyph in `--color-action`, `web/src/components/AppShell.tsx:103`). The README
header shows it in light and dark mode. No raster-only logo.

## Owned files

Create, under `docs/images/brand/` (names frozen by Phase 1):
- `skill-hub-logo-light.svg`: horizontal logo, mark plus wordmark, for light backgrounds.
- `skill-hub-logo-dark.svg`: the same for dark backgrounds.
- `skill-hub-mark-light.svg`: square mark only (works at 16 px), for light backgrounds.
- `skill-hub-mark-dark.svg`: square mark for dark backgrounds.

PNG renders go to `/tmp/skillhub-docs-rewrite/logo/` for review; they are not committed.

## Design inputs

- Screenshots: `docs/images/web/home.png`, `skills-list.png`, `dark-skills-list.png` (sample the real colors).
- Tokens of the default theme `precision` (`web/src/state/appearance.ts:49`,
  `web/src/design-system/themes/precision.css`), including its display font Fraunces (bundled through
  `@fontsource/fraunces`, imported in `web/src/design-system/fonts/precision.ts:1`).
- Skill: try `/ak:frontend-design` (or spawn the `ui-ux-designer` agent) for the concept. If the Skill tool is
  unavailable, follow the screenshots and theme tokens directly and say so in the report.

## Requirements

- The mark: a diamond-based shape that reads at 16 px (no thin strokes below 1.5 px at 16 px, no inner text).
  Keep it recognizable as the app's diamond, refined (for example a faceted or layered diamond); avoid generic
  clip-art.
- The wordmark: "Skill Hub" in a serif face, converted to outline paths so GitHub renders the same glyphs on
  every system (an SVG shown through `<img>` cannot load web fonts). Use Fraunces (SIL OFL, which allows
  outlining). Outline with a script in `/tmp/skillhub-docs-rewrite/logo/` using `opentype.js` installed into
  that folder; load the font from the `.woff` file of the installed `@fontsource/fraunces` package by resolving
  it inside the script with `createRequire(<worktree>/web/package.json)` (never type that install path in a
  shell command; a hook blocks it). Fallback if outlining fails: `<text>` with
  `font-family="Fraunces, Georgia, 'Times New Roman', serif"`, flagged in the report as a known rendering risk.
- Colors: light variant uses the sage green and the dark ink of the light theme; dark variant uses colors that
  meet WCAG AA contrast (>= 3:1 for the graphic, >= 4.5:1 for the wordmark) on GitHub's dark background
  `#0d1117`; light variant the same on `#ffffff`.
- SVG hygiene: `viewBox` set, no fixed `width`/`height` other than as intrinsic size hints, no embedded raster,
  no external references, no scripts, no `<style>` with `@import`, `role="img"` and a `<title>Skill Hub</title>`.
  Each file <= 10 KB (aim far lower); valid XML (`python3 -c "import xml.dom.minidom,sys;
  xml.dom.minidom.parse(sys.argv[1])" <file>`).

## Steps

1. Draft the concept (skill or agent), 2-3 mark options; pick one and record why in the report.
2. Build the four SVGs; outline the wordmark.
3. Render each SVG to PNG at 16, 32, 128 and 512 px wide (horizontal logo: also 200 and 400 px wide, the README
   size) on white and on `#0d1117`, with a short Node script that uses Playwright's Chromium from the web package
   (`createRequire(<worktree>/web/package.json)('playwright')` or `@playwright/test`), writing into
   `/tmp/skillhub-docs-rewrite/logo/`.
4. Look at every PNG (read the images). Fix anything blurry, clipped, low-contrast or unreadable at 16 px.
5. Commit only the four SVGs (`docs(brand): add the Skill Hub logo and mark`).
6. Hand the PNG folder path to the lead for the visual review.

## Validation

- Four SVGs exist, each valid XML, <= 10 KB, with `viewBox`, `<title>` and no external references
  (`grep -E "href=\"http|<image|@import|<script" docs/images/brand/*.svg` returns nothing).
- Renders at 16, 32, 128, 512 px were looked at; the report lists what was checked and any compromise.
- The lead approves the look before Phase 16 wires it in.

## Risks

- Font outlining tool unavailable offline: fallback described above, flagged.
- Over-designed mark that fails at 16 px: the 16 px render is the gate.

## Rollback

Revert the commit; the README header falls back to the plain H1 from Phase 9.
