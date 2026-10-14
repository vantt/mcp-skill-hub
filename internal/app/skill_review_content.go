package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

// Baselines a skill's files are compared with on the review screen.
const (
	reviewBaselineApproved = "approved" // the content a person last approved
	reviewBaselineServed   = "served"   // what agents get today
	reviewBaselineNone     = "none"     // a draft: every file is new
)

// ReviewFile is one content file on the full review screen.
type ReviewFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Binary    bool   `json:"binary"`
	Changed   bool   `json:"changed"`
	// Change is "A", "M" or "D" for a changed file and empty for an unchanged one.
	Change    string  `json:"change,omitempty"`
	Added     int     `json:"added"`
	Removed   int     `json:"removed"`
	Diff      string  `json:"diff,omitempty"`
	Truncated bool    `json:"truncated"`
	Content   *string `json:"content,omitempty"`
}

// SkillReviewContent is everything a person reads before approving a skill:
// every file, what changed against the baseline, the license and the origin.
type SkillReviewContent struct {
	Result
	SkillID       string `json:"skill_id"`
	State         string `json:"state"`
	Title         string `json:"title"`
	License       string `json:"license"`
	Origin        string `json:"origin,omitempty"`
	ThirdParty    bool   `json:"third_party"`
	Approved      bool   `json:"approved"`
	ContentDigest string `json:"content_digest"`
	// Baseline is "approved", "served" or "none".
	Baseline       string       `json:"baseline"`
	BaselineCommit string       `json:"baseline_commit,omitempty"`
	Files          []ReviewFile `json:"files"`
}

// ReviewSkillContent reads a skill's files for the full review screen. The
// files are compared with the last approved content when one is recorded, with
// what agents get today for another skill, and with nothing for a draft.
func (service SkillService) ReviewSkillContent(ctx context.Context, path, id string) (SkillReviewContent, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillReviewContent{}, err
	}
	hub, err := loadHubSkill(root, id)
	if err != nil {
		return SkillReviewContent{}, err
	}
	if hub == nil {
		return SkillReviewContent{}, skill.ErrNotFound
	}
	trust, err := service.contentTrustWithChanges(ctx, path, id)
	if err != nil {
		return SkillReviewContent{}, err
	}
	meta := parseSkillReviewMeta(hub.meta)
	meta.Name, _ = catalog.DeriveSkillNameAndDesc(hub.tree["SKILL.md"].Data, meta.Name, meta.Description, id)
	_, _, license := parseSkillMDFrontmatter(hub.tree["SKILL.md"].Data)
	_, origin := queueFacts(ctx, root, id)
	review := SkillReviewContent{
		SkillID: id, State: meta.Status, Title: meta.Name, License: reviewLicense(license, hub.tree), Origin: origin,
		ThirdParty: trust.ThirdParty, Approved: trust.Approved, ContentDigest: trust.ContentDigest,
	}
	baseline, commit := reviewBaseline(ctx, root, id, hub, trust)
	review.Baseline, review.BaselineCommit = commit.kind, commit.commit
	review.Files = reviewFiles(hub.tree, baseline, review.Baseline == reviewBaselineNone)
	review.Result = NewResult(StatusOK, fmt.Sprintf("%s: %s, %d changed.", id, pluralize(len(review.Files), "file"), changedCount(review.Files)))
	return review, ctx.Err()
}

type baselineSource struct{ kind, commit string }

// reviewBaseline returns the files a skill is compared with.
func reviewBaseline(ctx context.Context, root, id string, hub *hubSkill, trust ContentTrust) (map[string][]byte, baselineSource) {
	if parseSkillReviewMeta(hub.meta).Status == "draft" {
		return nil, baselineSource{kind: reviewBaselineNone}
	}
	if changes := trust.ChangesSinceApproval; changes != nil && changes.Found {
		if files, ok := approvedFiles(ctx, root, hub.relDir, changes.Commit); ok {
			return files, baselineSource{kind: reviewBaselineApproved, commit: changes.Commit}
		}
	}
	if files, ok := servedFiles(ctx, root, id, hub.relDir); ok {
		return files, baselineSource{kind: reviewBaselineServed}
	}
	return nil, baselineSource{kind: reviewBaselineNone}
}

// approvedFiles reads the skill folder as it was in the approval commit.
func approvedFiles(ctx context.Context, root, relDir, commit string) (map[string][]byte, bool) {
	blobs, ok := gitTreeBlobs(ctx, root, commit, relDir)
	if !ok {
		return nil, false
	}
	files := make(map[string][]byte, len(blobs))
	for rel, id := range blobs {
		if _, excluded := folderExclusion(rel); excluded {
			continue
		}
		data, err := gitOutput(ctx, root, "cat-file", "blob", id)
		if err != nil {
			return nil, false
		}
		files[rel] = data
	}
	return files, true
}

// servedFiles reads the skill folder as the published catalog serves it.
func servedFiles(ctx context.Context, root, id, relDir string) (map[string][]byte, bool) {
	manifest, err := skill.GetManifestAnyState(ctx, root, id)
	if err != nil {
		return nil, false
	}
	files := map[string][]byte{}
	for _, row := range manifest.Resources {
		rel, inside := strings.CutPrefix(row.Path, relDir+"/")
		if !inside {
			continue
		}
		if _, excluded := folderExclusion(rel); excluded {
			continue
		}
		data, err := skill.ReadResource(ctx, root, manifest.CatalogSnapshot, row.Path, row.Digest)
		if err != nil {
			return nil, false
		}
		files[rel] = data
	}
	return files, true
}

// reviewFiles lists every current file, SKILL.md first, then the files the
// baseline had that are gone. Without a baseline every file is new.
func reviewFiles(current map[string]treeFile, baseline map[string][]byte, allNew bool) []ReviewFile {
	paths := make([]string, 0, len(current))
	for rel := range current {
		paths = append(paths, rel)
	}
	for rel := range baseline {
		if _, kept := current[rel]; !kept {
			paths = append(paths, rel)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if (paths[i] == "SKILL.md") != (paths[j] == "SKILL.md") {
			return paths[i] == "SKILL.md"
		}
		return paths[i] < paths[j]
	})
	files := make([]ReviewFile, 0, len(paths))
	for _, rel := range paths {
		now, has := current[rel]
		before, hadBefore := baseline[rel]
		data := now.Data
		if !has {
			data = before
		}
		file := ReviewFile{Path: rel, SizeBytes: int64(len(data)), Binary: !isPlainText(data)}
		switch {
		case !has:
			file.Changed, file.Change = true, "D"
		case allNew || !hadBefore:
			file.Changed, file.Change = true, "A"
		case !bytes.Equal(before, now.Data):
			file.Changed, file.Change = true, "M"
		}
		if !file.Binary {
			text := data
			if len(text) > maxEditedTextBytes {
				text, file.Truncated = text[:maxEditedTextBytes], true
			}
			if has {
				content := string(text)
				file.Content = &content
			}
			if file.Changed && !file.Truncated {
				file.Added, file.Removed = skill.CountLineChanges(before, now.Data)
				file.Diff = fileDiff(rel, file.Change, before, now.Data)
			}
		}
		files = append(files, file)
	}
	return files
}

func changedCount(files []ReviewFile) int {
	count := 0
	for _, file := range files {
		if file.Changed {
			count++
		}
	}
	return count
}

// reviewLicense names the license of a skill: the one its SKILL.md states, or
// the first line of a LICENSE file, or that none is stated.
func reviewLicense(stated string, tree map[string]treeFile) string {
	if stated != "" {
		return stated
	}
	names := make([]string, 0, 2)
	for rel := range tree {
		upper := strings.ToUpper(rel)
		if upper == "LICENSE" || upper == "LICENSE.MD" || upper == "LICENSE.TXT" || upper == "LICENCE" {
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	for _, rel := range names {
		for _, line := range strings.Split(string(tree[rel].Data), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return line + " (" + rel + ")"
			}
		}
	}
	return "not stated"
}

// PreviewSkillApproval plans approving the content a person has just read,
// through the same change `skill edit --approve-content` makes. The digest is
// the one the review screen showed; content that changed since is refused.
func (service SkillService) PreviewSkillApproval(ctx context.Context, path, id, contentDigest string) (SkillProposal, error) {
	trust, err := service.ContentTrustFor(ctx, path, id)
	if err != nil {
		return SkillProposal{}, err
	}
	if trust.ContentDigest != contentDigest {
		return SkillProposal{}, NewStaleProposalError(
			"The content changed after the review screen was opened.",
			"Open the review again and read the new content.")
	}
	return service.PreviewSkillUpdate(ctx, path, id, skill.UpdateInput{ContentReviewedDigest: &contentDigest}, false)
}

// contentTrustWithChanges is the content trust of a skill with, for a skill
// that was approved before, what changed since that approval.
func (service SkillService) contentTrustWithChanges(ctx context.Context, path, id string) (ContentTrust, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return ContentTrust{}, err
	}
	_, relDir, meta, err := locateSkillDir(root, id)
	if err != nil {
		return ContentTrust{}, err
	}
	entrypoint, digest, _ := inspectCanonicalEntrypoint(root, relDir)
	_, resources, _ := inventorySkillResources(root, filepath.Join(root, filepath.FromSlash(relDir)), entrypoint, digest)
	trust := reviewContentTrust(id, relDir, resources, meta)
	trust.ChangesSinceApproval = reviewChangesSinceApproval(ctx, approvalDiffInput{Root: root, SkillRelDir: relDir, SkillMetaBytes: meta, Resources: resources, Trust: trust, HistoryLimit: approvalHistoryLimit})
	return trust, nil
}
