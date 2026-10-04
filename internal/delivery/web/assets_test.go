package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestAssetsServing(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!DOCTYPE html><html><body>Skill Hub WebUI</body></html>"),
		},
		"assets/app-123.js": &fstest.MapFile{
			Data: []byte("console.log('app');"),
		},
	}

	s, err := New(Options{
		Workspace:  t.TempDir(),
		Token:      "test-token",
		ListenPort: 7421,
		Assets:     mockFS,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	handler := s.Handler()

	t.Run("deep link with Accept text/html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/skills/x", nil)
		req.Host = "127.0.0.1:7421"
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Skill Hub WebUI") {
			t.Errorf("body does not contain index.html: %s", rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}
	})

	t.Run("assets/app-123.js immutable cache header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app-123.js", nil)
		req.Host = "127.0.0.1:7421"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "console.log('app');") {
			t.Errorf("body unexpected: %s", rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Errorf("expected immutable cache header, got %q", cc)
		}
	})

	t.Run("missing asset without text/html Accept returns 404 empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/missing.png", nil)
		req.Host = "127.0.0.1:7421"
		req.Header.Set("Accept", "image/png,image/*")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("expected empty body, got %q", rec.Body.String())
		}
	})

	t.Run("unknown api path returns 404 JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
		req.Host = "127.0.0.1:7421"
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		var env map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("failed to decode error envelope: %v", err)
		}
		errObj, ok := env["error"].(map[string]any)
		if !ok {
			t.Fatalf("missing error object in response: %v", env)
		}
		if errObj["code"] != "invalid_request" {
			t.Errorf("code = %v, want invalid_request", errObj["code"])
		}
	})

	t.Run("empty MapFS returns 503 not built text", func(t *testing.T) {
		emptySrv, err := New(Options{
			Workspace:  t.TempDir(),
			Token:      "test-token",
			ListenPort: 7421,
			Assets:     fstest.MapFS{},
		})
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "127.0.0.1:7421"
		rec := httptest.NewRecorder()
		emptySrv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
		expectedText := "Skill Hub web UI is not built. Run make web-build, then rebuild skillhub."
		if !strings.Contains(rec.Body.String(), expectedText) {
			t.Errorf("body does not contain %q: %s", expectedText, rec.Body.String())
		}
	})
}
