# Learnings from forgentX `distill`

**Source project:** `/home/vantt/projects/forgentX`  
**Source commit:** `222110373f9f45e32964004625fd199dcdc11ee0`  
**Source path:** `core/skills/distill/`  
**Copied reference:** [`distill/`](distill/)  
**Reviewed artifacts:** `SKILL.md`, `CREATION-LOG.md`, all `references/`, `scripts/distill.mjs`, migration script  
**Review status:** Complete for the copied skill directory

## Decision vocabulary

| Decision | Meaning |
|---|---|
| `adopt` | Apply directly to Skill Hub design |
| `adapt` | Preserve the lesson with a different implementation |
| `planned` | Valuable, but scheduled after core V1 |
| `already_covered` | Existing design already expresses the lesson |
| `rejected` | Do not adopt; rationale required |
| `superseded` | Covered by a broader accepted lesson |

## Incorporation matrix

| ID | Learning | Decision | Target | Incorporation |
|---|---|---|---|---|
| DST-001 | Separate Observe → Compare → Decide | adopt | `06-source-learning-and-distillation.md` | incorporated |
| DST-002 | Source Observation is distinct from curated Insight | adopt | `06`, `05`, `01` | incorporated |
| DST-003 | Capture source candidates before requiring full onboarding | adopt | `05`, `06` | incorporated |
| DST-004 | Human triages source intake | adopt | `05`, `06` | incorporated |
| DST-005 | Incremental per-source cursor avoids re-analysis | already_covered | `06` clarification | incorporated |
| DST-006 | Advance cursor only after complete validated analysis | adapt | Binary atomically finalizes run | incorporated |
| DST-007 | Source types need different cursor semantics | adapt | Source Adapter model in `06` | incorporated |
| DST-008 | Diff locates change; current content proves current truth | adopt | Distiller hard gate in `06` | incorporated |
| DST-009 | Stable observation/insight IDs must survive wording changes | adopt | `06` identity rules | incorporated |
| DST-010 | Removed/moved/superseded knowledge uses tombstones | adopt | `06`, `05` lifecycle | incorporated |
| DST-011 | Rejection requires durable rationale | adopt | `05`, `06` | incorporated |
| DST-012 | Reopen only for materially new evidence | adopt | Insight lifecycle | incorporated |
| DST-013 | Coverage ledger prevents silent omission | adapt | Per-run scoped coverage in `06` | incorporated |
| DST-014 | Global taxonomy backfill can guarantee recall | adapt | Scoped backfill/coverage, not global V1 invariant | incorporated |
| DST-015 | Cross-source comparison exposes convergence/trade-offs | adopt | Comparison layer in `06` | incorporated |
| DST-016 | Topic deep-dive reuses paid indexes before source reads | planned | `06` optional mode | designed |
| DST-017 | Consult mode supports unknown vocabulary via domain definitions | planned | `06` optional mode | designed |
| DST-018 | Evidence pyramid should descend only when necessary | adopt | Progressive evidence protocol in `06` | incorporated |
| DST-019 | Do not rescan a complete source for a deep-dive | adopt | `06` cost controls | incorporated |
| DST-020 | Mechanical inventory and semantic judgment are separate | adopt | Binary/system-skill responsibility split | incorporated |
| DST-021 | Preserve source vocabulary as keywords | adopt | Observation schema | incorporated |
| DST-022 | Evidence paths/anchors must resolve | adopt | Validator rules | incorporated |
| DST-023 | Derived synthesis pins source cursors and becomes stale | adopt | Staleness model | incorporated |
| DST-024 | Source ↔ local mapping must work in both directions | adopt | Incorporation mapping | incorporated |
| DST-025 | Record actual outcome after incorporation | adopt | `05`, `06`, `04` | incorporated |
| DST-026 | R×E/F can help human triage | adapt | Optional, explainable priority hint only | incorporated |
| DST-027 | Status is local/offline; network check is explicit | adopt | `05`, `06`, `01` | incorporated |
| DST-028 | No-argument surface presents dashboard and next action | adopt | `05`, system curator behavior | incorporated |
| DST-029 | Machine-readable status output supports automation | adopt | MCP tools/CLI contract | incorporated |
| DST-030 | Headless execution persists ambiguity instead of blocking | adapt | Outstanding Decisions queue | incorporated |
| DST-031 | Init/fix must be idempotent and preserve user-owned bytes | already_covered | `doctor --fix`; managed block rule clarified | incorporated |
| DST-032 | Errors should state ERROR / WHY / FIX | adopt | CLI/MCP diagnostics | incorporated |
| DST-033 | Integrity check validates cursors, evidence and staleness | adopt | `doctor`/`validate` in `06` | incorporated |
| DST-034 | Scale only after a measured threshold | adopt | Split/index escalation guards | incorporated |
| DST-035 | Do not add search infrastructure without demonstrated need | adapt | FTS retained for product routing; vector optional | incorporated |
| DST-036 | Human owns adoption decisions | already_covered | Approval boundary | incorporated |
| DST-037 | Semantic proposal must not implement itself | already_covered | Distiller/applier split | incorporated |
| DST-038 | Mandatory manual seal | rejected as UX | Invariant adopted as binary auto-finalize | incorporated |
| DST-039 | Runtime state layer can become authority then sync Markdown | rejected | Violates canonical-files authority | incorporated |
| DST-040 | Force every source to backfill every taxonomy domain | rejected for V1 | Too costly at intended scale | incorporated |
| DST-041 | Hidden clone pull during an informational command | rejected | Network/mutation must be explicit | incorporated |

## Adopted conceptual pipeline

```mermaid
flowchart LR
    CAP[Capture source candidate] --> TRIAGE[Human source triage]
    TRIAGE --> CHECK[Check source revision]
    CHECK --> RUN[Prepare distill run]
    RUN --> OBS[Extract observations]
    OBS --> COVER[Record coverage]
    COVER --> COMP[Compare across sources]
    COMP --> INS[Propose insights]
    INS --> FINAL[Finalize run and cursor]
    FINAL --> DECIDE[Human insight triage]
    DECIDE --> PATCH[Generate patch proposal]
    PATCH --> APPLY[Human-approved apply]
    APPLY --> OUTCOME[Record later outcome]
```

## Key distinctions

```text
What a source contains
≠ What is worth learning
≠ What should be incorporated
≠ Whether incorporation helped in practice
```

These distinctions map to:

```text
Observation
→ Comparison
→ Insight
→ Incorporation
→ Outcome
```

## Explicit non-adoptions

### Manual sealing

The lesson is atomic cursor advancement, not a user-visible `seal` command. Skill Hub finalizes revision/cursor only after observations, coverage and run artifacts validate.

### Dual durable authority

No runtime store may become authoritative for insight/incorporation status and later sync backward into Markdown/YAML. Runtime indexes are rebuilt from canonical files.

### Global taxonomy backfill

Skill Hub keeps per-run coverage and targeted backfill. It does not require every source to cover every global taxonomy domain before serving at V1 scale.

### Automatic priority authority

Explainable scoring can order a human inbox, but cannot approve/reject/apply insights.

## Follow-up evidence

The copied skill reports limited pressure testing and little real-world validation for paper/living-doc and consult flows. Skill Hub should treat those patterns as hypotheses until its own evaluation corpus and dogfooding provide evidence.
