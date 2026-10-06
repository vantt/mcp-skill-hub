package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
)

func init() {
	registerRoutes((*Server).registerInsightRoutes)
}

func (s *Server) registerInsightRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/inbox", s.handleInboxList)
	mux.HandleFunc("GET /api/v1/insights/{id}", s.handleInsightGet)
	mux.HandleFunc("POST /api/v1/insights/{id}/decision", s.handleInsightDecide)
	mux.HandleFunc("POST /api/v1/insights/{id}/apply/preview", s.handleInsightApplyPreview)
	mux.HandleFunc("POST /api/v1/insights/apply/confirm", s.handleInsightApplyConfirm)
}

type inboxResponse struct {
	SchemaVersion string                  `json:"schema_version"`
	Status        app.Status              `json:"status"`
	Summary       string                  `json:"summary"`
	Groups        []app.InsightInboxGroup `json:"groups"`
	HasMore       bool                    `json:"has_more"`
	NextCursor    string                  `json:"next_cursor,omitempty"`
	Total         int                     `json:"total"`
}

func (s *Server) handleInboxList(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		val, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeAppError(w, app.NewInvalidRequestError("limit must be an integer", "Provide an integer between 1 and 100."))
			return
		}
		limit = val
	}
	normLimit, err := paging.NormalizeLimit(limit)
	if err != nil {
		writeAppError(w, app.NewInvalidRequestError(err.Error(), "Provide a limit between 1 and 100."))
		return
	}

	result, err := (app.InsightService{}).GetInsightInbox(r.Context(), s.workspace)
	if err != nil {
		writeError(w, err, false)
		return
	}

	owner := paging.Owner("inbox", result.Groups)
	cursor := r.URL.Query().Get("cursor")
	lastKey, err := paging.DecodeCursor(cursor, owner, "inbox")
	if err != nil {
		writeAppError(w, &app.Error{
			Code:      app.ErrorSnapshotExpired,
			Retryable: true,
			Render: app.ErrorRender{
				Error: "The inbox changed since this page was loaded.",
				Why:   "The cursor no longer matches the current inbox.",
				Fix:   "Reload the inbox from the first page.",
			},
		})
		return
	}

	paged, err := paging.Make(result.Groups, normLimit, lastKey, owner, "inbox", func(g app.InsightInboxGroup) string {
		return g.SkillID + "\x00" + g.Category
	})
	if err != nil {
		writeAppError(w, &app.Error{
			Code:      app.ErrorSnapshotExpired,
			Retryable: true,
			Render: app.ErrorRender{
				Error: "The inbox changed since this page was loaded.",
				Why:   "The cursor no longer matches the current inbox.",
				Fix:   "Reload the inbox from the first page.",
			},
		})
		return
	}

	resp := inboxResponse{
		SchemaVersion: result.SchemaVersion,
		Status:        result.Status,
		Summary:       result.Summary,
		Groups:        paged.Items,
		HasMore:       paged.HasMore,
		NextCursor:    paged.NextCursor,
		Total:         paged.Total,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInsightGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := (app.InsightService{}).GetInsightDetail(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, app.ErrInsightNotFound))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type insightDecisionRequest struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

func (s *Server) handleInsightDecide(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req insightDecisionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	if strings.TrimSpace(req.Decision) == "" {
		writeAppError(w, app.NewInvalidRequestError("decision is required", "Provide a valid decision (plan, reject, obsolete, or reopen)."))
		return
	}
	res, err := (app.InsightService{}).DecideInsight(r.Context(), s.workspace, id, app.InsightDecisionInput{
		Decision:  req.Decision,
		Rationale: req.Rationale,
	})
	if err != nil {
		writeError(w, err, errors.Is(err, app.ErrInsightNotFound))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type insightApplyMapping struct {
	ObservationID string `json:"observation_id"`
	Concept       string `json:"concept"`
}

type insightApplyPreviewRequest struct {
	Contents string                `json:"contents"`
	Mappings []insightApplyMapping `json:"mappings"`
}

func (s *Server) handleInsightApplyPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req insightApplyPreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}

	detail, err := (app.InsightService{}).GetInsightDetail(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, app.ErrInsightNotFound))
		return
	}

	skillDetail, err := s.skills.GetSkillDetail(r.Context(), s.workspace, detail.Insight.SkillID)
	if err != nil {
		writeError(w, err, false)
		return
	}

	targetPath := skillDetail.Path
	changes := []app.ApplicationChange{
		{
			Path:     targetPath,
			Contents: req.Contents,
		},
	}
	var mappings []app.ApplicationMapping
	for _, m := range req.Mappings {
		mappings = append(mappings, app.ApplicationMapping{
			ObservationID: strings.TrimSpace(m.ObservationID),
			ArtifactPath:  targetPath,
			Concept:       strings.TrimSpace(m.Concept),
		})
	}

	preview, err := (app.InsightService{}).PreviewInsightApplication(r.Context(), s.workspace, id, app.PreviewInsightInput{
		Changes:  changes,
		Mappings: mappings,
	})
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleInsightApplyConfirm(w http.ResponseWriter, r *http.Request) {
	var req confirmationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	pID := strings.TrimSpace(req.ProposalID)
	pDigest := strings.TrimSpace(req.ProposalDigest)
	baseVer := strings.TrimSpace(req.BaseVersion)
	if pID == "" || pDigest == "" || baseVer == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"proposal_id, proposal_digest, and base_version pins are required",
			"Supply all confirmation pins.",
		))
		return
	}

	result, err := (app.InsightService{}).ConfirmInsightApplication(r.Context(), s.workspace, pID, pDigest, baseVer)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if result.Error != nil {
		writeAppError(w, result.Error)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
