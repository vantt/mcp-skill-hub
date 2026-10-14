package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

// editedLine is what the project edit adds to the checked-out SKILL.md.
const editedLine = "Edited in the project only."

func checkoutAndEdit(t *testing.T, root string) (project string) {
	t.Helper()
	project = filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := (app.SkillCheckoutService{}).Checkout(context.Background(), root, app.SkillCheckoutInput{SkillID: "review-skill", ProjectDir: project})
	if err != nil || result.Error != nil {
		t.Fatalf("checkout = %#v, %v", result.Error, err)
	}
	entry := filepath.Join(project, ".claude", "skills", "review-skill", "SKILL.md")
	original, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, append(original, []byte("\n"+editedLine+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	return project
}

func commitFromProject(t *testing.T, root, project string, options app.SkillCommitApplyOptions) {
	t.Helper()
	service := app.SkillCommitService{}
	preview, err := service.Preview(context.Background(), root, app.SkillCommitInput{Target: "review-skill", ProjectDir: project})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview = %#v, %v", preview.Error, err)
	}
	result, err := service.Apply(context.Background(), root, preview, options)
	if err != nil || result.Error != nil {
		t.Fatalf("apply = %#v, %v", result.Error, err)
	}
}

func TestSkillGetServesTheReleasedVersionUntilTheCommit(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	session := connectDistributionSession(t, root)
	if got := callSkillGet(t, session, "review-skill"); !strings.Contains(got.Content, "Review carefully.") {
		t.Fatalf("released content = %q", got.Content)
	}
	project := checkoutAndEdit(t, root)
	if got := callSkillGet(t, session, "review-skill"); strings.Contains(got.Content, editedLine) || !strings.Contains(got.Content, "Review carefully.") {
		t.Fatalf("an edit in a project reached the runtime before the commit: %q", got.Content)
	}
	commitFromProject(t, root, project, app.SkillCommitApplyOptions{})
	if got := callSkillGet(t, session, "review-skill"); !strings.Contains(got.Content, editedLine) {
		t.Fatalf("the committed edit is not served: %q", got.Content)
	}
}

func TestSkillGetServesNewThirdPartyContentRightAfterACommitThatApprovesIt(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	review, err := (app.SkillService{}).ReviewSkill(t.Context(), root, "review-skill")
	if err != nil {
		t.Fatal(err)
	}
	setSkillMeta(t, root, func(doc map[string]any) {
		doc["sources"] = []any{map[string]any{"id": "upstream-skill", "roles": []string{"upstream"}, "kind": "github", "repository": "https://github.com/example/skills"}}
		doc["quality"] = map[string]any{"reviewed": true, "content_reviewed_digest": review.ContentTrust.ContentDigest}
	})
	session := connectDistributionSession(t, root)
	if got := callSkillGet(t, session, "review-skill"); !strings.Contains(got.Content, "Review carefully.") {
		t.Fatalf("approved third-party content must be served: %#v", got)
	}
	project := checkoutAndEdit(t, root)
	if got := callSkillGet(t, session, "review-skill"); strings.Contains(got.Content, editedLine) || !strings.Contains(got.Content, "Review carefully.") {
		t.Fatalf("an edit in a project reached the runtime before the commit: %q", got.Content)
	}
	commitFromProject(t, root, project, app.SkillCommitApplyOptions{Approve: true})
	got := callSkillGet(t, session, "review-skill")
	if !strings.Contains(got.Content, editedLine) || (got.Local != nil && got.Local.Status == app.LocalStatusReviewRequired) {
		t.Fatalf("the skill must never be blocked after a commit that approves: %#v", got)
	}
}
