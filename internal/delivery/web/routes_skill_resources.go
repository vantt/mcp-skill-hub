package web

import (
	"errors"
	"net/http"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func init() {
	registerRoutes((*Server).registerSkillResourceRoutes)
}

// registerSkillResourceRoutes exposes the companion files of a skill. An edit
// preview stores a skill_edit proposal that is confirmed through
// POST /api/v1/skills/proposals/{proposal_id}/confirm.
func (s *Server) registerSkillResourceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/resources", s.handleSkillResources)
	mux.HandleFunc("GET /api/v1/skills/{id}/resources/content", s.handleSkillResourceContent)
	mux.HandleFunc("POST /api/v1/skills/{id}/resources/preview", s.handleSkillResourcePreview)
}

func (s *Server) handleSkillResources(w http.ResponseWriter, r *http.Request) {
	list, err := s.skills.ListSkillResources(r.Context(), s.workspace, r.PathValue("id"))
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleSkillResourceContent(w http.ResponseWriter, r *http.Request) {
	resourcePath := r.URL.Query().Get("path")
	if resourcePath == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"The path query parameter is required.",
			"Pass the workspace-relative path of a resource listed for this skill.",
		))
		return
	}
	content, err := s.skills.ReadSkillResource(r.Context(), s.workspace, r.PathValue("id"), resourcePath)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, content)
}

type skillResourcePreviewRequest struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	ExpectedDigest string `json:"expected_digest"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func (s *Server) handleSkillResourcePreview(w http.ResponseWriter, r *http.Request) {
	var req skillResourcePreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	proposal, err := s.skills.PreviewSkillResourceUpdate(r.Context(), s.workspace, r.PathValue("id"), skill.ResourceUpdateInput{
		Path: req.Path, Content: []byte(req.Content), ExpectedDigest: req.ExpectedDigest, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		writePreviewError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}
