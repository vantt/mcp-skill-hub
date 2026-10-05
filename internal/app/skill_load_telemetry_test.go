package app

import (
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestRecordSkillLoad(t *testing.T) {
	t.Parallel()

	root := newDistributionFixtureWorkspace(t)
	writeDistributionFixtureSkill(t, root, "test-skill", "test trigger", servableEntrypoint("test-skill"), nil)
	rebuildFixtureCatalog(t, root)

	taskDescription := "find a skill to review code and push to git"
	allowedKeys := map[string]bool{
		"skill_id":         true,
		"resource_kind":    true,
		"surface":          true,
		"basis":            true,
		"attribution":      true,
		"first_activation": true,
		"status":           true,
		"reason_codes":     true,
		"duration_ms":      true,
		"error_code":       true,
		"after_load":       true,
	}

	checkValues := func(t *testing.T, payload map[string]any) {
		t.Helper()
		for k, v := range payload {
			if !allowedKeys[k] {
				t.Fatalf("unexpected payload key %q", k)
			}
			switch val := v.(type) {
			case string:
				if strings.Contains(val, "/") {
					t.Fatalf("payload value for %q contains '/': %q", k, val)
				}
				if strings.Contains(val, "skill://") {
					t.Fatalf("payload value for %q contains 'skill://': %q", k, val)
				}
				if strings.Contains(val, taskDescription) {
					t.Fatalf("payload value for %q contains task description: %q", k, val)
				}
			case []string:
				for _, item := range val {
					if strings.Contains(item, "/") || strings.Contains(item, "skill://") || strings.Contains(item, taskDescription) {
						t.Fatalf("payload list item for %q contains forbidden content: %q", k, item)
					}
				}
			case []any:
				for _, item := range val {
					s, _ := item.(string)
					if strings.Contains(s, "/") || strings.Contains(s, "skill://") || strings.Contains(s, taskDescription) {
						t.Fatalf("payload list item for %q contains forbidden content: %q", k, s)
					}
				}
			}
		}
	}

	t.Run("normal load", func(t *testing.T) {
		sink := &captureTelemetrySink{}
		RecordSkillLoad(t.Context(), sink, root, SkillLoad{
			SkillID:         "test-skill",
			ResourceKind:    "entrypoint",
			Surface:         "skill_get",
			Attribution:     "recommended",
			ResolutionID:    "res-123",
			SessionIDHash:   "0123456789abcdef0123456789abcdef",
			FirstActivation: true,
		})

		if len(sink.events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(sink.events))
		}
		event := sink.events[0]
		if event.Type != telemetry.EventSkillLoaded {
			t.Fatalf("expected event type %q, got %q", telemetry.EventSkillLoaded, event.Type)
		}
		if event.ResolutionID != "res-123" {
			t.Fatalf("expected ResolutionID 'res-123', got %q", event.ResolutionID)
		}
		if event.SessionIDHash != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("expected SessionIDHash, got %q", event.SessionIDHash)
		}
		if event.CatalogSnapshot == "" || event.PolicyRevision == "" {
			t.Fatalf("expected CatalogSnapshot and PolicyRevision to be populated, got %q, %q", event.CatalogSnapshot, event.PolicyRevision)
		}
		checkValues(t, event.Payload)
		if first, _ := event.Payload["first_activation"].(bool); !first {
			t.Fatal("expected first_activation: true")
		}
	})

	t.Run("blocked load", func(t *testing.T) {
		sink := &captureTelemetrySink{}
		RecordSkillLoad(t.Context(), sink, root, SkillLoad{
			SkillID:         "test-skill",
			ResourceKind:    "entrypoint",
			Surface:         "skill_get",
			Attribution:     "recommended",
			ResolutionID:    "res-456",
			SessionIDHash:   "0123456789abcdef0123456789abcdef",
			FirstActivation: false,
			Blocked:         true,
			ReasonCodes:     []string{"content_review_required"},
		})

		if len(sink.events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(sink.events))
		}
		event := sink.events[0]
		checkValues(t, event.Payload)
		if status, _ := event.Payload["status"].(string); status != "review_required" {
			t.Fatalf("expected status 'review_required', got %q", status)
		}
		if first, _ := event.Payload["first_activation"].(bool); first {
			t.Fatal("expected first_activation: false on blocked load")
		}
		var codes []string
		switch raw := event.Payload["reason_codes"].(type) {
		case []string:
			codes = raw
		case []any:
			for _, item := range raw {
				if s, ok := item.(string); ok {
					codes = append(codes, s)
				}
			}
		}
		if len(codes) != 1 || codes[0] != "content_review_required" {
			t.Fatalf("expected reason_codes ['content_review_required'], got %v", event.Payload["reason_codes"])
		}
	})

	t.Run("nil sink or empty skill ID", func(t *testing.T) {
		sink := &captureTelemetrySink{}
		RecordSkillLoad(t.Context(), nil, root, SkillLoad{SkillID: "test-skill"})
		RecordSkillLoad(t.Context(), sink, root, SkillLoad{SkillID: ""})
		if len(sink.events) != 0 {
			t.Fatalf("expected 0 events, got %d", len(sink.events))
		}
	})
}
