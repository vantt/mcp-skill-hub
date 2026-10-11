package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

const webResourceDir = "skills/core/review-skill"

// newResourceWebWorkspace adds a binary file and a text file above the edit
// limit next to the fixture's references/checks.md.
func newResourceWebWorkspace(t *testing.T) (string, *Server) {
	t.Helper()
	root := newWebWorkspace(t)
	files := map[string][]byte{
		"assets/logo.png":      []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
		"references/large.txt": bytes.Repeat([]byte("0123456789abcde\n"), (skill.MaxEditableResourceBytes/16)+64),
	}
	for relative, contents := range files {
		target := filepath.Join(root, filepath.FromSlash(webResourceDir+"/"+relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return root, newTestServer(t, root)
}

func resourceContentURL(id, path string) string {
	return "/api/v1/skills/" + id + "/resources/content?path=" + url.QueryEscape(path)
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (body: %s)", err, rec.Body.String())
	}
	return body
}

// tsInterfaceFields reads the field names of one exported interface from the
// web client's shared types, so the Go JSON shape cannot drift from them.
func tsInterfaceFields(t *testing.T, name string) []string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "api", "types.ts"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)export interface ` + name + ` \{(.*?)\n\}`).FindSubmatch(source)
	if block == nil {
		t.Fatalf("interface %s not found in types.ts", name)
	}
	var fields []string
	for _, match := range regexp.MustCompile(`(?m)^\s+([a-z_]+)\??:`).FindAllSubmatch(block[1], -1) {
		fields = append(fields, string(match[1]))
	}
	sort.Strings(fields)
	return fields
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertGolden(t *testing.T, name, root string, rec *httptest.ResponseRecorder) {
	t.Helper()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, rec.Body.Bytes(), "", "  "); err != nil {
		t.Fatalf("indent: %v", err)
	}
	pretty.WriteString("\n")
	normalized := normalizeGolden(pretty.String(), root)
	goldenPath := filepath.Join("testdata", "golden", name+".json")
	if *update {
		if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update)", goldenPath, err)
	}
	if normalized != string(expected) {
		t.Errorf("mismatch for %s:\nGOT:\n%s\nWANT:\n%s", name, normalized, expected)
	}
}

func TestSkillResourceListRoute(t *testing.T) {
	t.Parallel()
	root, srv := newResourceWebWorkspace(t)
	rec := get(t, srv, "/api/v1/skills/review-skill/resources")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	assertGolden(t, "skill-resources", root, rec)

	body := decodeBody(t, rec)
	if got, want := sortedKeys(body), tsInterfaceFields(t, "SkillResourceList"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("list fields = %v, want %v", got, want)
	}
	items, _ := body["items"].([]any)
	reasons := map[string]string{}
	for _, raw := range items {
		item := raw.(map[string]any)
		if got, want := sortedKeys(item), tsInterfaceFields(t, "SkillResourceItem"); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("item fields = %v, want %v", got, want)
		}
		path := item["path"].(string)
		if strings.HasSuffix(path, "/SKILL.md") || strings.Contains(path, "/.meta/") {
			t.Fatalf("list includes %s", path)
		}
		reasons[strings.TrimPrefix(path, webResourceDir+"/")] = item["reason"].(string)
	}
	want := map[string]string{"references/checks.md": "", "assets/logo.png": "binary", "references/large.txt": "too_large"}
	for path, reason := range want {
		if got, ok := reasons[path]; !ok || got != reason {
			t.Fatalf("reason for %s = %q (present %v), want %q; all: %v", path, got, ok, reason, reasons)
		}
	}

	if rec := get(t, srv, "/api/v1/skills/unknown-skill/resources"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown skill status = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestSkillResourceContentRoute(t *testing.T) {
	t.Parallel()
	root, srv := newResourceWebWorkspace(t)

	rec := get(t, srv, resourceContentURL("review-skill", webResourceDir+"/references/checks.md"))
	if rec.Code != http.StatusOK {
		t.Fatalf("text status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	assertGolden(t, "skill-resource-content", root, rec)
	text := decodeBody(t, rec)
	if got, want := sortedKeys(text), tsInterfaceFields(t, "SkillResourceContent"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("content fields = %v, want %v", got, want)
	}
	if text["content"] != "# Checks\n\nRun tests.\n" || text["editable"] != true || text["line_ending"] != "lf" {
		t.Fatalf("text content = %v", text)
	}

	rec = get(t, srv, resourceContentURL("review-skill", webResourceDir+"/assets/logo.png"))
	binary := decodeBody(t, rec)
	if _, has := binary["content"]; rec.Code != http.StatusOK || has || binary["reason"] != "binary" || binary["editable"] != false {
		t.Fatalf("binary = %d %v", rec.Code, binary)
	}

	rec = get(t, srv, resourceContentURL("review-skill", webResourceDir+"/references/large.txt"))
	large := decodeBody(t, rec)
	content, _ := large["content"].(string)
	if rec.Code != http.StatusOK || large["truncated"] != true || large["reason"] != "too_large" || len(content) == 0 || len(content) > skill.MaxEditableResourceBytes {
		t.Fatalf("large = %d truncated=%v reason=%v len=%d", rec.Code, large["truncated"], large["reason"], len(content))
	}

	if rec := get(t, srv, "/api/v1/skills/review-skill/resources/content"); rec.Code != http.StatusBadRequest || extractErrorCode(t, rec) != "invalid_request" {
		t.Fatalf("missing path = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec := get(t, srv, resourceContentURL("unknown-skill", webResourceDir+"/references/checks.md")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown skill = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestSkillResourceContentRefusesUnsafePaths(t *testing.T) {
	t.Parallel()
	root, srv := newResourceWebWorkspace(t)
	skillMarkdown, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(webResourceDir+"/SKILL.md")))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		webResourceDir + "/../review-skill/SKILL.md",
		webResourceDir + "/SKILL.md",
		webResourceDir + "/.meta/skill.yaml",
		webResourceDir + `\references\checks.md`,
		webResourceDir + "/references/checks.md\x00",
		"/etc/passwd",
		filepath.Join(root, filepath.FromSlash(webResourceDir+"/references/checks.md")),
		"skills/core/other-skill/notes.md",
	} {
		rec := get(t, srv, resourceContentURL("review-skill", path))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Fatalf("%q status = %d (body: %s)", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "Review carefully") || strings.Contains(rec.Body.String(), "Run tests.") || strings.Contains(rec.Body.String(), "root:") {
			t.Fatalf("%q leaked file bytes: %s", path, rec.Body.String())
		}
	}
	if bytes.Contains(skillMarkdown, []byte("Run tests.")) {
		t.Fatal("fixture SKILL.md must not share text with references/checks.md")
	}

	link := filepath.Join(root, filepath.FromSlash(webResourceDir+"/references/link.md"))
	if err := os.Symlink(filepath.Join(root, filepath.FromSlash(webResourceDir+"/references/checks.md")), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	rec := get(t, srv, resourceContentURL("review-skill", webResourceDir+"/references/link.md"))
	if rec.Code == http.StatusOK || rec.Code == http.StatusInternalServerError || strings.Contains(rec.Body.String(), "Run tests.") {
		t.Fatalf("symlink = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if code := extractErrorCode(t, rec); code != "snapshot_expired" || !strings.Contains(rec.Body.String(), "symlink") {
		t.Fatalf("symlink error = %s (body: %s)", code, rec.Body.String())
	}
}

func TestSkillResourcePreviewAndConfirmRoute(t *testing.T) {
	t.Parallel()
	root, srv := newResourceWebWorkspace(t)
	path := webResourceDir + "/references/checks.md"
	current := decodeBody(t, get(t, srv, resourceContentURL("review-skill", path)))
	digest := current["digest"].(string)
	updated := "# Checks\n\nRun all tests.\n"

	rec := postJSON(t, srv, "/api/v1/skills/review-skill/resources/preview", map[string]any{
		"path": path, "content": updated, "expected_digest": digest,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	preview := decodeBody(t, rec)
	if fullDiff, _ := preview["full_diff"].(string); !strings.Contains(fullDiff, "+Run all tests.") {
		t.Fatalf("full_diff = %q", fullDiff)
	}
	pins := extractProposalPins(t, rec)
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		t.Fatalf("pins = %+v", pins)
	}
	if onDisk, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); string(onDisk) != "# Checks\n\nRun tests.\n" {
		t.Fatalf("preview wrote the file: %q", onDisk)
	}

	rec = postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if confirmed := decodeBody(t, rec); confirmed["status"] != "applied" || confirmed["lifecycle_state"] != "active" {
		t.Fatalf("confirm = %v", confirmed)
	}
	if onDisk, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); string(onDisk) != updated {
		t.Fatalf("file after confirm = %q", onDisk)
	}

	// The old digest no longer matches the file now on disk.
	rec = postJSON(t, srv, "/api/v1/skills/review-skill/resources/preview", map[string]any{
		"path": path, "content": "# Mine\n", "expected_digest": digest,
	})
	if rec.Code != http.StatusConflict || extractErrorCode(t, rec) != "edit_conflict" {
		t.Fatalf("stale digest = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestSkillResourcePreviewRejectsBadRequests(t *testing.T) {
	t.Parallel()
	_, srv := newResourceWebWorkspace(t)
	path := webResourceDir + "/references/checks.md"
	current := decodeBody(t, get(t, srv, resourceContentURL("review-skill", path)))
	digest := current["digest"].(string)
	endpoint := "/api/v1/skills/review-skill/resources/preview"

	rec := postJSON(t, srv, endpoint, map[string]any{"path": path, "content": "x\n", "expected_digest": digest, "extra": true})
	if rec.Code != http.StatusBadRequest || extractErrorCode(t, rec) != "invalid_request" {
		t.Fatalf("unknown field = %d (body: %s)", rec.Code, rec.Body.String())
	}

	for name, body := range map[string]map[string]any{
		"no change":   {"path": path, "content": current["content"], "expected_digest": digest},
		"binary":      {"path": path, "content": "a\x00b", "expected_digest": digest},
		"not text":    {"path": webResourceDir + "/assets/logo.png", "content": "x\n", "expected_digest": digest},
		"SKILL.md":    {"path": webResourceDir + "/SKILL.md", "content": "x\n", "expected_digest": digest},
		"traversal":   {"path": webResourceDir + "/../review-skill/SKILL.md", "content": "x\n", "expected_digest": digest},
		"bad digest":  {"path": path, "content": "x\n", "expected_digest": "abc"},
		"too large":   {"path": path, "content": strings.Repeat("a", skill.MaxEditableResourceBytes+1), "expected_digest": digest},
		"empty path":  {"path": "", "content": "x\n", "expected_digest": digest},
		"meta file":   {"path": webResourceDir + "/.meta/skill.yaml", "content": "x\n", "expected_digest": digest},
		"backslashes": {"path": webResourceDir + `\references\checks.md`, "content": "x\n", "expected_digest": digest},
	} {
		rec := postJSON(t, srv, endpoint, body)
		if rec.Code != http.StatusBadRequest || extractErrorCode(t, rec) != "invalid_request" {
			t.Fatalf("%s = %d (body: %s)", name, rec.Code, rec.Body.String())
		}
		var result struct {
			Error struct {
				Render app.ErrorRender `json:"render"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Error.Render.Why == "" {
			t.Fatalf("%s has no WHY: %s", name, rec.Body.String())
		}
	}

	rec = postJSON(t, srv, "/api/v1/skills/unknown-skill/resources/preview", map[string]any{"path": path, "content": "x\n", "expected_digest": digest})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown skill = %d (body: %s)", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, srv, endpoint, map[string]any{"path": webResourceDir + "/references/absent.md", "content": "x\n", "expected_digest": digest})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unlisted path = %d (body: %s)", rec.Code, rec.Body.String())
	}

	payload := []byte(`{"path":"` + path + `","content":"x\n","expected_digest":"` + digest + `"}`)
	req := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	noOrigin := httptest.NewRecorder()
	srv.Handler().ServeHTTP(noOrigin, req)
	if noOrigin.Code != http.StatusForbidden {
		t.Fatalf("missing Origin = %d (body: %s)", noOrigin.Code, noOrigin.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Origin", "http://127.0.0.1:7421")
	req.Header.Set("Content-Type", "text/plain")
	wrongType := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wrongType, req)
	if wrongType.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain body = %d (body: %s)", wrongType.Code, wrongType.Body.String())
	}
}
