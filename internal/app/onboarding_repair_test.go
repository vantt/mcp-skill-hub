package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

// A hub made by an older binary lacks only the newer .gitignore entries. That
// is one fixable finding: the workspace stays valid and the other checks run.
func TestMissingGitignoreEntryIsOnlyAFixableFinding(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	ignore := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(content), "/.agents/skills/distill-lab/\n", "", 1)
	if stripped == string(content) {
		t.Fatal("fixture .gitignore has no distill-lab entry to remove")
	}
	if err := os.WriteFile(ignore, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}
	// The older binary published its index from the same files.
	if err := catalog.EnsureFreshOrRebuild(t.Context(), root); err != nil {
		t.Fatal(err)
	}

	report := RoleReport{Cwd: root, Hub: &HubRef{Root: root}}
	sections := Run(t.Context(), report, DepthFull)
	if got := codesOf(sections, SeverityProblem); !slices.Equal(got, []string{"gitignore_entry_missing"}) {
		t.Fatalf("problems = %v, want only the missing entry\n%+v", got, sections)
	}
	if skipped := codesOf(sections, SeveritySkipped); len(skipped) != 0 {
		t.Fatalf("checks skipped behind a missing .gitignore entry: %v", skipped)
	}
	if finding := findingByCode(t, sections, "gitignore_entry_missing"); finding.Fix == nil {
		t.Fatalf("the missing entry must be fixable: %+v", finding)
	}
	home, err := (CurationService{NoWait: true}).GetCurationHome(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if home.HomeSummary.InvalidWorkspaces != 0 || home.Error != nil {
		t.Fatalf("workspace reported invalid: %+v", home.Result)
	}

	fixes := ApplyFixes(t.Context(), sections)
	if len(fixes.NotFixed) != 0 {
		t.Fatalf("not fixed = %+v", fixes.NotFixed)
	}
	if got := codesOf(Run(t.Context(), report, DepthFull), SeverityProblem); len(got) != 0 {
		t.Fatalf("problems after the fix = %v", got)
	}
}

// In a hub, CLAUDE.md, AGENTS.md and GEMINI.md are committed hub content:
// removing the connection takes out the machine files and leaves them alone.
func TestDisconnectInAHubKeepsItsCommittedInstructionFiles(t *testing.T) {
	root := healthHub(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	instructions := map[string][]byte{}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		instructions[name] = raw
	}

	service := ConnectService{Store: userconfig.NewStore(t.TempDir())}
	result, _, err := service.Disconnect(t.Context(), DisconnectRequest{Project: root})
	if err != nil || result.Error != nil {
		t.Fatalf("disconnect: %v %+v", err, result.Error)
	}
	for name, want := range instructions {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != string(want) {
			t.Fatalf("%s changed by connect --remove: %v\n%s", name, err, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf(".mcp.json should be gone: %v", err)
	}
}

// In a client project the managed block goes; the user's own text stays, and a
// file that held nothing else is deleted.
func TestDisconnectInAProjectKeepsUserTextAndDeletesAnEmptiedFile(t *testing.T) {
	hub := healthHub(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := ConnectService{Store: userconfig.NewStore(t.TempDir())}
	if result, err := service.Connect(t.Context(), ConnectRequest{Project: project, Workspace: hub}); err != nil || result.Error != nil {
		t.Fatalf("connect: %v %+v", err, result.Error)
	}
	if result, _, err := service.Disconnect(t.Context(), DisconnectRequest{Project: project}); err != nil || result.Error != nil {
		t.Fatalf("disconnect: %v %+v", err, result.Error)
	}
	got, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil || strings.TrimSpace(string(got)) != "my notes" {
		t.Fatalf("CLAUDE.md after removal = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md was created by connect and held nothing else: %v", err)
	}
}
