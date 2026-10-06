package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestServerRoutesNoPanic(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	// Must not panic on constructor
	srv := newTestServer(t, root)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestSourceRoutesCheckValidation(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	// 1. None of the three fields -> 400
	recNone := postJSON(t, srv, "/api/v1/sources/check", map[string]any{})
	if recNone.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for no check fields, got %d: %s", recNone.Code, recNone.Body.String())
	}

	// 2. Two fields (all + due) -> 400
	recTwo1 := postJSON(t, srv, "/api/v1/sources/check", map[string]any{
		"all": true,
		"due": true,
	})
	if recTwo1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for all+due, got %d: %s", recTwo1.Code, recTwo1.Body.String())
	}

	// 3. Two fields (source_ids + all) -> 400
	recTwo2 := postJSON(t, srv, "/api/v1/sources/check", map[string]any{
		"source_ids": []string{"s1"},
		"all":        true,
	})
	if recTwo2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for source_ids+all, got %d: %s", recTwo2.Code, recTwo2.Body.String())
	}
}

func TestSourceRoutesCheckAllVsDue(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	// Seed two sources: one manual, one weekly
	manualRec := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "src-manual",
		Adapter:       "filesystem",
		Locator:       sourcepkg.Locator{Path: "sources/local"},
		Status:        "watching",
		Identity:      sourcepkg.Identity{Name: "src-manual", Canonical: "sources/local"},
		Monitoring:    sourcepkg.Monitoring{Enabled: false, Cadence: "manual"},
		Limits:        sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
	}
	manualBytes, _ := sourcepkg.MarshalCanonical(manualRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-manual.yaml"), manualBytes, 0o644)

	// all: true checks the manual source
	recAll := postJSON(t, srv, "/api/v1/sources/check", map[string]any{
		"all": true,
	})
	if recAll.Code != http.StatusOK {
		t.Fatalf("expected 200 for all: true, got %d: %s", recAll.Code, recAll.Body.String())
	}
	var resAll app.SourceCheckResult
	_ = json.Unmarshal(recAll.Body.Bytes(), &resAll)
	foundManualInAll := false
	for _, item := range resAll.Results {
		if item.SourceID == "src-manual" {
			foundManualInAll = true
			break
		}
	}
	if !foundManualInAll {
		t.Fatalf("expected src-manual in all: true results, got: %#v", resAll)
	}

	// due: true skips manual source
	recDue := postJSON(t, srv, "/api/v1/sources/check", map[string]any{
		"due": true,
	})
	if recDue.Code != http.StatusOK {
		t.Fatalf("expected 200 for due: true, got %d: %s", recDue.Code, recDue.Body.String())
	}
	var resDue app.SourceCheckResult
	_ = json.Unmarshal(recDue.Body.Bytes(), &resDue)
	for _, item := range resDue.Results {
		if item.SourceID == "src-manual" {
			t.Fatalf("src-manual should be skipped when due: true, but appeared in results: %#v", resDue)
		}
	}
}

func TestSourceRoutesSkillSourcesNotFound(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	rec := get(t, srv, "/api/v1/skills/unknown-skill-12345/sources")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown skill on /sources, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSourceRoutesAttachNonGitHub(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	// Non-GitHub locator -> 400
	rec := postJSON(t, srv, "/api/v1/skills/review-skill/sources/attach/preview", map[string]any{
		"locator": "https://gitlab.com/example/repo.git",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-GitHub locator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSourceRoutesAttachAndConfirm(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	// Seed source
	rec := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "src-attach-test",
		Adapter:       "git",
		Locator:       sourcepkg.Locator{Repository: "https://github.com/example/repo.git", Ref: "main"},
		Status:        "watching",
		Identity:      sourcepkg.Identity{Name: "src-attach-test", Canonical: "https://github.com/example/repo.git", DefaultBranch: "main"},
		Monitoring:    sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:        sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
	}
	recBytes, _ := sourcepkg.MarshalCanonical(rec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-attach-test.yaml"), recBytes, 0o644)

	// Attach preview
	recAttach := postJSON(t, srv, "/api/v1/skills/review-skill/sources/attach/preview", map[string]any{
		"source_id": "src-attach-test",
	})
	if recAttach.Code != http.StatusOK {
		t.Fatalf("expected 200 for attach preview, got %d: %s", recAttach.Code, recAttach.Body.String())
	}
	var attachProp app.SourceProposal
	_ = json.Unmarshal(recAttach.Body.Bytes(), &attachProp)
	pins := attachProp.Confirmation.Confirmation.Pins

	// Confirm via /api/v1/sources/proposals/{proposal_id}/confirm
	confirmURL := "/api/v1/sources/proposals/" + pins.ProposalID + "/confirm"
	recConfirm := postJSON(t, srv, confirmURL, map[string]any{
		"proposal_digest": pins.ProposalDigest,
		"base_version":    pins.BaseVersion,
	})
	if recConfirm.Code != http.StatusOK {
		t.Fatalf("expected 200 for source confirm, got %d: %s", recConfirm.Code, recConfirm.Body.String())
	}

	// Verify link file exists
	linkPath := filepath.Join(root, "sources", "skills", "LINK-review-skill--src-attach-test.yaml")
	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("expected link file at %s, got err: %v", linkPath, err)
	}
}

func TestUpstreamRoutesReviewAndConfirm(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	t.Parallel()
	root := newWebWorkspace(t)
	repoDir := t.TempDir()

	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "config", "user.name", "Tester")
	runGit(t, repoDir, "config", "user.email", "tester@example.com")
	runGit(t, repoDir, "config", "uploadpack.allowReachableSHA1InWant", "true")

	skillDir := filepath.Join(repoDir, "skills", "my-up-skill")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-up-skill\ndescription: Test\n---\nBody v1\n"), 0o644)
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "init base")
	baseCommit := runGit(t, repoDir, "rev-parse", "HEAD")

	adapter := sourcepkg.GitRepositoryAdapter{
		CacheRoot:         filepath.Join(root, "runtime", "sources", "git"),
		AllowFileProtocol: true,
	}

	srv := newTestServer(t, root)
	srv.upstream = app.UpstreamService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	srv.sources = app.SourceService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	// Add skill to workspace with file:// locator
	fileURL := "file://" + filepath.ToSlash(repoDir)
	addService := app.SkillAddService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	addPrev, err := addService.PreviewSkillAdd(context.Background(), root, app.SkillAddInput{
		Locator: fileURL,
		All:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	addRes, err := addService.ConfirmSkillAdd(context.Background(), root, addPrev, addPrev.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	_ = addRes

	// Now modify upstream repository
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-up-skill\ndescription: Test\n---\nBody v2 updated\n"), 0o644)
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "upstream update commit")
	_ = baseCommit

	// Check upstream first via /api/v1/skills/my-up-skill/upstream/check
	recCheck := postJSON(t, srv, "/api/v1/skills/my-up-skill/upstream/check", nil)
	if recCheck.Code != http.StatusOK {
		t.Fatalf("expected 200 for upstream check, got %d: %s", recCheck.Code, recCheck.Body.String())
	}

	// Upstream review
	recReview := postJSON(t, srv, "/api/v1/skills/my-up-skill/upstream/review", map[string]any{})
	if recReview.Code != http.StatusOK {
		t.Fatalf("expected 200 for upstream review, got %d: %s", recReview.Code, recReview.Body.String())
	}
	var revProp app.UpstreamUpdatePreview
	_ = json.Unmarshal(recReview.Body.Bytes(), &revProp)
	pins := revProp.Confirmation.Confirmation.Pins

	// Confirm through /api/v1/upstream/proposals/{proposal_id}/confirm
	confirmURL := "/api/v1/upstream/proposals/" + pins.ProposalID + "/confirm"
	recConfirm := postJSON(t, srv, confirmURL, map[string]any{
		"proposal_digest": pins.ProposalDigest,
		"base_version":    pins.BaseVersion,
	})
	if recConfirm.Code != http.StatusOK {
		t.Fatalf("expected 200 for upstream confirm, got %d: %s", recConfirm.Code, recConfirm.Body.String())
	}
	var updateResult app.UpstreamUpdateResult
	_ = json.Unmarshal(recConfirm.Body.Bytes(), &updateResult)
	if !strings.Contains(updateResult.Summary, "skillhub skill review") {
		t.Fatalf("expected summary to contain 'skillhub skill review', got %q", updateResult.Summary)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}
