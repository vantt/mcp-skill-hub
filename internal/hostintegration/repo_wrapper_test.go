package hostintegration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRepositoryDistillLabWrapperIsTheInstalledOne checks that this
// repository's Claude Code entry for distill-lab is the wrapper the installer
// writes, rendered from the editable source in .agents/skills/distill-lab/.
func TestRepositoryDistillLabWrapperIsTheInstalledOne(t *testing.T) {
	const regenerate = "copy the wrapper a fresh `skillhub init` writes to .claude/skills/distill-lab/SKILL.md, " +
		"rerun the hostintegration tests with SKILLHUB_UPDATE_GOLDEN=1 for testdata/wrappers/distill-lab.golden, " +
		"and, once a release shipped the old wrapper, add its digest to known-wrappers.go"
	repoRoot := filepath.Join("..", "..")
	source, err := os.ReadFile(filepath.Join(repoRoot, ".agents", "skills", "distill-lab", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	description, err := frontmatterDescription(source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := renderWrapper("distill-lab", description)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(repoRoot, ".claude", "skills", "distill-lab", "SKILL.md"),
		filepath.Join("testdata", "wrappers", "distill-lab.golden"),
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is not the wrapper rendered from .agents/skills/distill-lab/SKILL.md; %s", path, regenerate)
		}
	}
}
