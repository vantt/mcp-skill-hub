---
phase: 2
title: "Domain: read, classify and plan a resource edit"
status: completed
priority: P2
effort: "2.5h"
dependencies: []
---

# Phase 2: Domain: read, classify and plan a resource edit

## Context

The skill package owns safe canonical writes for skills. `Manager.PreviewUpdate`
(`internal/skill/lifecycle.go:257-380`) builds `mutation.Change` values and calls `Manager.plan`
(`:430-492`), which reads current bytes, plans the mutation, maps a moved `BeforeDigest` to
`*EditConflictError` (`:446-468`), stores nothing (the app layer stores, `internal/app/skill_lifecycle.go:124`)
and renders `FullDiff` through `RenderFullDiff`. `Manager.Confirm` applies and republishes (`:494-520`).
`loadSkill` (`:773-838`) finds the skill's metadata strictly (no symlinked collection or skill folder) and
`skillDirFromMetadataPath` (`:1101`) gives its folder. `normalizedDigest` (`:564`) and
`lifecycleIdempotencyKey` (`:557`) build request digests and idempotency keys. `GetManifestAnyState`
(`internal/skill/resource.go:47`) returns the catalog rows of a skill in any state.

Another agent is changing `RenderFullDiff` in `lifecycle.go`. Do **not** edit `lifecycle.go`; put all new code
in a new file of the same package and call the unexported helpers from there.

## Requirements

New `internal/skill/resource_edit.go`:

- `const MaxEditableResourceBytes = 1 << 20`.
- `type ResourceFile struct { Path string; Kind string; SizeBytes int64; Digest string; Editable bool; Reason string; LineEnding string; Truncated bool; Content []byte; IsText bool }`
  (`Reason`: `""`, `binary`, `too_large`, `missing`, `not_regular`; `LineEnding`: `lf`, `crlf`, `mixed`, `none`).
- `ListResourceFiles(ctx, root, id string) ([]ResourceFile, error)`: for each row of `GetManifestAnyState`
  except the skill's `SKILL.md` and `workspace.IsHubMeta` paths, classify the working-tree file (no `Content`).
  Unknown skill -> `ErrNotFound`.
- `ReadResourceFile(ctx, root, id, path string) (ResourceFile, error)`: same classification plus `Content` for
  text files (first `MaxEditableResourceBytes`, cut back to a whole UTF-8 rune, `Truncated` set when larger).
- `(Manager) PreviewResourceUpdate(ctx, root, id string, input ResourceUpdateInput, fullDiff bool) (Proposal, error)`
  with `type ResourceUpdateInput struct { Path string; Content []byte; ExpectedDigest string; IdempotencyKey string }`.
  - Resolves and validates the path as below; the target must currently be editable.
  - `ExpectedDigest` is required (`sha256:` + 64 hex); mismatch with the current file -> `*EditConflictError`.
  - New content: valid UTF-8, no NUL, at most `MaxEditableResourceBytes`; else a validation error.
  - Line endings: if the current file's `LineEnding` is `crlf`, convert `\r\n` to `\n` then `\n` to `\r\n` in
    the new content; otherwise keep the bytes as sent.
  - Identical bytes after that rule -> a validation error "no change to save".
  - One change only: `mutation.Change{Path: path, BeforeDigest: ExpectedDigest, Contents: newBytes}`; the
    metadata file is not rewritten.
  - Command `skill_edit`; request digest `normalizedDigest(struct{ID, Path, Content, ExpectedDigest string}{...})`;
    idempotency key from `lifecycleIdempotencyKey`; honour `mutation.LookupOperation` for an already-applied
    request the same way `PreviewUpdate` does (`:258-264`).
  - Calls `manager.plan(ctx, root, id, "skill_edit", changes, nil, nil, fullDiff, key, digest)`; nil metadata
    means no routing impact (`routingImpactRequired` compares equal, `:665-673`).
- Path resolution shared by all three functions:
  1. `loadSkill(root, id)` -> skill folder `dir`.
  2. `path` must be non-empty, use `/`, contain no `\`, NUL, empty, `.` or `..` segment, start with `dir + "/"`,
     not equal `dir + "/SKILL.md"`, and not be `workspace.IsHubMeta(path)`. Violations -> validation error
     that names the rule, never echoing file content.
  3. `path` must be a row of the skill's manifest (`GetManifestAnyState`); otherwise `ErrNotFound`.
  4. Open the workspace with `os.OpenRoot`; `Lstat` every segment from `dir` to the file: a symlink or
     non-directory parent -> `not_regular`; a symlink or non-regular file -> `not_regular`; absent -> `missing`.
  5. Size above `MaxEditableResourceBytes` -> `too_large` (view only, text still readable truncated);
     content with NUL or invalid UTF-8 -> `binary` (no content returned).

## Files

- Create: `internal/skill/resource_edit.go`, `internal/skill/resource_edit_test.go`
- Read only: `internal/skill/lifecycle.go`, `internal/skill/resource.go`, `internal/mutation/*`, `internal/workspace/*`

## Steps

1. Write the path resolver and classifier with table tests first.
2. Add `ListResourceFiles` and `ReadResourceFile`.
3. Add `PreviewResourceUpdate`; confirm through the existing `Manager.Confirm` in tests.
4. Tests on a temp workspace built like the existing tests in `lifecycle_test.go` (init, create a skill,
   add `scripts/run.sh` mode 0755, `references/guide.md`, `references/win.txt` with CRLF, `assets/logo.png`
   with NUL bytes, a 1.5 MiB text file, then rebuild the catalog):
   - classification of each file; `SKILL.md` and `.meta/skill.yaml` absent from the list;
   - rejected paths: `../`, absolute, backslash, NUL, `skills/core/other/...`, `SKILL.md`, `.meta/skill.yaml`,
     a path not in the manifest, a symlinked file, a file under a symlinked directory (created after rebuild);
   - preview + confirm edits `scripts/run.sh`: bytes on disk equal the previewed content, mode stays 0755,
     metadata file unchanged, `Summary.Modified` lists only that path, `FullDiff` contains the file header and
     the changed line with `-` and `+` (do not assert hunk layout);
   - CRLF file stays CRLF after an LF edit; a BOM round-trips;
   - stale `ExpectedDigest` -> `*EditConflictError`; file changed between preview and confirm -> confirm
     fails with `mutation.ErrConflict` or `ErrSnapshotExpired`;
   - binary or too-large new content, unchanged content -> validation errors;
   - lifecycle state of an active skill is still active after confirm.

## Validation

- `go test ./internal/skill/ -run 'Resource' -count=1`
- `go vet ./internal/skill/` and `go test ./internal/skill/ -count=1`

## Risks

- A symlinked parent created after the catalog was built is the hardest case; the per-segment `Lstat` test
  covers it, and the mutation layer refuses it again on write.
- `RenderFullDiff` output may change shape while this phase runs; assertions stay layout-independent.

## Rollback

Revert the commit (only after reverting Phase 3, which calls these functions).
