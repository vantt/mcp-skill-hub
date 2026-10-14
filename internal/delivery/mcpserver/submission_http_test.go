package mcpserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"golang.org/x/time/rate"
)

func submissionArchive(t *testing.T, id, label string, extra map[string]string) []byte {
	t.Helper()
	files := map[string][]byte{"SKILL.md": []byte("---\nname: " + id + "\ndescription: Does " + id + ". Use when you need " + id + ".\n---\n\n# " + id + "\n")}
	for rel, body := range extra {
		files[rel] = []byte(body)
	}
	return forgeSubmission(t, id, label, files)
}

// forgeSubmission writes a bundle archive without WriteBundle's own limits, so
// the hub's checks are what is tested.
func forgeSubmission(t *testing.T, id, label string, files map[string][]byte) []byte {
	t.Helper()
	var listed []skillruntime.BundleFile
	for rel, data := range files {
		listed = append(listed, skillruntime.BundleFile{Path: rel, Digest: sourcepkg.Digest(data), Size: int64(len(data))})
	}
	digest, err := skillruntime.ManifestDigest(listed)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(skillruntime.BundleMetadata{
		Schema: skillruntime.BundleSchema, SkillID: id, ManifestVersion: "sha256:" + digest, Files: listed,
		DepsKey: "0000000000000000", Submission: &skillruntime.BundleSubmission{ClientLabel: label},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	compressor := gzip.NewWriter(&out)
	archive := tar.NewWriter(compressor)
	write := func(name string, data []byte) {
		if err := archive.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write(skillruntime.BundleMetadataName, metadata)
	for _, file := range listed {
		write(file.Path, files[file.Path])
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func postSubmission(t *testing.T, hub *httpTestHub, token string, body []byte, mutate func(*http.Request)) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, hub.baseURL+HTTPSubmissionsPath, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", skillruntime.BundleMediaType)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reply, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, reply
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var reply bundleError
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("not an error envelope: %s", body)
	}
	return reply.Code
}

func TestSubmissionEndpointRequiresTheHubToken(t *testing.T) {
	hub := startHTTPTestHub(t, newBundleHubFixture(t))
	body := submissionArchive(t, "team-release", "machine-a", nil)
	for _, token := range []string{"", "wrong-token-wrong-token-wrong-token"} {
		response, _ := postSubmission(t, hub, token, body, nil)
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d, want 401", token, response.StatusCode)
		}
	}
	response, _ := postSubmission(t, hub, hub.token, body, func(request *http.Request) { request.Host = "evil.example" })
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Host: status = %d, want 403", response.StatusCode)
	}
	if queued, _ := app.ListSubmissions(hub.root); len(queued) != 0 {
		t.Fatalf("a refused request queued %+v", queued)
	}
}

func TestSubmissionEndpointQueuesAndChangesNothing(t *testing.T) {
	root := newBundleHubFixture(t)
	hub := startHTTPTestHub(t, root)
	before := skillFolderNames(t, root)

	response, body := postSubmission(t, hub, hub.token, submissionArchive(t, "team-release", "machine-a", map[string]string{"scripts/bump.sh": "echo 1\n"}), nil)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
	var reply submissionReply
	if err := json.Unmarshal(body, &reply); err != nil || reply.Status != "waiting" || !strings.HasPrefix(reply.SubmissionID, "SUB-") {
		t.Fatalf("reply = %s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "submissions", reply.SubmissionID, "submission.json")); err != nil {
		t.Fatalf("queue entry missing: %v", err)
	}
	queued, err := app.ReadSubmission(root, reply.SubmissionID)
	if err != nil || queued.SkillID != "team-release" || queued.ClientLabel != "machine-a" || len(queued.Files) != 2 {
		t.Fatalf("queued = %+v, %v", queued, err)
	}
	if after := skillFolderNames(t, root); after != before {
		t.Fatalf("skill folders changed: %s -> %s", before, after)
	}
	// The hub serves nothing of it.
	if response, _ := getBundle(t, hub, "team-release", nil); response.StatusCode != http.StatusNotFound {
		t.Fatalf("bundle of a queued submission = %d, want 404", response.StatusCode)
	}
}

func skillFolderNames(t *testing.T, root string) string {
	t.Helper()
	var names []string
	_ = filepath.WalkDir(filepath.Join(root, "skills"), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			names = append(names, rel)
		}
		return nil
	})
	return strings.Join(names, ",")
}

func TestSubmissionEndpointLimits(t *testing.T) {
	hub := startHTTPTestHub(t, newBundleHubFixture(t))
	many := map[string][]byte{"SKILL.md": []byte("---\nname: x\n---\n")}
	for index := 0; index < skillruntime.BundleMaxFiles; index++ {
		many[fmt.Sprintf("f/%03d.txt", index)] = []byte("x")
	}
	big := map[string][]byte{"SKILL.md": bytes.Repeat([]byte("0"), skillruntime.BundleMaxBytes+1<<20)}
	for name, files := range map[string]map[string][]byte{"513 files": many, "17 MiB": big} {
		response, body := postSubmission(t, hub, hub.token, forgeSubmission(t, "big-one", "machine-a", files), nil)
		if response.StatusCode != http.StatusRequestEntityTooLarge || errorCode(t, body) != "resource_limits_exceeded" {
			t.Errorf("%s: status = %d, body %s", name, response.StatusCode, body)
		}
	}
	// A body past the archive limit is refused before it is read in full.
	response, body := postSubmission(t, hub, hub.token, bytes.Repeat([]byte("z"), skillruntime.BundleMaxArchiveBytes+4096), nil)
	if response.StatusCode != http.StatusRequestEntityTooLarge && response.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized body: status = %d, body %s", response.StatusCode, body)
	}
}

func TestSubmissionEndpointRefusesUnsafeContent(t *testing.T) {
	hub := startHTTPTestHub(t, newBundleHubFixture(t))
	skillMD := []byte("---\nname: x\n---\n")
	for name, files := range map[string]map[string][]byte{
		"traversal":       {"SKILL.md": skillMD, "../escape.txt": []byte("x")},
		"absolute":        {"SKILL.md": skillMD, "/etc/passwd": []byte("x")},
		"hub metadata":    {"SKILL.md": skillMD, ".meta/skill.yaml": []byte("status: active\n")},
		"checkout marker": {"SKILL.md": skillMD, app.CheckoutMarkerName: []byte("{}")},
		"secret":          {"SKILL.md": skillMD, ".env": []byte("TOKEN=1")},
		"no SKILL.md":     {"notes.md": []byte("x")},
	} {
		response, body := postSubmission(t, hub, hub.token, forgeSubmission(t, "team-release", "machine-a", files), nil)
		if response.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
			t.Errorf("%s: status = %d, body %s", name, response.StatusCode, body)
		}
	}
	if response, _ := postSubmission(t, hub, hub.token, []byte("not an archive"), nil); response.StatusCode != http.StatusBadRequest {
		t.Errorf("garbage body: status = %d", response.StatusCode)
	}
	if queued, _ := app.ListSubmissions(hub.root); len(queued) != 0 {
		t.Fatalf("refused submissions were queued: %+v", queued)
	}
}

func TestSubmissionEndpointCapsTheQueueAndReplacesSameMachine(t *testing.T) {
	hub := startHTTPTestHub(t, newBundleHubFixture(t))
	for index := 0; index < app.MaxWaitingSubmissions; index++ {
		response, body := postSubmission(t, hub, hub.token, submissionArchive(t, "team-release", fmt.Sprintf("machine-%02d", index), nil), nil)
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("submission %d: %d %s", index, response.StatusCode, body)
		}
	}
	response, body := postSubmission(t, hub, hub.token, submissionArchive(t, "team-release", "machine-extra", nil), nil)
	if response.StatusCode != http.StatusTooManyRequests || errorCode(t, body) != "submission_queue_full" {
		t.Fatalf("51st submission: %d %s", response.StatusCode, body)
	}
	response, body = postSubmission(t, hub, hub.token, submissionArchive(t, "team-release", "machine-00", map[string]string{"notes.md": "again\n"}), nil)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("same machine again: %d %s", response.StatusCode, body)
	}
	if queued, _ := app.ListSubmissions(hub.root); len(queued) != app.MaxWaitingSubmissions {
		t.Fatalf("queue holds %d, want %d", len(queued), app.MaxWaitingSubmissions)
	}
}

func TestSubmissionEndpointMethodAndMediaType(t *testing.T) {
	hub := startHTTPTestHub(t, newBundleHubFixture(t))
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.baseURL+HTTPSubmissionsPath, nil)
	request.Header.Set("Authorization", "Bearer "+hub.token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != http.MethodPost {
		t.Errorf("GET: %d, Allow %q", response.StatusCode, response.Header.Get("Allow"))
	}
	response, _ = postSubmission(t, hub, hub.token, submissionArchive(t, "team-release", "m", nil), func(request *http.Request) { request.Header.Set("Content-Type", "application/json") })
	if response.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("wrong media type: %d", response.StatusCode)
	}
}

func TestSubmissionEndpointIsRateLimited(t *testing.T) {
	gate, adapter, token := newTestGate(t)
	gate.submissions = adapter.submissionHTTPHandler()
	gate.limiter = rate.NewLimiter(rate.Limit(0.001), 1)
	send := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7420"+HTTPSubmissionsPath, bytes.NewReader([]byte("x")))
		request.RemoteAddr = "127.0.0.1:50000"
		request.Header.Set("Content-Type", skillruntime.BundleMediaType)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		gate.ServeHTTP(recorder, request)
		return recorder
	}
	if first := send(); first.Code != http.StatusBadRequest {
		t.Fatalf("first request = %d, want 400 from the handler", first.Code)
	}
	if second := send(); second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429 from the limiter", second.Code)
	}
}
