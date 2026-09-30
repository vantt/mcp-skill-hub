package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativeFilesEnforcesV1ResourceBoundaries(t *testing.T) {
	t.Run("per-file exact limit and overflow", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "exact.bin")
		truncateTestFile(t, path, MaxCanonicalFileBytesV1)
		files, err := RelativeFiles(root)
		if err != nil || len(files) != 1 || files[0] != "exact.bin" {
			t.Fatalf("exact per-file limit: files=%v err=%v", files, err)
		}
		truncateTestFile(t, path, MaxCanonicalFileBytesV1+1)
		if _, err := RelativeFiles(root); err == nil || !strings.Contains(err.Error(), "exact.bin exceeds V1 per-file limit") {
			t.Fatalf("per-file overflow error = %v", err)
		}
	})

	t.Run("aggregate exact limit and overflow", func(t *testing.T) {
		root := t.TempDir()
		for index := int64(0); index < MaxCanonicalBytesV1/MaxCanonicalFileBytesV1; index++ {
			truncateTestFile(t, filepath.Join(root, fileName(index)), MaxCanonicalFileBytesV1)
		}
		if _, err := RelativeFiles(root); err != nil {
			t.Fatalf("exact aggregate limit: %v", err)
		}
		truncateTestFile(t, filepath.Join(root, "overflow.bin"), 1)
		if _, err := RelativeFiles(root); err == nil || !strings.Contains(err.Error(), "aggregate limit") || !strings.Contains(err.Error(), "overflow.bin") {
			t.Fatalf("aggregate overflow error = %v", err)
		}
	})

	t.Run("file count exact limit and overflow", func(t *testing.T) {
		root := t.TempDir()
		for index := 0; index < MaxCanonicalFilesV1; index++ {
			truncateTestFile(t, filepath.Join(root, fileName(int64(index))), 0)
		}
		if files, err := RelativeFiles(root); err != nil || len(files) != MaxCanonicalFilesV1 {
			t.Fatalf("exact file-count limit: count=%d err=%v", len(files), err)
		}
		truncateTestFile(t, filepath.Join(root, "overflow.bin"), 0)
		if _, err := RelativeFiles(root); err == nil || !strings.Contains(err.Error(), "file count exceeds V1 limit") {
			t.Fatalf("file-count overflow error = %v", err)
		}
	})
}

func TestInspectBoundsCanonicalMarkerAndGitignoreReads(t *testing.T) {
	for _, relative := range []string{".skillhub/schema-version", ".gitignore"} {
		t.Run(filepath.Base(relative), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if _, err := Apply(root); err != nil {
				t.Fatal(err)
			}
			truncateTestFile(t, filepath.Join(root, filepath.FromSlash(relative)), MaxCanonicalFileBytesV1+1)
			if _, err := Inspect(root); err == nil || !strings.Contains(err.Error(), relative+" exceeds V1 per-file limit") {
				t.Fatalf("Inspect error = %v", err)
			}
		})
	}
}

func TestRelativeFilesRejectsRootAndFileSymlinks(t *testing.T) {
	realRoot := t.TempDir()
	truncateTestFile(t, filepath.Join(realRoot, "real.txt"), 1)
	rootLink := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := RelativeFiles(rootLink); err == nil || !strings.Contains(err.Error(), "root symlink") {
		t.Fatalf("root symlink error = %v", err)
	}
	if err := os.Symlink(filepath.Join(realRoot, "real.txt"), filepath.Join(realRoot, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := RelativeFiles(realRoot); err == nil || !strings.Contains(err.Error(), "unsafe symlink: link.txt") {
		t.Fatalf("file symlink error = %v", err)
	}
}

func truncateTestFile(t *testing.T, path string, size int64) {
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

func fileName(index int64) string {
	const digits = "00000000"
	value := []byte(digits)
	for position := len(value) - 1; position >= 0 && index > 0; position-- {
		value[position] = byte('0' + index%10)
		index /= 10
	}
	return string(value) + ".bin"
}
