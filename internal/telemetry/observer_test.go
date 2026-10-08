package telemetry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestObserverTelemetryFieldsAllowlist(t *testing.T) {
	now := time.Now().UTC()
	event := Event{
		Version:         EventVersion,
		ID:              "evt_observer_test",
		Type:            EventResolutionCompleted,
		OccurredAt:      now,
		SessionIDHash:   "sess_1234567890abcdef",
		ResolutionID:    "res_test_resolution",
		CatalogSnapshot: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		PolicyRevision:  "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Client:          Client{Name: "claude-code", Version: "1.2"},
		Payload: map[string]any{
			"status":                    "resolved",
			"candidate_count":           2,
			"retrieval_candidate_count": 15,
			"prior_resolution_id":       "res_prior_123",
			"prior_kind":                "rejected",
			"prior_verified":            true,
			"top_skill_id":              "code-review",
			"recommended_skill_ids":     []string{"code-review"},
			"channels":                  []string{"fts", "rules"},
			"stage_ms": map[string]int64{
				"validation": 1,
				"retrieval":  8,
				"scoring":    2,
				"total":      11,
			},
		},
	}

	envelope, err := validateAndBuild(event, ContentModeNone, now, event.ID)
	if err != nil {
		t.Fatalf("validateAndBuild failed with observer fields: %v", err)
	}

	if envelope.Payload["retrieval_candidate_count"] != 15 {
		t.Fatalf("retrieval_candidate_count = %v, want 15", envelope.Payload["retrieval_candidate_count"])
	}
	if envelope.Payload["prior_verified"] != true {
		t.Fatalf("prior_verified = %v, want true", envelope.Payload["prior_verified"])
	}
}

func TestBackwardCompatibilityExistingEventsPassValidateStored(t *testing.T) {
	now := time.Now().UTC()
	// Simulating an event stored before this change (no retrieval_candidate_count, no prior_* fields)
	oldEvent := Event{
		Version:         EventVersion,
		ID:              "evt_old_resolution",
		Type:            EventResolutionCompleted,
		OccurredAt:      now,
		ResolutionID:    "res_old_1",
		CatalogSnapshot: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		PolicyRevision:  "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Client:          Client{Name: "skillhub"},
		Payload: map[string]any{
			"status":                "resolved",
			"candidate_count":       1,
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
		},
	}

	envelope, err := validateAndBuild(oldEvent, ContentModeNone, now, oldEvent.ID)
	if err != nil {
		t.Fatalf("validateAndBuild failed for old event: %v", err)
	}

	// validateStored must accept old envelope
	if err := validateStored(envelope, envelope.ID, envelope.OccurredAt, envelope.Type); err != nil {
		t.Fatalf("validateStored failed on old event: %v", err)
	}
}

func TestRecorderRawEvents(t *testing.T) {
	t.Parallel()
	recorder := newTestRecorder(t, Config{})

	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	evt1 := Event{
		ID:              "evt_raw_1",
		Type:            EventResolutionCompleted,
		OccurredAt:      now,
		SessionIDHash:   "sess_aaa",
		ResolutionID:    "res_raw_1",
		CatalogSnapshot: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		PolicyRevision:  "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Client:          Client{Name: "claude-code"},
		Payload: map[string]any{
			"status":                "resolved",
			"candidate_count":       1,
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
		},
	}
	evt2 := Event{
		ID:              "evt_raw_2",
		Type:            EventSkillLoaded,
		OccurredAt:      now.Add(5 * time.Minute),
		SessionIDHash:   "sess_aaa",
		ResolutionID:    "res_raw_1",
		CatalogSnapshot: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		PolicyRevision:  "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		Client:          Client{Name: "claude-code"},
		Payload: map[string]any{
			"skill_id":      "code-review",
			"resource_kind": "entrypoint",
			"surface":       "skill_get",
			"basis":         LoadBasisServerObserved,
			"attribution":   "recommended",
		},
	}

	recorder.Record(evt1)
	recorder.Record(evt2)
	recorder.Flush(t.Context())

	events, err := recorder.RawEvents(t.Context(), "2026-10-09", "2026-10-09")
	if err != nil {
		t.Fatalf("RawEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].ID != "evt_raw_1" || events[1].ID != "evt_raw_2" {
		t.Fatalf("events out of order: %+v", events)
	}
	if events[0].SessionIDHash != "sess_aaa" {
		t.Fatalf("session_id_hash = %q, want sess_aaa", events[0].SessionIDHash)
	}
}

func init() {
	_ = json.Marshal
}
