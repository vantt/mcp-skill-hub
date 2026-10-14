package web

import (
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
)

func queueSubmission(t *testing.T, root, id, label string, files map[string]string) string {
	t.Helper()
	contents := map[string][]byte{}
	for rel, body := range files {
		contents[rel] = []byte(body)
	}
	archive, err := app.BuildSubmissionBundleFromFiles(id, contents, skillruntime.BundleSubmission{
		ClientLabel: label, Excluded: []skillruntime.BundleExcluded{{Path: ".env", Reason: "looks like a secret"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := app.AddSubmission(root, strings.NewReader(string(archive)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return submission.ID
}

func newSkillMD(id string) string {
	return "---\nname: " + id + "\ndescription: Does " + id + " things. Use when you need " + id + ".\n---\n\n# " + id + "\n"
}

func TestSubmissionRoutesListReviewApproveAndLeaveTheQueue(t *testing.T) {
	t.Parallel()
	root, srv := newThirdPartyReviewWorkspace(t)
	sub := queueSubmission(t, root, "release-notes", "machine-a", map[string]string{"SKILL.md": newSkillMD("release-notes"), "scripts/notes.sh": "echo hi\n"})
	head := headCount(t, root)

	rec := get(t, srv, "/api/v1/review-queue")
	body := decodeBody(t, rec)
	var item map[string]any
	for _, raw := range body["items"].([]any) {
		candidate := raw.(map[string]any)
		assertKeysDeclared(t, candidate, "ReviewQueueItem")
		if candidate["kind"] == "submission" {
			item = candidate
		}
	}
	if item == nil || item["submission_id"] != sub || item["skill_id"] != "release-notes" || item["origin"] != "Submitted from machine-a" {
		t.Fatalf("queue = %v", body["items"])
	}

	rec = get(t, srv, "/api/v1/submissions/"+sub)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	review := decodeBody(t, rec)
	assertKeysDeclared(t, review, "SubmissionReview")
	if review["submission_id"] != sub || review["client_label"] != "machine-a" || review["origin"] != "Submitted from machine-a" || review["case"] != "new" ||
		review["baseline"] != "none" || review["third_party"] != true || review["approved"] != false || review["state"] != "new" {
		t.Fatalf("review = %v", review)
	}
	for _, raw := range review["files"].([]any) {
		file := raw.(map[string]any)
		assertKeysDeclared(t, file, "ReviewFile")
		if file["changed"] != true || file["change"] != "A" {
			t.Fatalf("file = %v", file)
		}
	}
	if excluded := review["excluded"].([]any); len(excluded) != 1 || excluded[0].(map[string]any)["path"] != ".env" {
		t.Fatalf("excluded = %v", review["excluded"])
	}
	digest := review["content_digest"].(string)

	if rec := postJSON(t, srv, "/api/v1/submissions/"+sub+"/approve/preview", map[string]string{"content_digest": "sha256:" + strings.Repeat("0", 64)}); rec.Code != http.StatusConflict {
		t.Fatalf("stale digest status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/v1/submissions/"+sub+"/approve/preview", map[string]string{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing digest status = %d", rec.Code)
	}
	rec = postJSON(t, srv, "/api/v1/submissions/"+sub+"/approve/preview", map[string]string{"content_digest": digest})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve preview = %d (body: %s)", rec.Code, rec.Body.String())
	}
	preview := decodeBody(t, rec)
	assertKeysDeclared(t, preview, "SubmissionApprovalPreview")
	message := preview["default_message"].(string)
	if !strings.HasPrefix(message, "Add release-notes (submitted from machine-a)") || !strings.Contains(message, "approve content") {
		t.Fatalf("default_message = %q", message)
	}
	if headCount(t, root) != head {
		t.Fatal("a preview must not commit")
	}
	if listed, _ := app.ListSubmissions(root); len(listed) != 1 {
		t.Fatalf("a preview changed the queue: %+v", listed)
	}

	pins := extractProposalPins(t, rec)
	// A submission id of another skill never drops the one that waits.
	other := queueSubmission(t, root, "unrelated", "machine-b", map[string]string{"SKILL.md": newSkillMD("unrelated")})
	rec = postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion, "commit_message": message, "submission_id": other,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if _, err := app.ReadSubmission(root, other); err != nil {
		t.Fatalf("a submission for another skill left the queue: %v", err)
	}
	if _, err := app.ReadSubmission(root, sub); err != nil {
		t.Fatalf("the applied submission is dropped only when its id is sent: %v", err)
	}
}

func TestSubmissionConfirmDropsTheAppliedSubmissionAndCommitsOnce(t *testing.T) {
	t.Parallel()
	root, srv := newThirdPartyReviewWorkspace(t)
	sub := queueSubmission(t, root, "release-notes", "machine-a", map[string]string{"SKILL.md": newSkillMD("release-notes")})
	before := mustAtoi(t, headCount(t, root))
	digest := decodeBody(t, get(t, srv, "/api/v1/submissions/"+sub))["content_digest"].(string)
	rec := postJSON(t, srv, "/api/v1/submissions/"+sub+"/approve/preview", map[string]string{"content_digest": digest})
	pins := extractProposalPins(t, rec)
	message := decodeBody(t, rec)["default_message"].(string)

	rec = postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion, "commit_message": message, "submission_id": sub,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if after := mustAtoi(t, headCount(t, root)); after != before+1 {
		t.Fatalf("commits %d -> %d, want exactly one more", before, after)
	}
	if subject, err := exec.Command("git", "-C", root, "log", "-1", "--format=%s").Output(); err != nil || strings.TrimSpace(string(subject)) != message {
		t.Fatalf("last commit = %q, %v", subject, err)
	}
	if rec := get(t, srv, "/api/v1/submissions/"+sub); rec.Code != http.StatusNotFound {
		t.Fatalf("an applied submission is still readable: %d", rec.Code)
	}
	trust, err := (app.SkillService{}).ContentTrustFor(t.Context(), root, "release-notes")
	if err != nil || !trust.Approved {
		t.Fatalf("content not approved in the same commit: %+v %v", trust, err)
	}
	queue := decodeBody(t, get(t, srv, "/api/v1/review-queue"))
	for _, raw := range queue["items"].([]any) {
		if raw.(map[string]any)["submission_id"] == sub {
			t.Fatalf("the applied submission is still queued: %v", queue["items"])
		}
	}
}

func TestSubmissionDiscardAndUnknownIDs(t *testing.T) {
	t.Parallel()
	root, srv := newThirdPartyReviewWorkspace(t)
	sub := queueSubmission(t, root, "release-notes", "machine-a", map[string]string{"SKILL.md": newSkillMD("release-notes")})
	head := headCount(t, root)

	rec := postJSON(t, srv, "/api/v1/submissions/"+sub+"/discard", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("discard = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if body := decodeBody(t, rec); body["submission_id"] != sub || body["status"] != "applied" {
		t.Fatalf("discard body = %v", body)
	}
	if rec := postJSON(t, srv, "/api/v1/submissions/"+sub+"/discard", map[string]string{}); rec.Code != http.StatusNotFound {
		t.Fatalf("second discard = %d", rec.Code)
	}
	for _, path := range []string{"/api/v1/submissions/" + sub, "/api/v1/submissions/SUB-NOPE", "/api/v1/submissions/..%2F..%2Fetc"} {
		if rec := get(t, srv, path); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
	}
	if rec := postJSON(t, srv, "/api/v1/submissions/SUB-NOPE/approve/preview", map[string]string{"content_digest": "sha256:x"}); rec.Code != http.StatusNotFound {
		t.Fatalf("approve preview of an unknown submission = %d", rec.Code)
	}
	if headCount(t, root) != head {
		t.Fatal("a discard must not commit")
	}
}

func TestSubmissionConfirmDropsOnlyTheSubmissionTheProposalWasMadeFrom(t *testing.T) {
	t.Parallel()
	root, srv := newThirdPartyReviewWorkspace(t)
	applied := queueSubmission(t, root, "release-notes", "machine-a", map[string]string{"SKILL.md": newSkillMD("release-notes")})
	bystander := queueSubmission(t, root, "release-notes", "machine-b", map[string]string{"SKILL.md": newSkillMD("release-notes"), "notes.md": "other machine\n"})
	digest := decodeBody(t, get(t, srv, "/api/v1/submissions/"+applied))["content_digest"].(string)
	rec := postJSON(t, srv, "/api/v1/submissions/"+applied+"/approve/preview", map[string]string{"content_digest": digest})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve preview = %d (body: %s)", rec.Code, rec.Body.String())
	}
	pins := extractProposalPins(t, rec)
	// Naming the other submission of the same skill must not drop it.
	rec = postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", map[string]string{
		"proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion, "submission_id": bystander,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d (body: %s)", rec.Code, rec.Body.String())
	}
	if _, err := app.ReadSubmission(root, bystander); err != nil {
		t.Fatalf("a submission the proposal was not made from left the queue: %v", err)
	}
}
