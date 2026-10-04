package web

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSecurityMiddleware(t *testing.T) {
	fakeIfaces := func() ([]InterfaceInfo, error) {
		return []InterfaceInfo{
			{
				Name: "wlan0",
				Up:   true,
				IPs:  []net.IP{net.ParseIP("10.0.0.5")},
			},
		}, nil
	}
	fakeHostname := func() (string, error) {
		return "devbox", nil
	}

	type testCase struct {
		name          string
		method        string
		path          string
		host          string
		token         string
		origin        string
		contentType   string
		body          io.Reader
		allowHosts    []string
		wantStatus    int
		expectHandler bool
	}

	tests := []testCase{
		{
			name:          "no token",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "127.0.0.1:7421",
			token:         "",
			wantStatus:    http.StatusUnauthorized,
			expectHandler: false,
		},
		{
			name:          "wrong token",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "127.0.0.1:7421",
			token:         "wrong-token",
			wantStatus:    http.StatusUnauthorized,
			expectHandler: false,
		},
		{
			name:          "right token",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "127.0.0.1:7421",
			token:         "correct-token",
			wantStatus:    http.StatusOK,
			expectHandler: true,
		},
		{
			name:          "Host evil.example:7421",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "evil.example:7421",
			token:         "correct-token",
			wantStatus:    http.StatusMisdirectedRequest,
			expectHandler: false,
		},
		{
			name:          "Host 10.0.0.9:7421",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "10.0.0.9:7421",
			token:         "correct-token",
			wantStatus:    http.StatusMisdirectedRequest,
			expectHandler: false,
		},
		{
			name:          "Host 10.0.0.5:7421",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "10.0.0.5:7421",
			token:         "correct-token",
			wantStatus:    http.StatusOK,
			expectHandler: true,
		},
		{
			name:          "Host devbox:7421",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "devbox:7421",
			token:         "correct-token",
			wantStatus:    http.StatusOK,
			expectHandler: true,
		},
		{
			name:          "Host devbox.local:7421",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "devbox.local:7421",
			token:         "correct-token",
			wantStatus:    http.StatusOK,
			expectHandler: true,
		},
		{
			name:          "Host 127.0.0.1:9999",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "127.0.0.1:9999",
			token:         "correct-token",
			wantStatus:    http.StatusMisdirectedRequest,
			expectHandler: false,
		},
		{
			name:          "Host without port",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "127.0.0.1",
			token:         "correct-token",
			wantStatus:    http.StatusMisdirectedRequest,
			expectHandler: false,
		},
		{
			name:          "--allow-host entry skillhub.lan with Host skillhub.lan:7421",
			method:        http.MethodGet,
			path:          "/api/test",
			host:          "skillhub.lan:7421",
			token:         "correct-token",
			allowHosts:    []string{"skillhub.lan"},
			wantStatus:    http.StatusOK,
			expectHandler: true,
		},
		{
			name:          "POST with foreign Origin",
			method:        http.MethodPost,
			path:          "/api/test",
			host:          "127.0.0.1:7421",
			token:         "correct-token",
			origin:        "http://evil.example:7421",
			contentType:   "application/json",
			body:          bytes.NewBufferString("{}"),
			wantStatus:    http.StatusForbidden,
			expectHandler: false,
		},
		{
			name:          "POST with text/plain",
			method:        http.MethodPost,
			path:          "/api/test",
			host:          "127.0.0.1:7421",
			token:         "correct-token",
			origin:        "http://127.0.0.1:7421",
			contentType:   "text/plain",
			body:          bytes.NewBufferString("hello"),
			wantStatus:    http.StatusUnsupportedMediaType,
			expectHandler: false,
		},
		{
			name:          "POST body over 4 MiB",
			method:        http.MethodPost,
			path:          "/api/test",
			host:          "127.0.0.1:7421",
			token:         "correct-token",
			origin:        "http://127.0.0.1:7421",
			contentType:   "application/json",
			body:          bytes.NewReader(make([]byte, 4<<20+1)),
			wantStatus:    http.StatusRequestEntityTooLarge,
			expectHandler: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(Options{
				Workspace:  t.TempDir(),
				Token:      "correct-token",
				ListenPort: 7421,
				AllowHosts: tc.allowHosts,
				Interfaces: fakeIfaces,
				Hostname:   fakeHostname,
			})
			if err != nil {
				t.Fatalf("failed to create server: %v", err)
			}

			var handlerCalled atomic.Bool
			innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled.Store(true)
				w.WriteHeader(http.StatusOK)
			})

			wrapped := s.withMiddleware(innerHandler)

			var body io.Reader
			if tc.body != nil {
				body = tc.body
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Host = tc.host
			req.RemoteAddr = "127.0.0.1:12345"
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}

			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if handlerCalled.Load() != tc.expectHandler {
				t.Errorf("handlerCalled = %v, want %v", handlerCalled.Load(), tc.expectHandler)
			}
		})
	}
}

func TestAuthThrottle(t *testing.T) {
	curTime := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	fakeClock := func() time.Time {
		return curTime
	}

	s, err := New(Options{
		Workspace:  t.TempDir(),
		Token:      "good-token",
		ListenPort: 7421,
		Now:        fakeClock,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	handler := s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := range 20 {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "127.0.0.1:7421"
		req.RemoteAddr = "192.0.2.1:12345"
		req.Header.Set("Authorization", "Bearer bad-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	{
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "127.0.0.1:7421"
		req.RemoteAddr = "192.0.2.1:12345"
		req.Header.Set("Authorization", "Bearer good-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request 21: expected 429, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if retryAfter := rec.Header().Get("Retry-After"); retryAfter != "60" {
			t.Fatalf("request 21: expected Retry-After: 60, got %q", retryAfter)
		}
	}

	{
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "127.0.0.1:7421"
		req.RemoteAddr = "192.0.2.2:12345"
		req.Header.Set("Authorization", "Bearer good-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request from 192.0.2.2: expected 200, got %d", rec.Code)
		}
	}

	curTime = curTime.Add(61 * time.Second)
	{
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Host = "127.0.0.1:7421"
		req.RemoteAddr = "192.0.2.1:12345"
		req.Header.Set("Authorization", "Bearer good-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request after 61s: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}
}
