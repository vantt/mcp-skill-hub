package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateStagedAndWorkingTree(t *testing.T) {
	root := initTestWorkspace(t)

	// 1. Working tree validate passes on clean workspace
	var stdout, stderr bytes.Buffer
	code := Run([]string{"validate", "--workspace", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clean validate failed (exit %d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Workspace validation passed.") {
		t.Errorf("expected passed message, got: %s", stdout.String())
	}
	// Commit the initialized workspace baseline so the Git index has all canonical baseline files
	_ = exec.Command("git", "-C", root, "add", "-A").Run()
	_ = exec.Command("git", "-C", root, "commit", "-m", "baseline").Run()

	// 2. Validate --staged passes on clean index
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "--staged", "--workspace", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clean staged validate failed (exit %d): %s", code, stderr.String())
	}
	// 3. Stage a broken skill file into git index
	brokenSkillDir := filepath.Join(root, "skills", "core", "broken-skill")
	_ = os.MkdirAll(brokenSkillDir, 0o700)
	brokenFile := filepath.Join(brokenSkillDir, "SKILL.md")
	// Invalid frontmatter: missing closing ---
	_ = os.WriteFile(brokenFile, []byte("---\nname: broken-skill\ndescription: Broken\n"), 0o600)

	gitCmd := exec.Command("git", "-C", root, "add", "skills/core/broken-skill/SKILL.md")
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v: %s", err, string(out))
	}

	// Staged validation should now fail
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "--staged", "--workspace", root}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 on broken staged file, got %d (stdout: %s, stderr: %s)", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Workspace validation failed:") {
		t.Errorf("stderr missing validation failed line:\n%s", stderr.String())
	}

	// 4. Incompatible / unknown flags fail before Git access
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "--bogus-flag", "--workspace", root}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 on unknown argument, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown argument") {
		t.Errorf("expected unknown argument error, got: %s", stderr.String())
	}
}
