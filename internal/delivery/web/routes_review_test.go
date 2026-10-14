package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

// newThirdPartyReviewWorkspace adds a third-party skill with a recorded
// approval that a later edit made stale, and a draft.
func newThirdPartyReviewWorkspace(t *testing.T) (string, *Server) {
	t.Helper()
	root, _ := newFilesWebWorkspace(t)
	srv := newTestServer(t, root)
	meta := "schema_version: 1\nid: pdf\nstatus: active\nrouting:\n  triggers:\n    - fill pdf forms\n  not_for:\n    - unrelated work\n  min_scope: single_step\n  examples:\n    - fill this pdf\n    - please fill a pdf form\n    - pdf form filling\nquality:\n  reviewed: false\nsources:\n  - id: pdf-src\n    roles:\n      - upstream\n    kind: github\n    repo: github.com/anthropics/skills\n"
	writeSkillTree(t, root, "pdf", "active", meta, map[string]string{"scripts/fill.py": "print(1)\n"})
	writeSkillTree(t, root, "team-release", "draft", strings.Replace(strings.Replace(meta, "id: pdf", "id: team-release", 1), "status: active", "status: draft", 1), nil)
	commitFixture(t, root)
	rebuildCatalog(t, root)

	trust, err := (app.SkillService{}).ContentTrustFor(t.Context(), root, "pdf")
	if err != nil || !trust.RequiresReview() {
		t.Fatalf("trust = %+v, %v", trust, err)
	}
	approve(t, srv, "pdf", trust.ContentDigest)
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "pdf", "scripts", "fill.py"), []byte("print(2)\nprint(3)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitFixture(t, root)
	rebuildCatalog(t, root)
	return root, srv
}

func writeSkillTree(t *testing.T, root, id, state, meta string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, "skills", "core", id)
	all := map[string]string{
		"SKILL.md":         "---\nname: " + id + "\ndescription: Does " + id + " things. Use when you need " + id + ".\nlicense: Proprietary\n---\n\n# " + id + "\n",
		".meta/skill.yaml": meta,
	}
	for rel, body := range files {
		all[rel] = body
	}
	for rel, body := range all {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func rebuildCatalog(t *testing.T, root string) {
	t.Helper()
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
}

// approve runs the approval the review screen runs: preview, then confirm.
func approve(t *testing.T, srv *Server, id, digest string) map[string]any {
	t.Helper()
	rec := postJSON(t, srv, "/api/v1/skills/"+id+"/approve/preview", map[string]string{"content_digest": digest})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve preview = %d (body: %s)", rec.Code, rec.Body.String())
	}
	pins := extractProposalPins(t, rec)
	rec = postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve confirm = %d (body: %s)", rec.Code, rec.Body.String())
	}
	return decodeBody(t, rec)
}

func TestReviewQueueRoute(t *testing.T) {
	t.Parallel()
	_, srv := newThirdPartyReviewWorkspace(t)
	rec := get(t, srv, "/api/v1/review-queue")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	assertKeysDeclared(t, body, "ReviewQueue")
	kinds := map[string]string{}
	for _, raw := range body["items"].([]any) {
		item := raw.(map[string]any)
		assertKeysDeclared(t, item, "ReviewQueueItem")
		kinds[item["skill_id"].(string)] = item["kind"].(string)
	}
	if len(kinds) != 2 || kinds["pdf"] != "content_changed" || kinds["team-release"] != "draft" {
		t.Fatalf("queue = %v", kinds)
	}
}

func TestReviewContentRouteAndApproval(t *testing.T) {
	t.Parallel()
	root, srv := newThirdPartyReviewWorkspace(t)
	rec := get(t, srv, "/api/v1/skills/pdf/review-content")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	review := decodeBody(t, rec)
	assertKeysDeclared(t, review, "SkillReviewContent")
	if review["license"] != "Proprietary" || review["origin"] != "github.com/anthropics/skills" || review["baseline"] != "approved" || review["third_party"] != true {
		t.Fatalf("review = %v", review)
	}
	changed := map[string]bool{}
	for _, raw := range review["files"].([]any) {
		file := raw.(map[string]any)
		assertKeysDeclared(t, file, "ReviewFile")
		changed[file["path"].(string)] = file["changed"].(bool)
	}
	if !changed["scripts/fill.py"] || changed["SKILL.md"] {
		t.Fatalf("changed = %v", changed)
	}
	if rec := get(t, srv, "/api/v1/skills/nothing/review-content"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown skill = %d", rec.Code)
	}

	// Content that changed since the screen was opened is not approved.
	if rec := postJSON(t, srv, "/api/v1/skills/pdf/approve/preview", map[string]string{"content_digest": "sha256:" + strings.Repeat("1", 64)}); rec.Code != http.StatusConflict || extractErrorCode(t, rec) != "stale_proposal" {
		t.Fatalf("stale digest = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/v1/skills/pdf/approve/preview", map[string]string{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing digest = %d", rec.Code)
	}

	before := headCount(t, root)
	confirmed := approve(t, srv, "pdf", review["content_digest"].(string))
	commit, _ := confirmed["commit"].(map[string]any)
	if commit["status"] != "committed" || headCount(t, root) == before {
		t.Fatalf("approval must be one commit: %v", confirmed)
	}
	queue := decodeBody(t, get(t, srv, "/api/v1/review-queue"))
	for _, raw := range queue["items"].([]any) {
		if raw.(map[string]any)["skill_id"] == "pdf" {
			t.Fatalf("an approved skill still waits: %v", queue)
		}
	}
}
