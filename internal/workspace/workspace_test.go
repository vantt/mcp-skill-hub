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
func TestInspectAcceptsCloneLikeAbsentEmptyCanonicalDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	// Simulate fresh git clone: delete all empty required canonical directories
	for _, dir := range RequiredDirectories() {
		dirPath := filepath.Join(root, filepath.FromSlash(dir))
		_ = os.Remove(dirPath)
	}
	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if len(plan.Findings) != 0 {
		t.Fatalf("expected 0 findings on clone-like workspace with absent empty directories, got: %#v", plan.Findings)
	}
}

func TestInspectRejectsCanonicalDirectoryCollisionWithFileOrSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	// Replace a required directory with a regular file
	target := filepath.Join(root, "history", "operations")
	_ = os.Remove(target)
	if err := os.WriteFile(target, []byte("collision"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	found := false
	for _, f := range plan.Findings {
		if f.ID == "directory_collision" && f.Path == "history/operations" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected directory_collision finding for history/operations, got: %#v", plan.Findings)
	}
}

func TestInspectWithOptionsDetachedIgnoresMissingGitRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	// Remove .git
	_ = os.RemoveAll(filepath.Join(root, ".git"))

	planAttached, err := InspectWithOptions(root, false)
	if err != nil {
		t.Fatal(err)
	}
	hasGitMissing := false
	for _, f := range planAttached.Findings {
		if f.ID == "git_repository_missing" {
			hasGitMissing = true
		}
	}
	if !hasGitMissing {
		t.Fatal("attached mode expected git_repository_missing finding")
	}

	planDetached, err := InspectWithOptions(root, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range planDetached.Findings {
		if f.ID == "git_repository_missing" {
			t.Fatalf("detached mode should not report git_repository_missing, got: %#v", f)
		}
	}
}

func TestIsHubMeta(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want bool
	}{
		{"skill.meta.yaml", true},
		{"skill.meta.yml", true},
		{"SKILL.META.YAML", true},
		{".meta", true},
		{".meta/skill.yaml", true},
		{".meta/distill.yaml", true},
		{".meta/sub/file.txt", true},
		{"skills/default/my-skill/skill.meta.yaml", true},
		{"skills/default/my-skill/skill.meta.yml", true},
		{"skills/default/my-skill/.meta", true},
		{"skills/default/my-skill/.meta/distill.yaml", true},
		{"skills/default/my-skill/.meta/skill.yaml", true},
		{"SKILL.md", false},
		{"skills/default/my-skill/SKILL.md", false},
		{"scripts/check.sh", false},
		{"skills/default/my-skill/scripts/check.sh", false},
		{"sub/.meta/file", false},
		{"sources/catalog/source.yaml", false},
		{"", false},
	}
	for _, tc := range cases {
		got := IsHubMeta(tc.path)
		if got != tc.want {
			t.Errorf("IsHubMeta(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
