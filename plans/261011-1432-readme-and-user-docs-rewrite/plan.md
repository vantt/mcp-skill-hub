---
title: "Rewrite the README and the user-facing docs"
description: "Turn the README and the user guides into a reader path that sells the value, installs in a minute and reaches a first win with a local skill, with a real logo, an MIT LICENSE and a fixed Home copy."
status: pending
priority: P2
effort: 24.5h
branch: docs/readme-and-guides-rewrite
tags: [docs, readme, onboarding, brand, license, web]
blockedBy: []
blocks: []
created: 2026-10-11
---

# Rewrite the README and the user-facing docs

## Why (evidence)

- The README pitches support for "Claude Code, Codex, and Gemini CLI" (`README.md:3`), but only Claude Code is
  verified (`docs/mcp-compatibility-matrix.json`). Its quickstart tells readers to run `git add -A` in the hub
  (`README.md:25`), which commits machine-specific connection files that `init` writes.
- `docs/user-guide.md` (536 lines) mixes setup, web UI, troubleshooting and two command tables; it carries
  release-wave notes (`:99`, `:122`), a broken plan link (`:115`) and a resolve example without `task.scope`
  (`:175`) that never returns a match.
- `docs/curating-skills.md` (645 lines) holds a second command table (`:606`) that disagrees with the first.
- There is no `LICENSE`, no `CONTRIBUTING.md`, no docs index and no logo.
- Home shows "1 source(s) are ready to distill" (`docs/images/web/home.png`). The text comes from the Go action
  summary (`internal/app/curation_home.go:467`) and is shown as is by `web/src/screens/home/HomeScreen.tsx:94`
  (`help?.title ?? topAction.summary`).

The decisions behind this plan are in `plans/reports/docs-rewrite-discussion-synthesis.md`. Its section "Owner
decisions of 2026-10-11" overrides everything above it. Decided items are not reopened here.

## Outcome and acceptance criteria

- [ ] A reader goes README -> Get started -> first win with a local skill, and every command on that path was
      run in a sandbox with a binary built from main, with output matching what the docs show.
- [ ] The doc set matches the decided table below, each file within its line budget.
- [ ] No banned claim appears anywhere in the user docs (list below).
- [ ] The three inbound anchors survive, and every H2 that left `user-guide.md` or `curating-skills.md` has a
      "Moved to" stub at its old place.
- [ ] Every relative link and anchor in `README.md`, `CONTRIBUTING.md` and `docs/*.md` resolves; every
      `skillhub` command in a code block matches `skillhub help <cmd>`; every image exists, has alt text and is
      <= 300 KB.
- [ ] `LICENSE` (MIT, "Van Tran", 2026) exists and the README shows a license badge and one license line.
- [ ] A logo set (horizontal logo and square mark, light and dark, SVG) lives in `docs/images/brand/` and the
      README header shows it with `<picture>`.
- [ ] Home says "1 source is ready to distill" (and the plural form for more), covered by a unit test;
      `docs/images/web/home.png` and its manifest entry are refreshed.
- [ ] `make check`, `make web-check` and `go test ./internal/hostintegration -run TestMatrixSynchronized` pass.

Non-goals: changing product behavior beyond the Home copy; a starter skill; a GIF; `README.vi.md`; editing
design records, contracts, `docs/PRD.md`, `system-skills/*`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`; documenting
the remote hub (`connect --hub`); re-shooting screenshots other than `home.png`.

## Doc set (decided, from the synthesis)

| File | Action | Purpose | Budget (lines) | Phase |
|---|---|---|---|---|
| `README.md` | rewrite | pitch, 60-second quickstart, docs map | <= 130 | 9, then 16 |
| `docs/getting-started.md` | new | install -> workspace -> connect -> create a skill -> first win | 150-220 | 2 |
| `docs/installation.md` | new | install options, pin a version, signatures, upgrade, uninstall, build from source | <= 90 | 3 |
| `CONTRIBUTING.md` | new | dev commands, test isolation, link to the release runbook | <= 60 | 4 |
| `LICENSE` | new | MIT, "Van Tran", 2026 | standard text | 4 |
| `docs/web-ui.md` | new | web UI tasks with screenshots, network and security | <= 200 | 5 |
| `docs/troubleshooting.md` | new | symptom -> fix | <= 150 | 6 |
| `docs/sources-and-learning.md` | new | upstream drift and updates, watch, check, distill, inbox | <= 280 | 7 |
| `docs/command-reference.md` | new | one merged command table | <= 90 | 8 |
| `docs/user-guide.md` | rewrite as "Using Skill Hub" | concepts, connecting agents, how the agent picks a skill, tools and secrets, moving machines | <= 250 | 12 |
| `docs/curating-skills.md` | trim | add, create, review, edit, lifecycle, commit, routing quality | <= 350 | 13 |
| `docs/release-runbook.md` | trim | drop duplicated install text; link installation.md | ~160 | 14 |
| `docs/README.md` | new | docs index by audience | <= 50 | 15 |
| `docs/images/brand/*.svg` | new | logo set | small, valid SVG | 10 |
| `web/src/...` Home copy, `docs/images/web/home.png`, `manifest.json` | fix | singular/plural copy | - | 11 |

Budgets count lines of the Markdown source including stubs. Stubs count against the budget of the file they
sit in.

## Content move map (where every current section goes)

Writers of new files copy and rewrite the content from its old place; the wave 2 phases then cut it from the old
file and leave a stub. Nothing is deleted before its new home exists.

| From (current heading) | To | Stub left at old place |
|---|---|---|
| `README.md` Quickstart | `README.md` Quickstart (rewritten) + `docs/getting-started.md` | - (heading text "Quickstart" kept) |
| `README.md` Installation details, Other install options | `docs/installation.md` | - |
| `README.md` Web UI | `README.md` "See it" (short) + `docs/web-ui.md` | - |
| `README.md` What next, Documentation | `README.md` Documentation | - |
| `README.md` Development | `CONTRIBUTING.md` | - |
| `user-guide.md` intro, Concepts in plain words | `user-guide.md` (plain; internals out: state basis and proposal pins point to `docs/contracts/`) | - |
| `user-guide.md` Set up and connect agents (Create the workspace) | `docs/getting-started.md` (user-guide keeps a two-line pointer) | - |
| `user-guide.md` Connect one project, Connect every project, What gets written, Undo a connection | `user-guide.md` (kept; permission rules in `<details>`; wave and version notes and the plan link removed) | - |
| `user-guide.md` Curate skills | `curating-skills.md` | H2 "Curate skills" |
| `user-guide.md` How the agent picks a skill | `user-guide.md` (resolve example fixed with `task.scope`; `local.*` payload in `<details>`) | - |
| `user-guide.md` Run skills that need tools or secrets | `user-guide.md` | - |
| `user-guide.md` Local Web UI (incl. Network Binding) | `docs/web-ui.md` | H2 "Local Web UI" plus H3 "Do common tasks in the Web UI" |
| `user-guide.md` Move to another machine | `user-guide.md` | - |
| `user-guide.md` Troubleshooting | `docs/troubleshooting.md` | H2 "Troubleshooting" |
| `user-guide.md` Command cheat sheet | `docs/command-reference.md` | H2 "Command cheat sheet" |
| `curating-skills.md` Start, Safety model, Add, Create, Review, Edit, Activate, Review and commit, Measure usage, Check routing quality | `curating-skills.md` (trimmed) | - |
| `curating-skills.md` Keep vendored skills up to date | `docs/sources-and-learning.md` | H2 "Keep vendored skills up to date" |
| `curating-skills.md` Learn from references | `docs/sources-and-learning.md` | H2 "Learn from references" |
| `curating-skills.md` Import repository skills as drafts | `docs/sources-and-learning.md` | H2 "Import repository skills as drafts (`skillhub source import`)" |
| `curating-skills.md` Advanced intake and recovery | `docs/sources-and-learning.md` | H2 "Advanced intake and recovery" |
| `curating-skills.md` Resource-verified fallback and state basis | one plain paragraph in `docs/troubleshooting.md`; detail stays in `docs/contracts/` | H2 "Resource-verified fallback and state basis" |
| `curating-skills.md` Command map | `docs/command-reference.md` | H2 "Command map" |
| `release-runbook.md` Install, upgrade, and uninstall | `docs/installation.md` | H2 kept with a one-line link |

A stub is the original heading text followed by one line: `Moved to [<Title>](<file>.md#<anchor>).` Headings
keep their exact text so their GitHub slugs do not change. No removal date (decided).

## Anchors that must survive

- `README.md#quickstart` (H2 text stays "Quickstart").
- `docs/user-guide.md#what-gets-written` (H3 text stays "What gets written").
- `docs/user-guide.md#do-common-tasks-in-the-web-ui` (stub H3; GitHub slugs are lowercase, so "Web UI" or
  "web UI" both give this slug; keep the existing text "Do common tasks in the Web UI").
- Every stub heading in the move map above.
- `docs/user-guide.md#run-skills-that-need-tools-or-secrets`, `#how-the-agent-picks-a-skill`,
  `#move-to-another-machine`, `#concepts-in-plain-words` (kept headings; other docs may link them).

New-file H2 lists are proposed in each phase file and frozen by Phase 1 in its report. After Phase 1, writers
link only to anchors on that frozen list.

## Style guide (14 rules; every writer follows all of them)

1. Write to "you", in the present tense and the active voice.
2. One idea per sentence. Sentences have at most 20 words; paragraphs at most 3 sentences.
3. Benefit first, then the steps.
4. Explain each product term once, in one line, where it first appears (use the glossary wording).
5. Avoid-terms stay out of the README and out of the first screen of every guide (list below).
6. Task headings start with a verb, in sentence case ("Create your first skill").
7. Code blocks: no `$` prompts; always a language tag; one action per block; a sentence above saying why;
   expected output in a separate `text` block; placeholders written as `<skill-id>`.
8. Bold only UI labels and key terms. Use `code` only for things you type and for file names.
9. Every image has alt text and a one-sentence caption, width 800 (phone shots 260), file <= 300 KB.
10. At most one callout per screen. GitHub alerts: IMPORTANT for "Skill Hub never commits or pushes for you",
    WARNING for plain HTTP, CAUTION for committing connection files, TIP for `-g`.
11. Long reference goes in `<details>` or on a separate page (install options, network binding, permission
    rules, Windows notes, the tall `create-skill.png`).
12. No emoji, no exclamation marks, no hype words ("simply", "just", "easy", "powerful", "seamless").
13. Say plainly what Skill Hub never does: it never commits, never pushes, never approves content for you.
14. Use the same word for the same thing everywhere (the glossary below).

Voice calibration (not final copy): "One set of skills. Every agent. Every project. Skill Hub gives your agent the
same hand-picked skills, wherever you work. When your agent starts a task, it asks the hub which skill fits, and
the hub suggests one. Nothing changes without your say-so."

## Glossary (frozen by Phase 1; writers use these words and one-line definitions)

| Term | One-line definition to use | Do not write |
|---|---|---|
| Skill Hub / the hub | The `skillhub` program that keeps your skills and answers your agent. | "server" in user prose, "daemon" |
| `skillhub` | The command you type. | "skill-hub", "SkillHub" |
| skill | A folder with a `SKILL.md` your agent follows and a small file that says when it applies. | "plugin", "prompt pack" |
| workspace | The Git repository that holds your skills; you create it with `skillhub init`. | "canonical repo", "skills repository" as a synonym |
| agent | The AI coding tool you work with, such as Claude Code. | "host", "client" (allowed only inside the "What gets written" table and agent config file names) |
| connect | Register the hub with an agent, for one project or for all (`-g`). | "integrate" (alias only in the command reference) |
| connection files | The files `connect` writes so an agent finds the hub. | "artifacts" |
| draft | A skill agents are not offered yet. | "inactive", "pending" |
| active | A skill agents can be offered. | "published", "live" |
| deprecated, archived | Being phased out; retired and kept for history. | - |
| recommendation | The hub's answer to "which skill fits?", or "no skill fits". | "match score", "semantic match" |
| source | A Git repository linked to one or more skills. | "feed" |
| upstream | The source a skill was copied from; you can pull its updates. | "vendored" (avoid-term) |
| learning reference | A source your curator agent reads for ideas; nothing is copied. | - |
| content approval | The terminal-only step that lets agents receive a skill from someone else's repository. | "trust gate", "digest approval" outside reference sections |
| curator agent | Your own agent with the distill-lab skill, which reads sources and proposes lessons. | "the hub learns" |
| web UI | The local browser screens started by `skillhub serve web`. | "Web UI", "WebUI", "dashboard" in headings (UI labels stay as shown) |
| lesson | A proposed improvement your curator agent records for a skill; you decide what to keep. | "insight" in README and guides' first screens |

Avoid-terms (never in the README, never on a guide's first screen; elsewhere explain once): catalog
generation, served state, canonical state, digest, proposal ID, provenance, vendored, 3-way merge, daemon,
host, embeddings. Product names are spelled Claude Code, Codex, Gemini CLI, GitHub, Git, Skill Hub.

## Claims that must not appear (banned)

- "Works with Claude Code, Codex and Gemini CLI" without qualification. Honest wording: "Works with Claude
  Code. `connect` also writes connection files for Codex and Gemini CLI; those are not yet verified end to end."
  Claude Desktop and Cursor are not mentioned as supported.
- "semantic", "AI-powered", "embeddings": matching uses skill metadata and full-text search.
- Automatic watching or scheduled checks: `source watch` records an interval; nothing runs in the background.
- "The hub learns or distills for you": your curator agent does that with the distill-lab skill.
- "The agent always asks the hub": write "your agent is told to ask the hub".
- The remote hub (`connect --hub`): not mentioned at all.
- `git add -A` (or `git add .`) in the workspace: never shown.
- `skillhub --version`: the command is `skillhub version`.
- "Restart your agent": write "open a new Claude Code session in that folder".
- "requires at least 3 examples": write "at least 3 examples are recommended".
- `skill add https://github.com/anthropics/skills --skill pdf`: fails on the 8 MiB source limit; use the
  working form that Phase 1 verifies (`--path skills/pdf` or a `/tree/main/skills/pdf` URL).

Writers never invent a feature, flag, output line or claim. Anything they cannot verify by running it or by
reading code is left out of the doc and listed in their phase report under "Unverified".

## Sandbox rules (every phase that runs `skillhub`)

Phase 1 builds one binary from this branch (which is main plus plan files) into
`/tmp/skillhub-docs-rewrite/bin/skillhub`. Every other phase uses that binary and its own sandbox directory:

```bash
SB=/tmp/skillhub-docs-rewrite/<phase-slug>          # one directory per phase, never shared
mkdir -p "$SB/home"
export HOME="$SB/home" USERPROFILE="$SB/home"
export XDG_CONFIG_HOME="$SB/home/.config" XDG_DATA_HOME="$SB/home/.local/share"
export XDG_STATE_HOME="$SB/home/.local/state" XDG_CACHE_HOME="$SB/home/.cache"
export PATH="/tmp/skillhub-docs-rewrite/bin:$PATH"
unset SKILLHUB_WORKSPACE
git config --global user.name "Docs Sandbox"; git config --global user.email "sandbox@example.invalid"
```

Never touch `/home/vantt/skill-hub`, `~/.claude`, `~/.codex`, `~/.gemini`, `~/.claude.json`, `~/.local/bin` or
any real agent config. Never use `connect -g` outside a sandbox HOME. Shell commands must not mention
`node_modules` or `dist` paths (a hook blocks them); resolve such paths inside a script instead. Network commands
(`skill add` from GitHub, `source check`) are allowed only when the phase lists them.

## Phases

| # | Phase | Wave | Effort | Depends on |
|---|---|---|---|---|
| 1 | [Freeze map, anchors, glossary; verify the first win](./phase-01-freeze-map-and-verify-first-win.md) | 0 | 2h | none |
| 2 | [Write docs/getting-started.md](./phase-02-getting-started.md) | 1 | 2h | 1 |
| 3 | [Write docs/installation.md](./phase-03-installation.md) | 1 | 1h | 1 |
| 4 | [Write CONTRIBUTING.md and LICENSE](./phase-04-contributing-and-license.md) | 1 | 0.5h | 1 |
| 5 | [Write docs/web-ui.md](./phase-05-web-ui.md) | 1 | 1.5h | 1 |
| 6 | [Write docs/troubleshooting.md](./phase-06-troubleshooting.md) | 1 | 1.5h | 1 |
| 7 | [Write docs/sources-and-learning.md](./phase-07-sources-and-learning.md) | 1 | 1.5h | 1 |
| 8 | [Write docs/command-reference.md](./phase-08-command-reference.md) | 1 | 1.5h | 1 |
| 9 | [Rewrite README.md body](./phase-09-readme-body.md) | 1 | 1.5h | 1 |
| 10 | [Design the logo set](./phase-10-logo-set.md) | 1 | 2h | 1 |
| 11 | [Fix the Home count copy and refresh home.png](./phase-11-home-copy-fix.md) | 1 | 1.5h | 1 |
| 12 | [Rewrite docs/user-guide.md with stubs](./phase-12-user-guide-rewrite.md) | 2 | 2h | 2, 3, 5, 6, 8 |
| 13 | [Trim docs/curating-skills.md with stubs](./phase-13-curating-skills-trim.md) | 2 | 1.5h | 6, 7, 8 |
| 14 | [Trim docs/release-runbook.md](./phase-14-release-runbook-trim.md) | 2 | 0.5h | 3, 4 |
| 15 | [Write docs/README.md index](./phase-15-docs-index.md) | 2 | 0.5h | 2-8 |
| 16 | [Wire the logo and license badge into the README](./phase-16-readme-header-wiring.md) | 2 | 0.5h | 4, 9, 10 |
| 17 | [Automated verification](./phase-17-automated-verification.md) | 3 | 1.5h | 2-16 |
| 18 | [Style, consistency and rendering review](./phase-18-style-and-rendering-review.md) | 3 | 1.5h | 2-16 |

## Dependency graph

```text
Wave 0   P1 lead: frozen map + anchors + glossary + verified commands + sandbox binary
           |
Wave 1   P2 getting-started  P3 installation  P4 CONTRIBUTING+LICENSE  P5 web-ui  P6 troubleshooting
         P7 sources-and-learning  P8 command-reference  P9 README body  P10 logo  P11 Home copy fix
           |            |              |              |         |          |
Wave 2   P12 user-guide (2,3,5,6,8)   P13 curating (6,7,8)   P14 runbook (3,4)
         P15 docs index (2-8)         P16 README header (4,9,10)
           |
Wave 3   P17 automated checks   P18 style + rendering review   (both read-only)
           |
         Lead fix pass (serial), re-run P17 script, commit
```

Critical path: P1 (2h) -> P2 (2h) -> P12 (2h) -> P17 (1.5h) -> fix pass, about 8h wall clock with full
parallelism. P10 -> P16 (2.5h) and P11 (1.5h) have slack.

## Parallel execution

| Phase | Owned files (exclusive) | Depends on | Can run with | Effort |
|---|---|---|---|---|
| 1 | `plans/261011-1432-readme-and-user-docs-rewrite/reports/wave0-frozen-map.md`; `/tmp/skillhub-docs-rewrite/` (outside the repo) | none | none | 2h |
| 2 | new `docs/getting-started.md` | 1 | 3-11 | 2h |
| 3 | new `docs/installation.md` | 1 | 2, 4-11 | 1h |
| 4 | new `CONTRIBUTING.md`, new `LICENSE` | 1 | 2, 3, 5-11 | 0.5h |
| 5 | new `docs/web-ui.md` | 1 | 2-4, 6-11 | 1.5h |
| 6 | new `docs/troubleshooting.md` | 1 | 2-5, 7-11 | 1.5h |
| 7 | new `docs/sources-and-learning.md` | 1 | 2-6, 8-11 | 1.5h |
| 8 | new `docs/command-reference.md` | 1 | 2-7, 9-11 | 1.5h |
| 9 | `README.md` (body; header block left as a marked placeholder) | 1 | 2-8, 10, 11 | 1.5h |
| 10 | new `docs/images/brand/skill-hub-logo-light.svg`, `skill-hub-logo-dark.svg`, `skill-hub-mark-light.svg`, `skill-hub-mark-dark.svg` | 1 | 2-9, 11 | 2h |
| 11 | `web/src/screens/home/action-cta.ts`, `web/src/screens/home/HomeScreen.tsx`, `web/src/screens/home/home.test.tsx`, `web/src/i18n/en.ts`, `docs/images/web/home.png`, `docs/images/web/manifest.json` | 1 | 2-10 | 1.5h |
| 12 | `docs/user-guide.md` | 2, 3, 5, 6, 8 | 13-16 | 2h |
| 13 | `docs/curating-skills.md` | 6, 7, 8 | 12, 14-16 | 1.5h |
| 14 | `docs/release-runbook.md` | 3, 4 | 12, 13, 15, 16 | 0.5h |
| 15 | new `docs/README.md` | 2-8 | 12-14, 16 | 0.5h |
| 16 | `README.md` (takes over from 9; header block, badges, license line) | 4, 9, 10 | 12-15 | 0.5h |
| 17 | none (read-only; scripts in `/tmp/skillhub-docs-rewrite/verify/`) | 2-16 | 18 | 1.5h |
| 18 | none (read-only; report only) | 2-16 | 17 | 1.5h |
| fix pass | any doc file, by the lead, serially | 17, 18 | none | in 17 |

Read-only for every writing phase: everything outside its owned files, in particular `docs/images/web/*`
(owned by 11), `docs/mcp-compatibility-matrix.json`, `docs/contracts/*`, `docs/design/*`, `schemas/*`, the
synthesis report and `web/e2e/*`. Phase reports go to `plans/261011-1432-readme-and-user-docs-rewrite/reports/`
with the name `{agent}-{YYMMDD-HHmm}-phase-NN.md`; each phase owns only its own report file.

Commit rule: each phase commits only its owned files with `git add <owned paths>` (never `git add -A` or
`git add .`), with a conventional message (`docs: ...`, `fix(web): ...`), no plan IDs or phase numbers in the
message, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Parallel writers share one
worktree, so commit with an explicit path list and retry if the index lock is busy; never stash, reset or
check out files you do not own. No push.

## Rules for every phase

- Read the synthesis report sections the phase file names, Phase 1's frozen map, and the old text being moved.
- Writing phases verify every command they show in their own sandbox (see Sandbox rules) and paste real output,
  trimmed. Output that depends on time, paths or tokens is shown with a placeholder and said so.
- Images come only from `docs/images/web/`, with the caption and alt text from `manifest.json`, through
  `<img src="..." alt="..." width="800">` (phone shot 260) so the 2x PNGs render at a readable size.
- UI and logo phases (10, 11): try `/ak:frontend-design` for visual decisions and `/ak:frontend-development`
  for React code (or the `ui-ux-designer` agent for the logo). Earlier implementing subagents could not invoke
  the Skill tool; if that happens, follow the existing conventions (the editorial `precision` theme tokens,
  `web/src/components/AppShell.tsx:103` mark, the i18n plural pattern in `web/src/i18n/index.ts:6-14`) and say
  so in the report. The lead reviews visually afterwards (rendered logo PNGs, re-shot `home.png`).
- No plan IDs, phase numbers or audit labels in docs, code comments, test names or commit messages.
- Each phase ends its report with `Status: DONE | DONE_WITH_CONCERNS | BLOCKED | NEEDS_CONTEXT`, a one- or
  two-sentence summary, and an "Unverified" list (may be empty).

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| Content is lost when sections move out of user-guide and curating-skills | Medium x High | Move map above; wave 2 phases diff old against new files and list every moved paragraph's new home in their report before cutting |
| A writer documents behavior that does not exist | Medium x High | Sandbox verification per command; "Unverified" list instead of prose; P17 compares every command with `skillhub help` |
| Parallel writers break each other's commits in the shared worktree | Medium x Medium | Exclusive owned files; explicit-path `git add`; no stash or reset; retry on index lock |
| Cross-links point to anchors that change during wave 1 | Medium x Medium | P1 freezes H2 lists; writers link only to frozen anchors; P17 checks all anchors |
| The first win cannot be shown end to end in a live Claude Code session | High x Medium | Docs promise only what the MCP stdio proof and `skillhub resolve` show; say what the reader should see; flagged as an owner follow-up |
| `make web-shots` overwrites all screenshots and they differ by a few pixels | High x Low | P11 shoots into a scratch `SHOTS_OUT_DIR` and copies only `home.png` and its manifest entry |
| SVG wordmark renders with a different font on GitHub | Medium x Medium | Outline the wordmark glyphs to paths (Fraunces, OFL); fallback is a serif font stack, flagged |
| Badges point to targets that do not exist (private repo, missing release) | Low x Medium | P16 opens each badge URL; P18 checks them in the rendered preview |
| Logo and favicon work touches web code owned by P11 | Low x Medium | Favicon is out of scope (follow-up); P10 writes only `docs/images/brand/` |

## Backwards compatibility

The inbound anchors listed above survive, and moved H2s keep a stub with the same heading text, so external links
into `user-guide.md` and `curating-skills.md` keep landing on a pointer. No code, CI or test links to these docs
(checked by the synthesis); `docs/mcp-compatibility-matrix.json` and `docs/images/web/` stay where they are. The
CLI and MCP summaries keep their text; only the web Home rendering changes.

## Rollback

Each phase is one commit touching only its owned files, so any phase reverts with `git revert <sha>`. Wave 2
trims (12, 13, 14) revert alone and restore the old full text; the new files remain valid on their own. Phase 16
reverts alone to the plain-text header from Phase 9. Phase 11 reverts alone (copy, test and image together). The
branch is not pushed; abandoning it leaves main untouched.

## Owner follow-ups (out of scope for this branch)

1. Resolve hint prints `task.scope` as a flat key that the API rejects.
2. Help example `skill add https://github.com/anthropics/skills --skill pdf` fails (8 MiB source limit).
3. `init` should gitignore the connection files it writes into the workspace.
4. `status` on an empty hub says "Next: continue normal work".
5. `skill review` shows `Git: clean` for an untracked skill folder.
6. The activation message suggests `git commit` without `git add`.
7. `validate` says active skills "require at least 3 examples" although activation succeeds with fewer.
8. The Go action summaries still say "source(s)", "skill(s)" and "lesson(s)" in `skillhub status` and MCP output
   (`internal/app/curation_home.go:445-480`); this branch fixes only the web Home rendering.
9. Use the new mark as the web UI favicon (needs `web/index.html`, a public asset and a rebuild; kept out so it
   does not collide with the Home fix in the same wave).
10. `README.vi.md` is deferred; it would double README maintenance.
11. Demonstrate an agent actually using a skill in a live Claude Code session (the matrix verifies registration
    and curation only), then strengthen the getting-started "what you should see" text.
12. Confirm the LICENSE copyright holder name ("Van Tran", taken from the git author).
13. Add a docs link and anchor check to CI (none exists today).

## Unresolved questions (for the owner)

1. The LICENSE holder is "Van Tran" from the git author; confirm or give the legal name or entity.
2. Badges assume the repository and its releases are public on GitHub; if not, the release and CI badges are
   dropped in Phase 16.
