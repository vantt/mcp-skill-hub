---
phase: 2
title: "Catalog continuity and truthful status"
status: completed
priority: P0
effort: "3-4d"
dependencies: []
---

# Phase 2: Catalog continuity and truthful status

## Context Links

- [BUG-07, BUG-08, BUG-13](./reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)
- [Resolver design](../../docs/design/03-resolver-design.md)
- [Storage authority hierarchy](../../docs/design/07-storage-and-mutation-model.md#3-authority-hierarchy)

## Objective

Keep the last verified catalog generation available when current canonical edits are invalid, expose the degraded state explicitly, preserve isolated servability checks, and prevent status from translating unknown counts into an empty workspace.

## Fixed Contracts

- Canonical files remain authoritative. A last-valid generation is an availability fallback, never a claim that current files are valid.
- Invalid canonical changes block rebuild and managed mutation. Catalog metadata, counts, and unchanged resources may use the atomically pinned published generation with degraded diagnostics.
- The catalog stores digests, not historical resource bytes. A changed/deleted resource is unavailable and excluded from distribution/resolution; its old bytes are never guessed. `skill review` may inspect the current broken canonical file, while `show/get` returns `resource_content_unavailable`.
- Fallback never chooses an arbitrary retained generation; it uses the verified published current pointer only.
- Exact `OpenSnapshot` semantics remain unchanged.
- Extend the existing `catalog.Status` with one serving mode (`current`, `fallback`, `unavailable`) rather than creating a second health enum. Health, serving mode, pointer, generation, and warning come from one locked observation.
- Resolver output pins the generation it actually used and recommends only skills whose live resources still match that generation.
- Public count integers remain schema-compatible. Existing workspace/index health plus internal `CountsKnown` controls human rendering; “No skills yet” appears only when counts are known.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/catalog/open.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/open_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/catalog.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/servable.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/servable_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/generation_lookup.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/catalog.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/catalog_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/resolver.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/resolver_continuity_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/curation_home.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/curation_home_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/distribution.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/distribution_test.go`


No other phase edits these files.

## Implementation Steps

1. Extend `catalog.Status` with serving mode and bounded diagnostics; do not duplicate missing/stale/corrupt/incompatible health states.
2. Define one lock/retry state machine: inspect canonical state and pin the current pointer under a shared lock; if rebuild is needed, release, rebuild under publication locking, then restart observation. Availability metadata and the opened generation MUST come from the same observation.
3. Keep `EnsureFreshOrRebuild` strict for callers requiring current publication. Add a deliberate read-open path that falls back only to the verified published pointer when canonical validation fails.
4. Verify pointer, SQLite integrity, generation identity, and each requested live resource digest. Corrupt/incompatible generations remain unavailable; changed/missing resources are isolated and reported, never served from guessed bytes.
5. Migrate resolver and distribution list/lookup/get paths to the deliberate read-open API. Resolver excludes changed-resource skills while unchanged skills remain available; standard MCP skill methods receive skipped/error diagnostics through their existing report/log/error surfaces.
6. Expose one state projection with explicit basis: current canonical facts, served/published generation facts, and whether each is known. Phase 3 consumes it for review and mutation results.
7. Keep public curation-home JSON integers unchanged. Populate internal count-knownness from the pinned catalog observation; invalid/unavailable state prioritizes repair guidance and never renders zero as empty.
8. Preserve active-skill isolation for structurally valid but unservable skills.

## Todo

- [x] Extend existing catalog status with atomic serving-mode/pointer pinning.
- [x] Preserve strict rebuild while adding verified resource-aware fallback reads.
- [x] Migrate resolver and distribution consumers.
- [x] Publish basis-aware servable/routing-state assessment.
- [x] Correct unknown-count semantics without changing the v1 JSON shape.
- [x] Add canonical-state race, changed-resource, corruption, and isolation tests.

## Success Criteria

- BUG-07: an unrelated invalid canonical edit does not disable unchanged valid skills; fallback is identified. A skill whose own resources changed is excluded and returns `resource_content_unavailable`.
- BUG-08: invalid workspace status never says “No skills yet” unless a known catalog count is actually zero.
- Mutations and rebuilds still fail on invalid canonical files.
- Corrupt/incompatible generations are never served, and fallback warning/generation cannot race into incoherent combinations.
- Resolver/distribution results remain snapshot-pinned and resource-digest verified.

## Verification

```bash
go test ./internal/catalog -run 'Test.*(Open|Status|Fallback|Servable|Integrity|Race)'
go test ./internal/app -run 'Test.*(Catalog|ResolverContinuity|Distribution|CurationHome)'
```

Smoke twice: break an unrelated canonical file and resolve/show an unchanged skill with a degraded warning; then break that skill's entrypoint and prove it is excluded, `show/get` reports content unavailable, review remains usable, and rebuild/mutation stay blocked.

## Risks and Security

- Fallback can hide that local edits are broken. Every fallback result must carry a machine-readable degraded state and human/server diagnostic.
- Never bypass resource digest verification or recommend a skill whose required live bytes differ from the pinned generation.
- Do not add historical content storage, pointer-GC changes, or a parallel health enum in this phase.

## Next Step

Phase 3 uses availability/servability facts for review and mutation results. Phase 6 renders truthful status.