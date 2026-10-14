package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// commitBaseHistoryLimit bounds the walk through the skill's hub history when a
// checkout recorded no usable base commit.
const commitBaseHistoryLimit = 100

// findCheckoutBase rebuilds the files the checkout started from, the base of a
// three-way merge. It tries the recorded hub commit, then a served snapshot,
// then the skill's recent history, and accepts a candidate only when its
// digest equals the one recorded at checkout. It reports false when none match,
// so the merge never guesses.
func findCheckoutBase(ctx context.Context, root, relDir, id string, marker CheckoutMarker) (map[string][]byte, bool) {
	matches := func(files map[string][]byte) bool {
		return files != nil && filesDigestOf(files) == marker.BaseVersion
	}
	if marker.BaseCommit != "" {
		if files := hubFilesAtCommit(ctx, root, marker.BaseCommit, relDir); matches(files) {
			return files, true
		}
	}
	if files := snapshotFilesMatching(root, id, marker.BaseVersion); files != nil {
		return files, true
	}
	commits, ok := gitLines(ctx, root, "log", "--format=%H", "--max-count=100", "--", relDir)
	if !ok {
		return nil, false
	}
	for _, commit := range commits[:min(len(commits), commitBaseHistoryLimit)] {
		if files := hubFilesAtCommit(ctx, root, commit, relDir); matches(files) {
			return files, true
		}
	}
	return nil, false
}

// hubFilesAtCommit reads the content files of a skill folder as they were at a
// hub commit. It returns nil when the commit or the folder is not there.
func hubFilesAtCommit(ctx context.Context, root, commit, relDir string) map[string][]byte {
	blobs, ok := gitTreeBlobs(ctx, root, commit, relDir)
	if !ok || len(blobs) == 0 {
		return nil
	}
	files := make(map[string][]byte, len(blobs))
	for rel := range blobs {
		if _, skip := sourcepkg.ExcludedFromImport(rel); skip {
			continue
		}
		data, err := gitOutput(ctx, root, "show", commit+":./"+relDir+"/"+rel)
		if err != nil {
			return nil
		}
		files[rel] = data
	}
	return files
}

// snapshotFilesMatching looks through the served snapshots of a skill for one
// whose content digest is version. Snapshots are collected when they stop
// being current, so this is a fallback and never the source of truth.
func snapshotFilesMatching(root, id, version string) map[string][]byte {
	folders, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(snapshotRootRelative), id+"@*"))
	if err != nil {
		return nil
	}
	for _, folder := range folders {
		files := readSnapshotContent(folder)
		if files != nil && filesDigestOf(files) == version {
			return files
		}
	}
	return nil
}

// readSnapshotContent reads the files a snapshot lists in its marker, without
// hub metadata. It returns nil when the marker or a listed file is unreadable.
func readSnapshotContent(folder string) map[string][]byte {
	raw, err := os.ReadFile(filepath.Join(folder, snapshotMarkerName))
	if err != nil {
		return nil
	}
	var marker snapshotMarker
	if json.Unmarshal(raw, &marker) != nil {
		return nil
	}
	files := make(map[string][]byte, len(marker.Files))
	for _, listed := range marker.Files {
		if workspace.IsHubMeta(listed.Path) || !safeTreePath(listed.Path) {
			continue
		}
		if _, skip := sourcepkg.ExcludedFromImport(listed.Path); skip {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(folder, filepath.FromSlash(listed.Path)))
		if readErr != nil {
			return nil
		}
		files[listed.Path] = data
	}
	return files
}
