package web

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func normalizeGolden(body, root string) string {
	s := strings.ReplaceAll(body, root, "<WORKSPACE>")
	s = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`).ReplaceAllString(s, "<TIME>")
	s = regexp.MustCompile(`sha256:[0-9a-f]{64}`).ReplaceAllString(s, "<DIGEST>")
	s = regexp.MustCompile(`gen-[A-Za-z0-9_-]+`).ReplaceAllString(s, "gen-<ID>")
	s = regexp.MustCompile(`(OP|RUN|PROP|PRP|INS|OBS)-[A-Za-z0-9_-]+`).ReplaceAllString(s, "<PREFIX>-<ID>")
	s = regexp.MustCompile(`"skillhub_version":\s*"[^"]*"`).ReplaceAllString(s, `"skillhub_version": "<VERSION>"`)
	// The platform doctor check reports the host OS; keep goldens OS-independent.
	s = strings.ReplaceAll(s, `"name": "`+runtime.GOOS+`"`, `"name": "<GOOS>"`)
	return s
}

func TestReadEndpointsGolden(t *testing.T) {
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "session", path: "/api/v1/session", wantStatus: http.StatusOK},
		{name: "home", path: "/api/v1/home", wantStatus: http.StatusOK},
		{name: "skills", path: "/api/v1/skills", wantStatus: http.StatusOK},
		{name: "skill-detail", path: "/api/v1/skills/review-skill", wantStatus: http.StatusOK},
		{name: "skill-review", path: "/api/v1/skills/review-skill/review", wantStatus: http.StatusOK},
		{name: "skill-usage", path: "/api/v1/skills/review-skill/usage?since=30d", wantStatus: http.StatusOK},
		{name: "skill-runtime", path: "/api/v1/skills/review-skill/runtime", wantStatus: http.StatusOK},
		{name: "sources", path: "/api/v1/sources", wantStatus: http.StatusOK},
		{name: "skill-sources", path: "/api/v1/skills/review-skill/sources", wantStatus: http.StatusOK},
		{name: "skill-unknown", path: "/api/v1/skills/unknown-skill", wantStatus: http.StatusNotFound},
		{name: "skill-runtime-unknown", path: "/api/v1/skills/unknown-skill/runtime", wantStatus: http.StatusNotFound},
		{name: "skills-bad-state", path: "/api/v1/skills?state=invalid", wantStatus: http.StatusBadRequest},
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
