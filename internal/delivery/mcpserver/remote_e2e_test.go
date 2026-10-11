package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// The tests in this file play machine B (the hub) and machine A (an agent
// host that uses it) on one computer. Each machine has its own home, config,
// state and cache directories; the hub's network listener and the stdio
// proxy run as separate processes; every port is chosen by the OS on
// 127.0.0.1, so nothing is reachable from another machine.

// machine is the set of per-user directories of one simulated computer.
type machine struct {
	root string
}

func newMachine(t *testing.T, name string) machine {
	t.Helper()
	m := machine{root: filepath.Join(t.TempDir(), name)}
	for _, directory := range []string{"home", "config", "data", "cache", "state", "runtime"} {
		if err := os.MkdirAll(filepath.Join(m.root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func (m machine) home() string  { return filepath.Join(m.root, "home") }
func (m machine) state() string { return filepath.Join(m.root, "state") }

// env is the test environment with this machine's directories; later entries
// win over the inherited ones.
func (m machine) env() []string {
	return append(remoteBenchEnv(),
		"HOME="+m.home(), "USERPROFILE="+m.home(),
		"XDG_CONFIG_HOME="+filepath.Join(m.root, "config"), "XDG_DATA_HOME="+filepath.Join(m.root, "data"),
		"XDG_CACHE_HOME="+filepath.Join(m.root, "cache"), "XDG_STATE_HOME="+m.state(),
		"XDG_RUNTIME_DIR="+filepath.Join(m.root, "runtime"),
		"SKILLHUB_USER_CONFIG_DIR="+filepath.Join(m.root, "config", "skillhub-user"),
		"SKILLHUB_WORKSPACE=", app.HubTokenEnv+"=",
	)
}

// hubProcess is `skillhub serve mcp --addr 127.0.0.1:0` running as its own
// process, the listener machine B starts.
type hubProcess struct {
	url       string
	tokenFile string
	command   *exec.Cmd
	stdout    *lockedBuffer
	stderr    *lockedBuffer
	exited    chan struct{}
	stopOnce  sync.Once
}

func startHubProcess(t *testing.T, binary, root string, env []string) *hubProcess {
	t.Helper()
	hub := &hubProcess{tokenFile: hubtoken.DefaultPath(root), stdout: &lockedBuffer{}, stderr: &lockedBuffer{}, exited: make(chan struct{})}
	hub.command = exec.Command(binary, "serve", "mcp", "--addr", "127.0.0.1:0", "--workspace", root)
	hub.command.Env = env
	hub.command.Stdout, hub.command.Stderr = hub.stdout, hub.stderr
	if err := hub.command.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = hub.command.Wait()
		close(hub.exited)
	}()
	t.Cleanup(func() { hub.stop(t) })
	const marker = "skillhub connect --hub "
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out := hub.stdout.String()
		if index := strings.Index(out, marker); index != -1 {
			rest := out[index+len(marker):]
			if end := strings.IndexByte(rest, ' '); end > 0 {
				hub.url = rest[:end]
				return hub
			}
		}
		select {
		case <-hub.exited:
			t.Fatalf("the listener exited before it was ready; stdout: %s; stderr: %s", out, hub.stderr.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatalf("the listener printed no connect line; stdout: %s; stderr: %s", hub.stdout.String(), hub.stderr.String())
	return nil
}

// stop interrupts the listener, as Ctrl-C does, and waits until it has
// flushed its telemetry and exited.
func (hub *hubProcess) stop(t *testing.T) {
	t.Helper()
	hub.stopOnce.Do(func() {
		if runtime.GOOS == "windows" {
			_ = hub.command.Process.Kill()
		} else {
			_ = hub.command.Process.Signal(os.Interrupt)
		}
		select {
		case <-hub.exited:
		case <-time.After(15 * time.Second):
			_ = hub.command.Process.Kill()
			<-hub.exited
			t.Error("the listener did not stop after an interrupt")
		}
	})
}

// proxyProcess is the stdio proxy an agent host on machine A starts, with an
// MCP client standing in for the agent.
type proxyProcess struct {
	session *mcp.ClientSession
	command *exec.Cmd
	stderr  *lockedBuffer
	startup time.Duration
	closed  bool
}

func startProxyProcess(t *testing.T, command *exec.Cmd) *proxyProcess {
	t.Helper()
	proxied := &proxyProcess{command: command, stderr: &lockedBuffer{}}
	command.Stderr = proxied.stderr
	started := time.Now()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).
		Connect(t.Context(), &mcp.CommandTransport{Command: command, TerminateDuration: 15 * time.Second}, nil)
	if err != nil {
		t.Fatalf("start the proxy: %v; stderr=%s", err, proxied.stderr.String())
	}
	proxied.startup = time.Since(started)
	proxied.session = session
	t.Cleanup(func() { proxied.close(t) })
	return proxied
}

// close ends the proxy's stdin, as an agent host does when it quits, and
// checks that it exited cleanly.
func (proxied *proxyProcess) close(t *testing.T) {
	t.Helper()
	if proxied.closed {
		return
	}
	proxied.closed = true
	if err := proxied.session.Close(); err != nil {
		t.Errorf("close the proxy: %v; stderr=%s", err, proxied.stderr.String())
	}
	if state := proxied.command.ProcessState; state == nil || state.ExitCode() != 0 {
		t.Errorf("proxy exit state %v; stderr=%s", state, proxied.stderr.String())
	}
}

// proxyCommand is the command `skillhub connect --hub` registers.
func proxyCommand(binary, hubURL, tokenFile string, env []string) *exec.Cmd {
	command := exec.Command(binary, "mcp", "proxy", "--hub", hubURL, "--token-file", tokenFile)
	command.Env = env
	return command
}

func toolCall(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) (*mcp.CallToolResult, []byte) {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	return result, encoded
}

func resolveFor(t *testing.T, session *mcp.ClientSession, task, requestID string) (resolveResult, []byte) {
	t.Helper()
	result, encoded := toolCall(t, session, "skill_resolve", map[string]any{
		"schema_version": SchemaVersion, "request_id": requestID,
		"task": map[string]any{"description": task, "scope": "multi_step"},
	})
	var outcome toolOutcome[resolveResult]
	if result.IsError || json.Unmarshal(encoded, &outcome) != nil || outcome.Result == nil {
		t.Fatalf("skill_resolve %q = %s", task, encoded)
	}
	return *outcome.Result, encoded
}

func decodeSkillGet(t *testing.T, encoded []byte) proxyGetResult {
	t.Helper()
	var outcome struct {
		Result *proxyGetResult `json:"result"`
	}
	if err := json.Unmarshal(encoded, &outcome); err != nil || outcome.Result == nil {
		t.Fatalf("skill_get = %s (%v)", encoded, err)
	}
	return *outcome.Result
}

// servedFiles counts the files of a fixture skill a hub serves (.meta/ is
// curation data and stays on the hub).
func servedFiles(t *testing.T, root, id string) int {
	t.Helper()
	count := 0
	directory := filepath.Join(root, "skills", "default", id)
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".meta" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// TestRemoteHubEndToEndThroughATunnel follows one user from setup to a skill
// run: machine B starts the listener, machine A reaches it through a
// tunnel-like relay on a second port (as `ssh -N -L` gives), copies the token
// file, runs `skillhub connect --hub`, and its agent host starts the proxy
// from the written entry. The agent resolves and gets every skill of the hub,
// runs a skill's check on machine A and reports feedback. The copies on A are
// byte-identical to the hub's, the answers match what B's own stdio server
// gives, the hub records the remote calls under the remote client label, and
// the token appears in no output and in no file except the two token files.
func TestRemoteHubEndToEndThroughATunnel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture check is a POSIX shell script")
	}
	binary := buildSkillHub(t)
	machineB, machineA := newMachine(t, "machine-b"), newMachine(t, "machine-a")
	root := newBundleHubFixture(t)
	skillIDs := []string{"test-audit", "herdr-cook-plan", bundleCheckSkill}

	hub := startHubProcess(t, binary, root, machineB.env())
	tunnel := startLink(t, strings.TrimPrefix(hub.url, "http://"), 0)
	if tunnel.url == hub.url {
		t.Fatal("the tunnel must use its own port")
	}
	token, err := hubtoken.Read(hub.tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	// Token hand-off: the user copies the file to a fixed place on A.
	tokenCopy := filepath.Join(machineA.home(), ".config", "skillhub", "hub-token")
	if err := os.MkdirAll(filepath.Dir(tokenCopy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenCopy, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	project := filepath.Join(machineA.home(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	connect := exec.Command(binary, "connect", "--hub", tunnel.url, "--token-file", tokenCopy, "--host", "claude")
	connect.Dir = project
	connect.Env = machineA.env()
	connectOutput, err := connect.CombinedOutput()
	if err != nil || !strings.Contains(string(connectOutput), "reachable, token accepted") {
		t.Fatalf("connect --hub: %v\n%s", err, connectOutput)
	}
	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil || json.Unmarshal(raw, &config) != nil {
		t.Fatalf("read the written entry: %v\n%s", err, raw)
	}
	entry := config.Servers["skillhub"]
	// connect records the binary's resolved path (macOS temp dirs are symlinks).
	wantBinary, err := filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	gotBinary, err := filepath.EvalSymlinks(entry.Command)
	if err != nil || gotBinary != wantBinary || !slices.Equal(entry.Args, []string{"mcp", "proxy", "--hub", tunnel.url, "--token-file", tokenCopy}) {
		t.Fatalf("written entry = %+v", entry)
	}
	agentHost := exec.Command(entry.Command, entry.Args...)
	agentHost.Env = machineA.env()
	proxied := startProxyProcess(t, agentHost)

	// Machine B's own agents keep using stdio on the same workspace.
	localCommand := exec.Command(binary, "mcp", "serve", "--profile", "runtime", "--workspace", root)
	localCommand.Env = machineB.env()
	localSession, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).
		Connect(t.Context(), &mcp.CommandTransport{Command: localCommand, TerminateDuration: 15 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = localSession.Close() })

	var answers [][]byte
	var checkResolution string
	for _, id := range skillIDs {
		resolved, encoded := resolveFor(t, proxied.session, remoteBenchTaskText(t, root, id), "REQ-e2e-"+id)
		answers = append(answers, encoded)
		if resolved.Resolution.Primary == nil || resolved.Resolution.Primary.ID != id {
			t.Fatalf("resolve through the tunnel chose %s", encoded)
		}
		if id == bundleCheckSkill {
			checkResolution = resolved.Resolution.ResolutionID
		}
		result, encoded := toolCall(t, proxied.session, "skill_get", map[string]any{"skill_id": id})
		answers = append(answers, encoded)
		got := decodeSkillGet(t, encoded)
		if result.IsError || got.Local == nil || got.Remote == nil || got.Remote.Cache != remoteCacheFetched || got.Remote.Hub != tunnel.url {
			t.Fatalf("skill_get %s through the proxy = %s", id, encoded)
		}
		digest := hubDigest(t, root, id)
		folder, files := folderDigest(t, got.Local.Path)
		if folder != digest || files != servedFiles(t, root, id) || !strings.HasPrefix(got.Local.Path, machineA.state()) {
			t.Fatalf("%s on machine A: digest %s with %d files at %s; hub digest %s with %d served files", id, folder, files, got.Local.Path, digest, servedFiles(t, root, id))
		}
		for name, value := range got.Local.Env {
			if !strings.HasPrefix(value, machineA.state()) {
				t.Fatalf("%s=%s does not point at machine A", name, value)
			}
		}
		if got.Local.Env[skillruntime.EnvSkillDir] != got.Local.Path {
			t.Fatalf("%s = %q, want the cached folder %q", skillruntime.EnvSkillDir, got.Local.Env[skillruntime.EnvSkillDir], got.Local.Path)
		}

		// The same skill through machine B's own stdio server.
		_, localEncoded := toolCall(t, localSession, "skill_get", map[string]any{"skill_id": id})
		var local struct {
			Result *skillGetResult `json:"result"`
		}
		if json.Unmarshal(localEncoded, &local) != nil || local.Result == nil || local.Result.Local == nil {
			t.Fatalf("local skill_get %s = %s", id, localEncoded)
		}
		if local.Result.Content != got.Content || local.Result.Local.ManifestVersion != got.Local.ManifestVersion || local.Result.Local.Status != got.Local.Status {
			t.Fatalf("%s differs between the local and the remote answer:\nlocal  %s\nremote %s", id, localEncoded, encoded)
		}
		var full struct {
			Result *proxySkillGetResult `json:"result"`
		}
		if json.Unmarshal(encoded, &full) != nil || full.Result == nil || !reflect.DeepEqual(local.Result.SkillDetail, full.Result.SkillDetail) {
			t.Fatalf("%s skill detail differs:\nlocal  %s\nremote %s", id, localEncoded, encoded)
		}
		localPreflight, remotePreflight := local.Result.Local.Preflight, got.Local.Preflight
		if (localPreflight == nil) != (remotePreflight == nil) ||
			(localPreflight != nil && (localPreflight.Check != remotePreflight.Check || localPreflight.Setup != remotePreflight.Setup ||
				!reflect.DeepEqual(localPreflight.Requires, remotePreflight.Requires) || !reflect.DeepEqual(localPreflight.LiveChecks, remotePreflight.LiveChecks))) {
			t.Fatalf("%s runtime spec differs:\nlocal  %+v\nremote %+v", id, localPreflight, remotePreflight)
		}
		if id != bundleCheckSkill {
			continue
		}
		// The check runs on machine A from the cached folder.
		check := exec.Command("sh", "-c", remotePreflight.Check)
		check.Dir = remotePreflight.WorkingDirectory
		check.Env = machineA.env()
		for name, value := range remotePreflight.Env {
			check.Env = append(check.Env, name+"="+value)
		}
		output, err := check.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "check passed") {
			t.Fatalf("check on machine A: %v\n%s", err, output)
		}
		ran, err := os.ReadFile(filepath.Join(remotePreflight.WorkingDirectory, "check-ran.txt"))
		if err != nil || !strings.HasPrefix(string(ran), "marker from the hub\n") {
			t.Fatalf("the check did not read the cached folder: %q, %v", ran, err)
		}
	}

	result, encoded := toolCall(t, proxied.session, "skill_feedback", map[string]any{
		"schema_version": SchemaVersion, "resolution_id": checkResolution,
		"event_id": "evt-remote-e2e", "outcome": "used", "selected_skill": bundleCheckSkill,
	})
	answers = append(answers, encoded)
	if result.IsError {
		t.Fatalf("skill_feedback through the proxy = %s", encoded)
	}
	proxied.close(t)
	if err := localSession.Close(); err != nil {
		t.Fatal(err)
	}
	hub.stop(t)

	for _, answer := range answers {
		if bytes.Contains(answer, []byte(root)) || bytes.Contains(answer, []byte(machineB.root)) {
			t.Fatalf("an answer on machine A names a path on machine B: %s", answer)
		}
	}
	recorder := openHubRecorder(t, root)
	remote := telemetry.Client{Name: "remote:claude-code", Version: "1"}
	var used, loads, remoteResolutions int
	for _, event := range recentRawEvents(t, recorder) {
		switch {
		case event.Type == telemetry.EventSkillUsed && event.Client == remote:
			used++
		case event.Type == telemetry.EventSkillLoaded && event.Client == remote && event.Payload["surface"] == "skill_bundle":
			loads++
		case event.Type == telemetry.EventResolutionCompleted && event.Client == remote:
			remoteResolutions++
		}
	}
	if used != 1 || loads != len(skillIDs) || remoteResolutions != len(skillIDs) {
		t.Fatalf("hub telemetry for machine A: %d used, %d bundle loads, %d resolutions; want 1, %d, %d", used, loads, remoteResolutions, len(skillIDs), len(skillIDs))
	}
	if found := filesContaining(t, machineA.root, [][]byte{[]byte("telemetry_events")}); len(found) != 0 {
		t.Fatalf("machine A keeps telemetry of its own: %v", found)
	}

	needle := [][]byte{[]byte(token)}
	outputs := map[string]string{
		"listener stdout": hub.stdout.String(), "listener stderr": hub.stderr.String(),
		"connect": string(connectOutput), "proxy stderr": proxied.stderr.String(),
	}
	for name, text := range outputs {
		if strings.Contains(text, token) {
			t.Fatalf("the token appears in the %s", name)
		}
	}
	for _, answer := range answers {
		if bytes.Contains(answer, []byte(token)) {
			t.Fatal("the token appears in an answer")
		}
	}
	for _, directory := range []string{machineA.root, root, machineB.root} {
		for _, path := range filesContaining(t, directory, needle) {
			if path != tokenCopy && path != hub.tokenFile {
				t.Fatalf("the token is stored in %s", path)
			}
		}
	}
}

// frontHub stands between the proxy and a real hub and can change how the
// hub answers bundle requests, to play an interrupted download, a truncated
// archive, or a hub from before bundles existed.
type frontHub struct {
	url  string
	mode atomic.Value // string: "", "cut", "truncate", "stall" or "legacy"
	// stalled receives one value each time a "stall" answer has sent its
	// first half and stopped, so a test can act in the middle of a download
	// instead of guessing how long the hub takes to build the bundle.
	stalled chan struct{}
}

func startFrontHub(t *testing.T, hub *httpTestHub) *frontHub {
	t.Helper()
	target, err := url.Parse(hub.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)
	forward.FlushInterval = -1
	front := &frontHub{stalled: make(chan struct{}, 4)}
	front.mode.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mode, _ := front.mode.Load().(string)
		switch {
		case mode == "legacy" && (request.URL.Path == HTTPBundlePath || request.URL.Path == HTTPStatusPath):
			http.NotFound(writer, request)
		case (mode == "cut" || mode == "truncate" || mode == "stall") && request.URL.Path == HTTPBundlePath:
			front.damageBundle(t, writer, request, hub.baseURL, mode)
		default:
			forward.ServeHTTP(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	front.url = server.URL
	return front
}

// damageBundle fetches the real bundle and sends only its first half: "cut"
// announces the full length and then drops the connection, as when the hub
// stops in the middle of a download; "stall" announces the full length and
// then sends nothing more until the client gives up; "truncate" announces the
// shorter length, so the HTTP answer is complete but the archive is not.
func (front *frontHub) damageBundle(t *testing.T, writer http.ResponseWriter, request *http.Request, hubURL, mode string) {
	// The real bundle is fetched even when the client gives up meanwhile: a
	// client that stops waiting is what some tests play, not a failure here.
	upstream, err := http.NewRequestWithContext(context.WithoutCancel(request.Context()), http.MethodGet, hubURL+request.URL.RequestURI(), nil)
	if err != nil {
		t.Error(err)
		return
	}
	upstream.Header = request.Header.Clone()
	response, err := http.DefaultTransport.RoundTrip(upstream)
	if err != nil {
		t.Error(err)
		return
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Error(err)
		return
	}
	for name, values := range response.Header {
		writer.Header()[name] = values
	}
	if response.StatusCode != http.StatusOK {
		writer.WriteHeader(response.StatusCode)
		_, _ = writer.Write(body)
		return
	}
	half := body[:len(body)/2]
	if mode == "truncate" {
		writer.Header().Set("Content-Length", fmt.Sprint(len(half)))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(half)
		return
	}
	writer.Header().Set("Content-Length", fmt.Sprint(len(body)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(half)
	writer.(http.Flusher).Flush()
	if mode == "stall" {
		select {
		case front.stalled <- struct{}{}:
		default:
		}
		<-request.Context().Done()
		return
	}
	connection, buffered, err := writer.(http.Hijacker).Hijack()
	if err != nil {
		t.Error(err)
		return
	}
	_ = buffered.Flush()
	_ = connection.Close()
}

// deadlineOnSignal is a context whose deadline passes when expired closes,
// so a test decides the moment a deadline passes by an event rather than by
// the wall clock.
type deadlineOnSignal struct {
	context.Context
	expired <-chan struct{}
}

func (ctx deadlineOnSignal) Done() <-chan struct{} { return ctx.expired }

func (ctx deadlineOnSignal) Err() error {
	select {
	case <-ctx.expired:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

// assertNothingCached checks that a failed fetch left no version and no
// half-written staging directory behind.
func assertNothingCached(t *testing.T, proxied *testProxy, id string) {
	t.Helper()
	if versions := proxied.proxy.cache.Versions(proxied.proxy.hubID, id); len(versions) != 0 {
		t.Fatalf("a failed fetch cached %d versions of %s", len(versions), id)
	}
	staging := filepath.Join(proxied.proxy.CacheDir(), ".staging")
	if entries, err := os.ReadDir(staging); err == nil && len(entries) != 0 {
		t.Fatalf("a failed fetch left %d entries in %s", len(entries), staging)
	}
}

func toolErrorOf(t *testing.T, result *mcp.CallToolResult, encoded []byte) toolError {
	t.Helper()
	var outcome struct {
		Error *toolError `json:"error"`
	}
	if !result.IsError || json.Unmarshal(encoded, &outcome) != nil || outcome.Error == nil {
		t.Fatalf("want a tool error, got %s", encoded)
	}
	return *outcome.Error
}

// TestRemoteFetchFailures covers what the agent on machine A sees when a
// fetch goes wrong, and that no broken copy is ever kept. A wrong token, an
// unapproved skill, a tampered archive, a hub that answers an error and a
// stopped hub with and without a cached copy are covered in proxy_test.go
// (TestProxyRejectsAWrongToken, TestProxyKeepsTheReviewGate,
// TestProxyRejectsATamperedBundle, TestProxyServesNoCachedCopyWhenTheHubAnswersAnError,
// TestProxyUsesTheCacheWhenTheHubIsDown); a hub of another version is
// explained by doctor (TestDoctorExplainsEachBrokenRemoteConnection,
// TestProbeHubTellsTheFailuresApart).
func TestRemoteFetchFailures(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	const id = "herdr-cook-plan"
	digest := hubDigest(t, root, id)

	t.Run("the hub stops in the middle of a download", func(t *testing.T) {
		front := startFrontHub(t, hub)
		proxied := startTestProxy(t, front.url, hub.token, t.TempDir(), nil)
		front.mode.Store("cut")
		result, encoded := callProxyTool(t, proxied.session, "skill_get", map[string]any{"skill_id": id})
		failure := toolErrorOf(t, result, encoded)
		if !failure.Retryable || (failure.Code != "resource_digest_mismatch" && failure.Code != "hub_unreachable") {
			t.Fatalf("interrupted download = %s", encoded)
		}
		assertNothingCached(t, proxied, id)

		front.mode.Store("")
		got, result, encoded := proxySkillGet(t, proxied.session, id)
		if result.IsError || got.Remote == nil || got.Remote.Cache != remoteCacheFetched {
			t.Fatalf("retry after the interrupted download = %s", encoded)
		}
		if folder, _ := folderDigest(t, got.Local.Path); folder != digest {
			t.Fatalf("retried copy digest %s, want %s", folder, digest)
		}
	})

	t.Run("a truncated archive is refused", func(t *testing.T) {
		front := startFrontHub(t, hub)
		front.mode.Store("truncate")
		proxied := startTestProxy(t, front.url, hub.token, t.TempDir(), nil)
		result, encoded := callProxyTool(t, proxied.session, "skill_get", map[string]any{"skill_id": id})
		if failure := toolErrorOf(t, result, encoded); failure.Code != "resource_digest_mismatch" || !failure.Retryable {
			t.Fatalf("truncated archive = %s", encoded)
		}
		assertNothingCached(t, proxied, id)
	})

	t.Run("a read-only cache directory is named in the error", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("needs POSIX permissions and a non-root user")
		}
		proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), nil)
		cacheDir := proxied.proxy.CacheDir()
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(cacheDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(cacheDir, 0o700) })
		resolveFor(t, proxied.session, remoteBenchTaskText(t, root, id), "REQ-read-only")
		result, encoded := callProxyTool(t, proxied.session, "skill_get", map[string]any{"skill_id": id})
		failure := toolErrorOf(t, result, encoded)
		if failure.Code != "internal_error" || !strings.Contains(failure.Message, cacheDir) || !strings.Contains(failure.SuggestedAction, "permissions") {
			t.Fatalf("read-only cache = %s", encoded)
		}
		if err := os.Chmod(cacheDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if got, result, encoded := proxySkillGet(t, proxied.session, id); result.IsError || got.Remote == nil || got.Remote.Cache != remoteCacheFetched {
			t.Fatalf("skill_get once the cache is writable again = %s", encoded)
		}
	})

	t.Run("a hub from before bundles still answers skill_get without a hub path", func(t *testing.T) {
		front := startFrontHub(t, hub)
		front.mode.Store("legacy")
		proxied := startTestProxy(t, front.url, hub.token, t.TempDir(), nil)
		got, result, encoded := proxySkillGet(t, proxied.session, id)
		hubSkillMD, err := os.ReadFile(filepath.Join(root, "skills", "default", id, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError || got.Content != string(hubSkillMD) || (got.Local != nil && got.Local.Path != "") ||
			bytes.Contains(encoded, []byte(root)) || bytes.Contains(encoded, []byte(strings.ReplaceAll(root, `\`, `\\`))) {
			t.Fatalf("skill_get from an older hub = %s", encoded)
		}
		assertNothingCached(t, proxied, id)
	})

	t.Run("a download that stalls counts as an unreachable hub and a cancelled one as cancelled", func(t *testing.T) {
		// Its own hub: the skill changes on it.
		root := newBundleHubFixture(t)
		hub := startHTTPTestHub(t, root)
		front := startFrontHub(t, hub)
		proxied := startTestProxy(t, front.url, hub.token, t.TempDir(), nil)
		old, result, encoded := proxySkillGet(t, proxied.session, id)
		if result.IsError || old.Remote == nil || old.Remote.Cache != remoteCacheFetched {
			t.Fatalf("first fetch = %s", encoded)
		}
		writeFixtureFile(t, filepath.Join(root, "skills", "default", id, "references", "dispatch.md"), []byte("# Dispatch\n\nChanged on the hub.\n"))
		rebuildFixtureCatalog(t, root)
		// The resolution names the new version, so the next get downloads it.
		resolveFor(t, proxied.session, remoteBenchTaskText(t, root, id), "REQ-stall")
		front.mode.Store("stall")
		request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "skill_get", Arguments: json.RawMessage(`{"skill_id":"` + id + `"}`)}}

		// The proxy's own deadline for one download, shortened: it passes once
		// the hub has sent half of the bundle and stopped.
		expired := make(chan struct{})
		go func() {
			select {
			case <-front.stalled:
			case <-t.Context().Done():
			}
			close(expired)
		}()
		stalled, err := proxied.proxy.skillGet(deadlineOnSignal{Context: t.Context(), expired: expired}, request)
		if err != nil {
			t.Fatalf("stalled download: %v", err)
		}
		encoded, _ = json.Marshal(stalled.StructuredContent)
		got := decodeSkillGet(t, encoded)
		if stalled.IsError || got.Remote == nil || got.Remote.Cache != remoteCacheOffline || got.Local.Path != old.Local.Path {
			t.Fatalf("a stalled download should fall back to the cached copy: %s", encoded)
		}

		// The agent host cancels the call in the middle of the download.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			select {
			case <-front.stalled:
			case <-ctx.Done():
			}
			cancel()
		}()
		cancelled, err := proxied.proxy.skillGet(ctx, request)
		if !errors.Is(err, context.Canceled) || cancelled != nil {
			t.Fatalf("a cancelled download = %+v, %v; want the cancellation", cancelled, err)
		}
		if versions := proxied.proxy.cache.Versions(proxied.proxy.hubID, id); len(versions) != 1 {
			t.Fatalf("%d cached versions after the failed downloads, want only the first one", len(versions))
		}
	})
}

// TestNetworkListenerReachesNoCurationTool calls every tool the full local
// server registers, through the listener and through the proxy: only the
// three runtime tools answer; every other name is an unknown tool. Paths of
// the web UI are not served by the listener either.
func TestNetworkListenerReachesNoCurationTool(t *testing.T) {
	root := newBundleHubFixture(t)
	full := connectInMemoryServer(t, root)
	listed, err := full.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTools := []string{"skill_feedback", "skill_get", "skill_resolve"}
	var curation []string
	for _, tool := range listed.Tools {
		if !slices.Contains(runtimeTools, tool.Name) {
			curation = append(curation, tool.Name)
		}
	}
	if len(curation) < 10 {
		t.Fatalf("the full server lists only %d tools besides the runtime ones: %v", len(curation), curation)
	}

	hub := startHTTPTestHub(t, root)
	direct := connectHTTPClient(t, hub.endpoint, hub.token)
	proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), nil)
	for name, session := range map[string]*mcp.ClientSession{"listener": direct, "proxy": proxied.session} {
		tools, err := session.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tool := range tools.Tools {
			names = append(names, tool.Name)
		}
		slices.Sort(names)
		if !slices.Equal(names, runtimeTools) {
			t.Fatalf("%s lists %v", name, names)
		}
		for _, tool := range curation {
			_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{}})
			if err == nil || !strings.Contains(err.Error(), "unknown tool") {
				t.Errorf("%s: %s = %v, want unknown tool", name, tool, err)
			}
		}
	}
	for _, path := range []string{"/", "/api/status", "/api/skills", "/mcp/curation", "/index.html"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.baseURL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+hub.token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s on the listener = %d, want 404", path, response.StatusCode)
		}
	}
}

// TestOneMachineStdioServerOpensNoPortAndStartsNoProcess pins the one-machine
// way of working: `skillhub mcp serve` over stdio holds no network socket,
// starts no child process and creates no hub token. The registrations
// `connect --workspace` writes are pinned byte for byte by
// TestConnectWorkspaceOutputIsPinned in the cli package.
func TestOneMachineStdioServerOpensNoPortAndStartsNoProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads the process's sockets and children from /proc")
	}
	binary := buildSkillHub(t)
	root := newBundleHubFixture(t)
	served := startMemoryServe(t, binary, root, nil)
	resolveFor(t, served.session, remoteBenchTaskText(t, root, "test-audit"), "REQ-one-machine")
	if _, encoded := toolCall(t, served.session, "skill_get", map[string]any{"skill_id": "test-audit"}); !bytes.Contains(encoded, []byte(`"path"`)) {
		t.Fatalf("local skill_get = %s", encoded)
	}
	pid := served.command.Process.Pid
	descriptors, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range descriptors {
		target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, descriptor.Name()))
		if err == nil && strings.HasPrefix(target, "socket:") {
			t.Fatalf("the stdio server holds a socket (fd %s -> %s)", descriptor.Name(), target)
		}
	}
	tasks, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		children, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, task.Name()))
		if err != nil {
			t.Skipf("this kernel does not list child processes in /proc: %v", err)
		}
		if strings.TrimSpace(string(children)) != "" {
			t.Fatalf("the stdio server started child processes: %s", children)
		}
	}
	served.close(t)
	if _, err := os.Stat(hubtoken.DefaultPath(root)); !os.IsNotExist(err) {
		t.Fatalf("the stdio server created a hub token: %v", err)
	}
}

// TestHubServesLocalStdioAndRemoteProxiesAtOnce runs machine B's two ways of
// serving side by side on one workspace: stdio servers for B's own agents and
// the network listener for two proxies of machine A, all calling
// skill_resolve at the same time. Every call writes its telemetry events; none
// is lost. The stdio servers and the proxies are separate processes; the
// listener runs in the test process, so it stops cleanly on every system
// (a separate process could only be killed on Windows, losing its last
// telemetry batch).
func TestHubServesLocalStdioAndRemoteProxiesAtOnce(t *testing.T) {
	const (
		localServers = 3
		proxies      = 2
		calls        = 25
		skills       = 8
	)
	binary := buildSkillHub(t)
	root := newMemoryWorkspace(t, binary, skills)
	hub := startHTTPTestHub(t, root)

	sessions := make([]*mcp.ClientSession, 0, localServers+proxies)
	var locals []*memoryServe
	var remotes []*proxyProcess
	for range localServers {
		served := startMemoryServe(t, binary, root, nil)
		locals = append(locals, served)
		sessions = append(sessions, served.session)
	}
	for range proxies {
		proxied := startProxyProcess(t, proxyCommand(binary, hub.baseURL, hub.tokenFile, append(os.Environ(), "XDG_STATE_HOME="+t.TempDir())))
		remotes = append(remotes, proxied)
		sessions = append(sessions, proxied.session)
	}

	expected := make([]int, len(sessions))
	failures := make([]error, len(sessions))
	var workers sync.WaitGroup
	for index, session := range sessions {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for call := range calls {
				<-ticker.C
				_, events, err := resolveFixtureSkill(t.Context(), session, skills, fmt.Sprintf("two-way-%d-%d", index, call), index*31+call)
				if err != nil {
					failures[index] = err
					return
				}
				expected[index] += events
			}
		}()
	}
	workers.Wait()
	for index, err := range failures {
		if err != nil {
			t.Fatalf("client %d: %v", index, err)
		}
	}
	for _, served := range locals {
		served.close(t)
	}
	for _, proxied := range remotes {
		proxied.close(t)
	}
	hub.stop(t)

	total := 0
	for _, count := range expected {
		total += count
	}
	if written := countTelemetryEvents(t, root); written != total {
		t.Fatalf("telemetry wrote %d resolution events, want %d (stdio and remote calls at once)", written, total)
	}
	// The same event kinds countTelemetryEvents counts.
	counted := []string{telemetry.EventResolutionCompleted, telemetry.EventResolutionRecommended, telemetry.EventResolutionFailed}
	remote := 0
	for _, event := range recentRawEvents(t, openHubRecorder(t, root)) {
		if slices.Contains(counted, event.Type) && strings.HasPrefix(event.Client.Name, telemetry.ClientRemotePrefix) {
			remote++
		}
	}
	remoteExpected := 0
	for _, count := range expected[localServers:] {
		remoteExpected += count
	}
	if remote != remoteExpected {
		t.Fatalf("%d resolution events carry the remote label, want %d", remote, remoteExpected)
	}
}

// TestRemoteTimingScriptTimesAFetchAndACachedRepeat runs the script a user
// runs on machine A to time a real hub, against a listener on loopback.
func TestRemoteTimingScriptTimesAFetchAndACachedRepeat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the timing script is a POSIX shell script")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("the timing script needs python3")
	}
	binary := buildSkillHub(t)
	root := newBundleHubFixture(t)
	hub := startHubProcess(t, binary, root, remoteBenchEnv())
	token, err := hubtoken.Read(hub.tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "remote-timing.sh"))
	if err != nil {
		t.Fatal(err)
	}
	stateDir, scratch := t.TempDir(), t.TempDir()
	command := exec.Command("sh", script, "--skill", "herdr-cook-plan", "--task", remoteBenchTaskText(t, root, "herdr-cook-plan"),
		"--hub", hub.url, "--token-file", hub.tokenFile)
	command.Dir = t.TempDir()
	command.Env = append(remoteBenchEnv(), "SKILLHUB_BIN="+binary, "XDG_STATE_HOME="+stateDir, "TMPDIR="+scratch)
	output, err := command.CombinedOutput()
	text := string(output)
	if err != nil {
		t.Fatalf("remote-timing.sh: %v\n%s", err, text)
	}
	for _, want := range []string{
		"Hub:     " + hub.url, "recommends herdr-cook-plan",
		"skill_get, first fetch", "downloaded from the hub",
		"skill_get, cached repeat", "served from the cache on this machine",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, token) {
		t.Fatal("the timing script printed the token")
	}
	if entries, _ := os.ReadDir(stateDir); len(entries) != 0 {
		t.Fatalf("the timing script used the user's own cache: %v", entries)
	}
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Fatalf("the timing script left its temporary cache behind: %v", entries)
	}
}
