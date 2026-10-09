package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/migration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestRealHubFixtureMigrationV1ToV2ToV3(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	liveHub := "/home/vantt/skill-hub"
	if _, err := os.Stat(liveHub); err != nil {
		t.Skipf("live hub not found at %s: %v", liveHub, err)
	}

	// Copy fixture files read-only from /home/vantt/skill-hub
	filesToCopy := []string{
		".skillhub/schema-version",
		"skills/default/test-audit/SKILL.md",
		"skills/default/test-audit/skill.meta.yaml",
		"skills/default/test-audit/.meta/skill.yaml",
		"skills/docs/markdown-to-epub/SKILL.md",
		"skills/docs/markdown-to-epub/skill.meta.yaml",
		"skills/default/herdr-cook-plan/SKILL.md",
		"skills/default/herdr-cook-plan/skill.meta.yaml",
	}

	for _, rel := range filesToCopy {
		src := filepath.Join(liveHub, filepath.FromSlash(rel))
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s from live hub: %v", src, err)
		}
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runGitInDir(t, root, "init")
	runGitInDir(t, root, "config", "user.name", "SkillHub Test")
	runGitInDir(t, root, "config", "user.email", "test@example.invalid")
	runGitInDir(t, root, "add", "-A")
	runGitInDir(t, root, "commit", "-m", "initial v1 workspace from live hub fixture")

	// Verify starting schema version is 1
	initVersion, err := migration.DetectVersion(root)
	if err != nil || initVersion != 1 {
		t.Fatalf("expected initial schema version 1, got %d (err: %v)", initVersion, err)
	}

	skillIDs := []string{"test-audit", "markdown-to-epub", "herdr-cook-plan"}
	type skillFactSnapshot struct {
		ThirdParty     bool
		Approved       bool
		RequiresReview bool
		ContentDigest  string
		ReasonCodes    []string
		UpstreamStatus string
		UpstreamRepo   string
		Name           string
	}

	ctx := context.Background()
	service := SkillService{}
	beforeSnapshots := make(map[string]skillFactSnapshot)

	for _, id := range skillIDs {
		review, err := service.ReviewSkill(ctx, root, id)
		if err != nil {
			t.Fatalf("ReviewSkill(%s) in v1: %v", id, err)
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
			Name:           review.Name,
		}
	}

	// Verify v1 baseline expectations:
	// test-audit is third-party (from openclaw.git), reviewed: true
	if !beforeSnapshots["test-audit"].ThirdParty {
		t.Fatalf("test-audit must be third-party in v1: %+v", beforeSnapshots["test-audit"])
	}
	// markdown-to-epub is local authoring (created_by: skillhub, no origin), name is "Markdown to Epub"
	if beforeSnapshots["markdown-to-epub"].ThirdParty {
		t.Fatalf("markdown-to-epub must be local in v1: %+v", beforeSnapshots["markdown-to-epub"])
	}
	if beforeSnapshots["markdown-to-epub"].Name != "Markdown to Epub" {
		t.Fatalf("markdown-to-epub name in v1 = %q, want %q", beforeSnapshots["markdown-to-epub"].Name, "Markdown to Epub")
	}
	// herdr-cook-plan is third-party (from herdr-cook-plan.git)
	if !beforeSnapshots["herdr-cook-plan"].ThirdParty {
		t.Fatalf("herdr-cook-plan must be third-party in v1: %+v", beforeSnapshots["herdr-cook-plan"])
	}

	// Step 1: Migrate v1 -> v3 (chains planV1ToV2 and planV2ToV3)
	proposal, err := migration.DefaultRegistry().Preview(root, 3)
	if err != nil {
		t.Fatalf("preview migration v1->v3: %v", err)
	}
	if proposal.SourceVersion != 1 || proposal.TargetVersion != 3 {
		t.Fatalf("expected proposal 1 -> 3, got %d -> %d", proposal.SourceVersion, proposal.TargetVersion)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm migration v1->v3: %v", err)
	}
	if receipt.OperationID == "" {
		t.Fatal("empty receipt operation ID for v1->v3")
	}
	v3Version, err := migration.DetectVersion(root)
	if err != nil || v3Version != 3 {
		t.Fatalf("expected version 3 after v1->v3, got %d (err: %v)", v3Version, err)
	}

	// Step 3: Verify all 3 skills in v3 match their v1 baselines
	for _, id := range skillIDs {
		review, err := service.ReviewSkill(ctx, root, id)
		if err != nil {
			t.Fatalf("ReviewSkill(%s) in v3: %v", id, err)
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
			Name:           review.Name,
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
		if id == "markdown-to-epub" && afterSnapshot.Name != "Markdown to Epub" {
			t.Errorf("markdown-to-epub must preserve custom name 'Markdown to Epub', got %q", afterSnapshot.Name)
		}
		if id == "test-audit" && afterSnapshot.Name != "Test Audit" {
			t.Errorf("test-audit must derive 'Test Audit' from SKILL.md H1, got %q", afterSnapshot.Name)
		}
		if id == "herdr-cook-plan" && afterSnapshot.Name != "Herdr Cook Plan" {
			t.Errorf("herdr-cook-plan must derive 'Herdr Cook Plan' from SKILL.md H1, got %q", afterSnapshot.Name)
		}
	}

	// Verify test-audit merged .meta/skill.yaml
	testAuditMetaPath := filepath.Join(root, "skills", "default", "test-audit", ".meta", "skill.yaml")
	metaBytes, err := os.ReadFile(testAuditMetaPath)
	if err != nil {
		t.Fatalf("read test-audit .meta/skill.yaml: %v", err)
	}

	var testAuditDoc struct {
		Sources []struct {
			ID         string   `yaml:"id"`
			Repo       string   `yaml:"repo"`
			Repository string   `yaml:"repository"`
			Roles      []string `yaml:"roles"`
			Synced     string   `yaml:"synced"`
			LearnPaths []string `yaml:"learn_paths"`
		} `yaml:"sources"`
	}
	if err := yaml.Unmarshal(metaBytes, &testAuditDoc); err != nil {
		t.Fatalf("unmarshal test-audit .meta/skill.yaml: %v", err)
	}

	if len(testAuditDoc.Sources) != 2 {
		t.Fatalf("expected 2 merged sources in test-audit, got %d: %#v", len(testAuditDoc.Sources), testAuditDoc.Sources)
	}

	var openclawSrc *struct {
		ID         string   `yaml:"id"`
		Repo       string   `yaml:"repo"`
		Repository string   `yaml:"repository"`
		Roles      []string `yaml:"roles"`
		Synced     string   `yaml:"synced"`
		LearnPaths []string `yaml:"learn_paths"`
	}
	var superpowersSrc *struct {
		ID         string   `yaml:"id"`
		Repo       string   `yaml:"repo"`
		Repository string   `yaml:"repository"`
		Roles      []string `yaml:"roles"`
		Synced     string   `yaml:"synced"`
		LearnPaths []string `yaml:"learn_paths"`
	}

	for i := range testAuditDoc.Sources {
		if testAuditDoc.Sources[i].ID == "openclaw" {
			openclawSrc = &testAuditDoc.Sources[i]
		}
		if testAuditDoc.Sources[i].ID == "superpowers" {
			superpowersSrc = &testAuditDoc.Sources[i]
		}
	}

	if openclawSrc == nil {
		t.Fatal("openclaw source not found in merged test-audit")
	}
	hasUpstream, hasLearning := false, false
	for _, r := range openclawSrc.Roles {
		if r == "upstream" {
			hasUpstream = true
		}
		if r == "learning" {
			hasLearning = true
		}
	}
	if !hasUpstream || !hasLearning {
		t.Fatalf("openclaw roles must have both upstream and learning, got: %v", openclawSrc.Roles)
	}
	if openclawSrc.Synced != "90563ee83bd60ece1d2819bc09015c01c81063c4" {
		t.Fatalf("openclaw synced cursor = %q, want 90563ee83bd60ece1d2819bc09015c01c81063c4", openclawSrc.Synced)
	}
	if len(openclawSrc.LearnPaths) != 6 {
		t.Fatalf("openclaw learn_paths lost; got %d entries: %v", len(openclawSrc.LearnPaths), openclawSrc.LearnPaths)
	}

	if superpowersSrc == nil {
		t.Fatal("superpowers source not found in merged test-audit")
	}
	if len(superpowersSrc.Roles) != 1 || superpowersSrc.Roles[0] != "learning" {
		t.Fatalf("superpowers roles = %v, want [learning]", superpowersSrc.Roles)
	}

	// Verify markdown-to-epub retained its custom name
	mteMetaBytes, err := os.ReadFile(filepath.Join(root, "skills", "docs", "markdown-to-epub", ".meta", "skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mteMetaBytes), "name: Markdown to Epub") {
		t.Fatalf("markdown-to-epub did not preserve custom name in .meta/skill.yaml:\n%s", string(mteMetaBytes))
	}
}
