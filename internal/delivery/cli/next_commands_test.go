package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSkillPreviewCLICommandsApplyReviewedProposal(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "bound-skill.md")
	if err := os.WriteFile(contentFile, []byte("---\nname: bound-skill\ndescription: Original description\n---\n\n# Bound Skill\n\nReview bound workflows and verify errors.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		args               []string
		state, description string
	}{
		{[]string{"create", "bound-skill", "--collection", "software", "--name", "Bound Skill", "--description", "Original description", "--trigger", "review bound skill", "--not-for", "other", "--min-scope", "single_step", "--content-file", contentFile}, "draft", "Original description"},
		{[]string{"edit", "bound-skill", "--description", "Reviewed description"}, "draft", "Reviewed description"},
		{[]string{"activate", "bound-skill"}, "active", "Reviewed description"},
		{[]string{"deprecate", "bound-skill"}, "deprecated", "Reviewed description"},
		{[]string{"archive", "bound-skill"}, "archived", "Reviewed description"},
	} {
		args := append([]string{"skill"}, step.args...)
		if step.args[0] == "create" {
			args = append(args, "--workspace", filepath.Join(t.TempDir(), "not-the-preview-workspace"))
		}
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

func TestLocalAddCLIConfirmsSnapshotNotChangedInput(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	input := filepath.Join(t.TempDir(), "local-skill")
	if err := os.MkdirAll(input, 0700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: local-skill\ndescription: Original reviewed content\n---\n\n# Original\n"
	if err := os.WriteFile(filepath.Join(input, "SKILL.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	code, output, stderr := runCLI(t, "skill", "add", input, "--workspace", root, "--json")
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
	if err := os.WriteFile(filepath.Join(input, "SKILL.md"), []byte("changed after review"), 0600); err != nil {
		t.Fatal(err)
	}
	code, output, stderr = runCLI(t, splitCLI(preview.CLI)[1:]...)
	if code != 0 {
		t.Fatalf("confirm: %d %s %s", code, output, stderr)
	}
	actual, err := os.ReadFile(filepath.Join(root, "skills", "default", "local-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(actual), "# Original") || strings.Contains(string(actual), "changed after review") {
		t.Fatalf("applied unreviewed input: %q %v", actual, err)
	}
}
