package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

const (
	// A resolution tells the proxy which version of a skill is current. It
	// trusts that answer for as long as the hub lets clients cache responses;
	// later, skill_get asks the hub again (one small exchange, no content when
	// the cached version is still current).
	proxyHintTTL = cacheTTLMS * time.Millisecond
	// A hub that does not answer a connection attempt within this time counts
	// as unreachable, so the agent is not kept waiting.
	proxyDialTimeout = 5 * time.Second
	// proxyResponseHeaderTimeout bounds the wait for the hub's answer.
	proxyResponseHeaderTimeout = 60 * time.Second
	// proxyGCInterval rate-limits cache garbage collection in one proxy.
	proxyGCInterval = time.Hour
	// proxyBundleTimeout bounds one bundle download, body included, so a
	// stalled hub cannot hold a skill's fetch lock indefinitely.
	proxyBundleTimeout = 2 * time.Minute
	// proxyMaxSkillMarkdown bounds the SKILL.md text returned from the cache.
	proxyMaxSkillMarkdown = skillruntime.BundleMaxBytes
)

var proxySkillIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ProxyOptions configures the stdio proxy that lets a local agent host use a
// hub on another machine.
type ProxyOptions struct {
	// HubURL is the hub's base URL, for example http://127.0.0.1:7420; the
	// MCP endpoint is HubURL + /mcp.
	HubURL string
	// Token is the shared bearer token. It is sent only in the Authorization
	// header and never written anywhere.
	Token string
	// StateDir is this machine's client state directory (see
	// skillruntime.ClientStateDir); fetched skills are cached below it.
	StateDir string
	// Warn receives plain-language warnings for the user, for example when the
	// hub cannot be reached. nil discards them. It never receives the token.
	Warn func(message string)
	// Transport, when set, is the base round tripper for every request to the
	// hub (tests count or delay requests with it). nil uses a keep-alive
	// transport that ignores proxy environment variables.
	Transport http.RoundTripper
}

// Proxy serves the runtime tools to a local agent host over stdio and
// forwards them to a remote hub. skill_resolve and skill_feedback are
// forwarded unchanged. skill_get is answered from a local cache of skill
// bundles keyed by the skill digest: a version is fetched once, in one
// request, verified against its digest, and extracted atomically; skill_get
// then returns local paths on this machine, so a skill's check, setup and
// scripts run here exactly as with a local hub. Nothing is ever executed by
// the proxy.
type Proxy struct {
	hubURL    string
	endpoint  string
	bundleURL string
	hubID     string
	cache     skillruntime.RemoteCache
	stateRoot string
	client    *http.Client
	server    *mcp.Server
	warn      func(string)
	now       func() time.Time

	connectMu sync.Mutex
	upstream  *mcp.ClientSession
	// sessionID mirrors upstream's ID so a bundle request never waits for a
	// connection attempt in progress.
	sessionID atomic.Value

	clientMu   sync.Mutex
	clientInfo *mcp.Implementation

	mu          sync.Mutex
	hints       map[string]skillHint
	fetchLocks  map[string]*sync.Mutex
	lastGC      time.Time
	unreachable bool

	// tokenRejected is set while the hub answers 401 to this machine's token.
	tokenRejected *atomic.Bool
}

// skillHint is what the latest resolution said about a skill.
type skillHint struct {
	digest         string
	reviewRequired bool
	at             time.Time
}

// NormalizeHubURL checks a hub base URL and returns it without a trailing
// slash. The URL must be http or https with a host, and carry no user
// information, query or fragment.
func NormalizeHubURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%q is not a URL", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%q must start with http:// or https://", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%q has no host", raw)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%q must not contain a user name or password; pass the token with --token-file", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%q must not contain a query or fragment", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, HTTPEndpointPath)
	return parsed.String(), nil
}

// NewProxy prepares a proxy for one hub. It does not contact the hub.
func NewProxy(ctx context.Context, options ProxyOptions) (*Proxy, error) {
	hubURL, err := NormalizeHubURL(options.HubURL)
	if err != nil {
		return nil, err
	}
	if options.Token == "" {
		return nil, errors.New("no hub token")
	}
	if options.StateDir == "" {
		return nil, errors.New("no client state directory")
	}
	hubID, err := skillruntime.HubID(hubURL)
	if err != nil {
		return nil, err
	}
	base := options.Transport
	if base == nil {
		base = newProxyTransport()
	}
	cache := skillruntime.RemoteCache{Root: skillruntime.RemoteCacheDir(options.StateDir)}
	proxy := &Proxy{
		hubURL:        hubURL,
		endpoint:      hubURL + HTTPEndpointPath,
		bundleURL:     hubURL + HTTPBundlePath,
		hubID:         hubID,
		cache:         cache,
		stateRoot:     cache.StateRoot(hubID),
		tokenRejected: &atomic.Bool{},
		warn:          options.Warn,
		now:           time.Now,
		hints:         map[string]skillHint{},
		fetchLocks:    map[string]*sync.Mutex{},
	}
	hubHost, err := url.Parse(hubURL)
	if err != nil {
		return nil, err
	}
	proxy.client = &http.Client{
		Transport: bearerRoundTripper{token: options.Token, host: hubHost.Host, base: base, rejected: proxy.tokenRejected},
		// A redirect is never followed: it could send the token, or a
		// different bundle, to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if proxy.warn == nil {
		proxy.warn = func(string) {}
	}
	if err := cache.RecordHubURL(hubID, hubURL); err != nil {
		return nil, fmt.Errorf("prepare the skill cache in %s: %w", cache.Root, err)
	}
	tools, err := runtimeToolDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "skillhub", Version: "1"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		PageSize:     paging.MaximumLimit,
	})
	for _, tool := range tools {
		switch tool.Name {
		case "skill_get":
			server.AddTool(tool, proxy.skillGet)
		case "skill_resolve":
			server.AddTool(tool, proxy.resolve)
		default:
			name := tool.Name
			server.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return proxy.forward(ctx, name, request)
			})
		}
	}
	proxy.server = server
	return proxy, nil
}

// HubID returns the cache directory name of the proxied hub.
func (proxy *Proxy) HubID() string { return proxy.hubID }

// CacheDir returns the directory holding this hub's cached skills.
func (proxy *Proxy) CacheDir() string { return proxy.cache.HubDir(proxy.hubID) }

// Run serves one agent host session on transport until it ends or ctx is
// cancelled, then closes the connection to the hub.
func (proxy *Proxy) Run(ctx context.Context, transport mcp.Transport) error {
	defer proxy.close()
	err := proxy.server.Run(ctx, transport)
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// ServeProxy runs the proxy on this process's stdin and stdout.
func ServeProxy(ctx context.Context, options ProxyOptions) error {
	proxy, err := NewProxy(ctx, options)
	if err != nil {
		return err
	}
	return proxy.Run(ctx, &mcp.StdioTransport{MaxLineLength: MaxFrameBytes})
}

func (proxy *Proxy) close() {
	proxy.connectMu.Lock()
	defer proxy.connectMu.Unlock()
	if proxy.upstream != nil {
		_ = proxy.upstream.Close()
		proxy.upstream = nil
		proxy.sessionID.Store("")
	}
	proxy.client.CloseIdleConnections()
}

// noteClient remembers the agent host's name from its session, so the hub
// connection, opened on the first call, carries the real host name.
func (proxy *Proxy) noteClient(request *mcp.CallToolRequest) {
	if request == nil || request.Session == nil {
		return
	}
	params := request.Session.InitializeParams()
	if params == nil || params.ClientInfo == nil {
		return
	}
	proxy.clientMu.Lock()
	defer proxy.clientMu.Unlock()
	if proxy.clientInfo == nil {
		info := *params.ClientInfo
		proxy.clientInfo = &info
	}
}

// newProxyTransport keeps connections to the hub alive between calls. HTTP/2
// needs TLS, which this transport does not use (the network is an SSH tunnel
// or a private VPN), so one kept-alive HTTP/1.1 connection carries the calls.
// Proxy environment variables are ignored: the hub is on a private network.
func newProxyTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: proxyDialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: proxyResponseHeaderTimeout,
	}
}

// bearerRoundTripper adds the token to requests to the hub's own host, and
// notes whether the hub currently refuses it.
type bearerRoundTripper struct {
	token    string
	host     string
	base     http.RoundTripper
	rejected *atomic.Bool
}

func (transport bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	if strings.EqualFold(request.URL.Host, transport.host) {
		clone.Header.Set("Authorization", "Bearer "+transport.token)
	}
	response, err := transport.base.RoundTrip(clone)
	if err == nil && transport.rejected != nil {
		transport.rejected.Store(response.StatusCode == http.StatusUnauthorized)
	}
	return response, err
}

// CloseIdleConnections lets http.Client close the base transport's idle
// connections.
func (transport bearerRoundTripper) CloseIdleConnections() {
	if closer, ok := transport.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// runtimeToolDefinitions returns the runtime tools exactly as a hub lists
// them, by registering them on a throwaway server and listing them in memory.
// The proxy therefore offers the same names, descriptions and schemas.
func runtimeToolDefinitions(ctx context.Context) ([]*mcp.Tool, error) {
	adapter := &Server{profile: ProfileRuntime}
	server := mcp.NewServer(&mcp.Implementation{Name: "skillhub", Version: "1"}, &mcp.ServerOptions{PageSize: paging.MaximumLimit})
	adapter.registerRuntimeTools(server)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "skillhub-proxy", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	var tools []*mcp.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// session returns the MCP session with the hub, connecting on first use.
func (proxy *Proxy) session(ctx context.Context) (*mcp.ClientSession, error) {
	proxy.connectMu.Lock()
	defer proxy.connectMu.Unlock()
	if proxy.upstream != nil {
		return proxy.upstream, nil
	}
	info := &mcp.Implementation{Name: "skillhub-proxy", Version: "1"}
	proxy.clientMu.Lock()
	if proxy.clientInfo != nil {
		info = proxy.clientInfo
	}
	proxy.clientMu.Unlock()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             proxy.endpoint,
		HTTPClient:           proxy.client,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	session, err := mcp.NewClient(info, nil).Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	proxy.upstream = session
	proxy.sessionID.Store(session.ID())
	proxy.noteReachable(true)
	return session, nil
}

// currentSessionID returns the hub session ID without connecting or waiting.
func (proxy *Proxy) currentSessionID() string {
	id, _ := proxy.sessionID.Load().(string)
	return id
}

func (proxy *Proxy) dropSession(session *mcp.ClientSession) {
	proxy.connectMu.Lock()
	defer proxy.connectMu.Unlock()
	if proxy.upstream == session {
		_ = session.Close()
		proxy.upstream = nil
		proxy.sessionID.Store("")
	}
}

// noteReachable warns once when the hub stops answering and once when it
// answers again.
func (proxy *Proxy) noteReachable(reachable bool) {
	proxy.mu.Lock()
	changed := proxy.unreachable == reachable
	proxy.unreachable = !reachable
	proxy.mu.Unlock()
	if !changed {
		return
	}
	if reachable {
		proxy.warn("The Skill Hub at " + proxy.hubURL + " answers again.")
		return
	}
	proxy.warn("The Skill Hub at " + proxy.hubURL + " cannot be reached. Agents continue without skills, except skills already cached on this machine. Check that the hub is running and that the SSH tunnel or VPN is up.")
}

// callHub calls a tool on the hub. A broken or expired session is replaced
// once, transparently; every forwarded tool is safe to repeat (resolution and
// skill_get read, feedback is deduplicated by event_id).
func (proxy *Proxy) callHub(ctx context.Context, name string, arguments json.RawMessage) (*mcp.CallToolResult, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		session, err := proxy.session(ctx)
		if err != nil {
			return nil, &hubUnreachableError{err: err}
		}
		params := &mcp.CallToolParams{Name: name}
		if len(arguments) > 0 {
			params.Arguments = arguments
		}
		result, err := session.CallTool(ctx, params)
		if err == nil {
			return result, nil
		}
		if answer, ok := hubAnswer(err); ok {
			return nil, answer
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		proxy.dropSession(session)
	}
	return nil, lastErr
}

// hubUnreachableError wraps a failure to reach the hub at all.
type hubUnreachableError struct{ err error }

func (e *hubUnreachableError) Error() string { return "the hub could not be reached: " + e.err.Error() }
func (e *hubUnreachableError) Unwrap() error { return e.err }

// hubAnswer returns the JSON-RPC error the hub itself answered with. The SDK
// also reports transport and closing failures as JSON-RPC errors, with codes
// outside the standard range; those mean the hub was not reached.
func hubAnswer(err error) (*jsonrpc.Error, bool) {
	var unreachable *hubUnreachableError
	if errors.As(err, &unreachable) {
		return nil, false
	}
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		return nil, false
	}
	switch rpcErr.Code {
	case jsonrpc.CodeParseError, jsonrpc.CodeInvalidRequest, jsonrpc.CodeMethodNotFound, jsonrpc.CodeInvalidParams, jsonrpc.CodeInternalError:
		return rpcErr, true
	}
	return nil, false
}

func (proxy *Proxy) forward(ctx context.Context, name string, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	proxy.noteClient(request)
	var arguments json.RawMessage
	if request != nil && request.Params != nil {
		arguments = request.Params.Arguments
	}
	result, err := proxy.callHub(ctx, name, arguments)
	if err != nil {
		if _, ok := hubAnswer(err); ok {
			return nil, err
		}
		if ctx.Err() != nil {
			// The agent host cancelled the call or its deadline passed.
			return nil, ctx.Err()
		}
		if proxy.tokenRejected.Load() {
			return proxy.tokenRejectedResult(), nil
		}
		proxy.noteReachable(false)
		return proxy.hubUnavailable(), nil
	}
	return result, nil
}

// resolve forwards skill_resolve and remembers the version each recommended
// skill has, so the skill_get that normally follows can use the cache without
// asking the hub.
func (proxy *Proxy) resolve(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := proxy.forward(ctx, "skill_resolve", request)
	if err == nil && result != nil && !result.IsError {
		proxy.noteHints(result.StructuredContent)
	}
	return result, err
}

func (proxy *Proxy) noteHints(structured any) {
	encoded, err := json.Marshal(structured)
	if err != nil {
		return
	}
	type recommended struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Setup   *struct {
			State string `json:"state"`
		} `json:"setup"`
	}
	var outcome struct {
		Result *struct {
			Resolution struct {
				Primary    *recommended  `json:"primary"`
				Supporting []recommended `json:"supporting"`
			} `json:"resolution"`
		} `json:"result"`
	}
	if json.Unmarshal(encoded, &outcome) != nil || outcome.Result == nil {
		return
	}
	items := outcome.Result.Resolution.Supporting
	if primary := outcome.Result.Resolution.Primary; primary != nil {
		items = append(items, *primary)
	}
	now := proxy.now()
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	for _, item := range items {
		digest := strings.TrimPrefix(item.Version, "sha256:")
		if !proxySkillIDPattern.MatchString(item.ID) || !skillruntime.ValidBundleDigest(digest) {
			continue
		}
		proxy.hints[item.ID] = skillHint{
			digest:         digest,
			reviewRequired: item.Setup != nil && item.Setup.State == skillruntime.StateReviewRequired,
			at:             now,
		}
	}
}

func (proxy *Proxy) hint(skillID string) (skillHint, bool) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	hint, ok := proxy.hints[skillID]
	if !ok || proxy.now().Sub(hint.at) > proxyHintTTL {
		return skillHint{}, false
	}
	return hint, true
}

func (proxy *Proxy) fetchLock(skillID string) *sync.Mutex {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	lock, ok := proxy.fetchLocks[skillID]
	if !ok {
		lock = &sync.Mutex{}
		proxy.fetchLocks[skillID] = lock
	}
	return lock
}

// Cache states reported in the remote field of a skill_get result.
const (
	remoteCacheHit     = "hit"
	remoteCacheFetched = "fetched"
	remoteCacheOffline = "offline"
)

// remoteDelivery tells the agent where the skill came from.
type remoteDelivery struct {
	Hub     string `json:"hub"`
	Cache   string `json:"cache"`
	Message string `json:"message,omitempty"`
}

// proxySkillGetResult is a skill_get result whose local block points at the
// cached copy on this machine.
type proxySkillGetResult struct {
	skillGetResult
	Remote *remoteDelivery `json:"remote,omitempty"`
}

// skillGet answers skill_get from the cache, fetching the skill bundle when
// the version is not cached. Skills that cannot be bundled (drafts, skills
// awaiting review, the bundled curator) are forwarded, so the hub answers
// them exactly as it answers a local caller.
func (proxy *Proxy) skillGet(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input skillGetInput
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		_ = json.Unmarshal(request.Params.Arguments, &input)
	}
	skillID := strings.TrimSpace(input.SkillID)
	if !proxySkillIDPattern.MatchString(skillID) {
		return proxy.forward(ctx, "skill_get", request)
	}
	lock := proxy.fetchLock(skillID)
	lock.Lock()
	defer lock.Unlock()
	hint, hinted := proxy.hint(skillID)
	if hinted && !hint.reviewRequired {
		if cached, ok := proxy.cache.Lookup(proxy.hubID, skillID, hint.digest); ok {
			return proxy.localResult(cached, remoteCacheHit, "")
		}
	}
	outcome := proxy.fetchBundle(ctx, skillID)
	switch outcome.kind {
	case fetchInstalled:
		proxy.rememberVersion(skillID, outcome.cached.Digest)
		return proxy.localResult(outcome.cached, remoteCacheFetched, "")
	case fetchNotModified:
		proxy.rememberVersion(skillID, outcome.cached.Digest)
		return proxy.localResult(outcome.cached, remoteCacheHit, "")
	case fetchForward:
		return proxy.forward(ctx, "skill_get", request)
	case fetchUnreachable:
		proxy.noteReachable(false)
		// A resolution that just said the skill awaits review rules out any
		// cached copy, even with the hub gone.
		if hinted && hint.reviewRequired {
			return proxy.hubUnavailable(), nil
		}
		if latest, ok := proxy.cache.Latest(proxy.hubID, skillID); ok {
			return proxy.localResult(latest, remoteCacheOffline, "The hub could not be reached, so this is the copy of the skill cached on this machine; it may be older than the hub's current version.")
		}
		return proxy.hubUnavailable(), nil
	default:
		if outcome.code == "" {
			// The agent host cancelled the call.
			return nil, outcome.err
		}
		return proxy.toolFailure(outcome.code, outcome.message, outcome.action), nil
	}
}

// rememberVersion records the version the hub just confirmed as current, like
// a resolution does, so the next skill_get in this session needs no request.
func (proxy *Proxy) rememberVersion(skillID, digest string) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	proxy.hints[skillID] = skillHint{digest: digest, at: proxy.now()}
}

type fetchKind int

const (
	fetchFailed fetchKind = iota
	fetchInstalled
	fetchNotModified
	fetchForward
	fetchUnreachable
)

type fetchOutcome struct {
	kind    fetchKind
	cached  skillruntime.CachedSkill
	err     error
	code    string
	message string
	action  string
}

// fetchBundle asks the hub for the current version of a skill in one request,
// naming the versions already cached, and installs a new version.
func (proxy *Proxy) fetchBundle(ctx context.Context, skillID string) fetchOutcome {
	ctx, cancel := context.WithTimeout(ctx, proxyBundleTimeout)
	defer cancel()
	var held []string
	for _, version := range proxy.cache.Versions(proxy.hubID, skillID) {
		held = append(held, version.Digest)
	}
	for attempt := 0; attempt < 2; attempt++ {
		outcome, retry := proxy.requestBundle(ctx, skillID, held)
		if !retry {
			return outcome
		}
		// The hub said a held version is current but it vanished from the
		// cache meanwhile (for example a concurrent cache clear): fetch it.
		held = nil
	}
	return fetchOutcome{kind: fetchFailed, code: "internal_error", message: "The cached skill kept disappearing while it was being used.", action: "Retry once."}
}

func (proxy *Proxy) requestBundle(ctx context.Context, skillID string, held []string) (fetchOutcome, bool) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.bundleURL+"?skill="+url.QueryEscape(skillID), nil)
	if err != nil {
		return fetchOutcome{kind: fetchFailed, err: err, code: "internal_error", message: "The bundle request could not be built.", action: "Retry once."}, false
	}
	request.Header.Set("Accept", skillruntime.BundleMediaType)
	if len(held) > 0 {
		tags := make([]string, 0, len(held))
		for _, digest := range held {
			tags = append(tags, `"`+digest+`"`)
		}
		request.Header.Set("If-None-Match", strings.Join(tags, ", "))
	}
	if id := proxy.currentSessionID(); id != "" {
		request.Header.Set("Mcp-Session-Id", id)
	}
	response, err := proxy.client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return fetchOutcome{kind: fetchFailed, err: ctx.Err()}, false
		}
		return fetchOutcome{kind: fetchUnreachable, err: err}, false
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		_ = response.Body.Close()
	}()
	proxy.noteReachable(true)
	digest := strings.Trim(strings.TrimPrefix(response.Header.Get("ETag"), "W/"), `"`)
	switch response.StatusCode {
	case http.StatusOK:
		if !skillruntime.ValidBundleDigest(digest) {
			return fetchOutcome{kind: fetchFailed, code: "hub_response_invalid", message: "The hub sent a skill bundle without a valid digest.", action: "Check that the hub runs the same skillhub version as this machine."}, false
		}
		cached, err := proxy.cache.Install(proxy.hubID, skillID, digest, response.Body)
		if err != nil {
			if errors.Is(err, skillruntime.ErrBundleDigestMismatch) || errors.Is(err, skillruntime.ErrBundleInvalid) {
				return fetchOutcome{kind: fetchFailed, err: err, code: "resource_digest_mismatch", message: "The skill bundle failed its integrity check and was discarded.", action: "Retry once; if it fails again, check the connection to the hub."}, false
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return fetchOutcome{kind: fetchFailed, err: ctx.Err()}, false
			}
			if ctx.Err() != nil {
				return fetchOutcome{kind: fetchUnreachable, err: ctx.Err()}, false
			}
			return fetchOutcome{kind: fetchFailed, err: err, code: "internal_error", message: "The skill could not be stored in the local cache " + proxy.CacheDir() + ".", action: "Check free disk space and the permissions of that directory."}, false
		}
		proxy.collectGarbage(skillID, digest)
		return fetchOutcome{kind: fetchInstalled, cached: cached}, false
	case http.StatusNotModified:
		cached, ok := proxy.cache.Lookup(proxy.hubID, skillID, digest)
		if !ok {
			return fetchOutcome{}, true
		}
		return fetchOutcome{kind: fetchNotModified, cached: cached}, false
	case http.StatusUnauthorized:
		return fetchOutcome{kind: fetchFailed, code: hubTokenRejectedCode, message: hubTokenRejectedMessage, action: hubTokenRejectedAction}, false
	case http.StatusForbidden, http.StatusNotFound:
		// A skill awaiting review, a draft, the bundled curator, or a hub
		// without bundles: the hub's own skill_get answer is the right one.
		var body bundleError
		_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body)
		if response.StatusCode == http.StatusForbidden && body.Code != "content_review_required" {
			return fetchOutcome{kind: fetchFailed, code: "hub_refused", message: "The hub refused the request (HTTP 403): this machine's name for it is not accepted.", action: "Start the hub with --allow-host <the name used here>, or connect with an address the hub accepts."}, false
		}
		return fetchOutcome{kind: fetchForward}, false
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusInternalServerError:
		// The hub answered but cannot serve the skill now (busy, catalog
		// stale, or a file changed on disk). Only an unreachable hub falls
		// back to the cache; an answer like this may concern trust or content.
		return fetchOutcome{kind: fetchFailed, code: "hub_error", message: fmt.Sprintf("The hub could not serve this skill right now (HTTP %d).", response.StatusCode), action: "Continue without the skill and retry later; the hub's diagnostics name the cause."}, false
	default:
		return fetchOutcome{kind: fetchFailed, code: "hub_response_invalid", message: fmt.Sprintf("The hub answered the skill request with HTTP %d.", response.StatusCode), action: "Check that the hub runs the same skillhub version as this machine."}, false
	}
}

// collectGarbage trims this hub's cache at most once an hour, keeping the
// version just installed.
func (proxy *Proxy) collectGarbage(skillID, digest string) {
	now := proxy.now()
	proxy.mu.Lock()
	due := now.Sub(proxy.lastGC) >= proxyGCInterval
	if due {
		proxy.lastGC = now
	}
	proxy.mu.Unlock()
	if due {
		// A failed collection only leaves more files cached.
		_, _ = proxy.cache.CollectGarbage(proxy.hubID, skillruntime.DefaultGCPolicy, now, skillID, digest)
	}
}

// localResult builds the skill_get answer for a cached version: the hub's
// skill detail, SKILL.md from the cached folder, and a local block whose
// paths and variables point at this machine.
func (proxy *Proxy) localResult(cached skillruntime.CachedSkill, cacheState, message string) (*mcp.CallToolResult, error) {
	var detail app.SkillDetail
	if len(cached.Metadata.Detail) > 0 {
		if err := json.Unmarshal(cached.Metadata.Detail, &detail); err != nil {
			return proxy.toolFailure("internal_error", "The cached skill metadata could not be read.", "Run `skillhub cache clear --hub --yes` and retry."), nil
		}
	}
	content, err := readCachedMarkdown(cached.Directory)
	if err != nil {
		return proxy.toolFailure("internal_error", "The cached SKILL.md could not be read.", "Run `skillhub cache clear --hub --yes` and retry."), nil
	}
	local, err := app.ClientLocalSkill(proxy.stateRoot, cached.Directory, cached.Metadata)
	if err != nil {
		return proxy.toolFailure("internal_error", "The skill's state directory on this machine could not be prepared in "+proxy.stateRoot+".", "Check the permissions of that directory and retry."), nil
	}
	result := proxySkillGetResult{
		skillGetResult: skillGetResult{SkillDetail: detail, Content: content, Local: &local},
		Remote:         &remoteDelivery{Hub: proxy.hubURL, Cache: cacheState, Message: message},
	}
	return structuredResult(toolOutcome[proxySkillGetResult]{SchemaVersion: SchemaVersion, Result: &result}, false)
}

func readCachedMarkdown(directory string) (string, error) {
	file, err := os.Open(filepath.Join(directory, "SKILL.md"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, proxyMaxSkillMarkdown))
	return string(data), err
}

const (
	hubTokenRejectedCode    = "hub_token_rejected"
	hubTokenRejectedMessage = "The hub refused the token of this machine, so no skill is available right now."
	hubTokenRejectedAction  = "Continue the task without a skill. To restore skills, copy the current token file from the hub machine and pass it with --token-file."
)

func (proxy *Proxy) tokenRejectedResult() *mcp.CallToolResult {
	return proxy.toolFailure(hubTokenRejectedCode, hubTokenRejectedMessage, hubTokenRejectedAction)
}

func (proxy *Proxy) hubUnavailable() *mcp.CallToolResult {
	return proxy.toolFailure("hub_unreachable",
		"The Skill Hub at "+proxy.hubURL+" could not be reached, so no skill is available right now.",
		"Continue the task without a skill. To restore skills, check that the hub is running and that the SSH tunnel or VPN to it is up.")
}

func (proxy *Proxy) toolFailure(code, message, action string) *mcp.CallToolResult {
	item := toolError{Code: code, Message: message, Retryable: code == "hub_unreachable" || code == "hub_error" || code == "resource_digest_mismatch", SuggestedAction: action}
	result, err := structuredResult(toolOutcome[proxySkillGetResult]{SchemaVersion: SchemaVersion, Error: &item}, true)
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
	}
	return result
}

// structuredResult returns value as structured content with the same JSON
// in a text block, the way the hub's own tools answer.
func structuredResult(value any, isError bool) (*mcp.CallToolResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		IsError:           isError,
		StructuredContent: json.RawMessage(encoded),
		Content:           []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
	}, nil
}
