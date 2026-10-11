package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyIgnoresTheConnectionFilesThatHoldMachinePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"/.mcp.json", "/.claude/settings.local.json", "/.codex/", "/.gemini/settings.json"} {
		if !hasLine(string(ignore), entry) {
			t.Errorf(".gitignore lacks %s:\n%s", entry, ignore)
		}
	}
}

func TestInspectAsksForMissingMachineFileIgnoreEntriesOnAnExistingWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/runtime/\n/.skillhub/transactions/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, finding := range plan.Findings {
		if finding.ID == "gitignore_entry_missing" && finding.Fixable {
			missing++
		}
	}
	if missing != 4 {
		t.Fatalf("want 4 fixable missing entries, got %d: %#v", missing, plan.Findings)
	}
}

func TestInspectReportsAMachineFileThatGitAlreadyTracks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", root, "add", "-f", ".mcp.json").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}

	plan, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range plan.Findings {
		if finding.ID == "machine_file_tracked" && finding.Path == ".mcp.json" && !finding.Fixable {
			return
		}
	}
	t.Fatalf("a tracked .mcp.json was not reported: %#v", plan.Findings)
}
