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
				guidance := stdout
				alreadyCurrent := step == 2
				if jsonOutput {
					var response struct {
						app.Result
						CurationGuidance string `json:"curation_guidance"`
					}
					if err := json.Unmarshal([]byte(stdout), &response); err != nil {
						t.Fatalf("machine-readable output: %v: %s", err, stdout)
					}
					guidance = response.CurationGuidance
					if strings.Contains(response.Summary, "/mcp") {
						t.Fatal("guidance must not be appended to existing JSON summary")
					}
				}
				wantCount := 1
				if alreadyCurrent {
					wantCount = 0
				}
				if got := strings.Count(guidance, "/mcp"); got != wantCount {
					t.Fatalf("%v reminder count = %d, want %d: %s", call, got, wantCount, stdout)
				}
				if !alreadyCurrent {
					for _, action := range []string{"skillhub-curation", "Disable", "Enable"} {
						if !strings.Contains(guidance, action) {
							t.Fatalf("curation guidance missing %q: %s", action, guidance)
						}
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

func TestDoctorLegacyClaudeFullProjectSuggestsConnect(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := initTestWorkspace(t)
	project := t.TempDir()
	if code, _, stderr := runCLI(t, "connect", "--project", project, "--workspace", workspace, "--yes"); code != 0 {
		t.Fatal(stderr)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{
		"type": "stdio", "command": binary, "args": []string{"mcp", "serve", "--workspace", workspace},
	}}})
	if err != nil {
		t.Fatal(err)
	}
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
	if result.Error != nil || result.Status == app.StatusError {
		t.Fatalf("valid full entry must not be broken: %+v", result)
	}
	for _, item := range result.Items {
		if item.ID != "project_connection_outdated" {
			t.Fatalf("unexpected legacy finding: %+v", item)
		}
	}
	for _, action := range result.SuggestedActions {
		if !strings.HasPrefix(action.Command, "skillhub connect --project ") {
			t.Fatalf("legacy entry should at most suggest connect: %+v", action)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, legacy) {
		t.Fatalf("doctor modified legacy registration: %v", err)
	}
}
