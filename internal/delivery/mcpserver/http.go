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
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
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

// ErrNonLoopbackAddress reports a listen address reachable from other machines
// while the private network has not been confirmed.
var ErrNonLoopbackAddress = errors.New("the listen address is not a loopback address")

// ErrWildcardAddress reports a listen address that binds every network interface.
// Trusting a private network does not make that safe, because the same listener
// would also answer on any public interface, so a specific address is required.
var ErrWildcardAddress = errors.New("the listen address binds every network interface")

// HTTPOptions configures the network listener.
type HTTPOptions struct {
	Workspace string
	// TokenFile holds the shared bearer token; it must exist before serving.
	TokenFile string
	// TrustPrivateNetwork allows a non-loopback listen address. The caller
	// asserts that the network is private (an SSH tunnel or a VPN such as
	// Tailscale), because the transport is plain HTTP.
	TrustPrivateNetwork bool
	// AllowHosts adds Host header names (for example a VPN DNS name) to the
	// loopback names, the listen address, and this machine's host name.
	AllowHosts []string
	// Diagnostics receives warnings; nil discards them. It never receives the
	// token or request content.
	Diagnostics io.Writer
	// Ready, when set, is called once the server accepts requests.
	Ready func(address net.Addr)
}

// CheckListenAddress validates a host:port listen address. A non-loopback
// address is refused with ErrNonLoopbackAddress unless trustPrivateNetwork is
// set, because the listener speaks plain HTTP and the task text of every
// request would cross the network unencrypted. Even then an address that binds
// every interface (0.0.0.0, [::] or no host) is refused with ErrWildcardAddress.
func CheckListenAddress(address string, trustPrivateNetwork bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address %q: use host:port, for example 127.0.0.1:7420", address)
	}
	if port == "" {
		return fmt.Errorf("invalid address %q: the port is missing", address)
	}
	if _, err := net.LookupPort("tcp", port); err != nil {
		return fmt.Errorf("invalid port in address %q", address)
	}
	if isLoopbackHost(host) {
		return nil
	}
	if !trustPrivateNetwork {
		return ErrNonLoopbackAddress
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		return ErrWildcardAddress
	}
	return nil
}

// IsLoopbackAddress reports whether a host:port listen address stays on this
// machine.
func IsLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// ServeHTTP serves the runtime tools (skill_resolve, skill_get,
// skill_feedback) over Streamable HTTP on listener until ctx ends. No curation
// tool is registered on this server. Every request needs the shared bearer
// token in the Authorization header. On shutdown it closes all sessions, stops
// the HTTP server, and flushes the telemetry recorder.
func ServeHTTP(ctx context.Context, listener net.Listener, options HTTPOptions) error {
	if listener == nil {
		return errors.New("no listener")
	}
	if err := CheckListenAddress(listener.Addr().String(), options.TrustPrivateNetwork); err != nil {
		return err
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

	gate := newHTTPGate(server, tokens, allowedHosts(listener.Addr(), options.AllowHosts), adapter.logger)
	gate.bundle = adapter.bundleHTTPHandler(server)
	httpServer := &http.Server{
		Handler:           gate,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpKeepAliveIdle,
		MaxHeaderBytes:    httpMaxHeaderBytes,
		ErrorLog:          httpErrorLog(adapter.logger),
	}
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	if options.Ready != nil {
		options.Ready(listener.Addr())
	}

	select {
	case err := <-served:
		gate.closeSessions()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
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
	if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return shutdownErr
}

// httpGate checks every request before it reaches the MCP handler: Host and
// Origin, path, shutdown state, authentication throttle, bearer token, rate
// limit, and the session cap. The token is compared in constant time and is
// never logged or echoed.
type httpGate struct {
	server   *mcp.Server
	handler  http.Handler
	bundle   http.Handler
	tokens   *hubtoken.Source
	hosts    map[string]bool
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

func newHTTPGate(server *mcp.Server, tokens *hubtoken.Source, hosts map[string]bool, logger *slog.Logger) *httpGate {
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
		limiter:     rate.NewLimiter(rate.Limit(httpRequestsPerSecond), httpRequestBurst),
		failures:    newAuthFailures(time.Now),
		maxSessions: httpMaxSessions,
		logger:      logger,
	}
}

func (gate *httpGate) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !gate.hosts[normalizeHost(request.Host)] {
		http.Error(writer, "Forbidden: the Host header is not a name of this hub; add it with --allow-host", http.StatusForbidden)
		return
	}
	if origin := request.Header.Get("Origin"); origin != "" && !gate.originAllowed(origin) {
		http.Error(writer, "Forbidden: cross-origin requests are not accepted", http.StatusForbidden)
		return
	}
	if request.URL.Path != HTTPEndpointPath && request.URL.Path != HTTPStatusPath && (request.URL.Path != HTTPBundlePath || gate.bundle == nil) {
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

func (gate *httpGate) originAllowed(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	return gate.hosts[normalizeHost(parsed.Host)]
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

// allowedHosts lists the Host names accepted for a listener: the loopback
// names, the listen IP (or every interface IP for a wildcard address), this
// machine's host name with and without ".local", and the extra names. The list
// is fixed at start-up: if an interface address changes, restart the listener.
func allowedHosts(address net.Addr, extra []string) map[string]bool {
	hosts := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	if tcp, ok := address.(*net.TCPAddr); ok && tcp.IP != nil {
		if tcp.IP.IsUnspecified() {
			if addresses, err := net.InterfaceAddrs(); err == nil {
				for _, item := range addresses {
					if network, ok := item.(*net.IPNet); ok {
						hosts[network.IP.String()] = true
					}
				}
			}
		} else {
			hosts[tcp.IP.String()] = true
		}
	}
	if name, err := os.Hostname(); err == nil && name != "" {
		name = normalizeHost(name)
		hosts[name] = true
		hosts[name+".local"] = true
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
