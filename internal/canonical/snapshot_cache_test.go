package canonical

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func newCachedScanWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "skills/collection/alpha/skill.meta.yaml", "schema_version: 1\nid: alpha\nname: Alpha\n")
	write(t, root, "skills/collection/alpha/SKILL.md", "# Alpha\n\nVersion A.\n")
	write(t, root, "skills/collection/beta/SKILL.md", "# Beta\n")
	write(t, root, "sources/catalog/source.yaml", "id: SRC-1\n")
	ageTree(t, root, time.Minute)
	return root
}

// ageTree moves every canonical file's modification time into the past, so the
// cache may trust its stamps as for a hub that was not edited recently.
func ageTree(t *testing.T, root string, age time.Duration) {
	t.Helper()
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-age).Truncate(time.Second)
	for _, relative := range files {
		if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(relative)), past, past); err != nil {
			t.Fatal(err)
		}
	}
}

func requireCachedScanMatches(t *testing.T, cache *snapshotCache, root string) Snapshot {
	t.Helper()
	cached, err := cache.scan(root, nil)
	if err != nil {
		t.Fatalf("cached scan: %v", err)
	}
	full, err := Scan(root)
	if err != nil {
		t.Fatalf("full scan: %v", err)
	}
	if !reflect.DeepEqual(cached, full) {
		t.Fatalf("cached scan differs from full scan:\ncached %+v\nfull   %+v", cached, full)
	}
	return cached
}

func TestScanCachedDetectsEveryKindOfChange(t *testing.T) {
	changes := []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{"edit keeping size and restoring the modification time", func(t *testing.T, root string) {
			path := filepath.Join(root, "skills/collection/alpha/SKILL.md")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("# Alpha\n\nVersion B.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
		}},
		{"edit keeping size within the same second", func(t *testing.T, root string) {
			write(t, root, "skills/collection/alpha/SKILL.md", "# Alpha\n\nVersion C.\n")
		}},
		{"replace through rename keeping size and modification time", func(t *testing.T, root string) {
			path := filepath.Join(root, "skills/collection/alpha/SKILL.md")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			temporary := filepath.Join(root, "skills/collection/alpha/SKILL.md.tmp")
			if err := os.WriteFile(temporary, []byte("# Alpha\n\nVersion D.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(temporary, path); err != nil {
				t.Fatal(err)
			}
		}},
		{"add a file", func(t *testing.T, root string) {
			write(t, root, "skills/collection/gamma/SKILL.md", "# Gamma\n")
		}},
		{"delete a file", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "skills/collection/beta/SKILL.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"rename a file", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "skills/collection/beta/SKILL.md"), filepath.Join(root, "skills/collection/beta/README.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"rename a directory", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "skills/collection/beta"), filepath.Join(root, "skills/collection/delta")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			root := newCachedScanWorkspace(t)
			cache := &snapshotCache{now: time.Now}
			before := requireCachedScanMatches(t, cache, root)
			// The second scan is served entirely from trusted cached digests.
			if again := requireCachedScanMatches(t, cache, root); again.ProjectionInputDigest != before.ProjectionInputDigest {
				t.Fatal("unchanged tree produced a different snapshot")
			}
			change.mutate(t, root)
			after := requireCachedScanMatches(t, cache, root)
			if after.ProjectionInputDigest == before.ProjectionInputDigest {
				t.Fatal("cached scan did not observe the change")
			}
			if after.CatalogSnapshot == before.CatalogSnapshot {
				t.Fatal("catalog snapshot did not observe a change under skills/")
			}
		})
	}
}

func TestScanCachedRevertedEditMatchesOriginalSnapshot(t *testing.T) {
	root := newCachedScanWorkspace(t)
	cache := &snapshotCache{now: time.Now}
	original := requireCachedScanMatches(t, cache, root)
	write(t, root, "skills/collection/alpha/SKILL.md", "# Alpha\n\nVersion B, longer.\n")
	if edited := requireCachedScanMatches(t, cache, root); edited.ProjectionInputDigest == original.ProjectionInputDigest {
		t.Fatal("edit was not observed")
	}
	write(t, root, "skills/collection/alpha/SKILL.md", "# Alpha\n\nVersion A.\n")
	if reverted := requireCachedScanMatches(t, cache, root); reverted.ProjectionInputDigest != original.ProjectionInputDigest {
		t.Fatal("reverted edit did not restore the original snapshot")
	}
}

// TestScanCachedTrustsOnlyStampsOutsideTheRacyWindow plants a wrong digest under
// the file's current stamp. Only an entry hashed clearly after the file's last
// modification may be reused; a recent modification forces a fresh read.
func TestScanCachedTrustsOnlyStampsOutsideTheRacyWindow(t *testing.T) {
	root := newCachedScanWorkspace(t)
	const relative = "skills/collection/alpha/SKILL.md"
	plant := func(t *testing.T, cache *snapshotCache, hashedAfterModification time.Duration) {
		t.Helper()
		key, err := filepath.Abs(root)
		if err != nil {
			t.Fatal(err)
		}
		tree := cache.load(key)
		entries := make(map[string]stampedDigest, len(tree.entries))
		for path, entry := range tree.entries {
			entries[path] = entry
		}
		entry := entries[relative]
		entry.digest = "sha256:planted"
		entry.hashedAt = entry.stamp.modTime + int64(hashedAfterModification)
		entries[relative] = entry
		cache.store(key, &cachedTree{entries: entries, snapshot: tree.snapshot})
	}
	digestOf := func(snapshot Snapshot) string {
		for _, file := range snapshot.Files {
			if file.Path == relative {
				return file.Digest
			}
		}
		t.Fatalf("%s missing from snapshot", relative)
		return ""
	}

	t.Run("recent modification is read again", func(t *testing.T) {
		cache := &snapshotCache{now: time.Now}
		requireCachedScanMatches(t, cache, root)
		plant(t, cache, racyWindow-time.Millisecond)
		requireCachedScanMatches(t, cache, root)
	})
	t.Run("modification at the window boundary is read again", func(t *testing.T) {
		cache := &snapshotCache{now: time.Now}
		requireCachedScanMatches(t, cache, root)
		plant(t, cache, racyWindow)
		requireCachedScanMatches(t, cache, root)
	})
	t.Run("old modification reuses the cached digest without reading", func(t *testing.T) {
		cache := &snapshotCache{now: time.Now}
		requireCachedScanMatches(t, cache, root)
		plant(t, cache, racyWindow+time.Millisecond)
		snapshot, err := cache.scan(root, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := digestOf(snapshot); got != "sha256:planted" {
			t.Fatalf("trusted entry was not reused: digest %s", got)
		}
	})
}

func TestScanCachedRehashesFilesModifiedJustBeforeTheScan(t *testing.T) {
	root := newCachedScanWorkspace(t)
	write(t, root, "skills/collection/alpha/SKILL.md", "# Alpha\n\nVersion R.\n")
	cache := &snapshotCache{now: time.Now}
	requireCachedScanMatches(t, cache, root)
	key, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := cache.load(key).entries["skills/collection/alpha/SKILL.md"]
	if entry.trusted() {
		t.Fatal("a file written just before the scan must not be trusted on its stamp alone")
	}
	if !cache.load(key).entries["skills/collection/beta/SKILL.md"].trusted() {
		t.Fatal("an old file should be trusted on its stamp")
	}
}

func TestScanCachedRejectsObservedConcurrentModification(t *testing.T) {
	root := newCachedScanWorkspace(t)
	cache := &snapshotCache{now: time.Now}
	_, err := cache.scan(root, func() {
		write(t, root, "sources/catalog/source.yaml", "id: SRC-1\nname: changed\n")
	})
	if err == nil || !strings.Contains(err.Error(), "changed during scan") {
		t.Fatalf("cached scan accepted observed concurrent modification: %v", err)
	}
	requireCachedScanMatches(t, cache, root)
}

func TestScanCachedRejectsSymlinkLikeScan(t *testing.T) {
	root := newCachedScanWorkspace(t)
	cache := &snapshotCache{now: time.Now}
	requireCachedScanMatches(t, cache, root)
	if err := os.Symlink(filepath.Join(root, "sources/catalog/source.yaml"), filepath.Join(root, "skills/collection/beta/linked.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Scan(root); err == nil {
		t.Fatal("full scan accepted a symlink")
	}
	if _, err := cache.scan(root, nil); err == nil {
		t.Fatal("cached scan accepted a symlink")
	}
}

func TestScanCachedIsSafeForConcurrentCallers(t *testing.T) {
	root := newCachedScanWorkspace(t)
	want, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	cache := &snapshotCache{now: time.Now}
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		group.Go(func() {
			for range 20 {
				got, err := cache.scan(root, nil)
				if err != nil {
					errs <- err
					return
				}
				if got.ProjectionInputDigest != want.ProjectionInputDigest || got.CatalogSnapshot != want.CatalogSnapshot {
					errs <- os.ErrInvalid
					return
				}
				got.Files[0].Digest = "caller-owned" // must not leak into the cache
			}
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	requireCachedScanMatches(t, cache, root)
}
