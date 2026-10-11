---
phase: 5
title: "Write docs/web-ui.md"
status: pending
priority: P2
effort: "1.5h"
wave: 1
dependencies: [1]
---

# Phase 5: Write docs/web-ui.md

## Audience and goal

A user who prefers clicking to typing. The page shows each common task once, with one picture next to the steps,
and says plainly what the web UI never does (commit, push, approve content).

## Owned files

- Create: `docs/web-ui.md` (<= 200 lines).

## Sources to read

- `docs/user-guide.md:219-369` (Local Web UI, tasks, network binding): the main source; rewrite, do not
  paste.
- `README.md:66-97` (Web UI and network notes).
- `docs/images/web/manifest.json` (captions and alt text).
- Style guide and glossary in plan.md.

## Required sections (proposed H2s)

1. Intro: what the web UI is for (one or two sentences) and the never-does line.
2. `## Start the web UI`: `skillhub serve web`; the printed URL format with the token shown as a placeholder;
   `--no-open`.
3. `## Do common tasks`: H3 per task, verb-first, sentence case: See what needs your attention, Find a skill,
   Check one skill (including the Resources tab and the content approval command), Activate, deprecate or
   archive a skill, Fix a skill's description and routing fields, Create a skill, Add skills from GitHub, Act on
   upstream updates, Manage one skill's sources, Check sources, Hand a source to your curator agent, Change the
   theme. Keep the task texts close to the current ones; they were walked through and signed off.
4. `## Keep it safe on your network`: `<details>` with the binding rules, `--loopback-only`, `--allow-host`,
   `--addr`; one WARNING alert for plain HTTP.

## Commands to verify (`SB=/tmp/skillhub-docs-rewrite/web-ui`)

`skillhub serve web --no-open` (start in a sandbox workspace, read the printed line, stop it);
`skillhub help serve` for `--loopback-only`, `--allow-host`, `--addr`, `--no-open`; `skillhub skill archive
--help` form via `skillhub help skill`; `skillhub skill outdated --check` (help only, no network);
`skillhub source backfill --skill <skill-id>` (help only); the approval command form
`skillhub skill edit <skill-id> --approve-content <digest>` (help only). Do not claim UI behavior you cannot
see in the screenshots or in the current, signed-off text; list anything else as unverified.

## Images (one per task, from the manifest; width 800, phone 260)

| File | Use under | Alt (from manifest) | Caption (from manifest) |
|---|---|---|---|
| `home.png` | See what needs your attention | The Skill Hub home screen showing the status of the hub and the next action. | Home: what needs attention, and the one next step to take. |
| `skills-list.png` | Find a skill | The skills list with a state filter and skills that are draft, active or deprecated. | Skills: every skill with its state, and filters to find one. |
| `mobile-skills.png` | Find a skill (width 260) | The skills list on a narrow phone screen. | The same list on a 390 px wide screen. |
| `skill-review.png` | Check one skill | The Review tab of an active skill showing its content check and readiness. | Review: whether agents can use this skill, and what to check first. |
| `skill-review-draft.png` | Check one skill | The Review tab of a draft skill listing the missing fields with links to fix each one. | A draft that cannot be activated yet, with links to the fields to fix. |
| `skill-activate-confirm.png` | Activate | A confirmation dialog asking to activate a skill, listing what will change. | Activate: the confirmation that shows what changes before anything is saved. |
| `editor-preview.png` | Fix fields | The editor tab with a dialog that previews the changed description as a diff. | Editor: preview the exact change before it is saved. |
| `editor-conflict.png` | Fix fields | The conflict drawer explaining that the skill changed elsewhere and offering ways to continue. | Conflict: the skill changed somewhere else while the form was open. |
| `create-skill.png` | Create a skill (inside `<details>`, it is tall) | The create skill form with hints under the fields. | Create a skill: the form tells you which fields are needed to activate it later. |
| `add-from-github-discover.png` | Add skills from GitHub | The add from GitHub form with a repository address filled in. | Add from GitHub: paste a repository address to find skills in it. |
| `sources.png` | Check sources | The sources list with one source and the result of its last check. | Sources: where skills learn from, and when each was last checked. |
| `distill-handoff.png` | Hand a source | The distill hand-off screen with a brief ready to copy for the curator agent. | Distill: copy the brief and give it to your curator agent. |
| `dark-skills-list.png` | Change the theme | The skills list in the dark theme. | The same list in the dark theme. |

`home.png` is re-shot by Phase 11 in parallel; reference the same path. If Phase 11 fails, Phase 17 removes the
`home.png` image from this page and keeps the text.

## Anchors

Other files link to `web-ui.md#start-the-web-ui`, `#do-common-tasks` and `#keep-it-safe-on-your-network` (or
the frozen equivalents). The user guide keeps the stub `#do-common-tasks-in-the-web-ui` pointing here.

## Acceptance criteria

- <= 200 lines; style guide met; one callout per screen; every image <= 300 KB with manifest alt text.
- UI labels in bold exactly as the screenshots show them.
- Never-does sentence present; no banned claim (no automatic checks: "Check now" runs only when clicked).
- Commands verified against `skillhub help`; links resolve.

## Rollback

Revert the commit.
