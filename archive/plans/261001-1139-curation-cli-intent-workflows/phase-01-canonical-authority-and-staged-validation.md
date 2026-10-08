---
phase: 1
title: "Canonical authority and staged validation"
status: completed
priority: P0
effort: "4-5d"
dependencies: []
---

# Phase 1: Canonical authority and staged validation

## Context Links

- [BUG-04, BUG-05, BUG-06](./reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)
- [Storage and mutation model](../../../docs/design/07-storage-and-mutation-model.md)
- [CLI conventions: staged validation](./reports/researcher-261001-1644-cli-conventions-evidence.md)

## Objective

Make one canonical validator authoritative for working-tree validation, mutation planning, and catalog publication; make clone-like missing empty directories valid; treat skill companions as opaque bounded resources; provide literal Git-index validation; and harden cancellation before canonical displacement.

## Fixed Contracts

- `canonical.Validate` owns every structural acceptance rule. Catalog projection may assert validated invariants but MUST NOT reject a state accepted by canonical validation.
- Canonical tree validation is separable from live-workspace environment checks. Working-tree mode requires the real Git workspace; detached staged mode receives original-index conflict/identity facts from its caller and does not require `.git` in the materialized tree.
- A missing canonical directory is equivalent to an empty directory only when no file/symlink occupies that path and no referenced entity requires missing content. Unsafe path types remain errors.
- `SKILL.md` must be non-empty UTF-8 Markdown. Any other bounded regular file beneath the skill root, including empty/binary/YAML files, is an opaque resource; symlinks, traversal, devices, sockets, and FIFOs are rejected. Only canonical entity paths receive YAML identity/reference validation.
- Origin metadata is versioned under `skill.meta.yaml.provenance.origin`; legacy `provenance.source_id/revision/path` remains valid. Local origin persists only kind, content/folder digests, transformations, added time, and selected skill-relative path—never a host-path-derived label.
- Staged validation enumerates stage-0 index entries, rejects unmerged/symlink/submodule/special modes, reads literal blobs without checkout conversion, and materializes them through Go filesystem APIs in a private directory. It never runs filters, stashes, resets, checks out, writes the object database, or changes the live index/worktree.
- New application errors use one typed vocabulary and committed schema before CLI/MCP adapters map them.
- Cancellation before canonical displacement causes no writes. Touched confirmation paths use a context-aware mutation option; once displacement starts, publication/recovery reaches a durable explicit outcome rather than returning an ordinary cancellation.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/canonical/canonical.go`
- `/home/vantt/projects/mcp-skill-hub/internal/canonical/skill.go`
- `/home/vantt/projects/mcp-skill-hub/internal/canonical/canonical_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/canonical/resource_limits_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/workspace/workspace.go`
- `/home/vantt/projects/mcp-skill-hub/internal/workspace/workspace_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/operations.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/operations_test.go`
- `/home/vantt/projects/mcp-skill-hub/schemas/skill-metadata.schema.json`
- `/home/vantt/projects/mcp-skill-hub/schemas/embed_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/input.go`
- `/home/vantt/projects/mcp-skill-hub/internal/catalog/catalog_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/mutation/mutation.go`
- `/home/vantt/projects/mcp-skill-hub/internal/mutation/transaction.go`
- `/home/vantt/projects/mcp-skill-hub/internal/mutation/mutation_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/mutation/hardening_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/errors.go`
- `/home/vantt/projects/mcp-skill-hub/schemas/error-envelope.schema.json`

May create:

- `/home/vantt/projects/mcp-skill-hub/internal/app/git_index_snapshot.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/git_index_snapshot_test.go`

No other phase edits these files.

## Implementation Steps

1. Move the SKILL.md frontmatter-name/directory-ID rule from `WorkspaceService.ValidateWorkspace` into canonical skill validation. Reconcile catalog input shape checks so they consume canonical validation or remain non-rejecting defensive assertions. Add parity tests across every projected entity kind.
2. Change workspace inspection so Git-untracked empty directories are optional in clone-like state while existing non-directory/symlink collisions remain findings. Add fresh-clone fixtures that delete empty canonical directories before validation and rebuild preparation.
3. Classify files by canonical path role before YAML parsing. Replace the three-directory resource allowlist with a safe relative-resource contract; preserve arbitrary regular companion bytes, including empty and YAML/binary resources, under current hard bounds.
4. Define strict, backward-compatible origin fields with no absolute or path-derived local label. Reject unsafe/unknown structures without invalidating legacy watched-source provenance.
5. Split canonical-tree validation from live Git/workspace checks. Working-tree validation composes both; detached validation accepts explicitly supplied original-index health facts.
6. Implement raw index capture: pin the index identity, enumerate NUL-delimited stage-0 entries, reject unmerged/non-regular Git modes, read exact blobs through a non-filtering plumbing/library path, materialize with private permissions, recheck index identity, then run detached canonical validation.
7. Define typed errors and schema entries for locator ambiguity, selection, conflicts, limits, source changes, edit conflicts, stale proposals, validation, unsupported local watch, and unavailable resource content. CLI/MCP consume these constants rather than strings.
8. Add an optional caller context to mutation confirmation/lock acquisition without breaking untouched callers. Define/test the no-cancellation point and explicit recoverable outcome after canonical displacement.
9. Return reusable staged/unstaged path summaries for Phase 3 review without exposing raw file contents or machine-local absolute paths.

## Todo

- [x] Centralize all publication acceptance rules in canonical validation.
- [x] Accept absent empty canonical directories after clone without weakening unsafe-path checks.
- [x] Treat noncanonical skill companions as opaque bounded regular resources.
- [x] Add backward-compatible, privacy-safe structured origin validation.
- [x] Implement detached canonical validation and literal Git-index blob materialization.
- [x] Add typed shared errors and committed schema coverage.
- [x] Add context-aware pre-displacement cancellation semantics.
- [x] Add filter-execution, index-race, workspace/index non-mutation, and validator-parity tests.

## Success Criteria

- BUG-05: every state accepted by `skillhub validate` is accepted for catalog projection/publication; frontmatter and projection-shape mismatches fail both.
- BUG-06: a clone-like workspace with omitted empty directories validates; malformed directory replacements still fail.
- BUG-04 prerequisite: top-level files such as `LICENSE.txt`, `forms.yaml`, empty files, binary assets, and nested templates validate as resources without entity/reference parsing.
- Staged-invalid/worktree-valid and staged-valid/worktree-invalid cases return opposite results; a configured Git smudge/process filter is never executed.
- Index bytes/identity and Git status are identical before and after staged validation; valid detached snapshots do not fail for missing `.git`.
- Cancellation while waiting for the mutation lock writes nothing; cancellation after displacement returns an explicit applied/recovery outcome.

## Verification

```bash
go test ./internal/canonical ./internal/workspace ./internal/catalog -run 'Test.*(Validate|Input|Entity|Resource|Clone)'
go test ./internal/app -run 'Test.*(ValidateWorkspace|Staged|GitIndex|Error)'
go test ./internal/mutation -run 'Test.*(Cancel|Confirm|Recovery|Lock)'
go test ./schemas
```

Smoke with a disposable repository: configure a filter command that would create a sentinel, stage raw content, run staged validation, and prove the sentinel is absent. Compare the index file digest and `git ls-files --stage -z` bytes before/after rather than calling write-producing Git commands.

## Risks and Security

- Broad companion support increases attack surface. Never execute imported resources; classify canonical entity paths before YAML parsing and validate path/type/size before reading or mutation planning.
- Run Git with hardened argv/environment and use literal object reads, never checkout/smudge semantics or a shell.
- Detached mode may suppress only repository-presence checks; it still receives original-index conflicts and enforces every canonical content/layout invariant.
- Missing-directory tolerance must not accept a symlink or regular file at a required directory path.

## Next Step

Phases 3 and 5 consume the frozen canonical/origin/resource contracts. Phase 6 consumes staged validation.