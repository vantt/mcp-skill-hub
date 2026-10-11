# Automated verification report

Read-only run of the phase 17 checklist on branch `docs/readme-and-guides-rewrite` at `ce4f3d0`. Scripts and
outputs are in `/tmp/skillhub-docs-rewrite/verify/` (`verify_links.py`, `verify_commands.py`, `step*.txt|log`).
The doc set is the 12 files in `verify/files.txt` (README, CONTRIBUTING, docs/README and the nine guides).

Besides the frozen wave 0 binary, a second binary was built from current `main` (`e5ccd56`, via `git archive`
into `verify/main-src`, output `verify/skillhub-main`) because main has moved since the branch point. Its help
captures are in `verify/help-main/`.

## Checklist results

| # | Step | Result |
|---|---|---|
| 1 | Relative links and anchors (incl. HTML `src`/`srcset`, all survivors and stub headings) | PASS. 0 broken targets or anchors; all 28 required survivor anchors exist. The checker was negative-tested. |
| 2 | Commands against help (fenced and inline `skillhub ...`, per-subcommand flag sets) | PASS against the wave 0 captures and against `main` captures. No `skillhub --version`. |
| 3 | Banned text, exclamation marks, emoji, README avoid-terms | 1 defect (D3). Legitimate hits: `update --version` (installation, command reference), `just now` in `source list` output, "embedding" in runbook maintainer prose, `[Plans](plans/)` in the docs index maintainer section (design records, decided), README `--host claude` (required flag, accepted exception to the `host` avoid-term). |
| 4 | Codex and Gemini CLI claims | 1 low defect (D7). README table, getting-started:5, user-guide:46, troubleshooting:48 are qualified; the "What gets written" table is exempt. |
| 5 | Images | PASS. 15 references exist, all have alt text equal to the manifest alt, captions present, widths 800 (phone 260), largest file well under 300 KB; 4 brand SVGs parse as XML and have no external references. |
| 6 | Mermaid | PASS. One block (README), byte-identical to the synthesis block; renders with `@mermaid-js/mermaid-cli` (Chromium needed `-p puppeteer.json` with `--no-sandbox` on this host because of the AppArmor user-namespace restriction). |
| 7 | Line budgets | PASS. README 130, getting-started 220, installation 89, CONTRIBUTING 53, user-guide 249, web-ui 193, troubleshooting 147, curating-skills 348, sources-and-learning 277, command-reference 88, docs/README 40, release-runbook 166. |
| 8 | First-win replay | PASS. `docs/getting-started.md` in order in `SB=verify-firstwin` (wave 0 binary) and `verify-firstwin-main` (main binary): every `text` block matches, resolve prints "Resolved commit-style (medium confidence) ...", commit is 2 then 4 files, status is clean. README Quickstart replayed in `verify-readme` (also in a non-Git project folder): all exit 0, doctor "Workspace is healthy.". |
| 9 | Builds and tests | PASS. `make check`, `make web-check`, `go test ./internal/hostintegration -run TestMatrixSynchronized` all exit 0 (logs `step9-*.log`); worktree clean afterwards. |
| 10 | Scope check | PASS against the real branch base `dcba0b3` (only owned files of phases 1-16, the plan folder and the synthesis; under `docs/images/web/` only `home.png`; no `internal/`, `docs/design/`, `docs/contracts/`, `schemas/`, `system-skills/`, `web/e2e/`, matrix). `git diff main...HEAD` as written in the phase file does NOT pass: see D2. |
| 11 | Fallbacks | Phase 11 landed (`home.png` shows "1 source is ready to distill"), so `home.png` stays in web-ui.md. Phase 10 visual approval is the lead's call (not checked here). |

## Defects

Ranked most severe first. Line numbers are on `ce4f3d0`.

### D1. `docs/user-guide.md:103-109` says undoing a connection is manual; main ships `connect --remove` (high once merged)

- What is wrong: "Removing a connection is manual" plus three hand-edit steps. On `main`, `skillhub connect --remove
  [--project <dir>] [-g] [--host ...] [--yes]` removes the scope's Skill Hub entry, its instruction block and an
  unchanged system-curator skill, and leaves unrelated agent configuration alone (`help-main/help-connect.txt`).
  Verified with the main binary in `verify-firstwin-main/home/myproject`: the preview lists ".mcp.json deleted,
  .claude/settings.local.json deleted, curator skill deleted, CLAUDE.md deleted" and prints
  `skillhub connect --remove --host claude-code --yes`. The wave 0 binary predates the feature, so no writer saw it.
- Fix: after rebasing onto main (D2), replace the section body with: one sentence on what it removes and keeps; a
  `bash` block `skillhub connect --remove`; a sentence "Add `--yes` to remove it; add `-g` for the user-account
  connection, or `--host claude` for one agent."; keep "Your workspace and your skills stay untouched." Add a row
  "Remove a connection | `skillhub connect --remove [--project <dir>] [-g] [--host ...] [--yes]`" to
  `docs/command-reference.md` "Set up". Leave out `--cache` and `--hub` (remote hub, banned). The heading text
  "Undo a connection" stays. If the owner considers `--remove` part of the remote-hub work that is kept out of the
  docs, keep the manual steps but drop the word "manual"; list that choice for the owner.
- Severity: high (the page is wrong on the branch it merges into).

### D2. The branch is based on commits that `main` carries under other hashes (medium)

- What is wrong: the branch starts on `80c1959` / `53bfc05` / `dcba0b3`; `main` has the same content as
  `42af5c0` / `a06bf19` (`git diff a06bf19 dcba0b3` is empty). So `git diff --stat main...HEAD` (the phase's step
  10 command) lists out-of-scope files: `Makefile`, `docs/design/remote-hub.md`,
  `internal/delivery/mcpserver/remote_bench_test.go`, `plans/reports/cook-261011-remote-phase1.md` and an older
  plan's `plan.md` / `phase-01-baseline-and-decisions.md`. A merge would also duplicate those three commits in
  history. `git merge-tree --write-tree main HEAD` is clean, and `main` changed no doc this branch touches.
- Fix: `git rebase --onto main dcba0b3 docs/readme-and-guides-rewrite`, then re-run steps 1-10 with a binary built
  from the rebased branch (main's help for `connect`, `doctor`, `mcp`, `serve` and the top level changed). The
  main-binary run in this report already shows steps 2 and 8 still pass, apart from D1.
- Severity: medium (process; blocks a clean step 10 and an honest "verified against main").

### D3. `docs/command-reference.md:74` prints the banned string `git add -A`

- What is wrong: "Add the workspace's own folders by name, never with `git add -A`". The plan bans showing
  `git add -A` anywhere in the user docs; getting-started and curating-skills already avoid it.
- Fix: "Add the workspace's own folders by name, never the whole folder; see [Curating
  skills](curating-skills.md#review-and-commit-workspace-changes)."
- Severity: medium-low.

### D4. `docs/command-reference.md:7` calls `source capture` "the one exception" to preview-first

- What is wrong: `skillhub skill env set` and `skill env unset` also write at once, without a preview or `--yes`
  (verified: `echo dummy | skillhub skill env set commit-style MY_KEY` prints "Stored MY_KEY for skill
  commit-style." and writes the env file; `unset` prints "Removed MY_KEY ...").
- Fix: "Commands that change files show a preview first. Add `--yes` to write. `source capture` and
  `skill env set|unset` write at once."
- Severity: low.

### D5. `docs/user-guide.md:139` points the `--json` answer at the request schema

- What is wrong: "Add `--json` for the full answer; the format is in [the request schema](../schemas/skill-resolve-request-v1.schema.json)."
  The answer follows `schemas/skill-resolve-response-v1.schema.json`; the request schema describes the input.
- Fix: "Add `--json` for the full answer. The request format is in [the request
  schema](../schemas/skill-resolve-request-v1.schema.json), the answer format in [the response
  schema](../schemas/skill-resolve-response-v1.schema.json)."
- Severity: low.

### D6. `README.md:94` and `docs/curating-skills.md:13` state what the agent answers as fact (unverified)

- What is wrong: "Your agent shows the state of your hub and suggests one next step." / "Your agent checks the hub
  and suggests one next step." No live session was run (frozen map section 7 and "Unverified"; phase 9 report
  lists it as unverified). The plan says unverifiable claims are left out or described modestly.
- Fix: describe the instruction, which is verifiable in `system-skills/curator/SKILL.md:144` ("Show Curation Home
  and one next action"): README "The curator skill that `connect` installed tells your agent to show the hub's
  status and one next step."; curating-skills "The curator skill tells your agent to check the hub and suggest one
  next step. You see the same suggestion in the terminal:".
- Severity: low.

### D7. Unqualified Codex and Gemini CLI mentions (step 4)

- `docs/troubleshooting.md:59`: "limit `-g` to agents with real folders (`--host codex,gemini`)" recommends the
  unverified agents with no qualifier. Fix: append "(Codex and Gemini CLI are not yet verified end to end)".
- `docs/command-reference.md:14`: `--host claude|codex|gemini` in the connect row, no qualifier on the page. Fix: add
  to the sentence list at the top or under the Set up table: "Only Claude Code is verified end to end; `connect`
  also writes files for Codex and Gemini CLI."
- Severity: low.

### D8. `docs/curating-skills.md:269` says `connect` writes connection files into the workspace

- What is wrong: "`init` and `connect` write connection files into the workspace". `connect` writes into the
  project folder (or the home folder with `-g`); only `init` writes them into the workspace (frozen map section 7;
  replay `git -C ~/skillhub status` shows only `init`'s files).
- Fix: "`init` writes connection files into the workspace, and some hold absolute paths for this machine."
- Severity: low.

### D9. `docs/user-guide.md:62-65` trims the `connect -g` output silently

- What is wrong: the `text` block shows only the Claude Code line; the real output also has Codex, Gemini CLI, the
  WARNING and the trust-dialog lines. The project example above says "Output is trimmed"; this one does not, so a
  reader sees extra lines they were not told about.
- Fix: add "Output is trimmed." after the block (or show the Codex and Gemini CLI lines, which are then
  qualified by the sentence at line 46).
- Severity: low.

## Left for the style review (not counted above)

"MCP server" in `troubleshooting.md:48` and `command-reference.md:86` (glossary avoids "server" in prose; it is the
Claude Code label), "Web UI" as link text in several pages, and README:84 stating doctor output in prose instead of
a `text` block (phase 9 choice). These are phase 18 calls.

## Owner items

- D1 depends on whether `connect --remove` may be documented while the remote hub stays out of the docs.
- Synthesis "Remote hub (`connect --hub`): not shipped" is no longer true on `main` (`serve mcp --addr`, `mcp
  proxy`, `token`, `cache`, `connect --hub` exist). The decision to leave it out of user docs still holds; the
  docs contain no mention of it (step 3 grep for `connect --hub` and `remote hub`: no hits).

Status: DONE_WITH_CONCERNS
Summary: All mechanical checks pass (links, anchors, commands vs help, images, mermaid, budgets, first-win replay on the frozen and main binaries, make check/web-check/matrix test); nine defects found, the main one being that `main` now ships `connect --remove`, which makes the user guide's "Undo a connection" section wrong, and the branch needs a rebase onto main for a clean scope check.
Concerns/Blockers: D1 needs an owner decision on documenting `connect --remove`; D2 rebase should precede the fix pass so the re-run verifies against main.
