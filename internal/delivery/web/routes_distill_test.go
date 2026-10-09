package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestSkillDistillGetRoute(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(root, "skills", "default", "test-audit")
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Audit\n\nContent.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 2\nid: test-audit\nstatus: active\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Read real test-audit distill.yaml from testdata
	testdataBytes, err := os.ReadFile(filepath.Join("..", "..", "migration", "testdata", "test-audit-distill.yaml"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "distill.yaml"), testdataBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, root)

	// 1. GET /api/v1/skills/test-audit/distill -> 200 OK
	rec := get(t, srv, "/api/v1/skills/test-audit/distill")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /distill status = %d: %s", rec.Code, rec.Body.String())
	}
	var gotDoc distill.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &gotDoc); err != nil {
		t.Fatal(err)
	}
	if len(gotDoc.Lessons) != 42 {
		t.Fatalf("expected 42 lessons, got %d", len(gotDoc.Lessons))
	}
	for i, l := range gotDoc.Lessons {
		if l.Key == "" {
			t.Fatalf("lesson[%d] missing key", i)
		}
		if l.Layer == "" {
			t.Fatalf("lesson[%d] (%s) missing layer", i, l.Key)
		}
		if l.Score.Why == "" {
			t.Fatalf("lesson[%d] (%s) missing score", i, l.Key)
		}
		if l.Decision.State == "" {
			t.Fatalf("lesson[%d] (%s) missing decision.state", i, l.Key)
		}
	}

	// 2. GET /api/v1/skills/nonexistent/distill -> error status
	rec404 := get(t, srv, "/api/v1/skills/nonexistent/distill")
	if rec404.Code == http.StatusOK {
		t.Fatalf("expected error status for nonexistent skill, got %d", rec404.Code)
	}

	// 3. POST /api/v1/skills/test-audit/distill -> 405 Method Not Allowed
	recPost := postJSON(t, srv, "/api/v1/skills/test-audit/distill", map[string]string{"foo": "bar"})
	if recPost.Code != http.StatusMethodNotAllowed && recPost.Code != http.StatusNotFound {
		t.Fatalf("expected 404 or 405 for POST /distill, got %d", recPost.Code)
	}
}
