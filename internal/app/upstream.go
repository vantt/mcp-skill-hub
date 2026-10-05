package app

import (
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func is40Hex(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := range s {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func deriveLocalStatus(originFilesDigest, currentLocalDigest string) string {
	if originFilesDigest == "" {
		return "unknown"
	}
	if originFilesDigest == currentLocalDigest {
		return "clean"
	}
	return "modified"
}

func deriveUpstreamStatus(
	origin SkillOrigin,
	sourceID string,
	sourceRec *sourcepkg.Record,
	state *sourcepkg.UpstreamState,
	currentLocalDigest string,
) (status string, local string, errStr string) {
	local = deriveLocalStatus(origin.FilesDigest, currentLocalDigest)

	if (origin.Kind == "github" || origin.Kind == "git") && sourceID == "" {
		return "untracked", local, ""
	}

	if is40Hex(origin.Ref) {
		return "pinned", local, ""
	}

	if sourceRec == nil || !sameRepository(sourceRec.Locator.Repository, origin.Repository) || sourceRec.Locator.Ref != origin.Ref {
		return "unavailable", local, "source_origin_mismatch"
	}

	if state == nil || state.BaseCommit != origin.Commit || !sameRepository(state.Repository, origin.Repository) || state.Ref != origin.Ref || state.Path != origin.Path {
		return "unknown", local, ""
	}

	if state.Upstream == "unavailable" {
		return "unavailable", local, state.LastError
	}

	if state.Upstream == "removed" {
		return "upstream_removed", local, ""
	}

	if state.Upstream == "same" {
		if local == "modified" {
			return "modified", local, ""
		}
		return "up_to_date", local, ""
	}

	if state.Upstream == "changed" {
		if local == "modified" {
			return "diverged", local, ""
		}
		return "update_available", local, ""
	}

	if state.Upstream == "pinned" {
		return "pinned", local, ""
	}

	return "unknown", local, ""
}

func workingTreeSkillFilesDigest(root, skillRelDir string) string {
	fullSkillDir := filepath.Join(root, filepath.FromSlash(skillRelDir))
	entrypointRelPath, entrypointDigest, _ := inspectCanonicalEntrypoint(root, skillRelDir)
	_, resources, _ := inventorySkillResources(root, fullSkillDir, entrypointRelPath, entrypointDigest)
	prefix := skillRelDir + "/"
	files := make([]skillruntime.ResourceDigest, 0, len(resources))
	for _, resource := range resources {
		if relative := strings.TrimPrefix(resource.Path, prefix); relative != resource.Path {
			files = append(files, skillruntime.ResourceDigest{Path: relative, Digest: resource.Digest})
		}
	}
	return skillruntime.ContentDigest(files, skillruntime.Spec{}, false)
}
