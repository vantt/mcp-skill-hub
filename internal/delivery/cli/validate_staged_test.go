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

func TestValidateCLIOutputsMetadataLintWarnings(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)

	// Seed skill A with absolute install path in SKILL.md
	skillADir := filepath.Join(root, "skills", "core", "skill-a")
	if err := os.MkdirAll(skillADir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillAMeta := `schema_version: 1
id: skill-a
name: Skill A
status: active
description: First skill with trigger.
routing:
  triggers: [manage cloud infrastructure]
  not_for: [unrelated tasks]
  min_scope: single_step
  examples: [ex1, ex2, ex3]
quality:
  reviewed: true
`
	if err := os.WriteFile(filepath.Join(skillADir, "skill.meta.yaml"), []byte(skillAMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	skillAMD := `---
name: skill-a
description: First skill with trigger.
---
# Skill A
cp helper.sh ~/.claude/skills/target
`
	if err := os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte(skillAMD), 0o644); err != nil {
		t.Fatal(err)
	}

	// Seed skill B colliding with skill A's trigger
	skillBDir := filepath.Join(root, "skills", "core", "skill-b")
	if err := os.MkdirAll(skillBDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillBMeta := `schema_version: 1
id: skill-b
name: Skill B
status: active
description: Second skill colliding with first.
routing:
  triggers: [manage cloud infrastructure]
  not_for: [unrelated tasks]
  min_scope: single_step
  examples: [ex1, ex2, ex3]
quality:
  reviewed: true
`
	if err := os.WriteFile(filepath.Join(skillBDir, "skill.meta.yaml"), []byte(skillBMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	skillBMD := `---
name: skill-b
description: Second skill colliding with first.
---
# Skill B
Normal skill body.
`
	if err := os.WriteFile(filepath.Join(skillBDir, "SKILL.md"), []byte(skillBMD), 0o644); err != nil {
		t.Fatal(err)
	}

	// Rebuild catalog so active skills are queryable
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild failed: %s", stderr.String())
	}

	// 1. JSON validate output
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"validate", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0 for validate with warnings, got %d: %s", code, stderr.String())
	}
	var res app.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal validate result: %v", err)
	}
	foundCollision := false
	foundAbsolute := false
	for _, w := range res.Warnings {
		if w.Code == "trigger_collision" {
			foundCollision = true
		}
		if w.Code == "absolute_install_path" {
			foundAbsolute = true
		}
	}
	if !foundCollision {
		t.Fatalf("expected trigger_collision warning in JSON, got: %v", res.Warnings)
	}
	if !foundAbsolute {
		t.Fatalf("expected absolute_install_path warning in JSON, got: %v", res.Warnings)
	}

	// 2. Human validate output prints WARN lines
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"validate", "--workspace", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0 for human validate, got %d: %s", code, stderr.String())
	}
	humanOutput := stdout.String()
	if !strings.Contains(humanOutput, "WARN trigger_collision") {
		t.Fatalf("expected WARN trigger_collision in human output, got:\n%s", humanOutput)
	}
	if !strings.Contains(humanOutput, "WARN absolute_install_path") {
		t.Fatalf("expected WARN absolute_install_path in human output, got:\n%s", humanOutput)
	}
}
