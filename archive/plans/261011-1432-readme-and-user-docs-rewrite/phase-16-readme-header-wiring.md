---
phase: 16
title: "Wire the logo and license badge into the README"
status: completed
priority: P2
effort: "0.5h"
wave: 2
dependencies: [4, 9, 10]
---

# Phase 16: Wire the logo and license badge into the README

## Goal

Replace the header placeholder from Phase 9 with the decided header block: logo, tagline, badges (latest
release, CI, license MIT) and the link row Quickstart · User guide · Curating skills. Skip stars, downloads,
coverage and "PRs welcome".

## Owned files

- `README.md` (takes over from Phase 9; only the header block, plus the optional Home image below).

## Requirements

1. Logo through `<picture>`:
   ```html
   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="docs/images/brand/skill-hub-logo-dark.svg">
     <img src="docs/images/brand/skill-hub-logo-light.svg" alt="Skill Hub" width="280">
   </picture>
   ```
   The H1 `# Skill Hub` stays for accessibility and the page title (keep it, visually small is fine). Width
   chosen after looking at the render; 240-320 px.
2. Tagline line from Phase 9, unchanged.
3. Badges (each must resolve):
   - Release: `https://img.shields.io/github/v/release/vantt/mcp-skill-hub` linking to
     `https://github.com/vantt/mcp-skill-hub/releases/latest`.
   - CI: `https://github.com/vantt/mcp-skill-hub/actions/workflows/ci.yml/badge.svg` linking to the workflow
     page (`.github/workflows/ci.yml`, `name: CI`).
   - License: `https://img.shields.io/badge/license-MIT-green` (or the shields `github/license` form) linking
     to `LICENSE`.
   Open each URL with `curl -sI` (or WebFetch) and confirm a 200 and an image content type. If the release or
   CI badge does not resolve (private repo, no release), drop that badge and say so in the report.
4. Link row: `[Quickstart](#quickstart) · [User guide](docs/user-guide.md) · [Curating
   skills](docs/curating-skills.md)`.
5. Do not add `home.png` to the README: the hero stays `skills-list.png` (decided) and the "See it" image is
   `skill-review.png`.
6. The license line from Phase 9 stays; make sure `LICENSE` exists.

## Commands to verify

`curl -sI` on each badge URL; `ls LICENSE docs/images/brand/*.svg`.

## Acceptance criteria

- README <= 130 lines; header renders in light and dark (checked in Phase 18); every badge resolves;
  `#quickstart` link works.

## Rollback

Revert the commit; the plain header from Phase 9 returns.
