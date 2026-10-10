# Phase 2: .meta Rule

## Goal
Establish the `.meta` boundary rule across the entire codebase. Introduce `IsHubMeta(relPath)` to replace every exact-name special case of `skill.meta.yaml`. Exclude hub metadata from content digests, distributions, indexing, servable sizes, and upstream comparisons. Strip incoming `.meta/` from upstreams with `upstream_meta_ignored` warnings so upstreams cannot self-approve. Exclude `.meta/distill.yaml` from catalog snapshots while keeping `.meta/skill.yaml` included. Bump the canonical schema version from 1 to 2 with a registered migration step.

## Context & Constraints
- Plan decisions: D4, D5, D6, D10; Red Team findings S1, S3, S5, S7.
- Do NOT move `skill.meta.yaml` to `.meta/skill.yaml` yet (that belongs to Phase 4).
- Never remove telemetry event types or change `EventVersion`.
- Bump canonical workspace schema version (`workspace.SchemaVersion = "2"`) with a registered migration step so older binaries refuse the workspace instead of misjudging trust.

## Files to Modify
- `internal/workspace/workspace.go`:
  - Define `IsHubMeta(relPath string) bool` (top-level `skill.meta.yaml`/`.yml` or first path segment `.meta`, supporting skill-relative and workspace paths).
  - Bump `SchemaVersion = "2"`.
- `internal/skillruntime/trust.go`:
  - Use `IsHubMeta` in `withoutMeta`.
  - Re-export `IsHubMeta`.
- `internal/skillruntime/hints.go`:
  - Replace `file.Path == "skill.meta.yaml"` with `IsHubMeta(file.Path)`.
- `internal/app/distribution.go`:
  - Replace exact name in `distributedRelativePath` with `IsHubMeta(relative)`.
- `internal/app/skill_discovery.go`:
  - In `buildDiscoveredItem`, if companion path satisfies `IsHubMeta(relPath)`, ignore it and record an `upstream_meta_ignored` warning.
- `internal/app/source_import.go` & `internal/app/skill_add.go`:
  - Propagate `upstream_meta_ignored` warnings when discovered items contain ignored meta files.
- `internal/app/upstream_update.go`:
  - In `readLocalSkillFiles`, ignore `IsHubMeta(cleanRel)`.
  - In `validateAndFilterUpstreamPaths`, check `IsHubMeta(p)` and emit `upstream_meta_ignored`.
- `internal/app/upstream_origin.go`:
  - In `filesDigestOf`, exclude `IsHubMeta(clean)`.
- `internal/app/skill_review_changes.go`:
  - Replace exact manifest name in `gitTreeBlobs` and `currentBlobs` with `!IsHubMeta(relative)` so `.meta/` is excluded from blob sets.
- `internal/app/skill_review*.go`, `internal/app/operations.go`, `internal/app/insight.go`, `internal/app/source_backfill.go`:
  - Audit and replace any exact `skill.meta.yaml` filtering or checks with `IsHubMeta`.
- `internal/catalog/project.go`:
  - Exclude `IsHubMeta(file.Path)` from `resources` insertion and `resource_fts`.
- `internal/catalog/build.go`:
  - Exclude `IsHubMeta(file.Path)` from searchable resource check.
- `internal/catalog/servable.go`:
  - Replace `file.Path == directory+"/skill.meta.yaml"` with `IsHubMeta(file.Path)`.
- `internal/catalog/input.go`:
  - In `catalogAffecting`: return false for `.meta/distill.yaml`.
  - In `classifyEntity` & `isCanonicalEntityPath`: recognize `.meta/skill.yaml` alongside `skill.meta.yaml`.
- `internal/canonical/canonical.go`:
  - In `catalogAffecting`: return false for `.meta/distill.yaml`.
  - In `entityPath`: recognize `skill.meta.yaml` and `.meta/skill.yaml`.
- `internal/canonical/skill.go`:
  - In `validateSkills`, recognize `.meta/skill.yaml` and allow `.meta/*` companion resources.
- `internal/migration/migration.go`:
  - Bump `CurrentVersion = 2`.
  - Add `planV1ToV2` step to migrate `.skillhub/schema-version` from 1 to 2.
- `docs/design/07-storage-and-mutation-model.md`:
  - Document canonical schema version 2 and minimum binary version requirement.

## Implementation Steps
1. Define `IsHubMeta(relPath string) bool` in `internal/workspace/workspace.go` and test it thoroughly on all path variants.
2. Replace all exact `skill.meta.yaml` special cases with `IsHubMeta` across `internal/skillruntime`, `internal/canonical`, `internal/catalog`, and `internal/app`.
3. Inbound `.meta/` stripping: update `skill_discovery.go` and `upstream_update.go` to ignore `.meta/` files from upstream sources and emit `upstream_meta_ignored` warnings.
4. Update `catalogAffecting` in `internal/canonical/canonical.go` and `internal/catalog/input.go` to exclude `.meta/distill.yaml`.
5. Bump schema version to 2 in `internal/workspace/workspace.go` and add `planV1ToV2` migration step in `internal/migration/migration.go`.
6. Update docs in `docs/design/07-storage-and-mutation-model.md`.

## Verification & Tests
- `TestUpstreamMetaCannotSelfApproveOnImportAndUpstreamUpdate`: fixture shipping `.meta/skill.yaml` with pre-filled approval digest has `.meta/` stripped, emits `upstream_meta_ignored` warning, and creates unapproved draft skill.
- `TestDistillYamlDoesNotChangeCatalogSnapshot`: writing `.meta/distill.yaml` leaves `CatalogSnapshot` and `ContentDigest` unchanged, whereas `.meta/skill.yaml` changes `CatalogSnapshot`.
- `TestMetaFilesNeverDistributedOrInSnapshots`: ensure `.meta/*` files are never returned by `resources/read` and never copied into snapshot directories.
- `TestCanonicalSchemaMigrationV1ToV2`: existing v1 workspace successfully migrates to v2 via `skillhub migrate`; newer schema (v3) is rejected by version check.

## Acceptance Criteria
- `IsHubMeta` predicate is used consistently across all identified sites.
- Upstream-supplied `.meta/` is stripped with warnings on import, add, and upstream update.
- Writing `.meta/distill.yaml` changes neither content digest, review state, nor catalog snapshot.
- `.meta/` files are excluded from distribution resources and runtime snapshots.
- Workspace schema version 2 is enforced; v1 workspaces migrate cleanly; newer versions fail closed.
