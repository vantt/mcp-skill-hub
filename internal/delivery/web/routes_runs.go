package web

import (
	"errors"
	"net/http"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func init() {
	registerRoutes((*Server).registerRunRoutes)
}

func (s *Server) registerRunRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/runs/{id}", s.handleRunGet)
	mux.HandleFunc("POST /api/v1/runs/{id}/cancel", s.handleRunCancel)
}

func (s *Server) handleRunGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := s.distill.GetDistillRun(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, app.ErrDistillRunNotFound))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type cancelRunRequest struct{}

func (s *Server) handleRunCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req cancelRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}

	res, err := s.distill.CancelDistillRun(r.Context(), s.workspace, id)
	if err != nil {
		if errors.Is(err, app.ErrDistillRunNotFound) {
			writeError(w, err, true)
			return
		}
		if existing, getErr := s.distill.GetDistillRun(r.Context(), s.workspace, id); getErr == nil && existing.Run.State == "cancelled" {
			writeJSON(w, http.StatusOK, existing)
			return
		}
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
