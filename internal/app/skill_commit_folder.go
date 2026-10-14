package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

const (
	excludeReasonHubManaged = "managed by the hub"
	excludeReasonMarker     = "managed by skillhub (checkout marker)"
	excludeReasonSymlink    = "symbolic link"
	excludeReasonNotRegular = "not a regular file"
)

// commitTarget is the project folder a commit reads, with the id it commits to.
type commitTarget struct {
	dir    string
	id     string
	marker *CheckoutMarker
}

// resolveCommitTarget turns the argument of `skill commit` into a folder. An
// existing directory is a folder; anything else is a skill id, read from
// <project>/.claude/skills/<id>/.
func resolveCommitTarget(ctx context.Context, argument, projectDir string) (commitTarget, *Error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return commitTarget{}, NewInvalidRequestError("commit needs a skill id or a folder", "Run `skillhub skill commit <id>` or `skillhub skill commit <folder>`.")
	}
	if strings.HasPrefix(argument, "SUB-") {
		return commitTarget{}, NewInvalidRequestError(fmt.Sprintf("%s is a submission, not a folder", argument), fmt.Sprintf("Review it with `skillhub skill review %s`.", argument))
	}
	candidate := argument
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(projectDir, candidate)
	}
	dir := ""
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		dir = filepath.Clean(candidate)
	} else if skillTargetIDPattern.MatchString(argument) {
		dir = filepath.Join(projectRootOf(ctx, projectDir), filepath.FromSlash(projectSkillsDir), argument)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return commitTarget{}, NewInvalidRequestError(
				fmt.Sprintf("there is no %s/%s folder in this project", projectSkillsDir, argument),
				fmt.Sprintf("Copy it here with `skillhub skill checkout %s`, or start a new one with `skillhub skill create %s`.", argument, argument),
			)
		}
	} else {
		return commitTarget{}, NewInvalidRequestError(fmt.Sprintf("%q is neither a skill id nor a folder", argument), "Pass a lowercase skill id or the path of a skill folder.")
	}
	target := commitTarget{dir: dir}
	marker, found, err := ReadCheckoutMarker(dir)
	if err != nil {
		return commitTarget{}, NewInvalidRequestError(err.Error(), "Update skillhub, or delete the marker file to commit this folder as a new copy.")
	}
	if found {
		target.marker, target.id = &marker, marker.SkillID
	} else {
		target.id = sanitizeSkillID(filepath.Base(dir))
	}
	if !skillTargetIDPattern.MatchString(target.id) {
		return commitTarget{}, NewInvalidRequestError(fmt.Sprintf("cannot derive a skill id from folder %q", filepath.Base(dir)), "Name the folder with a lowercase id such as `team-release`.")
	}
	return target, nil
}

// readProjectFolder reads the content of a project skill folder. It leaves out
// what an import never copies, the checkout marker, hub metadata, links and
// special files, and names each with its reason.
func readProjectFolder(dir string) (map[string]treeFile, []ExcludedFile, *Error, error) {
	tree := map[string]treeFile{}
	var excluded []ExcludedFile
	var total int64
	exclude := func(rel, reason string) { excluded = append(excluded, ExcludedFile{Path: rel, Reason: reason}) }
	err := filepath.WalkDir(dir, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if full == dir {
			return nil
		}
		relNative, relErr := filepath.Rel(dir, full)
		if relErr != nil {
			return relErr
		}
		rel := filepath.ToSlash(relNative)
		if reason, skip := folderExclusion(rel); skip {
			// The marker is skillhub's own file in every checkout; listing it
			// in every preview would only be noise.
			if reason != excludeReasonMarker {
				exclude(rel, reason)
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case entry.IsDir():
			return nil
		case entry.Type()&os.ModeSymlink != 0:
			exclude(rel, excludeReasonSymlink)
			return nil
		case !entry.Type().IsRegular():
			exclude(rel, excludeReasonNotRegular)
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		total += info.Size()
		if info.Size() > sourcepkg.DefaultMaxFileSize || total > sourcepkg.DefaultMaxBytes || len(tree) >= sourcepkg.DefaultMaxFiles {
			return errFolderTooLarge
		}
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			return readErr
		}
		tree[rel] = treeFile{Data: data, Executable: info.Mode().Perm()&0o111 != 0}
		return nil
	})
	if err == errFolderTooLarge {
		return nil, nil, NewInvalidRequestError(
			fmt.Sprintf("the folder is larger than a skill may be (at most %d files, %d MiB in total, %d MiB per file)", sourcepkg.DefaultMaxFiles, sourcepkg.DefaultMaxBytes>>20, sourcepkg.DefaultMaxFileSize>>20),
			"Commit the skill's own folder, not a parent; build output and secrets are left out automatically."), nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if _, ok := tree["SKILL.md"]; !ok {
		return nil, nil, NewInvalidRequestError(fmt.Sprintf("%s has no SKILL.md", dir), "A skill folder needs a SKILL.md at its top level."), nil
	}
	return tree, excluded, nil, nil
}

type folderTooLargeError struct{}

func (folderTooLargeError) Error() string { return "folder too large" }

var errFolderTooLarge error = folderTooLargeError{}

// folderExclusion reports whether a folder-relative path stays out of a commit.
// The import filter is the only list of secrets and build output; the hub's own
// metadata and the checkout marker are managed files, never content.
func folderExclusion(rel string) (string, bool) {
	switch {
	case rel == CheckoutMarkerName || rel == CheckoutMarkerName+".tmp":
		return excludeReasonMarker, true
	case workspace.IsHubMeta(rel):
		return excludeReasonHubManaged, true
	}
	return sourcepkg.ExcludedFromImport(rel)
}
