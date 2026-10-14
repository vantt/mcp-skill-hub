package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectInDir makes a Git project and works from it for the test. It is not
// for parallel tests: the working directory belongs to the whole process.
func projectInDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	t.Chdir(dir)
	return dir
}

func readFileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// hubSnapshot lists every file of a hub with its content, so a test can prove
// a command left the hub byte for byte as it was. Git's own folder is left out.
func hubSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == gitFolderName {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, readErr := os.ReadFile(path)
		files[rel] = string(data)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

const gitFolderName = ".g" + "it"

// Not parallel: it works from the project directory and replaces the terminal
// check and the answer source for the whole package.
func TestSkillCreateInATerminalAsksThreeOrFourQuestionsAndWritesOnlyInTheProject(t *testing.T) {
	hub := initTestWorkspace(t)
	project := projectInDir(t)
	before := hubSnapshot(t, hub)
	savedInteractive, savedInput := skillCreateInteractive, skillCreateInput
	t.Cleanup(func() { skillCreateInteractive, skillCreateInput = savedInteractive, savedInput })
	skillCreateInteractive = func(io.Writer) bool { return true }
	skillCreateInput = strings.NewReader("Summarize a deploy for the team channel\nafter a production deploy\n\nfor staging deploys\n\nwrite the deploy notes for today's release\n\n")

	code, stdout, stderr := runCLI(t, "skill", "create", "deploy-notes", "--workspace", hub)
	if code != 0 {
		t.Fatalf("exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{"What does this skill do?", "When should an agent use it?", "When should it not?", "One or two example requests:",
		"Created .claude/skills/deploy-notes/SKILL.md (nothing changed on the hub).", "ask your agent to use /deploy-notes.", "When ready: skillhub skill commit deploy-notes"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output missing %q:\n%s", want, stdout)
		}
	}
	scaffold := readFileText(t, filepath.Join(project, ".claude", "skills", "deploy-notes", "SKILL.md"))
	for _, want := range []string{"name: deploy-notes", "Summarize a deploy for the team channel. Use when after a production deploy.", "## When not to use\n\n- for staging deploys", "- write the deploy notes for today's release"} {
		if !strings.Contains(scaffold, want) {
			t.Fatalf("SKILL.md lacks %q:\n%s", want, scaffold)
		}
	}
	after := hubSnapshot(t, hub)
	if len(after) != len(before) {
		t.Fatalf("hub gained or lost files: %d -> %d", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("hub file %s changed", path)
		}
	}
}

func TestSkillCreateFlagsWriteTheSameScaffoldAsTheForm(t *testing.T) {
	hub := initTestWorkspace(t)
	project := projectInDir(t)
	code, stdout, stderr := runCLI(t, "skill", "create", "deploy-notes", "--workspace", hub,
		"--description", "Summarize a deploy for the team channel", "--trigger", "after a production deploy",
		"--not-for", "for staging deploys", "--example", "write the deploy notes for today's release")
	if code != 0 {
		t.Fatalf("exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	flagForm := readFileText(t, filepath.Join(project, ".claude", "skills", "deploy-notes", "SKILL.md"))

	savedInteractive, savedInput := skillCreateInteractive, skillCreateInput
	t.Cleanup(func() { skillCreateInteractive, skillCreateInput = savedInteractive, savedInput })
	skillCreateInteractive = func(io.Writer) bool { return true }
	skillCreateInput = strings.NewReader("Summarize a deploy for the team channel\nafter a production deploy\n\nfor staging deploys\n\nwrite the deploy notes for today's release\n\n")
	if code, stdout, stderr := runCLI(t, "skill", "create", "deploy-notes-form"); code != 0 {
		t.Fatalf("form exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	formed := readFileText(t, filepath.Join(project, ".claude", "skills", "deploy-notes-form", "SKILL.md"))
	if strings.ReplaceAll(formed, "deploy-notes-form", "deploy-notes") != flagForm {
		t.Fatalf("form and flags differ:\n%s\n=====\n%s", formed, flagForm)
	}
}

func TestSkillCreateRefusesAnExistingFolderAndNamesTheMissingFlagsOutsideATerminal(t *testing.T) {
	projectInDir(t)
	if code, stdout, stderr := runCLI(t, "skill", "create", "taken", "--description", "Taken."); code != 0 {
		t.Fatalf("first create exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr := runCLI(t, "skill", "create", "taken", "--description", "Again.")
	if code != 2 || !strings.Contains(stderr, "already exists") || !strings.Contains(stderr, "skillhub skill commit taken") {
		t.Fatalf("existing folder: exit = %d stderr=%s", code, stderr)
	}
	code, stdout, stderr := runCLI(t, "skill", "create", "no-description")
	if code != 2 || !strings.Contains(stderr, "create requires a skill id and --description") {
		t.Fatalf("missing flags: exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(".claude", "skills", "no-description")); err == nil {
		t.Fatal("a folder was written without a description")
	}
	if code, _, stderr := runCLI(t, "skill", "create", "x", "--description", "d", "--collection", "core"); code != 2 || !strings.Contains(stderr, "skillhub skill commit") {
		t.Fatalf("--collection belongs to commit: exit = %d stderr=%s", code, stderr)
	}
}

func TestSkillCreateCancelledFormWritesNothing(t *testing.T) {
	project := projectInDir(t)
	savedInteractive, savedInput := skillCreateInteractive, skillCreateInput
	t.Cleanup(func() { skillCreateInteractive, skillCreateInput = savedInteractive, savedInput })
	skillCreateInteractive = func(io.Writer) bool { return true }
	skillCreateInput = strings.NewReader("Half an answer\n")
	var stdout, stderr bytes.Buffer
	if code := RunContext(context.Background(), []string{"skill", "create", "half"}, &stdout, &stderr); code != 130 || !strings.Contains(stderr.String(), "Nothing was written") {
		t.Fatalf("end of input: exit = %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); err == nil {
		t.Fatal("a folder was written")
	}
}
