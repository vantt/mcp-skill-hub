package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestDoctorFixPreservesBlockedWorkspaceAndUnregisteredHosts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace, project := initTestWorkspace(t), t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{"type": "stdio", "command": binary, "args": []string{"mcp", "serve", "--workspace", workspace}}}})
	path := filepath.Join(project, ".mcp.json")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	codexPath := filepath.Join(project, ".codex", "config.toml")
	codex := []byte("# mcp_servers.skillhub is not registered\n[mcp_servers.skillhub-other]\ncommand = \"keep\"\n")
	if err := os.WriteFile(codexPath, codex, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workspace, ".skillhub", "schema-version")
	original, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("outdated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	code, stdout, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix", "--json")
	if code != 1 {
		t.Fatalf("doctor --fix with a schema that needs a migration exit = %d, want 1: %s", code, stderr)
	}
	var blocked doctorJSON
	if err := json.Unmarshal([]byte(stdout), &blocked); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(blocked.NotFixed, func(item app.NotFixedItem) bool {
		return item.Code == "hub_schema" && strings.Contains(item.Manual, "skillhub maintain migrate")
	}) {
		t.Fatalf("the schema problem is not reported as not fixed with the migrate command: %s", stdout)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, legacy) {
		t.Fatalf("blocked repair changed external registration: %s %v", after, err)
	}
	if err := os.WriteFile(marker, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix"); code != 0 {
		t.Fatal(stderr)
	}
	if after, err := os.ReadFile(codexPath); err != nil || !bytes.Equal(after, codex) {
		t.Fatalf("doctor installed unregistered Codex host: %s %v", after, err)
	}
}

func TestDoctorFixAddsIgnoreEntriesOfAHubThatStillNeedsItsMigration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
	ignore := filepath.Join(hub, ".gitignore")
	replaceInFile(t, ignore, "/.codex/\n", "")
	if err := os.WriteFile(ignore, append([]byte("# mine\nnotes/\n"), mustRead(t, ignore)...), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(hub, ".skillhub", "schema-version")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitOutput(t, hub, "rev-parse", "HEAD"))
	t.Chdir(hub)

	code, stdout, stderr := runCLI(t, "doctor", "--workspace", hub, "--fix", "--json")
	if code != 1 {
		t.Fatalf("doctor --fix exit = %d, want 1 while the migration is pending: %s%s", code, stdout, stderr)
	}
	var result doctorJSON
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Fixed, func(item app.FixedItem) bool { return slices.Contains(item.Codes, "gitignore_entry_missing") }) {
		t.Fatalf("the ignore entries were not added: %s", stdout)
	}
	if !slices.ContainsFunc(result.NotFixed, func(item app.NotFixedItem) bool {
		return item.Code == "hub_schema" && strings.Contains(item.Manual, "maintain migrate")
	}) {
		t.Fatalf("the migration is not named as the next step: %s", stdout)
	}
	after := string(mustRead(t, ignore))
	if !strings.HasPrefix(after, "# mine\nnotes/\n") || !strings.Contains(after, "/.codex/\n") || strings.Count(after, "/runtime/\n") != 1 {
		t.Fatalf(".gitignore after doctor --fix:\n%s", after)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("doctor --fix wrote the schema marker: %v", err)
	}
	if got := strings.TrimSpace(gitOutput(t, hub, "rev-parse", "HEAD")); got != head {
		t.Fatalf("doctor --fix made a commit: %s -> %s", head, got)
	}
	if code, _, stderr := runCLI(t, "maintain", "migrate", "--workspace", hub, "--yes"); code != 0 {
		t.Fatalf("migrate after doctor --fix exit = %d: %s", code, stderr)
	}
	if code, stdout, stderr := runCLI(t, "doctor", "--workspace", hub, "--fix"); code != 0 {
		t.Fatalf("doctor --fix after the migration exit = %d: %s%s", code, stdout, stderr)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestDoctorFixDoesNotRedirectOtherWorkspaceConnections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace, other, project := initTestWorkspace(t), t.TempDir(), t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{"type": "stdio", "command": binary, "args": []string{"mcp", "serve", "--workspace", other}}}})
	paths := []string{filepath.Join(project, ".mcp.json"), filepath.Join(home, ".claude.json")}
	for _, path := range paths {
		if err := os.WriteFile(path, legacy, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(project)
	if code, _, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix"); code != 0 {
		t.Fatal(stderr)
	}
	for _, path := range paths {
		if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, legacy) {
			t.Fatalf("doctor redirected unrelated connection %s: %s %v", path, after, err)
		}
	}
}
