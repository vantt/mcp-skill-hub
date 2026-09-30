package canonical

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestValidateAndScanRejectOversizedCanonicalInput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "skills", "oversized", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(workspace.MaxCanonicalFileBytesV1 + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	issues, err := Validate(root)
	if err != nil || !strings.Contains(issueMessages(issues), "skills/oversized/SKILL.md exceeds V1 per-file limit") {
		t.Fatalf("validation result: issues=%v err=%v", issues, err)
	}
	if _, err := Scan(root); err == nil || !strings.Contains(err.Error(), "skills/oversized/SKILL.md exceeds V1 per-file limit") {
		t.Fatalf("scan error = %v", err)
	}
}

func TestReadCanonicalFileAcceptsExactLimitAndRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "exact.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(workspace.MaxCanonicalFileBytesV1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootHandle.Close()
	contents, err := readCanonicalFile(rootHandle, "exact.bin")
	if err != nil || int64(len(contents)) != workspace.MaxCanonicalFileBytesV1 {
		t.Fatalf("exact-limit read: bytes=%d err=%v", len(contents), err)
	}
	if err := os.Symlink(path, filepath.Join(root, "link.bin")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readCanonicalFile(rootHandle, "link.bin"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink error = %v", err)
	}
}
