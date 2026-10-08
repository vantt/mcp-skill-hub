---
title: "Phase 14 — Telemetry, reproducibility and evaluation"
status: done
---

# Phase 14 — Telemetry, reproducibility and evaluation

## Objective

Measure routing and UX quality without making telemetry authoritative or privacy-invasive.

### Deliverables

- local `runtime/telemetry.db`;
- allowlisted/versioned event envelope;
- retention/purge/export;
- replay manifests and commands;
- routing, curation UX and distillation metrics;
- CI/nightly evaluation reports;
- sanitized fixture promotion workflow.

### Tasks

1. Implement default `content_mode: none`.
2. Store IDs/enums/counts/timing/reason codes only by default.
3. Use bounded async writes and drop counters.
4. Ensure telemetry failure cannot change command/resolution result.
5. Separate recommended/activated/loaded/used/completed/useful semantics.
6. Add curation metrics:
   - turns to next action;
   - unnecessary confirmations;
   - prompts per batch;
   - auto-finalization rate;
   - recovery completion;
   - routine Git-noise target zero.
7. Implement JSONL export as disposable artifact only.
8. Create experiment manifests pinning snapshot/policy/schema/model where relevant.
9. Add deterministic replay for committed sanitized cases.
10. Gate policy promotion through reviewed canonical changes.

### Exit gate

- Purging telemetry does not affect validate/rebuild/resolve.
- No raw conversation/full task is persisted by default.
- Evaluation reports distinguish statistical uncertainty and multiple valid outcomes.
- No online telemetry path mutates production policy automatically.

## Dependencies

- Phase 13 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
- Preserve authority from `docs/PRD.md`, `docs/design/*`, `final.md`, and the V1 product boundary in `plan.md`.

## Related files

Expected file ownership will be refined before cooking this phase. The likely affected areas are:

- `/home/vantt/projects/mcp-skill-hub/cmd/skillhub/`
- `/home/vantt/projects/mcp-skill-hub/internal/`
- `/home/vantt/projects/mcp-skill-hub/schemas/`
- `/home/vantt/projects/mcp-skill-hub/system-skills/`
- `/home/vantt/projects/mcp-skill-hub/testdata/`
- `/home/vantt/projects/mcp-skill-hub/docs/`

## Validation gate

Before this phase is considered done, run the narrow tests added for the phase plus the current shared gates:

```bash
go test ./...
go vet ./...
```

Add phase-specific commands from the roadmap section above when implementation reaches this phase. If shared public contracts change, also run relevant CLI/MCP contract tests and delete-runtime/rebuild tests.

## Risks

- Implementing this phase before its prerequisites can weaken Git-first and derived-state invariants.
- Adding shortcuts to satisfy a command surface can create a second source of truth outside canonical files.
- User-facing behavior can drift from Phase 01 UX fixtures if adapters format results independently.

## Execution evidence

- Telemetry uses a versioned allowlist, content-free default envelope, bounded asynchronous recording, WAL storage, retention, purge, export, drop counters, and path-anchored SQLite access. Recorder failures are isolated from domain results.
- Feedback distinguishes recommended, activated, loaded, used, abandoned, rejected, completed, failed, utility, and finite reason/basis enums. Selected skills must come from retained ordered recommendations; MCP discovery publishes the same enums.
- `curation_session_record` records explicit host/evaluator measurements idempotently without inventing measurements in the curator. Source, distill, resolver, MCP, and feedback paths are instrumented.
- Exact replay fails closed on unavailable retained generations. Reports include undefined metrics, deterministic confidence intervals, partitions, exclusions, multiple acceptable outcomes, and paired policy comparisons.
- The committed strict golden corpus and committed CLI suite/manifest/workspace overlay run through CI. Promotion creates an incomplete sanitized draft for human review and never mutates production policy.
- Purge/delete tests force a full catalog rebuild and prove validation, snapshot identity, and resolver output are unchanged.
- Independent review found no remaining behavioral blocker; the implementation is present in the working tree and will become repository-tracked when the authorized ship/commit step runs.

## Rollback

- Revert files touched by this phase using Git.
- If canonical workspace mutations were introduced, verify recovery can roll forward or restore the previous valid state before deleting runtime artifacts.
- Runtime databases and caches remain disposable and may be removed with `rm -rf runtime/` in test workspaces only.
