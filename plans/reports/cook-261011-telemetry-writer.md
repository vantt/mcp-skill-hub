# Telemetry writer under many `mcp serve` processes (2026-10-11)

Follows the "pre-existing problem 1" in `cook-261011-local-shared-phase1.md`. Only `internal/telemetry` changed.

## Outcome

- **No events lost.** `make mcp-fanout` (20 processes, 30 s, temp workspace), three runs at each rate:

  | rate per process | run 1 | run 2 | run 3 |
  |---|---|---|---|
  | 10/s | 12000/12000, 0 lost | 12000/12000, 0 lost | 12000/12000, 0 lost |
  | 1/s | 1200/1200, 0 lost | 1200/1200, 0 lost | 1200/1200, 0 lost |

  Before the change, the same driver lost 2 of 12000 on both baseline runs I made.
- **Each batch is much cheaper.** The new Go benchmarks run on a store that already holds 12,000 events and write one event per batch (`go test -run '^$' -bench SingleEventBatch -benchtime 3s ./internal/telemetry/`):

  | benchmark | before | after |
  |---|---|---|
  | one recorder, time per batch | 10.7 ms | 0.76 to 0.83 ms |
  | 20 recorders on one store, wall time per batch | 12.5 ms | 0.28 to 0.56 ms |
  | 20 recorders, mean wait for one's own batch | 164 ms | 5.2 to 9.7 ms |

- **Side effect:** total working memory for 20 processes at 10/s dropped from 737 MB RSS / 366 MB PSS to 677 MB RSS / 305 MB PSS, because there are far fewer reopens. Resolve latency in the driver is too noisy to draw a conclusion from.

## What changed

1. **The write connection stays open across batches** (`writerConnection` in `store.go`). Before reusing it for a batch, the writer revalidates the anchored directory through the existing `ensureCurrent`. The database entry must still be the same regular file the connection opened, with mode 0600 (the mode check is skipped on Windows). Any mismatch closes the connection and falls back to the full `openDatabase`. Holding the file open also stops its inode from being reused, so a recreated file cannot pass as the original.
2. **The connection is closed** after any failed batch, before every administrative operation (flush, health, purge, close, reads, cases), after 5 s with no queued work (so an idle process holds no handle; on Windows this also keeps the file deletable), and after it has been in use for 5 minutes.
3. **Corruption detection is kept.** `quick_check` and the schema statements still run on every full open. That covers start-up, every admin operation, every reopen after an identity change, idle period or failed batch, and at least once every 5 minutes while a process stays busy.
4. **A busy open is retried.** When several processes create the store or switch it to WAL at the same moment, SQLite can return `SQLITE_BUSY` without waiting on `busy_timeout`. The new multi-process test reproduced this during initialization. `openDatabase` now retries a busy open with backoff (5 ms up to 100 ms), stopping at the context deadline or the operation timeout.
5. **The full-table size sum is skipped when it cannot be over the limit.** `trimLogicalSize` summed the length of every stored event on every batch. Every counted TEXT byte lives in a database page, so if `page_size * page_count` is at most the maximum, the logical size is too, and the sum is skipped. With the 100 MB default this skips it almost always.
6. **Hardening:** `openDatabase` now refuses an existing database entry that is not a regular file, for example a symlink placed after the store was anchored. Before, such an entry was followed.

No schema change, no migration, no event type removed, and `EventVersion` is unchanged.

## Safety properties

All existing tests pass unchanged, including `store_confinement_test.go`, `TestDeleteAndRecreateDatabase`, `TestRecorderRecoversAfterRuntimeDirectoryIsRecreated`, `TestPurgeIsAnchoredAgainstParentSwapAndFaults` and `TestDatabaseCorruptionDegradesWithoutRecordError`. Keeping the connection open did not conflict with any property those tests state. New tests:

- `store_concurrency_test.go`:
  - In-process: 8 recorders on one store.
  - Multi-process: 10 writer processes (the test binary started again as a helper) while another writer is killed mid-stream. The test requires that no event is dropped, that no write fails (a write that waits past the operation timeout fails), that `quick_check` passes after the crash, and that the store is still writable. The multi-process test fails on the old code (initialization `SQLITE_BUSY` and dropped events) and passes on the new code.
  - The two benchmarks above.
- `store_writer_test.go`: the connection is reused, it reopens when the file is replaced, when its permissions change and after the reverify interval, it refuses a symlinked entry (the outside file stays untouched), and it is released after an idle period.
- `race_detector_on_test.go` / `race_detector_off_test.go`: helper pacing. The race detector slows SQLite many times over, so instrumented writers produce events more slowly.

## Verification

- `make check`: fmt-check, vet, lint (0 issues) and all tests pass.
- `go test -race ./internal/telemetry ./internal/delivery/mcpserver` passes, and telemetry also passes with `-race -count=3`.
- `go test -count=5 ./internal/telemetry` passes.
- Every process I started (fan-out servers, writer helpers) has exited. No real hub was touched.

## Open questions

- `RecordCase` still does a full open and closes the write connection. It runs only for disagreement cases, which are opt-in and capped at 50 a day, so I left it alone. Should it share the writer?
- Windows: while a process is writing (until 5 s after its last event), the open connection blocks deleting `telemetry.db`, as SQLite already does during a write. The fan-out driver does not run on Windows, so these Windows numbers are not measured.
