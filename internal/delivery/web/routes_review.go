package web

import (
	"net/http"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
)

func init() {
	registerRoutes((*Server).registerReviewRoutes)
}

// registerReviewRoutes exposes what waits for a person and the full review of
// one skill. Approving is a preview confirmed through the generic skill
// confirm, the same change `skill edit --approve-content` makes.
func (s *Server) registerReviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/review-queue", s.handleReviewQueue)
	mux.HandleFunc("GET /api/v1/skills/{id}/review-content", s.handleReviewContent)
	mux.HandleFunc("POST /api/v1/skills/{id}/approve/preview", s.handleApprovePreview)
}

func (s *Server) handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	queue, err := s.skills.ReviewQueue(r.Context(), s.workspace)
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, queue)
}

func (s *Server) handleReviewContent(w http.ResponseWriter, r *http.Request) {
	review, err := s.skills.ReviewSkillContent(r.Context(), s.workspace, r.PathValue("id"))
	if err != nil {
		writeFileError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

type approvePreviewRequest struct {
	ContentDigest string `json:"content_digest"`
}

func (s *Server) handleApprovePreview(w http.ResponseWriter, r *http.Request) {
	var req approvePreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	digest := strings.TrimSpace(req.ContentDigest)
	if digest == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"content_digest is required.",
			"Send the content digest the review screen showed.",
		))
		return
	}
	proposal, err := s.skills.PreviewSkillApproval(r.Context(), s.workspace, r.PathValue("id"), digest)
	if err != nil {
		writeFileError(w, err, true)
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}
