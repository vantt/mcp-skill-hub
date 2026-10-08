package app

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

type revisionAtAdapter interface {
	RevisionAt(context.Context, sourcepkg.Source, string) (sourcepkg.Revision, error)
}

func makeRevisionAtCallback(ctx context.Context, adapter sourcepkg.Adapter, src sourcepkg.Source, commit string) func(repoPath string) (sourcepkg.Revision, error) {
	revAdapter, ok := adapter.(revisionAtAdapter)
	if !ok {
		return func(repoPath string) (sourcepkg.Revision, error) {
			return sourcepkg.Revision{}, sourcepkg.ErrHistoryUnavailable
		}
	}
	return func(repoPath string) (sourcepkg.Revision, error) {
		scoped := src
		scoped.Locator.Path = repoPath
		return revAdapter.RevisionAt(ctx, scoped, commit)
	}
}

func repoRelativeSkillPath(scopePath, skillDir string) string {
	parts := make([]string, 0, 2)
	if s := strings.Trim(filepath.ToSlash(scopePath), "/"); s != "" && s != "." {
		parts = append(parts, s)
	}
	if d := strings.Trim(filepath.ToSlash(skillDir), "/"); d != "" && d != "." {
		parts = append(parts, d)
	}
	if len(parts) == 0 {
		return ""
	}
	return path.Join(parts...)
}

func filesDigestOf(files map[string][]byte) string {
	resDigests := make([]skillruntime.ResourceDigest, 0, len(files))
	for relPath, bytes := range files {
		clean := filepath.ToSlash(filepath.Clean(relPath))
		if workspace.IsHubMeta(clean) {
			continue
		}
		resDigests = append(resDigests, skillruntime.ResourceDigest{
			Path:   clean,
			Digest: sourcepkg.Digest(bytes),
		})
	}
	return skillruntime.ContentDigest(resDigests, skillruntime.Spec{}, false)
}

func importedSkillFiles(item DiscoveredSkillItem, targetID string) (map[string][]byte, []string, error) {
	normMD, transforms, err := ensureImportedSkillFrontmatter(item.SkillMDBytes, targetID, item.Description)
	if err != nil {
		return nil, nil, err
	}
	files := make(map[string][]byte, 1+len(item.CompanionBytes))
	files["SKILL.md"] = normMD
	for compPath, compBytes := range item.CompanionBytes {
		cleanPath := filepath.ToSlash(filepath.Clean(compPath))
		files[cleanPath] = compBytes
	}
	return files, transforms, nil
}

func buildGitOrigin(
	captured SkillOrigin,
	scopePath string,
	skillDir string,
	files map[string][]byte,
	transforms []string,
	revisionAt func(path string) (sourcepkg.Revision, error),
) SkillOrigin {
	origin := captured
	origin.Path = repoRelativeSkillPath(scopePath, skillDir)
	if origin.Name == "" {
		if skillDir != "" {
			origin.Name = sanitizeSkillID(filepath.Base(skillDir))
		} else if scopePath != "" {
			origin.Name = sanitizeSkillID(filepath.Base(scopePath))
		}
	}
	origin.Transformations = transforms
	if revisionAt != nil {
		if rev, err := revisionAt(origin.Path); err == nil {
			origin.FolderDigest = rev.ContentDigest
		} else {
			origin.FolderDigest = ""
		}
	} else {
		origin.FolderDigest = ""
	}
	origin.FilesDigest = filesDigestOf(files)
	if mdBytes, ok := files["SKILL.md"]; ok {
		origin.ContentDigest = sourcepkg.Digest(mdBytes)
	}
	return origin
}
