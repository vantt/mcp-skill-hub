# Wave 3 · Worktree F prompt: observer review fixes

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-observer && <agent command>
```

```text
You are worktree F. An independent review of the observer/telemetry code now on
main (worktrees D and E) found the defects below. The lead verified them. Fix
them test-first, in the Go repo mcp-skill-hub.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
- Worktree /home/vantt/projects/mcp-skill-hub-observer, branch
  wave3/observer-review-fixes (the lead created it from main c5696e4).
  `git -C /home/vantt/projects/mcp-skill-hub-observer status -sb` must show it.
- ABSOLUTE paths only, under /home/vantt/projects/mcp-skill-hub-observer/.
- Never write to /home/vantt/projects/mcp-skill-hub (lead's main checkout),
  /home/vantt/projects/mcp-skill-hub-simplify (worktree C) or /home/vantt/skill-hub.

Worktree C is rewriting internal/delivery/mcpserver/server.go (registerTools),
MCP tools, web routes, CLI distill/inbox/insight and the curator right now. Do
NOT edit server.go, distribution.go, registerTools, internal/{canonical,
workspace,migration,skill,distill,insight,source,mutation,systemskills},
system-skills/, web/src, or schemas other than telemetry-event-v1. Item 5
below is therefore telemetry/CLI-side only.

Read first: docs/plans/observer-handoff-wave3.md, docs/plans/observer-phase-02-case-journal.md,
docs/plans/observer-phase-01b-baseline.md, docs/design/04 §4.1 and §9, AGENTS.md.

Fix, in this order, one commit per item (or tightly related pair), make check
green before each:

1. HIGH, redactor gaps (internal/telemetry/redactor.go:30,35). These pass
   through unchanged today: `access_token=abc123secret`, `DB_PASSWORD=p4ss`,
   `GITHUB_TOKEN=...`, `client_secret: zzz`, `AWS_SECRET_ACCESS_KEY=...`,
   `api-key: k1`, `x-api-key=foo`, `Authorization: Basic ...`, `sk-proj-abc...`.
   Match prefixed/hyphenated keys (no leading \b; e.g.
   `[A-Za-z0-9_-]*(api[_-]?key|password|passwd|secret|token)`), add
   `Basic\s+\S+`, allow `-` inside `sk-...`. Add every string above as a test
   case. Do not over-redact ordinary words ("tokenizer", "secretary"): test that too.

2. MEDIUM, case kinds (internal/app/resolver.go:60-73). normalize
   (internal/resolver/normalize.go:183) only accepts prior.kind clarification or
   rejected, so needs_context_resolved, repeated_gap and scope_mismatch never
   fire correctly, and a failed request still writes a case with unvalidated
   fields.
   - Write no case when resultErr != nil.
   - Branch on the normalized request (after normalize), not the raw Kind.
   - Map the spec's kinds onto what exists: a `clarification` prior that
     resolves → needs_context_resolved; verified rejected → verified_reformulation;
     unverified rejected → rejected. A repeated gap = no_skill for the same
     resolution fingerprint seen before in this session (tracker), not a
     prior.kind. Drop scope_mismatch unless the request model has it; say so in
     the phase-02 doc.
   - Tests per kind, and "Rejected" (mixed case) behaves like "rejected".

3. MEDIUM, baseline per-bucket counts (internal/app/usage_baseline.go:188 with
   usage_chains.go:424-426). Each bucket uses the global counts map, so
   bypass_rate and needs_context_answer_rate are identical across buckets and
   negative_after_load can exceed 1 and get clamped to 1.0. Build counts per
   bucket by filtering events on snapshot and client. Test with two buckets that
   have different counts; assert every rate is in [0,1] without clamping (make
   makeRate report the overflow in a test-visible way, or assert numerator ≤
   denominator before calling it).

4. MEDIUM, SQLite off the worker (internal/telemetry/cases.go:95-178,
   recorder.go:282-291). RecordCase, Cases and OldestRawEventTime open the DB in
   the caller goroutine (full quick_check + schema setup each time, inside
   skill_resolve) and only take the read gate, so they can overlap maintenance
   (DELETE/VACUUM/checkpoint) and purge.
   - RecordCase enqueues onto the existing worker; it must not block the request.
   - Cases / OldestRawEventTime / any admin read run through the worker or take
     the gate so they cannot overlap maintenance or purge.
   - A purge must not be undone by a racing RecordCase.
   - Do the cap check and insert in one BEGIN IMMEDIATE transaction; if the count
     query fails, do not insert (fail closed); total-cap eviction removes
     count-limit+1 rows, not one.
   - Tests: concurrent RecordCase calls never exceed the daily cap; purge during
     RecordCase leaves no cases; run `go test -race` on the package.

5. MEDIUM, two O3 counters cannot measure (server.go:157-160, 298-310; do NOT
   edit server.go). In go-sdk, unknown methods are rejected before middleware,
   so unsupported_method_calls is always 0; unlisted_resource_reads is already
   blocked by ReadResource and only differs after a catalog change (false
   positive). In the funnel/baseline output show both as `unknown` (with a
   reason) instead of a confident 0, and write a follow-up for worktree C with
   the exact server.go change (count at transport level / compare against the
   pinned manifest, or remove). Also: snapshot_expired must not count
   ErrNotFound; put that in the same follow-up.

6. LOW, batch:
   - tools_list_bytes (rollup.go:95-97) sums every tools/list response; report
     the latest or max value per client, not the sum.
   - activation_tracker.go:256 resolutionData returns the oldest entry for a
     resolution_id while attributeDetails picks the newest; iterate newest-first.
   - PurgeCases/PruneCases are only called from tests: wire them in or delete
     them (purge already deletes the whole DB file; state which).
   - TestToolsListSize asserts nothing: keep it as a measurement but assert the
     payload is non-empty and the runtime set is a subset of all tools.

Rules: AGENTS.md; never remove telemetry event types or change EventVersion;
additive schema only; conventional commits ending with
"Co-Authored-By: <your real model name> <noreply@...>"; make check green
before EVERY commit; no push, no merge. If main moves, rebase on main and run
make check again.

Final report: commits (hash + subject), per item what changed and the test that
proves it, the follow-up text for C (item 5), make check result, deviations.
End with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and one sentence.
```
