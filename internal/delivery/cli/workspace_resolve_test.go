package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initWorkspace(t *testing.T) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "ws")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", ws, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init failed: %d: %s", code, stderr.String())
	}
	return ws
}

func TestWorkspaceResolutionPrecedence(t *testing.T) {
	wsFlag := initWorkspace(t)
	wsEnv := initWorkspace(t)
	wsConn := initWorkspace(t)
	wsCwd := initWorkspace(t)

	// Set up project with connection file pointing to wsConn
	proj := t.TempDir()
	mcpJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, wsConn)
	if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. Flag wins over Env, Connection, and Discovery
	t.Setenv(workspaceEnvVar, wsEnv)
	chdir(t, proj)
	got, err := resolveWorkspace(wsFlag)
	if err != nil {
		t.Fatalf("resolveWorkspace(flag) failed: %v", err)
	}
	if got != wsFlag {
		t.Fatalf("expected flag workspace %q, got %q", wsFlag, got)
	}

	// 2. Env wins over Connection and Discovery
	got, err = resolveWorkspace("")
	if err != nil {
		t.Fatalf("resolveWorkspace(env) failed: %v", err)
	}
	if got != wsEnv {
		t.Fatalf("expected env workspace %q, got %q", wsEnv, got)
	}

	// 3. Connection file wins over Discovery (when inside project with connection file)
	t.Setenv(workspaceEnvVar, "")
	got, err = resolveWorkspace("")
	if err != nil {
		t.Fatalf("resolveWorkspace(conn) failed: %v", err)
	}
	if got != wsConn {
		t.Fatalf("expected conn workspace %q, got %q", wsConn, got)
	}

	// 4. Discovery finds workspace from inside wsCwd
	chdir(t, filepath.Join(wsCwd, "skills"))
	got, err = resolveWorkspace("")
	if err != nil {
		t.Fatalf("resolveWorkspace(discovery) failed: %v", err)
	}
	if got != wsCwd {
		t.Fatalf("expected discovery workspace %q, got %q", wsCwd, got)
	}
}

func TestWorkspaceResolutionConnectionFileFormats(t *testing.T) {
	ws := initWorkspace(t)

	// Test Claude Code (.mcp.json)
	t.Run("ClaudeCode", func(t *testing.T) {
		proj := t.TempDir()
		mcpJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, ws)
		if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, proj)
		t.Setenv(workspaceEnvVar, "")
		got, err := resolveWorkspace("")
		if err != nil || got != ws {
			t.Fatalf("expected %q, got %q (err=%v)", ws, got, err)
		}
	})

	// Test Codex (.codex/config.toml)
	t.Run("CodexTOML", func(t *testing.T) {
		proj := t.TempDir()
		if err := os.MkdirAll(filepath.Join(proj, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		codexTOML := fmt.Sprintf("[mcp_servers.skillhub]\ncommand = \"skillhub\"\nargs = [\"mcp\", \"serve\", \"--workspace\", %q]\ncwd = %q\n", ws, ws)
		if err := os.WriteFile(filepath.Join(proj, ".codex", "config.toml"), []byte(codexTOML), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, proj)
		t.Setenv(workspaceEnvVar, "")
		got, err := resolveWorkspace("")
		if err != nil || got != ws {
			t.Fatalf("expected %q, got %q (err=%v)", ws, got, err)
		}
	})

	// Test Gemini (.gemini/settings.json)
	t.Run("GeminiSettings", func(t *testing.T) {
		proj := t.TempDir()
		if err := os.MkdirAll(filepath.Join(proj, ".gemini"), 0o755); err != nil {
			t.Fatal(err)
		}
		geminiJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, ws)
		if err := os.WriteFile(filepath.Join(proj, ".gemini", "settings.json"), []byte(geminiJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, proj)
		t.Setenv(workspaceEnvVar, "")
		got, err := resolveWorkspace("")
		if err != nil || got != ws {
			t.Fatalf("expected %q, got %q (err=%v)", ws, got, err)
		}
	})

	// Test upward search from subfolder of connected project
	t.Run("SubfolderOfConnectedProject", func(t *testing.T) {
		proj := t.TempDir()
		sub := filepath.Join(proj, "src", "pkg", "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		mcpJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, ws)
		if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, sub)
		t.Setenv(workspaceEnvVar, "")
		got, err := resolveWorkspace("")
		if err != nil || got != ws {
			t.Fatalf("expected %q from subfolder, got %q (err=%v)", ws, got, err)
		}
	})
}

func TestWorkspaceResolutionUntrustedInput(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")

	// 1. Relative path in connection file must be rejected
	t.Run("RelativePathRejected", func(t *testing.T) {
		proj := t.TempDir()
		mcpJSON := `{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace","relative/path"]}}}`
		if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, proj)
		_, err := resolveWorkspace("")
		if err == nil {
			t.Fatal("expected error for relative path in connection file")
		}
	})

	// 2. Non-existent path in connection file must be rejected
	t.Run("NonExistentPathRejected", func(t *testing.T) {
		proj := t.TempDir()
		mcpJSON := `{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace","/nonexistent/path/xyz"]}}}`
		if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, proj)
		_, err := resolveWorkspace("")
		if err == nil {
			t.Fatal("expected error for non-existent path in connection file")
		}
	})

	// 3. Existing directory without .skillhub/schema-version must be rejected
	t.Run("UninitializedDirRejected", func(t *testing.T) {
		proj := t.TempDir()
		emptyDir := t.TempDir()
		mcpJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, emptyDir)
		if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		chdir(t, proj)
		_, err := resolveWorkspace("")
		if err == nil {
			t.Fatal("expected error for uninitialized dir in connection file")
		}
	})

	// 4. Symlink escaping project root must be rejected
	t.Run("EscapingSymlinkRejected", func(t *testing.T) {
		proj := t.TempDir()
		externalDir := t.TempDir()
		externalFile := filepath.Join(externalDir, "escape.json")
		ws := initWorkspace(t)
		mcpJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, ws)
		if err := os.WriteFile(externalFile, []byte(mcpJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(externalFile, filepath.Join(proj, ".mcp.json")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		chdir(t, proj)
		_, err := resolveWorkspace("")
		if err == nil {
			t.Fatal("expected escaping symlink to be rejected")
		}
	})

	// 5. Oversized connection file (> 1MB) must be rejected
	t.Run("OversizedFileRejected", func(t *testing.T) {
		proj := t.TempDir()
		largeFile, err := os.Create(filepath.Join(proj, ".mcp.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := largeFile.Truncate(2 * 1024 * 1024); err != nil {
			largeFile.Close()
			t.Fatal(err)
		}
		largeFile.Close()
		chdir(t, proj)
		_, err = resolveWorkspace("")
		if err == nil {
			t.Fatal("expected oversized connection file to be rejected")
		}
	})
}

func TestWorkspaceResolutionNeverScansHome(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	// Put a valid workspace right inside fakeHome/.skillhub/schema-version
	if err := os.MkdirAll(filepath.Join(fakeHome, ".skillhub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeHome, ".skillhub", "schema-version"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. Starting at home directory directly
	chdir(t, fakeHome)
	_, err := resolveWorkspace("")
	if err == nil {
		t.Fatal("expected resolution to fail because home directory must never be scanned")
	}
	if err.Why != "No Skill Hub workspace was found from the current directory." {
		t.Fatalf("unexpected Why: %q", err.Why)
	}

	// 2. Starting at a subdirectory of home directory without a workspace
	sub := filepath.Join(fakeHome, "sub", "project")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)
	_, err = resolveWorkspace("")
	if err == nil {
		t.Fatal("expected resolution to stop before home directory")
	}
	if err.Why != "No Skill Hub workspace was found from the current directory." {
		t.Fatalf("unexpected Why: %q", err.Why)
	}
}

func TestConsistentErrorMessages(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")

	// 1. Non-existent path
	_, err := resolveWorkspace("/nonexistent/workspace/path")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
	expectedWhy := "No Skill Hub workspace at /nonexistent/workspace/path."
	if err.Why != expectedWhy {
		t.Fatalf("expected Why %q, got %q", expectedWhy, err.Why)
	}
	if err.Fix != workspaceFix {
		t.Fatalf("expected Fix %q, got %q", workspaceFix, err.Fix)
	}

	// 2. Existing path that is not an initialized workspace
	emptyDir := t.TempDir()
	_, err = resolveWorkspace(emptyDir)
	if err == nil {
		t.Fatal("expected error for uninitialized path")
	}
	expectedWhy = fmt.Sprintf("No Skill Hub workspace was found at %s.", emptyDir)
	if err.Why != expectedWhy {
		t.Fatalf("expected Why %q, got %q", expectedWhy, err.Why)
	}
	if err.Fix != workspaceFix {
		t.Fatalf("expected Fix %q, got %q", workspaceFix, err.Fix)
	}

	// 3. No workspace given and discovery fails
	cleanDir := t.TempDir()
	chdir(t, cleanDir)
	_, err = resolveWorkspace("")
	if err == nil {
		t.Fatal("expected error when no workspace found")
	}
	expectedWhy = "No Skill Hub workspace was found from the current directory."
	if err.Why != expectedWhy {
		t.Fatalf("expected Why %q, got %q", expectedWhy, err.Why)
	}
	if err.Fix != workspaceFix {
		t.Fatalf("expected Fix %q, got %q", workspaceFix, err.Fix)
	}
}

func TestNoNonInteractiveUseWordingInAnyError(t *testing.T) {
	emptyDir := t.TempDir()
	chdir(t, emptyDir)
	t.Setenv(workspaceEnvVar, "")

	commands := [][]string{
		{"status"},
		{"doctor"},
		{"validate"},
		{"diff"},
		{"rebuild"},
		{"skill", "list"},
		{"skill", "show", "foo"},
		{"source", "list"},
		{"source", "capture", "https://example.com", "--reason", "test"},
		{"distill", "prepare", "--all-changed"},
		{"inbox"},
		{"insight", "show", "INS-1"},
		{"check"},
		{"migrate"},
		{"resolve", "--request", "req.json"},
		{"telemetry", "health"},
	}

	for _, cmd := range commands {
		var stdout, stderr bytes.Buffer
		code := Run(cmd, &stdout, &stderr)
		if code == 0 {
			t.Errorf("command %v succeeded unexpectedly", cmd)
		}
		allOutput := stdout.String() + "\n" + stderr.String()
		if strings.Contains(strings.ToLower(allOutput), "non-interactive") {
			t.Errorf("command %v contained 'non-interactive' wording:\n%s", cmd, allOutput)
		}
	}
}

func TestSubcommandTyposAndFlagsReportedBeforeWorkspaceError(t *testing.T) {
	emptyDir := t.TempDir()
	chdir(t, emptyDir)
	t.Setenv(workspaceEnvVar, "")

	// 1. skill lst (typo) -> reports unknown subcommand, NOT workspace error
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "lst"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected skill lst to fail")
	}
	if !strings.Contains(stderr.String(), "unsupported skill subcommand") {
		t.Fatalf("expected unsupported subcommand error, got:\n%s", stderr.String())
	}

	// 2. skill create --id foo (missing required flags) -> reports missing flags, NOT workspace error
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"skill", "create", "--id", "foo"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected skill create --id foo to fail")
	}
	if !strings.Contains(stderr.String(), "create requires --id, --collection, --name, and --description") {
		t.Fatalf("expected missing flags error, got:\n%s", stderr.String())
	}

	// 3. source foo (typo) -> reports unsupported subcommand, NOT workspace error
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"source", "foo"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected source foo to fail")
	}
	if !strings.Contains(stderr.String(), "unsupported source subcommand") {
		t.Fatalf("expected unsupported subcommand error, got:\n%s", stderr.String())
	}

	// 4. distill foo (typo) -> reports unsupported subcommand, NOT workspace error
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"distill", "foo"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected distill foo to fail")
	}
	if !strings.Contains(stderr.String(), "unsupported distill subcommand") {
		t.Fatalf("expected unsupported subcommand error, got:\n%s", stderr.String())
	}

	// 5. insight foo (typo) -> reports unsupported subcommand, NOT workspace error
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"insight", "foo"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected insight foo to fail")
	}
	if !strings.Contains(stderr.String(), "unsupported insight subcommand") {
		t.Fatalf("expected unsupported subcommand error, got:\n%s", stderr.String())
	}

	// 6. telemetry purge without --yes -> reports --yes requirement, NOT workspace error
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"telemetry", "purge"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected telemetry purge to fail")
	}
	if !strings.Contains(stderr.String(), "telemetry purge requires --yes") {
		t.Fatalf("expected --yes requirement error, got:\n%s", stderr.String())
	}
}

func TestDoctorFixDoesNotCreateWorkspaceAtMissingPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "nonexistent", "workspace")
	var stdout, stderr bytes.Buffer

	// doctor without fix on missing path fails and creates nothing
	code := Run([]string{"doctor", "--workspace", missingPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("doctor on missing path should fail")
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("doctor created directory: %v", err)
	}

	// doctor --fix --yes on missing path fails and creates nothing
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"doctor", "--fix", "--yes", "--workspace", missingPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("doctor --fix --yes on missing path should fail")
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("doctor --fix --yes created directory: %v", err)
	}
	if !strings.Contains(stderr.String(), "No Skill Hub workspace at "+missingPath) {
		t.Fatalf("expected 'No Skill Hub workspace at %s', got:\n%s", missingPath, stderr.String())
	}
}

func TestStatusInsideConnectedProjectSucceeds(t *testing.T) {
	ws := initWorkspace(t)
	proj := t.TempDir()

	mcpJSON := fmt.Sprintf(`{"mcpServers":{"skillhub":{"command":"skillhub","args":["mcp","serve","--workspace",%q]}}}`, ws)
	if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(mcpJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	chdir(t, proj)
	t.Setenv(workspaceEnvVar, "")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status inside connected project failed (code=%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Workspace is") {
		t.Fatalf("unexpected status output:\n%s", stdout.String())
	}
}

func TestUnknownIDsExitNonZero(t *testing.T) {
	ws := initWorkspace(t)

	// 1. check <unknown-id> exits non-zero and reports unknown source
	var stdout, stderr bytes.Buffer
	code := Run([]string{"check", "unknown-source-id", "--workspace", ws}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("check unknown-source-id should exit non-zero")
	}
	if !strings.Contains(stderr.String(), "unknown-source-id") {
		t.Fatalf("expected error mentioning unknown-source-id, got:\n%s", stderr.String())
	}

	// 2. skill show <unknown-id> exits non-zero
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"skill", "show", "nonexistent-skill", "--workspace", ws}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("skill show nonexistent-skill should exit non-zero")
	}
	if !strings.Contains(stderr.String(), "skill not found") {
		t.Fatalf("expected 'skill not found', got:\n%s", stderr.String())
	}

	// 3. No raw OS errors like "statat" in error output
	if strings.Contains(stderr.String(), "statat") {
		t.Fatalf("raw OS error 'statat' leaked in stderr:\n%s", stderr.String())
	}
}

func TestAllCommandFailuresExitNonZero(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")
	cleanDir := t.TempDir()
	chdir(t, cleanDir)

	table := []struct {
		name string
		args []string
	}{
		{"status missing ws", []string{"status"}},
		{"validate missing ws", []string{"validate"}},
		{"doctor missing ws", []string{"doctor"}},
		{"diff missing ws", []string{"diff"}},
		{"rebuild missing ws", []string{"rebuild"}},
		{"skill list missing ws", []string{"skill", "list"}},
		{"skill show missing id", []string{"skill", "show"}},
		{"skill unknown subcommand", []string{"skill", "invalidsub"}},
		{"source list missing ws", []string{"source", "list"}},
		{"source unknown subcommand", []string{"source", "invalidsub"}},
		{"distill prepare missing ws", []string{"distill", "prepare", "--all-changed"}},
		{"distill unknown subcommand", []string{"distill", "invalidsub"}},
		{"inbox missing ws", []string{"inbox"}},
		{"insight show missing id", []string{"insight", "show"}},
		{"insight unknown subcommand", []string{"insight", "invalidsub"}},
		{"check missing ws", []string{"check"}},
		{"resolve missing args", []string{"resolve"}},
		{"eval unknown subcommand", []string{"eval", "invalidsub"}},
		{"telemetry unknown subcommand", []string{"telemetry", "invalidsub"}},
		{"mcp invalid subcommand", []string{"mcp", "invalidsub"}},
		{"unknown root command", []string{"nonexistentcommand"}},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tt.args, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("command %v exited 0, want non-zero", tt.args)
			}
		})
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}
