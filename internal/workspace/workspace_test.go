package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	first, err := Apply(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Findings) != 0 {
		t.Fatalf("remaining findings after first apply: %#v", first.Findings)
	}
	second, err := Apply(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Findings) != 0 {
		t.Fatalf("remaining findings after second apply: %#v", second.Findings)
	}
	for _, relative := range append(RequiredDirectories(), ".skillhub/schema-version", ".gitignore") {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Errorf("missing %s: %v", relative, err)
		}
	}
}

func TestInspectRejectsNestedGitWorkspace(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	nested := filepath.Join(root, "nested")
	if _, err := Apply(nested); err == nil {
		t.Fatal("Apply accepted a nested workspace")
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("unsafe nested target was created: %v", err)
	}
}

func TestApplyRejectsDescendantSymlinkBeforeWriting(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, ".skillhub")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Apply(root); err == nil {
		t.Fatal("Apply accepted a .skillhub symlink")
	}
	if _, err := os.Stat(filepath.Join(external, "schema-version")); !os.IsNotExist(err) {
		t.Fatalf("Apply wrote through .skillhub symlink: %v", err)
	}
}

func TestApplyRejectsSourceCheckout(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root); err == nil {
		t.Fatal("Apply accepted a source checkout")
	}
}
