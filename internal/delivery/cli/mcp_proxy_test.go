package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
)

// addCheckSkill adds an active skill whose runtime check runs a script from
// its own folder, then rebuilds the catalog.
func addCheckSkill(t *testing.T, root string) {
	t.Helper()
	directory := filepath.Join(root, "skills", "default", "check-skill")
	files := map[string]string{
		".meta/skill.yaml": "schema_version: 1\nid: check-skill\nstatus: active\nquality:\n  reviewed: true\n" +
			"routing:\n  triggers: [verify the remote check fixture]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n" +
			"runtime:\n  setup:\n    check: sh \"$SKILLHUB_SKILL_DIR/scripts/check.sh\"\n",
		"SKILL.md":              "---\nname: check-skill\ndescription: Runs a check script from its folder.\n---\n\n# Check skill\n",
		"scripts/check.sh":      "#!/bin/sh\nset -e\ncat \"$SKILLHUB_SKILL_DIR/references/marker.txt\"\necho check passed\n",
		"references/marker.txt": "marker from the hub\n",
	}
	for relative, content := range files {
		path := filepath.Join(directory, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
}

// startListener runs `skillhub serve mcp --addr 127.0.0.1:0` in process and
// returns the hub URL it printed; the listener stops when the test ends.
func startListener(t *testing.T, ws string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr syncBuffer
	exitCh := make(chan int, 1)
	go func() {
		exitCh <- RunContext(ctx, []string{"serve", "mcp", "--addr", "127.0.0.1:0", "--workspace", ws}, &stdout, &stderr)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exitCh:
		case <-time.After(15 * time.Second):
			t.Error("listener did not stop")
		}
	})
	const marker = "skillhub connect --hub "
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if index := strings.Index(stdout.String(), marker); index != -1 {
			rest := stdout.String()[index+len(marker):]
			if end := strings.IndexByte(rest, ' '); end != -1 {
				return rest[:end]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no connect line; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	return ""
}

func TestMCPProxyRunsARemoteSkillCheckOverStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture check is a POSIX shell script")
	}
	ws := initTestWorkspace(t)
	addCheckSkill(t, ws)
	hub := startListener(t, ws)
	tokenFile := hubtoken.DefaultPath(ws)
	token, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	// The agent host starts the proxy exactly as connect --hub registers it.
	command := exec.Command(os.Args[0], "mcp", "proxy", "--hub", hub, "--token-file", tokenFile)
	command.Env = append(os.Environ(), cliHelperEnv+"=1")
	var stderr syncBuffer
	command.Stderr = &stderr
	session, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect to the proxy: %v; stderr=%s", err, stderr.String())
	}
	call := func(name string, arguments map[string]any) json.RawMessage {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v, %v; stderr=%s", name, result, err, stderr.String())
		}
		encoded, _ := json.Marshal(result.StructuredContent)
		return encoded
	}
	call("skill_resolve", map[string]any{
		"schema_version": "1", "request_id": "REQ-cli-proxy",
		"task": map[string]any{"description": "verify the remote check fixture", "scope": "multi_step"},
	})
	var outcome struct {
		Result struct {
			Local app.LocalSkill `json:"local"`
		} `json:"result"`
	}
	if err := json.Unmarshal(call("skill_get", map[string]any{"skill_id": "check-skill"}), &outcome); err != nil {
		t.Fatal(err)
	}
	local := outcome.Result.Local
	if local.Preflight == nil || !strings.HasPrefix(local.Path, filepath.Join(stateDir, "skillhub", "remote")) {
		t.Fatalf("skill_get through the proxy subprocess: local = %+v", local)
	}
	check := exec.Command("sh", "-c", local.Preflight.Check)
	check.Dir = local.Preflight.WorkingDirectory
	check.Env = os.Environ()
	for name, value := range local.Preflight.Env {
		check.Env = append(check.Env, name+"="+value)
	}
	output, err := check.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "marker from the hub") || !strings.Contains(string(output), "check passed") {
		t.Fatalf("check on the client: %v\n%s", err, output)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close: %v; stderr=%s", err, stderr.String())
	}
	if command.ProcessState == nil || command.ProcessState.ExitCode() != 0 {
		t.Fatalf("proxy exit state %v; stderr=%s", command.ProcessState, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Skill Hub proxy:  forwarding skill_resolve, skill_get and skill_feedback to "+hub) {
		t.Fatalf("proxy start-up line missing from stderr:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), token) {
		t.Fatal("the proxy printed the token")
	}
}

func TestMCPProxyStartupErrorsGolden(t *testing.T) {
	t.Setenv(hubTokenEnv, "")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no hub",
			args: []string{"mcp", "proxy"},
			want: "ERROR: The request cannot be accepted.\n" +
				"WHY: mcp proxy requires --hub <url>\n" +
				"FIX: Run `skillhub mcp proxy --hub <url> --token-file <path>`; `skillhub connect --hub` writes this\n" +
				"     for your agent host.\n",
		},
		{
			name: "URL with credentials",
			args: []string{"mcp", "proxy", "--hub", "http://user:secret@127.0.0.1:7420"},
			want: "ERROR: The hub URL cannot be used.\n" +
				"WHY: \"http://user:secret@127.0.0.1:7420\" must not contain a user name or password; pass the token\n" +
				"     with --token-file\n" +
				"FIX: Pass the base URL the hub printed when it started, for example --hub http://127.0.0.1:7420.\n",
		},
		{
			name: "no token",
			args: []string{"mcp", "proxy", "--hub", "http://127.0.0.1:7420"},
			want: "ERROR: No hub token was given.\n" +
				"WHY: Every request to a remote hub needs the hub's shared token.\n" +
				"FIX: Pass --token-file <path> with a copy of the hub's token file, or set SKILLHUB_HUB_TOKEN.\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != 2 || stdout != "" || stderr != tc.want {
				t.Fatalf("exit %d, stdout %q, stderr:\n%s\nwant:\n%s", code, stdout, stderr, tc.want)
			}
		})
	}

	t.Run("unusable token in the environment", func(t *testing.T) {
		t.Setenv(hubTokenEnv, "short token\nwith a newline")
		code, stdout, stderr := runCLI(t, "mcp", "proxy", "--hub", "http://127.0.0.1:7420")
		want := "ERROR: SKILLHUB_HUB_TOKEN does not hold a usable token.\n" +
			"WHY: A token is one line of letters, digits and the characters - . _ ~ + / =.\n" +
			"FIX: Set SKILLHUB_HUB_TOKEN to the content of the hub's token file, or pass --token-file <path>.\n"
		if code != 2 || stdout != "" || stderr != want {
			t.Fatalf("exit %d, stdout %q, stderr:\n%s\nwant:\n%s", code, stdout, stderr, want)
		}
	})

	t.Run("token file readable by others", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows files have no group or other permission bits")
		}
		tokenFile := filepath.Join(t.TempDir(), "hub-token")
		if err := os.WriteFile(tokenFile, []byte(strings.Repeat("ab", 32)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runCLI(t, "mcp", "proxy", "--hub", "http://127.0.0.1:7420", "--token-file", tokenFile)
		flat := strings.Join(strings.Fields(stderr), " ")
		want := "ERROR: The hub token file " + tokenFile + " cannot be used."
		fix := "FIX: Run `chmod 600 " + tokenFile + "`: the token must be readable by you only."
		if code != 2 || stdout != "" || !strings.HasPrefix(flat, want) || !strings.HasSuffix(flat, fix) || strings.Contains(stderr, strings.Repeat("ab", 32)) {
			t.Fatalf("exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
		}
	})
}

func TestProxyBannerGolden(t *testing.T) {
	var out bytes.Buffer
	writeProxyBanner(&out, "http://127.0.0.1:7420", "/home/user/.local/state/skillhub/remote/127.0.0.1-7420-0123456789")
	want := "Skill Hub proxy:  forwarding skill_resolve, skill_get and skill_feedback to http://127.0.0.1:7420\n" +
		"Skill cache:      /home/user/.local/state/skillhub/remote/127.0.0.1-7420-0123456789\n"
	if out.String() != want {
		t.Fatalf("banner:\n%s\nwant:\n%s", out.String(), want)
	}
	out.Reset()
	lockedWarning(&out)("The Skill Hub at http://127.0.0.1:7420 cannot be reached.")
	if out.String() != "WARNING: The Skill Hub at http://127.0.0.1:7420 cannot be reached.\n" {
		t.Fatalf("warning = %q", out.String())
	}
}

func digestOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// cacheSkill places one fetched version of a skill in the client cache.
func cacheSkill(t *testing.T, stateDir, hubURL, skillID, body string) {
	t.Helper()
	hubID, err := skillruntime.HubID(hubURL)
	if err != nil {
		t.Fatal(err)
	}
	cache := skillruntime.RemoteCache{Root: skillruntime.RemoteCacheDir(filepath.Join(stateDir, "skillhub"))}
	if err := cache.RecordHubURL(hubID, hubURL); err != nil {
		t.Fatal(err)
	}
	contents := map[string][]byte{"SKILL.md": []byte(body)}
	file := skillruntime.BundleFile{Path: "SKILL.md", Digest: digestOf(body), Size: int64(len(body))}
	digest, err := skillruntime.ManifestDigest([]skillruntime.BundleFile{file})
	if err != nil {
		t.Fatal(err)
	}
	metadata := skillruntime.BundleMetadata{Schema: skillruntime.BundleSchema, SkillID: skillID, ManifestVersion: "sha256:" + digest, DepsKey: "0123456789abcdef", Files: []skillruntime.BundleFile{file}}
	var archive bytes.Buffer
	if err := skillruntime.WriteBundle(&archive, metadata, contents); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Install(hubID, skillID, digest, &archive); err != nil {
		t.Fatal(err)
	}
}

func TestCacheClearGolden(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	cacheDir := filepath.Join(stateDir, "skillhub", "remote")

	code, stdout, stderr := runCLI(t, "cache", "clear", "--dry-run")
	want := "No skills fetched from a remote hub are cached on this machine.\n" +
		"Cache directory:  " + cacheDir + "\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("empty cache: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}

	cacheSkill(t, stateDir, "http://127.0.0.1:7420", "alpha", strings.Repeat("a", 2048))
	cacheSkill(t, stateDir, "http://127.0.0.1:7420", "beta", strings.Repeat("b", 1024))
	cacheSkill(t, stateDir, "http://100.64.0.2:7420", "alpha", strings.Repeat("c", 100))

	code, stdout, stderr = runCLI(t, "cache", "clear", "--dry-run")
	want = "Dry run: the copies of skills fetched from remote hubs were not removed.\n" +
		"Cache directory:  " + cacheDir + "\n" +
		"\n" +
		"Would change:\n" +
		"Hub                     Skills  Versions  Size\n" +
		"http://100.64.0.2:7420  1       1         100 bytes\n" +
		"http://127.0.0.1:7420   2       2         3.0 KB\n" +
		"\n" +
		"  - An agent fetches each skill again from its hub the next time it uses it.\n" +
		"  - The skills' working directories (installed dependencies) and stored secrets are kept.\n" +
		"  - Nothing on the hub changes.\n" +
		"\n" +
		"Next: Clear the cache:\n" +
		"  $ skillhub cache clear\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("dry run: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}

	code, stdout, stderr = runCLI(t, "cache", "clear", "--hub", "http://127.0.0.1:7420/")
	want = "Removed 2 cached skill versions (3.0 KB).\n" +
		"Cache directory:  " + cacheDir + "\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("clear one hub: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}
	code, stdout, _ = runCLI(t, "cache", "clear", "--hub", "http://100.64.0.2:7420", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "Dry run: the copies of skills fetched from http://100.64.0.2:7420 were not removed.") || !strings.Contains(stdout, "  $ skillhub cache clear --hub http://100.64.0.2:7420\n") {
		t.Fatalf("dry run of the other hub: exit %d, stdout:\n%s", code, stdout)
	}
	code, stdout, _ = runCLI(t, "cache", "clear")
	if code != 0 || stdout != "Removed 1 cached skill version (100 bytes).\nCache directory:  "+cacheDir+"\n" {
		t.Fatalf("clear every hub: exit %d, stdout:\n%s", code, stdout)
	}

	code, stdout, stderr = runCLI(t, "cache", "clear", "--hub")
	want = "ERROR: The request cannot be accepted.\n" +
		"WHY: --hub requires the hub's URL; leave --hub out to clear every hub's cache\n" +
		"FIX: Run `skillhub cache clear [--hub <url>] [--dry-run]`.\n"
	if code != 2 || stdout != "" || stderr != want {
		t.Fatalf("--hub without a URL: exit %d, stdout %q, stderr:\n%s\nwant:\n%s", code, stdout, stderr, want)
	}
	if code, _, stderr := runCLI(t, "cache"); code != 2 || !strings.Contains(stderr, "cache requires the clear subcommand") {
		t.Fatalf("bare cache: exit %d, stderr %q", code, stderr)
	}
}

func TestHelpDescribesTheRemoteCommands(t *testing.T) {
	code, stdout, _ := runCLI(t, "help")
	if code != 0 || strings.Contains(stdout, "mcp proxy") || !strings.Contains(stdout, "  cache     Clear the copies of skills fetched from a remote hub") {
		t.Fatalf("global help:\n%s", stdout)
	}
	code, stdout, _ = runCLI(t, "help", "mcp")
	if code != 0 || !strings.Contains(stdout, "skillhub mcp proxy --hub <url> [--token-file <path>]") || !strings.Contains(stdout, "SKILLHUB_HUB_TOKEN") {
		t.Fatalf("help mcp:\n%s", stdout)
	}
	code, stdout, _ = runCLI(t, "cache", "--help")
	if code != 0 || !strings.HasPrefix(stdout, "Usage: skillhub cache clear [--hub <url>] [--dry-run]") {
		t.Fatalf("cache --help:\n%s", stdout)
	}
}
