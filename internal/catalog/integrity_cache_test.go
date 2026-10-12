package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// settledGeneration builds a catalog whose generation file looks like one
// published some time ago, and returns the pointer and the file path.
func settledGeneration(t *testing.T) (string, Pointer, string) {
	t.Helper()
	root := newFreshnessWorkspace(t)
	pointer, err := readPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	path := generationPath(root, pointer)
	past := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	return root, pointer, path
}

// verifyAndCountChecks counts quick checks through a process-wide counter, so
// the tests that use it do not run in parallel with other tests.
func verifyAndCountChecks(t *testing.T, root string, pointer Pointer) (checks int64, err error) {
	t.Helper()
	before := integrityVerified.checks.Load()
	err = verifyPointerDatabase(t.Context(), root, pointer)
	return integrityVerified.checks.Load() - before, err
}

func TestIntegrityIsCheckedOncePerFileVersion(t *testing.T) {
	root, pointer, _ := settledGeneration(t)
	for call := range 4 {
		checks, err := verifyAndCountChecks(t, root, pointer)
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if call == 0 {
			want = 1
		}
		if checks != want {
			t.Fatalf("call %d ran %d quick checks, want %d", call, checks, want)
		}
	}
}

func TestIntegrityOfARecentlyWrittenFileIsCheckedEveryTime(t *testing.T) {
	root := newFreshnessWorkspace(t)
	pointer, err := readPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	// The file was just published, inside the window in which a later write could
	// keep its modification time, so its result is not reused yet.
	for call := range 3 {
		if checks, err := verifyAndCountChecks(t, root, pointer); err != nil || checks != 1 {
			t.Fatalf("call %d: %d quick checks, %v; want 1 and no error", call, checks, err)
		}
	}
}

func TestIntegrityIsCheckedAgainWhenTheFileChanges(t *testing.T) {
	changes := []struct {
		name    string
		mutate  func(t *testing.T, path string)
		wantErr bool
	}{
		{
			name: "contents overwritten",
			mutate: func(t *testing.T, path string) {
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("corrupt SQLite database content"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: true,
		},
		{
			name: "bytes flipped keeping size and modification time",
			mutate: func(t *testing.T, path string) {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				// Break every page after the header page, so the damage does not depend on
				// which page holds which table.
				for index := 4096; index < len(contents); index += 512 {
					contents[index] ^= 0xFF
				}
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, contents, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: true,
		},
		{
			name: "replaced by a same-size copy through rename",
			mutate: func(t *testing.T, path string) {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				replacement := filepath.Join(filepath.Dir(path), "replacement.tmp")
				if err := os.WriteFile(replacement, contents, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			root, pointer, path := settledGeneration(t)
			if checks, err := verifyAndCountChecks(t, root, pointer); err != nil || checks != 1 {
				t.Fatalf("first verification: %d checks, %v", checks, err)
			}
			if checks, err := verifyAndCountChecks(t, root, pointer); err != nil || checks != 0 {
				t.Fatalf("second verification: %d checks, %v; want a reused result", checks, err)
			}
			change.mutate(t, path)
			checks, err := verifyAndCountChecks(t, root, pointer)
			if checks != 1 {
				t.Fatalf("verification after the change ran %d quick checks, want 1", checks)
			}
			if (err != nil) != change.wantErr {
				t.Fatalf("verification after the change error = %v, want error %t", err, change.wantErr)
			}
		})
	}
}

func TestInspectDetectsACorruptedGenerationAfterAHealthyCheck(t *testing.T) {
	root, _, path := settledGeneration(t)
	requireCatalogState(t, root, StateHealthy)
	requireCatalogState(t, root, StateHealthy)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt SQLite database content"), 0o600); err != nil {
		t.Fatal(err)
	}
	requireCatalogState(t, root, StateCorrupt)
}
