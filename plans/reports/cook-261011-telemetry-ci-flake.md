# `TestWriterProcessesOnOneStoreLoseNoEvents` fails on CI (2026-10-11)

Follows `cook-261011-telemetry-writer.md`. Only `internal/telemetry` changed.

## Outcome

It is a real product bug, and the test also had a speed-dependent part.

- **Product bug:** an event batch that waited longer than one operation timeout (5 s) for the SQLite write lock was dropped after that single attempt, although the store was healthy and a second attempt would have succeeded. On a CPU-starved machine, ten writers on one store keep a waiting writer off the lock for that long. The kill step plays no part.
- **Fix:** contended store writes are retried with backoff within a bounded budget of three operation timeouts. This covers event batches, initialization, and the maintenance run by flush, health and close. A batch that hits `SQLITE_BUSY` keeps its write connection. A batch that committed but whose size trim afterwards failed now counts as written.
- **Test:** each writer helper now holds back while 8 events are already queued. A slow machine makes the test take longer, but it can no longer overflow a writer's queue. The assertions stay as strict as before: every writer writes all 400 events, with no drops and no errors.

## Reproduction and evidence

All runs used temp dirs on ext4 outside the repo, an isolated `HOME`/`XDG_*`, and `GOMAXPROCS=2`.

**`taskset -c 0,1` alone does not reproduce the failure on this machine.** With the original code, it passed 14 of 14 race runs. That includes two busy-loop CPU burners pinned to the same cores, a single core, and tmpfs vs ext4. One fast core is still fast. To emulate slow CI vCPUs I added a cgroup CPU quota (`systemd-run --user --scope -p CPUQuota=N% taskset -c 0,1 ...`):

| original code, original test, `-race` | result | test time |
|---|---|---|
| `CPUQuota=50%` | 3/3 pass | 17 s |
| `CPUQuota=30%` | **0/6 pass** | 25 s |

CI took 13.6 s on Linux and 17.7 s on Windows, against 10.4 s on this machine without a quota.

To see what failed, I added temporary logging to a scratch copy. None of it is committed. It recorded the batch error, the phase that failed, how long the batch took, and the kill time:

- Every failure was `context deadline exceeded`. It happened in the **first INSERT** of the batch, on a **reused** connection (connection acquired in about 30 µs), after waiting 5.0 to 5.4 s for the write lock.
- The failures came 10 to 20 s after the crashing writer was killed, and well after every writer had started producing. Before any failure, batches routinely waited 1 to 4.7 s (`SLOWBATCH n=1 took=3.4s`).
- The lock is not held by anything abnormal. Under the quota, a 64-event batch holds the lock for 0.5 to 1.7 s. With ten writers, one writer's turn can take more than 5 s, and SQLite's busy handler is not fair: a long waiter polls at 100 ms intervals while newcomers grab the lock.
- The CI health lines fit the same pattern. On Linux, each affected writer lost exactly one batch (`Accepted - Written` = 16, 6, 64, 8, 1, with `Errors:1`), of varying size, which matches the reproduction. On Windows, writer 0 shows `Errors:1` with nothing lost, which means a non-batch failure, most likely initialization timing out while eleven processes start. Initialization is now retried too.

**Candidates ruled out by the logs:** the killed writer (no failures near the kill, and the store passed `quick_check` afterwards), WAL recovery, the `quick_check`/5-minute reverify (failures happened on reused connections), `trimLogicalSize` (it was never the failing phase), and start-up contention (all writers were producing long before the failures).

**Two facts found along the way shaped the fix:**

1. SQLite's busy wait does not observe context cancellation. With `busy_timeout=5000` and a 150 ms context, a statement still waited 5.0 s. So the busy timeout, not the context, bounds each attempt. It now equals the operation timeout. The default stays 5000 ms, so nothing changes for production.
2. When a context ends a transaction, database/sql discards the connection, because modernc's `ResetSession` refuses an interrupted connection. A retry after an interrupt therefore goes through a full reopen, and the reopen's schema statements need the same write lock. Each retry attempt is therefore allowed a fifth longer than the busy wait, so contention ends the attempt with `SQLITE_BUSY` while the connection is still usable.

## What changed

- `store.go`
  - `retryContended(parent, config, attempt)`: retries while the error is `SQLITE_BUSY` or the attempt ran out of time. The backoff goes from 5 ms to 100 ms. The total is capped at `writeRetryBudget` (3) times the operation timeout, or earlier if the caller's context ends. Any other error returns at once.
  - `writeEventsRetrying` and `maintainStoreRetrying` wrap it.
  - `writeEvents` keeps the connection on `SQLITE_BUSY` from a statement before COMMIT. It releases the connection for any other error: a deadline, a failed COMMIT (SQLite can leave that transaction open), or a non-busy error.
  - `busy_timeout` now follows `OperationTimeout`.
  - A trim failure after a successful commit is wrapped as `errBatchCommitted`.
- `recorder.go`
  - Batches go through `writeEventsRetrying`.
  - Flush, health and close run `maintainStoreRetrying`, bounded by the caller's context.
  - A batch with `errBatchCommitted` counts as written and records an error, instead of counting stored events as dropped.
- Re-writing a batch is safe: inserts use `INSERT OR IGNORE` on the event ID, and only an inserted row updates its rollups.
- No change to path confinement, the regular-file check, the 0600 mode, `quick_check` at open, the schema, event types or `EventVersion`.

Worst case: the recorder goroutine can now spend up to 15 s on one contended batch, where it used to spend 5 s. `Record` never blocks, so events offered meanwhile either queue or are counted as `Dropped`. `Flush`/`Close`/`Health` callers still return at their own deadlines.

## Tests

- New `store_retry_test.go`. Each test takes the write lock with `BEGIN IMMEDIATE` from a second connection, so the tests are deterministic and need no timing guesses:
  - A batch blocked past one operation timeout is written by a later attempt, and the retry reuses the connection (one open).
  - A lock held past the budget fails after about the budget.
  - A non-contention error is not retried.
  - A recorder keeps an event through 600 ms of contention with a 250 ms operation timeout: `Errors` = 0, `Dropped` = 0.
- `store_concurrency_test.go`: the closed-loop producer (`writerHelperBacklog` = 8) and a 5-minute parent deadline. The assertions are unchanged and strict.

The new test alone does not hide the bug. The original product code with the new closed-loop test, at `CPUQuota=30%` with `-race`, passed only **5/8** runs, each failure being a lost batch with `context deadline exceeded`. The fixed code with the new test passed 8/8 in the same conditions before the final matrix below.

## Verification

Final matrix, fixed code, 20 runs each (`-test.count=20`, `taskset -c 0,1`, `GOMAXPROCS=2`):

| condition | result | test time |
|---|---|---|
| `-race` | 20/20 pass | about 10.6 s each |
| no race | 20/20 pass | about 2.2 s each |
| `CPUQuota=30%`, no race | 20/20 pass | 3.0 to 11.5 s |
| `CPUQuota=30%`, `-race` | 20/20 pass | 33 to 77 s (it was 0/6 before the fix) |

These binaries were compiled just before a lint-only rewrite of one boolean condition in `retryContended`, which applies De Morgan's law and does not change behaviour. All the checks below ran on the final code.

- `go test -race -count=2 ./internal/telemetry ./internal/delivery/mcpserver`: pass (44 s, 189 s).
- `make check` (fmt-check, vet, lint with 0 issues, full test suite): pass.
- `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...`: pass.
- Every process I started has exited: the test binaries, writer helpers and CPU burners. No real hub was touched, and the temp dirs live outside the repo.

## Not verified

- **Windows.** I could not run the test there. The Windows CI failure has the same shape (a 17.7 s run, one lost batch per writer, plus one writer with an error and no loss, which points at initialization). Both paths are now retried, but the next Windows CI run is the real check.
- The quota emulation is harsher than CI: cgroup throttling freezes the lock holder too. It is still possible that CI has another stall source that I did not see.

## Open questions

- Should SQLite's unfair busy wait be addressed in the product? For example, a short cross-process write queue, or taking the lock with `BEGIN IMMEDIATE` and retrying in-process with short sleeps instead of one long busy wait. At real `mcp serve` rates (at most 10 calls/s per process, mostly single-event batches) contention stays far below what is needed to starve a writer, so I left it as it is.
- `RecordCase`, preview, export and the other read paths still make one attempt. They are synchronous, return their errors to the caller, and do not lose accepted events.
