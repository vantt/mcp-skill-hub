package cli

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
)

// startStoppableListener runs `skillhub serve --addr 127.0.0.1:0` in
// process and returns the hub URL it printed and a function that stops it.
// The listener also stops when the test ends.
func startStoppableListener(t *testing.T, ws string) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr syncBuffer
	exitCh := make(chan int, 1)
	go func() {
		exitCh <- RunContext(ctx, []string{"serve", "mcp", "--addr", "127.0.0.1:0", "--workspace", ws}, &stdout, &stderr)
	}()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case <-exitCh:
		case <-time.After(15 * time.Second):
			t.Error("listener did not stop")
		}
	}
	t.Cleanup(stop)
	const marker = "skillhub connect --hub "
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if index := strings.Index(stdout.String(), marker); index != -1 {
			rest := stdout.String()[index+len(marker):]
			if end := strings.IndexByte(rest, ' '); end != -1 {
				return rest[:end], stop
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no connect line; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	return "", stop
}

// copyHubToken copies the hub's token file the way a user brings it to
// machine A, and returns the copy's path and the token.
func copyHubToken(t *testing.T, ws string) (string, string) {
	t.Helper()
	token, err := hubtoken.Read(hubtoken.DefaultPath(ws))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hub-token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, token
}

// isolateClient gives the test its own home and state directory, as for a
// fresh machine A.
func isolateClient(t *testing.T) (home, state string) {
	t.Helper()
	home, state = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("SKILLHUB_WORKSPACE", "")
	t.Setenv(hubTokenEnv, "")
	return home, state
}

// containsPath reports whether a host file names path, either as it is or,
// in JSON and TOML strings, with each Windows backslash escaped.
func containsPath(text, path string) bool {
	return strings.Contains(text, path) || strings.Contains(text, strings.ReplaceAll(path, `\`, `\\`))
}

// flatText joins wrapped output into one line of single-spaced words.
func flatText(text string) string { return strings.Join(strings.Fields(text), " ") }

func treeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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

func TestConnectHubWritesEachHostsEntryAndNeverTheToken(t *testing.T) {
	home, state := isolateClient(t)
	ws := initTestWorkspace(t)
	hub, _ := startStoppableListener(t, ws)
	tokenFile, token := copyHubToken(t, ws)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"mcp", "proxy", "--hub", hub, "--token-file", tokenFile}
	cacheDir, err := hostintegration.RemoteAccessDir(hub)
	if err != nil || !strings.HasPrefix(cacheDir, state) {
		t.Fatalf("cache directory %q, %v", cacheDir, err)
	}
	var outputs strings.Builder
	files := map[string][]string{
		"claude": {".claude/settings.local.json", ".mcp.json", "CLAUDE.md"},
		"codex":  {".codex/config.toml", "AGENTS.md"},
		"gemini": {".gemini/settings.json", "GEMINI.md"},
	}
	for _, host := range []string{"claude", "codex", "gemini"} {
		project := t.TempDir()
		code, stdout, stderr := runConnectCLI(t, project, "--hub", hub, "--token-file", tokenFile, "--host", host)
		outputs.WriteString(stdout + stderr)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s%s", host, code, stdout, stderr)
		}
		tree := treeOf(t, project)
		if got := slices.Sorted(maps.Keys(tree)); !slices.Equal(got, files[host]) {
			t.Fatalf("%s wrote %v, want %v", host, got, files[host])
		}
		parsed, _ := parseHost(host)
		adapter := hostintegration.SupportedAdapters()[slices.IndexFunc(hostintegration.SupportedAdapters(), func(a hostintegration.Adapter) bool { return a.Host == parsed })]
		config := tree[adapter.ConfigRelativePath]
		registration := hostintegration.ReadRegistration(parsed, []byte(config))
		if registration.HubURL != hub || registration.TokenFile != tokenFile || registration.Workspace != "" {
			t.Fatalf("%s registration = %+v", host, registration)
		}
		if !containsPath(config, binary) || strings.Contains(config, "skillhub-curation") || strings.Contains(config, `"cwd"`) || strings.Contains(config, "cwd =") {
			t.Fatalf("%s config:\n%s", host, config)
		}
		if host != "claude" && !containsPath(config, cacheDir) {
			t.Fatalf("%s config does not allow the skill cache %s:\n%s", host, cacheDir, config)
		}
		if host == "claude" {
			var entry struct {
				Servers map[string]struct {
					Type    string   `json:"type"`
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal([]byte(config), &entry); err != nil {
				t.Fatal(err)
			}
			if len(entry.Servers) != 1 || entry.Servers["skillhub"].Type != "stdio" || !slices.Equal(entry.Servers["skillhub"].Args, wantArgs) {
				t.Fatalf("Claude entry: %s", config)
			}
			if !containsPath(tree[".claude/settings.local.json"], cacheDir) || !strings.Contains(tree[".claude/settings.local.json"], "Bash(skillhub:*)") {
				t.Fatalf("Claude permissions: %s", tree[".claude/settings.local.json"])
			}
		}
		for path, content := range tree {
			if strings.Contains(content, token) {
				t.Fatalf("%s holds the token", path)
			}
		}
	}

	code, stdout, stderr := runCLI(t, "connect", "-g", "--hub", hub, "--token-file", tokenFile, "--json")
	outputs.WriteString(stdout + stderr)
	if code != 0 {
		t.Fatalf("global: exit %d: %s%s", code, stdout, stderr)
	}
	var response struct {
		app.Result
		Remote app.RemoteConnection `json:"remote"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != app.StatusApplied || response.Remote.Hub != hub || response.Remote.Check.Outcome != app.ProbeAccepted || response.Remote.CacheDir != cacheDir {
		t.Fatalf("global JSON: %s", stdout)
	}
	want := []string{".claude.json", ".claude/CLAUDE.md", ".claude/settings.json", ".codex/AGENTS.md", ".codex/config.toml", ".gemini/GEMINI.md", ".gemini/settings.json"}
	homeTree := treeOf(t, home)
	if got := slices.Sorted(maps.Keys(homeTree)); !slices.Equal(got, want) {
		t.Fatalf("global connect wrote %v, want %v", got, want)
	}
	for path, content := range homeTree {
		if strings.Contains(content, token) {
			t.Fatalf("%s holds the token", path)
		}
	}
	if strings.Contains(outputs.String(), token) {
		t.Fatal("connect printed the token")
	}
}

func TestConnectHubEntryStartsAProxyThatResolvesAndFetchesASkill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture check is a POSIX shell script")
	}
	_, state := isolateClient(t)
	ws := initTestWorkspace(t)
	addCheckSkill(t, ws)
	hub, _ := startStoppableListener(t, ws)
	tokenFile, token := copyHubToken(t, ws)
	project := t.TempDir()
	if code, stdout, stderr := runConnectCLI(t, project, "--hub", hub, "--token-file", tokenFile, "--host", "claude"); code != 0 {
		t.Fatalf("connect: %s%s", stdout, stderr)
	}
	// Start exactly what the agent host would start from the written entry.
	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	entry := config.Servers["skillhub"]
	command := exec.Command(entry.Command, entry.Args...)
	command.Env = append(os.Environ(), cliHelperEnv+"=1")
	var stderr syncBuffer
	command.Stderr = &stderr
	session, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("start the proxy from the entry: %v; stderr=%s", err, stderr.String())
	}
	call := func(name string, arguments map[string]any) json.RawMessage {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v, %v; stderr=%s", name, result, err, stderr.String())
		}
		encoded, _ := json.Marshal(result.StructuredContent)
		return encoded
	}
	resolved := call("skill_resolve", map[string]any{
		"schema_version": "1", "request_id": "REQ-connect-hub",
		"task": map[string]any{"description": "verify the remote check fixture", "scope": "multi_step"},
	})
	if !strings.Contains(string(resolved), "check-skill") {
		t.Fatalf("resolve through the proxy: %s", resolved)
	}
	var outcome struct {
		Result struct {
			Local app.LocalSkill `json:"local"`
		} `json:"result"`
	}
	if err := json.Unmarshal(call("skill_get", map[string]any{"skill_id": "check-skill"}), &outcome); err != nil {
		t.Fatal(err)
	}
	local := outcome.Result.Local
	if !strings.HasPrefix(local.Path, filepath.Join(state, "skillhub", "remote")) {
		t.Fatalf("skill folder %q is not in the client cache", local.Path)
	}
	if data, err := os.ReadFile(filepath.Join(local.Path, "references", "marker.txt")); err != nil || string(data) != "marker from the hub\n" {
		t.Fatalf("fetched file: %q, %v", data, err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v; stderr=%s", err, stderr.String())
	}
	if command.ProcessState == nil || command.ProcessState.ExitCode() != 0 {
		t.Fatalf("proxy exit state %v; stderr=%s", command.ProcessState, stderr.String())
	}
	if strings.Contains(stderr.String(), token) {
		t.Fatal("the proxy printed the token")
	}
	// The doctor now reports the fetch.
	t.Chdir(project)
	code, stdout, _ := runCLI(t, "doctor")
	if code != 0 || !strings.HasSuffix(stdout, "\nHealthy.\n") || !strings.Contains(stdout, "(1 skill cached)") {
		t.Fatalf("doctor after a fetch:\n%s", stdout)
	}
}

func TestConnectSwitchesAScopeAndRemoveRestoresIt(t *testing.T) {
	isolateClient(t)
	ws := initTestWorkspace(t)
	hub, _ := startStoppableListener(t, ws)
	tokenFile, _ := copyHubToken(t, ws)
	project := t.TempDir()
	seed := map[string]string{
		".mcp.json":          "{\n  \"mcpServers\": {\n    \"other\": {\n      \"command\": \"other-server\"\n    }\n  }\n}\n",
		"CLAUDE.md":          "# Project notes\n",
		".codex/config.toml": "model = \"gpt-5\"\n",
	}
	for relative, content := range seed {
		path := filepath.Join(project, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := treeOf(t, project)

	if code, _, stderr := runConnectCLI(t, project, ws); code != 0 {
		t.Fatal(stderr)
	}
	code, stdout, stderr := runConnectCLI(t, project, "--hub", hub, "--token-file", tokenFile, "--dry-run")
	if code != 0 || !strings.Contains(stdout, "This replaces:") || !strings.Contains(flatText(stdout), "Claude Code: the local hub at "+ws) || !strings.Contains(stdout, "Claude Code: will connect (.mcp.json, .claude/settings.local.json, remove system-curator skill)") || !strings.Contains(stdout, "Agent Skills folder (.agents/skills): will connect (remove system-curator skill)") {
		t.Fatalf("switch preview: exit %d\n%s%s", code, stdout, stderr)
	}
	if code, _, stderr := runConnectCLI(t, project, "--hub", hub, "--token-file", tokenFile); code != 0 {
		t.Fatal(stderr)
	}
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Servers) != 2 || strings.Contains(string(raw), "\"serve\"") || containsPath(string(raw), ws) {
		t.Fatalf("one entry for the scope expected after the switch:\n%s", raw)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "system-curator", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("curator skill kept for a remote hub: %v", err)
	}

	code, stdout, _ = runConnectCLI(t, project, "--remove", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "Dry run: the Skill Hub connection for project") || !strings.Contains(stdout, "  $ skillhub connect --remove\n") {
		t.Fatalf("remove dry run:\n%s", stdout)
	}
	if after := treeOf(t, project); maps.Equal(before, after) {
		t.Fatal("the dry run removed the connection")
	}
	if code, stdout, stderr := runConnectCLI(t, project, "--remove"); code != 0 || !strings.Contains(stdout, "removed.") {
		t.Fatalf("remove: exit %d\n%s%s", code, stdout, stderr)
	}
	if after := treeOf(t, project); !maps.Equal(before, after) {
		t.Fatalf("remove did not restore the project.\nbefore: %v\nafter:  %v", before, after)
	}
	if code, stdout, _ := runConnectCLI(t, project, "--remove"); code != 0 || !strings.Contains(stdout, "nothing to remove") {
		t.Fatalf("second remove:\n%s", stdout)
	}
}

func TestConnectRemoveCacheClearsTheHubsFetchedSkills(t *testing.T) {
	_, state := isolateClient(t)
	project := t.TempDir()
	hub := "http://127.0.0.1:7420"
	if code, _, stderr := runConnectCLI(t, project, "--hub", hub, "--host", "claude"); code != 0 {
		t.Fatal(stderr)
	}
	if code, _, stderr := runCLI(t, "connect", "-g", "--hub", hub, "--host", "claude"); code != 0 {
		t.Fatal(stderr)
	}
	cacheSkill(t, state, hub, "alpha", strings.Repeat("a", 2048))
	cacheSkill(t, state, "http://100.64.0.2:7420", "beta", "other hub")
	code, stdout, _ := runConnectCLI(t, project, "--remove", "--cache", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "Cached skills that will be cleared:") || !strings.Contains(stdout, "http://127.0.0.1:7420  1       1         2.0 KB") ||
		!strings.Contains(flatText(stdout), "WARNING: Claude Code in your user account still uses http://127.0.0.1:7420; its agents fetch the skills again after the cache is cleared.") {
		t.Fatalf("preview:\n%s", stdout)
	}
	code, stdout, _ = runCLI(t, "connect", "--remove", "-g", "--dry-run")
	if code != 0 || !strings.Contains(flatText(stdout), "Projects that were connected while your user account had the Skill Hub instructions did not get their own copy") {
		t.Fatalf("global removal preview:\n%s", stdout)
	}
	code, stdout, _ = runConnectCLI(t, project, "--remove", "--cache", "--json", "--dry-run")
	var preview struct {
		app.Result
		Removal app.Disconnection `json:"removal"`
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &preview) != nil || preview.Status != app.StatusActionRequired ||
		len(preview.Removal.Entries) != 1 || len(preview.Removal.Cache) != 1 || preview.Removal.Cache[0].Versions != 1 {
		t.Fatalf("JSON preview:\n%s", stdout)
	}
	if code, stdout, _ := runConnectCLI(t, project, "--remove", "--cache"); code != 0 || !strings.Contains(stdout, "Cleared cached skills:") {
		t.Fatalf("apply:\n%s", stdout)
	}
	code, stdout, _ = runCLI(t, "cache", "clear", "--dry-run")
	if code != 0 || strings.Contains(stdout, "http://127.0.0.1:7420") || !strings.Contains(stdout, "http://100.64.0.2:7420") {
		t.Fatalf("the other hub's cache must stay:\n%s", stdout)
	}
}

// legacyHub answers like a hub listener from before version reporting.
func legacyHub(t *testing.T, token string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/mcp" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(writer, "Bad Request: missing session ID", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestDoctorExplainsEachBrokenRemoteConnection(t *testing.T) {
	_, state := isolateClient(t)
	ws := initTestWorkspace(t)
	hub, stop := startStoppableListener(t, ws)
	tokenFile, token := copyHubToken(t, ws)
	project := t.TempDir()
	if code, _, stderr := runConnectCLI(t, project, "--hub", hub, "--token-file", tokenFile, "--host", "claude"); code != 0 {
		t.Fatal(stderr)
	}
	t.Chdir(project)
	doctor := func() string {
		t.Helper()
		code, stdout, stderr := runCLI(t, "doctor")
		if healthy := strings.HasSuffix(stdout, "\nHealthy.\n"); (healthy && code != 0) || (!healthy && code != 1) || stderr != "" {
			t.Fatalf("doctor exit %d: %s%s", code, stdout, stderr)
		}
		if strings.Contains(stdout, token) {
			t.Fatal("doctor printed the token")
		}
		return stdout
	}
	failed := func(output string) string {
		t.Helper()
		var lines []string
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "FAILED") {
				lines = append(lines, strings.TrimSpace(line))
			}
		}
		if len(lines) != 1 {
			t.Fatalf("want exactly one failed check:\n%s", output)
		}
		return lines[0]
	}

	healthy := doctor()
	if !strings.HasPrefix(healthy, "This project uses the hub at "+hub+"\n") || !strings.HasSuffix(healthy, "\nHealthy.\n") || strings.Contains(healthy, "FAILED") || strings.Contains(healthy, "FIX:") {
		t.Fatalf("healthy:\n%s", healthy)
	}

	other := strings.Repeat("ab", 32)
	if err := os.WriteFile(tokenFile, []byte(other+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrongToken := doctor()
	if !strings.Contains(flatText(wrongToken), "The hub refused the token") || !strings.Contains(flatText(wrongToken), "FIX: Token accepted: Copy the current token file from machine B again") {
		t.Fatalf("wrong token:\n%s", wrongToken)
	}
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unwritable := ""
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if err := os.Chmod(state, 0o500); err != nil {
			t.Fatal(err)
		}
		unwritable = doctor()
		if err := os.Chmod(state, 0o700); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(flatText(unwritable), "is not writable") || !strings.Contains(flatText(unwritable), "chmod u+w "+state) {
			t.Fatalf("unwritable cache:\n%s", unwritable)
		}
	}

	stop()
	stopped := doctor()
	if flat := flatText(stopped); !strings.Contains(flat, "Nothing answers at "+hub) || !strings.Contains(flat, "the hub is stopped, or the SSH tunnel or the Tailscale connection is down") ||
		!strings.Contains(flat, "skillhub serve --addr "+strings.TrimPrefix(hub, "http://")) || !strings.Contains(flat, "ssh -N -L") {
		t.Fatalf("stopped hub:\n%s", stopped)
	}

	old := legacyHub(t, token)
	if code, _, stderr := runConnectCLI(t, project, "--hub", old, "--token-file", tokenFile, "--host", "claude"); code != 0 {
		t.Fatal(stderr)
	}
	oldHub := doctor()
	if !strings.Contains(flatText(oldHub), "The hub on machine B runs an older Skill Hub") || !strings.Contains(flatText(oldHub), "Update skillhub on machine B") {
		t.Fatalf("old hub:\n%s", oldHub)
	}

	messages := []string{failed(wrongToken), failed(stopped), failed(oldHub)}
	if unwritable != "" {
		messages = append(messages, failed(unwritable))
	}
	for i := range messages {
		for j := i + 1; j < len(messages); j++ {
			if messages[i] == messages[j] {
				t.Fatalf("two failures print the same message: %s", messages[i])
			}
		}
	}

	code, stdout, _ := runCLI(t, "doctor", "--json")
	var result doctorJSON
	if code != 1 || json.Unmarshal([]byte(stdout), &result) != nil || result.Role != app.RoleClientRemote || len(result.Sections) != 1 {
		t.Fatalf("doctor --json:\n%s", stdout)
	}
	var problems []string
	for _, finding := range result.Sections[0].Findings {
		if finding.Severity == app.SeverityProblem {
			problems = append(problems, finding.Code)
		}
	}
	if !slices.Equal(problems, []string{"remote_version"}) || strings.Contains(stdout, token) {
		t.Fatalf("doctor --json problems = %v:\n%s", problems, stdout)
	}
}

func TestDoctorWithAWorkspaceAlsoChecksRemoteEntries(t *testing.T) {
	isolateClient(t)
	ws := initTestWorkspace(t)
	if code, _, stderr := runCLI(t, "connect", "-g", "--hub", "http://127.0.0.1:9", "--host", "claude"); code != 0 {
		t.Fatal(stderr)
	}
	code, stdout, _ := runCLI(t, "doctor", "--workspace", ws)
	if code != 1 || !strings.HasPrefix(stdout, "Hub at "+ws+"\n") || !strings.Contains(stdout, "\nYour user account uses the hub at http://127.0.0.1:9\n") ||
		!strings.Contains(flatText(stdout), "Used by ok Claude Code (your user account)") || !strings.Contains(flatText(stdout), "SKILLHUB_HUB_TOKEN is not set") {
		t.Fatalf("doctor with a workspace and a remote entry:\n%s", stdout)
	}
}

func TestConnectUsageErrorsGolden(t *testing.T) {
	isolateClient(t)
	tests := []struct {
		args []string
		want string
	}{
		{
			[]string{"connect", "/w", "--hub", "http://127.0.0.1:7420"},
			"ERROR: A hub folder and --hub cannot be used together.\n" +
				"WHY: A folder is a hub on this machine and --hub a hub on another machine; one scope points at one\n" +
				"     hub.\n" +
				"FIX: Run `skillhub connect <path>` for the hub on this machine, or\n" +
				"     `skillhub connect --hub <url> --token-file <file>` for a hub on another machine.\n",
		},
		{
			[]string{"connect", "--remove", "--hub", "http://127.0.0.1:7420"},
			"ERROR: --remove cannot be combined with a hub folder, --hub or --token-file.\n" +
				"WHY: --remove takes out the Skill Hub entry of the scope, whichever hub it points at.\n" +
				"FIX: Run `skillhub connect --remove [-g|--global] [--host <name>]` in the project folder.\n",
		},
		{
			[]string{"connect", "--token-file", "/t"},
			"ERROR: --token-file needs --hub.\n" +
				"WHY: The token file is for a hub on another machine; a hub on this machine needs no token.\n" +
				"FIX: Add --hub <url>, or leave out --token-file.\n",
		},
		{
			[]string{"connect", "--cache"},
			"ERROR: --cache needs --remove.\n" +
				"WHY: --cache clears the skills fetched from a remote hub when its entry is removed.\n" +
				"FIX: Add --remove, or clear the cache alone with `skillhub cache clear --hub <url>`.\n",
		},
		{
			[]string{"connect", "--hub", "http://127.0.0.1:7420/mcp"},
			"ERROR: The hub URL cannot be used.\n" +
				"WHY: \"http://127.0.0.1:7420/mcp\" has the path /mcp; the hub URL has no path (the proxy adds /mcp\n" +
				"     itself).\n" +
				"FIX: Pass --hub http://127.0.0.1:7420.\n",
		},
		{
			[]string{"connect", "--hub", "127.0.0.1:7420"},
			"ERROR: The hub URL cannot be used.\n" +
				"WHY: \"127.0.0.1:7420\" must start with http:// or https://.\n" +
				"FIX: Pass the base URL the hub printed when it started, for example --hub http://127.0.0.1:7420.\n",
		},
	}
	for _, tc := range tests {
		code, stdout, stderr := runCLI(t, tc.args...)
		if code != 2 || stdout != "" || stderr != tc.want {
			t.Errorf("%v: exit %d, stdout %q, stderr:\n%s\nwant:\n%s", tc.args, code, stdout, stderr, tc.want)
		}
	}
	missing := filepath.Join(t.TempDir(), "hub-token")
	code, _, stderr := runCLI(t, "connect", "--hub", "http://127.0.0.1:7420", "--token-file", missing)
	flat := strings.Join(strings.Fields(stderr), " ")
	if code != 2 || !strings.HasPrefix(flat, "ERROR: The token file "+missing+" cannot be used. WHY: The file does not exist on this machine. FIX: Copy the token file from machine B") || !strings.Contains(flat, "SKILLHUB_HUB_TOKEN") {
		t.Fatalf("missing token file: exit %d\n%s", code, stderr)
	}
}

func TestRemoteConnectAndRemoveOutputGolden(t *testing.T) {
	connection := &app.RemoteConnection{
		Hub: "http://127.0.0.1:7420", TokenFile: "/home/a/.config/skillhub/hub-token", TokenSource: "file",
		CacheDir: "/home/a/.local/state/skillhub/remote/127.0.0.1-7420-0123456789",
		Check:    app.HubProbe{Outcome: app.ProbeUnreachable, Reason: "connection refused"},
		Replaces: []app.ScopeEntry{{Host: "Claude Code", Target: "the local hub at /home/a/skillhub"}},
	}
	preview := app.NewResult(app.StatusActionRequired, "Dry run: agent connection for project /home/a/app to the hub at http://127.0.0.1:7420 was not written.")
	preview.Items = []app.Item{
		{ID: "host_claude-code_mcp-registration", Summary: "claude-code (x) mcp-registration: /home/a/app/.mcp.json", Impact: "Would be written."},
		{ID: "host_claude-code_host-permissions", Summary: "claude-code (x) host-permissions: /home/a/app/.claude/settings.local.json", Impact: "Would be written."},
		{ID: "host_claude-code_native-skill:.claude/skills/system-curator/SKILL.md", Summary: "claude-code (x) native-skill: /home/a/app/.claude/skills/system-curator/SKILL.md", Impact: "Would be deleted."},
		{ID: "host_agents_shared-skill:.agents/skills/system-curator/SKILL.md", Summary: "agents (shared) shared-skill: /home/a/app/.agents/skills/system-curator/SKILL.md", Impact: "Would be deleted."},
	}
	preview.Warnings = []app.Warning{{Code: "hub_unreachable", Summary: "The hub does not answer at http://127.0.0.1:7420 right now (connection refused). The connection can be written anyway; start `skillhub serve` on machine B (and open the SSH tunnel if you use one) before you start the agent."}}
	preview.SuggestedActions = []app.Action{{Command: "skillhub connect --hub http://127.0.0.1:7420 --token-file /home/a/.config/skillhub/hub-token"}}
	var out strings.Builder
	renderRemoteConnect(termui.NewWithWidth(&out, 100), preview, connection)
	want := "Dry run: agent connection for project /home/a/app to the hub at http://127.0.0.1:7420 was not\n" +
		"written.\n" +
		"Would change:\n" +
		"  - Claude Code: will connect (.mcp.json, .claude/settings.local.json, remove system-curator skill)\n" +
		"  - Agent Skills folder (.agents/skills): will connect (remove system-curator skill)\n" +
		"This replaces:\n" +
		"  - Claude Code: the local hub at /home/a/skillhub\n" +
		"\n" +
		"Hub:          http://127.0.0.1:7420\n" +
		"Token:        /home/a/.config/skillhub/hub-token (only this path is written, never the token)\n" +
		"Hub check:    not reachable now (connection refused)\n" +
		"Skill cache:  /home/a/.local/state/skillhub/remote/127.0.0.1-7420-0123456789\n" +
		"\n" +
		"  - Curation stays on the hub machine: no curation server and no system-curator skill are installed\n" +
		"    here.\n" +
		"  - To get the token to this machine, run the scp line that `skillhub serve` prints on machine B,\n" +
		"    for example\n" +
		"    `scp -p <user>@<machine-b>:<hub-folder>/runtime/config/.hub-token ~/.skillhub-token-127.0.0.1-7420`,\n" +
		"    which keeps the file private (mode 0600); or set SKILLHUB_HUB_TOKEN in the environment the agent\n" +
		"    host starts the proxy with.\n" +
		"WARNING: The hub does not answer at http://127.0.0.1:7420 right now (connection refused). The\n" +
		"         connection can be written anyway; start `skillhub serve` on machine B (and open the SSH\n" +
		"         tunnel if you use one) before you start the agent.\n" +
		"\n" +
		"To write these files, run:\n" +
		"  $ skillhub connect --hub http://127.0.0.1:7420 --token-file /home/a/.config/skillhub/hub-token\n"
	if out.String() != want {
		t.Fatalf("connect --hub preview:\n%s\nwant:\n%s", out.String(), want)
	}

	applied := app.NewResult(app.StatusApplied, "Agent connection for project /home/a/app to the hub at http://127.0.0.1:7420 written.")
	applied.Items = preview.Items[:1]
	applied.Items[0].Impact = "Written."
	connection.Replaces = nil
	connection.TokenFile, connection.TokenSource = "", "environment"
	connection.Check = app.HubProbe{Outcome: app.ProbeAccepted, Status: &app.HubStatus{Service: "skillhub", Version: app.CurrentHubStatus().Version, RemoteAPI: app.RemoteAPILevel}}
	out.Reset()
	renderRemoteConnect(termui.NewWithWidth(&out, 100), applied, connection)
	version := app.CurrentHubStatus().Version
	want = "Agent connection for project /home/a/app to the hub at http://127.0.0.1:7420 written.\n" +
		"  - Claude Code: connected (.mcp.json)\n" +
		"\n" +
		"Hub:          http://127.0.0.1:7420\n" +
		"Token:        SKILLHUB_HUB_TOKEN in the environment the agent host starts in\n" +
		"Hub check:    reachable, token accepted, Skill Hub " + version + " (this machine: " + version + ")\n" +
		"Skill cache:  /home/a/.local/state/skillhub/remote/127.0.0.1-7420-0123456789\n" +
		"\n" +
		"  - Curation stays on the hub machine: no curation server and no system-curator skill are installed\n" +
		"    here.\n" +
		"\n" +
		"Next: restart your agent so it starts the proxy; if it cannot reach the hub, run `skillhub doctor`\n"
	if out.String() != want {
		t.Fatalf("connect --hub result:\n%s\nwant:\n%s", out.String(), want)
	}

	removal := app.NewResult(app.StatusActionRequired, "Dry run: the Skill Hub connection for project /home/a/app was not removed.")
	removal.Items = []app.Item{
		{ID: "host_claude-code_mcp-registration", Summary: "claude-code (x) mcp-registration: /home/a/app/.mcp.json", Impact: "Would be deleted."},
		{ID: "host_claude-code_bootstrap-instructions", Summary: "claude-code (x) bootstrap-instructions: /home/a/app/CLAUDE.md", Impact: "Would be written."},
	}
	removal.SuggestedActions = []app.Action{{Command: "skillhub connect --remove --project /home/a/app --cache"}}
	out.Reset()
	renderConnectRemove(termui.NewWithWidth(&out, 100), removal, &app.Disconnection{
		Entries: []app.ScopeEntry{{Host: "Claude Code", Target: "the remote hub at http://127.0.0.1:7420"}},
		Cache:   []app.CacheUsage{{Hub: "http://127.0.0.1:7420", Skills: 2, Versions: 3, Bytes: 3072}},
	}, true)
	want = "Dry run: the Skill Hub connection for project /home/a/app was not removed.\n" +
		"Would change:\n" +
		"  - Claude Code: will remove (.mcp.json deleted, CLAUDE.md)\n" +
		"The entries point at:\n" +
		"  - Claude Code: the remote hub at http://127.0.0.1:7420\n" +
		"\n" +
		"Cached skills that will be cleared:\n" +
		"Hub                    Skills  Versions  Size\n" +
		"http://127.0.0.1:7420  2       3         3.0 KB\n" +
		"\n" +
		"  - Unrelated host configuration stays as it is, and nothing changes on the hub.\n" +
		"\n" +
		"To remove the connection, run:\n" +
		"  $ skillhub connect --remove --project /home/a/app --cache\n"
	if out.String() != want {
		t.Fatalf("connect --remove preview:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestDoctorSectionOutputGolden(t *testing.T) {
	section := app.Section{Kind: app.SectionClient, Root: "/home/a/app", Title: "This project uses the hub at http://127.0.0.1:7420", Findings: []app.Finding{
		{Code: "remote_used_by", Label: "Used by", Severity: app.SeverityOK, Message: "Claude Code, Codex (project /home/a/app), Gemini CLI (your user account)"},
		{Code: "remote_token_file", Label: "Hub token", Severity: app.SeverityOK, Message: "read from /home/a/hub-token"},
		{Code: "remote_reachable", Label: "Hub reachable", Severity: app.SeverityOK, Message: "answered in 2 ms"},
		{Code: "remote_token_accepted", Label: "Token accepted", Severity: app.SeverityOK, Message: "the hub accepts the token"},
		{Code: "remote_version", Label: "Hub version", Severity: app.SeverityProblem, Message: "The hub on machine B runs an older Skill Hub (v1.0.0) than this machine (v1.1.0); some remote features will not work.", Reason: "Update skillhub on machine B, then restart `skillhub serve` there."},
		{Code: "remote_cache", Label: "Skill cache", Severity: app.SeverityProblem, Message: "The skill cache /c is not writable (permission denied), so skills fetched from the hub cannot be stored.", Reason: "Make it writable for your user (for example `chmod u+w /c`), or set XDG_STATE_HOME to a writable directory in the environment the agent host starts in."},
		{Code: "remote_last_fetch", Label: "Last fetch", Severity: app.SeverityOK, Message: "2026-10-11 15:04 (2 skills cached)"},
		{Code: "remote_reconnect", Label: "Connection files", Severity: app.SeverityProblem, Message: "Out of date: claude-code bootstrap-instructions /home/a/app/CLAUDE.md.", Manual: "skillhub connect --hub http://127.0.0.1:7420 --token-file /home/a/hub-token"},
		{Code: "hub_index_stale", Label: "Search index", Severity: app.SeverityProblem, Message: "The search index is stale.", Fix: &app.FixAction{Summary: "rebuild the search index"}},
	}}
	var out strings.Builder
	p := termui.NewWithWidth(&out, 100)
	p.Raw(section.Title + "\n")
	renderFindings(p, section, false, "skillhub doctor --fix")
	want := "This project uses the hub at http://127.0.0.1:7420\n" +
		"\n" +
		"Check             Status  Detail\n" +
		"Used by           ok      Claude Code, Codex (project /home/a/app), Gemini CLI (your user account)\n" +
		"Hub token         ok      read from /home/a/hub-token\n" +
		"Hub reachable     ok      answered in 2 ms\n" +
		"Token accepted    ok      the hub accepts the token\n" +
		"Hub version       FAILED  The hub on machine B runs an older Skill Hub (v1.0.0) than this machine\n" +
		"                          (v1.1.0); some remote features will not work.\n" +
		"Skill cache       FAILED  The skill cache /c is not writable (permission denied), so skills fetched\n" +
		"                          from the hub cannot be stored.\n" +
		"Last fetch        ok      2026-10-11 15:04 (2 skills cached)\n" +
		"Connection files  FAILED  Out of date: claude-code bootstrap-instructions /home/a/app/CLAUDE.md.\n" +
		"Search index      FAILED  The search index is stale.\n" +
		"\n" +
		"FIX:  Hub version: Update skillhub on machine B, then restart `skillhub serve` there.\n" +
		"FIX:  Skill cache: Make it writable for your user (for example `chmod u+w /c`), or set\n" +
		"      XDG_STATE_HOME to a writable directory in the environment the agent host starts in.\n" +
		"FIX:  Connection files: Run:\n" +
		"      `skillhub connect --hub http://127.0.0.1:7420 --token-file /home/a/hub-token`\n" +
		"FIX:  Search index: `skillhub doctor --fix` will rebuild the search index.\n"
	if out.String() != want {
		t.Fatalf("doctor section:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestHelpDescribesRemoteConnect(t *testing.T) {
	code, stdout, _ := runCLI(t, "help", "connect")
	for _, want := range []string{"--hub <url>", "--token-file <file>", "--remove", "--cache", "SKILLHUB_HUB_TOKEN", "scp", "system-curator"} {
		if code != 0 || !strings.Contains(stdout, want) {
			t.Fatalf("help connect lacks %q:\n%s", want, stdout)
		}
	}
	code, stdout, _ = runCLI(t, "help")
	if code != 0 || !strings.Contains(stdout, "skillhub connect --hub <url> --token-file <file>\n") {
		t.Fatalf("global help:\n%s", stdout)
	}
	code, stdout, _ = runCLI(t, "help", "doctor")
	if code != 0 || !strings.Contains(stdout, "remote hub") {
		t.Fatalf("help doctor:\n%s", stdout)
	}
}

func TestConnectRefusesATokenFileThatAnotherHubUses(t *testing.T) {
	isolateClient(t)
	tokenFile := filepath.Join(t.TempDir(), "hub-token")
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("a", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, second := "http://127.0.0.1:9", "http://127.0.0.1:10"
	if code, _, stderr := runCLI(t, "connect", "-g", "--hub", first, "--token-file", tokenFile, "--host", "claude"); code != 0 {
		t.Fatalf("first hub: %s", stderr)
	}
	project := t.TempDir()
	code, _, stderr := runConnectCLI(t, project, "--hub", second, "--token-file", tokenFile, "--host", "claude")
	flat := flatText(stderr)
	if code != 2 || !strings.Contains(flat, "is already used for the hub at "+first) || !strings.Contains(flat, hubtoken.ClientPath(second)) {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("a refused connect wrote the project entry: %v", err)
	}
	// Switching the user account itself to another hub replaces its entry, so
	// the same file is not used twice.
	if code, _, stderr := runCLI(t, "connect", "-g", "--hub", second, "--token-file", tokenFile, "--host", "claude"); code != 0 {
		t.Fatalf("switching the same scope: %s", stderr)
	}
}
