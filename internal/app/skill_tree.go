package app

import (
	"os"
	"path/filepath"
	"strings"
)

// treeFile is one skill content file with the one mode bit that matters.
type treeFile struct {
	Data       []byte
	Executable bool
}

// readHubSkillTree reads the content files of a hub skill folder: everything a
// reader of the skill sees, without hub metadata and without files an import
// never copies. File modes are kept so a script stays executable.
func readHubSkillTree(root, skillRelDir string) (map[string]treeFile, error) {
	contents, err := readLocalSkillFiles(root, skillRelDir)
	if err != nil {
		return nil, err
	}
	tree := make(map[string]treeFile, len(contents))
	for rel, data := range contents {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(skillRelDir), filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		tree[rel] = treeFile{Data: data, Executable: info.Mode().Perm()&0o111 != 0}
	}
	return tree, nil
}

// treeBytes drops the modes of a tree, for the merge engine and digests.
func treeBytes(tree map[string]treeFile) map[string][]byte {
	contents := make(map[string][]byte, len(tree))
	for rel, file := range tree {
		contents[rel] = file.Data
	}
	return contents
}

// skillBaseVersion is the digest a checkout records and a commit compares: the
// content files of the skill, without hub metadata.
func skillBaseVersion(tree map[string]treeFile) string {
	return filesDigestOf(treeBytes(tree))
}

// safeTreePath reports whether a skill-relative path stays inside the folder.
func safeTreePath(rel string) bool {
	return filepath.IsLocal(filepath.FromSlash(rel)) && !strings.Contains(rel, "\\")
}
