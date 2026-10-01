package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
)

func TestValidateWorkspaceApplicationContract(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	result, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil || result.Status != StatusOK || result.Error != nil {
		t.Fatalf("validate = %#v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil || result.Status != StatusError || result.Error == nil || result.Error.Code != ErrorWorkspaceInvalid {
		t.Fatalf("invalid validate = %#v, %v", result, err)
	}
}

func TestGetCurationDiffGroupsRelativeCanonicalPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)
	changes := map[string]string{
		"skills/testing/example/SKILL.md": "# Example\n",
		"distill/comparisons/CMP-1.yaml":  "id: CMP-1\n",
		"config/local.yaml":               "enabled: true\n",
	}
	for relative, contents := range changes {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (WorkspaceService{}).GetCurationDiff(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Dirty || len(result.Groups) != 3 || len(result.SuggestedActions) != 1 {
		t.Fatalf("diff = %#v", result)
	}
	for _, group := range result.Groups {
		for _, file := range group.Files {
			if filepath.IsAbs(file.Path) || strings.Contains(file.Path, root) || strings.Contains(file.Path, "runtime/") {
				t.Fatalf("diff leaked local path: %#v", file)
			}
		}
	}
}

func TestParsePorcelainV2RenameClassifiesSourceAndDestination(t *testing.T) {
	output := []byte("2 R. N... 100644 100644 100644 aaaaaaa bbbbbbb R100 sources/catalog/new.yaml\x00skills/core/old/SKILL.md\x00")
	files, err := parsePorcelain(output)
	if err != nil {
		t.Fatal(err)
	}
	want := []DiffFile{{Path: "skills/core/old/SKILL.md", Status: "D"}, {Path: "sources/catalog/new.yaml", Status: "A"}}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("rename effects = %#v, want %#v", files, want)
	}
	groups := groupDiffFiles(files)
	if len(groups) != 2 || groups[0].Kind != "active_skills" || groups[1].Kind != "source_learning" {
		t.Fatalf("rename groups = %#v", groups)
	}
}

func TestParsePorcelainRejectsAbsolutePath(t *testing.T) {
	if _, err := parsePorcelain([]byte("? /tmp/secret\x00")); err == nil {
		t.Fatal("absolute Git path was accepted")
	}
}

func TestInspectGitDisablesConfiguredFSMonitor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)
	marker := filepath.Join(t.TempDir(), "fsmonitor-called")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf called > \"$SKILLHUB_FSMONITOR_MARKER\"\nprintf '\\0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", root, "config", "core.fsmonitor", hook)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("configure fsmonitor: %v: %s", err, output)
	}
	t.Setenv("SKILLHUB_FSMONITOR_MARKER", marker)
	if _, err := inspectGit(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("offline Git status invoked fsmonitor: %v", err)
	}
}
func TestValidateWorkspaceAddsNoRuleBeyondCanonicalValidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	// 1. Workspace is valid: both return 0 issues
	issues, err := canonical.Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 || res.Status != StatusOK {
		t.Fatalf("clean workspace mismatch: canonical=%d, app=%s", len(issues), res.Status)
	}

	// 2. Add an invalid skill with frontmatter mismatch
	skillDir := filepath.Join(root, "skills", "software", "test-skill")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte("schema_version: 1\nid: test-skill\nname: Test\nstatus: draft\ndescription: Test\nrouting: {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: WRONG\n---\n# Test\n"), 0o644)

	issues, err = canonical.Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err = (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != len(res.Items) {
		t.Fatalf("issue count mismatch: canonical=%d, app=%d (ValidateWorkspace added or dropped rules)", len(issues), len(res.Items))
	}
	if res.Status != StatusError {
		t.Fatalf("expected StatusError, got %s", res.Status)
	}
}
