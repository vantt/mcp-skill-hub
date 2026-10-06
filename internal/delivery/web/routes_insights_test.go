package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/distill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestInboxPaging(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	adapter := fakeSourceAdapter{
		files: map[string]map[string][]byte{
			"r1": {"SKILL.md": []byte("# Version 1\n")},
			"r2": {"SKILL.md": []byte("# Version 2\n")},
		},
		fail: map[string]error{},
	}
	runID := seedRun(t, root, adapter)
	service := app.DistillService{
		Clock:    app.SystemClock{},
		Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter},
	}
	runRes, err := service.GetDistillRun(context.Background(), root, runID)
	if err != nil {
		t.Fatalf("get run failed: %v", err)
	}
	run := runRes.Run
	obsID := distill.ObservationID("source-a", "retry-review")

	// Seed 3 insights in 3 groups: category reliability, clarity, security
	sub := app.DistillSubmission{
		Coverage: []distill.CoverageEntry{
			{Resource: "SKILL.md", Status: "analyzed", Reason: "Read target."},
		},
		Findings: []app.FindingSubmission{
			{
				StableKey:  "retry-review",
				Status:     "active",
				What:       "The source reviews retries.",
				Vocabulary: []string{"retry"},
				Evidence: []distill.Evidence{
					{
						Revision:      distill.IdentityOf(run.ToRevision),
						RunID:         run.ID,
						PackageDigest: run.PackageDigest,
						Path:          "SKILL.md",
						Locator:       "SKILL.md",
						Digest:        sourcepkg.Digest(adapter.files["r2"]["SKILL.md"]),
					},
				},
			},
		},
		Insights: []app.InsightSubmission{
			{
				StableKey:      "insight-rel",
				SkillID:        "review-skill",
				Recommendation: "Add retry reliability guidance.",
				ObservationIDs: []string{obsID},
				Category:       "reliability",
				Priority:       "high",
				Rationale:      "Improves reliability.",
			},
			{
				StableKey:      "insight-cla",
				SkillID:        "review-skill",
				Recommendation: "Add clarity guidance.",
				ObservationIDs: []string{obsID},
				Category:       "clarity",
				Priority:       "medium",
				Rationale:      "Improves clarity.",
			},
			{
				StableKey:      "insight-sec",
				SkillID:        "review-skill",
				Recommendation: "Add security review guidance.",
				ObservationIDs: []string{obsID},
				Category:       "security",
				Priority:       "low",
				Rationale:      "Improves security.",
			},
		},
	}
	if _, err := service.SubmitDistillRun(context.Background(), root, runID, sub); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	srv := newTestServer(t, root)

	// 1. Page 1 with limit=2 -> 2 groups, next_cursor present, has_more=true
	rec1 := get(t, srv, "/api/v1/inbox?limit=2")
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 for page 1, got %d: %s", rec1.Code, rec1.Body.String())
	}
	var page1 inboxResponse
	if err := json.Unmarshal(rec1.Body.Bytes(), &page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Groups) != 2 {
		t.Fatalf("expected 2 groups on page 1, got %d", len(page1.Groups))
	}
	if !page1.HasMore {
		t.Fatalf("expected has_more: true on page 1")
	}
	if page1.NextCursor == "" {
		t.Fatalf("expected non-empty next_cursor on page 1")
	}

	// 2. Page 2 with limit=2&cursor=... -> 1 group, has_more=false
	rec2 := get(t, srv, "/api/v1/inbox?limit=2&cursor="+page1.NextCursor)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for page 2, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var page2 inboxResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Groups) != 1 {
		t.Fatalf("expected 1 group on page 2, got %d", len(page2.Groups))
	}
	if page2.HasMore {
		t.Fatalf("expected has_more: false on page 2")
	}

	// 3. Tampered cursor -> 410 snapshot_expired
	cursorBytes, err := base64.RawURLEncoding.DecodeString(page1.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	cursorBytes[len(cursorBytes)-2] ^= 1
	tampered := base64.RawURLEncoding.EncodeToString(cursorBytes)
	recTampered := get(t, srv, "/api/v1/inbox?limit=2&cursor="+tampered)
	if recTampered.Code != http.StatusGone {
		t.Fatalf("expected 410 for tampered cursor, got %d: %s", recTampered.Code, recTampered.Body.String())
	}
	var errResult app.Result
	if err := json.Unmarshal(recTampered.Body.Bytes(), &errResult); err != nil {
		t.Fatal(err)
	}
	if errResult.Error == nil || errResult.Error.Code != app.ErrorSnapshotExpired {
		t.Fatalf("expected error code snapshot_expired, got: %#v", errResult.Error)
	}

	// 4. Adding an insight between pages -> old cursor returns 410 snapshot_expired
	// We can decide an insight to change the inbox groups or state
	firstInsightID := distill.InsightID("review-skill", "insight-rel")
	_, err = (app.InsightService{}).DecideInsight(context.Background(), root, firstInsightID, app.InsightDecisionInput{
		Decision:  "reject",
		Rationale: "Not needed now.",
	})
	if err != nil {
		t.Fatalf("decide insight failed: %v", err)
	}
	recOldCursor := get(t, srv, "/api/v1/inbox?limit=2&cursor="+page1.NextCursor)
	if recOldCursor.Code != http.StatusGone {
		t.Fatalf("expected 410 for cursor after inbox change, got %d: %s", recOldCursor.Code, recOldCursor.Body.String())
	}
}

func TestInsightEndpoints(t *testing.T) {
	t.Parallel()
	root := newWebWorkspace(t)
	insID := seedPendingInsight(t, root)
	srv := newTestServer(t, root)

	// 1. Detail 200
	recDetail := get(t, srv, "/api/v1/insights/"+insID)
	if recDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 for insight detail, got %d: %s", recDetail.Code, recDetail.Body.String())
	}
	var detailRes app.InsightDetailResult
	if err := json.Unmarshal(recDetail.Body.Bytes(), &detailRes); err != nil {
		t.Fatal(err)
	}
	if detailRes.Insight.ID != insID {
		t.Fatalf("expected insight ID %s, got %s", insID, detailRes.Insight.ID)
	}

	// 2. Unknown ID -> 404 invalid_request
	recUnknown := get(t, srv, "/api/v1/insights/INS-UNKNOWN-ID")
	if recUnknown.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown insight, got %d: %s", recUnknown.Code, recUnknown.Body.String())
	}

	// 3. Decision without rationale -> 400
	recNoRationale := postJSON(t, srv, "/api/v1/insights/"+insID+"/decision", map[string]any{
		"decision":  "plan",
		"rationale": "",
	})
	if recNoRationale.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty rationale, got %d: %s", recNoRationale.Code, recNoRationale.Body.String())
	}

	// 4. Body with idempotency_key -> 400
	recWithKey := postJSON(t, srv, "/api/v1/insights/"+insID+"/decision", map[string]any{
		"decision":        "plan",
		"rationale":       "Plan it.",
		"idempotency_key": "some-key",
	})
	if recWithKey.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for body with idempotency_key, got %d: %s", recWithKey.Code, recWithKey.Body.String())
	}

	// 5. Decision 'plan' with rationale -> applied
	recDecide := postJSON(t, srv, "/api/v1/insights/"+insID+"/decision", map[string]any{
		"decision":  "plan",
		"rationale": "Plan this improvement.",
	})
	if recDecide.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid decision, got %d: %s", recDecide.Code, recDecide.Body.String())
	}
	var decideRes1 app.InsightDecisionResult
	if err := json.Unmarshal(recDecide.Body.Bytes(), &decideRes1); err != nil {
		t.Fatal(err)
	}
	if decideRes1.Insight.Status != "planned" {
		t.Fatalf("expected status 'planned', got %s", decideRes1.Insight.Status)
	}

	// 6. Same decision request again -> same operation_id
	recDecideAgain := postJSON(t, srv, "/api/v1/insights/"+insID+"/decision", map[string]any{
		"decision":  "plan",
		"rationale": "Plan this improvement.",
	})
	if recDecideAgain.Code != http.StatusOK {
		t.Fatalf("expected 200 for idempotent decision, got %d: %s", recDecideAgain.Code, recDecideAgain.Body.String())
	}
	var decideRes2 app.InsightDecisionResult
	if err := json.Unmarshal(recDecideAgain.Body.Bytes(), &decideRes2); err != nil {
		t.Fatal(err)
	}
	if decideRes2.OperationID != decideRes1.OperationID {
		t.Fatalf("expected same operation_id %s, got %s", decideRes1.OperationID, decideRes2.OperationID)
	}

	// 7. Apply preview with unchanged content -> 400
	skillDetail, err := srv.skills.GetSkillDetail(context.Background(), root, "review-skill")
	if err != nil {
		t.Fatal(err)
	}
	obsID := distill.ObservationID("source-a", "retry-review")

	recUnchanged := postJSON(t, srv, "/api/v1/insights/"+insID+"/apply/preview", map[string]any{
		"contents": skillDetail.Content,
		"mappings": []map[string]any{
			{"observation_id": obsID, "concept": "retry"},
		},
	})
	if recUnchanged.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unchanged content, got %d: %s", recUnchanged.Code, recUnchanged.Body.String())
	}

	// 8. Apply preview with missing mapping -> 400
	newContent := skillDetail.Content + "\n\n## Retry Guidance\nAlways retry failed calls.\n"
	recMissingMapping := postJSON(t, srv, "/api/v1/insights/"+insID+"/apply/preview", map[string]any{
		"contents": newContent,
		"mappings": []map[string]any{},
	})
	if recMissingMapping.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing mapping, got %d: %s", recMissingMapping.Code, recMissingMapping.Body.String())
	}

	// 9. Complete valid apply preview -> action_required with diff
	recValidPreview := postJSON(t, srv, "/api/v1/insights/"+insID+"/apply/preview", map[string]any{
		"contents": newContent,
		"mappings": []map[string]any{
			{"observation_id": obsID, "concept": "retry"},
		},
	})
	if recValidPreview.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid apply preview, got %d: %s", recValidPreview.Code, recValidPreview.Body.String())
	}
	var previewRes app.InsightApplicationPreview
	if err := json.Unmarshal(recValidPreview.Body.Bytes(), &previewRes); err != nil {
		t.Fatal(err)
	}
	if previewRes.Status != "action_required" {
		t.Fatalf("expected status 'action_required', got %s", previewRes.Status)
	}
	if !strings.Contains(previewRes.Diff, "Retry Guidance") {
		t.Fatalf("expected diff to contain 'Retry Guidance', got: %s", previewRes.Diff)
	}

	// 10. Confirm with wrong digest -> 409 stale_proposal
	recWrongDigest := postJSON(t, srv, "/api/v1/insights/apply/confirm", map[string]any{
		"proposal_id":     previewRes.ProposalID,
		"proposal_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"base_version":    previewRes.BaseCatalogVersion,
	})
	if recWrongDigest.Code != http.StatusConflict {
		t.Fatalf("expected 409 for wrong digest, got %d: %s", recWrongDigest.Code, recWrongDigest.Body.String())
	}
	var staleErrRes app.Result
	if err := json.Unmarshal(recWrongDigest.Body.Bytes(), &staleErrRes); err != nil {
		t.Fatal(err)
	}
	if staleErrRes.Error == nil || staleErrRes.Error.Code != app.ErrorStaleProposal {
		t.Fatalf("expected error code stale_proposal, got: %#v", staleErrRes.Error)
	}

	// 11. Confirm with exact pins -> 200 and receipt
	recConfirm := postJSON(t, srv, "/api/v1/insights/apply/confirm", map[string]any{
		"proposal_id":     previewRes.ProposalID,
		"proposal_digest": previewRes.ProposalDigest,
		"base_version":    previewRes.BaseCatalogVersion,
	})
	if recConfirm.Code != http.StatusOK {
		t.Fatalf("expected 200 for exact confirm pins, got %d: %s", recConfirm.Code, recConfirm.Body.String())
	}
	var confirmRes app.InsightApplicationResult
	if err := json.Unmarshal(recConfirm.Body.Bytes(), &confirmRes); err != nil {
		t.Fatal(err)
	}
	if confirmRes.Status != "applied" {
		t.Fatalf("expected status 'applied', got %s", confirmRes.Status)
	}
	if confirmRes.OperationID == "" {
		t.Fatalf("expected non-empty operation_id in confirmation receipt")
	}
}
