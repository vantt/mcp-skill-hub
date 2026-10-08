# Skill Hub V1 — Phased Implementation Plan

**Trạng thái:** Completed (Đã chuyển thành plans/260928-1435-skillhub-v1 và triển khai hoàn tất)  
**Loại repository hiện tại:** Greenfield/documentation-first  
**Design sources:** [final decision](../final.md), [architecture](../../docs/design/01-system-architecture.md), [protocol](../../docs/design/02-agent-hub-protocol.md), [resolver](../../docs/design/03-resolver-design.md), [telemetry/evaluation](../../docs/design/04-telemetry-reproducibility-evaluation.md), [curation UX](../../docs/design/05-curation-lifecycle.md), [source learning](../../docs/design/06-source-learning-and-distillation.md), [storage/mutation](../../docs/design/07-storage-and-mutation-model.md)

## 1. Execution principle

Implementation order follows:

```text
UX acceptance transcripts
→ CLI/MCP contracts
→ application commands
→ domain model and invariants
→ canonical repositories/mutations
→ derived SQLite projections
→ integrations and optimization
```

Không bắt đầu bằng CRUD tables rồi expose lên UX. Mỗi vertical slice phải chứng minh một user intent hoàn chỉnh qua application service; CLI và MCP là adapters của cùng semantics.

## 2. V1 product boundary

### In scope

- one Go binary installed via `curl | sh`;
- one primary local Git data workspace per machine/OS user;
- bundled System Curator Skill;
- CLI and MCP stdio surfaces;
- `doctor`, `doctor --fix`, `init`, `validate`, `rebuild`;
- canonical skill/source/learning/decision files;
- crash-safe multi-file mutation and recovery;
- rebuildable immutable SQLite/FTS catalog generations;
- source intake/check and Git/filesystem/document adapters;
- distill findings, coverage, comparisons and insight proposals;
- pinned preview/confirm semantic mutation;
- evidence-first resolver and MCP skill distribution;
- local operational state, telemetry and evaluation;
- stock-client bootstrap instructions and compatibility tests.

### Explicitly out of V1

- Web UI;
- required Node/Python runtime;
- cloud/multi-tenant service;
- automatic Git commit/push/pull;
- server-side LLM required for normal resolution;
- vector retrieval before measured need;
- auto-apply upstream changes;
- global canonical `eventlog.jsonl`;
- database export/import as workspace portability mechanism.

## 3. Proposed codebase shape

Final package names may change, but dependency direction must remain:

```text
cmd/skillhub/
internal/
  domain/                 # pure entities, value objects and invariants
  app/                    # application commands/read models
  canonical/              # file schemas, scanner and repository
  mutation/               # lock, transaction WAL and recovery
  catalog/                # SQLite generation build/publish/read
  curation/               # skills, proposals, inbox and outcomes
  sources/                # intake, adapters and revision checks
  learning/               # distill runs/findings/comparisons
  resolver/               # retrieval/scoring/decision
  distribution/           # manifests/resources/snapshot pinning
  telemetry/              # disposable event storage
  delivery/
    cli/
    mcp/
  doctor/
  hostconfig/             # MCP registration/bootstrap remediation
  systemskills/           # embedded System Curator Skill assets
schemas/                   # canonical/tool JSON schemas
system-skills/             # source assets embedded into binary
testdata/
  ux/
  workspaces/
  resolver/
  recovery/
```

Dependency rule:

```text
delivery → app → domain/ports
infrastructure adapters → app/domain ports
app/domain must not import CLI, MCP or SQLite packages
```

## 4. Milestone overview

| Phase | Milestone | Primary proof |
|---:|---|---|
| 0 | UX and contract freeze | Conversation fixtures define expected actions/confirmations |
| 1 | Go/repository foundation | Reproducible binary and CI on supported OSes |
| 2 | Workspace and canonical read model | Init/validate scans an empty/sample workspace |
| 3 | Mutation and recovery engine | Fault injection always recovers old or intended new state |
| 4 | SQLite generation/rebuild | Delete runtime and reproduce equivalent query results offline |
| 5 | Curation Home and operational CLI | `skillhub status` recommends one correct next action |
| 6 | Skill lifecycle | Create/edit/activate/deprecate through preview/confirm |
| 7 | Source intake and monitoring | Capture/onboard/check without unchanged Git churn |
| 8 | Distillation pipeline | Valid submission auto-finalizes atomically; failed run does not advance cursor |
| 9 | Insight application and outcomes | Stale proposal cannot apply; provenance remains bidirectional |
| 10 | Resolver | Evidence-first resolver meets golden-eval gates |
| 11 | MCP distribution/protocol | Resolve/load resources with snapshot/digest consistency |
| 12 | System Curator and host setup | Stock clients can discover/use curation flow via bootstrap |
| 13 | Telemetry and evaluation | Local privacy defaults, replay and UX metrics work |
| 14 | Hardening and V1 release | Cross-platform install/upgrade/recovery release gates pass |
| 15 | Post-core options | Deep-dive/consult/vector/remote only after V1 evidence |

---

## Phase 0 — Freeze UX behavior before implementation

### Goal

Convert design prose into executable acceptance examples before schemas/services become expensive to change.

### Deliverables

- `testdata/ux/curation-home/*.yaml`;
- `testdata/ux/source-maintenance/*.yaml`;
- `testdata/ux/insight-apply/*.yaml`;
- `testdata/ux/recovery/*.yaml`;
- initial command/tool result envelopes;
- action safety/confirmation policy fixtures;
- error code registry with `ERROR / WHY / FIX` rendering.

### Tasks

1. Encode conversation fixtures for:
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
2. Define progressive disclosure assertions:
   - L0 status has counts + one recommendation;
   - findings/digests/full diff absent by default;
   - L1/L2/L3 available on demand.
3. Define confirmation fixtures from the safety matrix.
4. Draft stable JSON result envelopes:
   - `status`, `summary`, `items`, `suggested_actions`, `warnings`, `error`;
   - IDs/digests/revisions represented as opaque strings.
5. Define natural-language intent examples for bundled System Curator Skill.
6. Mark fields as public contract, experimental or internal.

### Exit gate

- Product/design review approves happy paths and failure copy.
- Every planned CLI/MCP mutation maps to one application command.
- No command exists only because a database table exists.
- Fixtures prove a normal user never needs cursor/snapshot terminology.

## Phase 1 — Go foundation, CI and release skeleton

### Goal

Create a minimal, dependency-disciplined Go binary suitable for cross-platform release.

### Deliverables

- Go module and package boundaries;
- `cmd/skillhub` entry point;
- version/build metadata;
- structured error/result types;
- config loading and workspace flag plumbing;
- CI for Linux/macOS/Windows;
- lint, unit, race and cross-compile jobs;
- release archive/checksum/signing skeleton;
- install script prototype.

### Tasks

1. Pin supported Go version and dependency update policy.
2. Choose CLI and MCP libraries after license/prototype review.
3. Prototype pure-Go SQLite + FTS5 on every release target.
4. Establish deterministic test clock/ID/digest interfaces.
5. Add filesystem abstraction only where fault injection needs it; avoid broad mock frameworks.
6. Add stable exit codes and machine-readable error codes.
7. Embed placeholder system-skill assets with `go:embed`.
8. Generate SBOM/checksums in release pipeline.

### Exit gate

- `skillhub version` works on supported platforms.
- Empty CLI starts without Node/Python/CGO runtime dependency.
- Race detector and cross-platform SQLite/FTS smoke tests pass.
- Dependency direction test/linter prevents delivery imports in domain/app.

## Phase 2 — Workspace bootstrap and canonical read model

### Goal

Create/open/validate a Git-first workspace without yet implementing business mutations.

### Deliverables

- workspace locator and primary-workspace config;
- `skillhub init <path>` skeleton;
- `skillhub validate`;
- canonical schema version reader;
- deterministic scanner;
- strict parsers for initial canonical entities;
- Git state inspection;
- `.gitignore` for `runtime/` and `.skillhub/transactions/`;
- first `doctor` findings and fix-plan model.

### Tasks

1. Implement workspace containment and nested/source-repository safety checks.
2. Create canonical empty directory/files and initialize Git when approved.
3. Implement one-file-per-ID layouts for source-learning entities.
4. Define schemas for:
   - skill metadata/resources;
   - source candidates/sources/links;
   - run/finding/comparison/insight;
   - proposal/incorporation/outcome;
   - operation receipt;
   - config/eval entities.
5. Implement two-pass validation:
   - identity/shape;
   - references/invariants.
6. Detect unresolved Git merge conflicts and unsafe symlinks.
7. Compute deterministic per-file digests, `catalog_snapshot` and `projection_input_digest`.
8. Make `init` and `doctor --fix --workspace` call one remediation application service.
9. Ensure `doctor` is read-only and fix plan requires approval/`--yes`.

### Tests

- empty directory initialization;
- valid existing compatible repository;
- reject nested/source checkout without explicit approval;
- duplicate ID and broken reference fixtures;
- path/symlink escape fixtures;
- deterministic digest across OS path separators;
- repeated init/fix is idempotent.

### Exit gate

- A workspace can be cloned/opened and fully validated without network.
- Invalid canonical state reports exact path/entity and actionable fix.
- No durable ID/cursor exists only in memory or SQLite.

## Phase 3 — Canonical mutation transaction and recovery

### Goal

Provide the only sanctioned write path for CLI/MCP/domain commands.

### Deliverables

- shared/exclusive cross-process workspace lock;
- `WriteSet`, proposal, precondition and operation abstractions;
- `.skillhub/transactions/<op>/` WAL;
- staged after/before images;
- atomic path replacement;
- operation receipts;
- idempotency lookup contract;
- recovery classifier and `doctor --fix` actions;
- conflict-safe external edit handling.

### Tasks

1. Implement `PlanMutation` and `ConfirmMutation` boundaries.
2. Verify proposal ID, digest, base catalog snapshot and per-path before digests.
3. Validate virtual result before any canonical replacement.
4. Stage and fsync manifest/after-images.
5. Apply domain paths in deterministic order and receipt last.
6. Detect path state by before/after/unknown digest.
7. Implement default approved roll-forward recovery.
8. Implement explicit rollback only when all before-image conditions hold.
9. Persist/rebuild idempotency keys from operation receipts.
10. Return operation ID, changed paths, snapshots and Git dirty state.
11. Refuse normal writes while recovery is pending.

### Fault-injection matrix

Inject failure after:

- transaction directory creation;
- each staged file/fsync;
- manifest phase update;
- first/middle/last canonical rename;
- operation receipt write;
- canonical post-validation;
- journal cleanup.

### Exit gate

Every injected case recovers to exactly:

```text
old valid canonical state
or
intended new valid canonical state
```

No accepted mixed state, silent overwrite or duplicate retry is possible.

## Phase 4 — Rebuildable immutable SQLite catalog generations

### Goal

Build all query/search projections from canonical files and publish them atomically.

### Deliverables

- `skillhub rebuild`;
- `BuildCatalogGeneration` application service;
- immutable `runtime/catalog/generations/*.db`;
- atomic `runtime/catalog/current.json`;
- `runtime/operational.db`;
- `runtime/telemetry.db` placeholder;
- generation integrity/smoke checks;
- stale detector and old-generation GC policy.

### Tasks

1. Create immutable build-input scanner from Phase 2.
2. Build initial relational/FTS schema for:
   - skills/resources/routing metadata;
   - sources/revisions;
   - findings/comparisons/insights;
   - provenance/outcomes;
   - operation/idempotency lookup.
3. Insert all projections in one SQLite transaction.
4. Store schema versions, builder version, both digests and row counts.
5. Run integrity, FK and representative query checks.
6. Reacquire exclusive lock and recheck projection digest before publish.
7. Atomic-write generation pointer; never truncate live generation.
8. Let readers pin old generations; defer GC safely.
9. Recreate missing operational DB empty/default.
10. Never reconstruct telemetry.
11. Trigger same service from init, explicit rebuild, doctor fix and eligible startup.

### Tests

- delete all `runtime/` then rebuild offline;
- same canonical bytes on two OSes produce same logical digests/results;
- canonical changes during build prevent stale publish;
- corrupt new generation leaves previous pointer valid;
- source-only changes alter projection digest without altering catalog snapshot;
- operation receipts restore idempotency lookup without event replay.

### Exit gate

- No `.db` file is tracked by Git.
- Full rebuild is correctness oracle.
- SQLite bytes need not match, but logical rows/query results and digests do.
- Missing telemetry/operational history cannot change catalog behavior.

## Phase 5 — Curation Home, status and operational CLI

### Goal

Deliver the first useful UX slice before implementing all domain capabilities.

### Deliverables

- `GetCurationHome` read model;
- `skillhub status`, `--json`, `--quiet`;
- `workspace_validate`, `workspace_rebuild`, `workspace_diff` application/tool contracts;
- prioritized `ActionItem` rules;
- grouped Git/canonical diff summary;
- long-operation progress events.

### Tasks

1. Implement priority ordering from UX spec.
2. Keep `status` strictly local/offline.
3. Detect workspace invalid, recovery pending, stale index and Git dirty.
4. Show unsupported action categories as zero/not-configured without misleading errors.
5. Render one recommended next action.
6. Ensure human and JSON outputs share one read model.
7. Make rebuild progress visible and cancellable before publish where safe.

### Exit gate

- UX fixtures from Phase 0 pass for all currently implemented action types.
- Healthy home is one concise response.
- `status` performs zero network calls.

## Phase 6 — Curated skill lifecycle

### Goal

Support manual skill creation/editing safely before upstream learning automation.

### Deliverables

- create/edit/activate/deprecate/archive commands;
- preview/confirm proposal flow;
- skill metadata/resource validation;
- catalog snapshot changes and generation publish;
- routing-impact hook interface;
- external editor validate/rebuild flow.

### Tasks

1. Generate minimal draft from explicit fields; Agent can supply inferred draft content.
2. Keep draft inactive until required fields validate and activation is approved.
3. Implement direct edit as pinned proposal, not uncontrolled overwrite.
4. Return diff summary/full diff on demand.
5. Preserve provenance/history during deprecate/archive.
6. Implement snapshot-expired behavior for old resource manifests.
7. Add operation receipts for every managed mutation.

### Exit gate

- Create → preview → activate → resolve/read basic skill works.
- Stale edit proposal is rejected.
- Invalid external edit does not publish a new generation.
- Successful mutation reports active local state and uncommitted Git state.

## Phase 7 — Source intake, adapters and monitoring

### Goal

Capture and monitor upstream sources without changing curated skills or creating routine Git noise.

### Deliverables

- source candidate capture/list/triage;
- source catalog/link records;
- adapters for Git repository, filesystem and immutable/living documents as scoped;
- explicit network check command/tool;
- runtime check/scheduler state;
- due/changed/unavailable action items;
- source trust/license metadata and limits.

### Tasks

1. Implement capture with locator + reason only.
2. Detect identity/default branch/path/license where available.
3. Present one consolidated onboarding proposal.
4. Implement adapter contract: identify/current revision/diff/read/list.
5. Enforce protocol, size, timeout, traversal and credential policies.
6. Persist new meaningful revision/digest canonically.
7. Store unchanged check time/latency/retry/transient availability only in operational DB.
8. Ensure one source failure does not fail batch checks.
9. Add scheduler only to `serve`/explicit OS timer mode.
10. Never execute source scripts.

### Exit gate

- Repeated unchanged checks leave `git status` unchanged.
- New revision creates durable actionable work.
- `hub_status` itself performs no fetch.
- Clone/rebuild retains source revisions/decisions but may legitimately lose check timestamps.

## Phase 8 — Distillation and learning pipeline

### Goal

Transform pinned source revisions into durable findings/evidence/proposals without applying active skill changes.

### Deliverables

- prepare/start run;
- immutable source revision packages;
- findings/observations and stable identity;
- coverage ledger;
- tombstone/supersession;
- cross-source comparisons;
- insight proposal generation/validation;
- submit/auto-finalize;
- retry/cancel/AwaitingDecision recovery.

### Tasks

1. Pin source/from/to revisions and changed scope.
2. Require evidence reads from target revision; diff only scopes work.
3. Define stable finding identity and source vocabulary.
4. Validate evidence locators/digests.
5. Require every changed resource be analyzed/deferred/unreadable/out-of-scope with reason.
6. Generate tombstones for removed knowledge.
7. Build/update comparisons and staleness markers.
8. Keep Observation (“source says”) separate from Insight (“we should adopt”).
9. Auto-finalize valid submissions in one canonical WriteSet.
10. Advance cursor only with run/findings/coverage/insights atomically.
11. Enter AwaitingDecision only for blocking ambiguity/coverage.
12. Batch multiple sources while isolating failures.

### Exit gate

- Failed run never advances cursor.
- Finalized run always has valid artifacts and complete coverage classification.
- Normal successful run needs no user finalize action.
- Distillation never modifies active skill content.
- Findings are hidden from default UX but queryable on demand.

## Phase 9 — Insight inbox, apply and outcomes

### Goal

Turn source learning into reviewed semantic changes with full traceability.

### Deliverables

- grouped/ranked insight inbox;
- plan/reject/obsolete decisions;
- preview/apply proposal;
- source-to-local mappings;
- incorporation/outcome records;
- changed-upstream impact query;
- operation diff/undo guidance.

### Tasks

1. Rank by evidence, impact and staleness without auto-adopting.
2. Keep rejection rationale and prevent same-evidence reproposal.
3. Reopen only with materially new evidence/rationale.
4. Generate immutable ApplicationProposal with base/digest/path preconditions.
5. Validate affected skill/routing state before confirm.
6. Apply skill, Insight decision, incorporation mapping and receipt atomically.
7. Query local artifact → insight → finding → source revision.
8. Query changed finding → affected local artifacts.
9. Record outcome only from explicit evidence after meaningful use/review.
10. Provide operation-level diff and safe Git restore/revert guidance.

### Exit gate

- Changed base/path makes proposal stale and applies nothing.
- Apply updates all provenance/mapping records or none.
- Same evidence cannot silently reopen rejected Insight.
- Outcome is never inferred merely from apply/use.

## Phase 10 — Evidence-first resolver

### Goal

Resolve tasks against active curated skills with deterministic evidence and calibrated abstention.

### Deliverables

- request validation/normalization;
- FTS/rule candidate generation;
- requirement/exclusion tri-state logic;
- scoring and deterministic tie-breaking;
- `resolved`, `needs_context`, `no_skill` decisions;
- clarification questions;
- supporting-skill selection;
- explanation/reason codes;
- cache keyed by catalog/fact/activation context.

### Tasks

1. Implement rich structured request contract without requiring catalog taxonomy.
2. Separate task evidence from ambient repository facts.
3. Enforce hard incompatibility before score.
4. Add curated trigger/not-for/relationship signals.
5. Implement bounded clarification that can change decision.
6. Support canonical equivalent/near-duplicate policy.
7. Return one primary recommendation in normal path.
8. Keep vector retrieval and LLM rerank behind disabled interfaces.
9. Build initial golden corpus of roughly 150 cases/30–50 skills.
10. Calibrate thresholds from held-out cases, not anecdotes.

### Exit gate

- Golden evaluation meets agreed precision/abstention/ambiguity gates.
- No-skill and negation cases are explicitly covered.
- Same request + snapshot + policy + facts produces same deterministic result.
- Resolver never activates a skill.

## Phase 11 — MCP protocol and skill distribution

### Goal

Expose resolver, curation and progressive resource loading through MCP stdio.

### Deliverables

- MCP server lifecycle;
- `skill_resolve` and `skill_feedback`;
- standards-compatible skill manifest/list/get/resources;
- curation tools from the UX mapping;
- pagination and structured errors;
- snapshot/resource digest pinning;
- client capability negotiation where available.

### Tasks

1. Map MCP handlers directly to application commands/read models.
2. Implement request/response JSON Schemas and versioning.
3. Keep recommendation separate from distribution.
4. Return manifest before content and resources only on demand.
5. Ensure resolved snapshot and loaded digests match.
6. Return `snapshot_expired` instead of silently serving changed resource.
7. Bound payload sizes and paginate findings/diffs.
8. Add protocol conformance and malformed-input tests.
9. Test multiple stdio processes against one workspace lock/generation model.

### Exit gate

- CLI and MCP produce semantically equivalent results for shared commands.
- No MCP handler writes canonical files directly.
- Resolve → get → read preserves identity/version/digests.
- Stock-client compatibility matrix has no undocumented assumptions.

## Phase 12 — Bundled System Curator Skill and host bootstrap

### Goal

Make the UX usable through ordinary Agent Hosts without requiring command memorization.

### Deliverables

- embedded/versioned System Curator Skill;
- Curation Home dialogue;
- progressive evidence/diff disclosure;
- batch maintenance orchestration;
- bootstrap managed instruction block;
- MCP client registration remediation;
- full `doctor --fix` dependency-ordered plan.

### Tasks

1. Encode natural-language intents and tool selection guidance.
2. Enforce “one primary question” and approval matrix in skill instructions/fixtures.
3. Translate internal terminology to user-facing wording.
4. Orchestrate check → distill → inbox without applying active content.
5. Present interrupted work before optional work.
6. Bundle skill/tool compatibility metadata into binary.
7. Implement host-specific MCP registration adapters where stable.
8. Insert/update idempotent managed bootstrap blocks.
9. Clearly label instruction-only activation coordination as best effort.
10. Ensure CLI remains independent recovery path if MCP/Agent fails.

### Exit gate

- “Curate my Skill Hub” works end-to-end in each supported stock client.
- Normal user does not navigate entities/states manually.
- System skill never bypasses binary validation/mutation services.
- Re-running doctor fix does not duplicate registrations/instructions.

## Phase 13 — Telemetry, reproducibility and evaluation

### Goal

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

## Phase 14 — Hardening, migrations and V1 release

### Goal

Turn the integrated prototype into a supportable local product.

### Deliverables

- canonical migration framework;
- derived schema rebuild policy;
- backup/recovery documentation;
- install/upgrade/uninstall scripts;
- signed release artifacts/checksums;
- security review and resource limits;
- performance budgets and compatibility matrix;
- V1 operational runbook.

### Tasks

1. Implement explicit canonical migration preview/confirm/receipt flow.
2. Rebuild automatically for derived-only schema changes when safe.
3. Test binary upgrade never silently mutates canonical files.
4. Add source/network/file size/time limits.
5. Fuzz parsers, path handling and MCP inputs.
6. Run race/deadlock tests with multiple stdio processes.
7. Benchmark startup, rebuild, resolve p95/p99 and large workspace limits.
8. Test disk-full/read-only/permission/clock-skew scenarios.
9. Verify install paths, primary workspace discovery and host remediation.
10. Document Git clone → rebuild → serve disaster recovery.
11. Run complete fault-injection suite on every release platform.
12. Freeze V1 canonical/tool schemas only after compatibility/eval gates.

### Exit gate

- `curl | sh` installs one verified binary.
- Clone + `skillhub rebuild` restores equivalent behavior offline.
- Every managed mutation has tested crash recovery.
- `doctor` explains all invalid/stale/recovery states.
- No critical/high security finding remains.
- Release candidate passes stock-client and cross-platform matrix.

## Phase 15 — Post-core capabilities, evidence-gated

Only start after V1 usage/evaluation demonstrates need.

### 15.1 Deep-dive mode

- targeted source area expansion;
- explicit budget/coverage;
- same finding/insight/mutation invariants.

### 15.2 Consult mode

- compare multiple sources for a curator question;
- pin all source revisions;
- create Comparison/Insight proposals, never direct active edits.

### 15.3 Additional source adapters

- non-Git APIs/living docs;
- opaque revisions/content digests;
- adapter-specific trust and replay limitations.

### 15.4 Vector retrieval

- only after FTS/rule error analysis;
- derived/disposable index;
- ablation proves quality gain worth complexity.

### 15.5 LLM fallback

- only for unresolved ambiguity;
- strict structured output;
- deterministic fallback behavior;
- privacy/cost budgets and evaluation.

### 15.6 Remote/multi-tenant mode

Requires separate design for authn/authz, tenancy, encryption, remote canonical storage and concurrency. Do not extrapolate local filesystem assumptions.

## 5. Cross-phase quality gates

Every phase must include:

### Contract tests

- human output and `--json` derive from same result;
- CLI/MCP map to same application service;
- error code and next action are stable.

### Storage tests

- canonical mutation only through mutation service;
- no business authority in runtime DB;
- no network during rebuild/status;
- delete-runtime test remains green.

### Security tests

- untrusted source/skill content treated as data;
- no script execution during scan/distill;
- path/symlink containment;
- secrets/absolute paths absent from receipts/telemetry.

### UX tests

- one recommended action;
- no unnecessary confirmation;
- progressive disclosure;
- active/uncommitted status after mutation;
- recovery language uses `ERROR / WHY / FIX`.

### Reproducibility tests

- clock/IDs injectable;
- request + snapshots + policy pin sufficient for replay;
- derived generation metadata explains builder/schema/input versions.

## 6. Required end-to-end scenarios

Before V1 release, automate these scenarios:

1. Fresh install → init → doctor → healthy status.
2. Clone canonical repo → delete runtime → rebuild → equivalent resolver behavior.
3. Create draft skill → preview → activate → resolve → load resource.
4. External valid edit → stale detection → rebuild → new snapshot.
5. External invalid edit → old generation preserved + actionable error.
6. Capture source → onboard → unchanged check leaves Git clean.
7. Detect source revision → batch distill → findings/insights, active skill unchanged.
8. Crash during cursor/finding write → doctor roll-forward → atomic finalized run.
9. Review Insight → preview → concurrent edit → stale apply rejected.
10. Regenerate proposal → approve → skill/provenance/receipt all change atomically.
11. Agent Host loads recommended skill with pinned digest.
12. Two Agent Hosts race a mutation → one succeeds, one conflicts/idempotently observes result.
13. Corrupt derived DB → rebuild while canonical remains untouched.
14. Telemetry DB deletion → behavior unchanged.
15. Git checkout to older valid revision → rebuild → matching historical behavior.

## 7. Release sequencing

Suggested usable increments:

### Developer Preview A

Phases 0–5:

```text
install/init/validate/rebuild/status
+ canonical mutation/recovery substrate
```

### Curator Preview B

Phases 6–9:

```text
skill lifecycle
+ source maintenance
+ distillation
+ insight apply
```

### Agent Preview C

Phases 10–12:

```text
resolver
+ MCP distribution
+ System Curator Skill
+ stock-client bootstrap
```

### V1 Release Candidate

Phases 13–14:

```text
telemetry/evaluation
+ migrations/security/performance
+ release/upgrade/recovery validation
```

A preview must not weaken storage invariants merely because later UX/features are absent.

## 8. Critical risks and mitigations

| Risk | Mitigation / phase gate |
|---|---|
| UX drifts into entity CRUD | Phase 0 fixtures and adapter parity reviews |
| Git working tree left mixed after crash | Phase 3 WAL + exhaustive fault injection |
| Rebuild publishes stale inputs | Phase 4/07 digest recheck under exclusive lock |
| Multi-process stdio races | OS advisory locks + idempotency + concurrency tests |
| SQLite becomes accidental authority | Delete-runtime CI test and dependency review |
| Hot aggregate YAML conflicts | One file per independently mutable stable ID |
| Scheduler dirties Git weekly | Runtime-only unchanged check state |
| Distill silently omits source scope | Mandatory coverage ledger |
| Insight apply TOCTOU | Proposal ID + digest + base/path preconditions |
| Resolver over-activates | Abstention/negation golden cases and calibration |
| Stock clients lack activation registry | Explicit bootstrap contract, best-effort wording, compatibility matrix |
| Telemetry captures sensitive content | Field allowlist, content-none default, bounded retention |
| Schema frozen too early | Freeze only after UX/client/evaluation gates |

## 9. Definition of V1 done

V1 is done only when:

- a non-expert can start with “Curate my Skill Hub” and follow suggested actions;
- CLI provides equivalent recovery/automation semantics;
- canonical repository alone restores durable behavior;
- no database/event export is required for restore;
- every managed mutation is idempotent, conflict-checked and crash-recoverable;
- normal unchanged checks do not dirty Git;
- distillation never auto-edits active skills;
- semantic apply always uses pinned preview + explicit approval;
- resolver can abstain and explain recommendation evidence;
- resolve/load uses one consistent catalog snapshot/digest set;
- telemetry can be deleted without behavioral loss;
- install/upgrade/rebuild work on all supported release targets;
- documented V1 acceptance, evaluation and security gates pass.
