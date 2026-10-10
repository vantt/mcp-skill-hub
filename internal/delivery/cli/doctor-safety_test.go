package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	code, stdout, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix", "--yes", "--json")
	if code != 0 {
		t.Fatalf("doctor failed: %d %s", code, stderr)
	}
	var blocked app.Result
	if err := json.Unmarshal([]byte(stdout), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.Status != app.StatusActionRequired {
		t.Fatalf("blocked schema reported success: %+v", blocked)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, legacy) {
		t.Fatalf("blocked repair changed external registration: %s %v", after, err)
	}
	if err := os.WriteFile(marker, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix", "--yes"); code != 0 {
		t.Fatal(stderr)
	}
	if after, err := os.ReadFile(codexPath); err != nil || !bytes.Equal(after, codex) {
		t.Fatalf("doctor installed unregistered Codex host: %s %v", after, err)
	}
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
	if code, _, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix", "--yes"); code != 0 {
		t.Fatal(stderr)
	}
	for _, path := range paths {
		if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, legacy) {
			t.Fatalf("doctor redirected unrelated connection %s: %s %v", path, after, err)
		}
	}
}
