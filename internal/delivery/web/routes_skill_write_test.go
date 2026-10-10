package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func hashWorkspaceFiles(t *testing.T, root string) string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "runtime") || strings.HasPrefix(rel, ".skillhub/transactions") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		data, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func postJSON(t *testing.T, srv *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Origin", "http://127.0.0.1:7421")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func extractProposalPins(t *testing.T, rec *httptest.ResponseRecorder) app.ConfirmationPins {
	t.Helper()
	var prop struct {
		Confirmation struct {
			Confirmation struct {
				Pins app.ConfirmationPins `json:"pins"`
			} `json:"confirmation"`
		} `json:"confirmation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &prop); err != nil {
		t.Fatalf("failed to decode proposal: %v (body: %s)", err, rec.Body.String())
	}
	return prop.Confirmation.Confirmation.Pins
}

func extractErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var res struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode error: %v (body: %s)", err, rec.Body.String())
	}
	return res.Error.Code
}

func extractOperationID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var res struct {
		OperationID string `json:"operation_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	return res.OperationID
}

func TestSkillWriteEndpoints(t *testing.T) {
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	// 1. Create preview for "drafted" -> hash files before & after, assert match
	hashBefore := hashWorkspaceFiles(t, root)
	createReq := skillCreatePreviewRequest{
		ID:          "drafted",
		Collection:  "core",
		Name:        "Drafted Skill",
		Description: "Draft description.",
		Content:     "---\nname: drafted\ndescription: Draft description.\n---\n\n# Drafted\n",
	}
	recCreate := postJSON(t, srv, "/api/v1/skills/create/preview", createReq)
	if recCreate.Code != http.StatusOK {
		t.Fatalf("create preview failed: %d, body: %s", recCreate.Code, recCreate.Body.String())
	}
	hashAfter := hashWorkspaceFiles(t, root)
	if hashBefore != hashAfter {
		t.Fatalf("workspace files changed during preview: before %s, after %s", hashBefore, hashAfter)
	}

	pins := extractProposalPins(t, recCreate)
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		t.Fatalf("missing confirmation pins: %#v", pins)
	}

	// 2. Confirm -> status applied
	confirmReq := skillConfirmRequest{
		ProposalDigest: pins.ProposalDigest,
		BaseVersion:    pins.BaseVersion,
	}
	recConfirm := postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", confirmReq)
	if recConfirm.Code != http.StatusOK {
		t.Fatalf("confirm failed: %d, body: %s", recConfirm.Code, recConfirm.Body.String())
	}
	var confirmRes struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(recConfirm.Body.Bytes(), &confirmRes); err != nil || confirmRes.Status != "applied" {
		t.Fatalf("confirm status unexpected: %s, err: %v", confirmRes.Status, err)
	}
	opID := extractOperationID(t, recConfirm)

	// 3. Confirm again with the same pins -> the same operation_id
	recConfirm2 := postJSON(t, srv, "/api/v1/skills/proposals/"+pins.ProposalID+"/confirm", confirmReq)
	if recConfirm2.Code != http.StatusOK {
		t.Fatalf("idempotent confirm failed: %d, body: %s", recConfirm2.Code, recConfirm2.Body.String())
	}
	opID2 := extractOperationID(t, recConfirm2)
	if opID != "" && opID != opID2 {
		t.Fatalf("operation_id mismatch: first %q, second %q", opID, opID2)
	}

	// 4. Update preview with wrong expected_content_digest -> status 409 and code edit_conflict
	wrongDigest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	newDesc := "New description"
	updateReq := skillUpdatePreviewRequest{
		Description:           &newDesc,
		ExpectedContentDigest: wrongDigest,
	}
	recUpdate := postJSON(t, srv, "/api/v1/skills/drafted/update/preview", updateReq)
	if recUpdate.Code != http.StatusConflict {
		t.Fatalf("expected status 409 for wrong digest, got %d (body: %s)", recUpdate.Code, recUpdate.Body.String())
	}
	if code := extractErrorCode(t, recUpdate); code != "edit_conflict" {
		t.Fatalf("expected code edit_conflict, got %q", code)
	}

	// 5. Transition drafted to archived -> classified error
	recArchived := postJSON(t, srv, "/api/v1/skills/drafted/transitions/preview", skillTransitionPreviewRequest{
		Target: "archived",
	})
	if recArchived.Code == http.StatusOK {
		t.Fatalf("expected error transitioning draft to archived, got 200")
	}
	if code := extractErrorCode(t, recArchived); code == "" {
		t.Fatalf("expected classified error for invalid transition, got %s", recArchived.Body.String())
	}

	// 6. Transition to active while required routing fields are missing -> status 400, code invalid_request
	recActive := postJSON(t, srv, "/api/v1/skills/drafted/transitions/preview", skillTransitionPreviewRequest{
		Target: "active",
	})
	if recActive.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for transition with missing routing, got %d: %s", recActive.Code, recActive.Body.String())
	}
	if code := extractErrorCode(t, recActive); code != "invalid_request" {
		t.Fatalf("expected code invalid_request, got %q", code)
	}

	// 7. Add preview with locator /etc -> status 400 and the WHY text from task 3.1
	recAddEtc := postJSON(t, srv, "/api/v1/skills/add/preview", skillAddPreviewRequest{
		Locator: "/etc",
	})
	if recAddEtc.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for /etc locator, got %d", recAddEtc.Code)
	}
	if !strings.Contains(recAddEtc.Body.String(), "The web UI accepts only public GitHub URLs.") {
		t.Fatalf("expected locator validation error message, got: %s", recAddEtc.Body.String())
	}

	// 8. Create preview A, create & confirm B, then confirm A -> status 409, code stale_proposal
	createA := postJSON(t, srv, "/api/v1/skills/create/preview", skillCreatePreviewRequest{
		ID:          "skill-a",
		Name:        "Skill A",
		Description: "A description",
		Content:     "---\nname: skill-a\ndescription: A\n---\n# A\n",
	})
	if createA.Code != http.StatusOK {
		t.Fatalf("create A failed: %d", createA.Code)
	}
	pinsA := extractProposalPins(t, createA)

	createB := postJSON(t, srv, "/api/v1/skills/create/preview", skillCreatePreviewRequest{
		ID:          "skill-b",
		Name:        "Skill B",
		Description: "B description",
		Content:     "---\nname: skill-b\ndescription: B\n---\n# B\n",
	})
	if createB.Code != http.StatusOK {
		t.Fatalf("create B failed: %d", createB.Code)
	}
	pinsB := extractProposalPins(t, createB)

	// Confirm B
	recConfirmB := postJSON(t, srv, "/api/v1/skills/proposals/"+pinsB.ProposalID+"/confirm", skillConfirmRequest{
		ProposalDigest: pinsB.ProposalDigest,
		BaseVersion:    pinsB.BaseVersion,
	})
	if recConfirmB.Code != http.StatusOK {
		t.Fatalf("confirm B failed: %d, body: %s", recConfirmB.Code, recConfirmB.Body.String())
	}

	// Confirm A with pinsA -> catalog snapshot changed -> 409 stale_proposal
	recConfirmA := postJSON(t, srv, "/api/v1/skills/proposals/"+pinsA.ProposalID+"/confirm", skillConfirmRequest{
		ProposalDigest: pinsA.ProposalDigest,
		BaseVersion:    pinsA.BaseVersion,
	})
	if recConfirmA.Code != http.StatusConflict {
		t.Fatalf("expected 409 for stale proposal confirmation, got %d (body: %s)", recConfirmA.Code, recConfirmA.Body.String())
	}
	if code := extractErrorCode(t, recConfirmA); code != "stale_proposal" {
		t.Fatalf("expected code stale_proposal, got %q", code)
	}
}

func TestSkillConfirmRequiresAllPins(t *testing.T) {
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	const expectedWhy = "proposal_id, proposal_digest, and base_version pins are required"

	// 1. /api/v1/skills/proposals/{proposal_id}/confirm
	// Each missing pin returns 400 and files unchanged
	testCases := []struct {
		name       string
		proposalID string
		body       skillConfirmRequest
	}{
		{
			name:       "missing proposal_digest",
			proposalID: "PROP-123",
			body:       skillConfirmRequest{ProposalDigest: "", BaseVersion: "base-1"},
		},
		{
			name:       "missing base_version",
			proposalID: "PROP-123",
			body:       skillConfirmRequest{ProposalDigest: "sha256:111", BaseVersion: ""},
		},
		{
			name:       "all missing",
			proposalID: "PROP-123",
			body:       skillConfirmRequest{ProposalDigest: "", BaseVersion: ""},
		},
	}

	for _, tc := range testCases {
		t.Run("skill_confirm/"+tc.name, func(t *testing.T) {
			hashBefore := hashWorkspaceFiles(t, root)
			rec := postJSON(t, srv, "/api/v1/skills/proposals/"+tc.proposalID+"/confirm", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), expectedWhy) {
				t.Fatalf("expected error why %q, got: %s", expectedWhy, rec.Body.String())
			}
			hashAfter := hashWorkspaceFiles(t, root)
			if hashBefore != hashAfter {
				t.Fatalf("workspace files changed on failed confirmation")
			}
		})
	}

	// 2. /api/v1/skills/add/confirm
	addTestCases := []struct {
		name string
		body confirmationRequest
	}{
		{
			name: "missing proposal_id",
			body: confirmationRequest{ProposalID: "", ProposalDigest: "sha256:111", BaseVersion: "base-1"},
		},
		{
			name: "missing proposal_digest",
			body: confirmationRequest{ProposalID: "PROP-123", ProposalDigest: "", BaseVersion: "base-1"},
		},
		{
			name: "missing base_version",
			body: confirmationRequest{ProposalID: "PROP-123", ProposalDigest: "sha256:111", BaseVersion: ""},
		},
	}

	for _, tc := range addTestCases {
		t.Run("skill_add_confirm/"+tc.name, func(t *testing.T) {
			hashBefore := hashWorkspaceFiles(t, root)
			rec := postJSON(t, srv, "/api/v1/skills/add/confirm", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), expectedWhy) {
				t.Fatalf("expected error why %q, got: %s", expectedWhy, rec.Body.String())
			}
			hashAfter := hashWorkspaceFiles(t, root)
			if hashBefore != hashAfter {
				t.Fatalf("workspace files changed on failed confirmation")
			}
		})
	}
}

func TestSkillCreatePreviewExplainsWhyItWasRejected(t *testing.T) {
	root := newWebWorkspace(t)
	srv := newTestServer(t, root)

	create := func(id, description string) *httptest.ResponseRecorder {
		return postJSON(t, srv, "/api/v1/skills/create/preview", skillCreatePreviewRequest{
			ID: id, Collection: "core", Name: "Name", Description: description,
		})
	}
	why := func(rec *httptest.ResponseRecorder) string {
		var res struct {
			Error struct {
				Render struct {
					Why string `json:"WHY"`
				} `json:"render"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
		}
		return res.Error.Render.Why
	}

	rec := create("drafted-twice", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing description: want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := why(rec); !strings.Contains(got, "description") {
		t.Fatalf("missing description: WHY should name the field, got %q", got)
	}
}
