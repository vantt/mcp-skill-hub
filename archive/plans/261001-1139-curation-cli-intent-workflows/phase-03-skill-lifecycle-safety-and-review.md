---
phase: 3
title: "Skill lifecycle safety and review"
status: completed
priority: P0
effort: "4-5d"
dependencies: [1, 2]
---

# Phase 3: Skill lifecycle safety and review

## Context Links

- [BUG-02, BUG-03, BUG-09, BUG-12, BUG-13](./reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)
- [Independent edit/review contract](./reports/curation-ux-cli-independent-evaluation.md#7-editreviewgovernance-contract)
- [Curation lifecycle design](../../../docs/design/05-curation-lifecycle.md)

## Objective

Make read-based skill edits optimistic-concurrency safe, prevent untouched scaffolds from activation, expose explicit publication/lifecycle/servability/routing facts, and implement a catalog-independent read-only review service.

## Fixed Contracts

- Editable reads return content and `sha256` digest from the same bytes. Read-based replacements carry `expected_content_digest`; mismatch returns `edit_conflict` before proposal persistence.
- `--editor` and MCP read-modify-write flows MUST provide the digest. Existing blind content replacement without a digest remains explicitly last-writer-wins for compatibility; description/routing-only intent does not require a prior content read even if generated frontmatter changes.
- No lock is held while an external editor is open. Existing mutation `BeforeDigest` remains the confirm-time guard.
- Untouched generated scaffold content cannot activate, even if routing fields are complete. The guard lives in the skill domain path and cannot be bypassed by CLI/MCP.
- State facts name their basis: current canonical lifecycle/readiness versus served published generation/servability/routing. Legacy `active_locally` remains a deprecated compatibility alias for current canonical `lifecycle_state == active`; no new code treats it as publication or routing truth.
- Review is read-only, local/offline, tolerant of catalog failure, and reports fact/unknown/warning separately. It does not assert semantic approval.
- One versioned, kind-tagged skill proposal envelope owns lifecycle and direct-add proposals. It stores exact pins/write set, expiry, bounded display metadata, and optional editor-recovery identifier; proposal-ID-only CLI confirmation dispatches by persisted kind.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/skill/lifecycle.go`
- `/home/vantt/projects/mcp-skill-hub/internal/skill/proposal_store.go`
- `/home/vantt/projects/mcp-skill-hub/internal/skill/proposal_store_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/skill/resource.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_lifecycle.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_lifecycle_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_list.go`

Create:

- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_review.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_review_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/skill/lifecycle_test.go`

No other phase edits these files.

## Implementation Steps

1. Add an editable-content result containing canonical bytes and digest. Add `ExpectedContentDigest` to `skill.UpdateInput`; when supplied, set/check the entrypoint `BeforeDigest` under mutation planning's lock. Keep omitted-digest blind replacements backward compatible and explicit.
2. Return typed `edit_conflict` with bounded expected/current digest identifiers and an exact recovery action. A failed precondition creates no proposal and changes no files.
3. Evolve the proposal artifact compatibly into a strict kind-tagged envelope for lifecycle and direct-add operations. Read legacy lifecycle artifacts, reject unknown kinds/fields, preserve exact mutation pins, and provide a dispatcher consumed by Phase 6.
4. Associate editor recovery artifacts by proposal ID without storing absolute source paths. Use the proposal TTL/cleanup boundary and expose an opaque recovery ID to application/JSON callers.
5. Replace ambiguous generated prose with a deterministic scaffold marker or exact generated-template predicate. Reject activation until user content replaces the scaffold. Existing meaningful skills without the marker remain unaffected.
6. Define a basis-aware skill-state projection using Phase 2 facts. Report current canonical lifecycle/readiness separately from served generation publication/servability/routing, including divergence/unknown state.
7. Implement `ReviewSkill`: read canonical skill files directly; collect canonical issues, activation readiness, resource status, served-generation differences, provenance/origin, Git staged/unstaged summaries from Phase 1, and deterministic next action. Do not rebuild catalog, fetch network content, or mutate state.
8. If current bytes no longer match the served generation, review displays both bases; `show/get` returns typed resource-content-unavailable rather than historical bytes.

## Todo

- [x] Add digest-pinned editable reads and explicit blind-replacement compatibility.
- [x] Add a backward-compatible kind-tagged skill proposal envelope/dispatcher.
- [x] Associate bounded editor recovery IDs with persisted proposals.
- [x] Add a domain-level untouched-scaffold activation guard.
- [x] Add basis-aware current/served state facts and correct the deprecated active alias.
- [x] Implement catalog-independent `ReviewSkill`.
- [x] Cover concurrent writes, proposal-kind dispatch, and broken-catalog review.

## Success Criteria

- BUG-02: writer A cannot overwrite writer B when A supplies the digest from its earlier read; B's bytes remain intact and no A proposal is persisted.
- BUG-09: untouched scaffolds fail activation with one exact edit action; edited meaningful content can activate.
- BUG-13: a draft reports current `lifecycle_state=draft`, current `routing_eligible=false`, and deprecated `active_locally=false`; fallback reports served facts separately.
- Review identifies current validity/origin/Git/readiness and served generation/servability/routing while making zero canonical/runtime mutations.
- Lifecycle and add proposals can be confirmed across processes by kind; legacy proposal artifacts and explicit-pin confirmation remain compatible.
- Changed-resource `show/get` never guesses historical bytes.

## Verification

```bash
go test ./internal/skill -run 'Test.*(Update|Conflict|Transition|Scaffold|Proposal|Dispatch|Resource)'
go test ./internal/app -run 'Test.*Skill(Lifecycle|Review|List)'
go test -race ./internal/skill ./internal/app
```

Smoke with two processes: hold a digest-pinned editor update, apply another managed update, release the first, and verify deterministic `edit_conflict`; then load a persisted lifecycle and add proposal by ID and verify kind dispatch.

## Risks and Security

- Hash comparison must cover the exact canonical bytes used to seed read-modify-write callers. Blind replacement remains possible only as an explicit compatibility path.
- Proposal/recovery metadata must not leak absolute local paths or full resource contents in diagnostics.
- Current canonical and served-generation facts must never collapse into one unqualified boolean.
- Scaffold detection must be deterministic and narrow; do not reject legitimate short content heuristically.

## Next Step

Phases 5–7 consume the proposal dispatcher and basis-aware state contract.