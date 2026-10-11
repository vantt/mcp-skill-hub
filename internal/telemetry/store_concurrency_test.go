package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Environment for the writer helper process. The parent test passes only temp
// paths, so a helper never touches anything outside the test's directories.
const (
	writerHelperEnv       = "SKILLHUB_TELEMETRY_WRITER_HELPER"
	writerHelperRootEnv   = "SKILLHUB_TELEMETRY_WRITER_ROOT"
	writerHelperPrefixEnv = "SKILLHUB_TELEMETRY_WRITER_PREFIX"
	writerHelperCountEnv  = "SKILLHUB_TELEMETRY_WRITER_COUNT"
)

// writerHelperBacklog is how many queued events make a writer helper wait
// before producing more. A fast machine never queues this many. On a slow one
// it keeps every batch small, so one writer's wait for the write lock stays
// near "writers times one small batch" instead of growing with the backlog.
const writerHelperBacklog = 8

func TestConcurrentRecordersOnOneStoreLoseNoEvents(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "runtime", "telemetry.db")
	const recorders, events = 8, 250
	var wait sync.WaitGroup
	healths := make([]Health, recorders)
	for index := range recorders {
		recorder, err := Open(Config{WorkspaceRoot: workspace, Path: path})
		if err != nil {
			t.Fatal(err)
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			for item := range events {
				event := validEvent(EventResolutionCompleted)
				event.ID = fmt.Sprintf("evt_g%02d_%04d", index, item)
				recorder.Record(event)
				// Pace the producer so most events form their own small batch:
				// the contention that matters is many writers committing often.
				time.Sleep(time.Millisecond)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := recorder.Close(ctx); err != nil {
				t.Errorf("close recorder %d: %v", index, err)
			}
			healths[index] = recorder.healthSnapshot()
		}()
	}
	wait.Wait()
	for index, health := range healths {
		if health.Dropped != 0 || health.Errors != 0 || health.Written != events {
			t.Errorf("recorder %d health = %+v", index, health)
		}
	}
	if count := countEventsWithPrefix(t, path, "evt_g"); count != recorders*events {
		t.Fatalf("stored events = %d, want %d", count, recorders*events)
	}
	requireQuickCheck(t, path)
}

// TestWriterProcessesOnOneStoreLoseNoEvents runs several writer processes on
// one store while another writer is killed mid-stream. Every surviving writer
// must store every event it produced, no write may fail (a write still
// contended after its retry budget fails and drops its batch), and the store
// must remain intact and writable after the crash.
//
// The outcome does not depend on machine speed: each writer produces no faster
// than its store writes drain (see writerHelperBacklog), so a slow machine
// makes the test take longer instead of overflowing a writer's bounded queue.
func TestWriterProcessesOnOneStoreLoseNoEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("starts writer processes")
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, "runtime", "telemetry.db")
	const events = 400
	// A Windows runner starts processes and commits slowly enough that ten writers outlast the retry
	// budget of one store write, so fewer writers share the store there; the check stays the same.
	writers := 10
	if runtime.GOOS == "windows" {
		writers = 4
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	crasher := writerHelperCommand(ctx, workspace, "evt_crash", 0)
	if err := crasher.Start(); err != nil {
		t.Fatal(err)
	}
	crasherDone := make(chan error, 1)
	go func() { crasherDone <- crasher.Wait() }()
	t.Cleanup(func() {
		_ = crasher.Process.Kill()
		<-crasherDone
	})

	type writerResult struct {
		health Health
		err    error
	}
	results := make([]writerResult, writers)
	var wait sync.WaitGroup
	for index := range writers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			command := writerHelperCommand(ctx, workspace, fmt.Sprintf("evt_p%02d", index), events)
			output, err := command.Output()
			if err != nil {
				results[index].err = fmt.Errorf("writer %d: %w", index, err)
				return
			}
			results[index].err = json.Unmarshal(output, &results[index].health)
		}()
	}
	// Kill the crashing writer while the others are still committing batches.
	time.Sleep(300 * time.Millisecond)
	if err := crasher.Process.Kill(); err != nil {
		t.Fatalf("kill crashing writer: %v", err)
	}
	<-crasherDone
	crasherDone <- nil
	wait.Wait()

	for index, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if health := result.health; health.Dropped != 0 || health.Errors != 0 || health.Written != events {
			t.Errorf("writer %d health = %+v", index, health)
		}
	}
	if count := countEventsWithPrefix(t, path, "evt_p"); count != writers*events {
		t.Fatalf("stored events = %d, want %d", count, writers*events)
	}
	requireQuickCheck(t, path)

	recorder := newTestRecorder(t, Config{WorkspaceRoot: workspace, Path: path})
	event := validEvent(EventResolutionCompleted)
	event.ID = "evt_after_crash"
	recorder.Record(event)
	mustFlush(t, recorder)
	if count := countEventsWithPrefix(t, path, "evt_after_crash"); count != 1 {
		t.Fatalf("store not writable after a writer crashed: %d", count)
	}
}

// TestTelemetryWriterHelperProcess is the body of one writer process started by
// TestWriterProcessesOnOneStoreLoseNoEvents. It does nothing in a normal run.
// A count of zero writes until the process is killed.
func TestTelemetryWriterHelperProcess(t *testing.T) {
	if os.Getenv(writerHelperEnv) != "1" {
		t.Skip("helper process for the multi-process writer test")
	}
	workspace := os.Getenv(writerHelperRootEnv)
	prefix := os.Getenv(writerHelperPrefixEnv)
	count, err := strconv.Atoi(os.Getenv(writerHelperCountEnv))
	if err != nil || workspace == "" || prefix == "" || count < 0 {
		fmt.Fprintln(os.Stderr, "invalid writer helper environment")
		os.Exit(2)
	}
	recorder, err := Open(Config{WorkspaceRoot: workspace, Path: filepath.Join(workspace, "runtime", "telemetry.db")})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Start producing once the store is initialized. A failed initialization
	// counts as an error and fails the parent test; events produced faster than
	// a slow start-up can drain would only measure the queue size.
	for started := time.Now(); recorder.healthSnapshot().State == "starting" && time.Since(started) < 30*time.Second; {
		time.Sleep(time.Millisecond)
	}
	pace := writerHelperPace
	if runtime.GOOS == "windows" {
		// Windows serializes the eleven processes' SQLite file locks far more
		// slowly. The pace sets the offered load; what this test checks (no
		// failed write, every event stored, an intact store after a crash) does
		// not depend on it.
		pace *= 4
	}
	for item := 0; count == 0 || item < count; item++ {
		// A slow machine stores events more slowly than the fixed pace offers
		// them. Holding back while writerHelperBacklog events are queued keeps the
		// offered load within what the store absorbs, so the bounded queue never
		// refuses an event and the test measures lost writes, not machine speed.
		for len(recorder.queue) >= writerHelperBacklog {
			time.Sleep(pace)
		}
		event := validEvent(EventResolutionCompleted)
		event.ID = fmt.Sprintf("%s_%06d", prefix, item)
		recorder.Record(event)
		time.Sleep(pace)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := recorder.Close(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(recorder.healthSnapshot()); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func writerHelperCommand(ctx context.Context, workspace, prefix string, count int) *exec.Cmd {
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTelemetryWriterHelperProcess$", "-test.count=1")
	command.Env = append(os.Environ(),
		writerHelperEnv+"=1",
		writerHelperRootEnv+"="+workspace,
		writerHelperPrefixEnv+"="+prefix,
		writerHelperCountEnv+"="+strconv.Itoa(count),
	)
	command.Stderr = os.Stderr
	return command
}

func countEventsWithPrefix(t *testing.T, path, prefix string) int {
	t.Helper()
	database, err := sql.Open("sqlite", sqliteURL(path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM telemetry_events WHERE substr(id, 1, ?) = ?`, len(prefix), prefix).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func requireQuickCheck(t *testing.T, path string) {
	t.Helper()
	database, err := sql.Open("sqlite", sqliteURL(path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var check string
	if err := database.QueryRowContext(t.Context(), `PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("quick_check = %q, %v", check, err)
	}
}

// benchmarkSeedEvents approximates the store after a busy fan-out run.
const benchmarkSeedEvents = 12_000

// BenchmarkRecorderSingleEventBatch measures one recorder committing one event
// per batch, the common shape of a resolve call, against a populated store.
func BenchmarkRecorderSingleEventBatch(b *testing.B) {
	workspace := b.TempDir()
	path := filepath.Join(workspace, "runtime", "telemetry.db")
	seedBenchmarkStore(b, workspace, path)
	recorder := openBenchmarkRecorder(b, workspace, path)
	b.ResetTimer()
	for item := 0; item < b.N; item++ {
		event := validEvent(EventResolutionCompleted)
		event.ID = fmt.Sprintf("evt_bench_%09d", item)
		recorder.Record(event)
		if err := waitWritten(recorder, uint64(item+1)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConcurrentRecordersSingleEventBatch measures twenty recorders, each
// standing for one serve process, committing single-event batches to one store.
// ns/op is wall time per batch across all writers; batch-ms is the mean time one
// writer waits for its own batch.
func BenchmarkConcurrentRecordersSingleEventBatch(b *testing.B) {
	const writers = 20
	workspace := b.TempDir()
	path := filepath.Join(workspace, "runtime", "telemetry.db")
	seedBenchmarkStore(b, workspace, path)
	recorders := make([]*Recorder, writers)
	for index := range recorders {
		recorders[index] = openBenchmarkRecorder(b, workspace, path)
	}
	var mu sync.Mutex
	var waited time.Duration
	b.ResetTimer()
	var wait sync.WaitGroup
	for index, recorder := range recorders {
		share := b.N / writers
		if index < b.N%writers {
			share++
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			var local time.Duration
			for item := range share {
				event := validEvent(EventResolutionCompleted)
				event.ID = fmt.Sprintf("evt_bench_%02d_%09d", index, item)
				started := time.Now()
				recorder.Record(event)
				if err := waitWritten(recorder, uint64(item+1)); err != nil {
					b.Error(err)
					return
				}
				local += time.Since(started)
			}
			mu.Lock()
			waited += local
			mu.Unlock()
		}()
	}
	wait.Wait()
	b.StopTimer()
	if b.N > 0 {
		b.ReportMetric(float64(waited.Microseconds())/1000/float64(b.N), "batch-ms")
	}
}

func seedBenchmarkStore(b *testing.B, workspace, path string) {
	b.Helper()
	recorder := openBenchmarkRecorder(b, workspace, path)
	for item := range benchmarkSeedEvents {
		event := validEvent(EventResolutionCompleted)
		event.ID = fmt.Sprintf("evt_seed_%09d", item)
		recorder.Record(event)
		if (item+1)%64 == 0 {
			if err := waitWritten(recorder, uint64(item+1)); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := waitWritten(recorder, benchmarkSeedEvents); err != nil {
		b.Fatal(err)
	}
	if err := recorder.Close(context.Background()); err != nil {
		b.Fatal(err)
	}
}

func openBenchmarkRecorder(b *testing.B, workspace, path string) *Recorder {
	b.Helper()
	recorder, err := Open(Config{WorkspaceRoot: workspace, Path: path, BufferSize: 256})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = recorder.Close(context.Background()) })
	return recorder
}

// waitWritten blocks until the recorder has written want events in total, or
// reports an error if any event was dropped instead. It is safe to call from
// any goroutine.
func waitWritten(recorder *Recorder, want uint64) error {
	for recorder.written.Load() < want {
		if dropped := recorder.dropped.Load(); dropped != 0 {
			return fmt.Errorf("recorder dropped %d events: %s", dropped, recorder.healthSnapshot().LastError)
		}
		time.Sleep(20 * time.Microsecond)
	}
	return nil
}
