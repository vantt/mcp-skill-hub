# Design System Patches

## Deleted CDN Font Imports

Fonts are self-hosted by the app; see fonts/<theme>.ts.

### atelier.css
- `@import url('https://[google-fonts-cdn]/css2?family=Manrope:wght@400;500;600;700;800&display=swap');`
- `@import url('https://[google-fonts-cdn]/css2?family=Space+Grotesk:wght@400;500;600;700&display=swap');   /* for typeface-set "grotesk" */`

### berich.css
- `@import url('https://[google-fonts-cdn]/css2?family=Be+Vietnam+Pro:wght@300;400;500;600;700;800&family=Spectral:ital,wght@0,400;0,500;0,600;0,700;1,500&family=IBM+Plex+Mono:wght@300..600&display=swap');`
- `@import url('https://[google-fonts-cdn]/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap');   /* for typeface-set "grotesk-warm" */`

### clickup.css
- `@import url('https://[google-fonts-cdn]/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=Inter:wght@400;500;600;700&family=Sometype+Mono:wght@400;500&display=swap');`
- `@import url('https://[google-fonts-cdn]/css2?family=Space+Grotesk:wght@400;500;600;700&display=swap');   /* for typeface-set "editorial-pop" */`

### moday.css
- `@import url('https://[google-fonts-cdn]/css2?family=Figtree:wght@400;500;600;700&family=Poppins:wght@400;500;600;700&display=swap');`

### precision.css
- `@import url('https://[google-fonts-cdn]/css2?family=Fraunces:opsz,wght@9..144,400;9..144,500&family=Newsreader:opsz,wght@6..72,400;6..72,500&family=Geist:wght@300..700&family=Geist+Mono:wght@300..600&display=swap');`
- `@import url('https://[google-fonts-cdn]/css2?family=Space+Grotesk:wght@400;500;600;700&display=swap');   /* for typeface-set "grotesk" */`

### terminal.css
- `@import url('https://[google-fonts-cdn]/css2?family=JetBrains+Mono:wght@400;500;600;700&display=swap');`
- `@import url('https://[google-fonts-cdn]/css2?family=IBM+Plex+Mono:wght@300..600&family=Sometype+Mono:wght@400;500&display=swap');   /* for typeface-sets "plex" / "sometype" */`

## Font Subsets Omitted from Packages

The following font packages in `@fontsource` do not publish a `vietnamese` subset file:
- `@fontsource/sometype-mono` (lacks `vietnamese-400.css`, `vietnamese-500.css`)
- `@fontsource/figtree` (lacks `vietnamese-400.css`, `vietnamese-500.css`, `vietnamese-600.css`, `vietnamese-700.css`)
- `@fontsource/poppins` (lacks `vietnamese-400.css`, `vietnamese-500.css`, `vietnamese-600.css`, `vietnamese-700.css`)
These fonts fall back to system fonts or next font family in the theme stack for Vietnamese text.

## Accessibility Patches

- `contract/components.css`: `.fg-table thead th` changed from `color: var(--color-text-subtle)` to `color: var(--color-text-muted)` to satisfy WCAG AA 4.5:1 minimum color contrast ratio for small text on light backgrounds.
- `themes/precision.css`: adjusted light scheme `--color-warning` from `#8a6410` to `#805c08` to achieve 5.08:1 contrast ratio against `--color-warning-tint` (`#f4e9cf`) for `.fg-chip--warning`.
