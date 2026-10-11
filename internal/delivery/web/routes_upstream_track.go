package web

import (
	"net/http"
)

func init() {
	registerRoutes((*Server).registerUpstreamTrackRoutes)
}

// registerUpstreamTrackRoutes exposes the preview that links a skill's
// untracked upstream to a source record. The proposal it stores is confirmed
// through POST /api/v1/sources/proposals/{proposal_id}/confirm.
func (s *Server) registerUpstreamTrackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/skills/{id}/upstream/track/preview", s.handleSkillUpstreamTrackPreview)
}

func (s *Server) handleSkillUpstreamTrackPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	preview, err := s.sources.PreviewTrackUpstream(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if preview.Error != nil {
		if preview.SkillNotFound() {
			writeJSON(w, http.StatusNotFound, preview.Result)
			return
		}
		writeAppError(w, preview.Error)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
