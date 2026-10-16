package app

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/workspace"
	"gopkg.in/yaml.v3"
)

// operationsDir holds the record of every applied change, one file each.
const operationsDir = "history/operations"

// unfinishedCommitPaths lists the hub files of a skill that were written but
// never committed, for example because Git refused the commit, together with
// the records of the operations that wrote them. It is empty when the skill's
// folder matches Git.
func unfinishedCommitPaths(ctx context.Context, root, relDir string) ([]string, []string, error) {
	skillPaths, err := workspace.DirtyPaths(ctx, root, []string{relDir})
	if err != nil || len(skillPaths) == 0 {
		return nil, nil, err
	}
	records, err := workspace.DirtyPaths(ctx, root, []string{operationsDir})
	if err != nil {
		return nil, nil, err
	}
	paths := append([]string{}, skillPaths...)
	var operations []string
	for _, record := range records {
		touches, err := operationTouches(root, record, relDir)
		if err != nil {
			return nil, nil, err
		}
		if touches {
			paths = append(paths, record)
			operations = append(operations, strings.TrimSuffix(path.Base(record), ".yaml"))
		}
	}
	sort.Strings(paths)
	sort.Strings(operations)
	return paths, operations, nil
}

// operationTouches reports whether the operation record at rel changed a file
// under relDir. A record that is gone (a deleted file Git still tracks) does not.
func operationTouches(root, rel, relDir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read the operation record %s: %w", rel, err)
	}
	var record struct {
		Changes []struct {
			Path string `yaml:"path"`
		} `yaml:"changes"`
	}
	if err := yaml.Unmarshal(data, &record); err != nil {
		// A record Skill Hub cannot read is not one it wrote for this skill.
		return false, nil
	}
	for _, change := range record.Changes {
		if strings.HasPrefix(change.Path, relDir+"/") {
			return true, nil
		}
	}
	return false, nil
}

// finishUnfinishedCommit commits the files an earlier change of the skill
// wrote but could not commit. With --no-commit it leaves them as they are.
func finishUnfinishedCommit(ctx context.Context, plan *commitPlan, preview SkillCommitPreview) SkillCommitResult {
	result := SkillCommitResult{SkillID: plan.id, Case: plan.caseName, Collection: plan.collection, ChangedPaths: preview.Uncommitted, Files: []SkillCommitFile{}, GitDirty: true}
	policy, asked := workspace.CommitPolicyFrom(ctx)
	if !asked || policy.Disabled {
		result.Result = NewResult(StatusOK, fmt.Sprintf("Nothing committed: %s matches the hub, and its files there stay uncommitted in Git.", plan.id))
		return result
	}
	message := fmt.Sprintf("Commit %s from a project (written earlier, not committed then)", plan.id)
	if policy.Message != "" {
		message = policy.Message
	}
	if len(preview.uncommittedOps) > 0 {
		message += "\n\nSkillhub-Operation: " + strings.Join(preview.uncommittedOps, "\nSkillhub-Operation: ")
	}
	lock, err := mutation.AcquireCommitLock(ctx, plan.root)
	var outcome workspace.CommitOutcome
	if err != nil {
		outcome = workspace.CommitOutcome{Status: workspace.CommitFailed, Message: message, Reason: "Another change was still being committed: " + err.Error()}
	} else {
		outcome = workspace.CommitPaths(ctx, plan.root, preview.Uncommitted, nil, message, policy.Interactive)
		_ = lock.Unlock()
	}
	result.Commit = &outcome
	if outcome.Status == workspace.CommitCommitted {
		result.Result = NewResult(StatusApplied, fmt.Sprintf("Committed the earlier change of %s.", plan.id))
		result.GitDirty = false
	} else {
		result.Result = NewResult(StatusActionRequired, fmt.Sprintf("The earlier change of %s is still not committed.", plan.id))
	}
	result.Items = append(result.Items, commitItem(outcome, result.GitDirty))
	return result
}
