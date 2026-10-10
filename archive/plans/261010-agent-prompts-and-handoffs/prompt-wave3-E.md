# Wave 3 · Worktree E prompt: case-journal follow-up, O7 baseline tooling, tools/list measurement

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-observer && <agent command>
```

```text
You are worktree E. You continue the observer plan
docs/plans/2026-10-08-observer-and-enrichment.md in the Go repo mcp-skill-hub,
after wave 2 (worktree D) merged into main at b32d678.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory, before anything else)
- Your worktree is /home/vantt/projects/mcp-skill-hub-observer, on the NEW branch
  wave3/observer-baseline. The lead already created it from main.
- Run `git -C /home/vantt/projects/mcp-skill-hub-observer status -sb`. It must show
  `## wave3/observer-baseline`. If it does not, stop and report.
- Every read, edit and write call uses an ABSOLUTE path under
  /home/vantt/projects/mcp-skill-hub-observer/. Never use a relative path.
- Never write to /home/vantt/projects/mcp-skill-hub (the lead's main checkout),
  /home/vantt/projects/mcp-skill-hub-simplify (worktree C) or /home/vantt/skill-hub
  (the live hub). Before each commit, `git -C /home/vantt/projects/mcp-skill-hub status -s`
  must show nothing but the lead's prompt files.

Worktree C works in parallel on simplify Phase 3 (removes the insight/run
lifecycle, ~15 MCP tools, and moves to .meta/distill.yaml). C owns registerTools,
insight_tools.go, distribution.go, internal/{canonical,workspace,migration,
distill,insight,source,mutation,systemskills}, system-skills/, and every schema
except telemetry-event-v1. Observer §5.1 (profile split), Phase 5 (a)
snapshot_expired current_uri, observer Phase 3 and Phase 4 all wait for C. Do NOT
start them.

Read before coding: observer plan §1.2 O7, §1.3, Phase 2, §5.1, §8, §9; doc 04
§4.1 and §9; docs/plans/observer-phase-01a-telemetry.md and
observer-phase-02-case-journal.md; the wave 2 code in internal/telemetry/cases.go,
redactor.go, internal/app/resolver.go, mcpserver/activation_tracker.go.

Scope, in this order. Each part is one or more commits with `make check` green.

Part 1: case-journal follow-ups (from the lead's review)
- Fact KEYS in a case's request.context.facts go through an allowlist of known
  safe tokens (lowercase [a-z0-9_.-], max 64 chars). A key that fails becomes
  "other". Values keep going through the redactor. Same rule in the resolver
  path and in the tracker (redactRequest). One shared helper, not two copies.
- NewEventID must never return a constant on failure ("evt_fallback" collides).
  Return an error, or fall back to a time+counter id that stays unique; callers
  must not record a case with a duplicate event_id.
- Move the case limits out of the SQL literals in cases.go (50 per day, 500
  total, 90-day retention) into named fields on telemetry.Config with those
  defaults, so tests can set small values. No behavior change.
- Tests for all three.

Handoff notes from worktree D (read them, they affect your work)
- Tracker RAM (notedResolution) lives only in activationTracker memory: 1h TTL,
  16 resolutions per session. After a server restart between skill_resolve and
  skills/get, the load falls back to attribution `unsolicited` with empty
  top-k. The baseline must report the unsolicited share per bucket so this
  loss is visible, not silently counted as ignores.
- The case flag is read on every RecordCase via IsCaseJournalEnabled
  (SKILLHUB_CASE_JOURNAL env, then runtime/case_journal.json). Event
  content_mode is always "none".
- On load events, topk_channels values are joined with "+" to fit the token
  regex. Split on "+" when you read them.
- internal/source TestGitRevisionAt can flake with a network broken pipe. It is
  C's package: do not edit it. If it fails, rerun make check once and mention
  it in your report.

Part 2: O7 baseline tooling (capture comes later, after C's Phase 4 merges)
- Write docs/plans/observer-phase-01b-baseline.md first: goal, files, steps,
  tests, acceptance, and the chosen N with its reasoning (state the margin of
  error a rate has at that N; the hub has low traffic, so justify the number,
  do not just pick one).
- Add `skillhub telemetry baseline [--since] [--min-chains N] [--json]
  [--write <path>]`:
  - It reuses the funnel/chains code (no second metric implementation). It
    reports per catalog_snapshot × client: resolved chains, no_skill chains,
    the O5 rates with raw counts, and first-valid day.
  - Sufficiency verdict per bucket: `sufficient` only when resolved ≥ N AND
    no_skill ≥ N; otherwise `insufficient` with the missing counts. A rate
    that cannot be measured is `unknown`, never 0.
  - Only the newest catalog_snapshot counts as the baseline window (the plan:
    the baseline starts after the snapshot change). Older snapshots are
    listed but marked `superseded`.
  - The window must lie inside raw retention; if part of it was pruned, say so.
  - `--write` writes the JSON to the given path (for docs/brainstorm/ or a
    report). It writes no task text or case content. Test that.
- Tests: insufficient → sufficient as events are added; a snapshot change
  resets the baseline; unknown vs 0; JSON contains no case text.

Part 3: tools/list measurement harness (measure only, no tool changes)
- Add a test-only or CLI dev measurement that starts the MCP server in-process
  and reports `tools/list` bytes and approximate tokens (bytes/4): total, per
  tool, and the share taken by outputSchema. Also report the same numbers for a
  hypothetical runtime set {skill_resolve, skill_get, skill_feedback}.
- It must not fail on sizes (it is a measurement, not a gate) and must not edit
  registerTools or any tool definition. Put it in a NEW file in
  internal/delivery/mcpserver (for example tools_size_test.go behind
  `-run ToolsListSize -v`) or under internal/delivery/cli telemetry*.go.
- Record today's numbers in observer-phase-01b-baseline.md next to the
  2026-10-08 numbers from §5.1, so §5.1 can be re-measured after C's Phase 3
  with the same tool.

Owned paths
- internal/telemetry/**
- internal/app/{resolver,usage,usage_chains,telemetry,caller_context}.go and new
  app files for the baseline
- internal/delivery/mcpserver/{activation_tracker,resolver_tools}.go and NEW
  test files in mcpserver (not server.go's registerTools)
- internal/delivery/cli/telemetry*.go
- schemas/telemetry-event-v1.schema.json (additive only, if needed)
- docs/design/04, docs/plans/observer-phase-*.md

Rules
- Read AGENTS.md and follow it. Never remove telemetry event types or change
  EventVersion. Additive schema changes land with their code.
- Conventional commits, each ending with
  "Co-Authored-By: <your real model name> <noreply@...>".
- `make check` green before EVERY commit. No push, no merge.
- If main moves (C merges), `git -C /home/vantt/projects/mcp-skill-hub-observer
  rebase main`, resolve conflicts only in your owned paths, run make check.

Final report
- worktree, branch, main checkout untouched
- commits (hash + subject)
- chosen N and its reasoning
- baseline command output on a test fixture (JSON excerpt)
- tools/list numbers measured today vs 2026-10-08
- tests added, make check result, deviations
- what is blocked on C (§5.1, Phase 5 (a), observer Phase 3/4)
- end with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and a one-sentence summary
```
