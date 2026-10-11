package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriterConnectionIsReusedAcrossBatches(t *testing.T) {
	config := newWriterTestConfig(t)
	opens := countSQLiteOpens(t)
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_reuse_1")
	first := writer.database
	writeTestEvent(t, config, &writer, "evt_reuse_2")
	if writer.database != first || opens.Load() != 1 {
		t.Fatalf("connection reused = %v, opens = %d; want reuse and one open", writer.database == first, opens.Load())
	}
}

func TestWriterConnectionReopensWhenDatabaseIsReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses to delete a database file that is open")
	}
	config := newWriterTestConfig(t)
	opens := countSQLiteOpens(t)
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_replaced_1")
	removeDatabaseFiles(t, config.Path)
	writeTestEvent(t, config, &writer, "evt_replaced_2")
	if opens.Load() != 2 {
		t.Fatalf("opens = %d, want a fresh open after the database was replaced", opens.Load())
	}
	if count := countEventsWithPrefix(t, config.Path, "evt_replaced_"); count != 1 {
		t.Fatalf("events in the recreated database = %d, want 1", count)
	}
}

func TestWriterConnectionReopensWhenPermissionsChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports synthetic permission bits")
	}
	config := newWriterTestConfig(t)
	opens := countSQLiteOpens(t)
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_mode_1")
	if err := os.Chmod(config.Path, 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestEvent(t, config, &writer, "evt_mode_2")
	info, err := os.Stat(config.Path)
	if err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 2 || info.Mode().Perm() != 0o600 {
		t.Fatalf("opens = %d, mode = %v; want a fresh open restoring 0600", opens.Load(), info.Mode().Perm())
	}
}

// A connection older than writerReverifyInterval is replaced through the full
// open, which repeats quick_check, so corruption detection stays periodic while
// a busy process keeps writing.
func TestWriterConnectionIsReverifiedPeriodically(t *testing.T) {
	config := newWriterTestConfig(t)
	opens := countSQLiteOpens(t)
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_reverify_1")
	writer.openedAt = writer.openedAt.Add(-writerReverifyInterval)
	writeTestEvent(t, config, &writer, "evt_reverify_2")
	if opens.Load() != 2 {
		t.Fatalf("opens = %d, want a fresh open after the reverify interval", opens.Load())
	}
}

func TestWriterConnectionRefusesSymlinkedDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses to delete a database file that is open")
	}
	config := newWriterTestConfig(t)
	var writer writerConnection
	defer writer.release()
	writeTestEvent(t, config, &writer, "evt_symlink_1")

	outside := filepath.Join(t.TempDir(), "telemetry.db")
	const outsideContents = "outside database must not be opened"
	if err := os.WriteFile(outside, []byte(outsideContents), 0o600); err != nil {
		t.Fatal(err)
	}
	removeDatabaseFiles(t, config.Path)
	if err := os.Symlink(outside, config.Path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := writeEvents(t.Context(), config, &writer, []storedEnvelope{testEnvelope(t, "evt_symlink_2")}); err == nil {
		t.Fatal("wrote through a symlinked database entry")
	}
	if writer.database != nil {
		t.Fatal("refused write left a connection open")
	}
	contents, err := os.ReadFile(outside)
	if err != nil || string(contents) != outsideContents {
		t.Fatalf("outside database changed: %q, %v", contents, err)
	}
}

func TestRecorderReleasesIdleWriterConnection(t *testing.T) {
	originalIdle := writerIdleRelease
	writerIdleRelease = 20 * time.Millisecond
	t.Cleanup(func() { writerIdleRelease = originalIdle })
	opens := countSQLiteOpens(t)
	recorder := newTestRecorder(t, Config{})

	recorder.Record(validEvent(EventResolutionCompleted))
	if err := waitWritten(recorder, 1); err != nil {
		t.Fatal(err)
	}
	afterFirst := opens.Load()
	time.Sleep(100 * time.Millisecond)
	recorder.Record(validEvent(EventResolutionCompleted))
	if err := waitWritten(recorder, 2); err != nil {
		t.Fatal(err)
	}
	if got := opens.Load(); got != afterFirst+1 {
		t.Fatalf("opens after an idle period = %d, want %d", got, afterFirst+1)
	}
}

func newWriterTestConfig(t *testing.T) Config {
	t.Helper()
	workspace := t.TempDir()
	config, err := normalizeStoreConfig(Config{
		WorkspaceRoot:    workspace,
		Path:             filepath.Join(workspace, "runtime", "telemetry.db"),
		Retention:        defaultRetention,
		RollupRetention:  defaultRollupRetention,
		MaxSizeBytes:     defaultMaxSize,
		ContentMode:      ContentModeNone,
		OperationTimeout: defaultOperationTimeout,
		Clock:            time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.storeAnchor == nil {
		t.Fatalf("store not anchored: %v", config.storeAnchorErr)
	}
	t.Cleanup(func() { _ = config.storeAnchor.close() })
	return config
}

// countSQLiteOpens counts full store opens through the pre-open seam. Set it
// before starting a recorder; it is restored after the test's later cleanups.
func countSQLiteOpens(t *testing.T) *atomic.Int64 {
	t.Helper()
	var opens atomic.Int64
	original := storeBeforeSQLiteOpen
	storeBeforeSQLiteOpen = func() error { opens.Add(1); return nil }
	t.Cleanup(func() { storeBeforeSQLiteOpen = original })
	return &opens
}

func testEnvelope(t *testing.T, id string) storedEnvelope {
	t.Helper()
	envelope, err := validateAndBuild(validEvent(EventResolutionCompleted), ContentModeNone, time.Now().UTC(), id)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func writeTestEvent(t *testing.T, config Config, writer *writerConnection, id string) {
	t.Helper()
	if err := writeEvents(t.Context(), config, writer, []storedEnvelope{testEnvelope(t, id)}); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
}

func removeDatabaseFiles(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm", ""} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
}
