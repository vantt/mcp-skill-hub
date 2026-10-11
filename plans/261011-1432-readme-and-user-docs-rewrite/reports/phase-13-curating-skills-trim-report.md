# Trim docs/curating-skills.md: report

File: `docs/curating-skills.md`, 645 -> 348 lines (budget 350, stubs included).

## Headings

All surviving H2 texts are unchanged, so their anchors hold: `#start-with-the-hub-not-a-command`, `#safety-model`,
`#add-a-skill-skillhub-skill-add`, `#create-a-skill-from-your-own-workflow-skillhub-skill-create`,
`#review-diagnostic-facts-skillhub-skill-review`, `#edit-and-improve-a-skill-skillhub-skill-edit`,
`#activate-deprecate-and-archive`, `#review-and-commit-workspace-changes`, `#measure-usage-skillhub-telemetry-funnel`,
`#check-routing-quality-skillhub-eval-routing`. No heading renamed. Inbound links from command-reference,
getting-started, sources-and-learning and troubleshooting resolve. H3s were reorganized (no other page links them):
"Add from a public GitHub repository" -> "Add from GitHub"; new "After you add a skill"; the edit-section H3s and
"Staged index validation", "Commit your changes", "Import local Claude Code transcripts" were folded into prose.

Stubs (exact old text, one "Moved to" line): Keep vendored skills up to date -> `sources-and-learning.md#keep-copied-skills-up-to-date`;
Learn from references -> `#learn-from-other-repositories`; Resource-verified fallback and state basis ->
`troubleshooting.md#workspace-and-index` plus `contracts/error-codes.md`; Import repository skills as drafts
(`skillhub source import`) -> `#import-many-skills-from-a-repository`; Advanced intake and recovery ->
`#triage-candidate-repositories`; Command map -> `command-reference.md`.

## Removed paragraphs and their new home

| Removed | New home or reason |
|---|---|
| Curator internals (`--json`, MCP vs CLI, "Codex and Gemini retain one full MCP entry in this wave", pinned confirmation) | One sentence ("curates through the same commands, asks before changes"); pins in Safety `<details>` |
| Two journey blocks and goal table (incl. `distill / inbox`) | One sentence; source path in sources-and-learning |
| Safety: cooperative scope, confirmation pins, no silent overwrites | Safety `<details>` |
| Safety: local add CLI-only | Add from a local folder |
| Safety: no local watch, watch is not a daemon | sources-and-learning.md (Learn from other repositories) |
| Safety: `local.status` / `content_review_required` details | troubleshooting.md "The agent reports review_required" |
| `--skill pdf --yes` and `--all --yes` GitHub examples | Replaced by the working `--path skills/pdf` form; `--skill`/`--all` described in prose |
| Local add privacy bullets (snapshot rules, companion inventory, license warnings) | Cut; see "Contradictions" (hidden files are copied) |
| Create requirements list | Flag list under Create |
| Review's 11-item fact list (served generation, provenance, runtime hints, lint) | Two plain sentences + Content trust block from a real run |
| Editor digest conflict and 24-hour recovery details | One sentence (refuses; text kept under `runtime/edits/`); 24-hour TTL dropped |
| Explicit blind replacement note | `--content-file` sentence |
| Keep vendored skills up to date (outdated, upstream, update, re-approval, backfill) | sources-and-learning.md#keep-copied-skills-up-to-date |
| Learn from references (attach, watch, triage, check, distill/inbox) | sources-and-learning.md#learn-from-other-repositories; inbox/insight dropped (commands no longer exist) |
| Resource-verified fallback and state basis | troubleshooting.md "The search index is stale"; contracts |
| `validate --staged` guarantees (literal index blobs, no hook installer) | One sentence + "Skill Hub does not install hooks" |
| `git add -A` commit block | Explicit path list from the frozen map |
| Funnel metric list | One sentence; `skillhub help telemetry` |
| Transcript privacy (session hash, 14-day window) | One sentence ("reads only tool-call entries") |
| Eval JSON envelope contract, CI gate | `metrics.recall` sentence; exit code 1 on a missed threshold |
| Import repository skills, Advanced intake | sources-and-learning.md |
| Command map table | command-reference.md |

## Commands verified (sandbox `/tmp/skillhub-docs-rewrite/curating`)

`status`; `skill add <local-folder>` (preview and `--yes`); `skill add https://github.com/anthropics/skills --path skills/pdf`
(preview, then `skill confirm`); full `skill create` with `--content-file`; `skill review` and `--verbose` (local and
GitHub skill, Content trust block); `skill edit` routing fields, `--description` (preview + short `skill confirm`),
`--example`/`--counter-example`, `--runtime-file` (preview and `--yes`); `skill activate`, `deprecate`, `archive`
(and active -> archived refused, exit 2); `diff`; `validate`; explicit-path `git add`; `validate --staged`;
`telemetry funnel`; `eval routing`, `--json` (`metrics` keys), `--min-recall 0.9` exits 1. `--editor` checked
in code (`internal/delivery/cli/skill_editor.go`), not run.

## Contradictions found

1. The old doc said local add captures only `SKILL.md`, `references/`, `scripts/`, `assets/` and skips hidden files.
   A run copied `other/a.txt`, `.hidden/b` and `.dotfile` too. The page now says hidden files are copied.
2. `skill review` on a freshly added local skill (untracked) showed `Git: clean`; the GitHub-added skill showed
   `uncommitted changes (... 1 untracked)`. Owner follow-up 5 does reproduce for local add.
3. `skillhub diff` also suggests `add -A` (in addition to `init` and `status`).

## Unverified

- `--approve-content` itself (the agent permission rules block it here); the page only tells readers to run the
  command the review prints.
- The "changes since approval" listing and `git diff` command after an approved skill changes (read from code
  names, not run).
- `telemetry import-transcripts` (not run; claim limited to "reads only tool-call entries", from code).
- MCP rejecting local paths for `skill add` (kept from old doc as "your agent asks you to run it").

Status: DONE_WITH_CONCERNS
Summary: curating-skills.md trimmed to 348 lines with all kept anchors and six stubs; commands verified in the sandbox.
Concerns: local add copies hidden files (old doc said otherwise); `skill review` shows `Git: clean` for a local add.
