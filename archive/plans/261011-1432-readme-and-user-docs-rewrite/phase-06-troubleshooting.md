---
phase: 6
title: "Write docs/troubleshooting.md"
status: completed
priority: P2
effort: "1.5h"
wave: 1
dependencies: [1]
---

# Phase 6: Write docs/troubleshooting.md

## Audience and goal

A user whose setup does not behave. Each entry is a symptom the reader can recognize (an error text or what they
see), then the fix in one or two commands. Start with `skillhub doctor`.

## Owned files

- Create: `docs/troubleshooting.md` (<= 150 lines).

## Sources to read

- `docs/user-guide.md:383-485` (Troubleshooting): main source.
- `docs/curating-skills.md:450-467` (Resource-verified fallback and state basis): becomes one plain paragraph.
- Synthesis "Traps newcomers hit".
- `reports/wave0-frozen-map.md` (verified error texts).
- Style guide and glossary in plan.md.

## Required sections (proposed H2s; each symptom is an H3 in sentence case)

1. `## Start with doctor`: `skillhub doctor`, then `skillhub doctor --fix` and `--fix --yes`; one line on what
   `--fix --yes` repairs and never touches (other workspaces).
2. `## Setup and connection`: No Skill Hub workspace was found (`SKILLHUB_WORKSPACE`, `--workspace`); The agent
   does not see the hub (new session, binary path moved, `connect` again); Trust prompts after connect (Claude
   Code; one line for Codex and Gemini CLI, marked not verified end to end); `connect -g` refuses because of a
   symbolic link; Missing permission entries.
3. `## Skills`: Activation refuses a draft (missing `--trigger`, `--not-for`, `--min-scope`); Resolve returns a
   question instead of a skill (`task.scope` missing); "review_required": content from someone else's repository
   is withheld (content approval in the terminal); "setup_failed" (`skillhub skill doctor <skill-id>`); Adding a
   large GitHub repository fails (use the working `--path` form from Phase 1).
4. `## Workspace and index`: The search index is stale (`skillhub validate`, `skillhub rebuild`); one plain
   paragraph replacing "Resource-verified fallback and state basis" with a link to `contracts/`; Fresh clone on a
   new machine (`skillhub rebuild`); Telemetry is degraded (`skillhub telemetry purge --yes`).
5. `## Windows`: `<details>` with SmartScreen and the firewall prompt.

Drop from user prose: degraded MCP startup tool lists (one plain sentence instead), `local.path` expiry
mechanics (one sentence), `validate --staged` (it belongs to curating-skills "Review and commit").

## Commands to verify (`SB=/tmp/skillhub-docs-rewrite/troubleshooting`)

Reproduce each symptom where it is safe and offline, and paste the real error text: no workspace found (run
`skillhub status` in an empty folder), activation refusal (bare `skill create` then `skill activate`), resolve
without `task.scope`, `skillhub doctor` and `skillhub doctor --fix` (preview) in a broken sandbox project (delete
`.mcp.json` first), `skillhub validate`, `skillhub rebuild`, `skillhub telemetry health`, `skillhub skill doctor
<skill-id>`. The symlink refusal: create `~/.claude` as a symlink inside the sandbox HOME and run
`skillhub connect -g --workspace ~/skillhub --yes` there (sandbox HOME only). Windows items are not runnable
here: keep their current wording, mark them unverified in the report.

## Images

None.

## Acceptance criteria

- <= 150 lines; each entry is symptom -> fix; error texts are real (copied from the sandbox run).
- No wave or version notes, no plan links, no avoid-terms outside one explained sentence.
- Links to `user-guide.md#what-gets-written`, `curating-skills.md` and `sources-and-learning.md` use frozen
  anchors.

## Rollback

Revert the commit.
