# Fix pass report

Applies the defects from `phase-17-automated-verification-report.md` (D1-D9) and
`phase-18-style-and-rendering-review-report.md` on branch `docs/readme-and-guides-rewrite`, rebased onto `main`
(`e5ccd56`). Commands were checked against a binary built from this worktree
(`/tmp/skillhub-docs-rewrite/bin/skillhub-main`), in the sandboxes `/tmp/skillhub-docs-rewrite/fixpass` and
`/tmp/skillhub-docs-rewrite/fixpass-firstwin`.

Commits: `935663f` (README, Get started), `d1f9971` (Using Skill Hub, command reference), `682e7ff` (other
guides, docs index, CONTRIBUTING).

## Fixed

### Automated verification (D1-D9)

| Defect | Fix |
|---|---|
| D1 `connect --remove` | "Undo a connection" now previews `skillhub connect --remove`, then explains `--yes`, `-g` and `--host claude`. The command reference has a "Remove a connection" row. `--cache`, `--hub` and the remote hub are not mentioned. Checked: preview, `--host claude --yes` (own text in `CLAUDE.md` kept, files that held only Skill Hub entries deleted), `-g` preview. |
| D2 rebase | Already done by the lead; merge base is `main` (`e5ccd56`). Out-of-docs changes on the branch are only the planned Home copy fix under `web/src/screens/home/` and `web/src/i18n/en.ts`. |
| D3 `git add -A` | Command reference: "Add the workspace's own folders by name; never stage the whole folder." |
| D4 preview exceptions | "`source capture` and `skill env set\|unset` write at once." Using Skill Hub and Curating skills now say "commands that change skills" show a preview. |
| D5 response schema | The resolve section links both the request and the response schema. |
| D6 agent claims | README and Curating skills say what the curator skill tells the agent (checked in `system-skills/curator/SKILL.md`). |
| D7 unqualified agents | Troubleshooting symlink fix and the command reference intro now say Codex and Gemini CLI are not yet verified end to end. |
| D8 `connect` writes into workspace | "`init` writes connection files into the workspace". |
| D9 trimmed `-g` output | "Output is trimmed." follows the block. |

### Style and rendering review

- **README:** agent-status sentence (D6), `-g` tip uses `--host claude`, next-step link goes to `#create-your-first-skill`, "skills" instead of "library" and "every skill change", "Web UI guide" link text, semicolon in the license line. The "Why" bullet "Your agent asks for the right skill" now says the agent is told to ask.
- **Get started:** staging step before the first-win prompt; `-g` TIP turned into a plain sentence (one callout per screen); request JSON wrapped over three lines; explanation of the "Host approval" line; `init` next steps explained; paragraph at line 5 cut to three sentences without using `connect` before it is defined; "web UI guide" link; italic caption; split sentence; "lessons" defined; "Git commit messages".
- **Installation:** paragraph split, sentence split, "Node.js 24".
- **Using Skill Hub:** response schema; agent-told wording in the tools table and the `local` details; workspace backup sentence no longer contradicts secrets; `--host claude` in all three connect commands with one sentence for the other agents; TIP turned into a sentence; four-sentence paragraphs fixed; long sentence split; stubs use "The web UI".
- **Curating skills:** curator-skill wording; Claude Code asks before `--yes`/`confirm` commands; preview claim narrowed; `init` only; link to `#activation-refuses-a-draft`; long `skill edit` commands wrapped; "Git"; paragraphs at 144 and 158 cut to three sentences; dropped the unverified "your agent asks you to run it".
- **Sources and learning:** `--skill-id <skill-id>` plus "The skill must already exist." (output updated; `0 candidates` matches the reader's order); `serve web --loopback-only`; commit block after staging lessons; `<digest>` placeholder with a note that it includes `sha256:`; curator agent defined at first use; import paragraph cut to three sentences.
- **Web UI:** first command is `serve web --loopback-only` (bare form stays under "Keep it safe on your network"); picture sentence; Home overview wording; source, curator agent and lesson defined at "Manage one skill's sources"; the two five-sentence bullets became two three-sentence paragraphs; 22-word sentence split.
- **Troubleshooting:** `--host claude` on reconnect; dropped "MCP server" sentence; grammar; check signatures before **Run anyway**; wrapped `skill edit`; long sentences split; request JSON wrapped.
- **Command reference:** D3, D4, D7; "Start the hub for agents by hand"; 25-word sentence split.
- **Docs index:** "agents and protocol revisions"; "Web UI guide" link text.
- **CONTRIBUTING:** gate sentence turned into a short list; isolation sentence split.

## Skipped (owner decisions, listed for the lead)

- CI badge renders "failing" (README:5): not changed.
- Logo plus duplicate `# Skill Hub` H1 (README:1-2): not changed.
- Header link name "User guide" (README:6): not changed.
- Mermaid labels "Web UI or CLI" and "Git repo": not changed.
- Agreed sample wording "it asks the hub which skill fits" (README:9, voice calibration): not changed.

## Left as is, with reason

- README:84 doctor output stays in the sentence: the README is at its 130-line budget (the review allowed this).
- Command reference tables scrolling at 390 px: the review marks this acceptable on GitHub.
- Style-scan hits left: `getting-started.md:207` (quoted error message), `curating-skills.md:34` and
  `troubleshooting.md:147` (bold label counted as part of the sentence); all predate this pass.

## Verification

- Line budgets: README 130, Get started 220, installation 90, CONTRIBUTING 59, Using Skill Hub 250, web UI 196,
  troubleshooting 149, Curating skills 350, Sources and learning 280, command reference 90, docs index 40,
  release runbook 166. All within budget.
- Links and anchors: `verify_links.py` over the 12 files, including the 28 survivor anchors and the stubs: PASS.
  The checker was negative-tested again (a missing target fails).
- Commands vs help: `verify_commands_main.py` against `main` help captures: PASS (negative test with an unknown
  `connect --remove` flag fails).
- JSON blocks in Get started, Using Skill Hub and Troubleshooting parse.
- First win replayed from the edited Get started in a fresh sandbox with nothing staged: init, commit (2 files),
  connect `--host claude`, doctor "Workspace is healthy.", create, activate, staging step (`git init`,
  `README.md`, `git add` leaves `README.md` staged), resolve "Resolved commit-style (medium confidence) ...",
  skill list, commit (4 files), `serve web --loopback-only`, `connect --remove` preview. The live Claude Code
  answer to the prompt was not run.
- `make check`, `make web-check` and `go test ./internal/hostintegration -run TestMatrixSynchronized`: all exit 0.
- Banned text: no `git add -A`, `connect --hub`, remote hub, hype words or exclamation marks in the user docs.
