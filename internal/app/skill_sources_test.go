package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestSkillSources(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	skillService := SkillService{}
	sourceService := SourceService{Clock: sourceClock{now: now}}
	ctx := context.Background()

	// 1. Local skill: Upstream == nil, empty Learning
	localPrev, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "local-skill",
		Collection:  "default",
		Name:        "Local Skill",
		Description: "Local skill without upstream or learning",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(ctx, root, localPrev, localPrev.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	resLocal, err := sourceService.SkillSources(ctx, root, "local-skill")
	if err != nil {
		t.Fatalf("SkillSources for local skill failed: %v", err)
	}
	if resLocal.Upstream != nil {
		t.Fatalf("expected nil Upstream for local skill, got %#v", resLocal.Upstream)
	}
	if len(resLocal.Learning) != 0 {
		t.Fatalf("expected empty Learning for local skill, got %#v", resLocal.Learning)
	}

	// 2. Skill with learning link: one reference with role "learning-source"
	linkPrev, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "learner-skill",
		Collection:  "default",
		Name:        "Learner Skill",
		Description: "Skill with a learning link",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(ctx, root, linkPrev, linkPrev.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	// Create source record
	rec := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "src-docs",
		Adapter:       "git",
		Locator:       sourcepkg.Locator{Repository: "https://github.com/example/docs.git", Ref: "main"},
		Status:        "watching",
		Identity:      sourcepkg.Identity{Name: "docs", Canonical: "https://github.com/example/docs.git", DefaultBranch: "main"},
		Monitoring:    sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:        sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
	}
	recBytes, _ := sourcepkg.MarshalCanonical(rec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-docs.yaml"), recBytes, 0o644)

	// Create link
	linkDoc := sourcepkg.Link{
		SchemaVersion: 1,
		ID:            "LINK-learner-skill--src-docs",
		SkillID:       "learner-skill",
		SourceID:      "src-docs",
		Role:          "learning-source",
	}
	linkBytes, _ := sourcepkg.MarshalCanonical(linkDoc)
	_ = os.WriteFile(filepath.Join(root, "sources", "skills", "LINK-learner-skill--src-docs.yaml"), linkBytes, 0o644)
	commitWorkspace(t, root)
	_, _ = catalog.BuildCatalogGeneration(ctx, root, catalog.BuildOptions{})

	resLearner, err := sourceService.SkillSources(ctx, root, "learner-skill")
	if err != nil {
		t.Fatalf("SkillSources for learner skill failed: %v", err)
	}
	if resLearner.Upstream != nil {
		t.Fatalf("expected nil Upstream for learner skill, got %#v", resLearner.Upstream)
	}
	if len(resLearner.Learning) != 1 {
		t.Fatalf("expected 1 Learning reference, got %d", len(resLearner.Learning))
	}
	ref := resLearner.Learning[0]
	if ref.SourceID != "src-docs" || ref.Role != "learning-source" || ref.Locator != "https://github.com/example/docs.git" {
		t.Fatalf("unexpected learning reference: %#v", ref)
	}

	// 3. Test ListSkillsWithUpstream
	listRes, err := skillService.ListSkillsWithUpstream(ctx, root, "")
	if err != nil {
		t.Fatalf("ListSkillsWithUpstream failed: %v", err)
	}
	if len(listRes.Skills) < 2 {
		t.Fatalf("expected at least 2 skills, got %d", len(listRes.Skills))
	}
}
