package hostintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testHubURL = "http://127.0.0.1:7420"

func fileTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			relative, _ := filepath.Rel(root, path)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			tree[filepath.ToSlash(relative)] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func planAndApply(t *testing.T, request Request) PlanResult {
	t.Helper()
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	again, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("plan again: %v", err)
	}
	if len(again.Changes) != 0 {
		t.Fatalf("a second plan still has %d changes: %+v", len(again.Changes), again.Changes)
	}
	return plan
}

func seedUnrelatedHostFiles(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		".mcp.json":                   "{\n  \"mcpServers\": {\n    \"other\": {\n      \"command\": \"other-server\"\n    }\n  }\n}\n",
		"CLAUDE.md":                   "# Project notes\n\nKeep tests fast.\n",
		".codex/config.toml":          "model = \"gpt-5\"\n\n[mcp_servers.other]\ncommand = \"other-server\"\n",
		".gemini/settings.json":       "{\"theme\": \"dark\"}\n",
		".claude/settings.local.json": "{\n  \"permissions\": {\n    \"allow\": [\"Bash(ls:*)\"]\n  }\n}\n",
		"AGENTS.md":                   "Use tabs.",
	}
	for relative, content := range files {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(relative)), []byte(content), 0o644)
	}
}

// absoluteBinary is the skillhub path written into entries; it need not exist.
func absoluteBinary(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "bin", "skillhub")
}

func remoteAccessDir(t *testing.T) string {
	t.Helper()
	dir, err := RemoteAccessDir(testHubURL)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRemotePlanWritesTheProxyEntryForEveryHost(t *testing.T) {
	root, binary := t.TempDir(), absoluteBinary(t)
	seedUnrelatedHostFiles(t, root)
	tokenFile := filepath.Join(t.TempDir(), "hub-token")
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	writeTestFile(t, tokenFile, []byte(token+"\n"), 0o600)
	hub := &RemoteHub{URL: testHubURL, TokenFile: tokenFile}
	plan := planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Hub: hub})
	for _, change := range plan.Changes {
		if change.Kind == ChangeNativeSkill {
			t.Fatalf("a remote hub must not install the curator skill: %+v", change)
		}
	}
	wantArgs := []string{"mcp", "proxy", "--hub", testHubURL, "--token-file", tokenFile}
	access := remoteAccessDir(t)

	var claude struct {
		Servers map[string]claudeMCPServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(readTestFile(t, filepath.Join(root, ".mcp.json")), &claude); err != nil {
		t.Fatal(err)
	}
	if got := claude.Servers["skillhub"]; got.Type != "stdio" || got.Command != binary || !slices.Equal(got.Args, wantArgs) {
		t.Fatalf("Claude entry = %+v", got)
	}
	if _, ok := claude.Servers["other"]; !ok || len(claude.Servers) != 2 {
		t.Fatalf("Claude servers = %v", claude.Servers)
	}

	var gemini map[string]json.RawMessage
	geminiRaw := readTestFile(t, filepath.Join(root, ".gemini", "settings.json"))
	if err := json.Unmarshal(geminiRaw, &gemini); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(geminiRaw, []byte(`"cwd"`)) || bytes.Contains(geminiRaw, []byte("skillhub-curation")) {
		t.Fatalf("Gemini remote entry has a working directory or a curation server: %s", geminiRaw)
	}
	if dirs, _ := jsonStringArrayAt(geminiRaw, geminiAllowedDirsPath); !slices.Equal(dirs, []string{access}) {
		t.Fatalf("Gemini included directories = %v, want %s", dirs, access)
	}

	codexRaw := readTestFile(t, filepath.Join(root, ".codex", "config.toml"))
	registration := ReadRegistration(HostCodex, codexRaw)
	if !registration.Remote() || registration.HubURL != testHubURL || registration.TokenFile != tokenFile {
		t.Fatalf("Codex registration = %+v\n%s", registration, codexRaw)
	}
	if bytes.Contains(codexRaw, []byte("cwd =")) || !bytes.Contains(codexRaw, []byte("enabled = true")) {
		t.Fatalf("Codex remote table:\n%s", codexRaw)
	}
	if roots, _ := codexWritableRoots(codexRaw); !slices.Equal(roots, []string{access}) {
		t.Fatalf("Codex writable roots = %v", roots)
	}

	permissions := readTestFile(t, filepath.Join(root, ".claude", "settings.local.json"))
	if dirs, _ := jsonStringArrayAt(permissions, claudeAllowedDirsPath); !slices.Equal(dirs, []string{access}) {
		t.Fatalf("Claude additional directories = %v", dirs)
	}
	for _, rule := range claudeCLIRules {
		values, _ := jsonStringArrayAt(permissions, []string{"permissions", rule.key})
		for _, value := range rule.values {
			if !slices.Contains(values, value) {
				t.Fatalf("Claude permission %s lacks %s: %s", rule.key, value, permissions)
			}
		}
	}

	for _, name := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		assertContains(t, filepath.Join(root, name), bootstrapStart)
	}
	for relative, content := range fileTree(t, root) {
		if strings.Contains(content, token) {
			t.Fatalf("%s holds the token", relative)
		}
		if strings.HasSuffix(relative, "system-curator/SKILL.md") {
			t.Fatalf("curator skill installed for a remote hub: %s", relative)
		}
	}
}

func TestRemoteEntryWithoutTokenFileRelysOnTheEnvironment(t *testing.T) {
	root, binary := t.TempDir(), absoluteBinary(t)
	planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Hosts: []Host{HostClaude}, Hub: &RemoteHub{URL: testHubURL}})
	registration := ReadRegistration(HostClaude, readTestFile(t, filepath.Join(root, ".mcp.json")))
	if registration.HubURL != testHubURL || registration.TokenFile != "" {
		t.Fatalf("registration = %+v", registration)
	}
}

func TestSwitchingAScopeBetweenLocalAndRemoteKeepsOneEntry(t *testing.T) {
	workspace, root, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	local := Request{Workspace: workspace, Root: root, Scope: ScopeProject, Binary: binary}
	remote := Request{Root: root, Scope: ScopeProject, Binary: binary, Hub: &RemoteHub{URL: testHubURL}}
	access := remoteAccessDir(t)
	workspaceDirs := RuntimeAccessDirs(workspace)

	planAndApply(t, local)
	planAndApply(t, remote)
	for _, adapter := range adapters {
		raw := readTestFile(t, filepath.Join(root, filepath.FromSlash(adapter.ConfigRelativePath)))
		if registration := ReadRegistration(adapter.Host, raw); registration.HubURL != testHubURL || registration.Workspace != "" {
			t.Fatalf("%s after switching to the remote hub: %+v", adapter.Host, registration)
		}
		// A Windows path appears in JSON and TOML with each backslash escaped.
		if bytes.Contains(raw, []byte(workspace)) || bytes.Contains(raw, []byte(strings.ReplaceAll(workspace, `\`, `\\`))) {
			t.Fatalf("%s config still names the local workspace:\n%s", adapter.Host, raw)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(adapter.SkillRelativePath))); !os.IsNotExist(err) {
			t.Fatalf("%s curator skill kept for a remote hub: %v", adapter.Host, err)
		}
	}
	permissions := readTestFile(t, filepath.Join(root, ".claude", "settings.local.json"))
	if dirs, _ := jsonStringArrayAt(permissions, claudeAllowedDirsPath); !slices.Equal(dirs, []string{access}) {
		t.Fatalf("Claude directories after switching = %v", dirs)
	}

	planAndApply(t, local)
	for _, adapter := range adapters {
		raw := readTestFile(t, filepath.Join(root, filepath.FromSlash(adapter.ConfigRelativePath)))
		if registration := ReadRegistration(adapter.Host, raw); registration.Remote() || registration.Workspace != workspace {
			t.Fatalf("%s after switching back: %+v", adapter.Host, registration)
		}
		if bytes.Contains(raw, []byte(access)) {
			t.Fatalf("%s config still allows the remote cache:\n%s", adapter.Host, raw)
		}
		if !bytes.Equal(readTestFile(t, filepath.Join(root, filepath.FromSlash(adapter.SkillRelativePath))), []byte(systemCuratorInstructions())) {
			t.Fatalf("%s curator skill not restored", adapter.Host)
		}
	}
	permissions = readTestFile(t, filepath.Join(root, ".claude", "settings.local.json"))
	if dirs, _ := jsonStringArrayAt(permissions, claudeAllowedDirsPath); !slices.Equal(dirs, workspaceDirs) {
		t.Fatalf("Claude directories after switching back = %v", dirs)
	}
}

func TestRemoveReturnsTheScopeToItsStateBeforeConnecting(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		for _, kind := range []string{"local", "remote"} {
			name := kind + map[bool]string{false: " in an empty project", true: " with unrelated host files"}[seeded]
			t.Run(name, func(t *testing.T) {
				workspace, root, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
				if seeded {
					seedUnrelatedHostFiles(t, root)
				}
				before := fileTree(t, root)
				connect := Request{Workspace: workspace, Root: root, Scope: ScopeProject, Binary: binary}
				if kind == "remote" {
					connect = Request{Root: root, Scope: ScopeProject, Binary: binary, Hub: &RemoteHub{URL: testHubURL, TokenFile: filepath.Join(root, "hub-token")}}
				}
				planAndApply(t, connect)
				if maps.Equal(before, fileTree(t, root)) {
					t.Fatal("connecting changed nothing")
				}
				planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Remove: true})
				after := fileTree(t, root)
				if !maps.Equal(before, after) {
					for path := range maps.Keys(after) {
						if before[path] != after[path] {
							t.Errorf("%s after removal:\n%q\nbefore:\n%q", path, after[path], before[path])
						}
					}
					for path := range maps.Keys(before) {
						if _, ok := after[path]; !ok {
							t.Errorf("%s was deleted", path)
						}
					}
				}
				for _, directory := range []string{".claude/skills/system-curator", ".agents/skills/system-curator", ".gemini/skills/system-curator"} {
					if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(directory))); !os.IsNotExist(err) {
						t.Errorf("empty curator directory %s left behind: %v", directory, err)
					}
				}
				if !seeded {
					if left, _ := os.ReadDir(root); len(left) != 0 {
						t.Errorf("an empty project keeps %d entries after removal, first %s", len(left), left[0].Name())
					}
				}
			})
		}
	}
}

func TestRemoveInTheUserScopeKeepsUnrelatedClaudeState(t *testing.T) {
	home, binary := t.TempDir(), absoluteBinary(t)
	original := []byte("{\n  \"numStartups\": 4,\n  \"projects\": {}\n}\n")
	writeTestFile(t, filepath.Join(home, ".claude.json"), original, 0o600)
	planAndApply(t, Request{Root: home, Scope: ScopeUser, Binary: binary, Hosts: []Host{HostClaude}, Hub: &RemoteHub{URL: testHubURL}})
	planAndApply(t, Request{Root: home, Scope: ScopeUser, Binary: binary, Hosts: []Host{HostClaude}, Remove: true})
	if got := readTestFile(t, filepath.Join(home, ".claude.json")); !bytes.Equal(got, original) {
		t.Fatalf(".claude.json after removal:\n%s", got)
	}
	assertMode(t, filepath.Join(home, ".claude.json"), 0o600)
}

func TestUserScopeRoundTripForEveryHost(t *testing.T) {
	home, binary := t.TempDir(), absoluteBinary(t)
	writeTestFile(t, filepath.Join(home, ".claude.json"), []byte("{\"numStartups\": 4}\n"), 0o600)
	writeTestFile(t, filepath.Join(home, ".codex", "config.toml"), []byte("model = \"gpt-5\"\n"), 0o644)
	writeTestFile(t, filepath.Join(home, ".gemini", "settings.json"), []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o644)
	writeTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), []byte("# Me\n"), 0o644)
	before := fileTree(t, home)
	planAndApply(t, Request{Workspace: t.TempDir(), Root: home, Scope: ScopeUser, Binary: binary})
	planAndApply(t, Request{Root: home, Scope: ScopeUser, Binary: binary, Hub: &RemoteHub{URL: testHubURL}})
	planAndApply(t, Request{Root: home, Scope: ScopeUser, Binary: binary, Remove: true})
	if after := fileTree(t, home); !maps.Equal(before, after) {
		t.Fatalf("user scope after local, remote and removal:\n%v\nbefore:\n%v", after, before)
	}
}

func TestRemoveLeavesAChangedCuratorSkillAndUnrelatedPermissions(t *testing.T) {
	workspace, root, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	planAndApply(t, Request{Workspace: workspace, Root: root, Scope: ScopeProject, Binary: binary, Hosts: []Host{HostClaude}})
	skill := filepath.Join(root, ".claude", "skills", "system-curator", "SKILL.md")
	writeTestFile(t, skill, []byte("my own curator notes\n"), 0o644)
	settings := filepath.Join(root, ".claude", "settings.local.json")
	raw := readTestFile(t, settings)
	updated, err := ensureJSONStringArray(raw, []string{"permissions", "allow"}, []string{"Bash(git:*)"})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, settings, updated, 0o644)

	planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Hosts: []Host{HostClaude}, Remove: true})
	if got := readTestFile(t, skill); string(got) != "my own curator notes\n" {
		t.Fatalf("changed curator skill = %q", got)
	}
	allow, _ := jsonStringArrayAt(readTestFile(t, settings), []string{"permissions", "allow"})
	if !slices.Equal(allow, []string{"Bash(git:*)"}) {
		t.Fatalf("allow after removal = %v", allow)
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf(".mcp.json holding only the Skill Hub entry was not deleted: %v", err)
	}
}

func TestRemoveCleansUpDirectoriesOfAnEntryThatIsGone(t *testing.T) {
	cases := map[string]func(t *testing.T, root, binary string){
		"remote entry deleted by hand": func(t *testing.T, root, binary string) {
			planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Hub: &RemoteHub{URL: testHubURL}})
			for _, adapter := range adapters {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(adapter.ConfigRelativePath))); err != nil {
					t.Fatal(err)
				}
			}
		},
		"state directory changed since connect": func(t *testing.T, root, binary string) {
			planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Hub: &RemoteHub{URL: testHubURL}})
			t.Setenv("XDG_STATE_HOME", t.TempDir())
		},
		"local entry deleted by hand": func(t *testing.T, root, binary string) {
			workspace := t.TempDir()
			writeTestFile(t, filepath.Join(workspace, ".skillhub", "schema-version"), []byte("3\n"), 0o644)
			planAndApply(t, Request{Workspace: workspace, Root: root, Scope: ScopeProject, Binary: binary, Hosts: []Host{HostClaude}})
			if err := os.Remove(filepath.Join(root, ".mcp.json")); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root, binary := t.TempDir(), absoluteBinary(t)
			setup(t, root, binary)
			planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Remove: true})
			if left := fileTree(t, root); len(left) != 0 {
				t.Fatalf("files left after removal: %v", left)
			}
		})
	}
	// A directory that only looks like a runtime directory stays.
	root, binary := t.TempDir(), absoluteBinary(t)
	userDirs := []byte(`{"permissions":{"additionalDirectories":["/data/runtime/config","/x/skillhub/remote/notes"]}}`)
	writeTestFile(t, filepath.Join(root, ".claude", "settings.local.json"), userDirs, 0o644)
	planAndApply(t, Request{Root: root, Scope: ScopeProject, Binary: binary, Hosts: []Host{HostClaude}, Remove: true})
	if got := readTestFile(t, filepath.Join(root, ".claude", "settings.local.json")); !bytes.Equal(got, userDirs) {
		t.Fatalf("user directories changed: %s", got)
	}
}

func TestRemovePlanIsEmptyWithoutAConnection(t *testing.T) {
	root, binary := t.TempDir(), absoluteBinary(t)
	seedUnrelatedHostFiles(t, root)
	plan, err := Plan(context.Background(), Request{Root: root, Scope: ScopeProject, Binary: binary, Remove: true})
	if err != nil || len(plan.Changes) != 0 {
		t.Fatalf("removal plan without a connection: %v %+v", err, plan.Changes)
	}
}

func TestRemoteAndRemoveRequestsAreValidated(t *testing.T) {
	root, binary := t.TempDir(), absoluteBinary(t)
	for name, request := range map[string]Request{
		"remote and remove":    {Root: root, Binary: binary, Hub: &RemoteHub{URL: testHubURL}, Remove: true},
		"remote without root":  {Binary: binary, Hub: &RemoteHub{URL: testHubURL}},
		"remote with path":     {Root: root, Binary: binary, Workspace: root, Hub: &RemoteHub{URL: testHubURL}},
		"bad URL":              {Root: root, Binary: binary, Hub: &RemoteHub{URL: "ftp://example"}},
		"relative token":       {Root: root, Binary: binary, Hub: &RemoteHub{URL: testHubURL, TokenFile: "hub-token"}},
		"removal without root": {Binary: binary, Remove: true},
	} {
		if _, err := Plan(context.Background(), request); err == nil {
			t.Errorf("%s: plan accepted", name)
		}
	}
}

func TestReadRegistrationTellsLocalAndRemoteEntriesApart(t *testing.T) {
	local := []byte(`{"mcpServers":{"skillhub":{"args":["mcp","serve","--workspace","/w"]}}}`)
	remote := []byte(`{"mcpServers":{"skillhub":{"args":["mcp","proxy","--hub","http://h:1","--token-file","/t"]}}}`)
	if got := ReadRegistration(HostClaude, local); !got.Present || got.Remote() || got.Workspace != "/w" {
		t.Fatalf("local = %+v", got)
	}
	if got := ReadRegistration(HostClaude, remote); !got.Remote() || got.HubURL != "http://h:1" || got.TokenFile != "/t" {
		t.Fatalf("remote = %+v", got)
	}
	if workspace, ok := RegisteredWorkspace(HostClaude, remote); !ok || workspace != "" {
		t.Fatalf("RegisteredWorkspace on a remote entry = %q, %v", workspace, ok)
	}
	if got := ReadRegistration(HostCodex, []byte("[mcp_servers.skillhub]\nargs = [\"mcp\", \"proxy\", \"--hub=http://h:2\"]\n")); got.HubURL != "http://h:2" {
		t.Fatalf("codex remote = %+v", got)
	}
}

func TestRemoveBootstrapUndoesAppend(t *testing.T) {
	for _, original := range []string{"", "notes\n", "notes", "a\r\nb\r\n", "# t\n\n\n"} {
		appended := appendBootstrap([]byte(original))
		removed, conflict := removeBootstrap(appended)
		want := strings.TrimRight(original, "\r\n")
		if strings.HasSuffix(original, "\n") && want != "" {
			want += detectNewline([]byte(original))
		}
		if conflict != "" || string(removed) != want {
			t.Errorf("remove(append(%q)) = %q, %q", original, removed, conflict)
		}
	}
	middle := []byte("top\n\n" + string(bootstrapBlock("\n")) + "\n\nbottom\n")
	if removed, _ := removeBootstrap(middle); string(removed) != "top\n\nbottom\n" {
		t.Fatalf("middle block removal = %q", removed)
	}
	if _, conflict := removeBootstrap([]byte(bootstrapStart + "\n")); conflict == "" {
		t.Fatal("unmatched marker accepted")
	}
}
