package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

func TestOpenSnapshotOpensExactCurrentGenerationReadOnly(t *testing.T) {
	root := newWorkspace(t)
	result := build(t, root, BuildOptions{})

	handle, err := OpenSnapshot(context.Background(), root, result.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if handle.Pointer.Generation != result.Pointer.Generation {
		t.Fatalf("opened generation %q, want %q", handle.Pointer.Generation, result.Pointer.Generation)
	}
	if _, err := handle.DB.Exec(`CREATE TABLE replay_write(value TEXT)`); err == nil {
		t.Fatal("snapshot database was writable")
	}
}

func TestOpenSnapshotOpensRetainedHistoricalGeneration(t *testing.T) {
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})
	writeReplaySkill(t, root, "first")
	second := build(t, root, BuildOptions{})
	if first.Pointer.CatalogSnapshot == second.Pointer.CatalogSnapshot {
		t.Fatal("fixture did not create a distinct catalog snapshot")
	}

	handle, err := OpenSnapshot(context.Background(), root, first.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if handle.Pointer.Generation != first.Pointer.Generation {
		t.Fatalf("opened generation %q, want retained %q", handle.Pointer.Generation, first.Pointer.Generation)
	}
}

func TestOpenSnapshotUnavailableNeverFallsBack(t *testing.T) {
	root := newWorkspace(t)
	current := build(t, root, BuildOptions{})
	missing := "sha256:" + strings.Repeat("f", 64)
	if missing == current.Pointer.CatalogSnapshot {
		t.Fatal("fixture snapshot unexpectedly matched missing snapshot")
	}

	handle, err := OpenSnapshot(context.Background(), root, missing)
	if handle != nil || !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("OpenSnapshot() = (%v, %v), want nil and ErrSnapshotUnavailable", handle, err)
	}
	if _, err := OpenSnapshot(context.Background(), root, ""); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("empty snapshot error = %v, want ErrSnapshotUnavailable", err)
	}
}

func TestOpenSnapshotRejectsCorruptMatchingGeneration(t *testing.T) {
	root := newWorkspace(t)
	result := build(t, root, BuildOptions{})
	path := generationPath(root, result.Pointer)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}

	handle, err := OpenSnapshot(context.Background(), root, result.Pointer.CatalogSnapshot)
	if handle != nil || !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("OpenSnapshot() = (%v, %v), want nil and ErrSnapshotUnavailable", handle, err)
	}
}

func TestOpenSnapshotRejectsSymlinkGeneration(t *testing.T) {
	root := newWorkspace(t)
	result := build(t, root, BuildOptions{})
	path := generationPath(root, result.Pointer)
	external := filepath.Join(t.TempDir(), "generation.db")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}

	handle, err := OpenSnapshot(context.Background(), root, result.Pointer.CatalogSnapshot)
	if handle != nil || !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("OpenSnapshot() = (%v, %v), want nil and ErrSnapshotUnavailable", handle, err)
	}
}

func TestOpenSnapshotPinProtectsGenerationAndIsCleanedOnClose(t *testing.T) {
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})
	writeReplaySkill(t, root, "second")
	build(t, root, BuildOptions{})

	handle, err := OpenSnapshot(context.Background(), root, first.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	pinPath := handle.pinPath
	if _, err := os.Stat(pinPath); err != nil {
		t.Fatalf("snapshot pin was not created: %v", err)
	}
	if err := CollectGarbage(root, 0); err != nil {
		t.Fatal(err)
	}
	oldPath := generationPath(root, first.Pointer)
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("pinned snapshot was collected: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pinPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot pin remains after close: %v", err)
	}
	if err := CollectGarbage(root, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(pinPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty snapshot pin directory remains after garbage collection: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed snapshot remained protected from GC: %v", err)
	}
}

func TestOpenSnapshotSelectsEquivalentGenerationByLexicalID(t *testing.T) {
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})
	second := build(t, root, BuildOptions{})
	if first.Pointer.CatalogSnapshot != second.Pointer.CatalogSnapshot {
		t.Fatal("equivalent builds produced different snapshots")
	}
	want := first.Pointer.Generation
	if second.Pointer.Generation < want {
		want = second.Pointer.Generation
	}

	handle, err := OpenSnapshot(context.Background(), root, first.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if handle.Pointer.Generation != want {
		t.Fatalf("opened generation %q, want lexical first %q", handle.Pointer.Generation, want)
	}
}

func writeReplaySkill(t *testing.T, root, description string) {
	t.Helper()
	writeCanonical(t, root, "skills/core/replay/skill.meta.yaml", "schema_version: 1\nid: replay\nname: Replay\nstatus: active\ndescription: "+description+"\nrouting:\n  triggers: [replay]\n  not_for: [unrelated]\n  min_scope: single_step\n")
	writeCanonical(t, root, "skills/core/replay/SKILL.md", "# Replay\n")
}

func TestConcurrentOpenAndCloseNeverFailsOnPinDirectory(t *testing.T) {
	root := newWorkspace(t)
	build(t, root, BuildOptions{})

	const workers, iterations = 8, 40
	failures := make(chan error, workers*iterations)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				handle, err := OpenCurrent(context.Background(), root)
				if err != nil {
					failures <- err
					continue
				}
				if err := handle.Close(); err != nil {
					failures <- err
				}
			}
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent open/close failed: %v", err)
	}
}

func TestOpenCurrentLockedBlocksMutationUntilClose(t *testing.T) {
	root := newWorkspace(t)
	build(t, root, BuildOptions{})
	handle, err := OpenCurrentLocked(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if lock, err := mutation.AcquireExclusiveLock(ctx, root, 100*time.Millisecond); err == nil {
		_ = lock.Unlock()
		t.Fatal("exclusive lock was granted while a locked handle was open")
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err := mutation.AcquireExclusiveLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatalf("exclusive lock unavailable after close: %v", err)
	}
	_ = lock.Unlock()
}

func TestOpenCurrentReportsUnavailableCatalogWithSentinel(t *testing.T) {
	root := newWorkspace(t)
	build(t, root, BuildOptions{})
	writeCanonical(t, root, "skills/core/late/skill.meta.yaml", "schema_version: 1\nid: late\n")
	_, err := OpenCurrentLocked(context.Background(), root)
	if !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatalf("OpenCurrentLocked() error = %v, want ErrCatalogUnavailable", err)
	}
}

func TestOpenWithFallbackServesCurrentWhenHealthy(t *testing.T) {
	root := newWorkspace(t)
	buildResult := build(t, root, BuildOptions{})

	handle, err := OpenWithFallback(t.Context(), root)
	if err != nil {
		t.Fatalf("OpenWithFallback failed: %v", err)
	}
	defer handle.Close()

	if handle.Status.ServingMode != ServingCurrent {
		t.Fatalf("serving mode = %v, want %v", handle.Status.ServingMode, ServingCurrent)
	}
	if handle.Status.State != StateHealthy {
		t.Fatalf("state = %v, want %v", handle.Status.State, StateHealthy)
	}
	if handle.Pointer.Generation != buildResult.Pointer.Generation {
		t.Fatalf("generation = %q, want %q", handle.Pointer.Generation, buildResult.Pointer.Generation)
	}
}

func TestOpenWithFallbackServesFallbackWhenCanonicalInvalid(t *testing.T) {
	root := newWorkspace(t)
	buildResult := build(t, root, BuildOptions{})

	// Break canonical files with an invalid status (BUG-07)
	writeCanonical(t, root, "skills/core/first/skill.meta.yaml", "schema_version: 1\nid: first\nname: First\nstatus: shiny\ndescription: Broken.\n")

	// EnsureFreshOrRebuild must strictly reject the invalid canonical file
	if err := EnsureFreshOrRebuild(t.Context(), root); err == nil {
		t.Fatal("EnsureFreshOrRebuild should fail on invalid canonical file")
	}

	// OpenCurrent must strictly reject because state is not healthy
	if _, err := OpenCurrent(t.Context(), root); !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatalf("OpenCurrent error = %v, want ErrCatalogUnavailable", err)
	}

	// OpenWithFallback must fall back to the published pointer with diagnostic warning
	handle, err := OpenWithFallback(t.Context(), root)
	if err != nil {
		t.Fatalf("OpenWithFallback should succeed on fallback: %v", err)
	}
	defer handle.Close()

	if handle.Status.ServingMode != ServingFallback {
		t.Fatalf("serving mode = %v, want %v", handle.Status.ServingMode, ServingFallback)
	}
	if handle.Status.Warning == "" {
		t.Fatal("fallback handle must carry diagnostic warning")
	}
	if handle.Pointer.Generation != buildResult.Pointer.Generation {
		t.Fatalf("served generation = %q, want published %q", handle.Pointer.Generation, buildResult.Pointer.Generation)
	}
}

func TestOpenWithFallbackAutoRebuildsWhenCanonicalValid(t *testing.T) {
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})

	// Add a new valid skill
	writeReplaySkill(t, root, "second")

	handle, err := OpenWithFallback(t.Context(), root)
	if err != nil {
		t.Fatalf("OpenWithFallback failed: %v", err)
	}
	defer handle.Close()

	if handle.Status.ServingMode != ServingCurrent {
		t.Fatalf("serving mode = %v, want %v", handle.Status.ServingMode, ServingCurrent)
	}
	if handle.Pointer.Generation == first.Pointer.Generation {
		t.Fatal("generation should have been rebuilt to include new skill")
	}
}

func TestOpenWithFallbackRejectsCorruptGenerationWhenCanonicalInvalid(t *testing.T) {
	root := newWorkspace(t)
	result := build(t, root, BuildOptions{})
	path := generationPath(root, result.Pointer)

	// Invalidate canonical
	writeCanonical(t, root, "skills/core/first/skill.meta.yaml", "schema_version: 1\nid: first\nstatus: shiny\n")

	// Corrupt database
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt SQLite database content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Must fail because corrupt generation cannot be served as fallback and canonical is invalid
	handle, err := OpenWithFallback(t.Context(), root)
	if handle != nil || !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatalf("OpenWithFallback() = (%v, %v), want nil and ErrCatalogUnavailable", handle, err)
	}
}

func TestOpenWithFallbackLockedRetainsLockUntilClose(t *testing.T) {
	root := newWorkspace(t)
	build(t, root, BuildOptions{})

	handle, err := OpenWithFallbackLocked(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if lock, err := mutation.AcquireExclusiveLock(ctx, root, 50*time.Millisecond); err == nil {
		_ = lock.Unlock()
		t.Fatal("exclusive lock should be blocked while locked handle is open")
	}

	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	lock, err := mutation.AcquireExclusiveLock(t.Context(), root, time.Second)
	if err != nil {
		t.Fatalf("exclusive lock should be available after close: %v", err)
	}
	_ = lock.Unlock()
}
