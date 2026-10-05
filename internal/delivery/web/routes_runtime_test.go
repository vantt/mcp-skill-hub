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

func post(t *testing.T, srv *Server, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r ioReader
	if body != nil {
		r = bytes.NewReader(body)
	} else {
		r = http.NoBody
	}
	req := httptest.NewRequest(http.MethodPost, path, r)
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Origin", "http://127.0.0.1:7421")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

type ioReader interface {
	Read(p []byte) (n int, err error)
}

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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, srv, tc.path)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if strings.Contains(rec.Body.String(), "SENTINEL-VENDOR-ENV-VALUE") {
				t.Fatalf("%s response leaked sentinel env value: %s", tc.name, rec.Body.String())
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
				t.Fatalf("failed to read golden file %s: %v (run go test -update)", goldenPath, err)
			}

			if normalized != string(expected) {
				t.Fatalf("mismatch for %s:\nGOT:\n%s\nWANT:\n%s", tc.name, normalized, string(expected))
			}
		})
	}
}

func TestWebHasNoContentApprovalPath(t *testing.T) {
	t.Parallel()
	root := newRuntimeWebWorkspace(t)
	srv := newTestServer(t, root)

	// 1. POST /api/v1/skills/vendor-skill/approve is not routed -> 404 or 405
	recApprove := post(t, srv, "/api/v1/skills/vendor-skill/approve", nil)
	if recApprove.Code != http.StatusNotFound && recApprove.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /approve should be rejected, got status %d: %s", recApprove.Code, recApprove.Body.String())
	}

	// 2. POST /api/v1/skills/vendor-skill/update/preview with content_reviewed_digest -> 400
	payload := `{"expected_content_digest":"sha256:123","content_reviewed_digest":"sha256:456"}`
	recPreview := post(t, srv, "/api/v1/skills/vendor-skill/update/preview", []byte(payload))
	if recPreview.Code != http.StatusBadRequest {
		t.Fatalf("POST /update/preview with content_reviewed_digest should return 400, got %d: %s", recPreview.Code, recPreview.Body.String())
	}
}
