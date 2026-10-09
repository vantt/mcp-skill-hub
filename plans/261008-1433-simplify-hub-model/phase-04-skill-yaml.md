# Phase 4: .meta/skill.yaml

## Overview

This phase migrates `skill.meta.yaml` to `.meta/skill.yaml` (D5, D6), establishes canonical schema version 3, and transitions skill-level metadata to the Git-native layout while preserving human content approval, trust verdicts, upstream synchronization cursors, and resolver signals.

## Field Destination Table

Every row has been verified against the Go structs in `internal/` and `schemas/skill-metadata.schema.json`.

| Field in v1 (`skill.meta.yaml`) | Destination in v3 (`.meta/skill.yaml`) | Status | Evidence & Verified Behavior |
|---|---|---|---|
| `schema_version` | `schema_version` | Kept | Kept as `1` in `.meta/skill.yaml`. (Workspace schema in `.skillhub/schema-version` is bumped from 2 to 3; document format schema version remains 1 per `canonical/skill.go:134`). |
| `id` | `id` | Kept | Required skill identifier; validated against kebab-case pattern and skill directory name. |
| `status` | `status` | Kept | Lifecycle state (`draft`, `active`, `deprecated`, `archived`). |
| `name` | (Derived) | Derived | Extracted from `SKILL.md` frontmatter `name:` or top H1 title. `internal/catalog` populates it into SQLite `skills` table and `skill_fts`. Redundant in metadata. |
| `description` | (Derived) | Derived | Extracted from `SKILL.md` frontmatter `description:` or first markdown paragraph. `internal/catalog` populates it into SQLite and resolver candidates. |
| `collection_id` | (Derived) | Derived | Derived from parent directory name `skills/<collection>/<id>`. `catalog/project.go:60-66` already handles path derivation if missing. |
| `collection` | (Deleted) | Deleted | Redundant with directory structure and `collection_id`. Unused by any production consumer. |
| `aliases` | `routing.aliases` | Moved | Grouped under `routing`. Resolver continues receiving aliases unchanged via `catalog/project.go` projection into SQLite and `evidence.go`. |
| `domain` | `routing.domain` | Moved | Grouped under `routing`. |
| `topics` | `routing.topics` | Moved | Grouped under `routing`. Projected into `skill_fts` keywords and candidate scoring. |
| `technologies` | `routing.technologies` | Moved | Grouped under `routing`. Projected into `skill_fts` keywords and tech matching in `evidence.go`. |
| `content` | (Deleted) | Deleted | Verified: Only type-checked as mapping in `canonical/skill.go:171`. Not stored in any struct, never written to SQLite, never read by resolver or runtime. Safely deleted. |
| `routing` | `routing` | Kept | Kept; contains `operations`, `triggers`, `not_for`, `min_scope`, `requirements`, `boosts`, `distinguish_from`, `supporting`, `equivalent_to`, `examples`, `counter_examples`, plus moved routing fields (`aliases`, `domain`, `topics`, `technologies`). |
| `runtime` | `runtime` | Kept | Kept verbatim; contains `requires` (`bins`, `env`, `platforms`) and `setup` (`command`, `check`). Bound into `ContentDigest` (`skillruntime.ContentDigest`) for content trust. |
| `quality` | `quality` | Kept | Kept. Verification of fields: <br>- `content_reviewed_digest`: KEPT. Critical for content trust and approval tracking (`internal/app/skill_review.go:393`, `internal/app/skill_review_changes.go:66`, `internal/app/skill_snapshot.go:300`). <br>- `routing_review_rationale`: KEPT. Required when `routing.not_for` is empty (`canonical/skill.go:310`, `app/skill_review.go:316`). <br>- `reviewed`: KEPT. Read by resolver (`internal/resolver/sqlite_catalog.go:235`, `internal/resolver/evidence.go:334`) to compute quality score. Deletion would alter resolver scoring without routing-eval evidence. <br>- `reviewed_at`: KEPT. Timestamp of review. <br>- `curated`: DELETED. Unused across Go structs. |
| `provenance` | `sources` | Replaced | Replaced by `sources` list (D6). Upstream origin and learning links are unified into sources with roles. |
| `history` | (Deleted) | Deleted | Git commit log is the authoritative history of lifecycle state transitions (D5). Unused in production code. |
| `created_at` | (Deleted) | Deleted | Derived from Git commit timestamps. Top-level metadata timestamp unused. |
| `updated_at` | (Deleted) | Deleted | Derived from Git commit timestamps. Top-level metadata timestamp unused. |

### Sources (D6)

The `sources` list replaces `provenance` and external learning links. Each source entry has:
- `id`: unique identifier for the source (string)
- `roles`: array containing `upstream` and/or `learning`
- `kind`: origin type (e.g., `github`, `git`, `local`)
- `repository`: URL or local repo path
- `ref`: branch/tag
- `commit`: current commit hash
- `path`: path in the repository
- `files_digest`: files digest for upstream drift detection
- `folder_digest`: optional folder digest
- `transformations`: applied transformations ([]string)
- `synced`: cursor representing the last synchronized state (written by upstream/distill sync)

### Third-Party Status (D5)

Third-party status is derived dynamically:
- A skill is third-party if and only if it has a source with role `upstream`.
- The `learning` role NEVER makes a skill third-party.
- Backward compatibility: if `sources` is empty (legacy manifest), fallback to checking `provenance.origin.kind` (`github`/`git`) or `provenance.source_id != ""`.

## Migration: planV2ToV3 (Schema "3")

The workspace migration step `planV2ToV3` bumps `.skillhub/schema-version` from 2 to 3 and transforms `skill.meta.yaml` to `.meta/skill.yaml` through the migration WAL:
1. **Idempotence:** Running `migrate` once transitions v2 to v3. Running it again is a no-op that reports `ErrAlreadyCurrent`.
2. **Version Guard:** Workspaces with schema version > 3 (e.g., v4) are refused with `ErrNoMigrationPath`.
3. **Phase 0 Compatibility:** If a skill already has `.meta/skill.yaml` (such as `test-audit` written during Phase 0 discovery experiments), `migrateSkillYAMLToV3` does NOT overwrite silently:
   - Verifies `id` matches between both files (fails if mismatched).
   - Merges `sources`: converts `provenance` from `skill.meta.yaml` into an `upstream` source and merges with existing `learning` sources in `.meta/skill.yaml`, preserving any `synced` cursor.
   - Merges `routing`, `quality`, and `runtime`.
   - Deletes `skill.meta.yaml` and updates `.meta/skill.yaml`.

## Code Changes

1. **Canonical (`internal/canonical`):**
   - Reads `.meta/skill.yaml` as the canonical skill metadata file (companion resources under `.meta/` allowed).
   - In `validateSkillMetadata`: allows deriving `name` and `description` from `SKILL.md` if omitted in `.meta/skill.yaml`. Validates `sources` list and `quality` fields.
2. **Catalog (`internal/catalog`):**
   - Corrects ID resolution in `input.go` when parent directory is `.meta` (resolves to skill directory name).
   - Derives `name` and `description` from `SKILL.md` during entity projection if absent in `.meta/skill.yaml`.
   - Projects `routing.aliases`, `routing.topics`, `routing.technologies`, and `routing.domain` into SQLite and `skill_fts`.
3. **Runtime & Trust (`internal/skillruntime`):**
   - `IsThirdParty` derives third-party status from `HasUpstream` ("has a source with role `upstream`"). The `learning` role never makes a skill third-party.
   - `ContentDigest` continues binding files (excluding `.meta/` and `skill.meta.yaml`) and `runtime`.
4. **Writers in `app/` and `skill/`:**
   - `internal/skill/lifecycle.go`: `PreviewCreate` writes `.meta/skill.yaml`. `loadSkill` checks `.meta/skill.yaml` first, fallback to `skill.meta.yaml`.
   - `internal/app/skill_add.go`: writes `.meta/skill.yaml`.
   - `internal/app/upstream_update.go`: updates `.meta/skill.yaml` (including `sources`), preserving `quality.content_reviewed_digest`.
   - `internal/app/operations.go` & `insight.go`: locate `.meta/skill.yaml`.
5. **Approval-History Walk (`internal/app/skill_review_changes.go`):**
   - `reviewChangesSinceApproval`: passes both `.meta/skill.yaml` and `skill.meta.yaml` to `git log` so the history walk bridges commits before and after migration.
   - Reads manifest at each commit from `.meta/skill.yaml`, falling back to `skill.meta.yaml`.
   - Excludes `.meta/` from blob sets (`gitTreeBlobs` and `currentBlobs` via `workspace.IsHubMeta`).
6. **Resolver (`internal/resolver`):**
   - Resolver continues receiving `aliases`, `topics`, `technologies`, and `domain` unchanged.
   - Only field accessors in `evidence.go` may be adjusted if needed. Note: Worktree D owns the rest of `internal/resolver`.

## Verification & Required Tests

1. **Trust and Upstream Consistency:**
   - `TestTrustVerdictAndUpstreamStatusUnchangedAfterMigration`: Realistic hub fixture with third-party skill (approved and unapproved), self-authored skill, and test-audit-like skill (with existing Phase 0 `.meta/skill.yaml`). Verify trust verdict, upstream status, and `ChangesSinceApproval` are identical before and after v2→v3 migration.
2. **Migration Idempotence and Version Check:**
   - `TestCanonicalSchemaMigrationV2ToV3`: v2 workspace migrates to v3. Running migration again succeeds idempotently. A v4 workspace is refused with `ErrNoMigrationPath`.
3. **Upstream Inbound Protection:**
   - `TestUpstreamMetaCannotSelfApproveOnImportAndUpstreamUpdate`: Upstream shipping `.meta/skill.yaml` has `.meta/` stripped with `upstream_meta_ignored` warning and cannot self-approve.
4. **Catalog Snapshot Invariant:**
   - `TestDistillYamlDoesNotChangeCatalogSnapshot`: Writing `.meta/skill.yaml` changes `CatalogSnapshot`. Writing `.meta/distill.yaml` does not.
5. **`make check` green:**
   - Format, vet, lint, and full test suite pass cleanly before every commit.
