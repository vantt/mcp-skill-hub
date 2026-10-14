package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
)

// HTTPPairPath is where a client trades a one-time invite for the hub token.
const HTTPPairPath = "/mcp/pair"

const (
	httpPairMaxBody     = 1 << 10
	httpPairReadTimeout = 5 * time.Second
	httpPairTransport   = "pairing is only offered over Tailscale or an SSH tunnel"
	httpPairFailureKey  = "pair|"
)

// Redeemer consumes a one-time invite. It reports an error for any invite that
// is unknown, expired or already used, without saying which.
type Redeemer interface {
	Redeem(code string) error
}

// servePair answers POST /mcp/pair. The checks run in a fixed order: method,
// transport (only with strict pairing), failure budget, body, hub token, invite. The hub token is read
// before the invite is redeemed so a token that cannot be read never burns an
// invite. Neither the invite, the token nor the request body is logged or
// echoed; only a redeem failure is counted, against the remote address.
func (gate *httpGate) servePair(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "use POST", http.StatusMethodNotAllowed)
		return
	}
	if gate.strictPairing && !gate.pairingTransportAllowed(request) {
		http.Error(writer, httpPairTransport, http.StatusForbidden)
		return
	}
	// Pairing keeps its own failure count: a stale bearer token on the same
	// address (every tunnel client is 127.0.0.1) must not block re-pairing.
	remote := httpPairFailureKey + remoteHost(request.RemoteAddr)
	if gate.failures.blocked(remote) {
		writer.Header().Set("Retry-After", fmt.Sprint(int(httpAuthFailureWindow.Seconds())))
		http.Error(writer, "too many failed attempts; wait a minute", http.StatusTooManyRequests)
		return
	}
	_ = http.NewResponseController(writer).SetReadDeadline(time.Now().Add(httpPairReadTimeout))
	raw, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, httpPairMaxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(writer, "the request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(writer, "the request body could not be read", http.StatusBadRequest)
		return
	}
	var body struct {
		Invite string `json:"invite"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Invite == "" {
		http.Error(writer, `send {"invite":"<code>"}`, http.StatusBadRequest)
		return
	}
	token, err := gate.tokens.Token()
	if err != nil {
		http.Error(writer, "the hub token cannot be read", http.StatusServiceUnavailable)
		return
	}
	if gate.invites.Redeem(body.Invite) != nil {
		gate.failures.record(remote)
		writePairJSON(writer, http.StatusForbidden, map[string]string{"error": "invite not valid"})
		return
	}
	writePairJSON(writer, http.StatusOK, map[string]string{"token": token})
}

// pairingTransportAllowed reports whether the connection stays on loopback or
// Tailscale, where the traffic is not readable on the wire, and carries no
// proxy header.
func (gate *httpGate) pairingTransportAllowed(request *http.Request) bool {
	local, _ := request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	var remoteAddr net.Addr
	if parsed, err := netip.ParseAddrPort(request.RemoteAddr); err == nil {
		remoteAddr = net.TCPAddrFromAddrPort(parsed)
	}
	return hostpolicy.PairingAllowed(local, remoteAddr, request.Header)
}

func writePairJSON(writer http.ResponseWriter, status int, value map[string]string) {
	encoded, err := json.Marshal(value)
	if err != nil {
		http.Error(writer, "internal error", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_, _ = writer.Write(encoded)
}
