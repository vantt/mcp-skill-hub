package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/version"
)

func init() {
	registerRoutes((*Server).registerReadRoutes)
}

func (s *Server) registerReadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/session", s.handleSession)
	mux.HandleFunc("GET /api/v1/home", s.handleHome)
	mux.HandleFunc("GET /api/v1/skills", s.handleSkills)
	mux.HandleFunc("GET /api/v1/skills/{id}", s.handleSkillDetail)
	mux.HandleFunc("GET /api/v1/skills/{id}/review", s.handleSkillReview)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sum := sha256.Sum256([]byte(s.workspace))
	session := map[string]any{
		"api_version":      1,
		"skillhub_version": version.Version,
		"workspace_id":     "sha256:" + hex.EncodeToString(sum[:]),
		"workspace_name":   filepath.Base(s.workspace),
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	home, err := s.curation.GetCurationHome(r.Context(), s.workspace)
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, home)
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state != "" {
		valid := false
		for _, candidate := range app.SkillStates {
			if state == candidate {
				valid = true
				break
			}
		}
		if !valid {
			writeAppError(w, app.NewInvalidRequestError("Unknown lifecycle state.", "Use draft, active, deprecated or archived."))
			return
		}
	}

	list, err := s.skills.ListSkills(r.Context(), s.workspace, state)
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleSkillDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := s.skills.GetSkillDetail(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleSkillReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	review, err := s.skills.ReviewSkill(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, review)
}
