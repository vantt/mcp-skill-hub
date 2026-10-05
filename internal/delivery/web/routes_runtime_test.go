package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeEndpointsGolden(t *testing.T) {
	root := newRuntimeWebWorkspace(t)
	srv := newTestServer(t, root)

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "skill-review-third-party", path: "/api/v1/skills/vendor-skill/review", wantStatus: http.StatusOK},
		{name: "skill-runtime-third-party", path: "/api/v1/skills/vendor-skill/runtime", wantStatus: http.StatusOK},
		{name: "skill-runtime-approved", path: "/api/v1/skills/approved-skill/runtime", wantStatus: http.StatusOK},
	}

	goldenDir := filepath.Join("testdata", "golden")
	if *update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("failed to create golden directory: %v", err)
		}
	}

	const sentinel = "SENTINEL-TEST-SECRET"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, srv, tc.path)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if strings.Contains(rec.Body.String(), sentinel) {
				t.Fatalf("response leaked sentinel secret: %s", rec.Body.String())
			}

			var pretty bytes.Buffer
			if err := json.Indent(&pretty, rec.Body.Bytes(), "", "  "); err != nil {
				t.Fatalf("failed to indent response JSON: %v", err)
			}
			pretty.WriteString("\n")

			normalized := normalizeGolden(pretty.String(), root)
			goldenPath := filepath.Join(goldenDir, tc.name+".json")

			if *update {
				if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
				return
			}

			expected, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v (run with -update)", goldenPath, err)
			}

			if normalized != string(expected) {
				t.Errorf("mismatch for %s:\nGOT:\n%s\nWANT:\n%s", tc.name, normalized, string(expected))
			}
		})
	}
}

func TestWebHasNoContentApprovalPath(t *testing.T) {
	root := newRuntimeWebWorkspace(t)
	srv := newTestServer(t, root)
	// 1. POST /api/v1/skills/vendor-skill/approve -> 404 or 405
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/vendor-skill/approve", strings.NewReader("{}"))
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Origin", "http://127.0.0.1:7421")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404 or 405 for /approve, got %d", rec.Code)
	}

	// 2. POST /api/v1/skills/vendor-skill/update/preview with content_reviewed_digest -> 400 DisallowUnknownFields
	updateBody := `{"expected_content_digest":"sha256:aaaa","content_reviewed_digest":"sha256:bbbb"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/skills/vendor-skill/update/preview", strings.NewReader(updateBody))
	req2.Host = "127.0.0.1:7421"
	req2.Header.Set("Authorization", "Bearer test-token")
	req2.Header.Set("Origin", "http://127.0.0.1:7421")
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for update/preview with content_reviewed_digest, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}
}
