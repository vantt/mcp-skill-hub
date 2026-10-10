package hostintegration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func claudeLocal(root string) string { return filepath.Join(root, ".claude", "settings.local.json") }

func applyAll(t *testing.T, request Request) {
	t.Helper()
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func jsonStrings(t *testing.T, path string, keys ...string) []string {
	t.Helper()
	got, err := jsonStringArrayAt(readTestFile(t, path), keys)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return got
}

func assertHasAll(t *testing.T, label string, got, want []string) {
	t.Helper()
	for _, entry := range want {
		found := false
		for _, have := range got {
			if have == entry {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s = %v, missing %s", label, got, entry)
		}
	}
}

func TestRuntimeDirsAreAddedForEveryHostWithoutClobberingUserEntries(t *testing.T) {
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		t.Run(string(scope), func(t *testing.T) {
			workspace := t.TempDir()
			root := t.TempDir()
			binary := filepath.Join(workspace, "bin", "skillhub")
			request := Request{Workspace: workspace, Root: root, Scope: scope, Binary: binary}
			dirs := RuntimeAccessDirs(workspace)

			claudeSettings := filepath.Join(root, ".claude", "settings.local.json")
			if scope == ScopeUser {
				claudeSettings = filepath.Join(root, ".claude", "settings.json")
			}
			writeTestFile(t, claudeSettings, []byte("{\n  \"model\": \"keep\",\n  \"permissions\": {\"allow\": [\"Bash(ls)\"], \"additionalDirectories\": [\"/user/dir\"]}\n}\n"), 0o600)
			geminiPath := filepath.Join(root, ".gemini", "settings.json")
			writeTestFile(t, geminiPath, []byte("{\"theme\": \"x\", \"context\": {\"fileName\": \"G.md\", \"includeDirectories\": [\"/user/gem\"]}}\n"), 0o600)
			codexPath := filepath.Join(root, ".codex", "config.toml")
			writeTestFile(t, codexPath, []byte("model = \"m\"\n\n[sandbox_workspace_write]\n# keep this comment\nnetwork_access = true\nwritable_roots = [\"/user/root\", '/lit/root']\n\n[features]\nweb_search = true\n"), 0o600)

			applyAll(t, request)

			claude := jsonStrings(t, claudeSettings, "permissions", "additionalDirectories")
			assertHasAll(t, "claude additionalDirectories", claude, append([]string{"/user/dir"}, dirs...))
			assertContains(t, claudeSettings, `"model": "keep"`)
			gemini := jsonStrings(t, geminiPath, "context", "includeDirectories")
			assertHasAll(t, "gemini includeDirectories", gemini, append([]string{"/user/gem"}, dirs...))
			assertContains(t, geminiPath, `"fileName": "G.md"`)
			roots, err := codexWritableRoots(readTestFile(t, codexPath))
			if err != nil {
				t.Fatalf("codex roots: %v", err)
			}
			assertHasAll(t, "codex writable_roots", roots, append([]string{"/user/root", "/lit/root"}, dirs...))
			assertContains(t, codexPath, "# keep this comment")
			assertContains(t, codexPath, "network_access = true")
			assertContains(t, codexPath, "web_search = true")
			if got := strings.Count(string(readTestFile(t, codexPath)), "[sandbox_workspace_write]"); got != 1 {
				t.Fatalf("sandbox_workspace_write tables = %d, want 1", got)
			}
		})
	}
}

func TestRuntimeDirsAreCreatedInEmptyConfigsAndAreIdempotent(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	request := Request{Workspace: workspace, Root: root, Binary: filepath.Join(workspace, "bin", "skillhub")}
	applyAll(t, request)
	dirs := RuntimeAccessDirs(workspace)

	assertHasAll(t, "claude", jsonStrings(t, claudeLocal(root), "permissions", "additionalDirectories"), dirs)
	for key, want := range map[string][]string{
		"allow": {"Bash(skillhub:*)"},
		"ask":   {"Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)"},
		"deny":  {"Bash(skillhub * --approve-content*)"},
	} {
		if got := jsonStrings(t, claudeLocal(root), "permissions", key); !slices.Equal(got, want) {
			t.Fatalf("fresh Claude %s = %v, want %v", key, got, want)
		}
	}
	assertHasAll(t, "gemini", jsonStrings(t, filepath.Join(root, ".gemini", "settings.json"), "context", "includeDirectories"), dirs)
	roots, err := codexWritableRoots(readTestFile(t, filepath.Join(root, ".codex", "config.toml")))
	if err != nil {
		t.Fatal(err)
	}
	assertHasAll(t, "codex", roots, dirs)

	again, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Changes) != 0 {
		t.Fatalf("second plan has %d changes, want 0", len(again.Changes))
	}
	if len(jsonStrings(t, claudeLocal(root), "permissions", "additionalDirectories")) != len(dirs) {
		t.Fatal("directories were duplicated")
	}
}

func TestInspectDetectsMissingRuntimeDirsAndFixAddsThem(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	request := Request{Workspace: workspace, Root: root, Binary: filepath.Join(workspace, "bin", "skillhub")}
	applyAll(t, request)

	// A connected host that lost the entries is reported as not current.
	writeTestFile(t, claudeLocal(root), []byte("{\"permissions\": {\"additionalDirectories\": []}}\n"), 0o600)
	inspection, err := Inspect(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	stale := 0
	for _, host := range inspection.Hosts {
		for _, file := range host.Files {
			if !file.Current {
				stale++
				if file.Kind != ChangeHostPermissions || host.Host != HostClaude {
					t.Fatalf("unexpected stale file %+v", file)
				}
			}
		}
	}
	if stale != 1 {
		t.Fatalf("stale files = %d, want 1", stale)
	}

	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Kind != ChangeHostPermissions {
		t.Fatalf("plan = %+v, want one permissions change", plan.Changes)
	}
	applyAll(t, request)
	assertHasAll(t, "claude", jsonStrings(t, claudeLocal(root), "permissions", "additionalDirectories"), RuntimeAccessDirs(workspace))

	// Dropping a Codex entry is detected through the shared config file.
	codexPath := filepath.Join(root, ".codex", "config.toml")
	raw := string(readTestFile(t, codexPath))
	writeTestFile(t, codexPath, []byte(strings.Replace(raw, tomlString(RuntimeAccessDirs(workspace)[2]), `"/elsewhere"`, 1)), 0o600)
	plan, err = Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Host != HostCodex || plan.Changes[0].Kind != ChangeMCP {
		t.Fatalf("plan = %+v, want one codex config change", plan.Changes)
	}
	applyAll(t, request)
	roots, _ := codexWritableRoots(readTestFile(t, codexPath))
	assertHasAll(t, "codex", roots, append([]string{"/elsewhere"}, RuntimeAccessDirs(workspace)...))
}

func TestRuntimeDirsRejectUnsafeShapesAsConflicts(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	request := Request{Workspace: workspace, Root: root, Hosts: []Host{HostClaude}, Binary: filepath.Join(workspace, "bin", "skillhub")}
	writeTestFile(t, claudeLocal(root), []byte(`{"permissions": {"additionalDirectories": "nope"}}`), 0o600)
	if _, err := Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "array of strings") {
		t.Fatalf("Plan error = %v, want array-of-strings conflict", err)
	}
	if _, err := ensureCodexWritableRoots([]byte("sandbox_workspace_write = { writable_roots = [] }\n"), []string{"/a"}); err == nil {
		t.Fatal("inline table definition must be rejected")
	}
	if _, err := ensureCodexWritableRoots([]byte("[sandbox_workspace_write]\nwritable_roots = [1]\n"), []string{"/a"}); err == nil {
		t.Fatal("non-string entries must be rejected")
	}
}

func TestEnsureJSONStringArrayKeepsFileWhenComplete(t *testing.T) {
	raw := []byte("{\n  \"permissions\": { \"additionalDirectories\": [\"/a/\"] }\n}\n")
	got, err := ensureJSONStringArray(raw, claudeAllowedDirsPath, []string{"/a"})
	if err != nil || string(got) != string(raw) {
		t.Fatalf("got %q, %v; want unchanged", got, err)
	}
	updated, err := ensureJSONStringArray(raw, claudeAllowedDirsPath, []string{"/a", "/b"})
	if err != nil || !json.Valid(updated) {
		t.Fatalf("updated %q, %v", updated, err)
	}
}

func TestSharedProjectSettingsSatisfyCheckButFixWritesLocalFile(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	request := Request{Workspace: workspace, Root: root, Hosts: []Host{HostClaude}, Binary: filepath.Join(workspace, "bin", "skillhub")}
	shared := filepath.Join(root, ".claude", "settings.json")
	encoded, _ := json.Marshal(map[string]any{"permissions": map[string]any{"additionalDirectories": RuntimeAccessDirs(workspace)}})
	writeTestFile(t, shared, encoded, 0o600)

	applyAll(t, request)
	if dirs := jsonStrings(t, claudeLocal(root), "permissions", "additionalDirectories"); len(dirs) != 0 {
		t.Fatalf("shared directory allowances duplicated locally: %v", dirs)
	}

	writeTestFile(t, shared, []byte("{}\n"), 0o600)
	applyAll(t, request)
	assertHasAll(t, "local", jsonStrings(t, claudeLocal(root), "permissions", "additionalDirectories"), RuntimeAccessDirs(workspace))
	if got := string(readTestFile(t, shared)); got != "{}\n" {
		t.Fatalf("shared settings were modified: %q", got)
	}
}
