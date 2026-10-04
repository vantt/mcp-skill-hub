package skillruntime

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Environment variables the host exports when it runs a skill's check, setup,
// or scripts.
const (
	EnvSkillDir = "SKILLHUB_SKILL_DIR"
	EnvStateDir = "SKILLHUB_STATE_DIR"
	// EnvConfigDir names the per-skill directory that may hold an `env` file
	// of secrets the user stored with `skillhub skill env set`.
	EnvConfigDir = "SKILLHUB_CONFIG_DIR"
)

// ConfigDirRoot is the workspace-relative directory holding one stable config
// directory per skill. It is not keyed by digest, so it survives skill updates.
const ConfigDirRoot = "runtime/config"

// StateDirRoot is the workspace-relative directory holding writable per-skill
// state directories, one per skill and dependency set.
const StateDirRoot = "runtime/envs"

// depsKeyLength is the number of hex characters of the dependency key used in
// a state directory name.
const depsKeyLength = 16

var (
	dependencyManifestNames = map[string]bool{
		"package.json": true, "package-lock.json": true, "npm-shrinkwrap.json": true,
		"pnpm-lock.yaml": true, "yarn.lock": true, "pyproject.toml": true,
		"uv.lock": true, "poetry.lock": true, "Pipfile": true, "Pipfile.lock": true,
		"go.mod": true, "go.sum": true, "Gemfile": true, "Gemfile.lock": true,
		"Cargo.toml": true, "Cargo.lock": true,
	}
	requirementsPattern = regexp.MustCompile(`^requirements.*\.txt$`)
	stateSkillIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	depsKeyPattern      = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// IsDependencyManifest reports whether relPath (relative to the skill folder,
// any depth) is a package manager manifest or lock file.
func IsDependencyManifest(relPath string) bool {
	base := path.Base(strings.ReplaceAll(relPath, "\\", "/"))
	return dependencyManifestNames[base] || requirementsPattern.MatchString(base)
}

// DependencyFiles returns the dependency manifest files among files, in the
// order given.
func DependencyFiles(files []ResourceDigest) []ResourceDigest {
	var found []ResourceDigest
	for _, file := range files {
		if IsDependencyManifest(file.Path) {
			found = append(found, file)
		}
	}
	return found
}

// DepsKey identifies a skill's dependency set: the first 16 hex characters of
// sha256 over the canonical JSON {"dependencies":[{path,digest}... sorted],
// "runtime": spec-or-null}. SKILL.md and other content edits leave it
// unchanged, so installed dependencies survive them.
func DepsKey(files []ResourceDigest, spec Spec, hasSpec bool) string {
	return hashFilesAndRuntime("dependencies", withoutMeta(DependencyFiles(files)), spec, hasSpec)[:depsKeyLength]
}

// StateDirName returns "<skill-id>@<deps-key>", the name of a skill's state
// directory under StateDirRoot.
func StateDirName(skillID, depsKey string) (string, error) {
	if !stateSkillIDPattern.MatchString(skillID) {
		return "", fmt.Errorf("invalid skill id %q", skillID)
	}
	if !depsKeyPattern.MatchString(depsKey) {
		return "", fmt.Errorf("invalid dependency key %q", depsKey)
	}
	return skillID + "@" + depsKey, nil
}
