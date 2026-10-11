# Catalog freshness check: cheap and never stale (2026-10-11)

Follow-up to the problem recorded in `plans/reports/cook-261011-local-shared-phase1.md` (problem 2) and in the "Phase 1 result and decision Q1" section of `plans/261011-0455-local-shared-hub-process/plan.md`.

## Outcome

- With 32 skills, the freshness check now allocates 136 KB instead of 437 KB and takes 1.2 ms instead of 2.4 ms. With 300 skills it allocates 1.06 MB instead of 3.71 MB and takes 7.0 ms instead of 18 to 21 ms.
- A whole in-process `skill_resolve` now allocates 396 KB instead of 707 KB and takes 2.8 ms instead of 4.1 ms with 32 skills. With 300 skills it allocates 2.68 MB instead of 5.33 MB and takes 12.2 ms instead of 25.7 ms.
- File contents no longer matter. With 32 skills whose bodies are 64 KiB each, the check allocated 9.79 MB before and allocates 183 KB now.
- The resolution output does not change. `ScanCached` returns the same `Snapshot` as `Scan`, and the tests compare the two with `reflect.DeepEqual` after every kind of change. The existing golden and resolver tests pass unchanged.
- No catalog format or schema change. The cache lives only in process memory, so nothing needs a migration.

## How freshness was checked before

`InspectWhileLocked` compares two digests from `canonical.Scan` with the values stored in the published pointer: `CatalogSnapshot` and `ProjectionInputDigest`. `Scan` lists the tree twice and reads and SHA-256 hashes every file twice; the second pass catches changes made during the scan. The pointer stores only the two aggregate digests, with no per-file stat data. Storing a stat fingerprint in the catalog would have meant a format change, so the fingerprint is kept in memory instead. A long-lived `mcp serve` process gets the full benefit; a one-shot CLI command pays one full read, as before.

## Design (`internal/canonical/snapshot_cache.go`)

1. On every call, the tree is listed with `workspace.RelativeFiles`. Its rules, limits, symlink refusal and sorting are the same as `Scan`'s. Every file is stamped with `Lstat` through `os.Root`. The stamp holds the size, mode, modification time in ns, change time in ns, device and inode. On Windows it holds the NTFS ChangeTime, the volume serial and the file index, read from a handle opened without reading the file.
2. The digest from an earlier call is reused only if the stamp is identical and the file is "trusted". A file is trusted when its modification time is more than `racyWindow` (5 s) before the wall-clock time at which that earlier hash started. This is Git's racy-timestamp rule. A later write must then move the modification time or the change time. A write in the same clock tick, on a filesystem with a coarse clock, or under modest NFS clock skew could keep the stamp unchanged; such a file stays inside the window and is read again on every check.
3. When every file is reused and the path count matches, the cached aggregate digests are returned. No file is opened and nothing is hashed. These digests describe the tree at the start of the call: each file kept its hashed contents from its last modification, before that moment, until it was stamped, after it.
4. Otherwise, because a file was added, removed, renamed or must be read, only the files that are not trusted are read twice, around a second listing and stamping. Any difference returns the same "changed during scan; retry" error that `Scan` returns.
5. The cache is per process and keyed by the absolute root. Each root's entry is replaced whole and never changed in place, so concurrent resolves are safe. It holds at most 16 roots.

`openWithFallback` also called `InspectWhileLocked`, which ran `verifyPointerDatabase` (SQLite open plus `PRAGMA quick_check`) a second time under the same lock. It now reuses the status it already verified. That removes one quick check per resolve. Its result is identical, because nothing can change between the two calls while the lock is held.

## Trade-offs and limits

- **Change time and file identity do the safety work.** Editing a file without changing its size and then restoring its modification time (`touch -d`, `tar`, `rsync -t`) is caught only because the change time or the inode changes. Linux, macOS and Windows provide both. On other systems (`stamp_other.go`, which is not a release target), only size, modification time and the racy window protect the cache, so a deliberately restored modification time could be missed there. The two tests that cover this fail as expected when change time and inode are removed on Linux.
- **The listing is not cached.** Skipping `RelativeFiles` by comparing directory modification times, as Git's untracked cache does, would save about 60% of what remains (about 92 KB and 0.5 ms with 32 skills). I did not do it. Directory modification times do not update reliably on every filesystem (FAT on Windows, some network filesystems), and Git keeps that cache opt-in for the same reason. It would also need a copy of the workspace's list of excluded paths, which is private to `internal/workspace`.
- **Recent edits cost about what they did before.** For 5 s after a file is written, each check reads that file twice. Files that were not touched are still reused.
- **Memory:** about one digest string plus a 48-byte stamp per canonical file per workspace.

## What remains in the check, and grows with the hub

After this change, most of the remaining time goes to `verifyPointerDatabase`, which is not part of the freshness comparison. On every request it opens the generation's SQLite file and runs `PRAGMA quick_check`, and quick check reads the whole database. With 32 small skills it is about half of the 1.2 ms. With 32 skills of 64 KiB it is 92% of 12.7 ms. That cost grows with the size of the database, not the canonical files. It is an integrity check of an immutable file, not a staleness check, so I left it alone. See open question 1.

## Measurements

Linux, i7-13700F, `go test -bench ... -benchtime=200x -count=3`, temp workspaces only. Canonical files were aged by 1 minute so they look like a hub that was not just edited. Before = `dcc24c7`, after = this branch.

| Benchmark | Before: time, bytes, allocs per op | After: time, bytes, allocs per op |
|---|---|---|
| `BenchmarkInspectWhileLocked` 32 skills | 2.39-2.52 ms, 437 KB, 7,014 | 1.20-1.28 ms, 136 KB, 2,419 |
| `BenchmarkInspectWhileLocked` 300 skills | 17.4-20.9 ms, 3.71 MB, 58,012 | 7.0-7.4 ms, 1.06 MB, 18,485 |
| `BenchmarkInspectWhileLocked` 32 skills, 64 KiB bodies | 19.1-21.4 ms, 9.79 MB, 7,988 | 12.7-12.8 ms, 183 KB, 2,423 (time is the SQLite quick check) |
| `BenchmarkSkillResolve` 32 skills | 4.03-4.18 ms, 707 KB, 9,825 | 2.72-3.04 ms, 396 KB, 5,095 |
| `BenchmarkSkillResolve` 300 skills | 25.3-26.1 ms, 5.33 MB, 70,970 | 12.2-12.5 ms, 2.68 MB, 31,300 |

Reproduce with `go test ./internal/catalog ./internal/app -run '^$' -bench 'BenchmarkInspectWhileLocked|BenchmarkSkillResolve' -benchtime=200x`.

`TestMCPHeapProfilePerformance` (with `SKILLHUB_PERF=1`, isolated `HOME` and `XDG_*`) still passes. Its fixture writes every file right before the 1,001 resolves, and the run lasts about 6 s, so most of those resolves fall inside the 5 s racy window and take the old, slower path. I did not edit that MCP server test. Use the benchmarks above for before and after numbers.

## Tests added

- `internal/canonical/snapshot_cache_test.go`
  - Every kind of change is detected, and the result equals `Scan`: an edit that keeps the size and restores the modification time, an edit that keeps the size within the same second, a replacement through rename that keeps the size and modification time, and adding, deleting or renaming a file or a directory.
  - A reverted edit gives back the original snapshot.
  - The racy rule: a wrong digest is planted under the current stamp. It is reused only when the file was modified more than the window before the hash, and it is read again at the window boundary and inside it.
  - Concurrent modification during a scan is rejected, symlinks are rejected like `Scan`, and concurrent callers are safe (under `-race`) and cannot change the cache by editing the slice they get back.
- `internal/catalog/freshness_test.go`: after each change, `Inspect` reports `stale` on the next check and `healthy` again once the change is undone. `OpenCurrent` refuses the catalog and `OpenWithFallback` rebuilds it after an edit that keeps the size.
- The mutation checks fail as expected: always trusting a file fails the racy tests, and removing change time and inode fails the metadata-restore tests.

## Verification

- `make check` passes: fmt-check, vet, lint with 0 issues, and the full test suite.
- `go test -race` passes for `internal/canonical`, `catalog`, `app`, `resolver`, `delivery/mcpserver` and `mutation`.
- `go vet` passes for `GOOS=windows`, `darwin`, `freebsd` and `GOARCH=386`, and the Windows test binary compiles. The Windows code path (`stamp_windows.go`) has not been run on Windows; the release workflow's Windows job will be its first run.
- Golangci-lint was not run with `GOOS=windows`, because the `go run` invocation cannot cross-compile the linter itself.

## Files

- `internal/canonical/snapshot_cache.go` (new): `ScanCached` and the stamp cache
- `internal/canonical/stamp_linux.go`, `stamp_darwin.go`, `stamp_windows.go`, `stamp_other.go` (new): change time and file identity for each platform
- `internal/canonical/canonical.go`: `newSnapshot`, `contentDigest` and `errChangedDuringScan` moved into shared helpers; `Scan` behaves the same
- `internal/catalog/open.go`: `InspectWhileLocked` uses `ScanCached`; `openWithFallback` no longer verifies the pointer database twice
- Tests: `internal/canonical/snapshot_cache_test.go`, `internal/catalog/freshness_test.go`, `internal/catalog/freshness_benchmark_test.go`, `internal/app/resolve_benchmark_test.go`

## Open questions

1. Should `verifyPointerDatabase` stop running `PRAGMA quick_check` on every request? It is now the largest part of the check, and it grows with the size of the database. One option is to cache "verified" per generation file, keyed by the same kind of stamp, because a published generation is immutable and is replaced through rename. The cost is that corruption on disk that leaves the file's metadata unchanged would be caught by SQLite only when a query reads a damaged page, not by an up-front check on every request. This is a product decision about integrity, not about staleness.
2. Should `workspace.RelativeFiles` take the stat data from the directory entries it already reads, instead of calling `os.Root.Lstat` again for each path (each call opens every directory in the path)? That would make the listing, now the largest part of the scan, roughly twice as cheap. It is in `internal/workspace`, outside this task's files.
3. Should the MCP memory fixture age its files, so its numbers show the steady state and not the 5 s after setup?
