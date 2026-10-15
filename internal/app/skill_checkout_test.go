package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// hubGit runs git in dir and returns its trimmed output.
func hubGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(gitIn(t, dir, args...))
}

// newGitHub returns a hub that is its own Git repository with an identity.
func newGitHub(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "hub")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	hubGit(t, root, "init", "-q", "-b", "main")
	hubGit(t, root, "config", "user.name", "Test")
	hubGit(t, root, "config", "user.email", "test@example.com")
	hubGit(t, root, "config", "commit.gpgsign", "false")
	hubGit(t, root, "add", "-A")
	hubGit(t, root, "commit", "-q", "--allow-empty", "-m", "baseline")
	return root
}

func newProjectDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hubGit(t, dir, "init", "-q", "-b", "main")
	return dir
}

// fixtureSkill describes a skill written straight into a hub fixture.
type fixtureSkill struct {
	id, state, origin string // origin "" is first-party, "github" is third-party
	files             map[string]string
	executable        []string
}

func seedHubSkill(t *testing.T, root string, skill fixtureSkill) string {
	t.Helper()
	if skill.state == "" {
		skill.state = "active"
	}
	dir := filepath.Join(root, "skills", "default", skill.id)
	meta := "schema_version: 1\nid: " + skill.id + "\nstatus: " + skill.state + "\nrouting:\n  triggers:\n    - run " + skill.id + "\n  not_for:\n    - unrelated work\n  min_scope: single_step\n  examples:\n    - run " + skill.id + " now\n    - please run " + skill.id + "\n    - use " + skill.id + " here\nquality:\n  reviewed: false\n"
	if skill.origin != "" {
		meta += "sources:\n  - id: " + skill.id + "-src\n    roles:\n      - upstream\n    kind: " + skill.origin + "\n    repo: github.com/example/" + skill.id + "\n"
	}
	files := map[string]string{"SKILL.md": "---\nname: " + skill.id + "\ndescription: Does " + skill.id + " things. Use when you need " + skill.id + ".\n---\n\n# " + skill.id + "\n\nline one\nline two\nline three\n"}
	for rel, body := range skill.files {
		files[rel] = body
	}
	files[".meta/skill.yaml"] = meta
	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range skill.executable {
		if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		// A Windows file system keeps no executable bit, so a hub that is a
		// Git repository gets it in its index, where the hub keeps modes.
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			hubGit(t, root, "add", "--chmod=+x", "--", "skills/default/"+skill.id+"/"+rel)
		}
	}
	return dir
}

func commitHubAll(t *testing.T, root, message string) {
	t.Helper()
	hubGit(t, root, "add", "-A")
	hubGit(t, root, "commit", "-q", "-m", message)
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestSkillCheckoutCopiesEveryContentFileAndLeavesTheHubAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	root := newGitHub(t)
	seedHubSkill(t, root, fixtureSkill{id: "review-checklist", files: map[string]string{"scripts/lint.sh": "#!/bin/sh\n", "references/notes.md": "notes\n"}, executable: []string{"scripts/lint.sh"}})
	commitHubAll(t, root, "add review-checklist")
	headBefore := hubGit(t, root, "rev-parse", "HEAD")
	project := newProjectDir(t)

	result, err := (SkillCheckoutService{}).Checkout(context.Background(), root, SkillCheckoutInput{SkillID: "review-checklist", ProjectDir: project})
	if err != nil || result.Error != nil {
		t.Fatalf("checkout failed: %v, %#v", err, result.Error)
	}
	if result.FileCount != 3 || result.State != "active" || result.Path != ".claude/skills/review-checklist" {
		t.Fatalf("result = %+v", result)
	}
	folder := filepath.Join(project, ".claude", "skills", "review-checklist")
	for rel, mode := range map[string]os.FileMode{"SKILL.md": 0o644, "scripts/lint.sh": 0o755, "references/notes.md": 0o644} {
		info, err := os.Stat(filepath.Join(folder, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s not copied: %v", rel, err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %v, want %v", rel, info.Mode().Perm(), mode)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, ".meta")); !os.IsNotExist(err) {
		t.Fatalf("hub meta reached the project: %v", err)
	}
	marker, found, err := ReadCheckoutMarker(folder)
	if err != nil || !found {
		t.Fatalf("marker: found=%v err=%v", found, err)
	}
	if marker.BaseCommit != headBefore || marker.SkillID != "review-checklist" || marker.BaseVersion == "" {
		t.Fatalf("marker = %+v, want base_commit %s", marker, headBefore)
	}
	if got := hubGit(t, root, "rev-parse", "HEAD"); got != headBefore {
		t.Fatalf("hub HEAD moved from %s to %s", headBefore, got)
	}
	if status := hubGit(t, root, "status", "--porcelain", "--", "skills"); status != "" {
		t.Fatalf("hub skills changed:\n%s", status)
	}
}

func TestSkillCheckoutGitignoreHasNoDuplicatesAndRefusesASecondCheckout(t *testing.T) {
	t.Parallel()
	root := newGitHub(t)
	seedHubSkill(t, root, fixtureSkill{id: "alpha"})
	seedHubSkill(t, root, fixtureSkill{id: "beta"})
	commitHubAll(t, root, "add skills")
	project := newProjectDir(t)
	service := SkillCheckoutService{}
	for _, id := range []string{"alpha", "beta"} {
		if result, err := service.Checkout(context.Background(), root, SkillCheckoutInput{SkillID: id, ProjectDir: project}); err != nil || result.Error != nil {
			t.Fatalf("checkout %s: %v, %#v", id, err, result.Error)
		}
	}
	data, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# skillhub checkout\n/.claude/skills/alpha/\n/.claude/skills/beta/\n"
	if string(data) != want {
		t.Fatalf(".gitignore = %q, want %q", data, want)
	}
	again, err := service.Checkout(context.Background(), root, SkillCheckoutInput{SkillID: "alpha", ProjectDir: project})
	if err != nil || again.Error == nil || again.Error.Code != ErrorSkillConflict || !strings.Contains(again.Error.Render.Fix, "skillhub skill commit alpha") {
		t.Fatalf("second checkout = %+v, %v", again.Error, err)
	}
	if data, _ := os.ReadFile(filepath.Join(project, ".gitignore")); string(data) != want {
		t.Fatalf(".gitignore changed on a refused checkout: %q", data)
	}
}

func TestSkillCheckoutRefusesAnUnmarkedFolderAndSkipsCoveredGitignore(t *testing.T) {
	t.Parallel()
	root := newGitHub(t)
	seedHubSkill(t, root, fixtureSkill{id: "alpha"})
	seedHubSkill(t, root, fixtureSkill{id: "beta"})
	commitHubAll(t, root, "add skills")
	project := newProjectDir(t)
	if err := os.MkdirAll(filepath.Join(project, ".claude", "skills", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := SkillCheckoutService{}
	refused, err := service.Checkout(context.Background(), root, SkillCheckoutInput{SkillID: "alpha", ProjectDir: project})
	if err != nil || refused.Error == nil || !strings.Contains(refused.Error.Render.Fix, "skillhub skill commit .claude/skills/alpha") {
		t.Fatalf("unmarked folder = %+v, %v", refused.Error, err)
	}
	if err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte("/.claude/skills/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result, err := service.Checkout(context.Background(), root, SkillCheckoutInput{SkillID: "beta", ProjectDir: project}); err != nil || result.Error != nil || result.GitignoreAdded != "" {
		t.Fatalf("covered checkout = %+v, %v", result, err)
	}
	if data, _ := os.ReadFile(filepath.Join(project, ".gitignore")); string(data) != "/.claude/skills/\n" {
		t.Fatalf(".gitignore = %q", data)
	}
}

func TestSkillCheckoutLeavesBaseCommitEmptyWhenTheSkillHasUncommittedEdits(t *testing.T) {
	t.Parallel()
	root := newGitHub(t)
	dir := seedHubSkill(t, root, fixtureSkill{id: "alpha"})
	commitHubAll(t, root, "add alpha")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: alpha\ndescription: edited\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := newProjectDir(t)
	if result, err := (SkillCheckoutService{}).Checkout(context.Background(), root, SkillCheckoutInput{SkillID: "alpha", ProjectDir: project}); err != nil || result.Error != nil || result.BaseCommit != "" {
		t.Fatalf("checkout = %+v, %v", result, err)
	}
}
