package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func init() {
	registerRoutes((*Server).registerUsageRoutes)
}

func (s *Server) registerUsageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/usage", s.handleSkillUsage)
}

func (s *Server) handleSkillUsage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sinceParam := r.URL.Query().Get("since")
	now := s.opts.Now().UTC()
	until := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var since time.Time

	switch sinceParam {
	case "", "30d":
		since = until.AddDate(0, 0, -30)
	case "7d":
		since = until.AddDate(0, 0, -7)
	case "90d":
		since = until.AddDate(0, 0, -90)
	case "180d":
		since = until.AddDate(0, 0, -180)
	default:
		writeAppError(w, app.NewInvalidRequestError("Invalid since parameter.", "Use 7d, 30d, 90d, or 180d."))
		return
	}

	usageService := app.UsageService{
		Skills: s.skills,
	}
	report, err := usageService.Funnel(r.Context(), s.workspace, app.FunnelQuery{
		Since:   since,
		Until:   until,
		SkillID: id,
	})
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, report)
}
