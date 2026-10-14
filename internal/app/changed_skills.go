package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
)

// policyFilePath is the recommendation policy of the hub, relative to its root.
const policyFilePath = "config/recommendation.yaml"

// fileReader reads one file of a library state by its path relative to the hub
// root. It reports false when the file is absent.
type fileReader func(rel string) ([]byte, bool)

// changedSkills is the set of skills a change touches, found from the paths the
// change lists.
type changedSkills struct {
	collections   map[string][]string // skill id -> collections it was seen in
	policyChanged bool
}

// parseChangedPaths maps changed paths of skills/<collection>/<id>/... to skill
// ids. A change to SKILL.md counts, because the description affects scoring.
func parseChangedPaths(paths []string) changedSkills {
	set := changedSkills{collections: map[string][]string{}}
	for _, path := range paths {
		path = filepath.ToSlash(path)
		if path == policyFilePath {
			set.policyChanged = true
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) < 4 || parts[0] != "skills" {
			continue
		}
		collection, id := parts[1], parts[2]
		known := set.collections[id]
		if !slices.Contains(known, collection) {
			set.collections[id] = append(known, collection)
		}
	}
	return set
}

// changes lists the changed skills with their version in the new state and in
// the old one. A side is nil when the skill is absent there or not active, so a
// draft that is edited is no change to the served library. The result is
// sorted by id.
func (set changedSkills) changes(newRead, oldRead fileReader) []skillChange {
	ids := make([]string, 0, len(set.collections))
	for id := range set.collections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var changes []skillChange
	for _, id := range ids {
		collections := set.collections[id]
		sort.Strings(collections)
		old, current := readActiveSkill(oldRead, collections, id), readActiveSkill(newRead, collections, id)
		if old == nil && current == nil {
			continue
		}
		changes = append(changes, skillChange{ID: id, Old: old, New: current})
	}
	return changes
}

// readActiveSkill decodes skill id from the first collection that has it. It
// returns nil when the skill is absent, cannot be decoded or is not active.
func readActiveSkill(read fileReader, collections []string, id string) *resolverpkg.Skill {
	for _, collection := range collections {
		dir := "skills/" + collection + "/" + id
		meta, ok := read(dir + "/skill.meta.yaml")
		if !ok {
			meta, ok = read(dir + "/.meta/skill.yaml")
		}
		if !ok {
			continue
		}
		document, _ := read(dir + "/SKILL.md")
		skill, err := decodeProposedSkill(id, meta, document)
		if err != nil || skill.Status != "active" {
			return nil
		}
		return &skill
	}
	return nil
}

// directoryReader reads files below a directory: the working tree or an index
// snapshot.
func directoryReader(dir string) fileReader {
	return func(rel string) ([]byte, bool) {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		return data, err == nil
	}
}

// headReader reads files as the last commit has them.
func headReader(ctx context.Context, root string) fileReader {
	return func(rel string) ([]byte, bool) {
		data, err := offlineGitCommand(ctx, root, "show", "HEAD:./"+rel).Output()
		return data, err == nil
	}
}

// hasGitHead reports whether root is in a Git repository with a commit.
func hasGitHead(ctx context.Context, root string) bool {
	return offlineGitCommand(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD").Run() == nil
}

// workingTreeChangedPaths lists the paths that differ from HEAD in the working
// tree, untracked skill files included. It reports false without Git or a
// commit, where no change can be told from the rest.
func workingTreeChangedPaths(ctx context.Context, root string) ([]string, bool) {
	if !hasGitHead(ctx, root) {
		return nil, false
	}
	tracked, err := offlineGitCommand(ctx, root, "diff", "--name-only", "-z", "--relative", "HEAD").Output()
	if err != nil {
		return nil, false
	}
	untracked, err := offlineGitCommand(ctx, root, "ls-files", "--others", "--exclude-standard", "-z", "--", "skills/", policyFilePath).Output()
	if err != nil {
		return nil, false
	}
	return append(splitNul(tracked), splitNul(untracked)...), true
}

// stagedChangedPaths lists the paths staged for the next commit. It reports
// false when Git cannot tell.
func stagedChangedPaths(ctx context.Context, root string) ([]string, bool) {
	staged, err := offlineGitCommand(ctx, root, "diff", "--cached", "--name-only", "-z", "--relative").Output()
	if err != nil {
		return nil, false
	}
	return splitNul(staged), true
}

func splitNul(output []byte) []string {
	var paths []string
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) > 0 {
			paths = append(paths, string(record))
		}
	}
	return paths
}
