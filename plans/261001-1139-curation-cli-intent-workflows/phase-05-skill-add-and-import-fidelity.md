---
phase: 5
title: "Skill add and import fidelity"
status: completed
priority: P0
effort: "4-5d"
dependencies: [1, 3, 4]
---

# Phase 5: Skill add and import fidelity

## Context Links

- [BUG-04, BUG-11, BUG-16](../reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)
- [Accepted add semantics](../reports/curation-ux-cli-independent-evaluation.md#5-input-resolution-and-normalization)
- [Existing curation guide import flow](../../docs/curating-skills.md#import-existing-skills-as-drafts)

## Objective

Implement atomic one-locator draft adoption without creating a watcher, preserve every safe companion resource, retain bounded provenance, and repair the existing watched-source import path without changing its advanced compatibility behavior.

## Fixed Contracts

- `PreviewSkillAdd`/`ConfirmSkillAdd` are separate from source capture/triage/import. They create skill files plus one operation receipt, never a source candidate, source record, or source-skill link.
- One discovered skill previews directly. Zero returns `no_skills`; multiple returns `selection_required` unless `--skill` or `--all` is explicit. `--yes` never resolves ambiguity.
- Default collection is `default`; imported state is draft; no activation/watch side effect.
- Existing target ID is `id_conflict`; explicit `--id` may rename a single selected skill. No overwrite and no silent suffix.
- Exact replay lookup precedes target-ID conflict detection. Same normalized origin, folder digest, target ID, selection, collection, transformations, and request digest returns the prior receipt even after disposable proposal/cache loss; a changed payload conflicts.
- Every bounded regular companion beneath the selected folder, including empty/binary/YAML files, is copied byte-for-byte and listed in preview. Unsafe/over-limit inputs fail the selected skill; nothing is silently dropped.
- Unknown/proprietary license is a warning, not a blocker.
- Direct-add proposals use Phase 3's kind-tagged skill proposal envelope; this phase creates no competing proposal namespace/store.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/app/source_import.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/source_import_test.go`

Create:

- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_add.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_add_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_discovery.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/skill_discovery_test.go`

No other phase edits these files.

## Implementation Steps

1. Extract pure skill discovery/inventory/ID/frontmatter helpers from source import into a shared app file owned by this phase. Preserve current watched-source selectors and conflict-skip behavior for legacy `source import`.
2. Fix folder-scoped discovery so the selected skill root is explicit and every descendant regular file is inventoried. Account for files/count/bytes before mutation planning; include failed reasons in bounded preview output.
3. Implement `SkillAddInput`, proposal display metadata, and result types. Inputs include normalized locator/snapshot, selection, optional target ID, collection, idempotency key, and full-diff choice.
4. Resolve/capture through Phase 4, discard transient host authority, then persist exact bytes/pins/write set through Phase 3's `skill_add` proposal kind. Confirmation never rereads upstream or requires the original path.
5. Derive deterministic idempotency key/request digest and call operation-receipt lookup before examining current target files. Only a non-replay then performs ID conflict detection.
6. Write `SKILL.md`, `skill.meta.yaml`, and companions in one context-aware mutation. Local origin contains only kind, snapshot/content digests, transformations, added time, and selected relative path. Create no source/intake/link files.
7. Preserve imported frontmatter body and supported fields while normalizing local identity. Reject malformed `SKILL.md`; do not silently rewrite content beyond previewed transformations.
8. Detect license declarations from supported frontmatter and common license companions; show facts/warnings without legal inference.
9. Return Phase 3's basis-aware state facts, changed paths, origin, and original/replayed operation receipt.
10. Add regressions for legacy source import companions, source provenance links, and conflict-skipping.

## Todo

- [x] Extract shared deterministic skill discovery.
- [x] Fix folder-root companion handling in legacy import.
- [x] Use the shared kind-tagged skill proposal envelope and exact confirmation.
- [x] Look up replay receipts before ID conflicts.
- [x] Write privacy-safe direct-add origin without source records/links.
- [x] Enforce explicit selection/conflict semantics and complete resource inventory.
- [x] Surface license/resource inventory and transformations.
- [x] Add local/Git/cache-loss/idempotency/stale-proposal regressions.

## Success Criteria

- BUG-04: folder-scoped import preserves `LICENSE.txt`, top-level Markdown/YAML, empty files, nested resources, and arbitrary bounded regular bytes.
- BUG-11: local folder add works from outside the workspace using immutable captured bytes without persisting the source path.
- BUG-16: declared license and license-file presence appear in preview; unknown remains an explicit warning.
- Direct add creates only the draft skill and operation receipt; source intake/catalog/link counts are unchanged.
- Editing/deleting the original after preview does not change confirmed bytes. Deleting proposal/cache after a successful lost response and rerunning the identical add returns the original receipt.
- A selected-skill conflict applies nothing and returns exact `--id` guidance; changed idempotency payload conflicts.

## Verification

```bash
go test ./internal/app -run 'Test.*(SkillAdd|SkillDiscovery|SourceImport|Companion|License|Idempotent)'
go test -race ./internal/app -run 'Test.*(SkillAdd|SourceImport)'
```

Smoke: add a disposable local skill containing top-level and nested companions, mutate the original after preview, confirm, then compare the canonical tree against the preview inventory/digests.

## Risks and Security

- Scripts and arbitrary companion bytes are data only. Never execute, source, chmod executable, deserialize as canonical entities, or inspect them through a shell.
- Imported bytes live only in the bounded shared proposal/snapshot stores with restrictive permissions and expiry; serialized artifacts must not contain the original local root.
- Maintain separate conflict policies: direct add fails a selected non-replay conflict; legacy bulk source import may continue reporting skipped conflicts.

## Next Step

Phases 6 and 7 expose the application workflow. Phase 8 documents beginner vs advanced paths.