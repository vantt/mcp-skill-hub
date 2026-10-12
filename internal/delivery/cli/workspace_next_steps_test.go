package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommitsAndAsksForNoGitCommand(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := runInitTo(t, root, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "git ") {
		t.Fatalf("init commits the hub itself; its output must not ask for a Git command:\n%s", stdout.String())
	}
	if status := gitOutput(t, root, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("Git status after init:\n%s", status)
	}
}

func TestStatusNextStepOfAFreshWorkspaceIsARealCommand(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := runInitTo(t, root, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	stdout.Reset()
	if code := Run([]string{"status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status=%d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "workspace_diff") || strings.Contains(stdout.String(), "--json") {
		t.Fatalf("the next step must be a command a person can run:\n%s", stdout.String())
	}
}

func TestInitAgainNamesASkillFileItLeftInPlace(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if code, stdout, stderr := runInitCLI(t, root); code != 0 {
		t.Fatalf("init=%d: %s%s", code, stdout, stderr)
	}
	edited := filepath.Join(root, ".agents", "skills", "distill-lab", "SKILL.md")
	if err := os.WriteFile(edited, []byte("my own distill-lab\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runInitCLI(t, root)
	if code != 0 || !strings.Contains(flatText(stdout), edited+" was left unchanged") {
		t.Fatalf("init again=%d:\n%s%s", code, stdout, stderr)
	}
}
