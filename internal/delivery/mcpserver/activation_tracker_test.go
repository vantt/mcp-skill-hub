package mcpserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
)

func TestActivationTrackerAttributionClasses(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	tracker := newActivationTracker(func() time.Time { return now })
	session := &mcp.ServerSession{}

	// Unsolicited: no resolutions noted yet
	resID, attr := tracker.attribute(session, "skill-a")
	if resID != "" || attr != "unsolicited" {
		t.Fatalf("expected empty resolution ID and unsolicited, got %q, %q", resID, attr)
	}

	// 1. Recommended
	tracker.noteResolution(session, resolverpkg.Response{
		ResolutionID: "res-1",
		Status:       resolverpkg.StatusResolved,
		Primary:      &resolverpkg.Recommendation{ID: "skill-a"},
		Supporting:   []resolverpkg.Supporting{{ID: "skill-b"}},
	})
	resID, attr = tracker.attribute(session, "skill-a")
	if resID != "res-1" || attr != "recommended" {
		t.Fatalf("expected res-1 and recommended, got %q, %q", resID, attr)
	}

	// 2. Supporting
	resID, attr = tracker.attribute(session, "skill-b")
	if resID != "res-1" || attr != "supporting" {
		t.Fatalf("expected res-1 and supporting, got %q, %q", resID, attr)
	}

	// 3. Override (status resolved)
	resID, attr = tracker.attribute(session, "skill-c")
	if resID != "res-1" || attr != "override" {
		t.Fatalf("expected res-1 and override, got %q, %q", resID, attr)
	}

	// Override with status already_covered
	tracker.noteResolution(session, resolverpkg.Response{
		ResolutionID: "res-2",
		Status:       resolverpkg.StatusAlreadyCovered,
		Primary:      &resolverpkg.Recommendation{ID: "skill-d"},
	})
	resID, attr = tracker.attribute(session, "skill-c")
	if resID != "res-2" || attr != "override" {
		t.Fatalf("expected res-2 and override, got %q, %q", resID, attr)
	}

	// 4. After no skill
	tracker.noteResolution(session, resolverpkg.Response{
		ResolutionID: "res-3",
		Status:       resolverpkg.StatusNoSkill,
	})
	resID, attr = tracker.attribute(session, "skill-c")
	if resID != "res-3" || attr != "after_no_skill" {
		t.Fatalf("expected res-3 and after_no_skill, got %q, %q", resID, attr)
	}

	// 5. After needs context
	tracker.noteResolution(session, resolverpkg.Response{
		ResolutionID: "res-4",
		Status:       resolverpkg.StatusNeedsContext,
	})
	resID, attr = tracker.attribute(session, "skill-c")
	if resID != "res-4" || attr != "after_needs_context" {
		t.Fatalf("expected res-4 and after_needs_context, got %q, %q", resID, attr)
	}

	// But skill-a from earlier res-1 within TTL is still recommended
	resID, attr = tracker.attribute(session, "skill-a")
	if resID != "res-1" || attr != "recommended" {
		t.Fatalf("expected res-1 and recommended for skill-a, got %q, %q", resID, attr)
	}
}

func TestActivationTrackerTTLExpiry(t *testing.T) {
	t.Parallel()

	current := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	tracker := newActivationTracker(func() time.Time { return current })
	session := &mcp.ServerSession{}

	tracker.noteResolution(session, resolverpkg.Response{
		ResolutionID: "res-1",
		Status:       resolverpkg.StatusResolved,
		Primary:      &resolverpkg.Recommendation{ID: "skill-a"},
	})

	resID, attr := tracker.attribute(session, "skill-a")
	if resID != "res-1" || attr != "recommended" {
		t.Fatalf("expected res-1, got %q, %q", resID, attr)
	}

	// Advance time past 2 hours
	current = current.Add(2*time.Hour + 1*time.Minute)

	resID, attr = tracker.attribute(session, "skill-a")
	if resID != "" || attr != "unsolicited" {
		t.Fatalf("expected expired resolution to return unsolicited, got %q, %q", resID, attr)
	}
}

func TestActivationTrackerLRUEviction(t *testing.T) {
	t.Parallel()

	current := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	tracker := newActivationTracker(func() time.Time { return current })

	sessions := make([]*mcp.ServerSession, 257)
	for i := range sessions {
		sessions[i] = &mcp.ServerSession{}
	}

	// Add 256 sessions
	for i := range 256 {
		current = current.Add(time.Second)
		tracker.noteResolution(sessions[i], resolverpkg.Response{
			ResolutionID: fmt.Sprintf("res-%d", i),
			Status:       resolverpkg.StatusResolved,
			Primary:      &resolverpkg.Recommendation{ID: fmt.Sprintf("skill-%d", i)},
		})
	}

	// Touch sessions[0] so it's recently used
	current = current.Add(time.Second)
	tracker.attribute(sessions[0], "skill-0")

	// Now sessions[1] is the oldest lastSeen.
	// Add 257th session
	current = current.Add(time.Second)
	tracker.noteResolution(sessions[256], resolverpkg.Response{
		ResolutionID: "res-256",
		Status:       resolverpkg.StatusResolved,
		Primary:      &resolverpkg.Recommendation{ID: "skill-256"},
	})

	// sessions[1] should have been evicted and its resolutions gone
	resID, attr := tracker.attribute(sessions[1], "skill-1")
	if resID != "" || attr != "unsolicited" {
		t.Fatalf("expected session-1 to be evicted, got %q, %q", resID, attr)
	}

	// sessions[0] should still be present
	resID, attr = tracker.attribute(sessions[0], "skill-0")
	if resID != "res-0" || attr != "recommended" {
		t.Fatalf("expected session-0 to remain, got %q, %q", resID, attr)
	}
}

func TestActivationTracker32ResolutionCap(t *testing.T) {
	t.Parallel()

	tracker := newActivationTracker(nil)
	session := &mcp.ServerSession{}

	for i := range 35 {
		tracker.noteResolution(session, resolverpkg.Response{
			ResolutionID: fmt.Sprintf("res-%d", i),
			Status:       resolverpkg.StatusResolved,
			Primary:      &resolverpkg.Recommendation{ID: fmt.Sprintf("skill-%d", i)},
		})
	}

	// Oldest 3 resolutions (0, 1, 2) should be capped out
	resID, attr := tracker.attribute(session, "skill-0")
	if attr != "override" { // skill-0 is not in primary/supporting anymore, newest (res-34) decides
		t.Fatalf("expected capped out resolution to not match primary, got %q, %q", resID, attr)
	}
	if resID != "res-34" {
		t.Fatalf("expected newest resolution res-34, got %q", resID)
	}

	// skill-3 (resolution 3) should still be in the 32 kept
	resID, attr = tracker.attribute(session, "skill-3")
	if resID != "res-3" || attr != "recommended" {
		t.Fatalf("expected res-3 and recommended, got %q, %q", resID, attr)
	}
}

func TestActivationTrackerMarkActivationDedupe(t *testing.T) {
	t.Parallel()

	tracker := newActivationTracker(nil)
	session1 := &mcp.ServerSession{}
	session2 := &mcp.ServerSession{}

	// First activation per (session, resolution, skill)
	if !tracker.markActivation(session1, "res-1", "skill-a") {
		t.Fatal("expected first activation to be true")
	}
	// Duplicate in same session/resolution/skill
	if tracker.markActivation(session1, "res-1", "skill-a") {
		t.Fatal("expected duplicate activation to be false")
	}

	// Empty resolution ID ("-")
	if !tracker.markActivation(session1, "", "skill-a") {
		t.Fatal("expected first activation with empty resID to be true")
	}
	if tracker.markActivation(session1, "", "skill-a") {
		t.Fatal("expected duplicate activation with empty resID to be false")
	}

	// Different skill
	if !tracker.markActivation(session1, "res-1", "skill-b") {
		t.Fatal("expected different skill to be true")
	}

	// Different resolution
	if !tracker.markActivation(session1, "res-2", "skill-a") {
		t.Fatal("expected different resolution to be true")
	}

	// Different session
	if !tracker.markActivation(session2, "res-1", "skill-a") {
		t.Fatal("expected different session to be true")
	}
}

func TestActivationTrackerNilSessionHandling(t *testing.T) {
	t.Parallel()

	tracker := newActivationTracker(nil)

	tracker.noteResolution(nil, resolverpkg.Response{
		ResolutionID: "res-nil",
		Status:       resolverpkg.StatusResolved,
		Primary:      &resolverpkg.Recommendation{ID: "skill-nil"},
	})

	resID, attr := tracker.attribute(nil, "skill-nil")
	if resID != "res-nil" || attr != "recommended" {
		t.Fatalf("expected res-nil and recommended on nil session, got %q, %q", resID, attr)
	}

	if !tracker.markActivation(nil, "res-nil", "skill-nil") {
		t.Fatal("expected first activation on nil session to be true")
	}
	if tracker.markActivation(nil, "res-nil", "skill-nil") {
		t.Fatal("expected duplicate activation on nil session to be false")
	}

	hash := tracker.sessionHash(nil)
	if len(hash) != 32 {
		t.Fatalf("expected 32-hex session hash, got %q", hash)
	}
}
