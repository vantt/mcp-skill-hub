# README body rewrite report

## What changed

`README.md` rewritten (129 lines, budget 130): header placeholder comment, `# Skill Hub`, tagline "One skill
library. Every agent. Every project.", 3-line pitch, `<picture>` hero (`skills-list.png`, dark twin
`dark-skills-list.png`, alt and caption from the manifest), then the frozen H2s: Why Skill Hub, How it works,
Quickstart, See it, Works with, Documentation, Contributing and license. Mermaid block is byte-identical to the
synthesis (checked with `diff`). Old install details and Development section were dropped from the README; their
new homes are `docs/installation.md` and `CONTRIBUTING.md` (owned by other phases).

## Verification (sandbox `SB=/tmp/skillhub-docs-rewrite/readme`, wave 0 binary)

- `skillhub init ~/skillhub --yes`: exit 0.
- `git -C ~/skillhub add .gitignore .skillhub/schema-version` + commit: 2 files, as in the frozen map.
- `skillhub connect --workspace ~/skillhub --host claude --yes` in a fresh project: exit 0; wrote `.mcp.json`,
  `.claude/` (including `skills/system-curator`), `CLAUDE.md`. So the "curate my Skill Hub" prompt has the
  curator skill available.
- `skillhub doctor`: "Workspace is healthy."
- `skillhub serve web --loopback-only --no-open` (stopped with `timeout`): prints
  `Skill Hub web UI: http://127.0.0.1:7421/#token=<token>`.
- Flags checked against `/tmp/skillhub-docs-rewrite/help/help-{init,connect,serve}.txt`.
- Installer one-liners copied verbatim from the old README (not run).
- Grep over the README for avoid-terms, banned claims, hype words, `add -A`, `--version`, "restart", `inbox`,
  `insight`, "server" and exclamation marks: none in prose (only `--host claude` as a flag).

## Decisions

- See it shows `skillhub serve web --loopback-only` instead of bare `skillhub serve web`: bare serve listens on
  all interfaces over plain HTTP on multi-interface machines, and the README has no room for the WARNING. The
  web UI guide covers network options.
- "a single binary, no server to run" became "One program, nothing to keep running" because the glossary bans
  "server" in user prose.
- Quickstart `doctor` output is stated in the sentence above the block (not a separate `text` block) to stay
  within the line budget.
- The `-g` hint is the one TIP alert in the Quickstart.

## Links to files not yet present at commit time

`docs/getting-started.md`, `docs/installation.md`, `docs/sources-and-learning.md`, `docs/web-ui.md`,
`docs/troubleshooting.md`, `docs/command-reference.md`, `docs/README.md` are written by parallel phases. The link
check at the end of the plan must confirm them.

## Unverified

- What the agent answers to "curate my Skill Hub" in a live session (README says modestly "Your agent shows the
  state of your hub and suggests one next step", consistent with the Claude Code evidence in
  `docs/mcp-compatibility-matrix.json`).
- The installer scripts.

Status: DONE
Summary: README body rewritten within budget with verified Quickstart commands, honest agent table and the exact
mermaid diagram.
