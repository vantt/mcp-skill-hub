package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

const webSkillDir = "skills/core/review-skill"

// newFilesWebWorkspace adds a binary file and a text file above the edit
// limit next to the fixture's references/checks.md.
func newFilesWebWorkspace(t *testing.T) (string, *Server) {
	t.Helper()
	root := newWebWorkspace(t)
	files := map[string][]byte{
		"assets/logo.png":      []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
		"references/large.txt": bytes.Repeat([]byte("0123456789abcde\n"), (skill.MaxEditableResourceBytes/16)+64),
	}
	for relative, contents := range files {
		target := filepath.Join(root, filepath.FromSlash(webSkillDir+"/"+relative))
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
	commitFixture(t, root)
	return root, newTestServer(t, root)
}

func fileContentURL(id, path string) string {
	return "/api/v1/skills/" + id + "/files/content?path=" + url.QueryEscape(path)
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

// assertKeysDeclared fails when a response has a field the web client's shared
// types do not declare. Optional fields may be absent from a response.
func assertKeysDeclared(t *testing.T, object map[string]any, name string) {
	t.Helper()
	declared := map[string]bool{}
	for _, field := range tsInterfaceFields(t, name) {
		declared[field] = true
	}
	for key := range object {
		if !declared[key] {
			t.Fatalf("%s does not declare %q", name, key)
		}
	}
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

func TestSkillFilesRoute(t *testing.T) {
	t.Parallel()
	root, srv := newFilesWebWorkspace(t)
	rec := get(t, srv, "/api/v1/skills/review-skill/files")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	assertGolden(t, "skill-files", root, rec)

	body := decodeBody(t, rec)
	assertKeysDeclared(t, body, "SkillFiles")
	files, _ := body["files"].([]any)
	if len(files) == 0 || files[0].(map[string]any)["path"] != "SKILL.md" {
		t.Fatalf("SKILL.md must come first: %v", files)
	}
	editable := map[string]bool{}
	for _, raw := range files {
		item := raw.(map[string]any)
		if got, want := sortedKeys(item), tsInterfaceFields(t, "SkillFileEntry"); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("item fields = %v, want %v", got, want)
		}
		path := item["path"].(string)
		if strings.HasPrefix(path, ".meta/") || strings.HasPrefix(path, "skill.meta") {
			t.Fatalf("list includes hub metadata %s", path)
		}
		editable[path] = item["editable"].(bool)
	}
	for path, want := range map[string]bool{"references/checks.md": true, "assets/logo.png": false, "references/large.txt": false} {
		if got, ok := editable[path]; !ok || got != want {
			t.Fatalf("editable[%s] = %v (present %v), want %v", path, got, ok, want)
		}
	}
	if rec := get(t, srv, "/api/v1/skills/unknown-skill/files"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown skill status = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestSkillFileContentRoute(t *testing.T) {
	t.Parallel()
	_, srv := newFilesWebWorkspace(t)
	rec := get(t, srv, fileContentURL("review-skill", "references/checks.md"))
	body := decodeBody(t, rec)
	if rec.Code != http.StatusOK || body["content"] != "# Checks\n\nRun tests.\n" || body["text"] != true {
		t.Fatalf("checks.md = %d %v", rec.Code, body)
	}
	assertKeysDeclared(t, body, "SkillFileContent")
	if skillMD := decodeBody(t, get(t, srv, fileContentURL("review-skill", "SKILL.md"))); skillMD["editable"] != true {
		t.Fatalf("SKILL.md = %v", skillMD)
	}
	if logo := decodeBody(t, get(t, srv, fileContentURL("review-skill", "assets/logo.png"))); logo["text"] != false || logo["content"] != nil {
		t.Fatalf("a binary file has no text: %v", logo)
	}
	for _, path := range []string{"", "../review-skill/SKILL.md", "/etc/passwd", `references\checks.md`, "references/missing.md", ".meta/skill.yaml"} {
		rec := get(t, srv, fileContentURL("review-skill", path))
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Fatalf("path %q = %d (body: %s)", path, rec.Code, rec.Body.String())
		}
	}
}

func headCount(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestSkillChangesPreviewAndConfirmMakeOneCommit(t *testing.T) {
	t.Parallel()
	root, srv := newFilesWebWorkspace(t)
	list := decodeBody(t, get(t, srv, "/api/v1/skills/review-skill/files"))
	base := list["base_version"].(string)
	checks := decodeBody(t, get(t, srv, fileContentURL("review-skill", "references/checks.md")))
	skillMD := decodeBody(t, get(t, srv, fileContentURL("review-skill", "SKILL.md")))
	before := headCount(t, root)

	rec := postJSON(t, srv, "/api/v1/skills/review-skill/changes/preview", map[string]any{
		"base_version": base,
		"changes": []map[string]any{
			{"action": "modify", "path": "SKILL.md", "content": strings.Replace(skillMD["content"].(string), "Review carefully.", "Review very carefully.", 1), "expected_digest": skillMD["digest"]},
			{"action": "add", "path": "references/ocr.md", "content": "# OCR\n"},
			{"action": "add", "path": "assets/pixel.bin", "content": "AAECAw==", "encoding": "base64"},
			{"action": "rename", "from": "references/checks.md", "path": "references/review-checks.md", "expected_digest": checks["digest"]},
			{"action": "delete", "path": "assets/logo.png"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	preview := decodeBody(t, rec)
	assertKeysDeclared(t, preview, "SkillChangesPreview")
	changes := map[string]string{}
	for _, raw := range preview["files"].([]any) {
		file := raw.(map[string]any)
		assertKeysDeclared(t, file, "SkillChangeFile")
		changes[file["path"].(string)] = file["change"].(string)
	}
	want := map[string]string{"SKILL.md": "M", "references/ocr.md": "A", "assets/pixel.bin": "A", "references/review-checks.md": "R", "assets/logo.png": "D"}
	if len(changes) != len(want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}
	for path, change := range want {
		if changes[path] != change {
			t.Fatalf("changes = %v, want %v", changes, want)
		}
	}
	if preview["default_message"] != "Commit review-skill: 5 files" {
		t.Fatalf("default_message = %v", preview["default_message"])
	}
	if headCount(t, root) != before {
		t.Fatal("a preview must not commit")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(webSkillDir+"/references/ocr.md"))); !os.IsNotExist(err) {
		t.Fatal("a preview must not write")
	}

	pins := extractProposalPins(t, rec)
	rec = postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion, "commit_message": "Tidy review-skill references",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	confirmed := decodeBody(t, rec)
	commit, _ := confirmed["commit"].(map[string]any)
	if confirmed["status"] != "applied" || commit["status"] != "committed" || commit["hash"] == "" {
		t.Fatalf("confirm = %v", confirmed)
	}
	if reach, _ := commit["reach"].(map[string]any); reach == nil {
		t.Fatalf("the commit must say who gets it: %v", commit)
	}
	if after, _ := strconv.Atoi(headCount(t, root)); after != mustAtoi(t, before)+1 {
		t.Fatalf("commits %s -> %d, want exactly one more", before, after)
	}
	if subject, err := exec.Command("git", "-C", root, "log", "-1", "--format=%s").Output(); err != nil || strings.TrimSpace(string(subject)) != "Tidy review-skill references" {
		t.Fatalf("last commit = %q, %v", subject, err)
	}
	if status, err := exec.Command("git", "-C", root, "status", "--porcelain=v1").Output(); err != nil || len(status) != 0 {
		t.Fatalf("the commit left changes behind: %q, %v", status, err)
	}
	onDisk := func(rel string) string {
		data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(webSkillDir+"/"+rel)))
		return string(data)
	}
	if onDisk("references/review-checks.md") != "# Checks\n\nRun tests.\n" || onDisk("references/ocr.md") != "# OCR\n" || onDisk("assets/pixel.bin") != "\x00\x01\x02\x03" {
		t.Fatal("the changeset was not written")
	}
	for _, gone := range []string{"references/checks.md", "assets/logo.png"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(webSkillDir+"/"+gone))); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", gone)
		}
	}

	// The tree the changes were made against is gone.
	rec = postJSON(t, srv, "/api/v1/skills/review-skill/changes/preview", map[string]any{
		"base_version": base, "changes": []map[string]any{{"action": "add", "path": "late.md", "content": "x"}},
	})
	if rec.Code != http.StatusConflict || extractErrorCode(t, rec) != "stale_proposal" {
		t.Fatalf("stale base = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func mustAtoi(t *testing.T, value string) int {
	t.Helper()
	number, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return number
}

func TestSkillChangesPreviewRefusals(t *testing.T) {
	t.Parallel()
	_, srv := newFilesWebWorkspace(t)
	base := decodeBody(t, get(t, srv, "/api/v1/skills/review-skill/files"))["base_version"].(string)
	endpoint := "/api/v1/skills/review-skill/changes/preview"
	one := func(change map[string]any) map[string]any {
		return map[string]any{"base_version": base, "changes": []map[string]any{change}}
	}

	if rec := postJSON(t, srv, endpoint, map[string]any{"base_version": base, "changes": []any{}, "extra": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d (body: %s)", rec.Code, rec.Body.String())
	}
	big := base64.StdEncoding.EncodeToString(make([]byte, 2<<20+1))
	if rec := postJSON(t, srv, endpoint, one(map[string]any{"action": "add", "path": "big.bin", "content": big, "encoding": "base64"})); extractErrorCode(t, rec) != "resource_limits_exceeded" {
		t.Fatalf("oversized upload = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, endpoint, one(map[string]any{"action": "modify", "path": "references/checks.md", "content": "x\n", "expected_digest": "sha256:" + strings.Repeat("0", 64)})); rec.Code != http.StatusConflict || extractErrorCode(t, rec) != "edit_conflict" {
		t.Fatalf("stale file digest = %d (body: %s)", rec.Code, rec.Body.String())
	}
	for name, change := range map[string]map[string]any{
		"traversal":   {"action": "add", "path": "../x.md", "content": "x"},
		"hub meta":    {"action": "add", "path": ".meta/skill.yaml", "content": "x"},
		"secret":      {"action": "add", "path": ".env", "content": "T=1"},
		"delete main": {"action": "delete", "path": "SKILL.md"},
		"backslashes": {"action": "add", "path": `a\b.md`, "content": "x"},
		"no action":   {"path": "a.md", "content": "x"},
		"bad base64":  {"action": "add", "path": "a.bin", "content": "###", "encoding": "base64"},
	} {
		rec := postJSON(t, srv, endpoint, one(change))
		if rec.Code != http.StatusBadRequest || extractErrorCode(t, rec) != "invalid_request" {
			t.Fatalf("%s = %d (body: %s)", name, rec.Code, rec.Body.String())
		}
	}
	if rec := postJSON(t, srv, "/api/v1/skills/unknown-skill/changes/preview", one(map[string]any{"action": "add", "path": "a.md", "content": "x"})); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown skill = %d (body: %s)", rec.Code, rec.Body.String())
	}
}
