# Lead handoff, 2026-10-10

State at the end of 2026-10-10: waves 1–5 merged, the three original plans archived. Read this first
in a new lead session, then the plans below.

## Where things are

- Repo `/home/vantt/projects/mcp-skill-hub`, main = origin/main (wave 5 done, curator 1.6.1).
  No other worktrees or branches.
- Live hub `/home/vantt/skill-hub`, schema **v5**, main = origin/main = `786c60e`. Local
  tags `pre-schema-v4` and `pre-schema-v5` (not pushed) mark the state before each
  migration.
- Binary `~/.local/bin/skillhub` built from `c84abe7` (stamp the commit with
  `-ldflags "-X github.com/vantt/mcp-skill-hub/internal/version.Commit=<sha>"`). Restart
  Claude Code after installing so the MCP server runs it. On 2026-10-10 the lead ran
  `connect -g` and `connect` in this repo with it: only `skillhub --profile runtime` is
  registered, the four CLI permission rules are in `~/.claude/settings.json`, curator 1.6.1.
- Case journal is **enabled** on the live hub (`runtime/case_journal.json`). The MCP server
  writes cases itself during `skill_resolve` (`internal/app/resolver.go:95`); agents do not.
  On 2026-10-10 the store held 13 events and 0 cases. Cases are written only on disagreement
  (`resolver.go:60-78`); with one active skill nearly every resolve is `catalog_gap`, so none
  arrive. The user is adding skills (drafts `herdr-cook-plan`, `markdown-to-epub` exist).

## Plans

**Open work lives in `docs/plans/2026-10-10-observation-backlog.md`** (mostly measurement: baseline, cases,
curator compliance; plus small fixes and host checks). The three original plans are in `archive/plans/`:

`archive/plans/261008-1433-simplify-hub-model/plan.md`: Phases 1–5 done (5 = digest-only receipts,
schema v5, `9b6c986`). Phase 0 (scorecard) is the user's own work with distill-lab.

`archive/plans/261008-observer-and-enrichment/2026-10-08-observer-and-enrichment.md` (status table after §8):

| Item | State |
|---|---|
| Phase 1, 2 | done |
| §5.1 MCP profiles (runtime / curation / all) | code done; Claude Code verified (2026-10-10, and no longer needed there); other hosts: backlog H2 |
| 5(a), skill_list paging | done |
| 5(c) one curator source per client | done (H, `51cb26b`..`1fdd787`, fix `d65b0e7`); no client verified for `skills_extension`, so all native hosts keep the native copy and the server hides the MCP copy from claude-code/codex/gemini |
| Phase 3 enrichment | **deferred until there are cases.** Decided 2026-10-10: option 1 (promote confirmed cases to Git eval cases, cite `repo@commit:path`; do not change distill-lab) |
| Phase 4 resolver | waits for the baseline (`skillhub telemetry baseline`), counting since the v4/v5 migration |
| `directoryRead` | deferred entirely (user decided 2026-10-10). Reopen only if a pure-MCP client is shown to miss files |

## Wave 5: curator via CLI (approved 2026-10-10)

`archive/plans/261010-curator-via-cli/2026-10-10-curator-via-cli.md`. On shell hosts runtime stays MCP (3 tools; needed
for session-aware cases), curation moves to the `system-curator` skill calling
`skillhub … --json`. User answered §9: all four yes.

| Step | State |
|---|---|
| 1–2 CLI gaps + curator rewrite | **done**, J merged (`fd4ac80`..`32db030`): bound `cli` confirm commands, `eval routing` envelope, `source import`, curator 1.6.0. Lead ran a real `claude -p` read-only curation with only the runtime MCP entry: used the CLI, hub unchanged |
| 3 Claude Code permission rules | **verified** by the user (§5.2): ask beats allow, flag order does not bypass, `--approve-content` denied |
| 4 `connect` writes the rules, drops `skillhub-curation` on Claude Code, warns about the trust dialog; fix the doctor HOME test | **done**, K merged (3 rounds, `…`..`7767a4b`): runtime-only Claude entry, 4 rules, no disconnect/receipts, tests isolated from HOME (incl. Go caches), `servable.go` fixed |
| 5 real-host smoke | **done** (design note §7 row 5) |
| 6 curator 1.6.1 | **done** (`c84abe7`); compliance still imperfect in `claude -p`, see row 7 |
| 7 measure curator compliance with repeatable `claude -p` scenarios | open |

Facts learned on 2026-10-10 (also in the matrix): Claude Code `/mcp` Disable persists across
restarts (tick = on, empty circle = off); `disabledMcpjsonServers` rejects a server instead
of disabling it; Claude Code defers MCP tool schemas; agents do not see permission prompts,
so trust what the user saw; the CLI rejects `--workspace` before the subcommand.

## Small open items

Superseded by the backlog (F1–F8, H1–H4); kept below as history.

- The real-hub test-audit skill has 0 examples (validate warns).
- Tests are isolated from HOME (TestMain in app, cli, mcpserver, web, hostintegration);
  `make check` passes in the user's plain shell.
- Wave 4 (2026-10-10): worktree I merged (`1ef2841`..`7cd3fb0`): telemetry help, schema `$id`
  preserved in shrink, Claude Code split profiles. Proposal pending:
  `archive/plans/261010-curator-via-cli/2026-10-10-curator-via-cli.md` §9 (answered).

- Simplify open decision 2 decided 2026-10-10: cross-skill comparisons stay per skill.

## How the user works with the lead

- Agents run in their own git worktree, started by the user with
  `cd <worktree> && omp --profile=<gemini-tetcu72|openai-tetcu72|openai-tetnu>`. The lead
  writes the prompt to `docs/plans/prompt-*.md`, the user pastes it and pastes back the
  report.
- The lead verifies independently. Build into the scratchpad. Clone the live hub with
  isolated `XDG_*` dirs before trying a migration. Run `git merge-tree --write-tree` and
  then `git merge --ff-only`. Run `make check` (rerun `TestGitRevisionAt` once if it fails
  with "broken pipe") and `make web-check` when web changes. For host changes, smoke
  `connect` in a temp HOME.
- Never edit distill-lab (`.claude/skills/distill-lab`). Never write to the live hub, migrate
  it, or push anything without the user asking. Do not touch a worktree while its agent runs.
- Tests must not read paths outside the repo or a temp dir (fixtures live in testdata).
- Never remove telemetry event types or change EventVersion. Use conventional commits.
- Reply to the user in Vietnamese, as one message they can paste where needed.
- The user's shell aliases `claude` to `--dangerously-skip-permissions` (`~/.zshrc:421`); for
  permission tests ask them to run `command claude`. They work on this server over SSH.
