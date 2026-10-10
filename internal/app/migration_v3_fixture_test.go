package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

	// A snapshot of the real hub at schema version 1 (vantt/skill-hub@0f88983),
	// taken before that hub was migrated.
	liveHub := filepath.Join("testdata", "live-hub-v1")

	// Copy the fixture files into a fresh workspace
	filesToCopy := []string{
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
		if err != nil && errors.Is(err, os.ErrNotExist) {
			cmd := exec.Command("git", "-C", liveHub, "show", "0f88983:"+rel)
			if out, cmdErr := cmd.Output(); cmdErr == nil {
				data = out
				err = nil
			}
		}
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
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("1\n"), 0o644)

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
	type routingSnapshot struct {
		Operations []string `yaml:"operations"`
		Triggers   []string `yaml:"triggers"`
		NotFor     []string `yaml:"not_for"`
		MinScope   string   `yaml:"min_scope"`
	}
	type skillFactSnapshot struct {
		ThirdParty     bool
		Approved       bool
		RequiresReview bool
		ContentDigest  string
		ReasonCodes    []string
		UpstreamStatus string
		UpstreamRepo   string
		Name           string
		Description    string
		Status         string
		Routing        routingSnapshot
		Resources      []ResourceItem
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
		_, _, metaBytes, _ := locateSkillDir(root, id)
		var metaDoc struct {
			Routing routingSnapshot `yaml:"routing"`
		}
		_ = yaml.Unmarshal(metaBytes, &metaDoc)

		var cleanResources []ResourceItem
		for _, r := range review.ResourceStatus.Resources {
			if !workspace.IsHubMeta(r.Path) {
				cleanResources = append(cleanResources, r)
			}
		}

		beforeSnapshots[id] = skillFactSnapshot{
			ThirdParty:     review.ContentTrust.ThirdParty,
			Approved:       review.ContentTrust.Approved,
			RequiresReview: review.ContentTrust.RequiresReview(),
			ContentDigest:  review.ContentTrust.ContentDigest,
			ReasonCodes:    review.ContentTrust.ReasonCodes,
			UpstreamStatus: up.Status,
			UpstreamRepo:   up.Repository,
			Name:           review.Name,
			Description:    review.Description,
			Status:         review.LifecycleState,
			Routing:        metaDoc.Routing,
			Resources:      cleanResources,
		}
	}

	// Verify v1 baseline expectations:
	// test-audit is third-party (from openclaw.git), reviewed: true, name is "test-audit"
	if !beforeSnapshots["test-audit"].ThirdParty {
		t.Fatalf("test-audit must be third-party in v1: %+v", beforeSnapshots["test-audit"])
	}
	if beforeSnapshots["test-audit"].Name != "test-audit" {
		t.Fatalf("test-audit name in v1 = %q, want %q", beforeSnapshots["test-audit"].Name, "test-audit")
	}

	// markdown-to-epub is local authoring (created_by: skillhub, no origin), name is "Markdown to Epub"
	if beforeSnapshots["markdown-to-epub"].ThirdParty {
		t.Fatalf("markdown-to-epub must be local in v1: %+v", beforeSnapshots["markdown-to-epub"])
	}
	if beforeSnapshots["markdown-to-epub"].Name != "Markdown to Epub" {
		t.Fatalf("markdown-to-epub name in v1 = %q, want %q", beforeSnapshots["markdown-to-epub"].Name, "Markdown to Epub")
	}

	// herdr-cook-plan is third-party (from herdr-cook-plan.git), name is "herdr-cook-plan"
	if !beforeSnapshots["herdr-cook-plan"].ThirdParty {
		t.Fatalf("herdr-cook-plan must be third-party in v1: %+v", beforeSnapshots["herdr-cook-plan"])
	}
	if beforeSnapshots["herdr-cook-plan"].Name != "herdr-cook-plan" {
		t.Fatalf("herdr-cook-plan name in v1 = %q, want %q", beforeSnapshots["herdr-cook-plan"].Name, "herdr-cook-plan")
	}
	if !strings.HasPrefix(beforeSnapshots["herdr-cook-plan"].Description, "Run an existing AgentKit plan") {
		t.Fatalf("herdr-cook-plan description corrupted in v1: %q", beforeSnapshots["herdr-cook-plan"].Description)
	}

	// Step 1: Migrate v1 -> v4 (chains planV1ToV2, planV2ToV3, and planV3ToV4)
	proposal, err := migration.DefaultRegistry().Preview(root, 4)
	if err != nil {
		t.Fatalf("preview migration v1->v4: %v", err)
	}
	if proposal.SourceVersion != 1 || proposal.TargetVersion != 4 {
		t.Fatalf("expected proposal 1 -> 4, got %d -> %d", proposal.SourceVersion, proposal.TargetVersion)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm migration v1->v4: %v", err)
	}
	if receipt.OperationID == "" {
		t.Fatal("empty receipt operation ID for v1->v4")
	}
	v4Version, err := migration.DetectVersion(root)
	if err != nil || v4Version != 4 {
		t.Fatalf("expected version 4 after v1->v4, got %d (err: %v)", v4Version, err)
	}

	// Step 2: Verify all 3 skills in v3 match their v1 baselines completely
	for _, id := range skillIDs {
		review, err := service.ReviewSkill(ctx, root, id)
		if err != nil {
			t.Fatalf("ReviewSkill(%s) in v3: %v", id, err)
		}
		up, _ := service.GetSkillUpstream(ctx, root, id)
		_, _, metaBytes, _ := locateSkillDir(root, id)
		var metaDoc struct {
			Routing routingSnapshot `yaml:"routing"`
		}
		_ = yaml.Unmarshal(metaBytes, &metaDoc)

		var cleanResources []ResourceItem
		for _, r := range review.ResourceStatus.Resources {
			if !workspace.IsHubMeta(r.Path) {
				cleanResources = append(cleanResources, r)
			}
		}

		afterSnapshot := skillFactSnapshot{
			ThirdParty:     review.ContentTrust.ThirdParty,
			Approved:       review.ContentTrust.Approved,
			RequiresReview: review.ContentTrust.RequiresReview(),
			ContentDigest:  review.ContentTrust.ContentDigest,
			ReasonCodes:    review.ContentTrust.ReasonCodes,
			UpstreamStatus: up.Status,
			UpstreamRepo:   up.Repository,
			Name:           review.Name,
			Description:    review.Description,
			Status:         review.LifecycleState,
			Routing:        metaDoc.Routing,
			Resources:      cleanResources,
		}

		before := beforeSnapshots[id]
		if afterSnapshot.Name != before.Name {
			t.Errorf("skill %s: Name changed from %q to %q", id, before.Name, afterSnapshot.Name)
		}
		if afterSnapshot.Description != before.Description {
			t.Errorf("skill %s: Description changed from %q to %q", id, before.Description, afterSnapshot.Description)
		}
		if afterSnapshot.Status != before.Status {
			t.Errorf("skill %s: Status changed from %q to %q", id, before.Status, afterSnapshot.Status)
		}
		if !reflect.DeepEqual(afterSnapshot.Routing, before.Routing) {
			t.Errorf("skill %s: Routing changed from %+v to %+v", id, before.Routing, afterSnapshot.Routing)
		}
		if !reflect.DeepEqual(afterSnapshot.Resources, before.Resources) {
			t.Errorf("skill %s: Resources changed from %+v to %+v", id, before.Resources, afterSnapshot.Resources)
		}
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
	// Item 3: Sources write exactly one key, repo (D2), never repository
	if openclawSrc.Repo != "https://github.com/openclaw/openclaw" {
		t.Fatalf("openclaw repo = %q, want https://github.com/openclaw/openclaw", openclawSrc.Repo)
	}
	if openclawSrc.Repository != "" {
		t.Fatalf("openclaw must not write duplicate repository key; got %q", openclawSrc.Repository)
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
	if superpowersSrc.Repo != "https://github.com/obra/superpowers" {
		t.Fatalf("superpowers repo = %q, want https://github.com/obra/superpowers", superpowersSrc.Repo)
	}
	if superpowersSrc.Repository != "" {
		t.Fatalf("superpowers must not write duplicate repository key; got %q", superpowersSrc.Repository)
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
