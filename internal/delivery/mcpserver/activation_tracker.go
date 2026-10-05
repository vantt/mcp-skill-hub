package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
)

const (
	maxSessionsPerTracker    = 256
	maxResolutionsPerSession = 32
	resolutionTTL            = 2 * time.Hour
)

type notedResolution struct {
	resolutionID  string
	status        string
	primaryID     string
	supportingIDs []string
	at            time.Time
}

type sessionState struct {
	hash        string
	lastSeen    time.Time
	resolutions []notedResolution
	activated   map[string]bool
}

func (s *sessionState) pruneResolutions(cutoff time.Time) {
	validStart := 0
	for validStart < len(s.resolutions) && s.resolutions[validStart].at.Before(cutoff) {
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
}

func newActivationTracker(now func() time.Time) *activationTracker {
	if now == nil {
		now = time.Now
	}
	return &activationTracker{
		now:      now,
		sessions: make(map[*mcp.ServerSession]*sessionState),
		anon:     newSessionState(now()),
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

func (t *activationTracker) noteResolution(session *mcp.ServerSession, response resolverpkg.Response) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessionState(session)
	now := t.now()
	state.lastSeen = now
	state.pruneResolutions(now.Add(-resolutionTTL))

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
		resolutionID:  response.ResolutionID,
		status:        string(response.Status),
		primaryID:     primaryID,
		supportingIDs: supportingIDs,
		at:            now,
	})
	if len(state.resolutions) > maxResolutionsPerSession {
		state.resolutions = state.resolutions[len(state.resolutions)-maxResolutionsPerSession:]
	}
}

func (t *activationTracker) attribute(session *mcp.ServerSession, skillID string) (string, string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessionState(session)
	now := t.now()
	state.lastSeen = now
	state.pruneResolutions(now.Add(-resolutionTTL))

	if len(state.resolutions) == 0 {
		return "", "unsolicited"
	}

	for i := len(state.resolutions) - 1; i >= 0; i-- {
		r := state.resolutions[i]
		if r.primaryID == skillID {
			return r.resolutionID, "recommended"
		}
		if slices.Contains(r.supportingIDs, skillID) {
			return r.resolutionID, "supporting"
		}
	}

	newest := state.resolutions[len(state.resolutions)-1]
	switch newest.status {
	case string(resolverpkg.StatusResolved), string(resolverpkg.StatusAlreadyCovered):
		return newest.resolutionID, "override"
	case string(resolverpkg.StatusNoSkill):
		return newest.resolutionID, "after_no_skill"
	case string(resolverpkg.StatusNeedsContext):
		return newest.resolutionID, "after_needs_context"
	default:
		return newest.resolutionID, "override"
	}
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
