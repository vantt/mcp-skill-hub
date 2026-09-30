package telemetry

import (
	"errors"
	"testing"
)

func TestRecordFeedbackAcceptsPrimaryAndSupportingRecommendationsOnly(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	recorder.Record(feedbackResolutionEvent("res_allowlist", []string{"primary-skill", "supporting-skill"}))

	for _, skillID := range []string{"primary-skill", "supporting-skill"} {
		_, err := recorder.RecordFeedback(t.Context(), Feedback{
			EventID: "evt_" + skillID, ResolutionID: "res_allowlist", Outcome: "used",
			SkillID: skillID, ReasonCode: FeedbackReasonHostReport,
		})
		if err != nil {
			t.Fatalf("recommended skill %q rejected: %v", skillID, err)
		}
	}

	_, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID: "evt_secret_skill", ResolutionID: "res_allowlist", Outcome: "used",
		SkillID: "sk_live_token_shaped_secret", ReasonCode: FeedbackReasonHostReport,
	})
	if !errors.Is(err, ErrFeedbackSkillNotRecommended) {
		t.Fatalf("unrecommended token-shaped skill error = %v", err)
	}
}

func TestRecordFeedbackRequiresEmptySkillWhenResolutionHasNoRecommendations(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	recorder.Record(feedbackResolutionEvent("res_no_skill", []string{}))

	if _, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID: "evt_no_selection", ResolutionID: "res_no_skill", Outcome: "completed",
		ReasonCode: FeedbackReasonWorkflowCompleted,
	}); err != nil {
		t.Fatalf("empty selection rejected: %v", err)
	}
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID: "evt_forced_selection", ResolutionID: "res_no_skill", Outcome: "failed",
		SkillID: "ghp_token_shaped_secret", ReasonCode: FeedbackReasonWorkflowFailed,
	}); !errors.Is(err, ErrFeedbackSkillNotRecommended) {
		t.Fatalf("selection for no-skill resolution error = %v", err)
	}
}

func TestFeedbackReasonCodesAreFiniteAndContentFree(t *testing.T) {
	for _, reason := range []string{
		FeedbackReasonUserRejected, FeedbackReasonScopeMismatch,
		FeedbackReasonCapabilityUnavailable, FeedbackReasonConstraintConflict,
		FeedbackReasonWorkflowCompleted, FeedbackReasonWorkflowFailed,
		FeedbackReasonAbandoned, FeedbackReasonHostReport,
	} {
		if err := validateFeedback(Feedback{EventID: "evt_reason", ResolutionID: "res_reason", Outcome: "failed", ReasonCode: reason}); err != nil {
			t.Fatalf("documented reason %q rejected: %v", reason, err)
		}
	}
	for _, reason := range []string{"unknown_reason", "sk_live_token_shaped_secret", "ghp_token_shaped_secret"} {
		if err := validateFeedback(Feedback{EventID: "evt_reason", ResolutionID: "res_reason", Outcome: "failed", ReasonCode: reason}); err == nil {
			t.Fatalf("unknown token-shaped reason %q accepted", reason)
		}
	}
}

func feedbackResolutionEvent(resolutionID string, recommended []string) Event {
	retained := make([]string, len(recommended))
	copy(retained, recommended)
	payload := map[string]any{
		"status":                "resolved",
		"recommended_skill_ids": retained,
	}
	if len(recommended) > 0 {
		payload["top_skill_id"] = recommended[0]
		payload["skill_id"] = recommended[0]
	}
	return Event{
		ID: "evt_" + resolutionID, Type: EventResolutionCompleted, ResolutionID: resolutionID,
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy",
		Client: Client{Name: "skillhub"}, Payload: payload,
	}
}
