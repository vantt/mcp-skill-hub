package hostintegration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedHostReadsEnforceBoundaryForPathAndRootReads(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "CLAUDE.md")
	truncateManagedTestFile(t, path, maxManagedHostFileBytes)
	contents, _, exists, err := readManagedFile(path, workspace)
	if err != nil || !exists || int64(len(contents)) != maxManagedHostFileBytes {
		t.Fatalf("exact-limit path read: bytes=%d exists=%v err=%v", len(contents), exists, err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	contents, _, exists, err = readManagedFileAtRoot(root, workspace, path)
	if err != nil || !exists || int64(len(contents)) != maxManagedHostFileBytes {
		t.Fatalf("exact-limit root read: bytes=%d exists=%v err=%v", len(contents), exists, err)
	}

	truncateManagedTestFile(t, path, maxManagedHostFileBytes+1)
	for name, read := range map[string]func() error{
		"path": func() error { _, _, _, err := readManagedFile(path, workspace); return err },
		"root": func() error { _, _, _, err := readManagedFileAtRoot(root, workspace, path); return err },
	} {
		t.Run(name, func(t *testing.T) {
			err := read()
			if err == nil || !strings.Contains(err.Error(), "CLAUDE.md exceeds read limit") {
				t.Fatalf("overflow error = %v", err)
			}
		})
	}
}

func TestManagedHostReadRejectsFileSymlink(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "real.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, ".mcp.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, _, err := readManagedFile(link, workspace); !errors.Is(err, ErrSymlink) {
		t.Fatalf("path symlink error = %v", err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, _, _, err := readManagedFileAtRoot(root, workspace, link); !errors.Is(err, ErrSymlink) {
		t.Fatalf("root symlink error = %v", err)
	}
}

func truncateManagedTestFile(t *testing.T, path string, size int64) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
