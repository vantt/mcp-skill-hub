package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

func writeDistillLab(t *testing.T, root string) {
	t.Helper()
	for _, skill := range systemskills.SystemSkills() {
		if skill.ID != systemskills.DistillSkillID {
			continue
		}
		for _, file := range skill.Files {
			path := filepath.Join(root, ".agents", "skills", skill.ID, filepath.FromSlash(file.Path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, path, file.Content)
		}
	}
}

func TestDistillLabIntact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if distillLabIntact(root) {
		t.Fatal("an empty folder holds an intact distill-lab")
	}
	writeDistillLab(t, root)
	if !distillLabIntact(root) {
		t.Fatal("the bundled files are not intact")
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills", "distill-lab", "SKILL.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if distillLabIntact(root) {
		t.Fatal("an edited SKILL.md is intact")
	}
}

func TestDistillLabIntactRefusesSymbolicLinks(t *testing.T) {
	t.Parallel()

	target, root := t.TempDir(), t.TempDir()
	writeDistillLab(t, target)
	if err := os.Symlink(filepath.Join(target, ".agents"), filepath.Join(root, ".agents")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if distillLabIntact(root) {
		t.Fatal("a distill-lab reached through a symbolic link is intact")
	}
}
