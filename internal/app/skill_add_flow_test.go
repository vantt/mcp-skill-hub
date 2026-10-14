package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/vantt/skillhub-mcp/internal/skill"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
)

func skillAddFlowFixture(t *testing.T) (string, SkillAddService) {
	t.Helper()
	root := newSourceWorkspace(t)
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"SKILL.md":    "---\nname: pdf\ndescription: Fill PDF forms. Use when the user asks to fill a pdf form. Do not use when the user wants to edit a word document.\nlicense: MIT\n---\n\n# PDF\n\nFill the form.\n",
		"forms.md":    "Form help.\n",
		"LICENSE.txt": "MIT\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(repoDir, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Commit("initial", &git.CommitOptions{Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	service := SkillAddService{
		Clock:    sourceClock{now: time.Now().UTC()},
		Adapters: map[string]sourcepkg.Adapter{"git": sourcepkg.GitRepositoryAdapter{CacheRoot: filepath.Join(root, "runtime", "sources", "git"), AllowFileProtocol: true}},
	}
	return root + "|" + localFileURL(repoDir), service
}

func TestSkillAddPreviewCarriesWhatAPersonDecidesOn(t *testing.T) {
	t.Parallel()
	rootAndURL, service := skillAddFlowFixture(t)
	root, url, _ := strings.Cut(rootAndURL, "|")

	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: url, FullDiff: true})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview: %v %#v", err, preview.Error)
	}
	if preview.Origin.Kind != "github" {
		t.Fatalf("origin kind comes from the host provider: %q", preview.Origin.Kind)
	}
	var paths []string
	for _, file := range preview.Files {
		paths = append(paths, file.Path)
	}
	if strings.Join(paths, ",") != "LICENSE.txt,SKILL.md,forms.md" {
		t.Fatalf("files = %v", paths)
	}
	if preview.RoutingSuggestions == nil || len(preview.RoutingSuggestions.Triggers) != 1 || len(preview.RoutingSuggestions.NotFor) != 1 {
		t.Fatalf("routing = %#v", preview.RoutingSuggestions)
	}
	if preview.Trust == nil || !preview.Trust.ThirdParty || !strings.HasPrefix(preview.Trust.ContentDigest, "sha256:") || !preview.ActivationReady {
		t.Fatalf("trust = %#v ready = %v missing = %v", preview.Trust, preview.ActivationReady, preview.ActivationMissing)
	}
	if preview.plannedApproved == nil || preview.plannedApproved.ID == preview.planned.ID {
		t.Fatalf("an approved variant of the plan must exist beside the plain one")
	}
	for _, want := range []string{"+Form help.", "+++ b/forms.md", "+Fill the form."} {
		if !strings.Contains(preview.FullDiff, want) {
			t.Errorf("full diff lacks %q:\n%s", want, preview.FullDiff)
		}
	}
	if strings.Contains(preview.FullDiff, "[added") {
		t.Errorf("the diff must show content, not a byte count:\n%s", preview.FullDiff)
	}
	meta, err := os.ReadFile(filepath.Join(root, "skills", "default", "pdf", ".meta", "skill.yaml"))
	if err == nil {
		t.Fatalf("a preview must not write the hub: %s", meta)
	}
}

func TestSkillAddWithRoutingFlagsOverridesSuggestions(t *testing.T) {
	t.Parallel()
	rootAndURL, service := skillAddFlowFixture(t)
	root, url, _ := strings.Cut(rootAndURL, "|")
	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: url, Routing: &skill.RoutingInput{Triggers: []string{"fill out a form"}}})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview: %v %#v", err, preview.Error)
	}
	if got := preview.RoutingSuggestions.Triggers; len(got) != 1 || got[0] != "fill out a form" || len(preview.RoutingSuggestions.NotFor) != 1 {
		t.Fatalf("flags replace only the fields they set: %#v", preview.RoutingSuggestions)
	}
}
