package web

import (
	"encoding/json"
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
	mux.HandleFunc("POST /api/v1/skills/{id}/distill", s.handlePostSkillDistill)
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

func (s *Server) handlePostSkillDistill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := findSkillDistillPath(s.workspace, id)
	if err != nil {
		writeError(w, app.NewInvalidRequestError("Skill not found.", "Check skill ID."), true)
		return
	}
	var doc distill.Document
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		writeError(w, app.NewInvalidRequestError("The request body is not valid JSON.", "Check document structure."), false)
		return
	}
	if err := distill.ValidateDocument(&doc); err != nil {
		writeError(w, app.NewInvalidRequestError("Invalid distill document: "+err.Error(), "Ensure all required fields are valid."), false)
		return
	}
	if err := distill.SaveDocument(path, &doc); err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}
