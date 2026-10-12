# Backlog items F9, F10, F11, F14 (2026-10-12)

Worktree branch `worktree-agent-af3b4f47eb9f0c2b3`, from main at `78af2c2`. Nothing pushed or tagged.

## F9: Windows hazards

- **Tests that leave a recorder open.** Every test that opens a telemetry recorder and closes it only at the end now also registers `t.Cleanup` right after the open (idempotent `Close`), so a failing test cannot leave `telemetry.db` held open while the temp directory is removed. Sites: `internal/telemetry/telemetry_test.go` (2), `internal/app` (usage, usage baseline, resolver x2, feedback), `internal/delivery/cli` (chains, baseline x2, cases, evaluation), `internal/delivery/web/fixtures_test.go`, `internal/delivery/mcpserver` (memory budget, activation telemetry). The other sites already used `defer` or `t.Cleanup`. I left the tests that block the writer on purpose through the `initializeBeforeOpen` hook alone: a cleanup `Close` there would wait on a hook that a failing test never released.
- **Production paths** that open a recorder (`mcp serve`, `resolve`, `telemetry` commands, feedback, usage) all close it with `defer` or the serve stop function. Nothing to change.
- **`stamp_windows.go` review** (not run; Windows is not available here):
  - `fileBasicInfo` matches `FILE_BASIC_INFO` (four 8-byte times, 4-byte attributes, 4 bytes padding, 40 bytes); `ChangeTime` is documented as valid for `FileBasicInfo`.
  - The file is opened through `os.Root.Open`, which Go opens with `FILE_SHARE_READ|WRITE|DELETE`, so the stamp does not block a replacement or deletion of the file; the handle is closed by `defer` before return.
  - `GetFileInformationByHandle` gives the 32-bit volume serial and the 64-bit file index (NTFS). ReFS has 128-bit ids, truncated here; the identity then relies on size, mtime and change time as well, which is acceptable.
  - Known limit: a file another process holds with a share mode that forbids reading makes the open fail, and the scan returns that error (no stale result is served).
  - NTFS updates `ChangeTime` lazily for cached writes while a handle is open; the 5 s racy window covers recent files, so a cached digest is reused only for files untouched for 5 s.
  - `GOOS=windows go vet ./...` and the Windows test binaries compile.
- **CI only (cannot be shown here):** the first run of `stamp_windows.go` against a real NTFS volume, the full telemetry and onboarding test suites on Windows, and the telemetry write-connection release on Windows (the connection closes after 5 s idle, so deletion is only blocked during activity). Run the Windows job after the push and wait for green before releasing.

## F10: integrity check per request

Decision: cache the `quick_check` result per file version, keep corruption detection when the file changes.

- `canonical.StatFile` / `FileIdentity` (`internal/canonical/file_identity.go`) expose the existing stamp (size, mode, mtime, ctime, device, inode, Windows file index) as an opaque comparable value, plus `Settled(at)` which applies the same 5 s racy rule as the canonical digest cache.
- `verifyPointerDatabase` (`internal/catalog/open.go`) now runs `quick_check` only when the generation file's identity is not in the in-process cache (`integrity_cache.go`, at most 16 files). The result is remembered only when the identity was identical before and after the check and the file was last modified more than 5 s before the check started. The metadata query that compares the generation with the pointer still runs on every request, so a mismatching or unreadable file still fails.
- Trade-off, as noted in the earlier report: damage that leaves size, mtime, ctime and inode untouched (a hardware fault or a write to the raw device) is found only when a query reads the damaged page. Any write through the file system, replacement, or metadata restore changes the identity and triggers a fresh check. The cache is per process, so a one-shot CLI command still pays one check.
- Tests (`internal/catalog/integrity_cache_test.go`): one check for four calls on a settled file; a just-written file is checked every time; after a healthy check a changed file is checked again for overwritten contents, bytes flipped with size and mtime restored, and replacement by a same-size copy through rename (errors for the first two, a pass for the copy); `Inspect` reports corrupt after a healthy check and a file change.
- Benchmark (`BenchmarkInspectWhileLocked`, 200 iterations x 3, Linux i7-13700F, generation file aged one minute; "before" is the same code with the cache lookup forced to miss):

  | case | before | after |
  |---|---|---|
  | 32 skills | 1.16-1.24 ms | 0.74-0.85 ms |
  | 300 skills | 6.6-7.3 ms | 5.9-6.9 ms |
  | 32 skills, 64 KiB bodies | 11.8-12.1 ms | 1.20-1.37 ms |

## F11

- **`RecordCase` opening a full store: unchanged.** It runs only for a disagreement case, which is opt-in and capped at 50 per day, so a full open (with `quick_check`) is rare and bounded. Sharing the writer would put a synchronous call with its own error semantics onto the writer goroutine for no measurable gain.
- **`workspace.RelativeFiles`: not changed, with evidence.** `DirEntry.Info()` on a `Root`-opened directory resolves the path outside the root, which gives up the confinement the code relies on. The confined alternative, one `Lstat` per file through a root opened on its directory, was implemented and measured: 0.61-0.65 ms to 0.60-0.65 ms (32 skills) and 5.8-6.5 ms to 5.4-5.6 ms (300 skills), at 5% more allocations. A 5 to 8% gain does not justify a rewrite, so I reverted it. I kept `internal/workspace/relative_files_test.go`: a comparison against an independent `fs.WalkDir` listing and a benchmark.
- **MCP heap-profile test:** `ageWorkspaceFiles` moves every file of the fixture a minute into the past before the resolves, so the 1,001 resolves measure the steady state and not the 5 s window after the setup. The test is gated by `SKILLHUB_PERF=1`; I could not run it here because the harness refuses a shell command that redirects `HOME`, and the test runs `skillhub init`. It compiles and skips normally.

## F14

- The Windows branch of `TestWriterProcessesOnOneStoreLoseNoEvents` already runs 4 writers plus the crashing one (commit `325373c`), with the helper pace multiplied by 4 on Windows. All assertions (dropped, errors, written count, event count, quick check, store writable after the crash) are the same on every platform. I made no further change: the backlog row predates that commit.
- **Queue size: keep 256.** One queue slot is a `request` of 456 bytes and the channel buffer is allocated at open, so 256 costs 117 KB per process and 1,024 would cost 467 KB (about 9 MB across 20 serve processes) for every process, busy or not. At the real rate (at most 10 calls per second, a few events each) 256 events is several seconds of write stall; the fan-out run in `cook-261011-telemetry-writer.md` lost 0 of 12,000 events at 20 processes, and starvation beyond the 15 s retry budget only arose in the CPU-quota emulation, where the test already throttles itself. Growing the queue would hide, not fix, a stall that long. No event type removed, `EventVersion` unchanged.

## Verification

`gofmt -l .` empty; `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=darwin go vet ./...` clean; `make lint` 0 issues; `go test -count=1 ./...` all pass; `go test -race` passes for canonical, catalog, workspace, telemetry, app, delivery/web, delivery/mcpserver, delivery/cli.

## Unresolved

- Windows CI result for F9 (first real run of `stamp_windows.go`).
- Heap-profile run with `SKILLHUB_PERF=1` needs a shell that may set `HOME`.
