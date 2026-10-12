# Per-generation memo: design note

Date: 2026-10-12. Scope: `internal/catalog`, `internal/app`. Nothing here changes a public contract, a telemetry event type or `EventVersion`.

## Problem

Every read request (`skills/list`, `skills/get`, `resources/read`, `skill_get`, `skill_resolve`) opens the current generation again: flock, pointer read, SQLite open and metadata query, tree lstat, a pin file written and removed, a second SQLite open, then the same queries and the same per-file hashing as the request before. The generation is immutable, so all of that derived data is the same until the pointer moves.

## What is cached

One memo per observed generation, holding values that are pure functions of the immutable generation:

| Value | Used by | Replaces |
|---|---|---|
| recommendation policy (parsed) | resolve | `routing_documents` query and JSON decode |
| decoded resolver `Skills()` | resolve | decoding every active skill's `content_json` |
| active skill id list | `skills/list` | one query |
| skill row: name, description, status, manifest document, resource rows, decoded routing and rationale | `skill_get`, manifests | three queries and a JSON decode |
| skill record: `DistributedSkill` manifest, manifest document, and per file the identity (size, mode, mtime, ctime, device, inode) seen when its bytes matched the pinned digest | `skills/list`, `skills/get`, `resources/read`, `Ensure`, resolve | reading and hashing every file of the skill |

## Key

`(workspace root, generation id, generation file identity)`. The file identity is the same `canonical.FileIdentity` the integrity cache uses, so a generation file that is replaced or rewritten under the same name gets a fresh memo. The catalog snapshot is part of the generation, so it is covered by the id.

## Lookup without opening SQLite

A request still takes the shared workspace lock, reads the pointer, stats the generation file and runs `canonical.ScanCached` (the freshness check against canonical files). New: the metadata check of `verifyPointerDatabase` is remembered next to the quick_check result for the same file identity, so an unchanged settled generation file no longer needs a SQLite open for verification. The database is then opened (and pinned) lazily, only when a request misses the memo (`catalog.View`). A memo hit opens no database, writes no pin file, and parses no schema.

## Invalidation

- Pointer moves (rebuild, canonical edit with rebuild): the generation id differs, so the lookup misses and a new memo replaces the old one for that root. Memory of the old generation is released as soon as the last in-flight request drops it.
- Generation file touched, replaced or corrupted: identity differs, so verification runs in full (quick_check and metadata) as it does today. A corrupted generation never reaches the memo because the view is only created after verification passes.
- Skill file edited without rebuild (the catalog serves the published generation as fallback): a record is trusted only if every file still has the identity recorded at verification and was settled (modified more than the 5 s racy window before it was hashed) at that time. Any difference sends the request through the full read and hash path, which reports a digest mismatch as today. Files inside the racy window are never trusted, so such a skill is simply not memoized until it settles; this is the same rule `ScanCached` uses.
- Failures are never memoized: not found, not servable, digest mismatch and I/O errors rebuild on the next request.

## Pins and locks

The shared workspace lock is held from observe until `Close`, exactly as with `OpenWithFallbackLocked`, so nothing mutates canonical files between the freshness check and the reads. When a request needs the database it gets the same pinned, read-only `Handle` as before (pin file created under the lock, removed on `Close`). No pin outlives the request, so there is no long-lived pin to release on generation change or shutdown.

A long-lived `*sql.DB` per generation was considered. After the memo, only resolve (FTS search) and cold misses still open a database; the measured cost is recorded in the cook report. It is adopted only if it pays for the pin lifecycle it would add.

## Memory bound

- One memo per root, at most 8 roots (clear on overflow), as `integrityCache` does.
- At most 1024 values per memo (policy, skills, id list, plus one row and one record per skill); on overflow the memo clears and refills. Memory is `O(active skills)` and does not grow with request count.
- A record holds metadata only (paths, digests, identities), never file bytes.

## Failure modes

- Concurrent misses build the same value twice; the last writer wins, both are equal.
- A caller mutating a shared value: values are treated as immutable and slices handed out by the memo are cloned where callers may append.
- Identity collision (a write that keeps size, mtime, ctime and inode): covered by the racy-window rule above; same residual risk class as `ScanCached` and the integrity cache.
- Clock skew, network file systems: files too recent relative to hash time are not trusted, as in `ScanCached`.
