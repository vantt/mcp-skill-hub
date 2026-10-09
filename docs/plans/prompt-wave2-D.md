# Wave 2 · Worktree D prompt: observer O3/O4 remainder → case journal (Phase 2) → O8

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-observer && <agent command>
```

```text
You are worktree D, implementing the rest of observer Phase 1 and then observer
Phase 2 of docs/plans/2026-10-08-observer-and-enrichment.md in the Go repo
mcp-skill-hub.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory, before anything else)
- Your worktree is /home/vantt/projects/mcp-skill-hub-observer, on branch
  wave2/observer-o3o4-cases. The lead already created it from main.
- Run `cd /home/vantt/projects/mcp-skill-hub-observer && git status -sb`. It must show
  `## wave2/observer-o3o4-cases`. If it does not, stop and report.
- Every read, edit and write call uses an ABSOLUTE path starting with
  /home/vantt/projects/mcp-skill-hub-observer/. Every bash call runs there.
  Many agent tools resolve relative paths against the process directory, not
  your bash cwd, so never use a relative path.
- Never write to these places. No edits, and no git add/commit/switch/branch/
  stash/reset/merge/worktree there:
  - /home/vantt/projects/mcp-skill-hub (the lead's main checkout)
  - /home/vantt/projects/mcp-skill-hub-simplify (worktree C)
  - /home/vantt/skill-hub (the live hub)
- Before each commit, run `git -C /home/vantt/projects/mcp-skill-hub status -s`.
  It must be empty except for the lead's plan/prompt files. If anything else
  shows, stop and report.

Another agent, worktree C, works in parallel in /home/vantt/projects/mcp-skill-hub-simplify
on plans/261008-1433-simplify-hub-model/plan.md: Phase 4 (skill.meta.yaml →
.meta/skill.yaml) and then Phase 3 (minimal distill model). C removes the
insight/incorporation lifecycle and owns registerTools.

Read these before coding:
- observer plan in full, especially §0, §1.1–1.3 (O3, O4, O8), Phase 2, Phase 5,
  §7 (D3, D4, D8), §8, §9 and the Red Team Review
- simplify plan D7, D10 and "Parallel execution"
- what wave 1 (worktree A) already did:
  - docs/plans/observer-phase-01a-telemetry.md
  - commits c748e94..8747e7b: CallerContext, client enum, prior_verified, chain
    metrics, telemetry chains, retention 30d, clarification/eval/catalog events
- docs/design/04 §3, §4.1, §9

Scope, in this order. Commit each part with `make check` green before starting the next.

Part 1: O3 remainder (counters, emit only)
- Phase 5 question 13 counters:
  - a resources/read for a path not listed in the skill's skills/get `resources`
  - a call to an unsupported skills/* method
- A `snapshot_expired` counter: a request on a skill URI whose digest is no longer current.
  Do NOT change the error message or the URI format. Phase 5 decision (a) belongs to a later wave.
- tools/list payload bytes per client (doc 04 §9.3).
- Use existing event types where they fit. A new event type or payload field is allowed only
  as an addition: add it to the Go allowlist (internal/telemetry/events.go) AND to
  schemas/telemetry-event-v1.schema.json in the same commit, and keep event_version "1".
- Emit from the MCP adapter handlers (server.go readResource / custom-method handlers) or the
  app layer. Do NOT edit registerTools or internal/app/distribution.go. If a counter needs one
  of those, write it as a follow-up instead.
- Expose the counters in `skillhub telemetry funnel`, with raw counts.

Part 2: O4, bounded top-k trace (observer D3)
- k ≤ 5, encoded as PARALLEL TOKEN ARRAYS on the resolution event:
  - `topk_skill_ids`
  - `topk_matched` (`<rank>:<field>`)
  - `topk_channels`
  Never use an array of objects, which violates the flat allowlist.
- Persist them only when the chain shows disagreement (override / after_no_skill /
  verified rejected). Otherwise keep them in tracker RAM only. Record rank and matched fields
  only, never scores (doc 04 §4.1 persist_candidate_scores: false).
- In internal/resolver, expose the data on Response (`json:"-"`) and emit only. Ranking and
  scoring must not change: routing eval results stay identical (prove it with
  `skillhub`'s routing eval test or the existing eval gate test).
- Keep the rule "invalid field → event rejected". Add a producer test that always emits valid
  top-k fields, and a funnel test that denominators do not drop after the fields are added.
- `skillhub telemetry chains` shows the rank of the chosen skill and its matched fields when
  top-k exists.

Part 3: observer Phase 2, the case journal (OPT-IN, observer D4)
- Write docs/plans/observer-phase-02-case-journal.md first: goal, files, steps, tests,
  acceptance.
- The redactor is its own deliverable and comes FIRST, with tests, before any persistence:
  - secret patterns (tokens, keys, URLs with credentials, emails)
  - path normalization (home dir → ~, absolute repo paths)
  - a length cap
  - a `redaction_version` label (the existing label "redact-v1" is in events.go:15)
- The opt-in flag is OFF by default. Keep it in the runtime/telemetry configuration your
  paths own; do not add it to internal/workspace (C's). Document it in doc 04 §4.1 as the
  `redacted` content mode for cases only.
- Tracker RAM keeps, per resolution:
  - task.description
  - operation
  - the normalized request, with fact values through an allowlist redaction
- Write a case to a separate runtime store (not Git, not the telemetry event table) only on
  disagreement:
  - override
  - after_no_skill
  - VERIFIED reformulation
  - needs_context → resolved
  - rejected / scope_mismatch
  - a repeated gap
  Agreeing resolutions write no content.
- Case shape follows the example in observer Phase 2, plus `catalog_snapshot`,
  `prior_verified`, the top-k arrays and `followup`.
- Limits:
  - retention 90 days
  - cap ~500 cases AND a per-day cap
  - `skillhub telemetry purge` also removes cases
- Case text is CLI-only: `skillhub telemetry cases [--since] [--kind] [--json]`. It must NOT
  appear in web JSON or in `telemetry export`. Add tests that assert both.
- Do not build any synthesis, lesson writing or replay. That is observer Phase 3 (wave 3);
  it writes lessons into `.meta/distill.yaml` after C's Phase 3.

Part 4: O8, agent guidance (one line, no new field)
- In the host CLAUDE.md template (internal/hostintegration/bootstrap.go) and this repo's
  CLAUDE.md template text:
  - When you do not use the recommended skill, re-resolve with `prior.kind: rejected` instead
    of picking one yourself (PRD §30).
  - When you use a different skill, call `skill_feedback` with the real `skill_id`.
- Do NOT edit the curator SKILL.md (internal/systemskills, system-skills/curator). Worktree C
  owns it this wave. Put the exact curator text in your final report as a follow-up.
- Update hostintegration golden/tests deliberately.

Required tests
- Event rows stored before your change still pass validateStored and stay readable by
  export, feedback and promotion. event_version stays "1".
- Unknown or garbled inputs to the new counters never reject the event.
- Top-k: valid parallel arrays, k ≤ 5, persisted only on disagreement, routing eval
  unchanged.
- Redactor: each secret class is removed, paths are normalized, the cap is applied.
- With the flag off, no case is ever written. With it on, only disagreement chains are written.
  Caps and retention are enforced. Case text is absent from web JSON and telemetry export.
  Purge removes cases.
- Funnel and web golden tests are updated deliberately, never deleted.

Owned paths in this wave
- internal/telemetry/**
- internal/app/{resolver,usage,usage_chains,curation_telemetry,skill_load_telemetry,telemetry,
  feedback,routing_eval,transcript_import,caller_context,catalog,error_classify}.go, plus new
  app files for cases and the redactor
- internal/resolver: emit/expose only, no ranking change
- internal/delivery/mcpserver/{activation_tracker,resolver_tools}.go, and in server.go only the
  readResource / custom-method handlers and the telemetry wiring (NOT registerTools)
- internal/delivery/cli/telemetry*.go
- telemetry views in internal/delivery/web and web/src (usage only)
- internal/hostintegration/bootstrap.go and its tests (O8 text only)
- schemas/telemetry-event-v1.schema.json
- docs/design/04
- docs/plans/observer-phase-*.md

Do NOT edit (worktree C's this wave):
- internal/{canonical,workspace,migration,skill,skillruntime,distill,insight,source,mutation,
  catalog,systemskills}
- system-skills/
- app/{distill*,insight*,operations,skill_* (except skill_load_telemetry.go),upstream_*,
  source_*,distribution}.go
- mcpserver registerTools and insight_tools.go
- schemas other than telemetry-event-v1
- docs/design/01 and docs/design/07
If you need a change there, write it as a follow-up in your final report.

Rules
- Read AGENTS.md and follow it.
- Never remove telemetry event types or change EventVersion.
- Preserve public contracts. Schema changes land in the same commit as their code.
- Conventional commits, small and focused, each ending with
  "Co-Authored-By: <your real model name> <noreply@...>".
- Run `make check` (fmt-check, vet, lint, test) green before EVERY commit.
- Never touch /home/vantt/skill-hub. Do not run the hub's MCP tools that write.
- Do not push and do not merge into main.
- If Parts 1–2 are done and Part 3 is too big for one session, stop after a clean commit and
  report exactly where Part 3 stands.
- When done, rebase on the latest main (C may have merged first), resolve conflicts only
  inside your owned paths, run `make check` again, and stop.

Final report
- worktree path, branch, and confirmation that the main checkout has none of your changes
- commits (hash + subject)
- new event types/fields and counters
- top-k encoding and the persistence rule
- redactor classes, the case store location, the flag name, caps
- tests added
- `make check` result
- deviations from the plan
- follow-ups:
  - for C: curator O8 text, any distribution.go counter
  - for wave 3: §5.1 profile split, O7 baseline N, observer Phase 3
- end with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and a one-sentence summary
```
