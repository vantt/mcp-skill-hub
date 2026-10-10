# Phase 3: Minimal Distill Model (D1-D3, D7)

## Overview

This phase replaces the heavyweight distillation subsystem (runs, revision packages, state machines, insight/incorporation entities, comparisons, LINK files, and top-level sources layout) with a skill-anchored knowledge model stored in `.meta/distill.yaml` per skill (D1, D7).

Git is established as the sole database and audit mechanism for source evidence (`repo@commit:path[#Lx-Ly]`). The binary adds only what Git lacks: preview/confirm pins, human content approval, and the workspace lock.

## Target Data Model: `.meta/distill.yaml` (D7)

Each skill maintains exactly one hub-side distillation document at `skills/<collection>/<id>/.meta/distill.yaml`.
Per D4 and Phase 2, this file is excluded from catalog snapshots, content digests, and distributions (`IsHubMeta`).

```yaml
goal:
  status: confirmed # draft | confirmed
  purpose: "Every test justifies its maintenance cost by protecting observable behavior..."
  in_scope:
    - authoring gate for new or changed tests
    - audits that find and remove junk
  out_of_scope:
    - writing features or fixing non-test production bugs
  failures_it_prevents:
    - junk tests accumulate
    - tests break under behavior-preserving refactoring

cursors:
  openclaw: 96370cf438dc9d67d2a13b08c14eb4142fecb767
  superpowers: 8ca22dba9a94f28898bbce59f2537ff4d87c747d

coverage:
  openclaw:
    read:
      - .agents/skills/test-audit
    not_read:
      - path: src/**/*.test.ts
        reason: the tests themselves

lessons:
  - key: mocks-mirror-the-real-shape
    layer: content # enforcement | content | history | validation | craft | wording | ecosystem
    what: "A mock reproduces the complete real shape it replaces..."
    notable: "Both sources reached this independently for the same reason..."
    where:
      - superpowers@8ca22dba9a94:skills/test-driven-development/writing-good-tests.md#L119-L122
      - openclaw@96370cf438dc:docs/help/testing/writing-tests.md#L121-L131
    contrast: extends # new | extends | contradicts | already-covered
    score:
      relevance: 3 # 0-3
      facts: [a, b, c, d, e] # distinct letters from a-e, requires 'a'
      impact: 5 # len(facts) if 'a' in facts else 0
      evidence: 2 # 1-3
      effort: 1 # 1-3
      why: "The mock shape failure is silent and breaks downstream integration."
    final_score: 30.0
    also_fits: [hub]
    decision:
      state: candidate # candidate | planned | ported | rejected
      reason: "Optional explanation (required if rejected)"
      at: "2026-10-09T08:15:00Z"
```

### Key Lesson Rules (distill-lab format)

1. **Authority:** `.claude/skills/distill-lab/scripts/distill.py` is the authority for the format.
2. **Scorecard Fields:** Relevance (0-3), Facts (subset of a-e, must contain a if non-empty), Impact (0-5, fact count), Evidence (1-3), Effort (1-3), Why (non-empty string), FinalScore (relevance * impact * evidence / effort).
3. **`where` Evidence & Cursors:**
   - References follow `repo@<commit>` or `repo@<commit>:path[#Lx-Ly]` with 7-40 hex commit SHAs.
   - Every source cited in `where` must be declared in `cursors`.
4. **Porting to Skill Content (D9):** Applying a lesson to skill content occurs through a standard `skill_update` preview showing the excerpt and requiring human confirmation.

## Components Kept, Removed, and Modified

### 1. Removed Components

- **Revision Packages:** `internal/distill/revision_package.go` and the gitignored package cache (`runtime/cache/packages/`). No packaging or extraction into temporary directories.
- **Run State Machine:** Runs, run transitions (`preparing`, `running`, `awaiting_decision`, `finalized`, `cancelled`), run ID generations, and run directories (`distill/runs/`).
- **Insight & Incorporation Lifecycle:** `internal/insight/` package (`insight.go`, `incorporation.go`, `proposal.go`, `outcome.go`) and corresponding entity directories (`distill/insights/`, `distill/proposals/`, `distill/incorporations/`, `distill/outcomes/`).
- **Comparisons & Reports:** `distill/comparisons/` and `distill/sources/` directories. Cross-skill or cross-source comparison entities are removed; multi-source convergence lives in the lesson `where` list.
- **LINK Files:** `sources/skills/LINK-*.yaml`. Relationship is expressed directly in `.meta/skill.yaml` sources list.
- **Top-Level `sources/` Layout:** `sources/intake/`, `sources/catalog/`, `sources/skills/`. Every source belongs to a skill (D3). Sources saved for later attach to an existing skill or to a new draft skill. Shared sources deduplicate through the git mirror cache.

### 2. Kept Components

- **Preview/Confirm Pin Store:** `mutation.Proposal`, runtime pin store in `runtime/pins/`, ensuring atomic preview and confirmation.
- **Workspace Lock:** `mutation.AcquireLock` and `mutation.AcquireSharedLock` against concurrent writers.
- **Human Content Approval:** Skill content approval (`quality.content_reviewed_digest`) remains strictly human and CLI-driven.
- **Stable Catalog Identity:** `catalog_snapshot` calculation (`internal/canonical.Snapshot`) remains stable.
- **Temporary Build-Only Catalog:** Ability to build an ephemeral catalog without publishing it (`BuildCatalogGeneration`, D10, observer replay).
- **Shared Git Mirror Cache:** `runtime/sources/git/` shared bare git repositories keyed by URL.

### 3. Added / Modified Components

- **`.meta/distill.yaml` Engine:** Structured reader, writer, and validator in `internal/distill/` and `internal/canonical/`.
- **Draft Skills (D3):** Unattached sources saved for later create a draft skill (`status: draft`). Draft skills are excluded from routing and routing evals.
- **Migration WAL `planV3ToV4` (Schema "4"):** Migrates legacy runs, insights, decisions, rejections, tombstones, and coverage gaps into `.meta/distill.yaml`.

## Contract Inventory Table

Status for every public interface affected by Phase 3:

| Interface Type | Identifier | Status | Notes / Replacement |
|---|---|---|---|
| **MCP Tool** | `inbox_list` | Removed | Replaced by reading `.meta/distill.yaml` lessons per skill. |
| **MCP Tool** | `insight_get` | Removed | Replaced by lesson inspection in `.meta/distill.yaml`. |
| **MCP Tool** | `insight_decide` | Removed | Replaced by updating lesson inline decision (`candidate`, `planned`, `ported`, `rejected`). |
| **MCP Tool** | `insight_apply_preview` | Removed | Replaced by standard `skill_update_preview` showing excerpt (D9). |
| **MCP Tool** | `insight_apply_confirm` | Removed | Replaced by standard `skill_update_confirm` (D9). |
| **MCP Tool** | `curation_run_prepare` | Removed | Distillation operates directly against skill sources without run state machine. |
| **MCP Tool** | `curation_run_start` | Removed | Run lifecycle removed. |
| **MCP Tool** | `curation_run_submit` | Removed | Atomic write to `.meta/distill.yaml`. |
| **MCP Tool** | `curation_run_get` | Removed | Read `.meta/distill.yaml` directly. |
| **MCP Tool** | `curation_run_retry` | Removed | Run lifecycle removed. |
| **MCP Tool** | `curation_run_cancel` | Removed | Run lifecycle removed. |
| **MCP Tool** | `observation_list` | Removed | Lessons in `distill.yaml` are the distilled observations. |
| **MCP Tool** | `comparison_get` | Removed | Multi-source convergence is recorded in lesson `where` entries. |
| **MCP Tool** | `outcome_record` | Removed | Insight/incorporation lifecycle removed. |
| **MCP Tool** | `source_intake_add` | Removed | Sources attach directly to skills (D3). |
| **MCP Tool** | `source_intake_list` | Removed | Intake directory deleted. |
| **MCP Tool** | `source_triage` | Removed | Intake directory deleted. |
| **MCP Tool** | `source_link_preview` | Removed | Sources declared directly in `.meta/skill.yaml`. |
| **MCP Tool** | `source_unwatch_preview` | Removed | Sources detached in `.meta/skill.yaml`. |
| **MCP Tool** | `source_list` | Added | Replaces the removed `sources/intake` and `distill/sources` exploration paths. |
| **MCP Tool** | `source_check` | Kept | Checks sources for upstream changes. |
| **MCP Tool** | `source_diff` | Kept | Unchanged. |
| **MCP Tool** | `source_import_preview` | Kept | Unchanged. |
| **MCP Tool** | `source_import_confirm` | Kept | Unchanged. |
| **MCP Tool** | `source_watch_preview` | Kept | Unchanged. |
| **MCP Tool** | `source_watch_confirm` | Kept | Unchanged. |
| **MCP Tool** | `skill_upstream_status` | Kept | Unchanged. |
| **MCP Tool** | `skill_resolve` | Kept | Unchanged. |
| **MCP Tool** | `skill_feedback` | Kept | Unchanged. |
| **MCP Tool** | `skill_list` | Kept | Unchanged. |
| **MCP Tool** | `skill_get` | Kept | Unchanged. |
| **MCP Tool** | `skill_review` | Kept | Unchanged. |
| **MCP Tool** | `skill_add_preview` | Kept | Unchanged. |
| **MCP Tool** | `skill_add_confirm` | Kept | Unchanged. |
| **MCP Tool** | `skill_create_preview` | Kept | Unchanged. |
| **MCP Tool** | `skill_create_confirm` | Kept | Unchanged. |
| **MCP Tool** | `skill_transition_preview` | Kept | Unchanged. |
| **MCP Tool** | `skill_transition_confirm` | Kept | Unchanged. |
| **MCP Tool** | `skill_update_preview` | Kept | Used for porting learning text into skill content (D9). |
| **MCP Tool** | `skill_update_confirm` | Kept | Used for confirming skill content updates. |
| **MCP Tool** | `routing_evaluate` | Kept | Unchanged. |
| **MCP Tool** | `curation_session_record` | Kept | Unchanged. |
| **MCP Tool** | `hub_status` | Kept | Action counts updated for minimal distill model. |
| **MCP Tool** | `workspace_validate` | Kept | Validates `.meta/distill.yaml`. |
| **MCP Tool** | `workspace_rebuild` | Kept | Unchanged. |
| **MCP Tool** | `workspace_diff` | Kept | Unchanged. |
| **Web Route** | `/api/v1/inbox` | Removed | Replaced by skill distill view. |
| **Web Route** | `/api/v1/insights/*` | Removed | Replaced by skill distill view. |
| **Web Route** | `/api/v1/distill/runs/*` | Removed | Run state machine removed. |
| **Web Route** | `/api/v1/distill/comparisons/*` | Removed | Comparison entities removed. |
| **Web Route** | `/api/v1/sources/intake` | Removed | Intake layout removed. |
| **Web Route** | `/api/v1/skills/{id}/distill` | Added | GET/POST for `.meta/distill.yaml`. |
| **Web Route** | `/api/v1/skills/*` | Kept | Detail, runtime, review, usage routes kept. |
| **Web Route** | `/api/v1/sources/*` | Kept | Source routes kept. |
| **CLI Command** | `skillhub distill` | Removed | Distill run state machine and subcommands removed in favor of direct `.meta/distill.yaml` knowledge model. |
| **CLI Command** | `skillhub inbox` | Removed | Inbox removed in favor of per-skill `.meta/distill.yaml` lessons. |
| **CLI Command** | `skillhub insight` | Removed | Insight decision/application lifecycle removed in favor of standard skill updates and inline lesson decisions. |
| **JSON Schema** | `schemas/distill.schema.json` | Added | Schema for `.meta/distill.yaml`. |
| **JSON Schema** | `schemas/distill-submission.schema.json` | Removed | Superseded by `distill.schema.json`. |
| **JSON Schema** | `schemas/insight.schema.json` | Removed | Entity removed. |
| **JSON Schema** | `schemas/proposal.schema.json` | Removed | Entity removed. |
| **JSON Schema** | `schemas/incorporation.schema.json` | Removed | Entity removed. |
| **JSON Schema** | `schemas/source-link.schema.json` | Removed | LINK files removed. |
| **JSON Schema** | `schemas/source-intake.schema.json` | Removed | Intake directory removed. |
| **JSON Schema** | `schemas/comparison.schema.json` | Removed | Comparison entities removed. |
| **JSON Schema** | `schemas/skill-metadata.schema.json` | Kept | Updated in Phase 4. |
| **JSON Schema** | `schemas/telemetry-event-v1.schema.json` | Kept | Kept verbatim (D10). |
| **Telemetry Event** | `eventPayloads` (all 31 event types) | Kept | **All 31 event types kept; EventVersion="1" preserved** (D10). |

## Migration: planV3ToV4 (Schema "4")

1. **WAL Step:** Registers `planV3ToV4` in `internal/migration/` to bump `.skillhub/schema-version` from 3 to 4.
2. **Data Transformation:**
   - Gathers legacy distill runs, observations, comparisons, insights, decisions, rejections, tombstones, and coverage gaps across `distill/` and `sources/`.
   - Identifies target skill for each insight and maps findings into lessons within that skill's `.meta/distill.yaml`.
   - Preserves human decisions (`rejected`, `planned`, `ported`).
   - Populates `seen_where` from historical evidence so decided lessons remain decided unless new evidence arrives.
   - Deletes legacy directories (`sources/intake/`, `distill/runs/`, `distill/comparisons/`, `distill/sources/`, `distill/insights/`, `distill/proposals/`, `distill/incorporations/`, `distill/outcomes/`, `sources/skills/LINK-*.yaml`).
3. **Pre-Phase-3 Fixture Test:**
   - Creates a realistic pre-Phase-3 fixture with runs, insights, human decisions, rejections, tombstones, and coverage gaps.
   - Runs migration v3→v4.
   - Asserts that all human decisions, reopen-only-on-new-evidence behavior, tombstones, and coverage gaps survive in `.meta/distill.yaml`.

## Server-Boundary Test

- Test verifying that every MCP tool referenced in the body of `system-skills/curator/SKILL.md` is registered in `internal/delivery/mcpserver/server.go` and declared in its `compatible-tools` frontmatter.
- Keeps `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md` strictly synchronized.

## Acceptance Criteria

1. `.meta/distill.yaml` is the single source of truth for skill distillation (goal, cursors, coverage, lessons).
2. `notable` is a short explanatory text; `contrast` and R/E/F scores are optional.
3. Adding a new `where` entry to a lesson reopens a decided lesson back to candidate.
4. Legacy entities (revision packages, runs, insights, incorporations, LINK files, intake) are completely removed without dead code.
5. Telemetry event types and `EventVersion: "1"` remain untouched.
6. Public contract table is recorded in `docs/design/07` alongside code removal.
7. `make check` is green with all tests passing.

## Recorded Deviations

1. **distill-lab specification alignment:** Per user decision, `.meta/distill.yaml` format matches `.claude/skills/distill-lab/scripts/distill.py`. Existing `.meta/distill.yaml` files are preserved byte-identical during v3->v4 migration. The `seen_where` automatic reopening rule and `usage:<case_id>` citations are deferred until distill-lab supports them.
