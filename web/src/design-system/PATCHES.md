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

## Vietnamese coverage

| Theme | Slot | Primary Family | Native Subset | Fallback in Stack |
|---|---|---|---|---|
| `precision` | `--font-display` | Newsreader | Yes | Georgia, serif |
| `precision` | `--font-body` | Geist | Yes | system-ui, sans-serif |
| `precision` | `--font-mono` | Geist Mono | Yes | ui-monospace, monospace |
| `precision` | `--font-accent` | Fraunces | Yes | Georgia, serif |
| `atelier` | `--font-display` | Manrope | Yes | system-ui, -apple-system, sans-serif |
| `atelier` | `--font-body` | Manrope | Yes | system-ui, -apple-system, sans-serif |
| `atelier` | `--font-mono` | ui-monospace | Yes (system) | 'SF Mono', Menlo, monospace |
| `atelier` | `--font-accent` | Manrope | Yes | system-ui, sans-serif |
| `berich` | `--font-display` | Spectral | Yes | Georgia, serif |
| `berich` | `--font-body` | Be Vietnam Pro | Yes | system-ui, sans-serif |
| `berich` | `--font-mono` | IBM Plex Mono | Yes | ui-monospace, monospace |
| `berich` | `--font-accent` | Spectral | Yes | Georgia, serif |
| `clickup` | `--font-display` | Plus Jakarta Sans | Yes | Inter, sans-serif |
| `clickup` | `--font-body` | Inter | Yes | 'Plus Jakarta Sans', sans-serif |
| `clickup` | `--font-mono` | Sometype Mono | No (omitted) | ui-monospace, monospace |
| `clickup` | `--font-accent` | Plus Jakarta Sans | Yes | Inter, sans-serif |
| `moday` | `--font-display` | Poppins | No (omitted) | 'Figtree', system-ui, sans-serif |
| `moday` | `--font-body` | Figtree | No (omitted) | system-ui, -apple-system, sans-serif |
| `moday` | `--font-mono` | ui-monospace | Yes (system) | 'SF Mono', Menlo, monospace |
| `moday` | `--font-accent` | Poppins | No (omitted) | system-ui, sans-serif |
| `terminal` | `--font-display` | JetBrains Mono | Yes | ui-monospace, monospace |
| `terminal` | `--font-body` | JetBrains Mono | Yes | ui-monospace, monospace |
| `terminal` | `--font-mono` | JetBrains Mono | Yes | ui-monospace, monospace |
| `terminal` | `--font-accent` | JetBrains Mono | Yes | ui-monospace, monospace |

## Accessibility Patches

- `contract/components.css`: `.fg-table thead th` changed from `color: var(--color-text-subtle)` to `color: var(--color-text-muted)` to satisfy WCAG AA 4.5:1 minimum color contrast ratio for small text on light backgrounds.
- `contract/components.css`: `.fg-btn--danger` set `color: #ffffff` to achieve >4.5:1 contrast against `--color-danger` (`#b23b34`).
- `themes/precision.css`: adjusted light scheme `--color-warning` from `#8a6410` to `#805c08` to achieve 5.08:1 contrast ratio against `--color-warning-tint` (`#f4e9cf`) for `.fg-chip--warning`.
- `themes/precision.css`: adjusted light scheme `--color-text-subtle` from `#847a63` to `#685f4b` to achieve ≥4.5:1 contrast on diff addition and deletion backgrounds. Dark scheme `--color-text-subtle` set to `var(--_ink-600)`.
- `themes/precision.css`: adjusted light scheme `--color-info` to `#764d08`, `--color-success` and `--color-positive` to `#386e30` to guarantee WCAG AA 4.5:1 contrast against their corresponding background tints.
