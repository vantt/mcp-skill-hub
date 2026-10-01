package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func TestReviewSkillUntouchedDraft(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}

	// Create a draft with no content (generates scaffold)
	created, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "review-scaffold",
		Collection:  "software",
		Name:        "Review Scaffold",
		Description: "A scaffold draft skill.",
		Routing:     skill.RoutingInput{Triggers: []string{"scaffold"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, created, created.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	review, err := service.ReviewSkill(ctx, root, "review-scaffold")
	if err != nil {
		t.Fatalf("ReviewSkill failed: %v", err)
	}

	if review.SkillID != "review-scaffold" {
		t.Fatalf("review.SkillID = %q, want review-scaffold", review.SkillID)
	}
	if review.LifecycleState != "draft" {
		t.Fatalf("review.LifecycleState = %q, want draft", review.LifecycleState)
	}
	if review.ActiveLocally {
		t.Fatal("review.ActiveLocally should be false for draft")
	}
	if review.RoutingEligible {
		t.Fatal("review.RoutingEligible should be false for draft")
	}
	if !review.Valid {
		t.Fatalf("review.Valid = false, canonical issues: %v", review.CanonicalIssues)
	}
	if !review.ActivationReadiness.UntouchedScaffold {
		t.Fatal("expected ActivationReadiness.UntouchedScaffold to be true")
	}
	if review.ActivationReadiness.Ready {
		t.Fatal("expected ActivationReadiness.Ready to be false for untouched scaffold")
	}
	if !strings.Contains(review.NextAction, "skill edit") || !strings.Contains(review.NextAction, "--editor") {
		t.Fatalf("unexpected next action for scaffold draft: %s", review.NextAction)
	}
}

func TestReviewSkillReadyDraft(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}

	// Create a draft with real instructions
	created, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "review-ready",
		Collection:  "software",
		Name:        "Review Ready",
		Description: "A ready draft skill.",
		Content:     []byte("# Real Instructions\n\nExecute safely with verification.\n"),
		Routing:     skill.RoutingInput{Triggers: []string{"ready"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, created, created.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	review, err := service.ReviewSkill(ctx, root, "review-ready")
	if err != nil {
		t.Fatalf("ReviewSkill failed: %v", err)
	}
	if review.ActivationReadiness.UntouchedScaffold {
		t.Fatal("review.ActivationReadiness.UntouchedScaffold should be false")
	}
	if !review.ActivationReadiness.Ready {
		t.Fatalf("review.ActivationReadiness.Ready should be true; missing: %v", review.ActivationReadiness.MissingFields)
	}
	if !strings.Contains(review.NextAction, "skill activate") {
		t.Fatalf("unexpected next action for ready draft: %s", review.NextAction)
	}
}

func TestReviewSkillActiveAndDivergence(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}

	createAndActivateSkill(t, service, root)

	// Review active skill before any external changes
	review, err := service.ReviewSkill(ctx, root, "consumer-review")
	if err != nil {
		t.Fatalf("ReviewSkill failed: %v", err)
	}
	if review.LifecycleState != "active" {
		t.Fatalf("review.LifecycleState = %q, want active", review.LifecycleState)
	}
	if !review.ActiveLocally {
		t.Fatal("review.ActiveLocally should be true for active skill")
	}
	if !review.RoutingEligible {
		t.Fatal("review.RoutingEligible should be true for active skill")
	}
	if review.Diverged {
		t.Fatal("review should not be diverged initially")
	}
	if !review.ServedFacts.Servable {
		t.Fatalf("ServedFacts.Servable should be true; reason: %s", review.ServedFacts.ServableReason)
	}

	// 1. Break another skill in workspace so workspace canonical validation fails
	// and catalog falls back to the published pointer
	brokenDir := filepath.Join(root, "skills", "software", "broken")
	_ = os.MkdirAll(brokenDir, 0o755)
	_ = os.WriteFile(filepath.Join(brokenDir, "skill.meta.yaml"), []byte("invalid-yaml: ["), 0o600)

	// Modify consumer-review canonical SKILL.md without rebuilding catalog
	entrypointPath := filepath.Join(root, "skills", "software", "consumer-review", "SKILL.md")
	content, err := os.ReadFile(entrypointPath)
	if err != nil {
		t.Fatal(err)
	}
	modifiedContent := append(content, []byte("\n## Modified Uncommitted\n\nExtra instructions.\n")...)
	if err := os.WriteFile(entrypointPath, modifiedContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// 2. Review must display both bases and report divergence
	reviewDiverged, err := service.ReviewSkill(ctx, root, "consumer-review")
	if err != nil {
		t.Fatalf("ReviewSkill on diverged skill failed: %v", err)
	}
	if !reviewDiverged.Diverged {
		t.Fatal("expected reviewDiverged.Diverged to be true")
	}
	if len(reviewDiverged.ChangedResources) == 0 {
		t.Fatal("expected ChangedResources to list SKILL.md")
	}
	if !strings.Contains(reviewDiverged.NextAction, "rebuild") {
		t.Fatalf("next action should recommend rebuild: %s", reviewDiverged.NextAction)
	}

	// 3. ReadSkill / show MUST return typed resource_content_unavailable rather than historical bytes (Step 8!)
	_, readErr := service.ReadSkill(ctx, root, "consumer-review")
	if readErr == nil {
		t.Fatal("expected ReadSkill to fail with resource_content_unavailable when live bytes differ")
	}
	var appErr *Error
	if !errors.As(readErr, &appErr) {
		t.Fatalf("expected *Error, got: %T (%v)", readErr, readErr)
	}
	if appErr.Code != ErrorResourceContentUnavailable {
		t.Fatalf("appErr.Code = %q, want %q", appErr.Code, ErrorResourceContentUnavailable)
	}
}

func TestReviewSkillTolerantOfBrokenCatalog(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}

	createAndActivateSkill(t, service, root)

	// Remove catalog database / pointers to simulate corrupted/missing catalog
	_ = os.RemoveAll(filepath.Join(root, "runtime", "catalog"))

	// ReviewSkill must not fail; it reads canonical files directly
	review, err := service.ReviewSkill(ctx, root, "consumer-review")
	if err != nil {
		t.Fatalf("ReviewSkill must tolerate broken/missing catalog, got error: %v", err)
	}
	if review.SkillID != "consumer-review" {
		t.Fatalf("review.SkillID = %q, want consumer-review", review.SkillID)
	}
	if review.LifecycleState != "active" {
		t.Fatalf("canonical LifecycleState = %q, want active", review.LifecycleState)
	}
	if review.ResourceStatus.ResourceCount == 0 {
		t.Fatal("expected ResourceStatus.ResourceCount > 0 from disk")
	}
	// ServedFacts should indicate not known or fallback
	if review.ServedFacts.Known {
		t.Logf("ServedFacts known: %v", review.ServedFacts)
	}
}

func TestReviewSkillDeprecatedAndArchived(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}

	createAndActivateSkill(t, service, root)

	// Deprecate
	depProposal, err := service.PreviewDeprecate(ctx, root, "consumer-review", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, depProposal, depProposal.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	depReview, err := service.ReviewSkill(ctx, root, "consumer-review")
	if err != nil {
		t.Fatal(err)
	}
	if depReview.LifecycleState != "deprecated" {
		t.Fatalf("LifecycleState = %q, want deprecated", depReview.LifecycleState)
	}
	if depReview.ActiveLocally || depReview.RoutingEligible {
		t.Fatal("deprecated skill should not be active locally or routing eligible")
	}
	if !strings.Contains(depReview.NextAction, "archive") {
		t.Fatalf("deprecated next action should mention archive: %s", depReview.NextAction)
	}

	// Archive
	archProposal, err := service.PreviewArchive(ctx, root, "consumer-review", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, archProposal, archProposal.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	archReview, err := service.ReviewSkill(ctx, root, "consumer-review")
	if err != nil {
		t.Fatal(err)
	}
	if archReview.LifecycleState != "archived" {
		t.Fatalf("LifecycleState = %q, want archived", archReview.LifecycleState)
	}
	if !strings.Contains(archReview.NextAction, "archived") {
		t.Fatalf("archived next action should report archived: %s", archReview.NextAction)
	}
}
