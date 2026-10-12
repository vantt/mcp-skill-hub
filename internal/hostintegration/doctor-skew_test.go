package hostintegration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// Unknown content at a system skill path is the user's: doctor reports it and
// doctor --fix leaves it in place. A version an earlier release shipped is
// Skill Hub's and is upgraded without a question.
func TestDoctorReportsAChangedCuratorAndLeavesIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "workspace")
	service := app.WorkspaceService{}
	if _, err := service.Init(root, app.InitOptions{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agents", "skills", "system-curator", "SKILL.md")
	stale := []byte("---\nname: system-curator\n---\nPrevious curator instructions.\n")
	if err := os.WriteFile(path, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := service.Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != app.StatusActionRequired {
		t.Fatalf("doctor status: %s", result.Status)
	}
	found := false
	for _, item := range result.Items {
		if item.ID == "host_agents_shared-skill:.agents/skills/system-curator/SKILL.md" && strings.Contains(item.Summary, path) && strings.Contains(item.Impact, "not written by Skill Hub") {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor omitted the changed curator: %#v", result.Items)
	}
	if _, err := service.DoctorFix(root, false); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(path); !bytes.Equal(content, stale) {
		t.Fatal("doctor --fix overwrote content Skill Hub did not write")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "system-curator", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("the wrapper still points at foreign content: %v", err)
	}
}

func TestDoctorFixUpgradesAShippedCuratorVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	command := exec.Command("git", "show", "v0.1.0:internal/systemskills/curator/SKILL.md")
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	previous, err := command.Output()
	if err != nil {
		t.Skipf("the v0.1.0 curator is not in this checkout: %v", err)
	}
	root := filepath.Join(t.TempDir(), "workspace")
	service := app.WorkspaceService{}
	if _, err := service.Init(root, app.InitOptions{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agents", "skills", "system-curator", "SKILL.md")
	if err := os.WriteFile(path, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DoctorFix(root, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, []byte(systemskills.CuratorBundle().Instructions)) {
		t.Fatal("doctor --fix did not upgrade a shipped curator version")
	}
}
