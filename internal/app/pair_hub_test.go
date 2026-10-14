package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const pairTestToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func pairServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestPairHubReturnsTheToken(t *testing.T) {
	server, _ := pairServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/mcp/pair" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct{ Invite string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Invite != "ABCD" {
			t.Errorf("body = %+v, %v", body, err)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("an Authorization header was sent")
		}
		_, _ = w.Write([]byte(`{"token":"` + pairTestToken + `"}`))
	})
	token, err := PairHub(context.Background(), server.URL, "ABCD")
	if err != nil || token != pairTestToken {
		t.Fatalf("PairHub = %q, %v", token, err)
	}
}

func TestPairHubStatusMessages(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"invite not valid", 403, "invite not valid", "this invite is not valid: it expired, was already used, or is wrong"},
		{"pairing refused", 403, "pairing is only offered over Tailscale or an SSH tunnel", "pairing is only offered over Tailscale or an SSH tunnel"},
		{"host refused", 403, "Forbidden: the Host header is not allowed", "the invite is still unused"},
		{"no route", 404, "", "this hub does not offer invites"},
		{"throttled", 429, "", "the hub is refusing attempts for a minute"},
		{"unavailable", 503, "the hub is shutting down", "the hub answered with an error (HTTP 503)"},
		{"something else", 418, "", "HTTP 418"},
		{"malformed json", 200, "{not json", "is not what a Skill Hub sends"},
		{"malformed token", 200, `{"token":"short"}`, "is not what a Skill Hub sends"},
		{"no token", 200, `{}`, "is not what a Skill Hub sends"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := pairServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			token, err := PairHub(context.Background(), server.URL, "ABCD")
			if err == nil || token != "" {
				t.Fatalf("PairHub = %q, %v; want an error", token, err)
			}
			var failure *Error
			if !asError(err, &failure) || !strings.Contains(failure.Render.Why, tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if failure.Render.Fix == "" {
				t.Fatal("no fix")
			}
		})
	}
}

func asError(err error, target **Error) bool {
	failure, ok := err.(*Error)
	*target = failure
	return ok
}

func TestPairHubDoesNotFollowRedirects(t *testing.T) {
	other, otherCalls := pairServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"` + pairTestToken + `"}`))
	})
	server, _ := pairServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/mcp/pair", http.StatusTemporaryRedirect)
	})
	if token, err := PairHub(context.Background(), server.URL, "ABCD"); err == nil || token != "" {
		t.Fatalf("PairHub = %q, %v; want an error", token, err)
	}
	if otherCalls.Load() != 0 {
		t.Fatal("the redirect target received the invite")
	}
}

func TestPairHubLimitsTheAnswer(t *testing.T) {
	server, _ := pairServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"` + pairTestToken + `","padding":"` + strings.Repeat("x", 8192) + `"}`))
	})
	if token, err := PairHub(context.Background(), server.URL, "ABCD"); err == nil || token != "" {
		t.Fatalf("PairHub = %q, %v; want an error for an oversized answer", token, err)
	}
}

func TestPairHubTimesOut(t *testing.T) {
	release := make(chan struct{})
	server, _ := pairServer(t, func(http.ResponseWriter, *http.Request) { <-release })
	t.Cleanup(func() { close(release) })
	previous := pairTimeout
	pairTimeout = 100 * time.Millisecond
	t.Cleanup(func() { pairTimeout = previous })
	_, err := PairHub(context.Background(), server.URL, "ABCD")
	var failure *Error
	if !asError(err, &failure) || !strings.Contains(failure.Render.Why, "does not answer") {
		t.Fatalf("error = %v", err)
	}
}

func TestPairHubNothingAnswers(t *testing.T) {
	server, _ := pairServer(t, func(http.ResponseWriter, *http.Request) {})
	url := server.URL
	server.Close()
	_, err := PairHub(context.Background(), url, "ABCD")
	var failure *Error
	if !asError(err, &failure) || !strings.Contains(failure.Render.Why, "connection refused") {
		t.Fatalf("error = %v", err)
	}
}

func TestPairHubIgnoresProxySettings(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY"} {
		t.Setenv(name, "http://127.0.0.1:1")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	server, calls := pairServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"` + pairTestToken + `"}`))
	})
	// Go never proxies loopback, so also check that the client itself has no proxy.
	if transport, ok := hubProbeClient().Transport.(*http.Transport); !ok || transport.Proxy != nil {
		t.Fatal("the client consults proxy settings")
	}
	token, err := PairHub(context.Background(), server.URL, "ABCD")
	if err != nil || token != pairTestToken || calls.Load() != 1 {
		t.Fatalf("PairHub = %q, %v (calls %d)", token, err, calls.Load())
	}
}

func TestPairHubErrorsNeverHoldTheInviteOrToken(t *testing.T) {
	const invite = "SECRETINVITE"
	server, _ := pairServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("invite not valid " + invite + " " + pairTestToken))
	})
	_, err := PairHub(context.Background(), server.URL, invite)
	if err == nil {
		t.Fatal("want an error")
	}
	text := err.Error() + err.(*Error).Render.Fix
	if strings.Contains(text, invite) || strings.Contains(text, pairTestToken) {
		t.Fatalf("error leaks a secret: %s", text)
	}
}
