# Logo set report

Status: done, awaiting the lead's visual approval.

## Files

- `docs/images/brand/skill-hub-logo-light.svg` (5133 B, viewBox 0 0 228 48)
- `docs/images/brand/skill-hub-logo-dark.svg` (5133 B)
- `docs/images/brand/skill-hub-mark-light.svg` (355 B, viewBox 0 0 32 32)
- `docs/images/brand/skill-hub-mark-dark.svg` (355 B)

PNG renders for review: `/tmp/skillhub-docs-rewrite/logo/png/` (40 files: mark at 16, 32, 128, 512 px;
logo at 16, 32, 128, 200, 400, 512 px; each on `#ffffff` and `#0d1117`). Overview sheets:
`/tmp/skillhub-docs-rewrite/logo/options.png` (three options), `final-sheet.png`, `zoom-small.png` (16 and
32 px renders magnified). Build scripts in the same folder: `build.mjs`, `render.mjs`, `sheet.mjs`,
`zoom.mjs`, `contrast.mjs`.

## Concept

The `/ak:frontend-design` skill was loaded (preserve route: brief taken from the existing precision theme
and screenshots). Brief: brand register, restrained color, sage accent of the default `moss` accent
(`#84b577`), Fraunces serif wordmark, signature = the app's diamond made into a cut gem.

Three marks were drawn and compared at 16 to 128 px (`options.png`):

1. **Faceted** (chosen): four facets meeting at a raised center point, lit from the top left. Reads as the
   app's diamond at 16 px and as a refined gem at large sizes; no strokes, no inner text.
2. Split: left/right two-tone halves. Clear but flat and generic.
3. Table: inner light diamond in a dark diamond. At 16 px the inner shape collapses to a blur.

A full diamond in the mid tone sits under the facets to hide anti-aliasing seams between them (seen in
the first 32 px render, fixed).

## Wordmark

"Skill Hub" in Fraunces 600, outlined to paths with `opentype.js` from the `.woff` of the installed
`@fontsource/fraunces` package (SIL OFL allows outlining). No `<text>`, so GitHub renders the same glyphs
everywhere. Path data is relative with one decimal to keep each logo near 5 KB.

## Colors and contrast (WCAG)

| Variant | Facets (lightest to darkest) | Weakest facet | Wordmark |
|---|---|---|---|
| Light, on `#ffffff` | `#6f9f62` `#5f8f54` `#4f8545` `#3f7a35` | 3.08:1 | `#1c1a15` 17.38:1 |
| Dark, on `#0d1117` | `#b3d6a9` `#9bc78f` `#84b577` `#6f9f62` | 6.14:1 | `#f0ece2` 16.04:1 |

Compromise: the app's own sage `#84b577` is only 2.37:1 on white, so the light variant uses darker sage
steps from the moss scale (`#5f8f54` is the theme's moss press color, `#3f7a35` its light-scheme accent).
The dark variant uses `#84b577` itself. Ink colors are the light and dark theme text colors.

## Checks

- Each file parses with `xml.dom.minidom`; each has `viewBox`, `role="img"` and `<title>Skill Hub</title>`;
  `width`/`height` equal the viewBox (intrinsic hints only).
- `grep -E 'href="http|<image|@import|<script|<text|<style' docs/images/brand/*.svg` returns nothing.
- Looked at: mark at 16 and 32 px (diamond and facet split readable, no seams, edges not blurred beyond
  normal anti-aliasing), mark at 128 and 512 px (clean facets), logo at 200 and 400 px (README size; wordmark
  crisp, mark vertically centered on the cap height), logo at 512 px.
- Expected limit: the horizontal logo at 16 and 32 px wide is unreadable, as any wordmark would be; use the
  mark at those sizes.

## Not done (out of scope)

- Favicon for the web UI: follow-up, as the plan says.
- Wiring into the README header: a later phase.
