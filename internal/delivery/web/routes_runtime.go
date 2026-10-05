package web

import (
	"errors"
	"net/http"

	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func init() {
	registerRoutes((*Server).registerRuntimeRoutes)
}

func (s *Server) registerRuntimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/runtime", s.handleSkillRuntime)
}

func (s *Server) handleSkillRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status, err := s.skills.RuntimeStatus(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, status)
}
