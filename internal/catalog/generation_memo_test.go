package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/canonical"
)

// The tests below count opened databases and inspect the process-wide memo
// store, so they do not run in parallel with other tests.

func observeForTest(t *testing.T, root string) *View {
	t.Helper()
	view, err := ObserveWithFallbackLocked(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = view.Close() })
	return view
}

func closeForTest(t *testing.T, view *View) {
	t.Helper()
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoIsSharedWhileTheGenerationIsUnchanged(t *testing.T) {
	root := newFreshnessWorkspace(t)
	first := observeForTest(t, root)
	MemoPut(first, "value", "kept")
	closeForTest(t, first)

	before := PinnedOpens()
	for range 3 {
		view := observeForTest(t, root)
		if got, ok := MemoGet[string](view, "value"); !ok || got != "kept" {
			t.Fatalf("memo value = %q, %v; want the value stored by the first request", got, ok)
		}
		closeForTest(t, view)
	}
	if opened := PinnedOpens() - before; opened != 0 {
		t.Fatalf("observing an unchanged generation opened %d databases, want 0", opened)
	}
}

func TestObservingAGenerationWritesNoPinUntilTheDatabaseIsNeeded(t *testing.T) {
	root := newFreshnessWorkspace(t)
	view := observeForTest(t, root)
	pins := filepath.Join(root, "runtime", "catalog", "pins", view.Pointer.Generation)
	if entries, _ := os.ReadDir(pins); len(entries) != 0 {
		t.Fatalf("observing wrote %d pin files", len(entries))
	}
	if _, err := view.DB(context.Background()); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(pins); err != nil || len(entries) != 1 {
		t.Fatalf("after opening the database there are %d pin files (%v), want 1", len(entries), err)
	}
	closeForTest(t, view)
	if entries, _ := os.ReadDir(pins); len(entries) != 0 {
		t.Fatalf("closing the view left %d pin files", len(entries))
	}
}

func TestRebuildReplacesTheMemoOfThePreviousGeneration(t *testing.T) {
	root := newFreshnessWorkspace(t)
	first := observeForTest(t, root)
	MemoPut(first, "value", "old")
	oldGeneration := first.Pointer.Generation
	closeForTest(t, first)

	writeCanonical(t, root, "skills/freshness/freshness-skill/notes.md", "Changed notes.\n")
	build(t, root, BuildOptions{})

	second := observeForTest(t, root)
	if second.Pointer.Generation == oldGeneration {
		t.Fatal("the rebuild kept the generation")
	}
	if got, ok := MemoGet[string](second, "value"); ok {
		t.Fatalf("memo of the previous generation leaked into the new one: %q", got)
	}
	generationMemos.mu.Lock()
	defer generationMemos.mu.Unlock()
	if memo := generationMemos.byRoot[filepath.Clean(root)]; memo == nil || memo.identity.generation != second.Pointer.Generation {
		t.Fatal("the memo store still holds a previous generation for this workspace")
	}
}

func TestTouchedGenerationFileGetsAFreshMemo(t *testing.T) {
	root := newFreshnessWorkspace(t)
	first := observeForTest(t, root)
	MemoPut(first, "value", "old")
	path := generationPath(root, first.Pointer)
	closeForTest(t, first)

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second := observeForTest(t, root)
	if got, ok := MemoGet[string](second, "value"); ok {
		t.Fatalf("memo survived a change to the generation file: %q", got)
	}
}

func TestCorruptedGenerationIsRebuiltNotServedFromItsMemo(t *testing.T) {
	root := newFreshnessWorkspace(t)
	first := observeForTest(t, root)
	MemoPut(first, "value", "old")
	pointer := first.Pointer
	path := generationPath(root, pointer)
	closeForTest(t, first)

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := len(contents) / 2; index < len(contents)/2+512; index++ {
		contents[index] ^= 0xff
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	second := observeForTest(t, root)
	if second.Pointer.Generation == pointer.Generation {
		t.Fatal("a corrupted generation was observed as healthy")
	}
	if got, ok := MemoGet[string](second, "value"); ok {
		t.Fatalf("memo of the corrupted generation was served: %q", got)
	}
}

func TestMemoMemoryDoesNotGrowWithGenerationsOrValues(t *testing.T) {
	root := newFreshnessWorkspace(t)
	for generation := range 3 * maxMemoRoots {
		writeCanonical(t, root, "skills/freshness/freshness-skill/notes.md", fmt.Sprintf("Notes %d.\n", generation))
		build(t, root, BuildOptions{})
		view := observeForTest(t, root)
		for value := range maxMemoValues + 100 {
			MemoPut(view, fmt.Sprintf("value-%d", value), value)
		}
		view.memo.mu.Lock()
		held := len(view.memo.values)
		view.memo.mu.Unlock()
		closeForTest(t, view)
		if held > maxMemoValues {
			t.Fatalf("generation %d holds %d memo values, want at most %d", generation, held, maxMemoValues)
		}
		generationMemos.mu.Lock()
		stored := len(generationMemos.byRoot)
		generationMemos.mu.Unlock()
		if stored > maxMemoRoots {
			t.Fatalf("memo store holds %d roots, want at most %d", stored, maxMemoRoots)
		}
	}
	generationMemos.mu.Lock()
	defer generationMemos.mu.Unlock()
	count := 0
	for key := range generationMemos.byRoot {
		if key == filepath.Clean(root) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("memo store holds %d entries for one workspace after many rebuilds, want 1", count)
	}
}

func TestMemoStoreIsBoundedAcrossWorkspaces(t *testing.T) {
	for range maxMemoRoots + 4 {
		root := newFreshnessWorkspace(t)
		view := observeForTest(t, root)
		closeForTest(t, view)
	}
	generationMemos.mu.Lock()
	defer generationMemos.mu.Unlock()
	if len(generationMemos.byRoot) > maxMemoRoots {
		t.Fatalf("memo store holds %d workspaces, want at most %d", len(generationMemos.byRoot), maxMemoRoots)
	}
}

func TestViewReportsCanonicalDigestsOfTheFreshnessScan(t *testing.T) {
	root := newFreshnessWorkspace(t)
	view := observeForTest(t, root)
	const path = "skills/freshness/freshness-skill/notes.md"
	snapshot, err := canonical.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	var want string
	for _, file := range snapshot.Files {
		if file.Path == path {
			want = file.Digest
		}
	}
	if got, ok := view.CanonicalDigest(path); !ok || got != want || want == "" {
		t.Fatalf("digest of %s = %q, %v; want %q", path, got, ok, want)
	}
	if _, ok := view.CanonicalDigest("skills/freshness/freshness-skill/missing.md"); ok {
		t.Fatal("a missing file has a digest")
	}
}

func TestPointerMetadataIsComparedOncePerFileVersion(t *testing.T) {
	root, pointer, path := settledGeneration(t)
	if _, err := verifyAndCountChecks(t, root, pointer); err != nil {
		t.Fatal(err)
	}
	identity, err := statGeneration(root, pointer.Database)
	if err != nil {
		t.Fatal(err)
	}
	if !integrityVerified.matches(path, identity, pointer) {
		t.Fatal("a verified file version does not remember the pointer it matched")
	}
	// A pointer that differs in any field is still compared with the file.
	other := pointer
	other.CatalogSnapshot = "sha256:other"
	if checks, err := verifyAndCountChecks(t, root, other); err == nil || checks != 0 {
		t.Fatalf("verifying a different pointer = %v after %d quick checks, want a metadata mismatch", err, checks)
	}
	// The same pointer is accepted again afterwards.
	if _, err := verifyAndCountChecks(t, root, pointer); err != nil {
		t.Fatal(err)
	}
}
