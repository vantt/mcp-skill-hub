package mcpserver

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// sessionHost is the clientInfo every test client reports.
var sessionHost = &mcp.Implementation{Name: "claude-code", Version: "1"}

// connectLocalSession opens an in-process session on server, the way a stdio
// host connects, reporting sessionHost as its clientInfo.
func connectLocalSession(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := mcp.NewClient(sessionHost, nil).Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// recordingListener remembers the address of every accepted connection, which
// is the client address the hub sees.
type recordingListener struct {
	net.Listener
	mu      sync.Mutex
	remotes []string
}

func (listener *recordingListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err == nil {
		listener.mu.Lock()
		listener.remotes = append(listener.remotes, conn.RemoteAddr().String())
		listener.mu.Unlock()
	}
	return conn, err
}

func (listener *recordingListener) seen() []string {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	return slices.Clone(listener.remotes)
}

// distinctClientIP returns a loopback address other than 127.0.0.1 to dial
// from, so a scan for the client address cannot match an unrelated string.
// Systems that configure only 127.0.0.1 (macOS) get nil, the default.
func distinctClientIP(t *testing.T, target string) net.IP {
	t.Helper()
	candidate := net.ParseIP("127.0.0.77")
	conn, err := (&net.Dialer{LocalAddr: &net.TCPAddr{IP: candidate}, Timeout: time.Second}).Dial("tcp", target)
	if err != nil {
		return nil
	}
	_ = conn.Close()
	return candidate
}

// remoteHTTPClient sends requests from localIP (nil for the default) and adds
// token as a bearer credential when token is set.
func remoteHTTPClient(t *testing.T, localIP net.IP, token string) *http.Client {
	t.Helper()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if localIP != nil {
		dialer.LocalAddr = &net.TCPAddr{IP: localIP}
	}
	base := &http.Transport{DialContext: dialer.DialContext}
	t.Cleanup(base.CloseIdleConnections)
	if token == "" {
		return &http.Client{Transport: base}
	}
	return &http.Client{Transport: bearerTransport{token: token, base: base}}
}

func connectRemoteSession(t *testing.T, hub *httpTestHub, localIP net.IP) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{Endpoint: hub.endpoint, HTTPClient: remoteHTTPClient(t, localIP, hub.token), MaxRetries: -1}
	session, err := mcp.NewClient(sessionHost, nil).Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatalf("connect over HTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func resolveTask(description, operation string, prior string) map[string]any {
	arguments := map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-remote-session",
		"task":           map[string]any{"description": description, "scope": "multi_step"},
		"operation":      operation,
	}
	if prior != "" {
		arguments["prior"] = map[string]any{"resolution_id": prior, "kind": "rejected", "context_revision": 1}
	}
	return arguments
}

// gapTask matches no skill of the test workspace.
const gapTask = "translate the release notes into french"

// sessionScenario holds the resolution IDs produced by runSessionScenario.
type sessionScenario struct {
	firstResolution, reformulation, secondRejected, thirdRejected, gap string
}

// runSessionScenario drives two concurrent sessions and one reconnect through
// the same calls, for a remote and a local server alike:
//   - second loads a skill before any resolution (an unsolicited load);
//   - first resolves, then rejects its own resolution (verified_reformulation);
//   - second rejects first's resolution, which it never saw (rejected);
//   - first resolves gapTask twice (repeated_gap), second once (no case);
//   - first closes; third, a new session, rejects first's resolution and
//     resolves gapTask once, and must start with empty memory.
func runSessionScenario(t *testing.T, first, second *mcp.ClientSession, reconnect func() *mcp.ClientSession) sessionScenario {
	t.Helper()
	var scenario sessionScenario
	callSkillGet(t, second, "review-skill")
	if _, err := first.ListTools(t.Context(), nil); err != nil {
		t.Fatal(err)
	}

	scenario.firstResolution = callSkillResolve(t, first, resolveTask("review changed code in module alpha", "review", "")).Resolution.ResolutionID
	scenario.reformulation = callSkillResolve(t, first, resolveTask("review changed code in module alpha and its tests", "review", scenario.firstResolution)).Resolution.ResolutionID
	scenario.secondRejected = callSkillResolve(t, second, resolveTask("review changed code in module beta", "review", scenario.firstResolution)).Resolution.ResolutionID
	callSkillGet(t, first, "review-skill")

	for _, session := range []*mcp.ClientSession{first, first, second} {
		gap := callSkillResolve(t, session, resolveTask(gapTask, "document", "")).Resolution
		if gap.Status != resolverpkg.StatusNoSkill {
			t.Fatalf("gap task status = %s, want %s", gap.Status, resolverpkg.StatusNoSkill)
		}
		scenario.gap = gap.ResolutionID
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close the first session: %v", err)
	}
	third := reconnect()
	scenario.thirdRejected = callSkillResolve(t, third, resolveTask("review changed code in module gamma", "review", scenario.firstResolution)).Resolution.ResolutionID
	callSkillResolve(t, third, resolveTask(gapTask, "document", ""))
	return scenario
}

// scenarioSessions checks the cases of runSessionScenario and returns the
// session hashes of the first, second, and third session.
func scenarioSessions(t *testing.T, scenario sessionScenario, cases []telemetry.CaseRecord) (first, second, third string) {
	t.Helper()
	byResolution := map[string][]telemetry.CaseRecord{}
	for _, record := range cases {
		byResolution[record.ResolutionID] = append(byResolution[record.ResolutionID], record)
	}
	single := func(resolutionID, kind string, priorVerified bool) telemetry.CaseRecord {
		t.Helper()
		records := byResolution[resolutionID]
		if len(records) != 1 || records[0].Kind != kind || records[0].PriorVerified != priorVerified || records[0].SessionHash == "" {
			t.Fatalf("cases for %s = %+v, want one %s case with prior_verified %v and a session hash", resolutionID, records, kind, priorVerified)
		}
		return records[0]
	}
	first = single(scenario.reformulation, "verified_reformulation", true).SessionHash
	second = single(scenario.secondRejected, "rejected", false).SessionHash
	third = single(scenario.thirdRejected, "rejected", false).SessionHash
	if first == second || first == third || second == third {
		t.Fatalf("sessions share a hash: first %s, second %s, third %s", first, second, third)
	}
	if gap := single(scenario.gap, "repeated_gap", false); gap.SessionHash != first {
		t.Fatalf("repeated_gap belongs to session %s, want the first session %s", gap.SessionHash, first)
	}
	if len(cases) != 4 {
		t.Fatalf("cases = %d, want 4 (reformulation, two rejections, one repeated gap): %+v", len(cases), cases)
	}
	return first, second, third
}

func caseKinds(cases []telemetry.CaseRecord) []string {
	kinds := make([]string, 0, len(cases))
	for _, record := range cases {
		kinds = append(kinds, record.Kind)
	}
	slices.Sort(kinds)
	return kinds
}

func recentRawEvents(t *testing.T, recorder *telemetry.Recorder) []telemetry.Event {
	t.Helper()
	now := time.Now().UTC()
	events, err := recorder.RawEvents(t.Context(), now.Add(-24*time.Hour).Format("2006-01-02"), now.Add(24*time.Hour).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func openHubRecorder(t *testing.T, root string) *telemetry.Recorder {
	t.Helper()
	recorder, err := (app.TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close(context.Background()) })
	return recorder
}

// assertClientLabels checks the client of every event of the scenario
// sessions: the resolutions, the loads, and the tools/list size metric.
func assertClientLabels(t *testing.T, events []telemetry.Event, sessions []string, want, unsolicited telemetry.Client) {
	t.Helper()
	var resolutions, loads, unsolicitedLoads, toolsList int
	for _, event := range events {
		if event.Type == telemetry.EventServerMetric && event.Payload["metric_name"] == "tools_list_bytes" {
			toolsList++
			if event.Client != want {
				t.Errorf("tools_list_bytes client = %+v, want %+v", event.Client, want)
			}
			continue
		}
		if !slices.Contains(sessions, event.SessionIDHash) {
			continue
		}
		switch {
		case event.Type == telemetry.EventSkillLoaded && event.Payload["attribution"] == "unsolicited":
			unsolicitedLoads++
			if event.Client != unsolicited {
				t.Errorf("unsolicited load client = %+v, want %+v", event.Client, unsolicited)
			}
		case event.Type == telemetry.EventSkillLoaded:
			loads++
			if event.Client != want {
				t.Errorf("load client = %+v, want %+v", event.Client, want)
			}
		case strings.HasPrefix(event.Type, "resolution."):
			resolutions++
			if event.Client != want {
				t.Errorf("%s client = %+v, want %+v", event.Type, event.Client, want)
			}
		}
	}
	if resolutions == 0 || loads == 0 || unsolicitedLoads == 0 || toolsList == 0 {
		t.Fatalf("events seen: %d resolutions, %d loads, %d unsolicited loads, %d tools/list metrics; want each at least one", resolutions, loads, unsolicitedLoads, toolsList)
	}
}

func TestRemoteSessionsKeepSeparateChainsAndCases(t *testing.T) {
	root := newMCPWorkspace(t)
	if err := telemetry.SetCaseJournalEnabled(root, true); err != nil {
		t.Fatal(err)
	}
	hub := startHTTPTestHub(t, root)
	first := connectHTTPClient(t, hub.endpoint, hub.token)
	second := connectHTTPClient(t, hub.endpoint, hub.token)
	scenario := runSessionScenario(t, first, second, func() *mcp.ClientSession {
		return connectHTTPClient(t, hub.endpoint, hub.token)
	})
	hub.stop(t)

	recorder := openHubRecorder(t, root)
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	firstHash, secondHash, thirdHash := scenarioSessions(t, scenario, cases)
	remoteHost := telemetry.Client{Name: "remote:claude-code", Version: "1"}
	for _, record := range cases {
		if record.Client != remoteHost {
			t.Errorf("%s case client = %+v, want %+v", record.Kind, record.Client, remoteHost)
		}
	}
	assertClientLabels(t, recentRawEvents(t, recorder), []string{firstHash, secondHash, thirdHash}, remoteHost, remoteHost)
}

// The same scenario on a local (stdio-style) server records the same case
// kinds per session, and its client labels stay exactly as before: the bare
// host name, and "skillhub" for a load without a prior resolution.
func TestLocalSessionsRecordTheSameCasesWithUnchangedLabels(t *testing.T) {
	root := newMCPWorkspace(t)
	if err := telemetry.SetCaseJournalEnabled(root, true); err != nil {
		t.Fatal(err)
	}
	adapter, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.remote {
		t.Fatal("a server built by New is labelled remote")
	}
	recorder := openHubRecorder(t, root)
	adapter.useRecorder(recorder)
	scenario := runSessionScenario(t, connectLocalSession(t, server), connectLocalSession(t, server), func() *mcp.ClientSession {
		return connectLocalSession(t, server)
	})
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	firstHash, secondHash, thirdHash := scenarioSessions(t, scenario, cases)
	localHost := telemetry.Client{Name: "claude-code", Version: "1"}
	for _, record := range cases {
		if record.Client != localHost {
			t.Errorf("%s case client = %+v, want %+v", record.Kind, record.Client, localHost)
		}
	}
	if want := []string{"rejected", "rejected", "repeated_gap", "verified_reformulation"}; !slices.Equal(caseKinds(cases), want) {
		t.Fatalf("local case kinds = %v, want %v", caseKinds(cases), want)
	}
	assertClientLabels(t, recentRawEvents(t, recorder), []string{firstHash, secondHash, thirdHash}, localHost, telemetry.Client{Name: "skillhub"})
}

func TestRemoteCaseJournalStaysOptIn(t *testing.T) {
	root := newMCPWorkspace(t)
	hub := startHTTPTestHub(t, root)
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	first := callSkillResolve(t, session, resolveTask("review changed code in module alpha", "review", "")).Resolution.ResolutionID
	callSkillResolve(t, session, resolveTask("review changed code in module alpha and its tests", "review", first))
	hub.stop(t)

	recorder := openHubRecorder(t, root)
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("a remote session wrote %d cases while the case journal is off", len(cases))
	}
	if len(recentRawEvents(t, recorder)) == 0 {
		t.Fatal("no telemetry event was recorded for the remote session")
	}
}

// filesContaining lists the files below root whose bytes contain any needle.
func filesContaining(t *testing.T, root string, needles [][]byte) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, needle := range needles {
			if bytes.Contains(content, needle) {
				found = append(found, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// After remote calls, failed authentications included, neither the bearer
// token nor the client address appears in the telemetry store, the case
// journal, the server diagnostics, or any other file of the workspace and the
// isolated home; remote task text is redacted on the hub exactly as local text.
func TestRemoteCallsStoreNoTokenAndNoClientAddress(t *testing.T) {
	root := newMCPWorkspace(t)
	if err := telemetry.SetCaseJournalEnabled(root, true); err != nil {
		t.Fatal(err)
	}
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &recordingListener{Listener: inner}
	hub := startHTTPTestHubOn(t, root, listener, HTTPOptions{})
	clientIP := distinctClientIP(t, inner.Addr().String())

	unauthenticated := remoteHTTPClient(t, clientIP, "")
	failures := map[string]func(*http.Request){
		"no token":       func(*http.Request) {},
		"wrong token":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("f", 64)) },
		"basic scheme":   func(r *http.Request) { r.Header.Set("Authorization", "Basic "+hub.token) },
		"token in query": func(r *http.Request) { r.URL.RawQuery = "token=" + hub.token },
	}
	for name, mutate := range failures {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, hub.endpoint, strings.NewReader(httpTestInitialize))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		mutate(request)
		response, err := unauthenticated.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, want 401", name, response.StatusCode)
		}
	}

	rawTask := "review changed code in module alpha; api_key=sk-live-0123456789abcdefghij and mail ops@example.com"
	session := connectRemoteSession(t, hub, clientIP)
	first := callSkillResolve(t, session, resolveTask("review changed code in module alpha", "review", "")).Resolution.ResolutionID
	reformulation := callSkillResolve(t, session, resolveTask(rawTask, "review", first)).Resolution.ResolutionID
	callSkillGet(t, session, "review-skill")
	// Bundle requests, delivered and not modified, go through the same scan.
	bundleClient := remoteHTTPClient(t, clientIP, hub.token)
	etag := ""
	for _, want := range []int{http.StatusOK, http.StatusNotModified} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.baseURL+HTTPBundlePath+"?skill=review-skill", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Mcp-Session-Id", session.ID())
		if etag != "" {
			request.Header.Set("If-None-Match", etag)
		}
		response, err := bundleClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("bundle request status = %d, want %d", response.StatusCode, want)
		}
		etag = response.Header.Get("ETag")
	}
	hub.stop(t)

	recorder := openHubRecorder(t, root)
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	var reformulationCase *telemetry.CaseRecord
	for index := range cases {
		if cases[index].ResolutionID == reformulation {
			reformulationCase = &cases[index]
		}
	}
	if reformulationCase == nil {
		t.Fatalf("no case for the remote reformulation: %+v", cases)
	}
	if got, want := reformulationCase.Task["description"], telemetry.NewRedactor(root).Redact(rawTask); got != want {
		t.Fatalf("remote case task = %q, want the hub redaction %q", got, want)
	}
	if loads := bundleLoads(recentRawEvents(t, recorder)); len(loads) != 1 {
		t.Fatalf("recorded %d bundle loads for one delivered and one confirmed bundle, want 1", len(loads))
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}

	remotes := listener.seen()
	if len(remotes) == 0 {
		t.Fatal("the listener saw no client connection")
	}
	needles := [][]byte{[]byte(hub.token), []byte("sk-live-0123456789abcdefghij"), []byte("ops@example.com")}
	for _, remote := range remotes {
		needles = append(needles, []byte(remote))
	}
	if clientIP != nil {
		needles = append(needles, []byte(clientIP.String()))
	} else {
		t.Log("only 127.0.0.1 is available for clients here; scanning for the full client address with its port")
	}
	scanned := []string{root}
	if home := os.Getenv("HOME"); home != "" {
		// TestMain puts HOME and every XDG directory under one temporary root.
		scanned = append(scanned, filepath.Dir(home))
	}
	for _, directory := range scanned {
		if found := filesContaining(t, directory, needles); len(found) > 0 {
			t.Fatalf("files contain the token, a raw secret, or the client address: %v", found)
		}
	}
	diagnostics := []byte(hub.diagnostics.String())
	for _, needle := range needles {
		if bytes.Contains(diagnostics, needle) {
			t.Fatal("the server diagnostics contain the token, a raw secret, or the client address")
		}
	}
}

// net/http names the client address when it recovers a handler panic; the
// hub's error log removes it.
func TestHTTPErrorLogDropsClientAddresses(t *testing.T) {
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler failure for the error log test")
	}))
	server.Config.ErrorLog = httpErrorLog(logger)
	server.Start()
	defer server.Close()

	client := remoteHTTPClient(t, nil, "")
	var local string
	client.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err == nil {
			local = conn.LocalAddr().String()
		}
		return conn, err
	}
	if response, err := client.Get(server.URL); err == nil {
		_ = response.Body.Close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "handler failure") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	output := logs.String()
	if !strings.Contains(output, "handler failure") || !strings.Contains(output, "<address>") {
		t.Fatalf("the panic was not logged with the address removed: %q", output)
	}
	if local == "" || strings.Contains(output, local) {
		t.Fatalf("the error log contains the client address %q: %q", local, output)
	}

	logs = lockedBuffer{}
	_ = httpErrorLog(logger).Output(1, "http: TLS handshake error from [2001:db8::7]:5000 and 192.0.2.7:443: EOF")
	if output := logs.String(); strings.Contains(output, "2001:db8") || strings.Contains(output, "192.0.2.7") {
		t.Fatalf("addresses survived in the error log: %q", output)
	}
}
