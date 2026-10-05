package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSkillUsageEndpoint(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	// Valid since values
	for _, since := range []string{"7d", "30d", "90d", "180d", ""} {
		path := "/api/v1/skills/review-skill/usage"
		if since != "" {
			path += "?since=" + since
		}
		rec := get(t, srv, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d (body: %s)", path, rec.Code, http.StatusOK, rec.Body.String())
		}
		var report app.FunnelReport
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatalf("unmarshal report: %v", err)
		}
		if report.Overall != nil {
			t.Fatalf("expected nil Overall for single skill endpoint, got: %+v", report.Overall)
		}
		if report.Skill == nil || report.Skill.SkillID != "review-skill" {
			t.Fatalf("expected Skill review-skill, got: %+v", report.Skill)
		}
	}

	// 400 bad since
	recBad := get(t, srv, "/api/v1/skills/review-skill/usage?since=invalid")
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("GET bad since = %d, want %d", recBad.Code, http.StatusBadRequest)
	}

	// 404 unknown skill
	recNotFound := get(t, srv, "/api/v1/skills/unknown-skill/usage")
	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("GET unknown skill = %d, want %d", recNotFound.Code, http.StatusNotFound)
	}

	// 401 without auth token
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/skills/review-skill/usage", nil)
	unauthReq.Host = "127.0.0.1:7421"
	recUnauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recUnauth, unauthReq)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("GET without auth token = %d, want %d", recUnauth.Code, http.StatusUnauthorized)
	}
}
