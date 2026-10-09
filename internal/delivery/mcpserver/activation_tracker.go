package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

const (
	maxSessionsPerTracker    = 256
	maxResolutionsPerSession = 32
	resolutionTTL            = 2 * time.Hour
)

type notedResolution struct {
	ResolutionID    string
	Status          string
	PrimaryID       string
	SupportingIDs   []string
	CatalogSnapshot string
	PolicyRevision  string
	Client          telemetry.Client
	At              time.Time
	TopKSkillIDs    []string
	TopKMatched     []string
	TopKChannels    []string
	PriorVerified   bool
	TaskDescription string
	Operation       string
	Request         map[string]any
}

type sessionState struct {
	hash        string
	lastSeen    time.Time
	resolutions []notedResolution
	activated   map[string]bool
}

func (s *sessionState) pruneResolutions(cutoff time.Time) {
	validStart := 0
	for validStart < len(s.resolutions) && s.resolutions[validStart].At.Before(cutoff) {
		validStart++
	}
	if validStart > 0 {
		s.resolutions = s.resolutions[validStart:]
	}
}

func newSessionState(now time.Time) *sessionState {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("read session entropy: " + err.Error())
	}
	return &sessionState{
		hash:      hex.EncodeToString(raw[:]),
		lastSeen:  now,
		activated: make(map[string]bool),
	}
}

type activationTracker struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[*mcp.ServerSession]*sessionState
	anon     *sessionState
	redactor *telemetry.Redactor
}

func newActivationTracker(now func() time.Time, workspacePath string) *activationTracker {
	if now == nil {
		now = time.Now
	}
	return &activationTracker{
		now:      now,
		sessions: make(map[*mcp.ServerSession]*sessionState),
		anon:     newSessionState(now()),
		redactor: telemetry.NewRedactor(workspacePath),
	}
}

func (t *activationTracker) sessionState(session *mcp.ServerSession) *sessionState {
	if session == nil {
		return t.anon
	}
	if state, ok := t.sessions[session]; ok {
		return state
	}
	if len(t.sessions) >= maxSessionsPerTracker {
		var oldestSession *mcp.ServerSession
		var oldestTime time.Time
		for s, state := range t.sessions {
			if oldestSession == nil || state.lastSeen.Before(oldestTime) {
				oldestSession = s
				oldestTime = state.lastSeen
			}
		}
		if oldestSession != nil {
			delete(t.sessions, oldestSession)
		}
	}
	state := newSessionState(t.now())
	t.sessions[session] = state
	return state
}

func (t *activationTracker) noteResolution(session *mcp.ServerSession, request resolverpkg.Request, response resolverpkg.Response, priorVerified bool, client ...telemetry.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessionState(session)
	now := t.now()
	state.lastSeen = now
	state.pruneResolutions(now.Add(-resolutionTTL))

	var c telemetry.Client
	if len(client) > 0 {
		c = client[0]
	}

	primaryID := ""
	if response.Primary != nil {
		primaryID = response.Primary.ID
	}
	supportingIDs := make([]string, 0, len(response.Supporting))
	for _, s := range response.Supporting {
		if s.ID != "" {
			supportingIDs = append(supportingIDs, s.ID)
		}
	}

	state.resolutions = append(state.resolutions, notedResolution{
		ResolutionID:    response.ResolutionID,
		Status:          string(response.Status),
		PrimaryID:       primaryID,
		SupportingIDs:   supportingIDs,
		CatalogSnapshot: response.CatalogSnapshot,
		PolicyRevision:  response.PolicyRevision,
		Client:          c,
		At:              now,
		TopKSkillIDs:    append([]string(nil), response.TopKSkillIDs...),
		TopKMatched:     append([]string(nil), response.TopKMatched...),
		TopKChannels:    append([]string(nil), response.TopKChannels...),
		PriorVerified:   priorVerified,
		TaskDescription: t.redactor.Redact(request.Task.Description),
		Operation:       request.Operation,
		Request:         redactRequest(t.redactor, request),
	})
	if len(state.resolutions) > maxResolutionsPerSession {
		state.resolutions = state.resolutions[len(state.resolutions)-maxResolutionsPerSession:]
	}
}

func (t *activationTracker) hasResolution(session *mcp.ServerSession, resolutionID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if resolutionID == "" {
		return false
	}
	state := t.sessionState(session)
	now := t.now()
	state.lastSeen = now
	state.pruneResolutions(now.Add(-resolutionTTL))

	for _, r := range state.resolutions {
		if r.ResolutionID == resolutionID {
			return true
		}
	}
	return false
}

func (t *activationTracker) attribute(session *mcp.ServerSession, skillID string) (string, string) {
	resID, attr, _, _, _ := t.attributeDetails(session, skillID)
	return resID, attr
}

func (t *activationTracker) attributeDetails(session *mcp.ServerSession, skillID string) (resolutionID, attribution, snapshot, policy string, client telemetry.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessionState(session)
	now := t.now()
	state.lastSeen = now
	state.pruneResolutions(now.Add(-resolutionTTL))

	if len(state.resolutions) == 0 {
		return "", "unsolicited", "", "", telemetry.Client{Name: "skillhub"}
	}

	for i := len(state.resolutions) - 1; i >= 0; i-- {
		r := state.resolutions[i]
		if r.PrimaryID == skillID {
			return r.ResolutionID, "recommended", r.CatalogSnapshot, r.PolicyRevision, r.Client
		}
		if slices.Contains(r.SupportingIDs, skillID) {
			return r.ResolutionID, "supporting", r.CatalogSnapshot, r.PolicyRevision, r.Client
		}
	}

	newest := state.resolutions[len(state.resolutions)-1]
	var attr string
	switch newest.Status {
	case string(resolverpkg.StatusResolved), string(resolverpkg.StatusAlreadyCovered):
		attr = "override"
	case string(resolverpkg.StatusNoSkill):
		attr = "after_no_skill"
	case string(resolverpkg.StatusNeedsContext):
		attr = "after_needs_context"
	default:
		attr = "override"
	}
	return newest.ResolutionID, attr, newest.CatalogSnapshot, newest.PolicyRevision, newest.Client
}

func (t *activationTracker) markActivation(session *mcp.ServerSession, resolutionID, skillID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessionState(session)
	state.lastSeen = t.now()

	resKey := resolutionID
	if resKey == "" {
		resKey = "-"
	}
	key := resKey + "\x00" + skillID
	if state.activated[key] {
		return false
	}
	state.activated[key] = true
	return true
}

func (t *activationTracker) sessionHash(session *mcp.ServerSession) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessionState(session)
	state.lastSeen = t.now()
	return state.hash
}

func redactRequest(redactor *telemetry.Redactor, req resolverpkg.Request) map[string]any {
	out := map[string]any{
		"operation": req.Operation,
		"task": map[string]any{
			"description": redactor.Redact(req.Task.Description),
			"scope":       req.Task.Scope,
		},
	}
	if req.Prior != nil {
		out["prior"] = map[string]any{
			"resolution_id":    req.Prior.ResolutionID,
			"context_revision": req.Prior.ContextRevision,
			"kind":             req.Prior.Kind,
			"question_id":      req.Prior.QuestionID,
			"answer":           redactor.Redact(req.Prior.Answer),
		}
	}
	facts := make([]map[string]any, len(req.Context.Facts))
	for i, f := range req.Context.Facts {
		facts[i] = map[string]any{"key": f.Key, "value": redactor.Redact(f.Value)}
	}
	out["context"] = map[string]any{"facts": facts}
	return out
}

func (t *activationTracker) resolutionData(session *mcp.ServerSession, resolutionID string) (notedResolution, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.sessionState(session)
	for _, res := range state.resolutions {
		if res.ResolutionID == resolutionID {
			return res, true
		}
	}
	return notedResolution{}, false
}
