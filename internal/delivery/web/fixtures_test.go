package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func newWebWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (app.WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	service := app.SkillService{}
	content := []byte("---\nname: review-skill\ndescription: Review changed code safely.\nlicense: Apache-2.0\n---\n\n# Review\n\nReview carefully.\n")
	created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          "review-skill",
		Collection:  "core",
		Name:        "Review Skill",
		Description: "Review changed code safely.",
		Content:     content,
		Routing: skill.RoutingInput{
			Operations: []string{"review"},
			Triggers:   []string{"review changed code"},
			NotFor:     []string{"write prose"},
			MinScope:   "multi_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create = %#v, %v", result, err)
	}
	activated, err := service.PreviewActivate(context.Background(), root, "review-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, activated, activated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate = %#v, %v", result, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "core", "review-skill", "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "review-skill", "references", "checks.md"), []byte("# Checks\n\nRun tests.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, "skills", "core", "review-skill", "skill.meta.yaml")
	if data, err := os.ReadFile(metaPath); err == nil {
		fixed := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`).ReplaceAll(data, []byte("2026-10-04T12:00:00.000000000Z"))
		if err := os.WriteFile(metaPath, fixed, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return root
}

func newTestServer(t *testing.T, root string) *Server {
	t.Helper()
	fakeIfaces := func() ([]InterfaceInfo, error) {
		return []InterfaceInfo{}, nil
	}
	srv, err := New(Options{
		Workspace:  root,
		Token:      "test-token",
		ListenPort: 7421,
		Interfaces: fakeIfaces,
		Assets:     fstest.MapFS{},
	})
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	return srv
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
