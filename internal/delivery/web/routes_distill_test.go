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

func TestSkillDistillRoutes(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(root, "skills", "default", "test-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Skill\n\nContent.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: test-skill\nstatus: draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc := distill.Document{
		Goal: "Learn error handling patterns",
		Lessons: []distill.Lesson{
			{
				Key:     "retry-jitter",
				What:    "Exponential backoff with jitter prevents retry storms.",
				Notable: "Avoids herd thundering.",
				Where:   []string{"usage:case_01"},
				Decision: distill.Decision{
					Status: "candidate",
				},
			},
		},
	}
	docBytes, err := distill.MarshalDocument(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "distill.yaml"), docBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, root)

	// 1. GET /api/v1/skills/test-skill/distill -> 200 OK
	rec := get(t, srv, "/api/v1/skills/test-skill/distill")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /distill status = %d: %s", rec.Code, rec.Body.String())
	}
	var gotDoc distill.Document
	if err := json.Unmarshal(rec.Body.Bytes(), &gotDoc); err != nil {
		t.Fatal(err)
	}
	if gotDoc.Goal != doc.Goal || len(gotDoc.Lessons) != 1 || gotDoc.Lessons[0].Key != "retry-jitter" {
		t.Fatalf("unexpected GET /distill response: %#v", gotDoc)
	}

	// 2. GET /api/v1/skills/nonexistent/distill -> 404 or 400
	rec404 := get(t, srv, "/api/v1/skills/nonexistent/distill")
	if rec404.Code == http.StatusOK {
		t.Fatalf("expected error status for nonexistent skill, got %d", rec404.Code)
	}

	// 3. POST /api/v1/skills/test-skill/distill -> 200 OK
	updatedDoc := doc
	updatedDoc.Goal = "Updated goal for testing"
	recPost := postJSON(t, srv, "/api/v1/skills/test-skill/distill", updatedDoc)
	if recPost.Code != http.StatusOK {
		t.Fatalf("POST /distill status = %d: %s", recPost.Code, recPost.Body.String())
	}

	// Verify file was updated on disk
	diskDoc, err := distill.LoadDocument(filepath.Join(skillDir, ".meta", "distill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if diskDoc.Goal != "Updated goal for testing" {
		t.Fatalf("file not updated on disk: got goal %q", diskDoc.Goal)
	}
}
