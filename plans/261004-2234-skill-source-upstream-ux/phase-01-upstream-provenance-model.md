---
title: "Phase 1: Upstream provenance model"
status: todo
---

# Phase 1: Upstream provenance model

## Context

- Plan: [plan.md](./plan.md) (decisions D1, D3). Research: [upstream UX](./reports/researcher-261004-2234-upstream-update-ux-patterns.md), [merge and fetch](./reports/researcher-261004-2234-three-way-merge-and-fetch.md).
- Read first: `AGENTS.md` (Testing section), `internal/app/skill_add.go`, `internal/canonical/skill.go:167-226`, `internal/source/git_repository.go`, `internal/skillruntime/trust.go:34-46`, `internal/app/skill_add_test.go:623-686` (real file:// Git harness).

## Overview

Make `provenance.origin` precise enough to track an upstream per skill: a repository-root-relative `path`, the commit actually read, a per-skill `folder_digest`, and a new `files_digest` that lets Skill Hub detect local edits offline. Add a Git adapter method that resolves a revision for a known commit, fetching that commit by SHA when the mirror lacks it.

## Requirements

1. Canonical validator accepts optional `provenance.origin.files_digest` (lowercase `sha256:<64 hex>`), same rule as `folder_digest`.
2. For GitHub/Git adds, each skill's origin gets:
   - `commit` = the commit whose files were read (`rev.Value`), not a separately resolved commit.
   - `path` = repository-root-relative skill folder: `path.Join(resolved.Path, item.SkillDir)` with empty parts dropped (`""` when the skill is at the repository root).
   - `folder_digest` = digest of the tree object at `origin.path` in `origin.commit` (what `GitRepositoryAdapter.CurrentRevision` returns as `ContentDigest` for that path).
   - `files_digest` = `skillruntime.ContentDigest(files, skillruntime.Spec{}, false)` where `files` are `{path relative to the skill folder, sourcepkg.Digest(bytes)}` for every file written into the skill folder except `skill.meta.yaml`.
3. Local-folder adds are unchanged (no `files_digest`).
4. `GitRepositoryAdapter.RevisionAt(ctx, source, commit)` returns a `Revision{Kind:"git-commit", Value: commit, ContentDigest: digest of scoped tree at source.Locator.Path}` without network when the commit is in the mirror; otherwise fetches exactly that commit at depth 1 (refspec `<sha>:refs/skillhub/commits/<sha>`), then resolves. Errors: `ErrHistoryUnavailable` when the remote refuses SHA wants (`git.ErrExactSHA1NotSupported`, "not our ref") or the commit does not exist; new sentinel `ErrPathNotFound` when the commit exists but `source.Locator.Path` is absent in its tree (`scopedObjectHash` maps go-git's `object.ErrEntryNotFound`/`ErrDirectoryNotFound` to it). Refs under `refs/skillhub/commits/` are never consulted by `resolveCommit` (branch/tag resolution only), so a fetched commit can never become a ref's HEAD.
5. `GitRepositoryAdapter.RemoteRefCommit(ctx, repository, ref) (string, error)`: the ref's commit from `ListAdvertisedRefs` (ls-remote: no pack transfer, no mirror write); accepts branch or tag names and `refs/heads/...`/`refs/tags/...`, preferring peeled tag commits. This is the cheap "did it move?" probe used by checks. (`ResolveRefCommit` is not suitable: it calls `syncMirror`, `git_repository.go:753-758`.)
6. Mirror locking (`github.com/gofrs/flock`, already a dependency): one unexported helper `withMirrorLock(ctx, mirror string, shared bool, fn func() error) error` locks `<mirror>.lock`, waits at most 30 s (`context.WithTimeout`), and returns the new sentinel `ErrMirrorBusy` ("another Skill Hub process is using the repository cache; retry shortly") on timeout. `syncMirror` and `RevisionAt` take the exclusive lock exactly once at their public entry and call unexported unlocked inner functions (`syncMirrorLocked`, `openOrCloneLocked`), so no code path re-locks while holding the lock. `Read`, `List`, `Diff`, and `CommitHasSkill` take the shared lock around `openMirror` use. The lock is per mirror path, so it serializes CLI, MCP, and WebUI processes alike.
7. Adapters that do not implement `RevisionAt` (test stubs) never get a substitute revision: the helper returns `ErrHistoryUnavailable`, `folder_digest` is left empty, and `files_digest` is still computed from the written bytes (it needs no adapter).

## Architecture

```text
captureRemoteSkillAddSource ─► rev (CurrentRevision at ref) ─► origin.commit = rev.Value
buildSkillAddChanges ─► per skill: repoPath = join(resolved.Path, SkillDir)
                      ─► folder_digest = adapter.RevisionAt(repo@ref, path=repoPath, rev.Value).ContentDigest
                      ─► files_digest  = ContentDigest(files written under skills/<col>/<id>/ minus meta)
```

The origin construction moves into one helper so phase 2 (`source import`) and phase 4 (update apply) reuse it.

## Related code files

Modify:
- `internal/canonical/skill.go` — add `files_digest` to the origin key set (line 179) and to the digest loop (line 207).
- `internal/app/skill_add.go` — `SkillOrigin` gains `FilesDigest string \`json:"files_digest,omitempty" yaml:"files_digest,omitempty"\``; `skillOriginToMap` writes it; `captureRemoteSkillAddSource` sets `Commit: rev.Value` (line 425) and keeps `resolved.Path` as scope; `buildSkillAddChanges` (line 554) uses the helper for path and digests.
- `internal/source/git_repository.go` — add `RevisionAt`, `RemoteRefCommit`, `ErrPathNotFound`, `ErrMirrorBusy`, `withMirrorLock`; split `syncMirror`.
- `schemas/skill-metadata.schema.json` — add `"files_digest": {"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"}` next to `folder_digest` (line 135); the origin object has `additionalProperties: false`, so the field is rejected without this.
- `internal/app/skill_add_test.go`, `internal/source/adapters_test.go` (or a new `internal/source/git_revision_at_test.go`), `internal/canonical/canonical_test.go`, `schemas/embed_test.go` (extend `TestSkillMetadataProvenanceAndStructuredOrigin` at line 200 with one valid `files_digest` case).

Create:
- `internal/app/upstream_origin.go` — `repoRelativeSkillPath(scopePath, skillDir string) string`; `filesDigestOf(files map[string][]byte) string` (keys relative to the skill folder, `skill.meta.yaml` skipped); `buildGitOrigin(...)` returning `SkillOrigin` with all four fields.

Do not modify any other file.

## Implementation steps

### Task 1.0 — Confirm the runtime plan is complete or does not overlap
- Goal: never edit a file a pending runtime phase still owns.
- Steps: run `grep -nE "\| (Pending|In progress) \|" plans/261004-1547-skill-execution-measurement-routing/plan.md`. If it prints nothing, continue. If it prints rows, collect the files of the pending runtime phases with `grep -ohE "(internal|schemas|web|system-skills|docs)/[A-Za-z0-9_./-]+\.(go|json|ts|tsx|md)" plans/261004-1547-skill-execution-measurement-routing/phase-*.md | sort -u > /tmp/runtime-files.txt` and compare them with every file this plan's phases 1–5 modify: `grep -nF -f /tmp/runtime-files.txt plans/261004-2234-skill-source-upstream-ux/phase-0[1-5]-*.md`.
- Verify: either the first command prints nothing, or the last command prints nothing. Any match is a failure (Failure Protocol).

### Task 1.1 — Canonical field
- Steps: in `internal/canonical/skill.go` add `"files_digest"` to the `stringSet(...)` at line 179 and to `[]string{"folder_digest", "content_digest"}` at line 207. Extend the existing provenance case table in `internal/canonical/canonical_test.go` with one valid and one invalid (`files_digest: abc`) case.
- Also add `files_digest` to `schemas/skill-metadata.schema.json` and one valid case to `TestSkillMetadataProvenanceAndStructuredOrigin`.
- Verify: `go test ./internal/canonical/ ./schemas/ -count=1` exits 0 and prints `ok` for both packages.

### Task 1.2 — `RevisionAt` and mirror lock
- Steps:
  1. Add `withMirrorLock`, `ErrMirrorBusy`, `ErrPathNotFound`, and split `syncMirror` into a locked entry plus `syncMirrorLocked`. Add `RemoteRefCommit` (Requirement 5). Add `func (adapter GitRepositoryAdapter) RevisionAt(ctx context.Context, source Source, commit string) (Revision, error)`: validate `validGitObject(commit)` and the remote URL; take the exclusive mirror lock once; open the mirror, or clone it with `syncMirrorLocked` when absent; if `repository.CommitObject(hash)` fails, fetch `config.RefSpec(commit + ":refs/skillhub/commits/" + commit)` with `Depth: 1` through `withSafeTransport`; map `git.ErrExactSHA1NotSupported` and "not our ref" errors to `ErrHistoryUnavailable`; compute `scopedObjectHash(commit, source.Locator.Path)`; return `Revision{Kind:"git-commit", Value: commit, ContentDigest: Digest([]byte(hash.String())), ObservedAt: adapter.Now().UTC()}`. Apply `checkMirrorSize` after a fetch.
  2. Wrap `Read`, `List`, `Diff`, `CommitHasSkill` mirror access in the shared lock.
  3. Tests (`TestGitRevisionAt`, file:// repositories; go-git's file transport runs the system `git-upload-pack`, which refuses unadvertised SHAs unless configured, so the fixture configures it): repository R1 with `git -C <R1> config uploadpack.allowReachableSHA1InWant true` (skip the test with `t.Skip` when `exec.LookPath("git")` fails, like other file:// tests need git anyway), commits A then B, mirror synced at depth 1 (B only): `RevisionAt(A)` succeeds and equals the digest of A's scoped tree; `RevisionAt(B, Path: "missing")` returns `ErrPathNotFound`; an unknown 40-hex commit returns `ErrHistoryUnavailable`; after fetching A, `resolveCommit(repo, "main")` still returns B. Repository R2 without the config: `RevisionAt(A)` returns `ErrHistoryUnavailable`. `RemoteRefCommit(R1, "main")` returns B and leaves the mirror directory unchanged (compare its file list before/after). A goroutine holding `withMirrorLock` exclusively makes a second caller with a 1 s context return `ErrMirrorBusy`.
- Verify: `go test ./internal/source/ -run 'TestGitRevisionAt' -count=1` exits 0 and prints `ok`.

### Task 1.3 — Origin helper and `skill add`
- Steps:
  1. Create `internal/app/upstream_origin.go` with the three functions above. `buildGitOrigin` takes the captured origin (repository, ref, commit, scope path, name), the skill folder (`item.SkillDir`), the written files (relative path → bytes), the transformations, `addedAt`, and a `revisionAt func(path string) (sourcepkg.Revision, error)` callback; it returns the origin with `Path`, `FolderDigest`, `FilesDigest`, `ContentDigest` (normalized SKILL.md digest, unchanged meaning).
     Build the callback from an optional interface declared in the same file, `type revisionAtAdapter interface{ RevisionAt(context.Context, sourcepkg.Source, string) (sourcepkg.Revision, error) }`. When the configured `sourcepkg.Adapter` does not implement it (test stubs such as the ones in `internal/app/source_import_test.go`), the callback returns `sourcepkg.ErrHistoryUnavailable` and `folder_digest` stays empty (Requirement 7); never substitute `CurrentRevision`. The `sourcepkg.Adapter` interface itself is not changed.
     Also add `importedSkillFiles(item DiscoveredSkillItem, targetID string) (files map[string][]byte, transforms []string, err error)`: normalized `SKILL.md` via `ensureImportedSkillFrontmatter(item.SkillMDBytes, targetID, item.Description)` plus `item.CompanionBytes`. `buildSkillAddChanges` and every later reconstruction (phases 3–5) use it on items produced by `DiscoverSkillsFromResources`, so description fallbacks (`skill_discovery.go:415-424`) and nested-skill exclusion (`:446-449`) are identical at add time and at reconstruction time.
  2. In `captureRemoteSkillAddSource` set `Commit: rev.Value`. Keep `resolved.Path` in a new unexported field of `skillAddCapture` (`scopePath`) instead of `origin.Path`.
  3. In `buildSkillAddChanges`, for `kind != "local"`, collect the bytes written for each skill (normalized `SKILL.md` plus `item.CompanionBytes`) and call `buildGitOrigin`. Pass `buildSkillAddChanges` the adapter-backed callback; for local adds keep the current code path.
  4. Extend `TestSkillAddRemoteGitRealAdapter` (do not add a near-duplicate): use a repository with `skills/a/SKILL.md` and `skills/b/SKILL.md` plus `skills/b/scripts/run.sh`, add with locator `file://<repo>/tree/<branch>/skills` and `All: true`, and assert for each written `skill.meta.yaml`: `origin.path` is `skills/a` / `skills/b`; `origin.commit` equals the repository HEAD; `folder_digest` differs between a and b; `files_digest` equals `SkillService.ContentTrustFor(ctx, root, id).ContentDigest` (no runtime block, so the two digests coincide). If the file:// locator form `/tree/` is not parsed for file URLs, use `SkillAddInput{Locator: fileURL}` with `Selection`/`All` on the repository root and assert `skills/a` paths the same way.
- Verify: `go test ./internal/app/ -run 'TestSkillAdd' -count=1` exits 0 and prints `ok`.

### Task 1.4 — Gate
- Verify: `make check` exits 0.

## Todo

- [ ] Task 1.0 overlap check
- [ ] Task 1.1 canonical `files_digest`
- [ ] Task 1.2 `RevisionAt`, `RemoteRefCommit`, mirror lock
- [ ] Task 1.3 origin helper + `skill add` fixes
- [ ] Task 1.4 `make check`

## Success criteria

- New GitHub/Git adds record repository-relative paths, the read commit, per-skill folder digests, and `files_digest`.
- `RevisionAt` reads a commit absent from a depth-1 mirror.
- No behavior change for local-folder adds; existing add tests pass unchanged.

## UX acceptance

No user-visible output changes in this phase. `skill add` preview/result text stays as today (`internal/delivery/cli/skill_add.go:94-135`).

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Hosts that refuse SHA wants (non-GitHub) | M×M | `ErrHistoryUnavailable`; later phases fall back to explicit per-file choices (D7). |
| Lock contention makes a CLI command wait on a running web check | M×L | Bounded 30 s wait, then `ErrMirrorBusy` with a retry message; single acquisition per public call (no re-entry). |
| Changing `origin.commit` source alters idempotency digests | L×L | `skillAddRequestDigest` uses `folder_digest` of the capture, unchanged in shape; replay tests cover it. |

## Security considerations

`RevisionAt` reuses the credential-free safe transport, URL validation, and mirror size limits. Only a validated 40-hex commit is ever placed in a refspec.

## Rollback

Revert the phase commit. Skills already written with `files_digest` keep validating only with this binary; remove the key manually if an older binary must read them.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
