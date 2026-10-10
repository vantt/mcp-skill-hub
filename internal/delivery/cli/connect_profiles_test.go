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

func TestConnectClaudeCurationGuidance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := initTestWorkspace(t)
	for _, command := range []string{"connect", "integrate"} {
		for _, jsonOutput := range []bool{false, true} {
			project := t.TempDir()
			args := []string{command, "--project", project, "--workspace", workspace}
			if jsonOutput {
				args = append(args, "--json")
			}
			for step, apply := range []bool{false, true, true} {
				call := slices.Clone(args)
				if apply {
					call = append(call, "--yes")
				}
				code, stdout, stderr := runCLI(t, call...)
				if code != 0 {
					t.Fatalf("%v exit = %d: %s", call, code, stderr)
				}
				if jsonOutput {
					var response struct {
						app.Result
						CurationGuidance string `json:"curation_guidance"`
					}
					if err := json.Unmarshal([]byte(stdout), &response); err != nil {
						t.Fatalf("machine-readable output: %v: %s", err, stdout)
					}
					if (response.CurationGuidance != "") != (step != 2) {
						t.Fatalf("unexpected optional guidance at step %d: %s", step, stdout)
					}
					if response.CurationGuidance != "" && strings.Contains(response.Summary, response.CurationGuidance) {
						t.Fatal("guidance leaked into JSON summary")
					}
				}
				if !apply {
					if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
						t.Fatalf("preview wrote MCP registration: %v", err)
					}
				}
			}
			data, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
			if err != nil {
				t.Fatal(err)
			}
			var registration struct {
				Servers map[string]struct {
					Args []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(data, &registration); err != nil {
				t.Fatal(err)
			}
			if len(registration.Servers) != 1 {
				t.Fatalf("Claude server count = %d, want 1: %s", len(registration.Servers), data)
			}
			want := []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}
			if !slices.Equal(registration.Servers["skillhub"].Args, want) {
				t.Fatalf("runtime args = %v, want %v", registration.Servers["skillhub"].Args, want)
			}
		}
	}
}

func TestConnectOtherHostsOmitCurationGuidance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := initTestWorkspace(t)
	for _, host := range []string{"codex", "gemini"} {
		project := t.TempDir()
		for _, jsonOutput := range []bool{false, true} {
			args := []string{"connect", "--project", project, "--workspace", workspace, "--host", host}
			if jsonOutput {
				args = append(args, "--json", "--yes")
			}
			code, stdout, stderr := runCLI(t, args...)
			if code != 0 || strings.Contains(stdout, "curation_guidance") || strings.Contains(stdout, "select skillhub-curation") {
				t.Fatalf("%s guidance leaked: exit=%d stdout=%s stderr=%s", host, code, stdout, stderr)
			}
		}
		path := filepath.Join(project, ".codex", "config.toml")
		if host == "gemini" {
			path = filepath.Join(project, ".gemini", "settings.json")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("skillhub-curation")) || bytes.Contains(data, []byte("--profile")) {
			t.Fatalf("%s must retain one full entry: %s", host, data)
		}
	}
}

func TestDoctorMigratesLegacyClaudeConnections(t *testing.T) {
	for _, shape := range []string{"full", "pair"} {
		t.Run(shape, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			workspace, project := initTestWorkspace(t), t.TempDir()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"mcp", "serve", "--workspace", workspace}
			servers := map[string]any{}
			if shape == "pair" {
				args = []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}
				servers["skillhub-curation"] = map[string]any{"type": "stdio", "command": binary, "args": []string{"mcp", "serve", "--profile", "curation", "--workspace", workspace}}
			}
			servers["skillhub"] = map[string]any{"type": "stdio", "command": binary, "args": args}
			legacy, _ := json.Marshal(map[string]any{"mcpServers": servers})
			path := filepath.Join(project, ".mcp.json")
			if err := os.WriteFile(path, legacy, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(project)
			code, stdout, stderr := runCLI(t, "doctor", "--workspace", workspace, "--json")
			if code != 0 {
				t.Fatalf("legacy doctor exit = %d: %s", code, stderr)
			}
			var result app.Result
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatal(err)
			}
			if result.Error != nil || result.Status != app.StatusActionRequired || len(result.Items) != 1 || result.Items[0].ID != "project_connection_outdated" {
				t.Fatalf("valid legacy connection should suggest migration, not be broken: %+v", result)
			}
			if len(result.SuggestedActions) != 1 || !strings.HasPrefix(result.SuggestedActions[0].Command, "skillhub connect --project ") {
				t.Fatalf("missing connect suggestion: %+v", result.SuggestedActions)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, legacy) {
				t.Fatalf("doctor preview modified legacy registration: %v", err)
			}
			if code, _, stderr := runCLI(t, "doctor", "--workspace", workspace, "--fix", "--yes"); code != 0 {
				t.Fatal(stderr)
			}
			after, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Servers map[string]struct {
					Args []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(after, &config); err != nil {
				t.Fatal(err)
			}
			want := []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}
			if len(config.Servers) != 1 || !slices.Equal(config.Servers["skillhub"].Args, want) {
				t.Fatalf("doctor --fix did not migrate %s: %s", shape, after)
			}
			if code, stdout, stderr := runCLI(t, "doctor", "--workspace", workspace, "--json"); code != 0 || strings.Contains(stdout, "connection_outdated") {
				t.Fatalf("migrated connection remains outdated: %d %s %s", code, stdout, stderr)
			}
		})
	}
}
