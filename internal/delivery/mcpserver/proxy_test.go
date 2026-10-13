package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
)

// countingTransport counts every request the proxy sends to the hub, and the
// bundle requests with their status codes and body bytes. With tamper set it
// corrupts every bundle body in transit.
type countingTransport struct {
	base   http.RoundTripper
	tamper bool

	mu          sync.Mutex
	requests    int
	bundles     int
	statuses    []int
	bundleBytes int64
}

func newCountingTransport() *countingTransport {
	return &countingTransport{base: http.DefaultTransport.(*http.Transport).Clone()}
}

func (transport *countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.requests++
	if request.URL.Path != HTTPBundlePath || err != nil {
		return response, err
	}
	transport.bundles++
	transport.statuses = append(transport.statuses, response.StatusCode)
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	transport.bundleBytes += int64(len(body))
	if transport.tamper && response.StatusCode == http.StatusOK && len(body) > 0 {
		body[len(body)/2] ^= 0xff
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

type transportCount struct {
	requests, bundles int
	bundleBytes       int64
}

func (transport *countingTransport) count() transportCount {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transportCount{requests: transport.requests, bundles: transport.bundles, bundleBytes: transport.bundleBytes}
}

func (transport *countingTransport) lastStatus() int {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.statuses) == 0 {
		return 0
	}
	return transport.statuses[len(transport.statuses)-1]
}

type testProxy struct {
	proxy    *Proxy
	session  *mcp.ClientSession
	warnings *lockedBuffer
}

// startTestProxy runs a proxy for hubURL on in-memory transports, the way an
// agent host runs it on stdio.
func startTestProxy(t *testing.T, hubURL, token, stateDir string, transport http.RoundTripper) *testProxy {
	t.Helper()
	warnings := &lockedBuffer{}
	proxy, err := NewProxy(t.Context(), ProxyOptions{
		HubURL: hubURL, Token: token, StateDir: stateDir, Transport: transport,
		Warn: func(message string) { _, _ = warnings.Write([]byte(message + "\n")) },
	})
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- proxy.Run(ctx, serverTransport) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("proxy stopped with %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("proxy did not stop")
		}
	})
	return &testProxy{proxy: proxy, session: session, warnings: warnings}
}

type proxyGetResult struct {
	Content string          `json:"content"`
	SkillID string          `json:"skill_id"`
	Local   *app.LocalSkill `json:"local"`
	Remote  *remoteDelivery `json:"remote"`
}

func callProxyTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) (*mcp.CallToolResult, []byte) {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s through the proxy: %v", name, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	return result, encoded
}

func proxyResolve(t *testing.T, session *mcp.ClientSession, root, id string) string {
	t.Helper()
	return proxyResolveOperation(t, session, root, id, "")
}

// proxyResolveOperation resolves for the given operation, which a test
// changes to get another resolution for the same skill.
func proxyResolveOperation(t *testing.T, session *mcp.ClientSession, root, id, operation string) string {
	t.Helper()
	params := map[string]any{
		"schema_version": SchemaVersion, "request_id": "REQ-proxy-" + id,
		"task": map[string]any{"description": remoteBenchTaskText(t, root, id), "scope": "multi_step"},
	}
	if operation != "" {
		params["operation"] = operation
	}
	result, encoded := callProxyTool(t, session, "skill_resolve", params)
	var outcome struct {
		Result resolveResult `json:"result"`
	}
	if result.IsError || json.Unmarshal(encoded, &outcome) != nil || outcome.Result.Resolution.Primary == nil || outcome.Result.Resolution.Primary.ID != id {
		t.Fatalf("resolve %s through the proxy = %s", id, encoded)
	}
	return strings.TrimPrefix(outcome.Result.Resolution.Primary.Version, "sha256:")
}

func proxySkillGet(t *testing.T, session *mcp.ClientSession, id string) (proxyGetResult, *mcp.CallToolResult, []byte) {
	t.Helper()
	result, encoded := callProxyTool(t, session, "skill_get", map[string]any{"skill_id": id})
	var outcome struct {
		Result *proxyGetResult `json:"result"`
	}
	if err := json.Unmarshal(encoded, &outcome); err != nil {
		t.Fatalf("decode skill_get %s: %v (%s)", id, err, encoded)
	}
	if outcome.Result == nil {
		return proxyGetResult{}, result, encoded
	}
	return *outcome.Result, result, encoded
}

func TestProxyOffersTheHubsRuntimeTools(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	direct := connectHTTPClient(t, hub.endpoint, hub.token)
	proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), nil)
	list := func(session *mcp.ClientSession) []*mcp.Tool {
		result, err := session.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		return result.Tools
	}
	want, got := list(direct), list(proxied.session)
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if !bytes.Equal(wantJSON, gotJSON) {
		t.Fatalf("proxy tools differ from the hub's:\nhub   %s\nproxy %s", wantJSON, gotJSON)
	}
}

func TestProxyFetchesWholeSkillsOnceAndServesThemFromTheCache(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	counter := newCountingTransport()
	stateDir := t.TempDir()
	proxied := startTestProxy(t, hub.baseURL, hub.token, stateDir, counter)
	session := proxied.session

	for _, id := range []string{"test-audit", "herdr-cook-plan"} {
		digest := proxyResolve(t, session, root, id)
		if digest != hubDigest(t, root, id) {
			t.Fatalf("resolution of %s names %s, hub pins %s", id, digest, hubDigest(t, root, id))
		}
		before := counter.count()
		got, result, encoded := proxySkillGet(t, session, id)
		after := counter.count()
		if result.IsError || got.Local == nil || got.Remote == nil || got.Remote.Cache != remoteCacheFetched {
			t.Fatalf("first skill_get %s = %s", id, encoded)
		}
		if after.requests-before.requests != 1 || after.bundles-before.bundles != 1 {
			t.Fatalf("first skill_get %s made %d requests (%d bundles), want exactly one bundle request", id, after.requests-before.requests, after.bundles-before.bundles)
		}
		local := got.Local
		if !strings.HasPrefix(local.Path, skillruntime.RemoteCacheDir(stateDir)) || !strings.HasSuffix(local.Path, filepath.Join(id, digest)) {
			t.Fatalf("local.path %s is not the cached copy", local.Path)
		}
		served := 0
		for _, file := range remoteBenchShapes[id] {
			if !strings.HasPrefix(file.path, ".meta/") {
				served++
			}
		}
		folder, files := folderDigest(t, local.Path)
		if folder != digest || files != served {
			t.Fatalf("cached %s folder = %s with %d files, hub digest %s with %d served files", id, folder, files, digest, served)
		}
		hubSkillMD, err := os.ReadFile(filepath.Join(root, "skills", "default", id, "SKILL.md"))
		if err != nil || got.Content != string(hubSkillMD) || got.SkillID != id {
			t.Fatalf("skill_get %s content differs from the hub's SKILL.md (%v)", id, err)
		}
		if local.Env[skillruntime.EnvSkillDir] != local.Path || !strings.HasPrefix(local.Env[skillruntime.EnvStateDir], stateDir) || !strings.HasPrefix(local.Env[skillruntime.EnvConfigDir], stateDir) {
			t.Fatalf("local.env = %v", local.Env)
		}
		if strings.Contains(string(encoded), root) {
			t.Fatalf("skill_get through the proxy names the hub's workspace: %s", encoded)
		}

		before = counter.count()
		again, _, encoded := proxySkillGet(t, session, id)
		after = counter.count()
		if after.requests != before.requests {
			t.Fatalf("a repeat skill_get %s made %d requests, want none", id, after.requests-before.requests)
		}
		if again.Remote == nil || again.Remote.Cache != remoteCacheHit || again.Local.Path != local.Path {
			t.Fatalf("repeat skill_get %s = %s", id, encoded)
		}
	}

	t.Run("a changed skill is fetched again, an unrelated change is not", func(t *testing.T) {
		oldDigest := hubDigest(t, root, "herdr-cook-plan")
		writeFixtureFile(t, filepath.Join(root, "skills", "default", "herdr-cook-plan", "references", "dispatch.md"), []byte("# Dispatch\n\nRewritten on the hub.\n"))
		editFixtureMeta(t, root, "test-audit", func(document map[string]any) {
			routing := document["routing"].(map[string]any)
			routing["triggers"] = append(routing["triggers"].([]any), "audit the fixture tests again")
		})

		newDigest := proxyResolve(t, session, root, "herdr-cook-plan")
		if newDigest == oldDigest {
			t.Fatal("editing a file did not change the skill digest")
		}
		before := counter.count()
		changed, _, encoded := proxySkillGet(t, session, "herdr-cook-plan")
		after := counter.count()
		if after.bundles-before.bundles != 1 || changed.Remote.Cache != remoteCacheFetched || !strings.HasSuffix(changed.Local.Path, newDigest) {
			t.Fatalf("changed skill = %s after %d bundle requests", encoded, after.bundles-before.bundles)
		}
		if digest, _ := folderDigest(t, changed.Local.Path); digest != newDigest {
			t.Fatalf("refetched folder digest %s, want %s", digest, newDigest)
		}

		proxyResolve(t, session, root, "test-audit")
		before = counter.count()
		unchanged, _, encoded := proxySkillGet(t, session, "test-audit")
		// A cache hit lets the hub count the load in a background request; wait
		// for it so its timing against the hub's speed cannot decide the check.
		proxied.proxy.backgroundTasks.Wait()
		after = counter.count()
		if after.bundleBytes != before.bundleBytes || unchanged.Remote.Cache != remoteCacheHit {
			t.Fatalf("a metadata-only change refetched test-audit: %s (%d bytes downloaded)", encoded, after.bundleBytes-before.bundleBytes)
		}
	})

	t.Run("a new session asks once and receives no content for a cached version", func(t *testing.T) {
		// A cache hit under a new resolution sends its load confirmation in
		// the background; let those requests end so they are not counted as
		// requests of the new session.
		proxied.proxy.backgroundTasks.Wait()
		fresh := startTestProxy(t, hub.baseURL, hub.token, stateDir, counter)
		before := counter.count()
		got, _, encoded := proxySkillGet(t, fresh.session, "test-audit")
		after := counter.count()
		if after.requests-before.requests != 1 || counter.lastStatus() != http.StatusNotModified || after.bundleBytes != before.bundleBytes {
			t.Fatalf("cached version without a resolution: %d requests, last status %d, %d content bytes", after.requests-before.requests, counter.lastStatus(), after.bundleBytes-before.bundleBytes)
		}
		if got.Remote == nil || got.Remote.Cache != remoteCacheHit {
			t.Fatalf("skill_get = %s", encoded)
		}
		before = counter.count()
		proxySkillGet(t, fresh.session, "test-audit")
		if after := counter.count(); after.requests != before.requests {
			t.Fatalf("a confirmed version was asked for again: %d requests", after.requests-before.requests)
		}
	})
}

func TestProxyRunsASkillCheckOnTheClient(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture check is a POSIX shell script")
	}
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	stateDir := t.TempDir()
	proxied := startTestProxy(t, hub.baseURL, hub.token, stateDir, nil)

	proxyResolve(t, proxied.session, root, bundleCheckSkill)
	got, result, encoded := proxySkillGet(t, proxied.session, bundleCheckSkill)
	if result.IsError || got.Local == nil || got.Local.Preflight == nil {
		t.Fatalf("skill_get check-skill = %s", encoded)
	}
	preflight := got.Local.Preflight
	if preflight.Check == "" || preflight.WorkingDirectory != got.Local.StateDirectory || !strings.HasPrefix(preflight.WorkingDirectory, stateDir) {
		t.Fatalf("preflight = %+v", preflight)
	}
	// Run the check the way the activation flow tells an agent to: in
	// working_directory, with the variables from env exported.
	command := exec.Command("sh", "-c", preflight.Check)
	command.Dir = preflight.WorkingDirectory
	command.Env = os.Environ()
	for name, value := range preflight.Env {
		command.Env = append(command.Env, name+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "check passed") {
		t.Fatalf("check failed: %v\n%s", err, output)
	}
	ran, err := os.ReadFile(filepath.Join(preflight.WorkingDirectory, "check-ran.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ran), "marker from the hub\n") || strings.Contains(string(ran), root) {
		t.Fatalf("the check did not run from the client's cached copy: %q", ran)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "envs")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(root, "runtime", "envs"))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), bundleCheckSkill+"@") {
				t.Fatal("the remote fetch prepared a state directory on the hub")
			}
		}
	}
}

func TestProxyKeepsTheReviewGate(t *testing.T) {
	root := newBundleHubFixture(t)
	editFixtureMeta(t, root, "test-audit", markFixtureThirdParty)
	hub := startHTTPTestHub(t, root)
	counter := newCountingTransport()
	proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), counter)

	got, result, encoded := proxySkillGet(t, proxied.session, "test-audit")
	if result.IsError || got.Local == nil || got.Local.Status != app.LocalStatusReviewRequired || got.Content != "" || got.Local.Path != "" {
		t.Fatalf("unapproved skill through the proxy = %s", encoded)
	}
	if counter.lastStatus() != http.StatusForbidden {
		t.Fatalf("bundle status = %d, want 403", counter.lastStatus())
	}

	// The same call on a local hub gives the same blocked answer.
	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	local, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })
	want, _, _ := proxySkillGet(t, local, "test-audit")
	if want.Local == nil || !reflect.DeepEqual(*want.Local, *got.Local) || want.Content != got.Content {
		t.Fatalf("blocked answers differ:\nlocal %+v\nproxy %+v", want.Local, got.Local)
	}
	if versions := proxied.proxy.cache.Versions(proxied.proxy.hubID, "test-audit"); len(versions) != 0 {
		t.Fatal("an unapproved skill was cached")
	}
}

func TestProxyRejectsATamperedBundle(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	counter := newCountingTransport()
	counter.tamper = true
	proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), counter)

	_, result, encoded := proxySkillGet(t, proxied.session, "herdr-cook-plan")
	if !result.IsError || !strings.Contains(string(encoded), "resource_digest_mismatch") {
		t.Fatalf("tampered bundle = %s", encoded)
	}
	if versions := proxied.proxy.cache.Versions(proxied.proxy.hubID, "herdr-cook-plan"); len(versions) != 0 {
		t.Fatal("a tampered bundle was cached")
	}
}

func TestProxyUsesTheCacheWhenTheHubIsDown(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	stateDir := t.TempDir()
	first := startTestProxy(t, hub.baseURL, hub.token, stateDir, nil)
	proxyResolve(t, first.session, root, "herdr-cook-plan")
	fetched, _, _ := proxySkillGet(t, first.session, "herdr-cook-plan")
	hub.stop(t)

	offline := startTestProxy(t, hub.baseURL, hub.token, stateDir, nil)
	got, result, encoded := proxySkillGet(t, offline.session, "herdr-cook-plan")
	if result.IsError || got.Remote == nil || got.Remote.Cache != remoteCacheOffline || got.Remote.Message == "" || got.Local.Path != fetched.Local.Path {
		t.Fatalf("skill_get with the hub down = %s", encoded)
	}
	if !strings.Contains(offline.warnings.String(), "cannot be reached") {
		t.Fatalf("no warning when the hub is down: %q", offline.warnings.String())
	}

	empty := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), nil)
	_, result, encoded = proxySkillGet(t, empty.session, "herdr-cook-plan")
	if !result.IsError || !strings.Contains(string(encoded), "hub_unreachable") {
		t.Fatalf("uncached skill with the hub down = %s", encoded)
	}
	resolved, encoded := callProxyTool(t, empty.session, "skill_resolve", map[string]any{
		"schema_version": SchemaVersion, "request_id": "REQ-down",
		"task": map[string]any{"description": "fetch the herdr-cook-plan fixture skill", "scope": "multi_step"},
	})
	if !resolved.IsError || !strings.Contains(string(encoded), "hub_unreachable") {
		t.Fatalf("resolve with the hub down = %s", encoded)
	}
}

func TestProxyServesNoCachedCopyWhenTheHubAnswersAnError(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	digest := hubDigest(t, root, "herdr-cook-plan")
	_, archive := getBundle(t, hub, "herdr-cook-plan", nil)

	busy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "the hub is busy", http.StatusServiceUnavailable)
	}))
	t.Cleanup(busy.Close)
	stateDir := t.TempDir()
	hubID, err := skillruntime.HubID(busy.URL)
	if err != nil {
		t.Fatal(err)
	}
	cache := skillruntime.RemoteCache{Root: skillruntime.RemoteCacheDir(stateDir)}
	if _, err := cache.Install(hubID, "herdr-cook-plan", digest, bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
	proxied := startTestProxy(t, busy.URL, hub.token, stateDir, nil)
	_, result, encoded := proxySkillGet(t, proxied.session, "herdr-cook-plan")
	if !result.IsError || !strings.Contains(string(encoded), "hub_error") || strings.Contains(proxied.warnings.String(), "cannot be reached") {
		t.Fatalf("an answering hub with an error = %s (warnings %q)", encoded, proxied.warnings.String())
	}

	busy.Close()
	got, result, encoded := proxySkillGet(t, proxied.session, "herdr-cook-plan")
	if result.IsError || got.Remote == nil || got.Remote.Cache != remoteCacheOffline {
		t.Fatalf("an unreachable hub should fall back to the cached copy: %s", encoded)
	}
}

func TestProxyServesNoCachedCopyAfterAReviewRequiredResolution(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	proxied := startTestProxy(t, hub.baseURL, hub.token, t.TempDir(), nil)
	proxyResolve(t, proxied.session, root, "test-audit")
	if got, _, encoded := proxySkillGet(t, proxied.session, "test-audit"); got.Remote == nil || got.Remote.Cache != remoteCacheFetched {
		t.Fatalf("first fetch = %s", encoded)
	}

	// Approval is withdrawn without a content change, so the digest stays.
	editFixtureMeta(t, root, "test-audit", markFixtureThirdParty)
	_, encoded := callProxyTool(t, proxied.session, "skill_resolve", map[string]any{
		"schema_version": SchemaVersion, "request_id": "REQ-review",
		"task": map[string]any{"description": remoteBenchTaskText(t, root, "test-audit"), "scope": "multi_step"},
	})
	if !strings.Contains(string(encoded), `"state":"review_required"`) {
		t.Fatalf("resolution does not flag the review: %s", encoded)
	}
	hub.stop(t)
	_, result, encoded := proxySkillGet(t, proxied.session, "test-audit")
	if !result.IsError || !strings.Contains(string(encoded), "hub_unreachable") {
		t.Fatalf("a skill flagged for review was served from the cache: %s", encoded)
	}
}

func TestProxyFollowsNoRedirect(t *testing.T) {
	var elsewhere sync.Mutex
	var leaked []string
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		elsewhere.Lock()
		defer elsewhere.Unlock()
		leaked = append(leaked, request.Header.Get("Authorization"))
	}))
	t.Cleanup(other.Close)
	redirecting := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, other.URL+request.URL.RequestURI(), http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)

	proxied := startTestProxy(t, redirecting.URL, strings.Repeat("ab", 32), t.TempDir(), nil)
	_, result, encoded := proxySkillGet(t, proxied.session, "test-audit")
	if !result.IsError {
		t.Fatalf("a redirect was accepted: %s", encoded)
	}
	elsewhere.Lock()
	defer elsewhere.Unlock()
	if len(leaked) != 0 {
		t.Fatalf("the proxy followed a redirect to another host (%d requests)", len(leaked))
	}
}

func TestProxyRejectsAWrongToken(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	proxied := startTestProxy(t, hub.baseURL, strings.Repeat("0", 64), t.TempDir(), nil)
	_, result, encoded := proxySkillGet(t, proxied.session, "test-audit")
	if !result.IsError || !strings.Contains(string(encoded), "hub_token_rejected") || strings.Contains(string(encoded), hub.token) {
		t.Fatalf("wrong token = %s", encoded)
	}
	resolved, encoded := callProxyTool(t, proxied.session, "skill_resolve", map[string]any{
		"schema_version": SchemaVersion, "request_id": "REQ-wrong-token",
		"task": map[string]any{"description": "fetch the test-audit fixture skill", "scope": "multi_step"},
	})
	if !resolved.IsError || !strings.Contains(string(encoded), "hub_token_rejected") {
		t.Fatalf("resolve with a wrong token = %s", encoded)
	}
}

func TestNormalizeHubURL(t *testing.T) {
	for raw, want := range map[string]string{
		"http://127.0.0.1:7420":      "http://127.0.0.1:7420",
		"http://127.0.0.1:7420/":     "http://127.0.0.1:7420",
		"http://127.0.0.1:7420/mcp":  "http://127.0.0.1:7420",
		"https://hub.example/prefix": "https://hub.example/prefix",
	} {
		if got, err := NormalizeHubURL(raw); err != nil || got != want {
			t.Errorf("NormalizeHubURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"127.0.0.1:7420", "ftp://hub", "http://user:secret@hub", "http://hub?token=x", "http://"} {
		if _, err := NormalizeHubURL(raw); err == nil {
			t.Errorf("NormalizeHubURL(%q) accepted", raw)
		}
	}
}

// startDelayProxy forwards loopback connections to target through a link
// with the given round-trip time (see startLink) and returns its URL.
func startDelayProxy(t *testing.T, target string, rtt time.Duration) string {
	t.Helper()
	return startLink(t, target, rtt).url
}

// benchLink is a loopback TCP relay in front of a hub. It counts the bytes
// that cross it in both directions and the HTTP requests sent through it, and
// can add a round-trip time: every chunk is delivered half a round trip after
// it was read in each direction, so a request and its answer pay one full
// round trip, as on a slower link. The first chunk of a connection waits one
// more round trip, the cost of the TCP handshake. It does not limit
// bandwidth. A zero round trip relays at once, which is how a request count
// and a byte count are taken on plain loopback.
type benchLink struct {
	url      string
	bytes    atomic.Int64
	requests atomic.Int64
}

// httpRequestLine ends the request line of every HTTP/1.1 request the proxy
// sends; counting it in the client-to-hub stream counts requests.
var httpRequestLine = []byte(" HTTP/1.1\r\n")

func (link *benchLink) counts() (requests, bytes int64) {
	return link.requests.Load(), link.bytes.Load()
}

func startLink(t *testing.T, target string, rtt time.Duration) *benchLink {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	link := &benchLink{url: "http://" + listener.Addr().String()}
	var connections sync.WaitGroup
	var mu sync.Mutex
	var open []net.Conn
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		for _, connection := range open {
			_ = connection.Close()
		}
		mu.Unlock()
		connections.Wait()
	})
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				_ = client.Close()
				continue
			}
			mu.Lock()
			open = append(open, client, server)
			mu.Unlock()
			connections.Add(2)
			var tail []byte
			upstream := func(data []byte) {
				link.bytes.Add(int64(len(data)))
				joined := append(tail, data...)
				link.requests.Add(int64(bytes.Count(joined, httpRequestLine)))
				tail = append([]byte(nil), joined[max(0, len(joined)-len(httpRequestLine)+1):]...)
			}
			downstream := func(data []byte) { link.bytes.Add(int64(len(data))) }
			go func() { defer connections.Done(); delayedCopy(server, client, rtt/2, rtt, upstream) }()
			go func() { defer connections.Done(); delayedCopy(client, server, rtt/2, 0, downstream) }()
		}
	}()
	return link
}

// delayedCopy copies source to destination, delivering each chunk delay after
// it was read (the first one firstExtra later still) and passing every chunk
// to observe as it is read.
func delayedCopy(destination, source net.Conn, delay, firstExtra time.Duration, observe func([]byte)) {
	type chunk struct {
		data []byte
		at   time.Time
	}
	chunks := make(chan chunk, 1024)
	go func() {
		defer close(chunks)
		buffer := make([]byte, 64<<10)
		for {
			n, err := source.Read(buffer)
			if n > 0 {
				data := append([]byte(nil), buffer[:n]...)
				if observe != nil {
					observe(data)
				}
				chunks <- chunk{data: data, at: time.Now().Add(delay + firstExtra)}
				firstExtra = 0
			}
			if err != nil {
				return
			}
		}
	}()
	for item := range chunks {
		time.Sleep(time.Until(item.at))
		if _, err := destination.Write(item.data); err != nil {
			break
		}
	}
	_ = destination.Close()
	_ = source.Close()
}
