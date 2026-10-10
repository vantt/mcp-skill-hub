package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func init() {
	registerRoutes((*Server).registerSkillWriteRoutes)
}

func (s *Server) registerSkillWriteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/skills/add/preview", s.handleSkillAddPreview)
	mux.HandleFunc("POST /api/v1/skills/add/confirm", s.handleSkillAddConfirm)
	mux.HandleFunc("POST /api/v1/skills/create/preview", s.handleSkillCreatePreview)
	mux.HandleFunc("POST /api/v1/skills/{id}/update/preview", s.handleSkillUpdatePreview)
	mux.HandleFunc("POST /api/v1/skills/{id}/transitions/preview", s.handleSkillTransitionPreview)
	mux.HandleFunc("POST /api/v1/skills/proposals/{proposal_id}/confirm", s.handleSkillConfirm)
}

// writePreviewError answers a create or edit preview that failed. Validation failures such as a duplicate
// skill id arrive classified as a generic invalid request with no reason, which leaves the form with
// nothing to show the person, so the validation message becomes the WHY.
func writePreviewError(w http.ResponseWriter, err error, notFound bool) {
	classified := app.ClassifyError(err)
	if !notFound && classified.Code == app.ErrorInvalidRequest && classified.Render.Why == "" {
		detailed := *classified
		detailed.Render.Why = err.Error()
		writeAppError(w, &detailed)
		return
	}
	writeError(w, err, notFound)
}

func decodeJSON(r *http.Request, v any) *app.Error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return app.NewInvalidRequestError(
			"The request body is not valid JSON for this endpoint.",
			"Check the JSON payload format and fields.",
		)
	}
	return nil
}

type skillAddPreviewRequest struct {
	Locator        string `json:"locator"`
	Selection      string `json:"selection,omitempty"`
	All            bool   `json:"all,omitempty"`
	TargetID       string `json:"target_id,omitempty"`
	Collection     string `json:"collection,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func (s *Server) handleSkillAddPreview(w http.ResponseWriter, r *http.Request) {
	var req skillAddPreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	if err := validateGitHubLocator(req.Locator); err != nil {
		writeAppError(w, err)
		return
	}
	proposal, err := s.skillAdd.PreviewSkillAdd(r.Context(), s.workspace, app.SkillAddInput{
		Locator:        req.Locator,
		Selection:      strings.TrimSpace(req.Selection),
		All:            req.All,
		TargetID:       strings.TrimSpace(req.TargetID),
		Collection:     strings.TrimSpace(req.Collection),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		FullDiff:       true,
	})
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

type confirmationRequest struct {
	ProposalID     string `json:"proposal_id"`
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

func (s *Server) handleSkillAddConfirm(w http.ResponseWriter, r *http.Request) {
	var req confirmationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	pID := strings.TrimSpace(req.ProposalID)
	pDigest := strings.TrimSpace(req.ProposalDigest)
	baseVer := strings.TrimSpace(req.BaseVersion)
	if pID == "" || pDigest == "" || baseVer == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"proposal_id, proposal_digest, and base_version pins are required",
			"Supply all confirmation pins.",
		))
		return
	}
	preview, err := s.skillAdd.LoadSkillAddProposal(r.Context(), s.workspace, pID)
	if err != nil {
		writeError(w, err, false)
		return
	}
	result, err := s.skillAdd.ConfirmSkillAdd(r.Context(), s.workspace, preview, app.ConfirmationPins{
		ProposalID:     pID,
		ProposalDigest: pDigest,
		BaseVersion:    baseVer,
	})
	if err != nil {
		writeError(w, err, false)
		return
	}
	if result.Error != nil {
		writeAppError(w, result.Error)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type skillCreatePreviewRequest struct {
	ID             string              `json:"id"`
	Collection     string              `json:"collection,omitempty"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	Content        string              `json:"content"`
	Routing        *skill.RoutingInput `json:"routing,omitempty"`
	Rationale      string              `json:"rationale,omitempty"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
}

func (s *Server) handleSkillCreatePreview(w http.ResponseWriter, r *http.Request) {
	var req skillCreatePreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	col := strings.TrimSpace(req.Collection)
	if col == "" {
		col = "core"
	}
	input := skill.CreateInput{
		ID:             strings.TrimSpace(req.ID),
		Collection:     col,
		Name:           strings.TrimSpace(req.Name),
		Description:    strings.TrimSpace(req.Description),
		Content:        []byte(req.Content),
		Rationale:      strings.TrimSpace(req.Rationale),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	}
	if req.Routing != nil {
		input.Routing = *req.Routing
	}
	proposal, err := s.skills.PreviewCreate(r.Context(), s.workspace, input, true)
	if err != nil {
		writePreviewError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

type skillUpdatePreviewRequest struct {
	Name                  *string             `json:"name,omitempty"`
	Description           *string             `json:"description,omitempty"`
	Content               *string             `json:"content,omitempty"`
	Routing               *skill.RoutingInput `json:"routing,omitempty"`
	Rationale             *string             `json:"rationale,omitempty"`
	ExpectedContentDigest string              `json:"expected_content_digest"`
	IdempotencyKey        string              `json:"idempotency_key,omitempty"`
}

func (s *Server) handleSkillUpdatePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req skillUpdatePreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	input := skill.UpdateInput{
		ExpectedContentDigest: req.ExpectedContentDigest,
		IdempotencyKey:        req.IdempotencyKey,
		Name:                  req.Name,
		Description:           req.Description,
		Routing:               req.Routing,
		Rationale:             req.Rationale,
	}
	if req.Content != nil {
		input.Content = []byte(*req.Content)
		input.SetContent = true
	}
	proposal, err := s.skills.PreviewSkillUpdate(r.Context(), s.workspace, id, input, true)
	if err != nil {
		writePreviewError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

type skillTransitionPreviewRequest struct {
	Target         string `json:"target"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func (s *Server) handleSkillTransitionPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req skillTransitionPreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	target := strings.TrimSpace(req.Target)
	if target != "active" && target != "deprecated" && target != "archived" {
		writeAppError(w, app.NewInvalidRequestError(
			"Target must be active, deprecated, or archived.",
			"Specify a valid lifecycle target state.",
		))
		return
	}
	proposal, err := s.skills.PreviewTransitionWithKey(r.Context(), s.workspace, id, target, true, req.IdempotencyKey)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

type skillConfirmRequest struct {
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

func (s *Server) handleSkillConfirm(w http.ResponseWriter, r *http.Request) {
	proposalID := r.PathValue("proposal_id")
	var req skillConfirmRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	pID := strings.TrimSpace(proposalID)
	pDigest := strings.TrimSpace(req.ProposalDigest)
	baseVer := strings.TrimSpace(req.BaseVersion)
	if pID == "" || pDigest == "" || baseVer == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"proposal_id, proposal_digest, and base_version pins are required",
			"Supply all confirmation pins.",
		))
		return
	}
	preview, err := s.skills.LoadSkillProposal(r.Context(), s.workspace, pID)
	if err != nil {
		writeError(w, err, false)
		return
	}
	result, err := s.skills.ConfirmSkillMutation(r.Context(), s.workspace, preview, app.ConfirmationPins{
		ProposalID:     pID,
		ProposalDigest: pDigest,
		BaseVersion:    baseVer,
	})
	if err != nil {
		writeError(w, err, false)
		return
	}
	if result.Error != nil {
		writeAppError(w, result.Error)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
