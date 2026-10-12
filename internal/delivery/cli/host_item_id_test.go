package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestParseHostItemID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id               string
		host, kind, path string
		ok               bool
	}{
		{"host_claude-code_mcp-registration", "claude-code", "mcp-registration", "", true},
		{"host_codex-cli_bootstrap-instructions", "codex-cli", "bootstrap-instructions", "", true},
		{"host_claude-code_native-skill:.claude/skills/distill-lab/SKILL.md", "claude-code", "native-skill", ".claude/skills/distill-lab/SKILL.md", true},
		{"host_agents_shared-skill:.agents/skills/distill-lab/scripts/lib/js_yaml:x.mjs", "agents", "shared-skill", ".agents/skills/distill-lab/scripts/lib/js_yaml:x.mjs", true},
		{"host_claude-code", "", "", "", false},
		{"host__native-skill", "", "", "", false},
		{"host_claude-code_:path", "", "", "", false},
		{"validation_issue", "", "", "", false},
	}
	for _, test := range cases {
		host, kind, path, ok := parseHostItemID(test.id)
		if host != test.host || kind != test.kind || path != test.path || ok != test.ok {
			t.Errorf("parseHostItemID(%q) = %q %q %q %v", test.id, host, kind, path, ok)
		}
	}
}

func TestConnectionLinesNeverNameTheSharedFolderAsAnAgent(t *testing.T) {
	t.Parallel()
	result := app.NewResult(app.StatusApplied, "Connected.")
	result.Items = []app.Item{
		{ID: "host_claude-code_mcp-registration", Summary: "claude-code (x) mcp-registration: /p/.mcp.json", Impact: "Written."},
		{ID: "host_claude-code_native-skill:.claude/skills/system-curator/SKILL.md", Summary: "claude-code (x) native-skill: /p/.claude/skills/system-curator/SKILL.md", Impact: "Written."},
		{ID: "host_agents_shared-skill:.agents/skills/system-curator/SKILL.md", Summary: "agents (shared) shared-skill: /p/.agents/skills/system-curator/SKILL.md", Impact: "Written."},
		{ID: "host_codex-cli_native-skill:.claude/skills/x/SKILL.md", Summary: "codex-cli (x) native-skill: /p/x", Impact: app.BlockedImpactPrefix + "not written by Skill Hub"},
	}
	if got := connectionSummaryLine(result); got != "Agent connection written: claude-code." {
		t.Fatalf("summary = %q", got)
	}
	lines := hostChangeLines(result, false)
	want := []string{
		"Claude Code: connected (.mcp.json, system-curator skill)",
		"Agent Skills folder (.agents/skills): connected (system-curator skill)",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines = %q", lines)
	}
	for _, line := range lines {
		if strings.HasPrefix(line, ":") || strings.Contains(line, "agents:") {
			t.Fatalf("a line names no agent: %q", line)
		}
	}
	if connectCurationGuidance(result) == "" {
		t.Fatal("curation guidance no longer fires for the Claude MCP item")
	}
}

func TestHubConnectNamesTheDistillLabSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hub := initTestWorkspace(t)
	for _, path := range []string{".claude/skills/distill-lab", ".agents/skills/distill-lab"} {
		if err := os.RemoveAll(filepath.Join(hub, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := runConnectCLI(t, hub, hub, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Claude Code: will connect (distill-lab skill)") || !strings.Contains(stdout, "Agent Skills folder (.agents/skills): will connect (distill-lab skill)") || strings.Contains(stdout, "curator skill") {
		t.Fatalf("connect output:\n%s", stdout)
	}
}
