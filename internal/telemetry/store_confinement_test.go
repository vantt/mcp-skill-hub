package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSQLiteOpenRemainsConfinedWhenRuntimeDirectoryIsSwapped(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	runtimePath := filepath.Join(workspace, "runtime")
	originalRuntime := filepath.Join(workspace, "runtime-original")
	outsideDatabase := filepath.Join(outside, "telemetry.db")
	const outsideContents = "outside database must not be opened"
	if err := os.WriteFile(outsideDatabase, []byte(outsideContents), 0o600); err != nil {
		t.Fatal(err)
	}

	originalHook := storeBeforeSQLiteOpen
	attempted := make(chan struct{})
	var once sync.Once
	var swapped bool
	var attackErr error
	storeBeforeSQLiteOpen = func() error {
		once.Do(func() {
			defer close(attempted)
			if err := os.Rename(runtimePath, originalRuntime); err != nil {
				attackErr = err
				return
			}
			if err := os.Symlink(outside, runtimePath); err != nil {
				attackErr = err
				_ = os.Rename(originalRuntime, runtimePath)
				return
			}
			swapped = true
		})
		return nil
	}
	defer func() { storeBeforeSQLiteOpen = originalHook }()

	recorder, err := Open(Config{
		WorkspaceRoot: workspace,
		Path:          filepath.Join(runtimePath, "telemetry.db"),
		BufferSize:    8,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recorder.Close(context.Background()) }()
	select {
	case <-attempted:
	case <-time.After(2 * time.Second):
		t.Fatal("SQLite open fault seam was not reached")
	}
	storeBeforeSQLiteOpen = originalHook

	contents, err := os.ReadFile(outsideDatabase)
	if err != nil || string(contents) != outsideContents {
		t.Fatalf("outside database changed during swap: %q, %v", contents, err)
	}
	if swapped {
		if err := os.Remove(runtimePath); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(originalRuntime, runtimePath); err != nil {
			t.Fatal(err)
		}
	} else if attackErr == nil {
		t.Fatal("runtime swap neither succeeded nor reported an unsupported primitive")
	}

	// A platform that blocks the rename still exercises normal confinement;
	// after restoring a successful attack, the same anchored store must recover
	// and support ordinary write, maintenance, and read operations.
	if err := recorder.Purge(t.Context()); err != nil {
		t.Fatalf("recover anchored telemetry store: %v (swap error: %v)", err, attackErr)
	}
	event := validEvent(EventResolutionCompleted)
	event.ID = "evt_confined"
	recorder.Record(event)
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != 1 {
		t.Fatalf("preview events = %d, want 1", preview.Events)
	}
	contents, err = os.ReadFile(outsideDatabase)
	if err != nil || string(contents) != outsideContents {
		t.Fatalf("outside database changed after normal operations: %q, %v", contents, err)
	}
}
