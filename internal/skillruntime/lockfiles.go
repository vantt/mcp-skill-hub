package skillruntime

import (
	"bytes"
	"path"
	"regexp"
	"slices"
	"strings"
)

// manifestLocks maps a manifest file name to its ecosystem and the lock files
// that pin it when they sit in the same directory.
var manifestLocks = map[string]struct {
	ecosystem string
	locks     []string
}{
	"package.json":   {"npm", []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock"}},
	"pyproject.toml": {"python", []string{"uv.lock", "poetry.lock"}},
	"go.mod":         {"go", []string{"go.sum"}},
	"Cargo.toml":     {"cargo", []string{"Cargo.lock"}},
	"Gemfile":        {"ruby", []string{"Gemfile.lock"}},
}

var goRequireLine = regexp.MustCompile(`(?m)^\s*require\b`)

// missingLockfiles lists, as "<ecosystem>: <manifest path>", every dependency
// manifest that has no lock file beside it. A requirements file counts only
// when a requirement is not pinned with `==`. go.mod counts only when it
// requires modules. Only paths are reported, never file text.
func missingLockfiles(files []HintFile) []string {
	present := make(map[string]bool, len(files))
	for _, file := range files {
		present[file.Path] = true
	}
	missing := []string{}
	for _, file := range files {
		directory, name := path.Split(file.Path)
		if entry, ok := manifestLocks[name]; ok {
			if name == "go.mod" && !goRequireLine.Match(file.Content) {
				continue
			}
			if !slices.ContainsFunc(entry.locks, func(lock string) bool { return present[directory+lock] }) {
				missing = append(missing, entry.ecosystem+": "+file.Path)
			}
			continue
		}
		if requirementsPattern.MatchString(name) && hasUnpinnedRequirement(file.Content) {
			missing = append(missing, "python: "+file.Path)
		}
	}
	slices.Sort(missing)
	return missing
}

// hasUnpinnedRequirement reports whether a requirements file lists a package
// without an exact `==` pin. Comments, blank lines, and option lines (-r, -e,
// --hash, ...) are ignored.
func hasUnpinnedRequirement(content []byte) bool {
	for _, raw := range bytes.Split(content, []byte("\n")) {
		line := strings.TrimSpace(string(raw))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if !strings.Contains(line, "==") {
			return true
		}
	}
	return false
}
