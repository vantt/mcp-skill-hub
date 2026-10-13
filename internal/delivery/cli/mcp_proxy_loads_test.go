package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/mcpserver"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

// hubLink sits between the proxy processes and the listener. It counts the
// bundle requests and their content bytes, delays every answer by one round
// trip, and can go silent: from then on no request is ever answered.
type hubLink struct {
	server *httptest.Server
	silent chan struct{}

	mu          sync.Mutex
	bundles     int
	statuses    []int
	bundleBytes int64
	once        sync.Once
}

const hubLinkRTT = 250 * time.Millisecond

type countingWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (writer *countingWriter) WriteHeader(status int) {
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *countingWriter) Write(data []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	n, err := writer.ResponseWriter.Write(data)
	writer.bytes += int64(n)
	return n, err
}

func (writer *countingWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func startHubLink(t *testing.T, hubURL string) *hubLink {
	t.Helper()
	target, err := url.Parse(hubURL)
	if err != nil {
		t.Fatal(err)
	}
	link := &hubLink{silent: make(chan struct{})}
	relay := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) { request.SetURL(target) }, FlushInterval: -1}
	link.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-link.silent:
			<-request.Context().Done()
			return
		case <-time.After(hubLinkRTT):
		case <-request.Context().Done():
			return
		}
		counting := &countingWriter{ResponseWriter: writer}
		relay.ServeHTTP(counting, request)
		if request.URL.Path == mcpserver.HTTPBundlePath {
			link.mu.Lock()
			link.bundles++
			link.statuses = append(link.statuses, counting.status)
			link.bundleBytes += counting.bytes
			link.mu.Unlock()
		}
	}))
	t.Cleanup(func() {
		link.goSilent()
		link.server.CloseClientConnections()
		link.server.Close()
	})
	return link
}

func (link *hubLink) goSilent() { link.once.Do(func() { close(link.silent) }) }

func (link *hubLink) count() (bundles int, lastStatus int, bundleBytes int64) {
	link.mu.Lock()
	defer link.mu.Unlock()
	if len(link.statuses) > 0 {
		lastStatus = link.statuses[len(link.statuses)-1]
	}
	return link.bundles, lastStatus, link.bundleBytes
}

// proxyProcess is a `skillhub mcp proxy` subprocess with an agent host
// session on its stdio.
type proxyProcess struct {
	session *mcp.ClientSession
	command *exec.Cmd
	stderr  *syncBuffer
}

func startProxyProcess(t *testing.T, hubURL, tokenFile string) *proxyProcess {
	t.Helper()
	command := exec.Command(os.Args[0], "mcp", "proxy", "--hub", hubURL, "--token-file", tokenFile)
	command.Env = append(os.Environ(), cliHelperEnv+"=1")
	stderr := &syncBuffer{}
	command.Stderr = stderr
	session, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: command, TerminateDuration: 10 * time.Second}, nil)
	if err != nil {
		t.Fatalf("start the proxy: %v; stderr=%s", err, stderr.String())
	}
	process := &proxyProcess{session: session, command: command, stderr: stderr}
	t.Cleanup(func() { process.stop(t) })
	return process
}

func (process *proxyProcess) stop(t *testing.T) {
	t.Helper()
	if process.command.ProcessState != nil {
		return
	}
	if err := process.session.Close(); err != nil {
		t.Errorf("close the proxy: %v; stderr=%s", err, process.stderr.String())
	}
}

func (process *proxyProcess) call(t *testing.T, name string, arguments map[string]any) json.RawMessage {
	t.Helper()
	result, err := process.session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("%s: %+v, %v; stderr=%s", name, result, err, process.stderr.String())
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	return encoded
}

func (process *proxyProcess) resolveAndGet(t *testing.T) (cache string, took time.Duration) {
	t.Helper()
	process.call(t, "skill_resolve", map[string]any{
		"schema_version": "1", "request_id": "REQ-proxy-loads",
		"task": map[string]any{"description": "verify the remote check fixture", "scope": "multi_step"},
	})
	started := time.Now()
	encoded := process.call(t, "skill_get", map[string]any{"skill_id": "check-skill"})
	took = time.Since(started)
	var outcome struct {
		Result struct {
			Remote struct {
				Cache string `json:"cache"`
			} `json:"remote"`
		} `json:"result"`
	}
	if err := json.Unmarshal(encoded, &outcome); err != nil {
		t.Fatal(err)
	}
	return outcome.Result.Remote.Cache, took
}

func hubBundleLoads(t *testing.T, ws string) []telemetry.Event {
	t.Helper()
	recorder, err := (app.TelemetryService{}).Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recorder.Close(context.Background()) }()
	now := time.Now().UTC()
	events, err := recorder.RawEvents(t.Context(), now.Add(-24*time.Hour).Format("2006-01-02"), now.Add(24*time.Hour).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	var loads []telemetry.Event
	for _, event := range events {
		if event.Type == telemetry.EventSkillLoaded && event.Payload["surface"] == "skill_bundle" {
			loads = append(loads, event)
		}
	}
	return loads
}

// Real proxy processes against a real listener: the hub labels the proxy's
// session with the host the agent named, counts a bundle fetch and a later
// cache hit as one load each in their own sessions, and the cache hit neither
// waits for the hub nor transfers content, also when the hub stops answering.
func TestMCPProxyProcessCountsLoadsOnTheHub(t *testing.T) {
	ws := initTestWorkspace(t)
	addCheckSkill(t, ws)
	listener := startListener(t, ws)
	tokenFile := hubtoken.DefaultPath(ws)
	token, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	link := startHubLink(t, listener)

	first := startProxyProcess(t, link.server.URL, tokenFile)
	if cache, _ := first.resolveAndGet(t); cache != "fetched" {
		t.Fatalf("first skill_get: cache %q, want fetched", cache)
	}
	first.stop(t)

	second := startProxyProcess(t, link.server.URL, tokenFile)
	bundlesBefore, _, bytesBefore := link.count()
	cache, hit := second.resolveAndGet(t)
	if cache != "hit" {
		t.Fatalf("second skill_get: cache %q, want hit", cache)
	}
	if hit > proxyHitBudget {
		t.Errorf("cache hit through the proxy process took %s, budget %s (round trip to the hub %s)", hit, proxyHitBudget, hubLinkRTT)
	}
	started := time.Now()
	for {
		if loads := hubBundleLoads(t, ws); len(loads) >= 2 {
			break
		}
		if time.Since(started) > 10*time.Second {
			t.Fatal("the hub recorded no load for the cache hit")
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Logf("cache hit %s; the hub recorded its load %s later", hit, time.Since(started))
	bundles, status, contentBytes := link.count()
	if bundles-bundlesBefore != 1 || status != http.StatusNotModified || contentBytes != bytesBefore {
		t.Fatalf("cache hit sent %d bundle requests, last status %d, %d content bytes; want one 304 with none", bundles-bundlesBefore, status, contentBytes-bytesBefore)
	}
	encoded := second.call(t, "skill_get", map[string]any{"skill_id": "check-skill"})
	if !strings.Contains(string(encoded), `"cache":"hit"`) {
		t.Fatalf("repeat skill_get = %.300s", encoded)
	}
	second.stop(t)
	if after, _, _ := link.count(); after != bundles {
		t.Fatalf("a version already counted in the session was confirmed again (%d more bundle requests)", after-bundles)
	}

	// The hub stops answering: the cache hit stays fast and works.
	third := startProxyProcess(t, link.server.URL, tokenFile)
	third.call(t, "skill_resolve", map[string]any{
		"schema_version": "1", "request_id": "REQ-proxy-silent",
		"task": map[string]any{"description": "verify the remote check fixture", "scope": "multi_step"},
	})
	link.goSilent()
	for range 3 {
		started := time.Now()
		encoded := third.call(t, "skill_get", map[string]any{"skill_id": "check-skill"})
		if took := time.Since(started); took > proxyHitBudget || !strings.Contains(string(encoded), `"cache":"hit"`) {
			t.Fatalf("cache hit with a silent hub took %s: %.300s", took, encoded)
		}
	}
	if strings.Contains(third.stderr.String(), "cannot be reached") {
		t.Fatalf("a background confirmation warned the user: %s", third.stderr.String())
	}
	third.stop(t)
	if third.command.ProcessState == nil || third.command.ProcessState.ExitCode() != 0 {
		t.Fatalf("proxy exit state with a silent hub: %v; stderr=%s", third.command.ProcessState, third.stderr.String())
	}

	loads := hubBundleLoads(t, ws)
	sessions := map[string]bool{}
	for _, load := range loads {
		sessions[load.SessionIDHash] = true
		if load.Client != (telemetry.Client{Name: "remote:claude-code", Version: "1"}) {
			t.Errorf("proxy load client = %+v, want remote:claude-code", load.Client)
		}
		if load.Payload["attribution"] != "recommended" {
			t.Errorf("proxy load attribution = %v, want recommended", load.Payload["attribution"])
		}
	}
	if len(loads) != 2 || len(sessions) != 2 {
		t.Fatalf("hub recorded %d bundle loads in %d sessions, want one fetch and one cache hit in two sessions", len(loads), len(sessions))
	}

	// The token reached no file of the hub but its own token file. (Client
	// addresses are scanned for in the listener's own tests, which control
	// the address a client dials from.)
	err = filepath.WalkDir(ws, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() || path == tokenFile {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, []byte(token)) {
			t.Errorf("%s contains the token", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
