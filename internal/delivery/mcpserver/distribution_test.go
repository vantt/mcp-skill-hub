package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

const bundleCheckSkill = "check-skill"

// newBundleHubFixture creates a workspace with skills shaped like test-audit
// and herdr-cook-plan (same file count and sizes, .meta/ included) and a skill
// whose runtime check runs a script from its folder.
func newBundleHubFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (app.WorkspaceService{}).Init(root, false); err != nil {
		t.Fatal(err)
	}
	for id, files := range remoteBenchShapes {
		directory := filepath.Join(root, "skills", "default", id)
		metadata := fmt.Sprintf("schema_version: 1\nid: %s\nstatus: active\nquality:\n  reviewed: true\nrouting:\n  triggers: [fetch the %s fixture skill]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, id)
		writeFixtureFile(t, filepath.Join(directory, ".meta", "skill.yaml"), []byte(metadata))
		for _, file := range files {
			content := remoteBenchFiller(file.size)
			if file.path == "SKILL.md" {
				header := fmt.Sprintf("---\nname: %s\ndescription: Fixture skill shaped like %s.\n---\n\n# %s\n\n", id, id, id)
				content = append([]byte(header), content[:max(0, file.size-len(header))]...)
			}
			writeFixtureFile(t, filepath.Join(directory, filepath.FromSlash(file.path)), content)
		}
	}
	directory := filepath.Join(root, "skills", "default", bundleCheckSkill)
	writeFixtureFile(t, filepath.Join(directory, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: check-skill\nstatus: active\nquality:\n  reviewed: true\n"+
		"routing:\n  triggers: [verify the remote check fixture]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n"+
		"runtime:\n  setup:\n    check: sh \"$SKILLHUB_SKILL_DIR/scripts/check.sh\"\n"))
	writeFixtureFile(t, filepath.Join(directory, "SKILL.md"), []byte("---\nname: check-skill\ndescription: Runs a check script from its folder.\n---\n\n# Check skill\n"))
	writeFixtureFile(t, filepath.Join(directory, "scripts", "check.sh"), []byte("#!/bin/sh\nset -e\ncat \"$SKILLHUB_SKILL_DIR/references/marker.txt\" > check-ran.txt\npwd >> check-ran.txt\necho check passed\n"))
	writeFixtureFile(t, filepath.Join(directory, "references", "marker.txt"), []byte("marker from the hub\n"))
	if _, err := (app.CatalogService{}).EnsureCatalog(t.Context(), root); err != nil {
		t.Fatalf("ensure catalog: %v", err)
	}
	return root
}

func writeFixtureFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := writeRemoteBenchFile(path, content); err != nil {
		t.Fatal(err)
	}
}

// editFixtureMeta rewrites a fixture skill's .meta/skill.yaml and rebuilds
// the catalog.
func editFixtureMeta(t *testing.T, root, id string, mutate func(map[string]any)) {
	t.Helper()
	path := filepath.Join(root, "skills", "default", id, ".meta", "skill.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	encoded, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, path, encoded)
	rebuildFixtureCatalog(t, root)
}

func rebuildFixtureCatalog(t *testing.T, root string) {
	t.Helper()
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
}

func markFixtureThirdParty(document map[string]any) {
	document["sources"] = []any{map[string]any{
		"id": "upstream-skill", "roles": []string{"upstream"}, "kind": "github", "repository": "https://github.com/example/skills",
	}}
}

// hubDigest returns the manifest digest the hub pins for a skill.
func hubDigest(t *testing.T, root, id string) string {
	t.Helper()
	entries, _, err := (app.DistributionService{}).LookupSkills(t.Context(), root, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(entries[id].Version, "sha256:")
}

// folderDigest computes the manifest digest of every file in directory
// except the cache marker, so a missing or extra file changes it.
func folderDigest(t *testing.T, directory string) (string, int) {
	t.Helper()
	var files []skillruntime.BundleFile
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(directory, path)
		relative = filepath.ToSlash(relative)
		if relative == skillruntime.BundleMetadataName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files = append(files, skillruntime.BundleFile{Path: relative, Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(data))})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := skillruntime.ManifestDigest(files)
	if err != nil {
		t.Fatal(err)
	}
	return digest, len(files)
}

func getBundle(t *testing.T, hub *httpTestHub, id string, mutate func(*http.Request)) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.baseURL+HTTPBundlePath+"?skill="+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+hub.token)
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, body
}

func TestBundleOverHTTPAndStdioShareOneCodePath(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	digest := hubDigest(t, root, "herdr-cook-plan")

	response, archive := getBundle(t, hub, "herdr-cook-plan", nil)
	if response.StatusCode != http.StatusOK || response.Header.Get("ETag") != `"`+digest+`"` || response.Header.Get("Content-Type") != skillruntime.BundleMediaType {
		t.Fatalf("bundle response = %d, headers %v", response.StatusCode, response.Header)
	}
	cache := skillruntime.RemoteCache{Root: t.TempDir()}
	cached, err := cache.Install("hub", "herdr-cook-plan", digest, bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if got, count := folderDigest(t, cached.Directory); got != digest || count != 13 {
		t.Fatalf("extracted folder = %s with %d files, want %s with 13 (.meta excluded)", got, count, digest)
	}

	notModified, body := getBundle(t, hub, "herdr-cook-plan", func(r *http.Request) {
		r.Header.Set("If-None-Match", `"`+strings.Repeat("0", 64)+`", "`+digest+`"`)
	})
	if notModified.StatusCode != http.StatusNotModified || len(body) != 0 || notModified.Header.Get("ETag") != `"`+digest+`"` {
		t.Fatalf("If-None-Match response = %d with %d bytes", notModified.StatusCode, len(body))
	}
	if stale, _ := getBundle(t, hub, "herdr-cook-plan", func(r *http.Request) { r.Header.Set("If-None-Match", `"`+strings.Repeat("0", 64)+`"`) }); stale.StatusCode != http.StatusOK {
		t.Fatalf("a stale held digest got %d", stale.StatusCode)
	}
	if missing, body := getBundle(t, hub, "no-such-skill", nil); missing.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "skill_not_found") {
		t.Fatalf("unknown skill = %d %s", missing.StatusCode, body)
	}
	if unauthorized, _ := getBundle(t, hub, "herdr-cook-plan", func(r *http.Request) { r.Header.Del("Authorization") }); unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bundle without a token = %d", unauthorized.StatusCode)
	}
	if post, _ := getBundle(t, hub, "herdr-cook-plan", func(r *http.Request) { r.Method = http.MethodPost }); post.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST bundle = %d", post.StatusCode)
	}

	// The stdio method returns the same bytes from the same code path.
	session := connectInMemory(t, root)
	result, err := mcp.CallCustomMethod[*bundleParams, *bundleResult](t.Context(), session, "skills/bundle", &bundleParams{SkillID: "herdr-cook-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest != digest || !bytes.Equal(result.Archive, archive) || result.MediaType != skillruntime.BundleMediaType {
		t.Fatalf("stdio bundle digest %s (%d bytes) differs from HTTP %s (%d bytes)", result.Digest, len(result.Archive), digest, len(archive))
	}
	held, err := mcp.CallCustomMethod[*bundleParams, *bundleResult](t.Context(), session, "skills/bundle", &bundleParams{SkillID: "herdr-cook-plan", IfNoneMatch: []string{digest}})
	if err != nil || !held.NotModified || len(held.Archive) != 0 {
		t.Fatalf("stdio bundle with a held digest = %+v, %v", held, err)
	}
}

// connectInMemory connects a client with the skills/bundle method to a
// stdio-equivalent server over in-memory transports.
func connectInMemory(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "bundle-test", Version: "1"}, nil)
	if err := mcp.AddSendingCustomMethod[*bundleParams, *bundleResult](client, "skills/bundle"); err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestBundleOfUnapprovedSkillIsBlockedLikeLocalReads(t *testing.T) {
	root := newBundleHubFixture(t)
	editFixtureMeta(t, root, "test-audit", markFixtureThirdParty)
	hub := startHTTPTestHub(t, root)

	response, body := getBundle(t, hub, "test-audit", nil)
	var refusal bundleError
	if err := json.Unmarshal(body, &refusal); err != nil {
		t.Fatalf("decode refusal %s: %v", body, err)
	}
	if response.StatusCode != http.StatusForbidden || refusal.Code != "content_review_required" || refusal.Message != contentReviewRequiredMessage("test-audit") || len(refusal.ReasonCodes) == 0 {
		t.Fatalf("unapproved bundle = %d %+v", response.StatusCode, refusal)
	}
	held := hubDigest(t, root, "test-audit")
	if again, _ := getBundle(t, hub, "test-audit", func(r *http.Request) { r.Header.Set("If-None-Match", `"`+held+`"`) }); again.StatusCode != http.StatusForbidden {
		t.Fatalf("a held digest bypassed the review gate: %d", again.StatusCode)
	}

	session := connectInMemory(t, root)
	_, err := mcp.CallCustomMethod[*bundleParams, *bundleResult](t.Context(), session, "skills/bundle", &bundleParams{SkillID: "test-audit"})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Message != contentReviewRequiredMessage("test-audit") || !strings.Contains(string(rpcErr.Data), "content_review_required") {
		t.Fatalf("stdio bundle of an unapproved skill = %v", err)
	}
}

func TestNetworkListenerNeverReturnsHubPaths(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	session := connectHTTPClient(t, hub.endpoint, hub.token)

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_get", Arguments: map[string]any{"skill_id": "herdr-cook-plan"}})
	if err != nil || result.IsError {
		t.Fatalf("skill_get over HTTP = %+v, %v", result, err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var outcome struct {
		Result struct {
			Content string         `json:"content"`
			Local   app.LocalSkill `json:"local"`
		} `json:"result"`
	}
	if err := json.Unmarshal(encoded, &outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.Result.Local.Status != app.LocalStatusReady || outcome.Result.Local.ManifestVersion != "sha256:"+hubDigest(t, root, "herdr-cook-plan") || outcome.Result.Content == "" {
		t.Fatalf("skill_get over HTTP local = %+v", outcome.Result.Local)
	}
	if strings.Contains(string(encoded), root) || strings.Contains(string(encoded), filepath.ToSlash(root)) || strings.Contains(string(encoded), "local_path") {
		t.Fatalf("skill_get over HTTP carries a hub path: %s", encoded)
	}

	entries, _, err := (app.DistributionService{}).LookupSkills(t.Context(), root, []string{"herdr-cook-plan"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: entries["herdr-cook-plan"].Resources[0].URI})
	if err != nil {
		t.Fatal(err)
	}
	readJSON, _ := json.Marshal(read)
	if strings.Contains(string(readJSON), "io.skillhub/local_path") || strings.Contains(string(readJSON), root) {
		t.Fatalf("resources/read over HTTP carries a hub path: %.300s", readJSON)
	}
}
