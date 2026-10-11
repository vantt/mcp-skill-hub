package systemskills

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

const (
	// DistillSkillID is the stable identity of the bundled distill-lab skill.
	DistillSkillID = "distill-lab"
	// DistillSkillVersion versions the bundled distill-lab files independently
	// of the binary.
	DistillSkillVersion = "0.1.0"

	skillEntryFile = "SKILL.md"
	distillRoot    = "distill-lab"
)

// distillLabFS holds the distill-lab bundle. It mirrors the editable source in
// .agents/skills/distill-lab/ of this repository byte for byte, because an
// embed directive cannot reach outside the package folder. Like every directory embed,
// it skips names that start with "." or "_".
//
//go:embed distill-lab
var distillLabFS embed.FS

// SkillFile is one file of a system skill bundle. Path is slash-separated and
// relative to the skill folder.
type SkillFile struct {
	Path    string
	Content []byte
}

// SystemSkill is a system skill that Skill Hub installs as files on disk.
type SystemSkill struct {
	ID      string
	Version string
	// Files lists SKILL.md first, then every other file sorted by path.
	Files []SkillFile
	// HubOnly marks a skill installed only in the hub folder itself.
	HubOnly bool
	// ServedOverMCP marks a skill listed by skills/list and skills/get.
	ServedOverMCP bool
}

var systemSkills = mustBuildSystemSkills()

func mustBuildSystemSkills() []SystemSkill {
	distillFiles, err := embeddedFiles(distillLabFS, distillRoot)
	if err != nil {
		panic(fmt.Sprintf("systemskills: read embedded %s bundle: %v", distillRoot, err))
	}
	return []SystemSkill{
		{
			ID:            CuratorSkillID,
			Version:       CuratorSkillVersion,
			Files:         []SkillFile{{Path: skillEntryFile, Content: []byte(CuratorSkill)}},
			ServedOverMCP: true,
		},
		{
			ID:      DistillSkillID,
			Version: DistillSkillVersion,
			Files:   distillFiles,
			HubOnly: true,
		},
	}
}

func embeddedFiles(fsys fs.FS, root string) ([]SkillFile, error) {
	var files []SkillFile
	err := fs.WalkDir(fsys, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		files = append(files, SkillFile{Path: strings.TrimPrefix(name, root+"/"), Content: content})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		if (files[i].Path == skillEntryFile) != (files[j].Path == skillEntryFile) {
			return files[i].Path == skillEntryFile
		}
		return files[i].Path < files[j].Path
	})
	if len(files) == 0 || files[0].Path != skillEntryFile {
		return nil, fmt.Errorf("%s has no %s", root, skillEntryFile)
	}
	return files, nil
}

// SystemSkills returns every system skill in a fixed order: system-curator,
// then distill-lab. The result is a deep copy, so callers may modify it.
func SystemSkills() []SystemSkill {
	out := make([]SystemSkill, len(systemSkills))
	for i, skill := range systemSkills {
		out[i] = skill
		out[i].Files = make([]SkillFile, len(skill.Files))
		for j, file := range skill.Files {
			out[i].Files[j] = SkillFile{Path: file.Path, Content: append([]byte(nil), file.Content...)}
		}
	}
	return out
}

// FileDigest returns the digest form used by KnownFileDigest and
// CuratorBundle: "sha256:" followed by lowercase hex.
func FileDigest(content []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(content))
}

// KnownFileDigest reports whether digest identifies bytes that Skill Hub wrote
// for the file at path (slash-separated, relative to the skill folder) of the
// system skill id: either the current bundle or a previously shipped version.
func KnownFileDigest(id, path, digest string) bool {
	for _, skill := range systemSkills {
		if skill.ID != id {
			continue
		}
		for _, file := range skill.Files {
			if file.Path == path && FileDigest(file.Content) == digest {
				return true
			}
		}
	}
	for _, known := range knownDigests[id][path] {
		if known == digest {
			return true
		}
	}
	return false
}
