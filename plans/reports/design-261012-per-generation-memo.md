# Per-generation memo: design note

Date: 2026-10-12. Scope: `internal/catalog`, `internal/app` (plus the listing in `internal/workspace`). Nothing here changes a public contract, a telemetry event type or `EventVersion`.

## Problem

Every read request (`skills/list`, `skills/get`, `resources/read`, `skill_get`, `skill_resolve`) opened the current generation again: flock, pointer read, SQLite open and metadata query, tree lstat, a pin file written and removed, a second SQLite open, then the same queries and the same per-file hashing as the request before. The generation is immutable, so all of that derived data is the same until the pointer moves.

## What is cached

One memo per observed generation, holding values that are pure functions of the immutable generation:

| Value | Used by | Replaces |
|---|---|---|
| recommendation policy (parsed) | resolve | `routing_documents` query and JSON decode |
| decoded resolver `Skills()` | resolve | decoding every active skill's `content_json` |
| active skill id list | `skills/list` | one query |
| skill row: name, description, status, manifest document, resource rows, decoded routing and rationale | `skill_get`, manifests, setup status | three queries and a JSON decode |
| skill record: the `DistributedSkill` manifest and, per distributed file, its canonical path and pinned digest | `skills/list`, `skills/get`, `resources/read`, `Ensure`, resolve | reading and hashing every file of the skill |

The curator skill compiled into the binary is built once per process.

## Key

`(workspace root, generation id, generation file identity)`. The file identity is the same `canonical.FileIdentity` the integrity cache uses, so a generation file that is replaced or rewritten under the same name gets a fresh memo.

## Lookup without opening SQLite

A request still takes the shared workspace lock, reads the pointer, stats the generation file and runs `canonical.ScanCached`, the freshness check against canonical files. Three things changed:

1. The metadata comparison of `verifyPointerDatabase` is remembered next to the quick_check result for the same file identity, so an unchanged generation file no longer needs a SQLite open to verify.
2. The database is opened (and pinned) lazily, only when a request misses the memo (`catalog.View.DB`). A memo hit opens no database, writes no pin file and parses no schema.
3. The view keeps the digests that scan computed. A cached record is trusted only while every file of the skill still has its pinned digest in that scan. `ScanCached` reuses a digest only for a file whose size, mode, times and file identity are unchanged and settled (older than the 5 s racy window) and reads and hashes every other file, so the record needs no identity bookkeeping of its own and an edited file is noticed on the next request. Bytes that are served (`resources/read`, `skill_get` content, snapshot export) are always read and hashed; only files nobody asked for are checked against the scan's digests.

## Invalidation

- Pointer moves (rebuild, canonical edit with rebuild): the generation id differs, the lookup misses and a new memo replaces the old one for that root.
- Generation file touched, replaced or corrupted: identity differs, so verification runs in full (quick_check and metadata) as before. A corrupted generation never reaches the memo because a view exists only after verification passed.
- Skill file edited without a rebuild (the catalog serves the published generation as fallback): the scan digest no longer equals the pinned digest, the record is not current, the build path reports the digest mismatch as before.
- Failures are never memoized: not found, not servable, digest mismatch and I/O errors rebuild on the next request.

## Pins and locks

The shared workspace lock is held from observe until `Close`, exactly as with `OpenWithFallbackLocked`, so nothing mutates canonical files between the freshness check and the reads. When a request needs the database it gets the same pinned, read-only handle as before (pin created under the lock, removed on `Close`). No pin outlives a request, so there is no long-lived pin to release on generation change or shutdown.

A long-lived `*sql.DB` per generation was measured and not adopted: see the cook report for the numbers. After the memo only resolve (FTS search) and cold misses open a database.

## Memory bound

- One memo per root, at most 8 roots (clear on overflow), as the integrity cache does.
- At most 1024 values per memo (policy, skills, id list, plus one row and one record per skill); on overflow the memo clears and refills. Memory is `O(active skills)` and does not grow with request count.
- A record holds metadata only (paths, digests, manifest), never file bytes. Rows hold the manifest document, a few KB per skill.

## Failure modes

- Concurrent misses build the same value twice; the last writer wins, both are equal.
- A caller mutating a shared value: values are treated as read-only; the resource list is cloned where `skill_get` hands it out.
- A write that keeps size, mtime, ctime and inode: covered by the racy-window rule, same residual risk class as `ScanCached` and the integrity cache.
- Clock skew, network file systems: files too recent relative to hash time are rehashed, as in `ScanCached`.
