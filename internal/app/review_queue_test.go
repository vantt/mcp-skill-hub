package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// approvedThirdParty seeds a third-party skill, approves its content and then
// changes a file, so its approval is stale. It returns the hub.
func approvedThirdParty(t *testing.T, id string) string {
	t.Helper()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: id, origin: "github", files: map[string]string{"scripts/fill.py": "print(1)\n", "references/a.md": "a\n"}})
	commitHubAll(t, hub, "add "+id)
	service := SkillService{}
	trust, err := service.ContentTrustFor(context.Background(), hub, id)
	if err != nil || !trust.ThirdParty || !trust.RequiresReview() {
		t.Fatalf("trust = %+v, %v", trust, err)
	}
	preview, err := service.PreviewSkillApproval(context.Background(), hub, id, trust.ContentDigest)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(commitCtx(), hub, preview, preview.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("approve: %v %+v", err, result.Error)
	}
	script := filepath.Join(hub, "skills", "default", id, "scripts", "fill.py")
	if err := os.WriteFile(script, []byte("print(2)\nprint(3)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitHubAll(t, hub, "upstream changed")
	return hub
}

func TestReviewQueueListsDraftsAndStaleApprovals(t *testing.T) {
	t.Parallel()
	hub := approvedThirdParty(t, "pdf")
	seedHubSkill(t, hub, fixtureSkill{id: "team-release", state: "draft"})
	seedHubSkill(t, hub, fixtureSkill{id: "fine", files: map[string]string{"x.md": "x\n"}})
	commitHubAll(t, hub, "more skills")

	queue, err := (SkillService{}).ReviewQueue(context.Background(), hub)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ReviewQueueItem{}
	for _, item := range queue.Items {
		got[item.SkillID] = item
	}
	if len(got) != 2 || got["pdf"].Kind != ReviewKindContentChanged || got["team-release"].Kind != ReviewKindDraft {
		t.Fatalf("items = %+v", queue.Items)
	}
	if got["pdf"].Origin != "github.com/example/pdf" || got["pdf"].Since == "" || !strings.Contains(got["pdf"].Reason, "changed") {
		t.Fatalf("pdf = %+v", got["pdf"])
	}
}

func TestReviewContentDiffsAgainstTheApprovedContent(t *testing.T) {
	t.Parallel()
	hub := approvedThirdParty(t, "pdf")
	review, err := (SkillService{}).ReviewSkillContent(context.Background(), hub, "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if review.Baseline != reviewBaselineApproved || review.BaselineCommit == "" || !review.ThirdParty || review.Approved {
		t.Fatalf("review = %+v", review)
	}
	if review.Origin != "github.com/example/pdf" || review.License != "not stated" || review.ContentDigest == "" {
		t.Fatalf("facts = origin %q license %q digest %q", review.Origin, review.License, review.ContentDigest)
	}
	byPath := map[string]ReviewFile{}
	for _, file := range review.Files {
		byPath[file.Path] = file
	}
	script := byPath["scripts/fill.py"]
	if !script.Changed || script.Change != "M" || script.Added != 2 || script.Removed != 1 || !strings.Contains(script.Diff, "+print(3)") {
		t.Fatalf("script = %+v", script)
	}
	if byPath["SKILL.md"].Changed || byPath["references/a.md"].Changed || review.Files[0].Path != "SKILL.md" {
		t.Fatalf("files = %+v", review.Files)
	}
}

func TestReviewContentOfADraftCountsEveryFileAsChanged(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "team-release", state: "draft", files: map[string]string{"LICENSE": "MIT License\n\ntext\n"}})
	commitHubAll(t, hub, "draft")
	review, err := (SkillService{}).ReviewSkillContent(context.Background(), hub, "team-release")
	if err != nil {
		t.Fatal(err)
	}
	if review.Baseline != reviewBaselineNone || review.License != "MIT License (LICENSE)" {
		t.Fatalf("review = %+v", review)
	}
	for _, file := range review.Files {
		if !file.Changed || file.Change != "A" {
			t.Fatalf("file = %+v", file)
		}
	}
}

func TestReviewContentApprovalMakesTheSkillServableInOneCommit(t *testing.T) {
	t.Parallel()
	hub := approvedThirdParty(t, "pdf")
	service := SkillService{}
	review, err := service.ReviewSkillContent(context.Background(), hub, "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewSkillApproval(context.Background(), hub, "pdf", "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("an approval of content that is not on the hub must be refused")
	}
	before := hubGit(t, hub, "rev-list", "--count", "HEAD")
	preview, err := service.PreviewSkillApproval(context.Background(), hub, "pdf", review.ContentDigest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmSkillMutation(commitCtx(), hub, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil || result.Error != nil || result.Commit == nil {
		t.Fatalf("confirm: %v %+v %+v", err, result.Error, result.Commit)
	}
	if hubGit(t, hub, "rev-list", "--count", "HEAD") == before {
		t.Fatal("approval made no commit")
	}
	trust, err := service.ContentTrustFor(context.Background(), hub, "pdf")
	if err != nil || trust.RequiresReview() || !trust.Approved {
		t.Fatalf("trust = %+v, %v", trust, err)
	}
	if queue, _ := service.ReviewQueue(context.Background(), hub); len(queue.Items) != 0 {
		t.Fatalf("queue = %+v", queue.Items)
	}
}
