package hostintegration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRootIsSeparateFromServedWorkspace(t *testing.T) {
	workspace := t.TempDir()
	project := t.TempDir()
	binary := filepath.Join(workspace, "bin", "skillhub")
	request := Request{Workspace: workspace, Root: project, Binary: binary}

	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Changes) != 9 {
		t.Fatalf("change count = %d, want 9", len(plan.Changes))
	}
	for _, change := range plan.Changes {
		if !strings.HasPrefix(change.Path, project+string(filepath.Separator)) {
			t.Fatalf("change path %s is outside the project root", change.Path)
		}
	}
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertJSONRegistration(t, filepath.Join(project, ".mcp.json"), binary, workspace, false)
	assertContains(t, filepath.Join(project, ".codex/config.toml"), "cwd = "+tomlString(workspace))
	assertContains(t, filepath.Join(project, "CLAUDE.md"), bootstrapStart)
	if _, err := os.Stat(filepath.Join(workspace, ".mcp.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace received host files, stat error = %v", err)
	}

	again, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	if len(again.Changes) != 0 {
		t.Fatalf("second plan has %d changes, want idempotent no-op", len(again.Changes))
	}
}

func TestUserScopeWritesHomeFilesAndPreservesUnrelatedClaudeKeys(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	binary := filepath.Join(workspace, "bin", "skillhub")
	claudeJSON := "{\n  \"numStartups\": 42,\n  \"projects\": {\"/some/project\": {\"mcpServers\": {\"local\": {\"command\": \"x\"}}}},\n  \"mcpServers\": {\"other\": {\"command\": \"other\"}}\n}\n"
	writeTestFile(t, filepath.Join(home, ".claude.json"), []byte(claudeJSON), 0o600)
	writeTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), []byte("# My global rules\n"), 0o640)
	request := Request{Workspace: workspace, Root: home, Scope: ScopeUser, Binary: binary}

	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertJSONRegistration(t, filepath.Join(home, ".claude.json"), binary, workspace, false)
	assertContains(t, filepath.Join(home, ".claude.json"), `"numStartups": 42`)
	assertContains(t, filepath.Join(home, ".claude.json"), `"local": {"command": "x"}`)
	assertContains(t, filepath.Join(home, ".claude.json"), `"other": {"command": "other"}`)
	assertMode(t, filepath.Join(home, ".claude.json"), 0o600)
	assertContains(t, filepath.Join(home, ".claude", "CLAUDE.md"), "# My global rules")
	assertContains(t, filepath.Join(home, ".claude", "CLAUDE.md"), bootstrapStart)

	for _, path := range []string{
		".claude/skills/system-curator/SKILL.md",
		".agents/skills/system-curator/SKILL.md",
		".gemini/skills/system-curator/SKILL.md",
		".codex/config.toml", ".codex/AGENTS.md",
		".gemini/settings.json", ".gemini/GEMINI.md",
	} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected user-scope file %s: %v", path, err)
		}
	}
	assertJSONRegistration(t, filepath.Join(home, ".gemini/settings.json"), binary, workspace, true)
	// Project-scope names must not leak into the home directory.
	for _, path := range []string{".mcp.json", "CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		if _, err := os.Stat(filepath.Join(home, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("project-scope file %s was written into home, stat error = %v", path, err)
		}
	}

	again, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	if len(again.Changes) != 0 {
		t.Fatalf("second plan has %d changes, want idempotent no-op", len(again.Changes))
	}
}

func TestUserScopeRefusesWhenClaudeConfigChangesAfterPlanning(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".claude.json"), []byte("{\"mcpServers\":{}}\n"), 0o600)
	request := Request{Workspace: workspace, Root: home, Scope: ScopeUser, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// Claude Code rewrites this file while it runs; a concurrent edit must win.
	writeTestFile(t, filepath.Join(home, ".claude.json"), []byte("{\"mcpServers\":{},\"changed\":true}\n"), 0o600)
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("Apply error = %v, want ErrStalePlan", err)
	}
	assertContains(t, filepath.Join(home, ".claude.json"), `"changed":true`)
}

func TestApplyRejectsPlanPathOutsideScopeContract(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	request := Request{Workspace: workspace, Root: home, Scope: ScopeUser, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// A user-scope plan replayed as project scope must not match the contract.
	plan.Scope = ScopeProject
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err == nil || !strings.Contains(err.Error(), "host contract") {
		t.Fatalf("Apply error = %v, want host contract violation", err)
	}
}
