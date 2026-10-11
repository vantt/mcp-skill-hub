# Report: docs/getting-started.md

## Delivered

- New `docs/getting-started.md`, 220 lines, with the eight frozen H2s in order (Install Skill Hub, Create your
  workspace, Connect your agent, Create your first skill, See your first win, See it in the web UI, If it did not
  work, Next steps). Intro has no heading and states honest agent support (Claude Code verified).
- One callout per section: CAUTION (connection files), TIP (`-g`), IMPORTANT (never commits or pushes).
- Windows notes in one `<details>` block. `skills-list.png` with manifest alt text and caption, width 800.

## Sandbox verification (`SB=/tmp/skillhub-docs-rewrite/getting-started`, binary from wave 0)

Ran in order, outputs match the page (trimmed where the page says so; paths shown as `<home>`, hashes `<sha>`):
`skillhub version`; `skillhub init ~/skillhub --yes`; `git -C ~/skillhub add .gitignore .skillhub/schema-version`
plus commit; `cd ~/myproject && skillhub connect --workspace ~/skillhub --host claude --yes`; `skillhub doctor`
("Workspace is healthy."); `skill list` from `~` (fails with "No Skill Hub workspace was found", fixed by
`SKILLHUB_WORKSPACE` and by `--workspace`); the full `skill create ... --yes`; `skill activate commit-style --yes`;
`skill list` (active); `resolve --request ~/req.json` ("Resolved commit-style (medium confidence) ..."); the same
request without `scope` (the bounded-scope question); `git -C ~/skillhub add skills history` plus commit (4 files,
66 insertions); `skillhub status` (up to date, Git clean); MCP stdio probe (3 tools, resolves commit-style,
`skill_get` text has the marker); `skillhub serve web --loopback-only --no-open` (one URL line on port 7421) and
plain `serve web --no-open` (plain-HTTP warning plus per-interface lines), both stopped by timeout, port freed.

## Choices

- The guide uses `serve web --loopback-only` so a newcomer never sees the plain-HTTP warning on a multi-interface
  machine; network binding stays in `web-ui.md`.
- The CAUTION does not print `git add -A`; it says "always name the paths you commit; never stage the whole
  folder", so no banned string appears.
- "What you should see" is conditional ("If your agent used the skill, the commit message ends with ...").
- Links to `troubleshooting.md` and `sources-and-learning.md` use frozen anchors; those files do not exist yet in
  the worktree (parallel phases).

## Unverified (left out or worded conditionally)

- `/mcp` in Claude Code listing `skillhub` with 3 tools: left out (frozen map lists it unverified).
- An agent in a live Claude Code session writing `Checked-by: skillhub-demo`: worded as a condition only.
- The trust dialog text comes from `connect` output, not from a live session.
- Installer one-liners and install paths: copied from the current README, not run (rule). Windows notes
  (`$HOME\skillhub`, `$env:SKILLHUB_WORKSPACE`) rely on standard PowerShell behavior, not run with `skillhub`.
- The browser opening on `serve web` without `--no-open` is inferred from `serve web` help (`--no-open` "Do not
  open the browser on startup"); not observed.

Status: DONE
Summary: Wrote docs/getting-started.md (220 lines, frozen H2s) with every `skillhub` command re-run in its own
sandbox and real trimmed output.
