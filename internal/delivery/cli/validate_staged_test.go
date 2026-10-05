package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestValidateStagedAndWorkingTree(t *testing.T) {
	t.Parallel()
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

func TestValidateCLIOutputWithWarnings(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)

	// Seed skill-c1 with an absolute install path reference in SKILL.md
	c1Dir := filepath.Join(root, "skills", "core", "skill-c1")
	_ = os.MkdirAll(c1Dir, 0o755)
	c1Meta := `schema_version: 1
id: skill-c1
name: Skill C1
status: active
description: First collision skill
routing:
  operations: [operate]
  triggers: [deploy container image to cloud]
  not_for: [unrelated tasks]
  min_scope: multi_step
  examples: [e1, e2, e3]
quality:
  reviewed: true
provenance:
  created_by: skillhub
`
	_ = os.WriteFile(filepath.Join(c1Dir, "skill.meta.yaml"), []byte(c1Meta), 0o644)
	_ = os.WriteFile(filepath.Join(c1Dir, "SKILL.md"), []byte("# Skill C1\nSee ~/.claude/skills/my-skill/run.sh for setup.\n"), 0o644)

	// Seed skill-c2 with identical trigger to create trigger collision
	c2Dir := filepath.Join(root, "skills", "core", "skill-c2")
	_ = os.MkdirAll(c2Dir, 0o755)
	c2Meta := `schema_version: 1
id: skill-c2
name: Skill C2
status: active
description: Second collision skill
routing:
  operations: [operate]
  triggers: [deploy container image to cloud]
  not_for: [unrelated tasks]
  min_scope: multi_step
  examples: [e1, e2, e3]
quality:
  reviewed: true
provenance:
  created_by: skillhub
`
	_ = os.WriteFile(filepath.Join(c2Dir, "skill.meta.yaml"), []byte(c2Meta), 0o644)
	_ = os.WriteFile(filepath.Join(c2Dir, "SKILL.md"), []byte("# Skill C2\nValid instructions.\n"), 0o644)

	// Rebuild catalog so active skills are available in SQLite
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild failed (exit %d): %s", code, stderr.String())
	}

	// 1. JSON output has warnings and exit code 0
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"validate", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("validate json failed (exit %d): %s", code, stderr.String())
	}
	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal validate result: %v", err)
	}

	hasCollision := false
	hasAbsPath := false
	for _, w := range result.Warnings {
		if w.Code == "trigger_collision" {
			hasCollision = true
		}
		if w.Code == "absolute_install_path" {
			hasAbsPath = true
		}
	}
	if !hasCollision {
		t.Errorf("expected trigger_collision in warnings: %+v", result.Warnings)
	}
	if !hasAbsPath {
		t.Errorf("expected absolute_install_path in warnings: %+v", result.Warnings)
	}

	// 2. Human output prints WARN lines and exit code 0
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "--workspace", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("validate human failed (exit %d): %s", code, stderr.String())
	}
	outStr := stdout.String()
	if !strings.Contains(outStr, "WARN trigger_collision") {
		t.Errorf("human output missing WARN trigger_collision:\n%s", outStr)
	}
	if !strings.Contains(outStr, "WARN absolute_install_path") {
		t.Errorf("human output missing WARN absolute_install_path:\n%s", outStr)
	}
}
