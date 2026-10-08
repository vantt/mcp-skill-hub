---
title: "Phase 01 — UX contract freeze"
status: done
---

# Phase 01 — UX contract freeze

## Objective

Convert product/design prose into executable UX acceptance fixtures, command/tool result envelopes, confirmation policy fixtures, and error rendering contracts before implementation hardens schemas and services.

## Dependencies

- Source authority: `docs/PRD.md`, `docs/design/02-agent-hub-protocol.md`, `docs/design/05-curation-lifecycle.md`, `docs/design/06-source-learning-and-distillation.md`, and `plans/skillhub-v1-implementation-plan.md`.
- No product code dependency.

## Related files

Create these files/directories during cook:

- `/home/vantt/projects/mcp-skill-hub/testdata/ux/curation-home/healthy.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/curation-home/recovery-required.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/curation-home/invalid-workspace.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/source-maintenance/sources-due.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/source-maintenance/changed-sources.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/source-maintenance/unavailable-source.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/insight-apply/high-value-insight-review.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/insight-apply/stale-proposal.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/insight-apply/git-dirty-after-apply.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/recovery/interrupted-run.yaml`
- `/home/vantt/projects/mcp-skill-hub/testdata/ux/recovery/partial-distill-failure.yaml`
- `/home/vantt/projects/mcp-skill-hub/schemas/result-envelope.schema.json`
- `/home/vantt/projects/mcp-skill-hub/schemas/error-envelope.schema.json`
- `/home/vantt/projects/mcp-skill-hub/schemas/action-confirmation-policy.schema.json`
- `/home/vantt/projects/mcp-skill-hub/docs/contracts/ux-fixtures.md`
- `/home/vantt/projects/mcp-skill-hub/docs/contracts/result-envelope.md`
- `/home/vantt/projects/mcp-skill-hub/docs/contracts/error-codes.md`

Do not modify application code in this phase.

## Requirements

- Encode Curation Home, source maintenance, insight review/apply, and recovery user journeys as machine-readable fixtures.
- Every fixture must assert progressive disclosure levels:
  - L0: counts and one recommendation.
  - L1: item summary and impact.
  - L2: evidence, comparison, or diff.
  - L3: raw canonical artifacts and diagnostics.
- Every mutation fixture must map to one application command name even if the command is not implemented yet.
- Error fixtures must render `ERROR`, `WHY`, and `FIX`.
- IDs, digests, revisions, snapshots, and policy revisions are opaque strings.
- Normal user-facing copy must not require understanding cursor or snapshot terminology.

## Implementation steps

1. Read the authority documents listed in Dependencies and extract only V1 user-facing behavior.
2. Define a compact YAML fixture shape with:
   - `scenario`, `given`, `when`, `expect.status`, `expect.summary`, `expect.suggested_actions`, `expect.confirmation`, and `expect.progressive_disclosure`.
3. Create the fixture directories under `testdata/ux/`.
4. Add fixtures for these scenarios:
   - healthy Curation Home;
   - invalid workspace;
   - failed/interrupted run before optional work;
   - sources due for check;
   - changed sources;
   - batch distill with one partial failure;
   - high-value insight review;
   - stale proposal;
   - Git dirty after apply;
   - recovery-required workspace.
5. Draft JSON Schemas for result envelopes, error envelopes, and confirmation policies.
6. Document the envelope and fixture contracts in `docs/contracts/`.
7. Cross-check every planned CLI/MCP mutation against one application command name.
8. Mark fields as `public`, `experimental`, or `internal` in schema descriptions or contract docs.
9. Keep examples deterministic and free of secrets, absolute local paths, or real credentials.

## Acceptance criteria

- `testdata/ux/` contains at least the ten required scenario fixtures.
- Every fixture has exactly one recommended next action at L0.
- Every error fixture includes `ERROR`, `WHY`, and `FIX` expectations.
- Result envelope schema includes `status`, `summary`, `items`, `suggested_actions`, `warnings`, and `error`.
- Confirmation policy schema distinguishes read-only, mechanical, network, semantic, destructive, Git commit, and Git push actions.
- Contract docs explain which fields are public, experimental, and internal.
- No fixture or doc introduces a web UI, cloud dependency, auto-apply behavior, or canonical eventlog.

## Validation commands

```bash
python3 - <<'PY'
from pathlib import Path
required = [
 'testdata/ux/curation-home/healthy.yaml',
 'testdata/ux/curation-home/recovery-required.yaml',
 'testdata/ux/curation-home/invalid-workspace.yaml',
 'testdata/ux/source-maintenance/sources-due.yaml',
 'testdata/ux/source-maintenance/changed-sources.yaml',
 'testdata/ux/source-maintenance/unavailable-source.yaml',
 'testdata/ux/insight-apply/high-value-insight-review.yaml',
 'testdata/ux/insight-apply/stale-proposal.yaml',
 'testdata/ux/insight-apply/git-dirty-after-apply.yaml',
 'testdata/ux/recovery/interrupted-run.yaml',
 'testdata/ux/recovery/partial-distill-failure.yaml',
 'schemas/result-envelope.schema.json',
 'schemas/error-envelope.schema.json',
 'schemas/action-confirmation-policy.schema.json',
 'docs/contracts/ux-fixtures.md',
 'docs/contracts/result-envelope.md',
 'docs/contracts/error-codes.md',
]
missing = [p for p in required if not Path(p).exists()]
if missing:
    raise SystemExit('missing required files: ' + ', '.join(missing))
PY
python3 -m json.tool schemas/result-envelope.schema.json >/dev/null
python3 -m json.tool schemas/error-envelope.schema.json >/dev/null
python3 -m json.tool schemas/action-confirmation-policy.schema.json >/dev/null
```

If a YAML parser is available in the final toolchain, add a YAML parse check for every `testdata/ux/**/*.yaml`. Do not add a Python dependency solely for this phase unless the project has already chosen one.

## Risks

- UX fixtures may overfit early wording and freeze poor copy.
- Fixtures may leak internal terms such as snapshot/cursor into normal user paths.
- Fixture schema can become too broad and behave like a second product spec.

## Rollback

- Remove the files created under `testdata/ux/`, `schemas/`, and `docs/contracts/` in this phase.
- No product code or runtime state should exist yet, so rollback is file deletion only.
