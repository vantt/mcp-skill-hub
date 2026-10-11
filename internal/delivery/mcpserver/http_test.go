package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
	"golang.org/x/time/rate"
)

const httpTestInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"http-test","version":"1"}}}`

// lockedBuffer collects diagnostics written by the server goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeAddrListener reports a chosen address while accepting on a real
// loopback socket, so the non-loopback rule can be tested without binding a
// reachable interface.
type fakeAddrListener struct {
	net.Listener
	addr net.Addr
}

func (listener fakeAddrListener) Addr() net.Addr { return listener.addr }

type httpTestHub struct {
	root        string
	baseURL     string
	endpoint    string
	token       string
	tokenFile   string
	diagnostics *lockedBuffer
	cancel      context.CancelFunc
	done        chan error
}

func (hub *httpTestHub) stop(t *testing.T) {
	t.Helper()
	hub.cancel()
	select {
	case err := <-hub.done:
		if err != nil {
			t.Fatalf("ServeHTTP returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("ServeHTTP did not stop after cancellation")
	}
}

func startHTTPTestHub(t *testing.T, root string) *httpTestHub {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return startHTTPTestHubOn(t, root, listener, HTTPOptions{})
}

func startHTTPTestHubOn(t *testing.T, root string, listener net.Listener, options HTTPOptions) *httpTestHub {
	t.Helper()
	tokenFile := hubtoken.DefaultPath(t.TempDir())
	if _, err := hubtoken.Ensure(tokenFile); err != nil {
		t.Fatal(err)
	}
	token, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := &lockedBuffer{}
	options.Workspace = root
	options.TokenFile = tokenFile
	options.Diagnostics = diagnostics
	ready := make(chan net.Addr, 1)
	options.Ready = func(address net.Addr) { ready <- address }
	ctx, cancel := context.WithCancel(context.Background())
	hub := &httpTestHub{root: root, token: token, tokenFile: tokenFile, diagnostics: diagnostics, cancel: cancel, done: make(chan error, 1)}
	go func() { hub.done <- ServeHTTP(ctx, listener, options) }()
	select {
	case <-ready:
	case err := <-hub.done:
		cancel()
		t.Fatalf("ServeHTTP stopped before it was ready: %v", err)
	case <-time.After(30 * time.Second):
		cancel()
		t.Fatal("ServeHTTP was not ready in time")
	}
	realAddress := listener.Addr().String()
	if fake, ok := listener.(fakeAddrListener); ok {
		realAddress = fake.Listener.Addr().String()
	}
	hub.baseURL = "http://" + realAddress
	hub.endpoint = hub.baseURL + HTTPEndpointPath
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			cancel()
			select {
			case <-hub.done:
			case <-time.After(15 * time.Second):
			}
		}
	})
	hub.cancel = func() { stopped = true; cancel() }
	return hub
}

// bearerTransport adds the hub token to every request, the way a client
// proxy will.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}

func connectHTTPClient(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearerTransport{token: token, base: http.DefaultTransport}},
		MaxRetries: -1,
	}
	session, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatalf("connect over HTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func postInitialize(t *testing.T, endpoint string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(httpTestInitialize))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	return response
}

func resolveArguments(description string) map[string]any {
	return map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-http-test",
		"task":           map[string]any{"description": description, "scope": "multi_step"},
		"operation":      "review",
	}
}

func TestHTTPServesRuntimeToolsOnlyWithToken(t *testing.T) {
	hub := startHTTPTestHub(t, newMCPWorkspace(t))

	t.Run("missing, wrong, or URL token is rejected", func(t *testing.T) {
		cases := map[string]func(*http.Request){
			"no header":      nil,
			"wrong token":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("0", 64)) },
			"basic scheme":   func(r *http.Request) { r.Header.Set("Authorization", "Basic "+hub.token) },
			"token in query": func(r *http.Request) { r.URL.RawQuery = "token=" + hub.token + "&access_token=" + hub.token },
		}
		for name, mutate := range cases {
			response := postInitialize(t, hub.endpoint, mutate)
			if response.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401", name, response.StatusCode)
			}
			if !strings.HasPrefix(response.Header.Get("WWW-Authenticate"), "Bearer") {
				t.Errorf("%s: WWW-Authenticate = %q", name, response.Header.Get("WWW-Authenticate"))
			}
		}
	})

	t.Run("bad Host or Origin is forbidden even with the token", func(t *testing.T) {
		authorized := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+hub.token) }
		badHost := postInitialize(t, hub.endpoint, func(r *http.Request) { authorized(r); r.Host = "rebind.example:80" })
		if badHost.StatusCode != http.StatusForbidden {
			t.Errorf("bad Host status = %d, want 403", badHost.StatusCode)
		}
		badOrigin := postInitialize(t, hub.endpoint, func(r *http.Request) { authorized(r); r.Header.Set("Origin", "http://evil.example") })
		if badOrigin.StatusCode != http.StatusForbidden {
			t.Errorf("bad Origin status = %d, want 403", badOrigin.StatusCode)
		}
		nullOrigin := postInitialize(t, hub.endpoint, func(r *http.Request) { authorized(r); r.Header.Set("Origin", "null") })
		if nullOrigin.StatusCode != http.StatusForbidden {
			t.Errorf("null Origin status = %d, want 403", nullOrigin.StatusCode)
		}
		sameOrigin := postInitialize(t, hub.endpoint, func(r *http.Request) { authorized(r); r.Header.Set("Origin", hub.baseURL) })
		if sameOrigin.StatusCode != http.StatusOK {
			t.Errorf("same Origin status = %d, want 200", sameOrigin.StatusCode)
		}
		otherPath := postInitialize(t, hub.baseURL+"/other", authorized)
		if otherPath.StatusCode != http.StatusNotFound {
			t.Errorf("other path status = %d, want 404", otherPath.StatusCode)
		}
	})

	session := connectHTTPClient(t, hub.endpoint, hub.token)

	t.Run("tools/list shows exactly the runtime tools", func(t *testing.T) {
		listed, err := session.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tool := range listed.Tools {
			names = append(names, tool.Name)
		}
		sort.Strings(names)
		want := []string{"skill_feedback", "skill_get", "skill_resolve"}
		if !slices.Equal(names, want) {
			t.Fatalf("tools over HTTP = %v, want %v", names, want)
		}
	})

	t.Run("a curation tool is unknown", func(t *testing.T) {
		for _, name := range []string{"workspace_rebuild", "skill_create_confirm", "source_import_confirm", "curation_session_record"} {
			_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
			if err == nil || !strings.Contains(err.Error(), "unknown tool") {
				t.Errorf("%s over HTTP = %v, want unknown tool", name, err)
			}
		}
	})

	t.Run("skill_resolve and skill_get work", func(t *testing.T) {
		resolved := callSkillResolve(t, session, resolveArguments("review changed code"))
		if resolved.Resolution.Primary == nil || resolved.Resolution.Primary.ID != "review-skill" {
			t.Fatalf("primary over HTTP = %#v", resolved.Resolution.Primary)
		}
		got := callSkillGet(t, session, "review-skill")
		if got.SkillID != "review-skill" {
			t.Fatalf("skill_get over HTTP = %#v", got)
		}
	})

	hub.stop(t)
	if strings.Contains(hub.diagnostics.String(), hub.token) {
		t.Fatal("the token appeared in server diagnostics")
	}
	preview, err := (app.TelemetryService{}).Preview(context.Background(), hub.root, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events == 0 {
		t.Fatal("no telemetry event was flushed on shutdown")
	}
	if bytes.Contains(preview.JSONL, []byte(hub.token)) {
		t.Fatal("the token appeared in telemetry")
	}
}

func TestHTTPShutdownClosesOpenSessions(t *testing.T) {
	hub := startHTTPTestHub(t, newMCPWorkspace(t))
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	if _, err := session.ListTools(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	// The client keeps a standalone event stream open; shutdown must end it
	// instead of waiting for the shutdown timeout.
	started := time.Now()
	hub.stop(t)
	if elapsed := time.Since(started); elapsed >= httpShutdownTimeout {
		t.Fatalf("shutdown took %s, want under %s", elapsed, httpShutdownTimeout)
	}
}

func TestCheckListenAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:7420", "[::1]:0", "localhost:0", "127.0.0.2:7420"} {
		if err := CheckListenAddress(address, false); err != nil {
			t.Errorf("CheckListenAddress(%q) = %v, want loopback accepted", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:7420", "[::]:7420", ":7420", "192.168.1.5:7420", "100.101.102.103:7420", "hub.example:7420"} {
		if err := CheckListenAddress(address, false); !errors.Is(err, ErrNonLoopbackAddress) {
			t.Errorf("CheckListenAddress(%q, false) = %v, want ErrNonLoopbackAddress", address, err)
		}
	}
	for _, address := range []string{"192.168.1.5:7420", "100.101.102.103:7420", "hub.example:7420"} {
		if err := CheckListenAddress(address, true); err != nil {
			t.Errorf("CheckListenAddress(%q, true) = %v, want accepted", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:7420", "[::]:7420", ":7420"} {
		if err := CheckListenAddress(address, true); !errors.Is(err, ErrWildcardAddress) {
			t.Errorf("CheckListenAddress(%q, true) = %v, want ErrWildcardAddress", address, err)
		}
	}
	for _, address := range []string{"7420", "127.0.0.1", "127.0.0.1:notaport", "127.0.0.1:99999"} {
		err := CheckListenAddress(address, true)
		if err == nil || errors.Is(err, ErrNonLoopbackAddress) {
			t.Errorf("CheckListenAddress(%q) = %v, want an invalid address error", address, err)
		}
	}
}

func TestHTTPNonLoopbackNeedsTrustedPrivateNetwork(t *testing.T) {
	root := newMCPWorkspace(t)
	reported := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 7420}

	refusedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer refusedListener.Close()
	tokenFile := hubtoken.DefaultPath(t.TempDir())
	if _, err := hubtoken.Ensure(tokenFile); err != nil {
		t.Fatal(err)
	}
	err = ServeHTTP(t.Context(), fakeAddrListener{Listener: refusedListener, addr: reported}, HTTPOptions{Workspace: root, TokenFile: tokenFile})
	if !errors.Is(err, ErrNonLoopbackAddress) {
		t.Fatalf("ServeHTTP on a non-loopback address without trust = %v, want ErrNonLoopbackAddress", err)
	}

	trustedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hub := startHTTPTestHubOn(t, root, fakeAddrListener{Listener: trustedListener, addr: reported}, HTTPOptions{TrustPrivateNetwork: true})
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	if _, err := session.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("trusted private network listener does not serve: %v", err)
	}
	hub.stop(t)
}

func TestHTTPAllowHostOnLoopbackListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hub := startHTTPTestHubOn(t, newMCPWorkspace(t), listener, HTTPOptions{AllowHosts: []string{"hub.tailnet.example"}})
	authorized := func(host string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+hub.token); r.Host = host }
	}
	if response := postInitialize(t, hub.endpoint, authorized("hub.tailnet.example:7420")); response.StatusCode != http.StatusOK {
		t.Fatalf("--allow-host name on a loopback listener status = %d, want 200", response.StatusCode)
	}
	if response := postInitialize(t, hub.endpoint, authorized("other.tailnet.example:7420")); response.StatusCode != http.StatusForbidden {
		t.Fatalf("unlisted name status = %d, want 403", response.StatusCode)
	}
	hub.stop(t)
}

func TestHTTPShutdownDrainsRunningRequests(t *testing.T) {
	gate, _, token := newTestGate(t)
	started := make(chan struct{})
	gate.handler = http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	})
	running := make(chan int, 1)
	go func() { running <- gateRequest(gate, token, "127.0.0.1:1", nil).Code }()
	<-started
	gate.beginShutdown()
	if response := gateRequest(gate, token, "127.0.0.1:2", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("a new request during shutdown status = %d, want 503", response.Code)
	}
	gate.drain(5 * time.Second)
	select {
	case code := <-running:
		if code != http.StatusOK {
			t.Fatalf("the running request finished with %d, want 200", code)
		}
	default:
		t.Fatal("drain returned before the running request finished")
	}
}

func TestHTTPRefusesToStartWithoutToken(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	err = ServeHTTP(t.Context(), listener, HTTPOptions{Workspace: newMCPWorkspace(t), TokenFile: hubtoken.DefaultPath(t.TempDir())})
	if err == nil || !strings.Contains(err.Error(), "hub token") {
		t.Fatalf("ServeHTTP without a token file = %v, want a token error", err)
	}
}

// newTestGate builds the request gate around a runtime server without a
// network listener.
func newTestGate(t *testing.T) (*httpGate, *Server, string) {
	t.Helper()
	adapter, server, err := New(newMCPWorkspace(t), nil, ProfileRuntime)
	if err != nil {
		t.Fatal(err)
	}
	tokenFile := hubtoken.DefaultPath(t.TempDir())
	if _, err := hubtoken.Ensure(tokenFile); err != nil {
		t.Fatal(err)
	}
	token, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	listenAddress := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7420}
	gate := newHTTPGate(server, hubtoken.NewSource(tokenFile), allowedHosts(listenAddress, []string{"hub.tailnet.example"}), adapter.logger)
	return gate, adapter, token
}

func gateRequest(gate *httpGate, token, remote string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7420"+HTTPEndpointPath, strings.NewReader(httpTestInitialize))
	request.RemoteAddr = remote
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if mutate != nil {
		mutate(request)
	}
	recorder := httptest.NewRecorder()
	gate.ServeHTTP(recorder, request)
	return recorder
}

func TestHTTPGateHostNames(t *testing.T) {
	gate, _, token := newTestGate(t)
	for _, host := range []string{"127.0.0.1:7420", "localhost:9000", "[::1]:7420", "HUB.tailnet.example.:7420"} {
		response := gateRequest(gate, token, "127.0.0.1:50000", func(r *http.Request) { r.Host = host })
		if response.Code == http.StatusForbidden {
			t.Errorf("Host %q forbidden: %s", host, response.Body.String())
		}
	}
	for _, host := range []string{"evil.example", "hub.tailnet.example.evil", "192.0.2.1:7420"} {
		response := gateRequest(gate, token, "127.0.0.1:50000", func(r *http.Request) { r.Host = host })
		if response.Code != http.StatusForbidden {
			t.Errorf("Host %q status = %d, want 403", host, response.Code)
		}
	}
}

func TestHTTPGateThrottlesFailedAuthentication(t *testing.T) {
	gate, _, token := newTestGate(t)
	for attempt := 0; attempt < httpAuthFailureLimit; attempt++ {
		if response := gateRequest(gate, "wrong-token-wrong-token-wrong-token", "198.51.100.7:1000", nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt, response.Code)
		}
	}
	if response := gateRequest(gate, "wrong-token-wrong-token-wrong-token", "198.51.100.7:1001", nil); response.Code != http.StatusTooManyRequests {
		t.Fatalf("a wrong token after %d failures status = %d, want 429", httpAuthFailureLimit, response.Code)
	}
	// Behind an SSH tunnel every client shares one address, so a client with
	// the valid token must still pass.
	if response := gateRequest(gate, token, "198.51.100.7:1002", nil); response.Code != http.StatusOK {
		t.Fatalf("the valid token from a throttled address status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if strings.Contains(gateRequest(gate, "", "198.51.100.9:1", nil).Body.String(), token) {
		t.Fatal("a rejection echoed the token")
	}
}

func TestHTTPGateRateLimitAndSessionCap(t *testing.T) {
	gate, _, token := newTestGate(t)
	gate.limiter = rate.NewLimiter(rate.Limit(0.001), 1)
	if response := gateRequest(gate, token, "127.0.0.1:1", nil); response.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if response := gateRequest(gate, token, "127.0.0.1:1", nil); response.Code != http.StatusTooManyRequests {
		t.Fatalf("over the rate status = %d, want 429", response.Code)
	}

	gate, _, token = newTestGate(t)
	gate.maxSessions = 1
	if response := gateRequest(gate, token, "127.0.0.1:1", nil); response.Code != http.StatusOK {
		t.Fatalf("first session status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if response := gateRequest(gate, token, "127.0.0.1:1", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("session over the cap status = %d, want 503", response.Code)
	}
	gate.closeSessions()
	if response := gateRequest(gate, token, "127.0.0.1:1", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("request while closing status = %d, want 503", response.Code)
	}
}

func TestHTTPSessionsKeepSeparatePriorResolutions(t *testing.T) {
	gate, adapter, token := newTestGate(t)
	server := httptest.NewServer(gate)
	// Cleanups run last-in first-out: sessions close before the server stops,
	// so Close does not wait on an open event stream.
	t.Cleanup(server.Close)
	t.Cleanup(gate.closeSessions)
	first := connectHTTPClient(t, server.URL+HTTPEndpointPath, token)
	second := connectHTTPClient(t, server.URL+HTTPEndpointPath, token)
	firstResolution := callSkillResolve(t, first, resolveArguments("review changed code")).Resolution.ResolutionID
	secondResolution := callSkillResolve(t, second, resolveArguments("review the diff")).Resolution.ResolutionID

	adapter.tracker.mu.Lock()
	defer adapter.tracker.mu.Unlock()
	if len(adapter.tracker.sessions) != 2 {
		t.Fatalf("tracked sessions = %d, want 2", len(adapter.tracker.sessions))
	}
	for _, state := range adapter.tracker.sessions {
		if len(state.resolutions) != 1 {
			t.Fatalf("a session holds %d resolutions, want only its own", len(state.resolutions))
		}
		if id := state.resolutions[0].ResolutionID; id != firstResolution && id != secondResolution {
			t.Fatalf("unexpected resolution %s", id)
		}
	}
}

// TestHTTPLoopbackResolveLatency records the loopback round trip of
// skill_resolve for the remote benchmark. It reports numbers and fails only
// on errors.
func TestHTTPLoopbackResolveLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("latency sample skipped in short mode")
	}
	hub := startHTTPTestHub(t, newMCPWorkspace(t))
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	const calls = 200
	durations := make([]time.Duration, 0, calls)
	for index := 0; index < calls; index++ {
		started := time.Now()
		callSkillResolve(t, session, resolveArguments(fmt.Sprintf("review changed code %d", index)))
		durations = append(durations, time.Since(started))
	}
	slices.Sort(durations)
	t.Logf("loopback HTTP skill_resolve over %d calls: median %s, p95 %s, max %s", calls, durations[calls/2], durations[calls*95/100], durations[calls-1])
	hub.stop(t)
}
