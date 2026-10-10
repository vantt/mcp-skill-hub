# Lead handoff, 2026-10-10

State after wave 3. Read this first in a new lead session, then the two plans.

## Where things are

- Repo `/home/vantt/projects/mcp-skill-hub`, main = origin/main = `d65b0e7`. No other
  worktrees or branches.
- Live hub `/home/vantt/skill-hub`, schema **v5**, main = origin/main = `786c60e`. Local
  tags `pre-schema-v4` and `pre-schema-v5` (not pushed) mark the state before each
  migration.
- Binary `~/.local/bin/skillhub` built from `d65b0e7`. The MCP server running in old
  Claude Code sessions predates v5; restart Claude Code before relying on it.
- Case journal is **enabled** on the live hub (`runtime/case_journal.json`). The MCP server
  writes cases itself during `skill_resolve` (`internal/app/resolver.go:95`); agents do not.
  On 2026-10-10 the store held 7 events and 0 cases.

## Plans

`plans/261008-1433-simplify-hub-model/plan.md`: Phases 1–5 done (5 = digest-only receipts,
schema v5, `9b6c986`). Phase 0 (scorecard) is the user's own work with distill-lab.

`docs/plans/2026-10-08-observer-and-enrichment.md` (status table after §8):

| Item | State |
|---|---|
| Phase 1, 2 | done |
| §5.1 MCP profiles (runtime / curation / all) | code done; no host verified for toggling, manual check on Claude Code pending |
| 5(a), skill_list paging | done |
| 5(c) one curator source per client | done (H, `51cb26b`..`1fdd787`, fix `d65b0e7`); no client verified for `skills_extension`, so all native hosts keep the native copy and the server hides the MCP copy from claude-code/codex/gemini |
| Phase 3 enrichment | **deferred until there are cases.** Blocker: distill-lab rejects `where: usage:<case_id>`. Lead recommendation: when cases exist, promote confirmed cases to Git eval cases and cite them as `repo@commit:path` (option 1 in observer-handoff-G.md §2); do not change distill-lab |
| Phase 4 resolver | waits for the baseline (`skillhub telemetry baseline`), counting since the v4/v5 migration |
| `directoryRead` | O3 counters were removed; lead recommends deferring entirely. User has not decided |

## Small open items

- `skillhub telemetry --help` does not list the `cases` subcommand.
- `internal/delivery/mcpserver/server.go:527-533` injects `$ref: "#/$defs/setup"` into the
  skill_resolve and routing_evaluate schemas; it would panic if root `$defs` were pruned.
  Latent today (profiles strip metadata only).
- The real-hub test-audit skill has 0 examples (validate warns).

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
