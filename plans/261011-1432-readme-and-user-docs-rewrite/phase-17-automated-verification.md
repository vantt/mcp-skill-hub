---
phase: 17
title: "Automated verification"
status: pending
priority: P2
effort: "1.5h"
wave: 3
owner: lead
dependencies: [2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]
---

# Phase 17: Automated verification

## Goal

Prove mechanically that the doc set is consistent: links, anchors, commands, images, diagrams and builds. No CI
check covers docs links, so this local run is the gate. The phase is read-only; findings go to the lead's fix
pass, after which the whole checklist runs again.

## Owned files

None in the repo. Scripts and outputs live in `/tmp/skillhub-docs-rewrite/verify/`. The lead's fix pass after
this phase may edit any doc file, serially, one commit per logical fix.

Scope of "the doc set": `README.md`, `CONTRIBUTING.md`, `docs/README.md` and every `docs/*.md` touched by this
branch (`getting-started`, `installation`, `user-guide`, `web-ui`, `troubleshooting`, `curating-skills`,
`sources-and-learning`, `command-reference`, `release-runbook`). Links from these files into untouched docs are
checked too (target must exist).

## Checklist (run in order; each step prints PASS or a list of failures)

1. **Relative links and anchors.** A Python script (`python3 -I verify_links.py <files>`) that:
   - extracts Markdown links `[..](target)` and HTML `src=`/`href=`/`srcset=` values, skipping `http(s):` and
     `mailto:`;
   - resolves each path relative to the file; the target must exist;
   - for `#anchor` (same file or `file.md#anchor`) builds GitHub slugs of every heading in the target
     (lowercase; strip everything except letters, digits, spaces, hyphens and underscores; spaces to hyphens;
     duplicate slugs get `-1`, `-2`) and checks the anchor is among them;
   - asserts the survivors: `README.md#quickstart`, `docs/user-guide.md#what-gets-written`,
     `docs/user-guide.md#do-common-tasks-in-the-web-ui`, and every stub heading from the plan's move map.
2. **Commands against help.** Extract every line starting with `skillhub ` inside fenced code blocks (bash,
   powershell, text excluded); for each, take the command path (words up to the first flag or placeholder) and
   compare every `--flag` with `/tmp/skillhub-docs-rewrite/help/` (refresh with `skillhub help <path>` using the
   Phase 1 binary under a sandbox HOME). Any unknown command or flag fails. Also fail on `skillhub --version`.
3. **Banned text.** `grep -nE` over the doc set for: `git add -A`, `git add \.`, `--version`, `semantic`,
   `AI-powered`, `embedding`, `simply`, `\bjust\b`, `\beasy\b`, `powerful`, `!` at the end of a sentence outside
   code, `wave`, `2\.1\.296`, `connect --hub`, `remote hub`, `Restart your agent`, `require at least 3`,
   `--skill pdf`, `plans/`, emoji (any char in U+1F300-U+1FAFF or U+2600-U+27BF outside code). Review each hit;
   code blocks and the release runbook's maintainer wording may be legitimate.
   Avoid-terms in `README.md`: `catalog generation`, `served`, `canonical`, `digest`, `proposal ID`,
   `provenance`, `vendored`, `3-way`, `daemon`, `host` (any hit in README fails).
4. **Agent claim check.** Every mention of Codex or Gemini CLI in the doc set is qualified as not verified end to
   end, or sits in the "What gets written" table.
5. **Images.** Every referenced image exists; every `<img>` and `![]()` has non-empty alt text; each file is
   <= 300 KB (`stat -c %s`); `docs/images/brand/*.svg` are valid XML (`python3 -c "import xml.dom.minidom..."`)
   and have no external references; alt text of `docs/images/web/*` images equals the manifest alt.
6. **Mermaid.** Extract each ```mermaid block to `.mmd` and render with
   `npx -y @mermaid-js/mermaid-cli -i x.mmd -o x.svg` from `/tmp/skillhub-docs-rewrite/verify/` (network needed
   once). Any render error fails. The README block must equal the synthesis block.
7. **Line budgets.** `wc -l` against the table in plan.md (README <= 130, getting-started 150-220, installation
   <= 90, CONTRIBUTING <= 60, user-guide <= 250, web-ui <= 200, troubleshooting <= 150, curating-skills <= 350,
   sources-and-learning <= 280, command-reference <= 90, docs/README <= 50, release-runbook about 160).
8. **First-win replay.** In a fresh sandbox (`SB=/tmp/skillhub-docs-rewrite/verify-firstwin`) copy every command
   from `docs/getting-started.md` in order, run it, and compare the output with the `text` blocks on the page.
   The resolve step must print "Resolved commit-style". Repeat the README Quickstart block.
9. **Builds and tests.**
   ```bash
   cd /home/vantt/projects/mcp-skill-hub-docs
   make check
   make web-check
   go test ./internal/hostintegration -run TestMatrixSynchronized
   ```
   All must pass. `make check` runs the full suite with isolated HOME/XDG, so it does not touch agent config.
10. **Scope check.** `git diff --stat main...HEAD` lists only the owned files of phases 1-16 plus the plan folder;
    nothing under `docs/design/`, `docs/contracts/`, `docs/PRD.md`, `system-skills/`, `AGENTS.md`, `schemas/`,
    `internal/` or `web/e2e/`; `docs/mcp-compatibility-matrix.json` unchanged; under `docs/images/web/` only
    `home.png` and `manifest.json`.
11. **Fallbacks.** If Phase 11 did not land, remove the `home.png` image from `docs/web-ui.md` in the fix pass.
    If Phase 10 did not pass the lead's visual review, the README keeps the plain H1 header.

## Fix pass (lead, after 17 and 18 report)

Merge the findings of both phases into one list; fix serially, one commit per file or concern, with
explicit-path `git add`; then re-run steps 1-8 of this checklist. Do not reopen decided items to fix a finding;
if a finding conflicts with a decision, list it for the owner.

## Acceptance criteria

- Every step prints PASS after the fix pass; the outputs are kept in `/tmp/skillhub-docs-rewrite/verify/` and
  summarized in the lead's handoff.

## Rollback

Nothing to roll back (read-only); fix-pass commits revert individually.
