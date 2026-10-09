package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/migration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestTrustVerdictAndUpstreamStatusUnchangedAfterMigration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGitInDir(t, root, "init")
	runGitInDir(t, root, "config", "user.name", "SkillHub Test")
	runGitInDir(t, root, "config", "user.email", "test@example.invalid")

	// 1. Third-party approved skill
	tpApprovedDir := filepath.Join(root, "skills", "core", "tp-approved")
	if err := os.MkdirAll(tpApprovedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tpApprovedMD := "# Third Party Approved\n\nApproved third party content.\n"
	if err := os.WriteFile(filepath.Join(tpApprovedDir, "SKILL.md"), []byte(tpApprovedMD), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(tpApprovedMD))
	rd := []skillruntime.ResourceDigest{
		{Path: "SKILL.md", Digest: "sha256:" + hex.EncodeToString(sum[:])},
	}
	expectedDigest := skillruntime.ContentDigest(rd, skillruntime.Spec{}, false)
	tpApprovedMeta := `schema_version: 1
id: tp-approved
name: Third Party Approved
status: active
routing:
  triggers: [tp approved]
  not_for: [none]
  min_scope: single_step
quality:
  reviewed: true
  content_reviewed_digest: ` + expectedDigest + `
provenance:
  source_id: src-tp-approved
  origin:
    kind: github
    repository: https://github.com/example/tp-approved
    commit: 1111111111111111111111111111111111111111
    path: ""
`
	if err := os.WriteFile(filepath.Join(tpApprovedDir, "skill.meta.yaml"), []byte(tpApprovedMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Third-party unapproved skill
	tpUnapprovedDir := filepath.Join(root, "skills", "core", "tp-unapproved")
	if err := os.MkdirAll(tpUnapprovedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tpUnapprovedMD := "# Third Party Unapproved\n\nUnapproved third party content.\n"
	if err := os.WriteFile(filepath.Join(tpUnapprovedDir, "SKILL.md"), []byte(tpUnapprovedMD), 0o644); err != nil {
		t.Fatal(err)
	}
	tpUnapprovedMeta := `schema_version: 1
id: tp-unapproved
name: Third Party Unapproved
status: active
routing:
  triggers: [tp unapproved]
  not_for: [none]
  min_scope: single_step
quality:
  reviewed: false
provenance:
  source_id: src-tp-unapproved
  origin:
    kind: github
    repository: https://github.com/example/tp-unapproved
    commit: 2222222222222222222222222222222222222222
    path: ""
`
	if err := os.WriteFile(filepath.Join(tpUnapprovedDir, "skill.meta.yaml"), []byte(tpUnapprovedMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Self-authored local skill
	localDir := filepath.Join(root, "skills", "core", "local-skill")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localMD := "# Local Skill\n\nSelf-authored local skill content.\n"
	if err := os.WriteFile(filepath.Join(localDir, "SKILL.md"), []byte(localMD), 0o644); err != nil {
		t.Fatal(err)
	}
	localMeta := `schema_version: 1
id: local-skill
name: Local Skill
status: active
routing:
  triggers: [local skill]
  not_for: [none]
  min_scope: single_step
quality:
  reviewed: false
`
	if err := os.WriteFile(filepath.Join(localDir, "skill.meta.yaml"), []byte(localMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Test-audit like skill with existing Phase 0 .meta/skill.yaml
	testAuditDir := filepath.Join(root, "skills", "core", "test-audit")
	if err := os.MkdirAll(filepath.Join(testAuditDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	testAuditMD := "# Test Audit\n\nAudit test skill content.\n"
	if err := os.WriteFile(filepath.Join(testAuditDir, "SKILL.md"), []byte(testAuditMD), 0o644); err != nil {
		t.Fatal(err)
	}
	testAuditOldMeta := `schema_version: 1
id: test-audit
name: Test Audit
status: active
routing:
  triggers: [test audit]
  not_for: [none]
  min_scope: single_step
quality:
  reviewed: false
provenance:
  source_id: src-test-audit
  origin:
    kind: github
    repository: https://github.com/example/audit-upstream
    commit: 4444444444444444444444444444444444444444
`
	if err := os.WriteFile(filepath.Join(testAuditDir, "skill.meta.yaml"), []byte(testAuditOldMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	testAuditPhase0Meta := `schema_version: 1
id: test-audit
status: active
sources:
  - id: distill-lab-source
    roles: [learning]
    repository: https://github.com/example/distill-learning
    commit: 5555555555555555555555555555555555555555
`
	if err := os.WriteFile(filepath.Join(testAuditDir, ".meta", "skill.yaml"), []byte(testAuditPhase0Meta), 0o644); err != nil {
		t.Fatal(err)
	}

	// Initial Git commit
	runGitInDir(t, root, "add", "-A")
	runGitInDir(t, root, "commit", "-m", "initial v2 workspace")

	skillIDs := []string{"tp-approved", "tp-unapproved", "local-skill", "test-audit"}
	type skillFactSnapshot struct {
		ThirdParty     bool
		Approved       bool
		RequiresReview bool
		ContentDigest  string
		ReasonCodes    []string
		UpstreamStatus string
		UpstreamRepo   string
	}

	ctx := context.Background()
	service := SkillService{}
	beforeSnapshots := make(map[string]skillFactSnapshot)

	for _, id := range skillIDs {
		review, err := service.ReviewSkill(ctx, root, id)
		if err != nil {
			t.Fatalf("ReviewSkill(%s) before migration: %v", id, err)
		}
		up, _ := service.GetSkillUpstream(ctx, root, id)
		beforeSnapshots[id] = skillFactSnapshot{
			ThirdParty:     review.ContentTrust.ThirdParty,
			Approved:       review.ContentTrust.Approved,
			RequiresReview: review.ContentTrust.RequiresReview(),
			ContentDigest:  review.ContentTrust.ContentDigest,
			ReasonCodes:    review.ContentTrust.ReasonCodes,
			UpstreamStatus: up.Status,
			UpstreamRepo:   up.Repository,
		}
	}

	// Assert pre-migration baseline
	if !beforeSnapshots["tp-approved"].ThirdParty || !beforeSnapshots["tp-approved"].Approved {
		t.Fatalf("tp-approved unexpected before state: %+v", beforeSnapshots["tp-approved"])
	}
	if !beforeSnapshots["tp-unapproved"].ThirdParty || beforeSnapshots["tp-unapproved"].Approved || !beforeSnapshots["tp-unapproved"].RequiresReview {
		t.Fatalf("tp-unapproved unexpected before state: %+v", beforeSnapshots["tp-unapproved"])
	}
	if beforeSnapshots["local-skill"].ThirdParty || beforeSnapshots["local-skill"].RequiresReview {
		t.Fatalf("local-skill unexpected before state: %+v", beforeSnapshots["local-skill"])
	}
	if !beforeSnapshots["test-audit"].ThirdParty {
		t.Fatalf("test-audit unexpected before state: %+v", beforeSnapshots["test-audit"])
	}

	// Perform migration v2 -> v3
	proposal, err := migration.DefaultRegistry().Preview(root, 3)
	if err != nil {
		t.Fatalf("preview migration v2->v3: %v", err)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm migration v2->v3: %v", err)
	}
	if receipt.OperationID == "" {
		t.Fatal("empty receipt operation ID")
	}

	version, err := migration.DetectVersion(root)
	if err != nil || version != 3 {
		t.Fatalf("expected version 3 after migration, got %d (err %v)", version, err)
	}

	// Verify all skills after migration
	for _, id := range skillIDs {
		review, err := service.ReviewSkill(ctx, root, id)
		if err != nil {
			t.Fatalf("ReviewSkill(%s) after migration: %v", id, err)
		}
		up, _ := service.GetSkillUpstream(ctx, root, id)
		afterSnapshot := skillFactSnapshot{
			ThirdParty:     review.ContentTrust.ThirdParty,
			Approved:       review.ContentTrust.Approved,
			RequiresReview: review.ContentTrust.RequiresReview(),
			ContentDigest:  review.ContentTrust.ContentDigest,
			ReasonCodes:    review.ContentTrust.ReasonCodes,
			UpstreamStatus: up.Status,
			UpstreamRepo:   up.Repository,
		}

		before := beforeSnapshots[id]
		if afterSnapshot.ThirdParty != before.ThirdParty {
			t.Errorf("skill %s: ThirdParty changed from %v to %v", id, before.ThirdParty, afterSnapshot.ThirdParty)
		}
		if afterSnapshot.Approved != before.Approved {
			t.Errorf("skill %s: Approved changed from %v to %v", id, before.Approved, afterSnapshot.Approved)
		}
		if afterSnapshot.RequiresReview != before.RequiresReview {
			t.Errorf("skill %s: RequiresReview changed from %v to %v", id, before.RequiresReview, afterSnapshot.RequiresReview)
		}
		if afterSnapshot.ContentDigest != before.ContentDigest {
			t.Errorf("skill %s: ContentDigest changed from %q to %q", id, before.ContentDigest, afterSnapshot.ContentDigest)
		}
		if !reflect.DeepEqual(afterSnapshot.ReasonCodes, before.ReasonCodes) {
			t.Errorf("skill %s: ReasonCodes changed from %v to %v", id, before.ReasonCodes, afterSnapshot.ReasonCodes)
		}
		if afterSnapshot.UpstreamStatus != before.UpstreamStatus {
			t.Errorf("skill %s: UpstreamStatus changed from %q to %q", id, before.UpstreamStatus, afterSnapshot.UpstreamStatus)
		}
		if afterSnapshot.UpstreamRepo != before.UpstreamRepo {
			t.Errorf("skill %s: UpstreamRepo changed from %q to %q", id, before.UpstreamRepo, afterSnapshot.UpstreamRepo)
		}
	}
}
