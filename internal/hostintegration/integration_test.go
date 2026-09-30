package hostintegration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

func TestPlanApplyAllHostsIsDependencyOrderedAndIdempotent(t *testing.T) {
	workspace := t.TempDir()
	binary := filepath.Join(workspace, "bin", "skillhub")
	request := Request{Workspace: workspace, Binary: binary}

	claudeJSON := "{\r\n  \"unrelated\": { \"keep\": true },\r\n  \"mcpServers\": {\r\n    \"other\": {\"command\":\"other\"}\r\n  }\r\n}"
	writeTestFile(t, filepath.Join(workspace, ".mcp.json"), []byte(claudeJSON), 0o600)
	geminiJSON := "{\n  \"theme\": \"user-owned\",\n  \"mcpServers\": {\"other\": {\"command\": \"x\"}}\n}\n"
	writeTestFile(t, filepath.Join(workspace, ".gemini/settings.json"), []byte(geminiJSON), 0o640)
	codexTOML := "model = \"gpt-current\"\n\n[features]\nweb_search = true\n"
	writeTestFile(t, filepath.Join(workspace, ".codex/config.toml"), []byte(codexTOML), 0o600)
	writeTestFile(t, filepath.Join(workspace, "CLAUDE.md"), []byte("# User instructions\r\n\r\nKeep this exact prose."), 0o640)
	writeTestFile(t, filepath.Join(workspace, "AGENTS.md"), []byte("# Agent rules\n"), 0o644)
	writeTestFile(t, filepath.Join(workspace, "GEMINI.md"), []byte("# Gemini rules\n"), 0o644)

	inspection, err := Inspect(context.Background(), request)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(inspection.Hosts) != 3 {
		t.Fatalf("host count = %d, want 3", len(inspection.Hosts))
	}
	for _, host := range inspection.Hosts {
		if host.Level != LevelNativeSkillBestEffort || len(host.Files) != 3 {
			t.Fatalf("unexpected host inspection: %+v", host)
		}
	}

	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Changes) != 9 {
		t.Fatalf("change count = %d, want 9", len(plan.Changes))
	}
	for index, change := range plan.Changes {
		wantKind := []ChangeKind{ChangeMCP, ChangeNativeSkill, ChangeBootstrap}[index/3]
		if change.Kind != wantKind {
			t.Fatalf("change %d kind = %s, want %s", index, change.Kind, wantKind)
		}
		if change.PreimageDigest == "" {
			t.Fatalf("change %d has no preimage digest", index)
		}
	}

	if _, err := Apply(context.Background(), plan, ApplyOptions{}); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("Apply without confirmation error = %v", err)
	}
	applied, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(applied.Changed) != 9 {
		t.Fatalf("applied count = %d, want 9", len(applied.Changed))
	}

	assertJSONRegistration(t, filepath.Join(workspace, ".mcp.json"), binary, workspace, false)
	assertJSONRegistration(t, filepath.Join(workspace, ".gemini/settings.json"), binary, workspace, true)
	assertContains(t, filepath.Join(workspace, ".mcp.json"), `"unrelated": { "keep": true }`)
	assertContains(t, filepath.Join(workspace, ".gemini/settings.json"), `"theme": "user-owned"`)
	assertContains(t, filepath.Join(workspace, ".codex/config.toml"), "model = \"gpt-current\"")
	assertContains(t, filepath.Join(workspace, ".codex/config.toml"), "cwd = "+tomlString(workspace))
	assertContains(t, filepath.Join(workspace, ".codex/config.toml"), "command = "+tomlString(binary))

	for _, path := range []string{
		".claude/skills/system-curator/SKILL.md",
		".agents/skills/system-curator/SKILL.md",
		".gemini/skills/system-curator/SKILL.md",
	} {
		content := readTestFile(t, filepath.Join(workspace, path))
		if string(content) != systemskills.CuratorBundle().Instructions {
			t.Fatalf("%s does not contain exact bundled curator bytes", path)
		}
	}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		content := string(readTestFile(t, filepath.Join(workspace, name)))
		if strings.Count(content, bootstrapStart) != 1 || strings.Count(content, bootstrapEnd) != 1 {
			t.Fatalf("%s managed marker counts are invalid", name)
		}
	}
	claudeContent := readTestFile(t, filepath.Join(workspace, "CLAUDE.md"))
	if !strings.Contains(string(claudeContent), "# User instructions\r\n\r\nKeep this exact prose.\r\n\r\n") || strings.Contains(strings.ReplaceAll(string(claudeContent), "\r\n", ""), "\n") {
		t.Fatal("CLAUDE.md user bytes or CRLF style were not preserved")
	}
	assertMode(t, filepath.Join(workspace, ".mcp.json"), 0o600)
	assertMode(t, filepath.Join(workspace, "CLAUDE.md"), 0o640)

	secondPlan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	if len(secondPlan.Changes) != 0 {
		t.Fatalf("second plan has %d changes, want 0", len(secondPlan.Changes))
	}
	secondApply, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true})
	if err != nil {
		t.Fatalf("reapply original plan: %v", err)
	}
	if len(secondApply.Changed) != 0 {
		t.Fatalf("second apply changed %d files", len(secondApply.Changed))
	}
}

func TestManagedBlockConflictsAreReportedWithoutWrites(t *testing.T) {
	tests := map[string]string{
		"duplicate": bootstrapStart + "\na\n" + bootstrapEnd + "\n" + bootstrapStart + "\nb\n" + bootstrapEnd,
		"nested":    bootstrapStart + "\n" + bootstrapStart + "\n" + bootstrapEnd + "\n" + bootstrapEnd,
		"reversed":  bootstrapEnd + "\n" + bootstrapStart,
		"unmatched": bootstrapStart + "\nbody",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "CLAUDE.md")
			writeTestFile(t, path, []byte(content), 0o644)
			request := Request{Workspace: workspace, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
			inspection, err := Inspect(context.Background(), request)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if inspection.Hosts[0].Files[2].Conflict == "" {
				t.Fatal("expected bootstrap conflict")
			}
			if _, err := Plan(context.Background(), request); !errors.Is(err, ErrConflict) {
				t.Fatalf("Plan error = %v, want conflict", err)
			}
			if got := string(readTestFile(t, path)); got != content {
				t.Fatal("read-only operations changed instruction file")
			}
		})
	}
}

func TestInvalidHostConfigIsAnInspectableConflict(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".mcp.json")
	writeTestFile(t, path, []byte(`{"mcpServers":`), 0o600)
	request := Request{Workspace: workspace, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}

	inspection, err := Inspect(context.Background(), request)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if conflict := inspection.Hosts[0].Files[0].Conflict; !strings.Contains(conflict, "invalid JSON") {
		t.Fatalf("config conflict = %q", conflict)
	}
	if _, err := Plan(context.Background(), request); !errors.Is(err, ErrConflict) {
		t.Fatalf("Plan error = %v, want conflict", err)
	}
	if got := string(readTestFile(t, path)); got != `{"mcpServers":` {
		t.Fatalf("inspection changed invalid config: %q", got)
	}
}

func TestPlanRejectsStalePreimage(t *testing.T) {
	workspace := t.TempDir()
	request := Request{Workspace: workspace, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	writeTestFile(t, filepath.Join(workspace, ".mcp.json"), []byte(`{"user":true}`), 0o644)
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("Apply error = %v, want stale plan", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude/skills/system-curator/SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("apply wrote later dependency before stale preflight: %v", err)
	}
}

func TestRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, ".claude")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	request := Request{Workspace: workspace, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	if _, err := Plan(context.Background(), request); !errors.Is(err, ErrSymlink) {
		t.Fatalf("Plan error = %v, want symlink rejection", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside directory changed: entries=%v err=%v", entries, err)
	}
}

func TestApplyAnchorsFinalRenameAgainstParentSymlinkSwap(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	request := Request{Workspace: workspace, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	originalHook := atomicWriteBeforeRename
	defer func() { atomicWriteBeforeRename = originalHook }()
	swapped := false
	atomicWriteBeforeRename = func(relative string) error {
		if swapped || relative != filepath.FromSlash(".claude/skills/system-curator/SKILL.md") {
			return nil
		}
		swapped = true
		managedParent := filepath.Join(workspace, ".claude")
		if err := os.Rename(managedParent, filepath.Join(workspace, ".claude-original")); err != nil {
			return err
		}
		return os.Symlink(outside, managedParent)
	}

	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err == nil {
		t.Fatal("Apply succeeded after managed parent was replaced by an escaping symlink")
	}
	if !swapped {
		t.Fatal("final-rename fault seam was not exercised")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside directory changed: entries=%v err=%v", entries, err)
	}
}

func TestInputValidationAndCancellation(t *testing.T) {
	workspace := t.TempDir()
	if _, err := Inspect(context.Background(), Request{Workspace: "relative", Binary: "/bin/skillhub"}); err == nil {
		t.Fatal("relative workspace accepted")
	}
	if _, err := Inspect(context.Background(), Request{Workspace: workspace, Binary: "skillhub"}); err == nil {
		t.Fatal("relative binary accepted")
	}
	if _, err := Inspect(context.Background(), Request{Workspace: workspace, Binary: "/bin/skillhub", Hosts: []Host{"unknown"}}); err == nil {
		t.Fatal("unknown host accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Inspect(ctx, Request{Workspace: workspace, Binary: "/bin/skillhub"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Inspect error = %v", err)
	}
}

func TestCodexTOMLReplacesCompleteMultilineManagedValues(t *testing.T) {
	raw := []byte(`[mcp_servers.skillhub]
# managed values may use any valid TOML representation
command = """old
[not.a.real.table]
command value""" # trailing managed comment
args = [
  "old", # embedded array comment
  "values",
]
cwd = '''old
multiline cwd'''
enabled = false # old state
unmanaged = [
  "keep", # unrelated comment
]

[other]
value = "keep"
`)
	updated, err := upsertCodexTOML(raw, "/new/bin", "/new/work")
	if err != nil {
		t.Fatalf("upsertCodexTOML: %v", err)
	}
	text := string(updated)
	for _, want := range []string{
		`command = "/new/bin"`,
		`args = ["mcp", "serve", "--workspace", "/new/work"]`,
		`cwd = "/new/work"`,
		`enabled = true`,
		"unmanaged = [\n  \"keep\", # unrelated comment\n]",
		"[other]\nvalue = \"keep\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated TOML does not contain %q:\n%s", want, text)
		}
	}
	for _, removed := range []string{"[not.a.real.table]", "embedded array comment", "old state"} {
		if strings.Contains(text, removed) {
			t.Fatalf("managed multiline value was only partially replaced; found %q", removed)
		}
	}
}

func TestCodexTOMLRejectsUnsafeMultilineValue(t *testing.T) {
	raw := []byte("[mcp_servers.skillhub]\nargs = [\n  \"unterminated\"\n")
	if _, err := upsertCodexTOML(raw, "/new/bin", "/new/work"); err == nil || !strings.Contains(err.Error(), "cannot safely update") {
		t.Fatalf("upsert error = %v, want safe conflict", err)
	}
}

func TestPlanChangePreviewsAreBoundedAndExcludeUnmanagedValues(t *testing.T) {
	workspace := t.TempDir()
	secret := "UNRELATED-CONFIG-SECRET-MUST-NOT-APPEAR"
	writeTestFile(t, filepath.Join(workspace, ".mcp.json"), []byte(`{"unrelatedSecret":"`+secret+`","mcpServers":{"skillhub":{"type":"stdio","command":"OLD-MANAGED-SECRET","args":["old"]}}}`), 0o600)
	writeTestFile(t, filepath.Join(workspace, "CLAUDE.md"), []byte("user prose "+secret+"\n"), 0o644)
	plan, err := Plan(context.Background(), Request{Workspace: workspace, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Changes) != 3 {
		t.Fatalf("changes = %d, want 3", len(plan.Changes))
	}
	for _, change := range plan.Changes {
		if change.Preview == "" || len(change.Preview) > maxChangePreviewBytes {
			t.Fatalf("invalid %s preview length %d", change.Kind, len(change.Preview))
		}
		if strings.Contains(change.Preview, secret) || strings.Contains(change.Preview, "OLD-MANAGED-SECRET") {
			t.Fatalf("%s preview exposed config content: %q", change.Kind, change.Preview)
		}
		switch change.Kind {
		case ChangeMCP:
			for _, field := range []string{"type:", "command:", "args:"} {
				if !strings.Contains(change.Preview, field) {
					t.Fatalf("MCP preview lacks owned field %s: %q", field, change.Preview)
				}
			}
		case ChangeNativeSkill:
			if !strings.Contains(change.Preview, "native skill exact file:") || !strings.Contains(change.Preview, " bytes)") {
				t.Fatalf("native skill preview lacks digest/size: %q", change.Preview)
			}
		case ChangeBootstrap:
			if !strings.Contains(change.Preview, bootstrapStart) || strings.Contains(change.Preview, "user prose") {
				t.Fatalf("bootstrap preview is not managed-block-only: %q", change.Preview)
			}
		}
	}
}

func TestTOMLAndJSONUpdatersPreserveUnmanagedContentAndFinalNewline(t *testing.T) {
	jsonRaw := []byte("{\n  \"user\": [1, {\"nested\": true}],\n  \"mcpServers\": {\n    \"skillhub\": {\"command\":\"old\"},\n    \"other\": {\"command\": \"keep\"}\n  }\n}")
	jsonUpdated, err := upsertJSONPath(jsonRaw, []string{"mcpServers", "skillhub"}, []byte(`{"command":"/new"}`))
	if err != nil {
		t.Fatalf("upsert JSON: %v", err)
	}
	if jsonUpdated[len(jsonUpdated)-1] == '\n' || !strings.Contains(string(jsonUpdated), `"user": [1, {"nested": true}]`) || !strings.Contains(string(jsonUpdated), `"other": {"command": "keep"}`) {
		t.Fatal("JSON updater changed unmanaged bytes or final newline")
	}

	tomlRaw := []byte("model = \"keep\"\r\n\r\n[mcp_servers.skillhub]\r\ncommand = \"old\"\r\nargs = [\"old\"]\r\ncustom_timeout = 99\r\ncwd = \"old\"\r\nenabled = false\r\n\r\n[other]\r\nvalue = \"keep\"\r\n")
	tomlUpdated, err := upsertCodexTOML(tomlRaw, "/new/bin", "/new/work")
	if err != nil {
		t.Fatalf("upsert TOML: %v", err)
	}
	text := string(tomlUpdated)
	for _, preserved := range []string{"model = \"keep\"", "custom_timeout = 99", "[other]\r\nvalue = \"keep\""} {
		if !strings.Contains(text, preserved) {
			t.Fatalf("TOML lost unmanaged bytes %q", preserved)
		}
	}
	if !strings.HasSuffix(text, "\r\n") || strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Fatal("TOML updater did not preserve CRLF/final newline")
	}
}

func assertJSONRegistration(t *testing.T, path, binary, workspace string, gemini bool) {
	t.Helper()
	var root struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(readTestFile(t, path), &root); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	var skillhub struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		CWD     string   `json:"cwd"`
		Trust   *bool    `json:"trust"`
	}
	if err := json.Unmarshal(root.MCPServers["skillhub"], &skillhub); err != nil {
		t.Fatalf("decode %s skillhub registration: %v", path, err)
	}
	if skillhub.Command != binary {
		t.Fatalf("%s command = %v", path, skillhub.Command)
	}
	if len(skillhub.Args) != 4 || skillhub.Args[0] != "mcp" || skillhub.Args[1] != "serve" || skillhub.Args[2] != "--workspace" || skillhub.Args[3] != workspace {
		t.Fatalf("%s args = %v", path, skillhub.Args)
	}
	if gemini && (skillhub.Trust == nil || *skillhub.Trust || skillhub.CWD != workspace) {
		t.Fatalf("Gemini safety fields = %+v", skillhub)
	}
}

func writeTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func assertContains(t *testing.T, path, want string) {
	t.Helper()
	if !strings.Contains(string(readTestFile(t, path)), want) {
		t.Fatalf("%s does not contain %q", path, want)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func TestJSONUpdaterPreservesUnmanagedRegistrationKeys(t *testing.T) {
	raw := []byte("{\n  \"mcpServers\": {\n    \"skillhub\": {\"command\":\"old\", \"env\": {\"A\": \"1\"}, \"timeout\": 5000, \"includeTools\": [\"x\"], \"args\": [\"old\"]}\n  }\n}\n")
	updated, err := upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, []byte(`{"command":"/new","args":["mcp","serve"],"cwd":"/w"}`))
	if err != nil {
		t.Fatalf("upsert JSON: %v", err)
	}
	var doc struct {
		MCPServers map[string]map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(updated, &doc); err != nil {
		t.Fatalf("invalid JSON %q: %v", updated, err)
	}
	server := doc.MCPServers["skillhub"]
	want := map[string]string{"command": `"/new"`, "args": `["mcp","serve"]`, "cwd": `"/w"`, "env": `{"A": "1"}`, "timeout": `5000`, "includeTools": `["x"]`}
	for key, value := range want {
		if string(server[key]) != value {
			t.Fatalf("%s = %s, want %s", key, server[key], value)
		}
	}
}

func TestJSONUpdaterConflictNamesFullKeyPath(t *testing.T) {
	_, err := upsertJSONPath([]byte(`{"mcpServers": null}`), []string{"mcpServers", "skillhub"}, []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), `"mcpServers"`) {
		t.Fatalf("error = %v, want message naming mcpServers", err)
	}
}

func TestTOMLUpdaterRejectsAlternateSkillhubDefinitions(t *testing.T) {
	cases := map[string]string{
		"inline table under mcp_servers table": "[mcp_servers]\nskillhub = { command = \"x\" }\n",
		"top-level dotted inline table":        "mcp_servers.skillhub = { command = \"x\" }\n",
		"dotted key under mcp_servers table":   "[mcp_servers]\nskillhub.command = \"x\"\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if out, err := upsertCodexTOML([]byte(raw), "/bin/skillhub", "/work"); err == nil {
				t.Fatalf("expected conflict, got output %q", out)
			}
		})
	}
}
