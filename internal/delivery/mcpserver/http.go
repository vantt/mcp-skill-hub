package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"golang.org/x/time/rate"
)

// HTTPEndpointPath is the only path the network listener serves.
const HTTPEndpointPath = "/mcp"

const (
	// One user runs up to about 20 agents at once; the cap leaves room for
	// sessions of crashed clients that wait for the idle timeout.
	httpMaxSessions = 128
	// An idle session is closed after this long; the client starts a new one.
	httpSessionIdleTimeout = time.Hour
	// The shared token allows this many requests per second on average, with
	// bursts up to httpRequestBurst. Agents call a few times per task.
	httpRequestsPerSecond = 100
	httpRequestBurst      = 200
	// A remote address that fails authentication this many times within
	// httpAuthFailureWindow is refused until the window passes.
	httpAuthFailureLimit  = 20
	httpAuthFailureWindow = time.Minute
	httpAuthFailureKeys   = 1024
	httpShutdownDrain     = 3 * time.Second
	httpShutdownTimeout   = 5 * time.Second
	httpReadHeaderTimeout = 10 * time.Second
	httpKeepAliveIdle     = 2 * time.Minute
	httpMaxHeaderBytes    = 64 << 10
)

// HTTPOptions configures the network listener.
type HTTPOptions struct {
	Workspace string
	// TokenFile holds the shared bearer token; it must exist before serving.
	TokenFile string
	// AllowHosts adds Host header names (for example a VPN DNS name) to the
	// names the shared Host policy accepts.
	AllowHosts []string
	// Diagnostics receives warnings; nil discards them. It never receives the
	// token or request content.
	Diagnostics io.Writer
	// Ready, when set, is called once the server accepts requests on every
	// listener, with the addresses the listeners are bound to.
	Ready func(addresses []net.Addr)
	// Invites redeems the one-time codes presented to POST /mcp/pair; nil
	// turns that route off (404).
	Invites Redeemer
	// StrictPairing makes the pair route answer only connections that stay
	// on loopback or Tailscale and carry no proxy header. Without it the route
	// answers any connection the listener accepts.
	StrictPairing bool
}

// ServeHTTP serves the runtime tools (skill_resolve, skill_get,
// skill_feedback) over Streamable HTTP on every listener until ctx ends: one
// HTTP server, one token and one Host policy for all of them. No curation
// tool is registered on this server. Every request needs the shared bearer
// token in the Authorization header. If one listener fails, the server stops
// on all of them. On shutdown it closes all sessions, stops the HTTP server,
// and flushes the telemetry recorder.
func ServeHTTP(ctx context.Context, listeners []net.Listener, options HTTPOptions) error {
	if len(listeners) == 0 {
		return errors.New("no listener")
	}
	addresses := make([]net.Addr, len(listeners))
	for index, listener := range listeners {
		if listener == nil {
			return errors.New("no listener")
		}
		addresses[index] = listener.Addr()
	}
	tokens := hubtoken.NewSource(options.TokenFile)
	if err := tokens.Check(); err != nil {
		return fmt.Errorf("read the hub token: %w", err)
	}
	adapter, server, stop, err := startServing(ctx, options.Workspace, options.Diagnostics, ProfileRuntime)
	if err != nil {
		return err
	}
	defer stop()
	// Every session of this server is a remote caller; set before the first
	// request so telemetry labels its client as remote and responses carry no
	// path on this machine's disk.
	adapter.remote = true

	hostname, _ := os.Hostname()
	hosts := hostpolicy.New(hostpolicy.Options{Listeners: addresses, AllowHosts: options.AllowHosts, Hostname: hostname})
	gate := newHTTPGate(server, tokens, hosts, originHosts(addresses, options.AllowHosts), adapter.logger)
	gate.bundle = adapter.bundleHTTPHandler(server)
	gate.invites = options.Invites
	gate.strictPairing = options.StrictPairing
	httpServer := &http.Server{
		Handler:           gate,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpKeepAliveIdle,
		MaxHeaderBytes:    httpMaxHeaderBytes,
		ErrorLog:          httpErrorLog(adapter.logger),
	}
	served := make(chan error, len(listeners))
	for _, listener := range listeners {
		go func() { served <- httpServer.Serve(listener) }()
	}
	if options.Ready != nil {
		options.Ready(addresses)
	}
	// remaining counts the Serve calls that have not returned yet.
	remaining := len(listeners)
	firstError := func(errs ...error) error {
		for _, err := range errs {
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}
		return nil
	}

	select {
	case err := <-served:
		// One listener stopped on its own: stop the others too.
		remaining--
		gate.closeSessions()
		_ = httpServer.Close()
		errs := []error{err}
		for ; remaining > 0; remaining-- {
			errs = append(errs, <-served)
		}
		return firstError(errs...)
	case <-ctx.Done():
	}
	// Refuse new requests, let calls already running finish (so a feedback
	// write is not cut off), then close the sessions, which also ends their
	// long-lived event streams so the HTTP server can stop.
	gate.beginShutdown()
	gate.drain(httpShutdownDrain)
	gate.closeSessions()
	// The serve context has ended; shutdown needs its own deadline.
	shutdownContext, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	shutdownErr := httpServer.Shutdown(shutdownContext)
	if errors.Is(shutdownErr, context.DeadlineExceeded) {
		_ = httpServer.Close()
		shutdownErr = nil
	}
	var errs []error
	for ; remaining > 0; remaining-- {
		errs = append(errs, <-served)
	}
	if err := firstError(errs...); err != nil {
		return err
	}
	return shutdownErr
}

// httpGate checks every request before it reaches the MCP handler: Host and
// Origin, path, shutdown state, authentication throttle, bearer token, rate
// limit, and the session cap. The token is compared in constant time and is
// never logged or echoed.
type httpGate struct {
	server        *mcp.Server
	handler       http.Handler
	bundle        http.Handler
	invites       Redeemer
	strictPairing bool
	tokens        *hubtoken.Source
	hosts         *hostpolicy.Policy
	// origins is the strict Origin set; it does not follow the Host policy's
	// acceptance of any IP literal (see originHosts).
	origins  map[string]bool
	limiter  *rate.Limiter
	failures *authFailures
	// maxSessions caps open sessions; a new session beyond it gets 503. The
	// count and the new session are not atomic, so concurrent first requests
	// may pass it by a few; the activation tracker stays bounded regardless.
	maxSessions int
	closing     atomic.Bool
	// inFlight counts requests other than long-lived event streams (GET) that
	// passed the shutdown check and have not finished yet.
	inFlight atomic.Int64
	logger   *slog.Logger
}

func newHTTPGate(server *mcp.Server, tokens *hubtoken.Source, hosts *hostpolicy.Policy, origins map[string]bool, logger *slog.Logger) *httpGate {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		SessionTimeout:      httpSessionIdleTimeout,
		MaxRequestBodyBytes: MaxFrameBytes,
		Logger:              logger,
		// The gate already checks every Host against the names of this hub.
		// The SDK's own check would also refuse an --allow-host name reaching
		// a loopback listener (for example through a local reverse proxy).
		DisableLocalhostProtection: true,
	})
	return &httpGate{
		server:      server,
		handler:     handler,
		tokens:      tokens,
		hosts:       hosts,
		origins:     origins,
		limiter:     rate.NewLimiter(rate.Limit(httpRequestsPerSecond), httpRequestBurst),
		failures:    newAuthFailures(time.Now),
		maxSessions: httpMaxSessions,
		logger:      logger,
	}
}

func (gate *httpGate) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !gate.hosts.Allows(request.Host) {
		http.Error(writer, "Forbidden: the Host header is not a name of this hub; add it with --allow-host", http.StatusForbidden)
		return
	}
	if origin := request.Header.Get("Origin"); origin != "" && !gate.originAllowed(origin, request.Host) {
		http.Error(writer, "Forbidden: cross-origin requests are not accepted", http.StatusForbidden)
		return
	}
	if request.URL.Path != HTTPEndpointPath && request.URL.Path != HTTPStatusPath &&
		(request.URL.Path != HTTPBundlePath || gate.bundle == nil) &&
		(request.URL.Path != HTTPPairPath || gate.invites == nil) {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodGet {
		gate.inFlight.Add(1)
		defer gate.inFlight.Add(-1)
	}
	if gate.closing.Load() {
		writer.Header().Set("Connection", "close")
		http.Error(writer, "the hub is shutting down", http.StatusServiceUnavailable)
		return
	}
	// Pairing carries no bearer token: the invite is its credential, checked by
	// the handler behind its own transport check and failure budget.
	if request.URL.Path == HTTPPairPath {
		gate.servePair(writer, request)
		return
	}
	// A valid token always passes. The throttle slows down only requests
	// without it: behind an SSH tunnel every client shares 127.0.0.1, so one
	// agent left with a rotated-out token must not lock the others out.
	if !gate.tokens.Valid(bearerToken(request.Header.Get("Authorization"))) {
		remote := remoteHost(request.RemoteAddr)
		if gate.failures.blocked(remote) {
			writer.Header().Set("Retry-After", fmt.Sprint(int(httpAuthFailureWindow.Seconds())))
			http.Error(writer, "too many failed authentication attempts; wait a minute", http.StatusTooManyRequests)
			return
		}
		gate.failures.record(remote)
		writer.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
		http.Error(writer, "Unauthorized: send the hub token as `Authorization: Bearer <token>`", http.StatusUnauthorized)
		return
	}
	if !gate.limiter.Allow() {
		writer.Header().Set("Retry-After", "1")
		http.Error(writer, "too many requests; slow down", http.StatusTooManyRequests)
		return
	}
	if request.URL.Path == HTTPBundlePath {
		gate.bundle.ServeHTTP(writer, request)
		return
	}
	if request.URL.Path == HTTPStatusPath {
		serveHubStatus(writer, request)
		return
	}
	if request.Method == http.MethodPost && request.Header.Get("Mcp-Session-Id") == "" && gate.sessionCount() >= gate.maxSessions {
		writer.Header().Set("Retry-After", "30")
		http.Error(writer, "the hub has too many open sessions; close idle agents or retry later", http.StatusServiceUnavailable)
		return
	}
	gate.handler.ServeHTTP(writer, request)
}

func (gate *httpGate) sessionCount() int {
	count := 0
	for range gate.server.Sessions() {
		count++
	}
	return count
}

// beginShutdown makes the gate refuse every new request with 503.
func (gate *httpGate) beginShutdown() { gate.closing.Store(true) }

// drain waits until the requests that passed the shutdown check have
// finished, or until grace has passed. Because a request is counted before it
// checks the shutdown flag, no request can start a session after drain sees
// zero.
func (gate *httpGate) drain(grace time.Duration) {
	deadline := time.Now().Add(grace)
	for gate.inFlight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

// closeSessions refuses new requests and closes every open session, which
// also ends their long-lived event streams so the HTTP server can stop.
func (gate *httpGate) closeSessions() {
	gate.beginShutdown()
	for _, session := range slices.Collect(gate.server.Sessions()) {
		_ = session.Close()
	}
}

// originAllowed accepts an Origin that names a bound address, loopback,
// localhost, an --allow-host name, or the request's own Host (a same-origin
// request). A browser page served from any other IP address is refused even
// when its Host header would pass, so a page on another machine of the
// network cannot drive the listener through a visitor's browser.
func (gate *httpGate) originAllowed(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	host := normalizeHost(parsed.Host)
	return gate.origins[host] || host == normalizeHost(requestHost)
}

// bearerToken extracts the credential of an `Authorization: Bearer` header.
// The scheme is case-insensitive (RFC 9110); nothing else carries a token.
func bearerToken(header string) string {
	scheme, credential, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(credential)
}

// normalizeHost lowercases a Host header value and drops its port, brackets,
// and trailing dot. The port is ignored on purpose: an SSH tunnel may forward
// a different local port to the hub, and the check guards against DNS
// rebinding, which is about the name.
func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	return strings.TrimSuffix(strings.ToLower(value), ".")
}

// networkAddressPattern matches an IPv4 address with an optional port and a
// bracketed IPv6 address with a port, the forms net/http prints for a peer.
var networkAddressPattern = regexp.MustCompile(`\[[0-9A-Fa-f:.%]+\]:[0-9]+|\b[0-9]{1,3}(?:\.[0-9]{1,3}){3}(?::[0-9]+)?\b`)

// httpErrorLog routes net/http's own error log to the diagnostics logger with
// network addresses removed. net/http names the client address in some lines,
// for example when it recovers a handler panic ("http: panic serving
// 192.0.2.1:5000: ..."), and the hub never keeps a client address.
func httpErrorLog(logger *slog.Logger) *log.Logger {
	return log.New(addressScrubbingWriter{logger: logger}, "", 0)
}

type addressScrubbingWriter struct{ logger *slog.Logger }

func (writer addressScrubbingWriter) Write(line []byte) (int, error) {
	message := networkAddressPattern.ReplaceAllString(strings.TrimRight(string(line), "\n"), "<address>")
	writer.logger.Warn(message)
	return len(line), nil
}

func remoteHost(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return remoteAddress
	}
	return host
}

// originHosts lists the names an Origin header may carry: the loopback names,
// each listen IP (every interface IP for a wildcard address), and the extra
// names. The list is fixed at start-up: if an interface address changes,
// restart the listener.
func originHosts(addresses []net.Addr, extra []string) map[string]bool {
	hosts := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	for _, address := range addresses {
		tcp, ok := address.(*net.TCPAddr)
		if !ok {
			continue
		}
		if tcp.IP == nil || tcp.IP.IsUnspecified() {
			if interfaceAddresses, err := net.InterfaceAddrs(); err == nil {
				for _, item := range interfaceAddresses {
					if network, ok := item.(*net.IPNet); ok {
						hosts[network.IP.String()] = true
					}
				}
			}
			continue
		}
		hosts[tcp.IP.String()] = true
	}
	for _, name := range extra {
		if normalized := normalizeHost(name); normalized != "" {
			hosts[normalized] = true
		}
	}
	return hosts
}

// authFailures counts failed authentications per remote address.
type authFailures struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string][]time.Time
}

func newAuthFailures(now func() time.Time) *authFailures {
	return &authFailures{now: now, entries: make(map[string][]time.Time)}
}

func (failures *authFailures) blocked(remote string) bool {
	failures.mu.Lock()
	defer failures.mu.Unlock()
	return len(failures.recent(remote)) >= httpAuthFailureLimit
}

func (failures *authFailures) record(remote string) {
	failures.mu.Lock()
	defer failures.mu.Unlock()
	if _, known := failures.entries[remote]; !known && len(failures.entries) >= httpAuthFailureKeys {
		failures.evictOldest()
	}
	failures.entries[remote] = append(failures.recent(remote), failures.now())
}

// recent drops expired failures for remote and returns the rest. The caller
// holds the lock.
func (failures *authFailures) recent(remote string) []time.Time {
	cutoff := failures.now().Add(-httpAuthFailureWindow)
	kept := failures.entries[remote][:0]
	for _, at := range failures.entries[remote] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(failures.entries, remote)
		return nil
	}
	failures.entries[remote] = kept
	return kept
}

func (failures *authFailures) evictOldest() {
	oldestKey := ""
	var oldest time.Time
	for key, times := range failures.entries {
		last := times[len(times)-1]
		if oldestKey == "" || last.Before(oldest) {
			oldestKey, oldest = key, last
		}
	}
	delete(failures.entries, oldestKey)
}
