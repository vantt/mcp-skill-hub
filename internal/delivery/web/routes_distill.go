package web

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/distill"
)

func init() {
	registerRoutes((*Server).registerDistillRoutes)
}

func (s *Server) registerDistillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/distill", s.handleGetSkillDistill)
}

func findSkillDistillPath(root, id string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "skills", "*", id, ".meta", "distill.yaml"))
	if err == nil && len(matches) == 1 {
		return matches[0], nil
	}
	p := filepath.Join(root, "skills", "default", id, ".meta", "distill.yaml")
	if _, statErr := os.Stat(filepath.Dir(p)); statErr == nil {
		return p, nil
	}
	return "", os.ErrNotExist
}

func (s *Server) handleGetSkillDistill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := findSkillDistillPath(s.workspace, id)
	if err != nil {
		writeError(w, app.NewInvalidRequestError("Skill distill knowledge not found.", "Check skill ID."), true)
		return
	}
	doc, err := distill.LoadDocument(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, app.NewInvalidRequestError("Skill distill knowledge not found.", "Check skill ID."), true)
			return
		}
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}
