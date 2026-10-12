package mcpserver

import (
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// bundleLoads returns the skill loads recorded for bundle requests.
func bundleLoads(events []telemetry.Event) []telemetry.Event {
	var loads []telemetry.Event
	for _, event := range events {
		if event.Type == telemetry.EventSkillLoaded && event.Payload["surface"] == "skill_bundle" {
			loads = append(loads, event)
		}
	}
	return loads
}

// sessionHashOf returns the session hash the hub recorded for a resolution.
func sessionHashOf(t *testing.T, events []telemetry.Event, resolutionID string) string {
	t.Helper()
	for _, event := range events {
		if event.ResolutionID == resolutionID && strings.HasPrefix(event.Type, "resolution.") && event.SessionIDHash != "" {
			return event.SessionIDHash
		}
	}
	t.Fatalf("no session hash recorded for resolution %s", resolutionID)
	return ""
}

func withSession(id string) func(*http.Request) {
	return func(request *http.Request) {
		if id != "" {
			request.Header.Set("Mcp-Session-Id", id)
		}
	}
}

func withSessionAndHeld(id, digest string) func(*http.Request) {
	return func(request *http.Request) {
		withSession(id)(request)
		request.Header.Set("If-None-Match", `"`+digest+`"`)
	}
}

func TestMarkBundleFetchCountsEachVersionOncePerSessionAndResolution(t *testing.T) {
	tracker := newActivationTracker(nil, t.TempDir())
	first, second := &mcp.ServerSession{}, &mcp.ServerSession{}
	steps := []struct {
		session *mcp.ServerSession
		skill   string
		digest  string
		res     string
		want    bool
	}{
		{first, "skill-a", "d1", "", true},
		{first, "skill-a", "d1", "", false},
		{first, "skill-a", "d2", "", true},
		{first, "skill-b", "d1", "", true},
		{second, "skill-a", "d1", "", true},
		{nil, "skill-a", "d1", "", true},
		{nil, "skill-a", "d1", "", true}, // shared by every client: never deduplicated
		{first, "skill-a", "", "", true},
		{first, "skill-a", "", "", false},
		{first, "skill-a", "d1", "res-2", true}, // another resolution proposing the same version
		{first, "skill-a", "d1", "res-2", false},
		{first, "skill-a", "d1", "", false},
	}
	for index, step := range steps {
		if got := tracker.markBundleFetch(step.session, step.skill, step.digest, step.res); got != step.want {
			t.Errorf("step %d: markBundleFetch(%s, %q, %q) = %v, want %v", index, step.skill, step.digest, step.res, got, step.want)
		}
	}
	if tracker.sessionHash(nil) == tracker.sessionHash(first) || tracker.sessionHash(first) == tracker.sessionHash(second) {
		t.Fatal("the no-session state shares a hash with a real session")
	}
}

// A bundle counts as one load per skill version and MCP session, however
// often it is fetched or revalidated; requests without a live session are
// counted once under the separate no-session state.
func TestBundleRequestsCountOneLoadPerVersionAndSession(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	first := connectHTTPClient(t, hub.endpoint, hub.token)
	second := connectHTTPClient(t, hub.endpoint, hub.token)
	task := resolveArguments(remoteBenchTaskText(t, root, "test-audit"))
	firstResolution := callSkillResolve(t, first, task).Resolution.ResolutionID
	// Resolution IDs are derived from the request, so the second session asks
	// for another operation to get a resolution ID of its own.
	task["operation"] = "test"
	secondResolution := callSkillResolve(t, second, task).Resolution.ResolutionID
	if firstResolution == secondResolution {
		t.Fatal("the two sessions share a resolution ID")
	}

	oldDigest := hubDigest(t, root, "test-audit")
	for _, step := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"first session fetches", withSession(first.ID()), http.StatusOK},
		{"first session fetches again", withSession(first.ID()), http.StatusOK},
		{"first session revalidates", withSessionAndHeld(first.ID(), oldDigest), http.StatusNotModified},
		{"second session revalidates", withSessionAndHeld(second.ID(), oldDigest), http.StatusNotModified},
		{"second session fetches", withSession(second.ID()), http.StatusOK},
		{"no session", nil, http.StatusOK},
		{"unknown session", withSessionAndHeld("no-such-session", oldDigest), http.StatusNotModified},
	} {
		if response, _ := getBundle(t, hub, "test-audit", step.mutate); response.StatusCode != step.status {
			t.Fatalf("%s: status %d, want %d", step.name, response.StatusCode, step.status)
		}
	}

	writeFixtureFile(t, filepath.Join(root, "skills", "default", "test-audit", "SKILL.md"), []byte("---\nname: test-audit\ndescription: Changed on the hub.\n---\n\n# test-audit\n"))
	rebuildFixtureCatalog(t, root)
	newDigest := hubDigest(t, root, "test-audit")
	if newDigest == oldDigest {
		t.Fatal("editing SKILL.md did not change the digest")
	}
	for range 2 {
		if response, _ := getBundle(t, hub, "test-audit", withSession(first.ID())); response.StatusCode != http.StatusOK || response.Header.Get("ETag") != `"`+newDigest+`"` {
			t.Fatalf("changed version: status %d, ETag %s", response.StatusCode, response.Header.Get("ETag"))
		}
	}
	hub.stop(t)

	events := recentRawEvents(t, openHubRecorder(t, root))
	firstHash, secondHash := sessionHashOf(t, events, firstResolution), sessionHashOf(t, events, secondResolution)
	perSession := map[string][]telemetry.Event{}
	for _, load := range bundleLoads(events) {
		perSession[load.SessionIDHash] = append(perSession[load.SessionIDHash], load)
		if load.Payload["skill_id"] != "test-audit" || load.Payload["resource_kind"] != "entrypoint" || load.Payload["basis"] != telemetry.LoadBasisServerObserved {
			t.Errorf("bundle load payload = %v", load.Payload)
		}
		if !strings.HasPrefix(load.Client.Name, telemetry.ClientRemotePrefix) {
			t.Errorf("bundle load client = %+v, want a remote label", load.Client)
		}
	}
	if got := len(perSession[firstHash]); got != 2 {
		t.Errorf("first session recorded %d bundle loads, want 2 (one per version)", got)
	}
	if got := len(perSession[secondHash]); got != 1 {
		t.Errorf("second session recorded %d bundle loads, want 1", got)
	}
	for _, load := range append(append([]telemetry.Event(nil), perSession[firstHash]...), perSession[secondHash]...) {
		if load.Payload["attribution"] != "recommended" || (load.ResolutionID != firstResolution && load.ResolutionID != secondResolution) {
			t.Errorf("session bundle load = resolution %s, payload %v; want it attributed to its session's resolution", load.ResolutionID, load.Payload)
		}
	}
	if first := perSession[firstHash]; len(first) > 0 && first[0].Payload["first_activation"] != true {
		t.Errorf("first bundle load of the session is not a first activation: %v", first[0].Payload)
	}
	delete(perSession, firstHash)
	delete(perSession, secondHash)
	if len(perSession) != 1 {
		t.Fatalf("requests without a live session were counted under %d session hashes, want exactly 1: %v", len(perSession), perSession)
	}
	// The no-session state is shared by every client, so its loads are not
	// deduplicated: one client's load must not hide another's.
	for hash, loads := range perSession {
		if hash == "" || len(loads) != 2 {
			t.Fatalf("no-session bundle loads = %d under hash %q, want 2: %+v", len(loads), hash, loads)
		}
		for _, load := range loads {
			if load.Payload["attribution"] != "unsolicited" || load.ResolutionID != "" {
				t.Fatalf("no-session bundle load = %+v, want unsolicited without a resolution", load)
			}
		}
	}
}

// A second resolution in the same session that proposes the same version
// gives another recommended activation, as a local skill_get does, while
// repeated fetches under one resolution still count once.
func TestBundleLoadCountsAgainUnderANewResolution(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	task := resolveArguments(remoteBenchTaskText(t, root, "test-audit"))
	firstResolution := callSkillResolve(t, session, task).Resolution.ResolutionID
	for range 2 {
		getBundle(t, hub, "test-audit", withSession(session.ID()))
	}
	task["operation"] = "test"
	secondResolution := callSkillResolve(t, session, task).Resolution.ResolutionID
	if firstResolution == secondResolution {
		t.Fatal("the two resolutions share an ID")
	}
	for range 2 {
		getBundle(t, hub, "test-audit", withSession(session.ID()))
	}
	hub.stop(t)

	loads := bundleLoads(recentRawEvents(t, openHubRecorder(t, root)))
	if len(loads) != 2 {
		t.Fatalf("recorded %d bundle loads, want 2 (one per resolution): %+v", len(loads), loads)
	}
	seen := map[string]bool{}
	for _, load := range loads {
		seen[load.ResolutionID] = true
		if load.Payload["attribution"] != "recommended" || load.Payload["first_activation"] != true {
			t.Errorf("bundle load payload = %v, want a recommended first activation", load.Payload)
		}
	}
	if !seen[firstResolution] || !seen[secondResolution] {
		t.Errorf("bundle loads cover resolutions %v, want %s and %s", seen, firstResolution, secondResolution)
	}
}

func TestBlockedBundleRecordsOneBlockedLoad(t *testing.T) {
	root := newBundleHubFixture(t)
	editFixtureMeta(t, root, "test-audit", markFixtureThirdParty)
	hub := startHTTPTestHub(t, root)
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	for range 2 {
		if response, _ := getBundle(t, hub, "test-audit", withSession(session.ID())); response.StatusCode != http.StatusForbidden {
			t.Fatalf("unapproved bundle status = %d", response.StatusCode)
		}
	}
	hub.stop(t)

	loads := bundleLoads(recentRawEvents(t, openHubRecorder(t, root)))
	if len(loads) != 1 {
		t.Fatalf("recorded %d blocked bundle loads, want 1: %+v", len(loads), loads)
	}
	payload := loads[0].Payload
	reasons, _ := payload["reason_codes"].([]any)
	if payload["status"] != "review_required" || payload["first_activation"] != false || len(reasons) == 0 || reasons[0] != "content_review_required" {
		t.Fatalf("blocked bundle load payload = %v", payload)
	}
}

// failingBundleTransport fails bundle requests at the network level while
// letting MCP calls through, as when only the bundle connection breaks.
type failingBundleTransport struct {
	base http.RoundTripper

	mu      sync.Mutex
	failing bool
	failed  int
}

func (transport *failingBundleTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.mu.Lock()
	fail := transport.failing && request.URL.Path == HTTPBundlePath
	if fail {
		transport.failed++
	}
	transport.mu.Unlock()
	if fail {
		return nil, errors.New("connection reset by the test")
	}
	return transport.base.RoundTrip(request)
}

func (transport *failingBundleTransport) setFailing(failing bool) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.failing = failing
}

func (transport *failingBundleTransport) failures() int {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.failed
}

func waitFor(t *testing.T, what string, done func() bool) time.Duration {
	t.Helper()
	started := time.Now()
	for !done() {
		if time.Since(started) > 10*time.Second {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return time.Since(started)
}

// A cache hit answers at once; the proxy then lets the hub count the load
// with one background conditional request that carries no content, once per
// version and hub session. The hub sits behind a link with a long round trip,
// so a hit that waited for the hub would blow the budget.
func TestProxyCacheHitCountsOneLoadOnTheHub(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	stateDir := t.TempDir()
	const rtt = 300 * time.Millisecond
	slowLink := startDelayProxy(t, strings.TrimPrefix(hub.baseURL, "http://"), rtt)
	counter := newCountingTransport()
	proxied := startTestProxy(t, slowLink, hub.token, stateDir, counter)
	proxyResolve(t, proxied.session, root, "herdr-cook-plan")
	if got, _, encoded := proxySkillGet(t, proxied.session, "herdr-cook-plan"); got.Remote == nil || got.Remote.Cache != remoteCacheFetched {
		t.Fatalf("first fetch over the slow link = %s", encoded)
	}
	proxied = startTestProxy(t, slowLink, hub.token, stateDir, counter)
	proxyResolve(t, proxied.session, root, "herdr-cook-plan")
	// As if an earlier call had found the hub down: a background success
	// must not tell the user the hub answers again.
	proxied.proxy.mu.Lock()
	proxied.proxy.unreachable = true
	proxied.proxy.mu.Unlock()
	before := counter.count()
	started := time.Now()
	got, _, encoded := proxySkillGet(t, proxied.session, "herdr-cook-plan")
	hit := time.Since(started)
	if got.Remote == nil || got.Remote.Cache != remoteCacheHit {
		t.Fatalf("skill_get of a cached version = %s", encoded)
	}
	if hit > cacheHitBudget {
		t.Errorf("cache hit took %s, budget %s (round trip to the hub %s)", hit, cacheHitBudget, rtt)
	}
	waited := waitFor(t, "the background confirmation", func() bool { return counter.count().bundles > before.bundles })
	t.Logf("cache hit %s; the hub confirmed the version %s later", hit, waited)
	after := counter.count()
	if after.bundles-before.bundles != 1 || counter.lastStatus() != http.StatusNotModified || after.bundleBytes != before.bundleBytes {
		t.Fatalf("background confirmation: %d bundle requests, last status %d, %d content bytes; want one 304 with none", after.bundles-before.bundles, counter.lastStatus(), after.bundleBytes-before.bundleBytes)
	}
	defer func() {
		if strings.Contains(proxied.warnings.String(), "answers again") {
			t.Errorf("a background confirmation told the user the hub answers again: %q", proxied.warnings.String())
		}
	}()
	waitFor(t, "the confirmation to finish", func() bool {
		proxied.proxy.mu.Lock()
		defer proxied.proxy.mu.Unlock()
		return len(proxied.proxy.confirming) == 0
	})

	before = counter.count()
	started = time.Now()
	again, _, _ := proxySkillGet(t, proxied.session, "herdr-cook-plan")
	if repeat := time.Since(started); repeat > cacheHitBudget || again.Remote.Cache != remoteCacheHit {
		t.Errorf("repeat cache hit took %s (%s), budget %s", repeat, again.Remote.Cache, cacheHitBudget)
	}
	time.Sleep(100 * time.Millisecond)
	if after := counter.count(); after.requests != before.requests {
		t.Fatalf("a version the hub already counted in this session was confirmed again: %d requests", after.requests-before.requests)
	}
	hub.stop(t)

	loads := bundleLoads(recentRawEvents(t, openHubRecorder(t, root)))
	perSession := map[string]int{}
	for _, load := range loads {
		perSession[load.SessionIDHash]++
		if load.Payload["attribution"] != "recommended" || load.ResolutionID == "" {
			t.Errorf("cache-hit load is not attributed to the session's resolution: %+v", load)
		}
		if load.Client.Name != "remote:claude-code" {
			t.Errorf("load client = %+v, want remote:claude-code", load.Client)
		}
	}
	if len(loads) != 2 || len(perSession) != 2 {
		t.Fatalf("recorded %d bundle loads in %d sessions, want one fetch and one cache hit in two sessions: %+v", len(loads), len(perSession), loads)
	}
}

// Cache hits count like local skill_get calls: once per resolution that
// proposes the version, so a second resolution in the same session that
// proposes the cached version gives a second load.
func TestProxyCacheHitCountsAgainUnderANewResolution(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	stateDir := t.TempDir()
	warm := startTestProxy(t, hub.baseURL, hub.token, stateDir, nil)
	proxyResolve(t, warm.session, root, "test-audit")
	proxySkillGet(t, warm.session, "test-audit")

	counter := newCountingTransport()
	proxied := startTestProxy(t, hub.baseURL, hub.token, stateDir, counter)
	for _, operation := range []string{"review", "test"} {
		before := counter.count()
		proxyResolveOperation(t, proxied.session, root, "test-audit", operation)
		for range 2 {
			if got, _, encoded := proxySkillGet(t, proxied.session, "test-audit"); got.Remote == nil || got.Remote.Cache != remoteCacheHit {
				t.Fatalf("skill_get of a cached version = %s", encoded)
			}
		}
		waitFor(t, "the background confirmation", func() bool { return counter.count().bundles > before.bundles })
		waitFor(t, "the confirmation to finish", func() bool {
			proxied.proxy.mu.Lock()
			defer proxied.proxy.mu.Unlock()
			return len(proxied.proxy.confirming) == 0
		})
		if got := counter.count().bundles - before.bundles; got != 1 {
			t.Fatalf("resolution for %s sent %d bundle requests, want 1", operation, got)
		}
	}
	hub.stop(t)

	resolutions := map[string]int{}
	for _, load := range bundleLoads(recentRawEvents(t, openHubRecorder(t, root))) {
		if load.SessionIDHash == "" || load.ResolutionID == "" || load.Payload["attribution"] != "recommended" {
			t.Errorf("bundle load = %+v, want a recommended load of a resolution", load)
		}
		resolutions[load.ResolutionID]++
	}
	counted := 0
	for _, loads := range resolutions {
		if loads == 1 {
			counted++
		}
	}
	if counted < 2 {
		t.Fatalf("bundle loads per resolution = %v, want the two resolutions of the proxied session to count once each", resolutions)
	}
}

// When the bundle confirmation cannot reach the hub, the cache hit is still
// served at once, nothing is retried in a loop, the user is not warned, and
// the proxy reports the load once through skill_feedback (outcome loaded)
// for the resolution its open hub session answered.
func TestProxyCacheHitWithoutTheBundleEndpointFallsBackToFeedback(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	stateDir := t.TempDir()
	warm := startTestProxy(t, hub.baseURL, hub.token, stateDir, nil)
	proxyResolve(t, warm.session, root, "test-audit")
	proxySkillGet(t, warm.session, "test-audit")

	transport := &failingBundleTransport{base: http.DefaultTransport.(*http.Transport).Clone()}
	proxied := startTestProxy(t, hub.baseURL, hub.token, stateDir, transport)
	digest := proxyResolve(t, proxied.session, root, "test-audit")
	transport.setFailing(true)
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		got, _, encoded := proxySkillGet(t, proxied.session, "test-audit")
		if hit := time.Since(started); hit > cacheHitBudget || got.Remote == nil || got.Remote.Cache != remoteCacheHit {
			t.Fatalf("cache hit %d with the bundle endpoint failing took %s: %.300s", attempt, hit, encoded)
		}
		waitFor(t, "the failed confirmation", func() bool {
			proxied.proxy.mu.Lock()
			defer proxied.proxy.mu.Unlock()
			return transport.failures() >= attempt && len(proxied.proxy.confirming) == 0
		})
	}
	time.Sleep(100 * time.Millisecond)
	if failures := transport.failures(); failures != 2 {
		t.Fatalf("%d bundle attempts for two cache hits, want one each and no retry loop", failures)
	}
	if strings.Contains(proxied.warnings.String(), "cannot be reached") {
		t.Fatalf("a background confirmation warned the user: %q", proxied.warnings.String())
	}
	sessionID := proxied.proxy.currentSessionID()
	hub.stop(t)

	var feedback []telemetry.Event
	for _, event := range recentRawEvents(t, openHubRecorder(t, root)) {
		if event.Type == telemetry.EventSkillLoaded && event.Payload["basis"] != telemetry.LoadBasisServerObserved {
			feedback = append(feedback, event)
		}
		if event.Type == telemetry.EventSkillUsed {
			t.Errorf("a cache hit reported use: %+v", event)
		}
	}
	if len(feedback) != 1 || feedback[0].ID != cachedLoadEventID(sessionID, "test-audit", digest) || feedback[0].ResolutionID == "" {
		t.Fatalf("fallback feedback events = %+v, want one loaded event with the fixed event_id", feedback)
	}
}

// switchableLink relays loopback connections to target until it is cut; from
// then on it closes the open connections and accepts new ones without ever
// answering, like a hub behind a tunnel that silently stopped forwarding.
type switchableLink struct {
	listener net.Listener
	target   string

	mu   sync.Mutex
	cut  bool
	open []net.Conn
}

func startSwitchableLink(t *testing.T, target string) *switchableLink {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	link := &switchableLink{listener: listener, target: target}
	var relays sync.WaitGroup
	t.Cleanup(func() {
		_ = listener.Close()
		link.closeAll()
		relays.Wait()
	})
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			link.mu.Lock()
			link.open = append(link.open, client)
			cut := link.cut
			link.mu.Unlock()
			if cut {
				continue // held open, never answered
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				_ = client.Close()
				continue
			}
			link.mu.Lock()
			link.open = append(link.open, server)
			link.mu.Unlock()
			relays.Add(2)
			go func() { defer relays.Done(); _, _ = io.Copy(server, client); _ = server.Close() }()
			go func() { defer relays.Done(); _, _ = io.Copy(client, server); _ = client.Close() }()
		}
	}()
	return link
}

func (link *switchableLink) url() string { return "http://" + link.listener.Addr().String() }

func (link *switchableLink) closeAll() {
	link.mu.Lock()
	defer link.mu.Unlock()
	for _, connection := range link.open {
		_ = connection.Close()
	}
	link.open = nil
}

func (link *switchableLink) cutOff() {
	link.mu.Lock()
	link.cut = true
	link.mu.Unlock()
	link.closeAll()
}

// With the hub gone, whether it refuses connections or never answers, a
// cache hit stays inside its budget, the user is not warned, and the proxy
// still stops promptly.
func TestProxyCacheHitWithTheHubDownStaysFast(t *testing.T) {
	for _, mode := range []string{"refused", "silent"} {
		t.Run(mode, func(t *testing.T) {
			root := newBundleHubFixture(t)
			hub := startHTTPTestHub(t, root)
			link := startSwitchableLink(t, strings.TrimPrefix(hub.baseURL, "http://"))
			stateDir := t.TempDir()
			warm := startTestProxy(t, link.url(), hub.token, stateDir, nil)
			proxyResolve(t, warm.session, root, "test-audit")
			proxySkillGet(t, warm.session, "test-audit")

			proxied := startTestProxy(t, link.url(), hub.token, stateDir, nil)
			proxyResolve(t, proxied.session, root, "test-audit")
			if mode == "refused" {
				hub.stop(t)
				_ = link.listener.Close()
				link.closeAll()
			} else {
				link.cutOff()
			}
			for range 3 {
				started := time.Now()
				got, _, encoded := proxySkillGet(t, proxied.session, "test-audit")
				if hit := time.Since(started); hit > cacheHitBudget || got.Remote == nil || got.Remote.Cache != remoteCacheHit {
					t.Fatalf("cache hit with the hub down took %s: %.300s", hit, encoded)
				}
			}
			if mode == "refused" {
				waitFor(t, "the background confirmations", func() bool {
					proxied.proxy.mu.Lock()
					defer proxied.proxy.mu.Unlock()
					return len(proxied.proxy.confirming) == 0
				})
			} else {
				proxied.proxy.mu.Lock()
				inFlight := len(proxied.proxy.confirming)
				proxied.proxy.mu.Unlock()
				if inFlight != 1 {
					t.Fatalf("%d confirmations in flight for three hits of one version, want 1", inFlight)
				}
			}
			if strings.Contains(proxied.warnings.String(), "cannot be reached") {
				t.Fatalf("a background confirmation warned the user: %q", proxied.warnings.String())
			}
			// Stopping the proxy cancels a confirmation the hub never answers
			// instead of waiting for its timeout. (Closing the hub session
			// afterwards is bounded separately by the MCP client.)
			proxy := proxied.proxy
			proxy.mu.Lock()
			proxy.stopBackground()
			proxy.mu.Unlock()
			stopped := make(chan struct{})
			go func() { proxy.backgroundTasks.Wait(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("stopping the proxy waited for a background confirmation the hub never answers")
			}
		})
	}
}

// A host that negotiates without an initialize handshake still names itself
// in each request; the proxy passes that name on to the hub.
func TestProxyNotesTheHostNameFromRequestMetadata(t *testing.T) {
	proxy := &Proxy{}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{mcp.MetaKeyClientInfo: map[string]any{"name": "codex", "version": "0.9"}}}}
	proxy.noteClient(request)
	if proxy.clientInfo == nil || proxy.clientInfo.Name != "codex" {
		t.Fatalf("client info = %+v, want codex", proxy.clientInfo)
	}
	proxy.noteClient(&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Meta: mcp.Meta{mcp.MetaKeyClientInfo: map[string]any{"name": "gemini"}}}})
	if proxy.clientInfo.Name != "codex" {
		t.Fatalf("a later request replaced the host name: %+v", proxy.clientInfo)
	}
	empty := &Proxy{}
	empty.noteClient(&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}})
	if empty.clientInfo != nil {
		t.Fatalf("a request without a name set client info %+v", empty.clientInfo)
	}
}
