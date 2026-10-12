package app

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// distillLabScript is the distill-lab script, relative to the hub folder.
const distillLabScript = ".agents/skills/" + systemskills.DistillSkillID + "/scripts/distill.mjs"

// distillLabIntact reports whether every file of the bundled distill-lab
// skill is in the hub's shared Agent Skills folder with the content of the
// current bundle (CRLF line endings read as LF), reached without a symbolic
// link. Only then does Skill Hub print a command that runs its script.
func distillLabIntact(root string) bool {
	for _, skill := range systemskills.SystemSkills() {
		if skill.ID != systemskills.DistillSkillID {
			continue
		}
		for _, file := range skill.Files {
			raw, ok := readWithoutSymlinks(root, path.Join(".agents", "skills", skill.ID, file.Path))
			if !ok || !systemskills.SameContent(raw, file.Content) {
				return false
			}
		}
		return true
	}
	return false
}

// readWithoutSymlinks reads the regular file at slashPath under root and
// fails when any component below root is a symbolic link or not what it
// should be.
func readWithoutSymlinks(root, slashPath string) ([]byte, bool) {
	current := root
	parts := strings.Split(slashPath, "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, false
		}
		last := index == len(parts)-1
		if (last && (!info.Mode().IsRegular() || info.Size() > maxGeneratedFileBytes)) || (!last && !info.IsDir()) {
			return nil, false
		}
	}
	raw, err := os.ReadFile(current)
	return raw, err == nil
}
