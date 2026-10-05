package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func init() {
	registerRoutes((*Server).registerSourceRoutes)
}

func (s *Server) registerSourceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills/{id}/sources", s.handleSkillSources)
	mux.HandleFunc("POST /api/v1/skills/{id}/upstream/check", s.handleSkillUpstreamCheck)
	mux.HandleFunc("POST /api/v1/skills/{id}/upstream/review", s.handleSkillUpstreamReview)
	mux.HandleFunc("POST /api/v1/upstream/proposals/{proposal_id}/confirm", s.handleUpstreamConfirm)
	mux.HandleFunc("POST /api/v1/skills/{id}/sources/attach/preview", s.handleSkillSourceAttachPreview)
	mux.HandleFunc("POST /api/v1/skills/{id}/sources/{source_id}/detach/preview", s.handleSkillSourceDetachPreview)
	mux.HandleFunc("GET /api/v1/sources", s.handleSourcesList)
	mux.HandleFunc("POST /api/v1/sources/check", s.handleSourcesCheck)
	mux.HandleFunc("POST /api/v1/sources/{id}/unwatch/preview", s.handleSourceUnwatchPreview)
	mux.HandleFunc("POST /api/v1/sources/proposals/{proposal_id}/confirm", s.handleSourceConfirm)
	mux.HandleFunc("POST /api/v1/sources/{id}/import/preview", s.handleSourceImportPreview)
	mux.HandleFunc("POST /api/v1/sources/import/confirm", s.handleSourceImportConfirm)
}

func (s *Server) handleSkillSources(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := s.sources.SkillSources(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSkillUpstreamCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := app.GetSkillUpstream(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, errors.Is(err, skill.ErrNotFound))
		return
	}
	if info.SourceID == "" || info.Status == "untracked" {
		writeAppError(w, app.NewInvalidRequestError(
			"Skill does not track an upstream repository.",
			"Attach an upstream source first.",
		))
		return
	}

	_, _ = s.sources.CheckSources(r.Context(), s.workspace, []string{info.SourceID}, false)
	updated, err := app.GetSkillUpstream(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type upstreamReviewRequest struct {
	TargetCommit   string                   `json:"target_commit,omitempty"`
	Resolutions    []app.UpstreamResolution `json:"resolutions,omitempty"`
	IdempotencyKey string                   `json:"idempotency_key,omitempty"`
}

func (s *Server) handleSkillUpstreamReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req upstreamReviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	input := app.UpstreamUpdateInput{
		SkillID:        id,
		TargetCommit:   req.TargetCommit,
		Resolutions:    req.Resolutions,
		IdempotencyKey: req.IdempotencyKey,
	}
	preview, err := s.upstream.PreviewUpdate(r.Context(), s.workspace, input)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if preview.Error != nil {
		writeAppError(w, preview.Error)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

type upstreamConfirmRequest struct {
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

func (s *Server) handleUpstreamConfirm(w http.ResponseWriter, r *http.Request) {
	proposalID := r.PathValue("proposal_id")
	var req upstreamConfirmRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	pID := strings.TrimSpace(proposalID)
	pDigest := strings.TrimSpace(req.ProposalDigest)
	baseVer := strings.TrimSpace(req.BaseVersion)
	if pID == "" || pDigest == "" || baseVer == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"proposal_id, proposal_digest, and base_version pins are required",
			"Supply all confirmation pins.",
		))
		return
	}

	pins := app.ConfirmationPins{
		ProposalID:     pID,
		ProposalDigest: pDigest,
		BaseVersion:    baseVer,
	}
	dispatched, err := s.skills.DispatchConfirmProposal(r.Context(), s.workspace, pID, &pins)
	if err != nil {
		writeError(w, err, false)
		return
	}
	result, ok := dispatched.(app.UpstreamUpdateResult)
	if !ok {
		writeError(w, fmt.Errorf("proposal %s is not an upstream update proposal", pID), false)
		return
	}
	if result.Error != nil {
		writeAppError(w, result.Error)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type sourceAttachPreviewRequest struct {
	SourceID string `json:"source_id,omitempty"`
	Locator  string `json:"locator,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Path     string `json:"path,omitempty"`
	Cadence  string `json:"cadence,omitempty"`
}

func (s *Server) handleSkillSourceAttachPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req sourceAttachPreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}

	if trimmed := strings.TrimSpace(req.Locator); trimmed != "" {
		if err := validateGitHubLocator(trimmed); err != nil {
			writeAppError(w, err)
			return
		}
	}

	input := app.SourceAttachInput{
		SkillID:  id,
		SourceID: req.SourceID,
		Locator:  req.Locator,
		Ref:      req.Ref,
		Path:     req.Path,
		Cadence:  req.Cadence,
	}
	preview, err := s.sources.PreviewAttach(r.Context(), s.workspace, input)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if preview.Error != nil {
		writeAppError(w, preview.Error)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleSkillSourceDetachPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sourceID := r.PathValue("source_id")
	preview, err := s.sources.PreviewDetach(r.Context(), s.workspace, id, sourceID)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if preview.Error != nil {
		writeAppError(w, preview.Error)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleSourcesList(w http.ResponseWriter, r *http.Request) {
	res, err := s.sources.ListSourceGroups(r.Context(), s.workspace)
	if err != nil {
		writeError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type sourcesCheckRequest struct {
	SourceIDs []string `json:"source_ids,omitempty"`
	All       *bool    `json:"all,omitempty"`
	Due       *bool    `json:"due,omitempty"`
}

func (s *Server) handleSourcesCheck(w http.ResponseWriter, r *http.Request) {
	var req sourcesCheckRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}

	var optionsCount int
	hasIDs := len(req.SourceIDs) > 0
	hasAll := req.All != nil && *req.All
	hasDue := req.Due != nil && *req.Due

	if hasIDs {
		optionsCount++
	}
	if hasAll {
		optionsCount++
	}
	if hasDue {
		optionsCount++
	}

	if optionsCount != 1 {
		writeAppError(w, app.NewInvalidRequestError(
			"Exactly one of source_ids, all, or due must be specified.",
			"Provide source_ids: [...], all: true, or due: true.",
		))
		return
	}

	var res app.SourceCheckResult
	var err error

	if hasAll {
		listRes, lErr := s.sources.ListSources(r.Context(), s.workspace, "")
		if lErr != nil {
			writeError(w, lErr, false)
			return
		}
		var ids []string
		for _, sItem := range listRes.Sources {
			ids = append(ids, sItem.Record.ID)
		}
		res, err = s.sources.CheckSources(r.Context(), s.workspace, ids, false)
	} else if hasDue {
		res, err = s.sources.CheckSources(r.Context(), s.workspace, nil, true)
	} else {
		res, err = s.sources.CheckSources(r.Context(), s.workspace, req.SourceIDs, false)
	}

	if err != nil {
		writeError(w, err, false)
		return
	}
	if res.Error != nil {
		writeAppError(w, res.Error)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSourceUnwatchPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	preview, err := s.sources.PreviewUnwatch(r.Context(), s.workspace, id)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if preview.Error != nil {
		writeAppError(w, preview.Error)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

type sourceConfirmRequest struct {
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

func (s *Server) handleSourceConfirm(w http.ResponseWriter, r *http.Request) {
	proposalID := r.PathValue("proposal_id")
	var req sourceConfirmRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	pID := strings.TrimSpace(proposalID)
	pDigest := strings.TrimSpace(req.ProposalDigest)
	baseVer := strings.TrimSpace(req.BaseVersion)
	if pID == "" || pDigest == "" || baseVer == "" {
		writeAppError(w, app.NewInvalidRequestError(
			"proposal_id, proposal_digest, and base_version pins are required",
			"Supply all confirmation pins.",
		))
		return
	}

	preview, err := s.sources.LoadSourceProposal(r.Context(), s.workspace, pID)
	if err != nil {
		writeError(w, err, false)
		return
	}
	pins := app.ConfirmationPins{
		ProposalID:     pID,
		ProposalDigest: pDigest,
		BaseVersion:    baseVer,
	}
	res, err := s.sources.ConfirmSourceProposal(r.Context(), s.workspace, preview, pins)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if res.Error != nil {
		writeAppError(w, res.Error)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type sourceImportPreviewRequest struct {
	Skills []string `json:"skills,omitempty"`
	Path   string   `json:"path,omitempty"`
}

func (s *Server) handleSourceImportPreview(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	var req sourceImportPreviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAppError(w, err)
		return
	}
	preview, err := s.sourceImport.PreviewSourceImport(r.Context(), s.workspace, app.SourceImportPreviewInput{
		SourceID: sourceID,
		Skills:   req.Skills,
		Path:     req.Path,
	})
	if err != nil {
		writeError(w, err, false)
		return
	}
	if preview.Error != nil {
		writeAppError(w, preview.Error)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleSourceImportConfirm(w http.ResponseWriter, r *http.Request) {
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
	preview, err := s.sourceImport.LoadSourceImportProposal(r.Context(), s.workspace, pID)
	if err != nil {
		writeError(w, err, false)
		return
	}
	pins := app.ConfirmationPins{
		ProposalID:     pID,
		ProposalDigest: pDigest,
		BaseVersion:    baseVer,
	}
	res, err := s.sourceImport.ConfirmSourceImport(r.Context(), s.workspace, preview, pins)
	if err != nil {
		writeError(w, err, false)
		return
	}
	if res.Error != nil {
		writeAppError(w, res.Error)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
