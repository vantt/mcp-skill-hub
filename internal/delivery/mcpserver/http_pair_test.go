package mcpserver

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const pairTestInvite = "invite-code-for-pair-tests"

// fakeRedeemer accepts each listed code once and records every call.
type fakeRedeemer struct {
	mu    sync.Mutex
	valid map[string]bool
	calls []string
}

func newFakeRedeemer(codes ...string) *fakeRedeemer {
	redeemer := &fakeRedeemer{valid: map[string]bool{}}
	for _, code := range codes {
		redeemer.valid[code] = true
	}
	return redeemer
}

func (redeemer *fakeRedeemer) Redeem(code string) error {
	redeemer.mu.Lock()
	defer redeemer.mu.Unlock()
	redeemer.calls = append(redeemer.calls, code)
	if !redeemer.valid[code] {
		return errors.New("not valid")
	}
	delete(redeemer.valid, code)
	return nil
}

func (redeemer *fakeRedeemer) callCount() int {
	redeemer.mu.Lock()
	defer redeemer.mu.Unlock()
	return len(redeemer.calls)
}

func (redeemer *fakeRedeemer) redeemable(code string) bool {
	redeemer.mu.Lock()
	defer redeemer.mu.Unlock()
	return redeemer.valid[code]
}

func pairBody(invite string) string { return `{"invite":"` + invite + `"}` }

// pairRequest sends a pairing request through the gate as if it arrived on the
// loopback listener from remote, with local as the address it was accepted on.
func pairRequest(gate *httpGate, remote, local, method, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1:7420"+HTTPPairPath, strings.NewReader(body))
	request.RemoteAddr = remote
	localAddr := &net.TCPAddr{IP: net.ParseIP(local), Port: 7420}
	request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, net.Addr(localAddr)))
	if mutate != nil {
		mutate(request)
	}
	recorder := httptest.NewRecorder()
	gate.ServeHTTP(recorder, request)
	return recorder
}

func newPairGate(t *testing.T, codes ...string) (*httpGate, *fakeRedeemer, string) {
	t.Helper()
	gate, _, token := newTestGate(t)
	redeemer := newFakeRedeemer(codes...)
	gate.invites = redeemer
	return gate, redeemer, token
}

func TestPairReturnsTokenOnceForValidInvite(t *testing.T) {
	gate, _, token := newPairGate(t, pairTestInvite)
	response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if got := response.Body.String(); got != `{"token":"`+token+`"}` {
		t.Fatalf("body does not carry exactly the hub token: %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	second := pairRequest(gate, "127.0.0.1:50001", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil)
	if second.Code != http.StatusForbidden || second.Body.String() != `{"error":"invite not valid"}` {
		t.Fatalf("second use = %d %q, want 403 invite not valid", second.Code, second.Body.String())
	}
	if strings.Contains(second.Body.String(), token) {
		t.Fatal("a refusal carried the token")
	}
}

func TestPairAcceptsPostOnly(t *testing.T) {
	gate, redeemer, _ := newPairGate(t, pairTestInvite)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", method, pairBody(pairTestInvite), nil)
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want 405", method, response.Code)
		}
	}
	if redeemer.callCount() != 0 {
		t.Fatal("a non-POST request reached the redeemer")
	}
}

func TestPairRejectsBadBodies(t *testing.T) {
	gate, redeemer, _ := newPairGate(t, pairTestInvite)
	huge := `{"invite":"` + strings.Repeat("a", 2048) + `"}`
	if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, huge, nil); response.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body status = %d, want 413", response.Code)
	}
	for name, body := range map[string]string{"not json": "invite=abc", "empty": "", "missing field": `{"code":"abc"}`, "empty invite": `{"invite":""}`, "wrong type": `{"invite":5}`} {
		if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, body, nil); response.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", name, response.Code)
		}
	}
	if redeemer.callCount() != 0 {
		t.Fatal("a malformed request reached the redeemer")
	}
	if !redeemer.redeemable(pairTestInvite) {
		t.Fatal("the invite was consumed")
	}
}

func TestPairDoesNotMixTokenAndInvite(t *testing.T) {
	gate, redeemer, token := newPairGate(t, pairTestInvite)
	response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(token), nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("the bearer token as an invite status = %d, want 403", response.Code)
	}
	if strings.Contains(response.Body.String(), token) {
		t.Fatal("the refusal echoed the token")
	}
	// An invite is not a bearer token for the MCP endpoint.
	if response := gateRequest(gate, pairTestInvite, "127.0.0.1:50000", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("an invite as a bearer token status = %d, want 401", response.Code)
	}
	if !redeemer.redeemable(pairTestInvite) {
		t.Fatal("using the invite as a bearer token consumed it")
	}
	// A bearer header does not stand in for the invite on the pairing route.
	response = pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, "{}", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) })
	if response.Code != http.StatusBadRequest {
		t.Fatalf("a bearer header without an invite status = %d, want 400", response.Code)
	}
}

func TestPairRouteIsOffWithoutInvites(t *testing.T) {
	gate, _, _ := newTestGate(t)
	response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestPairKeepsHostAndOriginChecks(t *testing.T) {
	gate, redeemer, _ := newPairGate(t, pairTestInvite)
	if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), func(r *http.Request) { r.Host = "evil.example" }); response.Code != http.StatusForbidden {
		t.Errorf("foreign Host status = %d, want 403", response.Code)
	}
	if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }); response.Code != http.StatusForbidden {
		t.Errorf("foreign Origin status = %d, want 403", response.Code)
	}
	if redeemer.callCount() != 0 {
		t.Fatal("a refused request reached the redeemer")
	}
}

func TestPairTransportMatrix(t *testing.T) {
	const refusal = "pairing is only offered over Tailscale or an SSH tunnel\n"
	cases := []struct {
		name          string
		remote, local string
		mutate        func(*http.Request)
		want          int
	}{
		{"loopback", "127.0.0.1:50000", "127.0.0.1", nil, http.StatusOK},
		{"tailscale", "100.101.102.103:50000", "100.101.102.103", nil, http.StatusOK},
		{"ipv6 loopback", "[::1]:50000", "::1", nil, http.StatusOK},
		{"mapped loopback", "[::ffff:127.0.0.1]:50000", "::ffff:127.0.0.1", nil, http.StatusOK},
		{"lan local", "127.0.0.1:50000", "192.168.1.20", nil, http.StatusForbidden},
		{"lan remote", "192.168.1.20:50000", "127.0.0.1", nil, http.StatusForbidden},
		{"unparsable remote", "nonsense", "127.0.0.1", nil, http.StatusForbidden},
		{"Forwarded", "127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("Forwarded", "for=1.2.3.4") }, http.StatusForbidden},
		{"X-Forwarded-For", "127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("X-Forwarded-For", "1.2.3.4") }, http.StatusForbidden},
		{"X-Forwarded-Host", "127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("X-Forwarded-Host", "x") }, http.StatusForbidden},
		{"X-Forwarded-Proto", "127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, http.StatusForbidden},
		{"X-Real-Ip", "127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("X-Real-Ip", "1.2.3.4") }, http.StatusForbidden},
		{"Via", "127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("Via", "1.1 proxy") }, http.StatusForbidden},
	}
	for _, tc := range cases {
		gate, redeemer, _ := newPairGate(t, pairTestInvite)
		gate.strictPairing = true
		response := pairRequest(gate, tc.remote, tc.local, http.MethodPost, pairBody(pairTestInvite), tc.mutate)
		if response.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, response.Code, tc.want)
			continue
		}
		if tc.want != http.StatusForbidden {
			continue
		}
		if response.Body.String() != refusal {
			t.Errorf("%s: body = %q, want the transport refusal", tc.name, response.Body.String())
		}
		if redeemer.callCount() != 0 || !redeemer.redeemable(pairTestInvite) {
			t.Errorf("%s: a refused transport touched the invite", tc.name)
		}
		if after := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil); after.Code != http.StatusOK {
			t.Errorf("%s: the invite was not redeemable afterwards: %d", tc.name, after.Code)
		}
	}
}

func TestPairWithoutStrictPairingAnswersAnyConnection(t *testing.T) {
	for name, tc := range map[string]struct {
		remote, local string
		mutate        func(*http.Request)
	}{
		"plain LAN":         {"192.168.1.20:50000", "192.168.1.5", nil},
		"public address":    {"8.8.8.8:50000", "192.168.1.5", nil},
		"behind a proxy":    {"127.0.0.1:50000", "127.0.0.1", func(r *http.Request) { r.Header.Set("X-Forwarded-For", "1.2.3.4") }},
		"unparsable remote": {"nonsense", "127.0.0.1", nil},
	} {
		gate, _, _ := newPairGate(t, pairTestInvite)
		if response := pairRequest(gate, tc.remote, tc.local, http.MethodPost, pairBody(pairTestInvite), tc.mutate); response.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", name, response.Code)
		}
	}
}

func TestPairThrottlesFailedInvitesPerRemote(t *testing.T) {
	gate, redeemer, _ := newPairGate(t, pairTestInvite)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	gate.failures = newAuthFailures(func() time.Time { return now })
	for attempt := 0; attempt < httpAuthFailureLimit; attempt++ {
		if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody("wrong"), nil); response.Code != http.StatusForbidden {
			t.Fatalf("attempt %d status = %d, want 403", attempt, response.Code)
		}
	}
	calls := redeemer.callCount()
	response := pairRequest(gate, "127.0.0.1:50001", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("a valid invite after %d failures status = %d, want 429", httpAuthFailureLimit, response.Code)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Error("the throttled reply has no Retry-After")
	}
	if redeemer.callCount() != calls || !redeemer.redeemable(pairTestInvite) {
		t.Fatal("a throttled request reached the redeemer")
	}
	// Another remote has its own budget.
	if response := pairRequest(gate, "100.64.0.9:50000", "100.101.102.103", http.MethodPost, pairBody(pairTestInvite), nil); response.Code != http.StatusOK {
		t.Fatalf("another remote status = %d, want 200", response.Code)
	}
	// The budget refills after the window.
	now = now.Add(httpAuthFailureWindow + time.Second)
	if response := pairRequest(gate, "127.0.0.1:50002", "127.0.0.1", http.MethodPost, pairBody("wrong"), nil); response.Code != http.StatusForbidden {
		t.Fatalf("after the window status = %d, want 403", response.Code)
	}
}

func TestPairIsNotBlockedByBearerFailuresFromTheSameAddress(t *testing.T) {
	gate, _, _ := newPairGate(t, pairTestInvite)
	for attempt := 0; attempt < httpAuthFailureLimit+5; attempt++ {
		gate.failures.record("127.0.0.1")
	}
	if !gate.failures.blocked("127.0.0.1") {
		t.Fatal("setup: bearer failures did not block the address")
	}
	if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil); response.Code != http.StatusOK {
		t.Fatalf("pairing after bearer failures status = %d, want 200", response.Code)
	}
}

func TestPairFailuresDoNotBlockBearerRequests(t *testing.T) {
	gate, _, _ := newPairGate(t, pairTestInvite)
	for attempt := 0; attempt < httpAuthFailureLimit+5; attempt++ {
		pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody("wrong"), nil)
	}
	if gate.failures.blocked("127.0.0.1") {
		t.Fatal("failed pairing attempts blocked the bearer check for the address")
	}
}

func TestPairRefusedTransportIsNotCounted(t *testing.T) {
	gate, _, _ := newPairGate(t, pairTestInvite)
	gate.strictPairing = true
	for attempt := 0; attempt < httpAuthFailureLimit+5; attempt++ {
		pairRequest(gate, "192.168.1.20:50000", "127.0.0.1", http.MethodPost, pairBody("wrong"), nil)
		pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody("wrong"), func(r *http.Request) { r.Header.Set("Via", "x") })
	}
	if gate.failures.blocked(httpPairFailureKey+"192.168.1.20") || gate.failures.blocked(httpPairFailureKey+"127.0.0.1") {
		t.Fatal("a request refused for its transport counted as a failure")
	}
	if response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil); response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

func TestPairUnreadableTokenLeavesInviteUnused(t *testing.T) {
	gate, redeemer, _ := newPairGate(t, pairTestInvite)
	if err := os.Remove(gate.tokens.Path()); err != nil {
		t.Fatal(err)
	}
	response := pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(pairTestInvite), nil)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "the hub token cannot be read") {
		t.Fatalf("status = %d body %q, want 503 with the reason", response.Code, response.Body.String())
	}
	if redeemer.callCount() != 0 || !redeemer.redeemable(pairTestInvite) {
		t.Fatal("an unreadable token consumed the invite")
	}
	if gate.failures.blocked(httpPairFailureKey + "127.0.0.1") {
		t.Fatal("an unreadable token counted as a failed invite")
	}
}

func TestPairResponsesNeverCarryInviteOrToken(t *testing.T) {
	gate, _, token := newPairGate(t, pairTestInvite)
	// The wrong-but-secret-looking value is refused; nothing may echo it.
	secret := "secret-invite-value-that-is-refused"
	for _, response := range []*httptest.ResponseRecorder{
		pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(secret), nil),
		pairRequest(gate, "192.168.1.20:50000", "127.0.0.1", http.MethodPost, pairBody(secret), nil),
		pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodGet, pairBody(secret), nil),
		pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, `{"invite":"`+secret+`", broken`, nil),
		pairRequest(gate, "127.0.0.1:50000", "127.0.0.1", http.MethodPost, pairBody(token), nil),
	} {
		dump := response.Body.String()
		for name, values := range response.Header() {
			dump += name + strings.Join(values, ",")
		}
		if strings.Contains(dump, secret) || strings.Contains(dump, token) {
			t.Fatalf("a refusal echoed the invite or the token: %q", dump)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("status %d carries no Cache-Control: no-store", response.Code)
		}
	}
}

func TestPairOverNetworkLogsNothingSecret(t *testing.T) {
	redeemer := newFakeRedeemer(pairTestInvite)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hub := startHTTPTestHubOn(t, newMCPWorkspace(t), listener, HTTPOptions{Invites: redeemer})
	post := func(body string) (int, string) {
		response, err := http.Post(hub.baseURL+HTTPPairPath, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		got, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(got)
	}
	if status, body := post(pairBody("wrong-invite-value")); status != http.StatusForbidden || strings.Contains(body, "wrong-invite-value") {
		t.Fatalf("a wrong invite = %d %q", status, body)
	}
	status, body := post(pairBody(pairTestInvite))
	if status != http.StatusOK || body != `{"token":"`+hub.token+`"}` {
		t.Fatalf("a valid invite = %d %q", status, body)
	}
	// The token and the invite open the endpoint and the route respectively,
	// never the other way round.
	if status, _ := post(pairBody(hub.token)); status != http.StatusForbidden {
		t.Fatalf("the token as an invite status = %d, want 403", status)
	}
	hub.stop(t)
	logged := hub.diagnostics.String()
	for _, secret := range []string{pairTestInvite, hub.token, "wrong-invite-value"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("diagnostics contain a secret: %q", logged)
		}
	}
}
