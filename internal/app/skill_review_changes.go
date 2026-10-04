package app

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

// approvalHistoryLimit bounds how many commits of a skill's manifest history
// the review walks to find the approved content.
const approvalHistoryLimit = 200

const approvalGitTimeout = 20 * time.Second

const skillManifestName = "skill.meta.yaml"

// ChangesSinceApproval describes how a third-party skill's folder differs from
// the content a human last approved, found in Git history. It is present only
// when an approval is recorded in the manifest. When the approved content
// cannot be located, Found is false and nothing else is reported.
type ChangesSinceApproval struct {
	Found               bool     `json:"found"`
	Commit              string   `json:"commit,omitempty"`
	DiffCommand         string   `json:"diff_command,omitempty"`
	Added               []string `json:"added"`
	Removed             []string `json:"removed"`
	Modified            []string `json:"modified"`
	RuntimeChanged      bool     `json:"runtime_changed"`
	ScriptsChanged      bool     `json:"scripts_changed"`
	DependenciesChanged bool     `json:"dependencies_changed"`
	HistoryTruncated    bool     `json:"history_truncated"`
}

// reviewChangesSinceApproval compares the skill folder with the most recent
// approved state in Git history. It returns nil when the skill has no recorded
// approval to compare against, when it is currently approved, or when Git is
// unavailable. The approved state is the oldest commit of the unbroken run of
// manifest commits that carry the recorded digest: that is where the approval
// was written, so later edits that kept the digest cannot hide changes.
// approvalDiffInput identifies the reviewed skill and bounds the history walk.
type approvalDiffInput struct {
	Root           string
	SkillRelDir    string
	SkillMetaBytes []byte
	Resources      []ResourceItem
	Trust          ContentTrust
	HistoryLimit   int
}

func reviewChangesSinceApproval(ctx context.Context, input approvalDiffInput) *ChangesSinceApproval {
	root, skillRelDir, skillMetaBytes, resources, trust, historyLimit := input.Root, input.SkillRelDir, input.SkillMetaBytes, input.Resources, input.Trust, input.HistoryLimit
	var document contentTrustDocument
	if err := yaml.Unmarshal(skillMetaBytes, &document); err != nil {
		return nil
	}
	approved := document.Quality.ContentReviewedDigest
	if !trust.ThirdParty || approved == "" || trust.Approved {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, approvalGitTimeout)
	defer cancel()

	metaPath := skillRelDir + "/" + skillManifestName
	commits, ok := gitLines(ctx, root, "log", "--format=%H", fmt.Sprintf("--max-count=%d", historyLimit+1), "--", metaPath)
	if !ok {
		return nil
	}
	changes := &ChangesSinceApproval{Added: []string{}, Removed: []string{}, Modified: []string{}}
	if len(commits) > historyLimit {
		changes.HistoryTruncated = true
		commits = commits[:historyLimit]
	}
	approvalCommit, approvedMeta := "", []byte(nil)
	runEndedInHistory := false
	for _, commit := range commits {
		meta, err := gitOutput(ctx, root, "show", commit+":./"+metaPath)
		var past contentTrustDocument
		matches := err == nil && yaml.Unmarshal(meta, &past) == nil && past.Quality.ContentReviewedDigest == approved
		if matches {
			approvalCommit, approvedMeta = commit, meta
			continue
		}
		if approvalCommit != "" {
			runEndedInHistory = true
			break
		}
	}
	// A run that reaches the end of a truncated walk may continue further back,
	// so its oldest visible commit is not known to be the approval.
	if approvalCommit == "" || (changes.HistoryTruncated && !runEndedInHistory) {
		return changes
	}
	approvedFiles, ok := gitTreeBlobs(ctx, root, approvalCommit, skillRelDir)
	if !ok {
		return changes
	}
	currentFiles, ok := currentBlobs(ctx, root, skillRelDir, resources)
	if !ok {
		return changes
	}
	changes.Found = true
	changes.Commit = approvalCommit
	changes.DiffCommand = fmt.Sprintf("git diff %s -- %s", approvalCommit[:12], skillRelDir)
	for name, blob := range currentFiles {
		switch before, existed := approvedFiles[name]; {
		case !existed:
			changes.Added = append(changes.Added, name)
		case before != blob:
			changes.Modified = append(changes.Modified, name)
		}
	}
	for name := range approvedFiles {
		if _, kept := currentFiles[name]; !kept {
			changes.Removed = append(changes.Removed, name)
		}
	}
	slices.Sort(changes.Added)
	slices.Sort(changes.Removed)
	slices.Sort(changes.Modified)

	for _, name := range slices.Concat(changes.Added, changes.Removed, changes.Modified) {
		if skillruntime.IsDependencyManifest(name) {
			changes.DependenciesChanged = true
		}
		if strings.HasPrefix(name, "scripts/") || skillruntime.InterpreterOf(name, readLeadingBytes(filepath.Join(root, filepath.FromSlash(skillRelDir), filepath.FromSlash(name)), hintReadLimit)) != "" {
			changes.ScriptsChanged = true
		}
	}
	pastSpec, pastHas := reviewRuntimeSpec(approvedMeta)
	currentSpec, currentHas := reviewRuntimeSpec(skillMetaBytes)
	changes.RuntimeChanged = pastHas != currentHas || !reflect.DeepEqual(pastSpec, currentSpec)
	return changes
}

// HasChanges reports whether anything differs from the approved content.
func (changes ChangesSinceApproval) HasChanges() bool {
	return len(changes.Added)+len(changes.Removed)+len(changes.Modified) > 0 || changes.RuntimeChanged
}

// gitTreeBlobs maps each file of the skill folder at commit (relative to the
// folder, manifest excluded) to its Git object id.
func gitTreeBlobs(ctx context.Context, root, commit, skillRelDir string) (map[string]string, bool) {
	output, err := gitOutput(ctx, root, "ls-tree", "-r", "-z", commit, "--", skillRelDir)
	if err != nil {
		return nil, false
	}
	blobs := map[string]string{}
	prefix := skillRelDir + "/"
	for _, record := range bytes.Split(output, []byte{0}) {
		meta, name, found := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if !found || len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		relative, inside := strings.CutPrefix(string(name), prefix)
		if inside && relative != skillManifestName {
			blobs[relative] = fields[2]
		}
	}
	return blobs, true
}

// currentBlobs maps each current skill file (manifest excluded) to the Git
// object id its working-tree content would have.
func currentBlobs(ctx context.Context, root, skillRelDir string, resources []ResourceItem) (map[string]string, bool) {
	prefix := skillRelDir + "/"
	var names, paths []string
	for _, resource := range resources {
		relative, inside := strings.CutPrefix(resource.Path, prefix)
		if !inside || relative == skillManifestName || strings.ContainsAny(resource.Path, "\r\n") {
			continue
		}
		names = append(names, relative)
		paths = append(paths, resource.Path)
	}
	blobs := map[string]string{}
	if len(paths) == 0 {
		return blobs, true
	}
	command := offlineGitCommand(ctx, root, "hash-object", "--stdin-paths")
	command.Stdin = strings.NewReader(strings.Join(paths, "\n") + "\n")
	output, err := command.Output()
	if err != nil {
		return nil, false
	}
	ids := strings.Fields(string(output))
	if len(ids) != len(names) {
		return nil, false
	}
	for index, name := range names {
		blobs[path.Clean(name)] = ids[index]
	}
	return blobs, true
}

func gitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	return offlineGitCommand(ctx, root, args...).Output()
}

func gitLines(ctx context.Context, root string, args ...string) ([]string, bool) {
	output, err := gitOutput(ctx, root, args...)
	if err != nil {
		return nil, false
	}
	return strings.Fields(string(output)), true
}
