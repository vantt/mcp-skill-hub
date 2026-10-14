package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
)

func init() {
	registerRoutes((*Server).registerSubmissionRoutes)
}

// registerSubmissionRoutes exposes the skills other machines sent for review.
// A submission is read like a skill's review content; approving it is a
// preview confirmed through the generic proposal confirm, which applies the
// files and approves their content as one change.
func (s *Server) registerSubmissionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/submissions/{sid}", s.handleSubmission)
	mux.HandleFunc("POST /api/v1/submissions/{sid}/approve/preview", s.handleSubmissionApprovePreview)
	mux.HandleFunc("POST /api/v1/submissions/{sid}/discard", s.handleSubmissionDiscard)
}

func writeSubmissionError(w http.ResponseWriter, err error, preview bool) {
	if errors.Is(err, app.ErrSubmissionNotFound) {
		writeJSON(w, http.StatusNotFound, app.ErrorResult(app.NewInvalidRequestError(
			"That submission is not waiting any more.",
			"It may have been approved or discarded; open Waiting for review.",
		)))
		return
	}
	if preview {
		writePreviewError(w, err, false)
		return
	}
	writeError(w, err, false)
}

func (s *Server) handleSubmission(w http.ResponseWriter, r *http.Request) {
	review, err := s.skills.ReviewSubmissionContent(r.Context(), s.workspace, r.PathValue("sid"))
	if err != nil {
		writeSubmissionError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

func (s *Server) handleSubmissionApprovePreview(w http.ResponseWriter, r *http.Request) {
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
	proposal, err := s.skills.PreviewSubmissionApproval(r.Context(), s.workspace, r.PathValue("sid"), digest)
	if err != nil {
		writeSubmissionError(w, err, true)
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

type submissionDiscardResult struct {
	app.Result
	SubmissionID string `json:"submission_id"`
}

func (s *Server) handleSubmissionDiscard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("sid")
	if err := app.DiscardSubmission(s.workspace, id); err != nil {
		writeSubmissionError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, submissionDiscardResult{Result: app.NewResult(app.StatusApplied, "Discarded "+id+"."), SubmissionID: id})
}

// dropAppliedSubmission removes a submission from the queue once the proposal
// made from it has been confirmed. It leaves any submission the proposal was
// not made from, whatever id the request names.
func (s *Server) dropAppliedSubmission(id, proposalID string) {
	id = strings.TrimSpace(id)
	if id == "" || !app.SubmissionBoundToProposal(s.workspace, id, proposalID) {
		return
	}
	if err := app.DiscardSubmission(s.workspace, id); err != nil {
		slog.Warn("an applied submission could not be removed from the queue", "submission_id", id, "error", err)
	}
}
