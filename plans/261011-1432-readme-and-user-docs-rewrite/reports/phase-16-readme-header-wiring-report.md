# README header wiring report

## What changed

`README.md` header placeholder comment replaced by: `<picture>` logo (light `skill-hub-logo-light.svg`, dark
`skill-hub-logo-dark.svg`, alt "Skill Hub", width 280), `# Skill Hub` H1 kept, tagline unchanged, badges
(latest release, CI, license MIT), link row Quickstart · User guide · Curating skills. No stars, downloads,
coverage or "PRs welcome". No `home.png`. Body and license line untouched.

README is 130 lines (budget 130). To fit, the `<picture>` sits on one line directly above the H1 (an ATX heading
may interrupt a paragraph), and tagline, badges and link row are one paragraph joined with backslash hard
breaks.

## Verification

- `gh api markdown` (gfm) on the header: GitHub wraps the logo in `<themed-picture>`, renders `<h1>Skill Hub</h1>`,
  then tagline, badges and link row on three lines (`<br>`).
- `curl -sIL`: release badge 200 `image/svg+xml` (shows v0.3.0); CI badge 200 `image/svg+xml`; license badge 200
  `image/svg+xml`; `releases/latest` 302 then 200; `actions/workflows/ci.yml` 200. `ci.yml` has `name: CI`.
- `ls LICENSE docs/images/brand/*.svg docs/user-guide.md docs/curating-skills.md`: all present. `## Quickstart`
  exists, so `#quickstart` resolves.

## Concerns

- The CI badge currently renders "CI - failing" for `main` on GitHub. It resolves, so it stays per the phase
  rule; the owner may want CI green before publishing the README, or drop the badge.
- Light/dark visual check of the header is left to the preview phase.

Status: DONE_WITH_CONCERNS
