package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// A batch that waits for the write lock longer than one operation timeout is
// written by a later attempt instead of being dropped, as long as the lock is
// released within the retry budget.
func TestBatchWaitingPastOperationTimeoutIsRetried(t *testing.T) {
	config := newWriterTestConfig(t)
	// Windows runners are slow enough that the fixed time windows leave no room for the retries, so the
	// timeout and the moment the lock is released scale together and keep their ratio.
	scale := time.Duration(1)
	if runtime.GOOS == "windows" {
		scale = 6
	}
	config.OperationTimeout = 250 * time.Millisecond * scale
	opens := countSQLiteOpens(t)
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_retry_seed")

	release := holdWriteLock(t, config.Path)
	time.AfterFunc(600*time.Millisecond*scale, release)
	attempts := 0
	err := retryContended(context.Background(), config, func(ctx context.Context) error {
		attempts++
		return writeEvents(ctx, config, &writer, []storedEnvelope{testEnvelope(t, "evt_retry_late")})
	})
	if err != nil {
		t.Fatalf("contended write failed within the retry budget after %d attempts: %v", attempts, err)
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d; want the first attempt to time out on the held lock", attempts)
	}
	// The busy wait ends before the deadline, so the contended attempt reports
	// SQLITE_BUSY and the retry reuses the connection instead of reopening.
	// Windows runners are slow enough for the deadline to fire inside the busy
	// wait, which makes the driver discard the connection, so only the outcome
	// (the event is stored once) is asserted there.
	if runtime.GOOS != "windows" && opens.Load() != 1 {
		t.Fatalf("opens = %d; want the retry to reuse the write connection", opens.Load())
	}
	if count := countEventsWithPrefix(t, config.Path, "evt_retry_late"); count != 1 {
		t.Fatalf("stored retried events = %d, want 1", count)
	}
}

// A lock that outlasts the whole retry budget still fails the write, so a
// stuck store cannot hold the recorder goroutine indefinitely.
func TestBatchContendedPastRetryBudgetFails(t *testing.T) {
	config := newWriterTestConfig(t)
	config.OperationTimeout = 150 * time.Millisecond
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_budget_seed")

	release := holdWriteLock(t, config.Path)
	defer release()
	started := time.Now()
	attempts := 0
	err := retryContended(context.Background(), config, func(ctx context.Context) error {
		attempts++
		return writeEvents(ctx, config, &writer, []storedEnvelope{testEnvelope(t, "evt_budget_lost")})
	})
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("write succeeded while another connection held the write lock")
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d; want retries before giving up", attempts)
	}
	if budget := writeRetryBudget * config.OperationTimeout; elapsed < budget || elapsed > budget+5*time.Second {
		t.Fatalf("gave up after %v; want about the retry budget %v", elapsed, budget)
	}
}

// Only contention is retried: any other failure returns at once.
func TestRetryContendedReturnsOtherErrorsAtOnce(t *testing.T) {
	config := Config{OperationTimeout: time.Second}
	failure := errors.New("unsafe telemetry database path")
	attempts := 0
	err := retryContended(context.Background(), config, func(context.Context) error { attempts++; return failure })
	if !errors.Is(err, failure) || attempts != 1 {
		t.Fatalf("err = %v after %d attempts; want the failure after one attempt", err, attempts)
	}
}

// The recorder keeps an accepted event through lock contention that lasts
// longer than one operation timeout, and reports neither an error nor a drop.
func TestRecorderKeepsEventsThroughTransientLockContention(t *testing.T) {
	workspace := t.TempDir()
	// The lock is released 150 ms before the retry budget ends. A slow Windows
	// runner needs longer than that for the first write after the release, so
	// the timeout and the release moment scale together there and keep their
	// ratio while the margin grows.
	scale := time.Duration(1)
	if runtime.GOOS == "windows" {
		scale = 6
	}
	config := Config{WorkspaceRoot: workspace, Path: filepath.Join(workspace, "runtime", "telemetry.db"), OperationTimeout: 250 * time.Millisecond * scale}
	recorder := newTestRecorder(t, config)
	mustFlush(t, recorder)

	release := holdWriteLock(t, recorder.config.Path)
	time.AfterFunc(600*time.Millisecond*scale, release)
	event := validEvent(EventResolutionCompleted)
	event.ID = "evt_contended"
	recorder.Record(event)
	if err := waitWritten(recorder, 1); err != nil {
		t.Fatal(err)
	}
	// On a slow runner the size trim after the commit can itself run out of
	// time: the event is stored and counted as written, and the failed trim is
	// reported as an error. That is the only error tolerated here.
	health := recorder.healthSnapshot()
	trimOnly := health.Errors == 0 || (health.Errors == 1 && strings.Contains(health.LastError, "committed; trim telemetry store"))
	if health.Dropped != 0 || !trimOnly {
		t.Fatalf("health = %+v; want no error and no drop", health)
	}
	if count := countEventsWithPrefix(t, recorder.config.Path, "evt_contended"); count != 1 {
		t.Fatalf("stored events = %d, want 1", count)
	}
}

// holdWriteLock takes the store's write lock from a separate connection and
// returns a function that releases it. The release is idempotent and also runs
// at cleanup.
func holdWriteLock(t *testing.T, path string) func() {
	t.Helper()
	database, err := sql.Open("sqlite", sqliteURL(path))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.Conn(context.Background())
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		_ = connection.Close()
		_ = database.Close()
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
			_ = connection.Close()
			_ = database.Close()
		})
	}
	t.Cleanup(release)
	return release
}
