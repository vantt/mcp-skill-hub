package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

func TestSkillPreviewCLICommandsApplyReviewedProposal(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "bound-skill.md")
	if err := os.WriteFile(contentFile, []byte("---\nname: bound-skill\ndescription: Original description\n---\n\n# Bound Skill\n\nReview bound workflows and verify errors.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := seedSkillArgs([]string{"bound-skill", "--collection", "software", "--name", "Bound Skill", "--description", "Original description", "--trigger", "review bound skill", "--not-for", "other", "--min-scope", "single_step", "--content-file", contentFile, "--workspace", root, "--yes"}); code != 0 {
		t.Fatalf("seed = %d: %s %s", code, stdout, stderr)
	}
	for _, step := range []struct {
		args               []string
		state, description string
	}{
		{[]string{"edit", "bound-skill", "--description", "Reviewed description"}, "draft", "Reviewed description"},
		{[]string{"activate", "bound-skill"}, "active", "Reviewed description"},
		{[]string{"deprecate", "bound-skill"}, "deprecated", "Reviewed description"},
		{[]string{"archive", "bound-skill"}, "archived", "Reviewed description"},
	} {
		args := append([]string{"skill"}, step.args...)
		code, output, stderr := runCLI(t, append(args, "--workspace", root, "--json")...)
		if code != 0 {
			t.Fatalf("preview %v = %d: %s %s", step.args, code, output, stderr)
		}
		var preview struct {
			app.SkillProposal
			CLI     string                          `json:"cli"`
			Actions []struct{ Command, CLI string } `json:"suggested_actions"`
		}
		if err := json.Unmarshal([]byte(output), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.CLI == "" {
			t.Fatalf("preview missing runnable CLI: %s", output)
		}
		for _, action := range preview.Actions {
			if action.CLI != preview.CLI || action.Command != preview.Command {
				t.Fatalf("action not bound to preview: %+v", action)
			}
		}
		confirm := splitCLI(preview.CLI)
		code, output, stderr = runCLI(t, confirm[1:]...)
		if code != 0 {
			t.Fatalf("confirm %s = %d: %s %s", preview.CLI, code, output, stderr)
		}
		code, output, stderr = runCLI(t, "skill", "show", "bound-skill", "--workspace", root, "--json")
		var read app.SkillReadResult
		if code != 0 || json.Unmarshal([]byte(output), &read) != nil || read.Manifest.Status != step.state || read.Manifest.Description != step.description {
			t.Fatalf("state after %v: %d %s %s", step.args, code, output, stderr)
		}
	}
}

func TestAddCLIConfirmsReviewedContentNotALaterCommit(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := addFixtureRepo(t, addFixtureFile{path: "SKILL.md", body: "---\nname: remote-skill\ndescription: Original reviewed content\n---\n\n# Original\n"})
	code, output, stderr := hub.run("skill", "add", repo, "--json")
	if code != 0 {
		t.Fatalf("preview: %d %s %s", code, output, stderr)
	}
	var preview struct {
		CLI string `json:"cli"`
	}
	if err := json.Unmarshal([]byte(output), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.CLI == "" {
		t.Fatalf("missing cli: %s", output)
	}
	commitFixtureFile(t, repo, "SKILL.md", "changed after review")
	code, output, stderr = runCLI(t, splitCLI(preview.CLI)[1:]...)
	if code != 0 {
		t.Fatalf("confirm: %d %s %s", code, output, stderr)
	}
	actual, err := os.ReadFile(filepath.Join(hub.root, "skills", "default", "remote-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(actual), "# Original") || strings.Contains(string(actual), "changed after review") {
		t.Fatalf("applied unreviewed input: %q %v", actual, err)
	}
}
