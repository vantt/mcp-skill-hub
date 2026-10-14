package web

import (
	"errors"
	"net/http"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

func init() {
	registerRoutes((*Server).registerSkillFileRoutes)
}

// registerSkillFileRoutes exposes a skill's whole file tree. A changeset
// preview stores one skill_edit proposal that is confirmed through
// POST /api/v1/skills/proposals/{proposal_id}/confirm, so every set of edits is
// one hub commit.
func (s *Server) registerSkillFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/files", s.handleSkillFiles)
	mux.HandleFunc("GET /api/v1/skills/{id}/files/content", s.handleSkillFileContent)
	mux.HandleFunc("POST /api/v1/skills/{id}/changes/preview", s.handleSkillChangesPreview)
}

// writeFileError answers a file request that failed. A skill or file that is
// not there gets a plain statement instead of the generic message the shared
// classifier gives to a missing object.
func writeFileError(w http.ResponseWriter, err error, preview bool) {
	if errors.Is(err, skill.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, app.ErrorResult(app.NewInvalidRequestError(
			"That skill or file is not in the workspace.",
			"Go back to the Files tab and pick a file from the list.",
		)))
		return
	}
	if preview {
		writePreviewError(w, err, false)
		return
	}
	writeError(w, err, false)
}

func (s *Server) handleSkillFiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.skills.ListSkillFiles(r.Context(), s.workspace, r.PathValue("id"))
	if err != nil {
		writeFileError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleSkillFileContent(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"The path query parameter is required.",
			"Pass the path, relative to the skill folder, of a file listed for this skill.",
		))
		return
	}
	content, err := s.skills.ReadSkillFile(r.Context(), s.workspace, r.PathValue("id"), rel)
	if err != nil {
		writeFileError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (s *Server) handleSkillChangesPreview(w http.ResponseWriter, r *http.Request) {
	var req app.SkillChangesInput
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	preview, err := s.skills.PreviewSkillChanges(r.Context(), s.workspace, r.PathValue("id"), req)
	if err != nil {
		writeFileError(w, err, true)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
