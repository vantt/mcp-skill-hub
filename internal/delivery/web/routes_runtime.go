package web

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

var validSkillID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func init() {
	registerRoutes((*Server).registerRuntimeRoutes)
}

func (s *Server) registerRuntimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/runtime", s.handleSkillRuntime)
}

func (s *Server) handleSkillRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validSkillID.MatchString(id) {
		writeAppError(w, app.NewInvalidRequestError("Invalid skill ID format.", "Use lowercase letters, numbers, and hyphens."))
		return
	}

	status, err := s.skills.RuntimeStatus(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, status)
}
