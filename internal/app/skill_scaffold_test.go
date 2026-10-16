package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldProjectSkillWritesOnlyInTheProjectAndFeedsTheCommit(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	hubBefore := hubGit(t, hub, "status", "--porcelain", "--untracked-files=all")
	project := newProjectDir(t)

	result, err := ScaffoldProjectSkill(context.Background(), SkillScaffoldInput{
		SkillID: "deploy-notes", ProjectDir: project, Description: "Summarize a deploy for the team channel",
		Triggers: []string{"after a production deploy", "write a commit message"}, NotFor: []string{"staging deploys"}, Examples: []string{"write the deploy notes for today's release"},
	})
	if err != nil || result.Error != nil {
		t.Fatalf("scaffold: %v %#v", err, result.Error)
	}
	if result.Path != ".claude/skills/deploy-notes/SKILL.md" || result.Summary != "Created .claude/skills/deploy-notes/SKILL.md (nothing changed on the hub)." {
		t.Fatalf("result = %+v", result)
	}
	folder := filepath.Join(project, ".claude", "skills", "deploy-notes")
	data, err := os.ReadFile(filepath.Join(folder, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: deploy-notes", "Summarize a deploy for the team channel. Use after a production deploy. Use it to write a commit message.", "## When to use\n\n- after a production deploy", "## When not to use\n\n- staging deploys", "## Examples\n\n- write the deploy notes for today's release", "## Instructions"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("SKILL.md lacks %q:\n%s", want, data)
		}
	}
	if ignore, err := os.ReadFile(filepath.Join(project, ".gitignore")); err != nil || !strings.Contains(string(ignore), "/.claude/skills/deploy-notes/\n") || result.GitignoreAdded != "/.claude/skills/deploy-notes/" {
		t.Fatalf("the created folder must stay out of the project's Git like a checkout: %q %v %+v", ignore, err, result)
	}
	if _, found, _ := ReadCheckoutMarker(folder); found {
		t.Fatalf("a created folder has no marker: its first commit is a new skill")
	}
	if hubGit(t, hub, "status", "--porcelain", "--untracked-files=all") != hubBefore {
		t.Fatalf("create changed the hub")
	}
	preview := previewCommit(t, hub, project, "deploy-notes", SkillCommitInput{})
	if preview.Case != commitCaseNew || preview.RoutingSuggestions == nil || len(preview.RoutingSuggestions.Triggers) != 2 || len(preview.RoutingSuggestions.Examples) != 1 {
		t.Fatalf("the commit must suggest the routing the form collected: %+v", preview.RoutingSuggestions)
	}
	if preview.ActivationReady {
		t.Fatalf("an untouched scaffold must not be offered for activation")
	}

	again, err := ScaffoldProjectSkill(context.Background(), SkillScaffoldInput{SkillID: "deploy-notes", ProjectDir: project, Description: "x"})
	if err != nil || again.Error == nil || again.Error.Code != ErrorSkillConflict {
		t.Fatalf("an existing folder must be refused, got %+v %v", again.Error, err)
	}
	if bad, err := ScaffoldProjectSkill(context.Background(), SkillScaffoldInput{SkillID: "Bad Id", ProjectDir: project, Description: "x"}); err != nil || bad.Error == nil {
		t.Fatalf("a bad id must be refused, got %+v %v", bad.Error, err)
	}
}
