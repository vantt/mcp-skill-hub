package app

import (
	"bytes"
	"sort"

	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
)

// submissionDepsKey fills the dependency key every bundle carries. A
// submission has no installed dependencies, so the key is a constant.
const submissionDepsKey = "0000000000000000"

// BuildSubmissionBundle writes the archive a machine sends to a hub for
// review: the skill's content files as a bundle, with the details of the
// submission in its metadata.
func BuildSubmissionBundle(skillID string, tree map[string]treeFile, details skillruntime.BundleSubmission) ([]byte, error) {
	paths := make([]string, 0, len(tree))
	for rel := range tree {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	files := make([]skillruntime.BundleFile, 0, len(paths))
	contents := make(map[string][]byte, len(paths))
	for _, rel := range paths {
		file := tree[rel]
		files = append(files, skillruntime.BundleFile{Path: rel, Digest: sourcepkg.Digest(file.Data), Size: int64(len(file.Data)), Executable: file.Executable})
		contents[rel] = file.Data
	}
	digest, err := skillruntime.ManifestDigest(files)
	if err != nil {
		return nil, err
	}
	metadata := skillruntime.BundleMetadata{
		Schema: skillruntime.BundleSchema, SkillID: skillID, ManifestVersion: "sha256:" + digest,
		Files: files, DepsKey: submissionDepsKey, Submission: &details,
	}
	var archive bytes.Buffer
	if err := skillruntime.WriteBundle(&archive, metadata, contents); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

// BuildSubmissionBundleFromFiles is BuildSubmissionBundle for callers that hold
// plain file contents, with no execute bits.
func BuildSubmissionBundleFromFiles(skillID string, files map[string][]byte, details skillruntime.BundleSubmission) ([]byte, error) {
	tree := make(map[string]treeFile, len(files))
	for rel, data := range files {
		tree[rel] = treeFile{Data: data}
	}
	return BuildSubmissionBundle(skillID, tree, details)
}
