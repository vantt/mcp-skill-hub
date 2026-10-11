---
phase: 9
title: "Rewrite README.md body"
status: completed
priority: P2
effort: "1.5h"
wave: 1
dependencies: [1]
---

# Phase 9: Rewrite README.md body

## Audience and goal

A developer landing on the GitHub page. In one screen they learn what Skill Hub is worth to them; in a minute
they can install it and connect a project; then they follow "Get started" to their first win. Sales-friendly
but true: every sentence must survive the banned-claims list.

## Owned files

- `README.md` (<= 130 lines). Phase 16 takes over the header block in wave 2.

## Sources to read

- Synthesis: "Value story", "README outline", header block, mermaid diagram, "Voice and presentation",
  "Claims that must NOT appear".
- `reports/wave0-frozen-map.md` (verified quickstart commands, safe add list).
- Current `README.md` (all of it; its install text moves to `docs/installation.md` and its Development section
  to `CONTRIBUTING.md`).
- Style guide and glossary in plan.md.

## Required sections (from the decided outline)

0. Header block placeholder: write `<!-- header: logo, tagline, badges, link row (wired in a later step) -->`
   followed by a plain `# Skill Hub` H1 and the tagline line. Choose the tagline from the three in the synthesis
   (recommend "One skill library. Every agent. Every project." but the agent table must keep the honesty rule).
1. Pitch: 2-3 sentences (<= 5 lines), benefit first.
2. Hero image: `<picture>` with `docs/images/web/dark-skills-list.png` for `prefers-color-scheme: dark` and
   `docs/images/web/skills-list.png` as default, width 800, alt and caption from the manifest. Not `home.png`.
3. `## Why Skill Hub`: 5 benefit bullets (your agent asks for the right skill; you approve every change; a single
   binary, no server to run; Git is the database; a local web UI).
4. `## How it works`: 3 steps plus the mermaid diagram copied exactly from the synthesis.
5. `## Quickstart` (heading text must stay "Quickstart"): install one-liners (Windows in `<details>`), `init` and
   commit the safe add list, `connect --host claude`, open a new Claude Code session in that folder, one prompt.
   Ends with a link: "Add your first skill -> [Get started](docs/getting-started.md)".
6. `## See it`: `skillhub serve web`, one screenshot (`skill-review.png`), link `docs/web-ui.md`.
7. `## Works with`: honest table: Claude Code (verified), Codex and Gemini CLI (connection files written, not
   yet verified end to end); link `docs/mcp-compatibility-matrix.json`.
8. `## Documentation`: links grouped Start (Get started, Installation), Guides (Using Skill Hub, Curating
   skills, Sources and learning, Web UI, Troubleshooting), Reference (Command reference, docs index),
   Maintainers (Contributing, release runbook). No design records here.
9. `## Contributing and license`: one line linking `CONTRIBUTING.md`; license line "Skill Hub is released under
   the MIT License, see [LICENSE](LICENSE)." (the file arrives in Phase 4; Phase 17 checks the link).

The one prompt in the Quickstart: use the one Phase 1 verified as meaningful without a skill (for example "Curate
my Skill Hub", which the bundled `system-curator` skill answers); describe the result modestly ("your agent
shows the state of your hub and suggests one next step") and only if Phase 1 confirms the curator skill is
installed by `connect`.

## Commands to verify (`SB=/tmp/skillhub-docs-rewrite/readme`)

The Quickstart block exactly as written (except the installer one-liners, copied verbatim): `skillhub init
~/skillhub --yes`, the safe add and commit, `skillhub connect --workspace ~/skillhub --host claude --yes`,
`skillhub doctor`, `skillhub serve web --no-open` (start and stop).

## Images

- Hero: `skills-list.png` and `dark-skills-list.png` (alt "The skills list with a state filter and skills that
  are draft, active or deprecated."; dark alt "The skills list in the dark theme.").
- See it: `skill-review.png` (alt "The Review tab of an active skill showing its content check and readiness.",
  caption "Review: whether agents can use this skill, and what to check first.").

## Anchors

`#quickstart` must survive. Do not link to anchors outside the frozen list.

## Acceptance criteria

- <= 130 lines including the placeholder header; style guide met; no avoid-terms anywhere in the README.
- No banned claim; the agent table is honest; no `git add -A`.
- Mermaid block identical to the synthesis (it was validated).
- Quickstart commands verified in the sandbox.

## Rollback

Revert the commit; the old README returns.
