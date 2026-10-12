# Per-generation memo: results

Date: 2026-10-12. Branch `worktree-agent-a786605e32f8189d1`, from main `6688bf3`. Nothing pushed or tagged. Design: `design-261012-per-generation-memo.md`.

## What changed

| Area | Change |
|---|---|
| `catalog.View` (new) | Observes the current generation under the shared workspace lock without opening its database. `View.DB` opens and pins it on the first request that needs it; `Close` removes the pin and releases the lock. `ObserveWithFallbackLocked` and `ObserveCurrentLocked` mirror the two existing `Open*Locked` entry points. `OpenWithFallback*` now run on the same observe step, so the freshness, rebuild and fallback logic exists once. |
| Generation memo (new) | `catalog.GenerationMemo`, one per root, keyed by generation id plus generation file identity, replaced when the pointer moves, capped at 8 roots and 1024 values. `MemoGet`, `MemoPut`, `Memoized` give typed access. |
| Pointer verification | The metadata comparison of `verifyPointerDatabase` is remembered with the quick_check result for an unchanged file, so verification no longer opens SQLite. |
| Freshness digests | The view keeps the digests the freshness scan computed (`View.CanonicalDigest`). Files nobody asked to read are verified against them instead of being read and hashed. |
| `internal/app` | `generation_memo.go` memoizes the policy, the decoded resolver skills, the active id list, skill rows and the `DistributedSkill` manifests. `DistributionService`, `ReadSkillCore`, resolve, runtime status and routing evaluation read through the view. `SnapshotService.Ensure` works from the manifest and loads file contents only when it must export (`loadDistributedSkillManifest`). The bundled curator entry is built once. |
| Skill snapshot touches | A reused snapshot or state directory is touched only when its mtime is older than 5 minutes (GC grace periods are hours and days). |
| Freshness scan (`internal/workspace`, `internal/canonical`) | The listing takes one lstat per file from its own directory and `stampTree` and the size limits reuse it (it stat every file three times through root-relative paths). Subdirectories are opened from their parent. |
| Test | `TestProxyFetchesWholeSkillsOnceAndServesThemFromTheCache` raced the hub's background load confirmation against the sampling of the request counter. It passed only because the hub was slower than the client path; with the faster hub it failed under `-race` 6/6. It now waits for the background task and asserts that no bundle bytes were downloaded, which is what the message always said. Only edit under `internal/delivery/mcpserver`. |

## Measurements

Before is main `6688bf3` (a copy of the tree with the same benchmark files), after is this branch, alternating runs on a shared machine, so read ranges. Isolated HOME, XDG_*, TMPDIR, `SKILLHUB_USER_CONFIG_DIR`, `TZ=Asia/Saigon`.

`BenchmarkLoadPaths` (`-benchtime 300x -count 3`, in-memory MCP session, telemetry recorder, about 190 KB skill):

| Request | Before | After |
|---|---|---|
| `skill_get` warm | 3.14-3.21 ms, 1.52 MB, 7.45k allocs | **1.68-2.04 ms**, 1.19 MB, 5.5k allocs |
| `skill_get` first call of a session | 4.40-4.81 ms, 2.0 MB | **2.27-2.59 ms**, 1.7 MB |
| `resources/read` (server handler) | 2.70-2.78 ms, 722 KB, 4.24k allocs | **0.39-0.45 ms**, 159 KB, 1.88k allocs |
| `skills/get` | 2.76-3.26 ms, 1.13 MB | **0.64-0.81 ms**, 0.56 MB |
| `skill_resolve` over MCP | 1.41-1.61 ms | **1.00-1.06 ms** |

`BenchmarkSkillResolve` in process (`-benchtime 100x -count 3`):

| Skills | Before | After |
|---|---|---|
| 32 | 2.32-2.48 ms, 400 KB, 5.0k allocs | **1.01-1.23 ms**, 235 KB, 2.7k allocs |
| 300 | 9.3-10.6 ms, 2.73 MB, 31.4k allocs | **3.6-4.1 ms**, 1.11 MB, 11.8k allocs |

`BenchmarkDistribution` (new, `internal/app/distribution_benchmark_test.go`, service calls behind skills/list, skills/get and resources/read; hub of N skills with five files each, unchanged generation; `-benchtime 100x -count 3`):

| Call | 32 skills before | 32 skills after | 300 skills before | 300 skills after |
|---|---|---|---|---|
| list | 5.9-6.9 ms | **0.56-0.73 ms** | 84-93 ms | **4.8-5.6 ms** |
| get | 2.19-2.29 ms | **0.74-0.82 ms** | 18.3-19.2 ms | **4.7-5.5 ms** |
| read (one file) | 2.18-2.31 ms | **0.83-0.84 ms** | 16.7-21.2 ms | **4.3-4.8 ms** |
| `ReadSkillCore` | 2.10-2.13 ms | **0.59-0.67 ms** | 18.0-26.5 ms | **4.5-5.2 ms** |
| `Ensure` (snapshot already exported) | 2.55-3.03 ms | **0.68-0.74 ms** | 19.0-23.7 ms | **4.6-5.1 ms** |

Memory budget (`SKILLHUB_PERF=1`, 32 skills, 1000 resolves over stdio, 20 s idle): RSS start 28.7 -> 28.8 MB, after 1001 resolves 40.2 -> 40.1 MB, after idle 38.9 -> 39.1 MB. Resolve p50 5.6 -> 3.1 ms, p95 10.6 -> 5.6 ms. Heap in use after the resolves 6.0 MB in both. Memory did not move.

Syscalls and writes per warm `skill_get` (strace of `BenchmarkLoadPaths/skill_get_warm`, 100 calls minus 0 calls): `openat` 527 -> 161, `newfstatat` 257 -> 167, `unlink` 2 -> 0 (pin removals), `utimensat` 2 -> 0, write-mode `openat` 4 -> 2 (the two left are telemetry events, not catalog or snapshot).

## Targets

- `skills/list` and `skills/get` well under 1 ms: met for a hub of 32 skills (0.56-0.82 ms at the service, 0.64-0.81 ms over MCP). Not met at 300 skills (4-5 ms): what is left is the freshness scan, one lstat per canonical file (1,800 files here, about 2.5 us each). It is the check that makes a stale or edited hub visible; removing it needs a file watcher, which is out of scope.
- `resources/read` costs one file: yes. A hit stats nothing beyond the scan, reads and hashes the requested file once, and opens no database (0.39-0.45 ms).
- Resolve time not growing with skill count: partly. The per-skill decode (12 ms at 300 skills in the review) is gone. What still grows is the scan, about 2.5 us per file, and the in-memory candidate loops: 1.0-1.2 ms at 32 skills, 3.6-4.1 ms at 300 (was 2.4 and 10).
- No extra disk writes on read-only requests: no pin file, no mtime rewrite (see syscall counts); only telemetry events remain.

## Skipped, with numbers

- Long-lived read-only `*sql.DB` per pinned generation: not adopted. After the memo, a resolve opens one database. From the CPU profile, `View.DB` (open and ping) is about 0.23 ms of a 1.1 ms resolve at 32 skills and the first-query schema parse adds roughly 0.1 ms, so at best 20-30% of a 1 ms call (about 6-9% at 300 skills). It would add a shared handle that needs reference counting between concurrent requests, a pin held for the life of the process released on generation change and on shutdown (the shutdown hook lives in `internal/delivery/mcpserver`, off limits for this work), and page cache resident for the whole life of the server. Not worth it for 0.2-0.3 ms.
- Merging the two observes in `skill_get` (`ReadSkillCore`, then `Ensure`): each costs one freshness scan (about 0.25 ms at one skill). It needs the handler in `internal/delivery/mcpserver` to share one view; left to the cleanup item there.

## Invariants

- No mutation between open and read: the view holds the shared workspace lock until `Close`, like `OpenWithFallbackLocked`; every served byte is read and hash-verified under it.
- Pins: a pin exists only while a request has the database open and is removed on `Close`; no cached handle holds one, so nothing to release on generation change or stop.
- Rebuild, edit, corruption: tests below. The existing freshness, integrity and hardening tests pass unchanged.
- Memory: `TestMemoMemoryDoesNotGrowWithGenerationsOrValues` (24 rebuilds, 1,124 values each: one memo per root, at most 1,024 values) and `TestMemoStoreIsBoundedAcrossWorkspaces` (12 workspaces: at most 8 roots).
- Telemetry: no event type or `EventVersion` change.

## Tests added

- `catalog/generation_memo_test.go`: memo shared while the generation is unchanged with 0 database opens; observing writes no pin until `DB`; rebuild replaces the memo; a touched generation file gets a fresh memo; a corrupted generation is rebuilt and not served from its memo; memory bounds; `CanonicalDigest`; pointer metadata compared once per file version and a different pointer still rejected.
- `app/generation_memo_test.go`: repeated list, get, read, `ReadSkillCore` and `Ensure` on an unchanged generation open no database and write no pin; a rebuild invalidates the manifest and serves the edited file; an edited file is noticed while the published generation is served as fallback (digest mismatch, skipped in the listing); a corrupted generation is detected after the memo filled; memoized policy and resolver skills equal the database ones.
- `app/skill_snapshot_touch_test.go`: reuse of a recent snapshot writes nothing.

## Checks

`gofmt -l .` clean; `go vet ./...` clean for linux, `GOOS=windows`, `GOOS=darwin`; `make lint` 0 issues (funlen 120); `go test -count=1 ./...` all packages pass; `go test -race` passes for app, catalog, canonical, workspace, resolver, skill and mcpserver.

## Notes for the caller

- `catalog.PinnedOpens()` is exported for tests that tell a memo hit from a miss.
- `workspace.StatFiles` is new; `RelativeFiles` is a wrapper over it. A file that vanishes while its directory is listed is now skipped instead of failing the scan.
- Values in the memo are shared and read-only. `ReadSkillCore` hands out a copy of the resource list; routing slices are shared and only encoded.
- Files owned by the parallel work (`server.go`, `skill_tools.go`, `recordLoad`, schema walking) were not touched.
