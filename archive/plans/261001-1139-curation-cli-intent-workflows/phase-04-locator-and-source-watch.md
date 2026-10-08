---
phase: 4
title: "Locator resolution and source watch"
status: completed
priority: P0
effort: "4-5d"
dependencies: []
---

# Phase 4: Locator resolution and source watch

## Context Links

- [BUG-01, BUG-10, BUG-11](./reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)
- [Independent locator contract](./reports/independent-evaluation-261001-1651-curation-cli-redesign-report.md#61-github-url-and-ref-resolution)
- [Source learning design](../../../docs/design/06-source-learning-and-distillation.md)

## Objective

Create one safe normalized-locator layer for public GitHub URLs and local add snapshots, fix monitoring opt-out, and provide atomic preview/confirm application services for explicit GitHub source watching.

## Fixed Contracts

- Supported GitHub forms: repository root, `.git`, `/tree/<ref>/<path>`, and `/blob/<ref>/<path>/SKILL.md`.
- Resolve slash-containing refs by querying a bounded fresh advertised head/tag set, selecting one unique longest matching ref prefix, fetching the exact selected ref/object, peeling annotated tags, and pinning the commit/tree digest. Ambiguity returns `ambiguous_ref` with exact `--ref`/`--path` guidance.
- Remote access reuses existing HTTPS/public-address policy and pinned Git snapshots. No GitHub REST dependency is introduced; stale cached/deleted refs cannot participate.
- Local CLI add accepts relative, home-relative, and absolute folders only for one-shot immutable capture. The authorized absolute root exists in memory only until bytes are copied to a workspace-private content-addressed snapshot, then is discarded. Root/descendant symlinks, Windows reparse points, special files, and path races are rejected.
- Persisted local locator/origin/proposal data contains only snapshot handle, digests, transformations, and selected skill-relative path—never absolute paths or path-derived labels.
- `source watch` supports public GitHub only, defaults to weekly cadence, creates a watching source record, and never imports skills. Local watch returns `local_watch_unsupported` without writes.
- Explicit monitoring false remains false regardless of cadence. Watch idempotency includes normalized locator, source ID, ref/path, cadence, monitoring policy, and pinned request digest; same locator with different policy is a conflict, not success or update.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/source/types.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/policy.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/git_repository.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/filesystem.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/adapters_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/filesystem_limits_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/records.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/source.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/source_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/nofollow_other.go`

Create:

- `/home/vantt/projects/mcp-skill-hub/internal/source/locator.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/locator_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/source_watch.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/source_watch_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/nofollow_windows.go`
- `/home/vantt/projects/mcp-skill-hub/internal/source/nofollow_windows_test.go`

No other phase edits these files.

## Implementation Steps

1. Define separate locator types: an in-memory authorized local root and a serializable normalized locator/snapshot reference. Make accidental serialization of the transient root impossible.
2. Parse GitHub URL routes structurally. Query bounded fresh advertised heads/tags under existing network policy; select uniquely, fetch the exact ref/object, peel annotated tags, prune stale candidates, and pin commit/tree digest. Require explicit flags when interpretation remains ambiguous.
3. Extend filesystem capture through an explicitly authorized one-shot root and explicit workspace-private cache root. Snapshot before proposal planning; immediately discard the host root; subsequent reads use content-addressed cache bytes even if the original changes/disappears.
4. Add Windows-specific handle-verified traversal that rejects all reparse points and path swaps. Keep descriptor-safe Linux/Darwin paths and a conservative unsupported-platform fallback.
5. Reject special files, path escape, oversized trees, encoded traversal, credentials in URLs, unsupported schemes/hosts, and private-address redirects.
6. Remove BUG-01's monitoring re-enable branch. Distinguish unspecified/default monitoring from explicit false; persist disabled cadence as manual for legacy onboarding.
7. Implement `PreviewSourceWatch` and `ConfirmSourceWatch` using the existing source proposal boundary: normalize locator, identify revision, derive a stable ID, persist exact pins/write set, and commit one source record/receipt.
8. Check exact watch-policy/request equality before returning idempotent success. Same locator with different policy or different locator with same ID returns typed conflict and exact guidance; no silent suffix/update.
9. Keep capture/triage/check/import paths working. Watch creates no skill and source checks remain explicit network actions.

## Todo

- [x] Add separate transient and serializable locator contracts.
- [x] Resolve/fetch advertised heads and tags, including slash refs, annotated tags, deletion, and ambiguity.
- [x] Add safe external local immutable capture with workspace-private cache and no path persistence.
- [x] Add Windows reparse-safe traversal and swap-race tests.
- [x] Fix explicit no-monitor handling.
- [x] Implement persisted source-watch preview/confirm with policy-aware idempotency.
- [x] Add URL, path, policy, collision, idempotency, and no-write rejection tests.

## Success Criteria

- BUG-01: no-monitor without cadence remains disabled.
- BUG-10: valid GitHub tree/blob URLs—including slash heads/tags—resolve or return actionable ambiguity, never an opaque clone error.
- BUG-11 prerequisite: an external local folder can be captured immutably for CLI add; no local source record can be watched and no host path appears in persisted bytes.
- Windows junction/reparse and path-swap fixtures cannot escape the authorized root.
- Source watch preview reports exact repository/ref/path/revision and policy. Only an identical locator+policy request is idempotent; stale/changed proposals fail confirmation.

## Verification

```bash
go test ./internal/source -run 'Test.*(Locator|Git|Filesystem|Snapshot|Symlink|Limit|Record|Proposal)'
go test ./internal/app -run 'Test.*(SourceWatch|SourceOnboarding|Monitoring)'
go test -race ./internal/source ./internal/app
```

Use local Git remotes for deterministic heads/tags, including `feature/x`, annotated tags, deleted refs, and same-name branch/tag collisions. Keep one optional bounded live GitHub smoke for Phase 9.

## Risks and Security

- GitHub route syntax is ambiguous when refs contain slashes; never guess without fresh advertised-ref evidence and exact object acquisition.
- Avoid command injection by using library APIs or argv-separated process calls; never invoke a shell.
- Raw external paths are a CLI-side user authority only. Phase 7 MUST reject them over MCP unless a future explicit host capability is designed.
- Snapshot/proposal caches are private, bounded, expiring, and byte-scanned in tests to prove the original absolute path is absent.

## Next Step

Phase 5 consumes normalized locators/snapshots. Phases 6 and 7 expose source watch and check aliases.