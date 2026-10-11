---
phase: 18
title: "Style, consistency and rendering review"
status: pending
priority: P2
effort: "1.5h"
wave: 3
dependencies: [2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]
---

# Phase 18: Style, consistency and rendering review

## Goal

A human-eye pass that the scripts in Phase 17 cannot do: does the reader path work, do all pages sound like one
voice, and does everything render well on GitHub in light mode, dark mode and at phone width. Read-only; the
reviewer writes one report and the lead fixes.

## Owned files

- Only the reviewer's report in `plans/261011-1432-readme-and-user-docs-rewrite/reports/`.

## Checklist

1. **Reader path.** Read README -> getting-started -> user-guide -> curating-skills -> sources-and-learning as a
   newcomer. Note every place where a term is used before it is explained, a step is missing, or a link sends
   the reader backwards without reason.
2. **Style guide.** Check each page against the 14 rules in plan.md; sample at least 20 sentences per page for
   length (<= 20 words) and paragraphs (<= 3 sentences); headings verb-first in sentence case; one callout per
   screen; code block conventions (language tag, no `$`, output in separate `text` blocks, `<skill-id>`
   placeholders).
3. **Glossary consistency.** The same word for the same thing across pages ("workspace" not "repo", "agent" not
   "host", "web UI", "content approval", "learning reference", "lesson"); each term explained once at first use.
4. **Claims.** Re-read every benefit sentence against the banned-claims list and the value story; flag anything
   stronger than the product does (especially what the reader "will see" in Claude Code).
5. **Rendering.** Render the doc set the way GitHub does and look at it:
   - Preferred: push nothing; use `gh markdown-preview` if installed, or render with the GitHub Markdown API
     (`gh api -X POST /markdown -f mode=gfm -f context=vantt/mcp-skill-hub -f text=@README.md`) into HTML
     wrapped with GitHub's light and dark Primer CSS, then take Playwright screenshots at 1280 px (light and
     dark) and 390 px, using the web package's Chromium through a script in `/tmp/skillhub-docs-rewrite/render/`.
   - Check: the `<picture>` logo switches between light and dark; badges render; GitHub alerts render as alerts;
     `<details>` blocks collapse; images are 800 px (phone shot 260) and readable; the mermaid diagram renders
     (GitHub renders mermaid client-side; if the API HTML does not, rely on Phase 17 step 6 and note it); tables
     do not overflow badly at 390 px.
   - Look at each screenshot (read the images) and list problems with the page and section.
6. **Pre-publish checklist (from the synthesis).** Every link and anchor resolves (from Phase 17); every command
   was run on a clean HOME and the shown output matches (from Phase 17 step 8); every image exists, has alt text
   and is <= 300 KB; rendered preview checked in light, dark and mobile width; spelling and product names
   (Claude Code, Codex, Gemini CLI, GitHub, Git, Skill Hub); tone pass against the style guide; badges point to
   real targets.
7. **Brand.** The logo looks right next to the H1 and tagline in both themes, and the mark at small size.

## Acceptance criteria

- One report with findings grouped by file, each with the exact line and a proposed fix; a "decided items
  touched" list if a finding would reopen an owner decision (the lead takes those to the owner instead of fixing).
- Ends with the status block.

## Rollback

Nothing to roll back (read-only).
