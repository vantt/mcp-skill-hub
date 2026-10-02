package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestFeedbackUsesInjectedRecorderAndPreservesSemanticIdempotency(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	recorder, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())
	seedFeedbackResolution(recorder, "res_test")

	service := FeedbackService{Recorder: recorder}
	input := FeedbackInput{SchemaVersion: "1", ResolutionID: "res_test", EventID: "evt_test", Outcome: "used", SelectedSkill: "consumer-review"}
	const callers = 8
	var wait sync.WaitGroup
	results := make(chan FeedbackResult, callers)
	errorsFound := make(chan error, callers)
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.Record(context.Background(), root, input)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent feedback: %v", err)
	}
	fresh, duplicates := 0, 0
	for result := range results {
		if result.PolicyMutated {
			t.Fatal("feedback reported a routing-policy mutation")
		}
		if result.Deduplicated {
			duplicates++
		} else {
			fresh++
		}
	}
	if fresh != 1 || duplicates != callers-1 {
		t.Fatalf("fresh=%d duplicates=%d", fresh, duplicates)
	}
	conflict := input
	conflict.Outcome = "failed"
	if _, err := service.Record(t.Context(), root, conflict); !errors.Is(err, telemetry.ErrFeedbackConflict) {
		t.Fatalf("event_id conflict error = %v", err)
	}
}

func TestFeedbackOwnsRecorderWhenNotInjected(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	recorder, err := telemetry.Open(telemetry.Config{WorkspaceRoot: root, Path: filepath.Join(root, "runtime", "telemetry.db")})
	if err != nil {
		t.Fatal(err)
	}
	seedFeedbackResolution(recorder, "res_owned")
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}

	result, err := (FeedbackService{}).Record(t.Context(), root, FeedbackInput{
		SchemaVersion: "1", ResolutionID: "res_owned", EventID: "evt_owned", Outcome: "completed",
	})
	if err != nil || result.Deduplicated || result.PolicyMutated {
		t.Fatalf("owned feedback = %#v, %v", result, err)
	}

	reader, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close(context.Background())
	preview, err := reader.Preview(t.Context(), 0)
	if err != nil || preview.Events != 2 {
		t.Fatalf("preview = %#v, %v", preview, err)
	}
}

func TestFeedbackRecordsLoadedAsDistinctFunnelEvent(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	recorder, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())
	seedFeedbackResolution(recorder, "res_loaded")
	result, err := (FeedbackService{Recorder: recorder}).Record(t.Context(), root, FeedbackInput{
		SchemaVersion: "1", ResolutionID: "res_loaded", EventID: "evt_loaded", Outcome: "loaded", SelectedSkill: "consumer-review",
	})
	if err != nil || result.Deduplicated {
		t.Fatalf("loaded feedback = %#v, %v", result, err)
	}
	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil || !strings.Contains(string(preview.JSONL), `"event_type":"skill.loaded"`) {
		t.Fatalf("loaded funnel preview = %q, %v", preview.JSONL, err)
	}
}

func TestFeedbackAcceptsSupportingRecommendationAndRejectsUnretainedIdentifiers(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	recorder, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())
	seedFeedbackResolutionWithRecommendations(recorder, "res_recommendations", []string{"consumer-review", "supporting-review"})
	service := FeedbackService{Recorder: recorder}
	reason := telemetry.FeedbackReasonHostReport
	if _, err := service.Record(t.Context(), root, FeedbackInput{
		SchemaVersion: "1", ResolutionID: "res_recommendations", EventID: "evt_supporting",
		Outcome: "used", SelectedSkill: "supporting-review", ReasonCode: &reason,
	}); err != nil {
		t.Fatalf("supporting recommendation rejected: %v", err)
	}
	for _, input := range []FeedbackInput{
		{SchemaVersion: "1", ResolutionID: "res_recommendations", EventID: "evt_arbitrary", Outcome: "used", SelectedSkill: "sk_live_token_shaped_secret"},
		{SchemaVersion: "1", ResolutionID: "res_recommendations", EventID: "evt_secret_reason", Outcome: "failed", ReasonCode: stringPointer("ghp_token_shaped_secret")},
	} {
		if _, err := service.Record(t.Context(), root, input); err == nil {
			t.Fatalf("unretained external value accepted: %#v", input)
		}
	}
}

func TestFeedbackRequiresEmptySelectionWhenResolutionRecommendedNoSkills(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	recorder, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())
	seedFeedbackResolutionWithRecommendations(recorder, "res_none", []string{})
	service := FeedbackService{Recorder: recorder}
	if _, err := service.Record(t.Context(), root, FeedbackInput{
		SchemaVersion: "1", ResolutionID: "res_none", EventID: "evt_none", Outcome: "completed",
	}); err != nil {
		t.Fatalf("empty selection rejected: %v", err)
	}
	if _, err := service.Record(t.Context(), root, FeedbackInput{
		SchemaVersion: "1", ResolutionID: "res_none", EventID: "evt_none_secret", Outcome: "used", SelectedSkill: "sk_live_token_shaped_secret",
	}); !errors.Is(err, telemetry.ErrFeedbackSkillNotRecommended) {
		t.Fatalf("selection for no-skill resolution error = %v", err)
	}
}

func TestFeedbackRejectsUnknownResolutionUnsupportedOutcomeAndUnbasedUtility(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	utility := "helpful"
	for _, input := range []FeedbackInput{
		{SchemaVersion: "1", ResolutionID: "res", EventID: "evt-1", Outcome: "helpful"},
		{SchemaVersion: "1", ResolutionID: "res", EventID: "evt-2", Outcome: "completed", Utility: &utility},
	} {
		if _, err := (FeedbackService{}).Record(t.Context(), root, input); err == nil {
			t.Fatalf("invalid feedback accepted: %#v", input)
		}
	}
	if _, err := (FeedbackService{}).Record(t.Context(), root, FeedbackInput{
		SchemaVersion: "1", ResolutionID: "res_missing", EventID: "evt_missing", Outcome: "used",
	}); !errors.Is(err, telemetry.ErrFeedbackResolutionNotFound) {
		t.Fatalf("unknown resolution error = %v", err)
	}
}

func seedFeedbackResolution(recorder *telemetry.Recorder, resolutionID string) {
	seedFeedbackResolutionWithRecommendations(recorder, resolutionID, []string{"consumer-review"})
}

func seedFeedbackResolutionWithRecommendations(recorder *telemetry.Recorder, resolutionID string, recommended []string) {
	retained := make([]string, len(recommended))
	copy(retained, recommended)
	payload := map[string]any{"status": "resolved", "recommended_skill_ids": retained}
	if len(recommended) > 0 {
		payload["top_skill_id"] = recommended[0]
	}
	recorder.Record(telemetry.Event{
		ID: "resolver_" + resolutionID, Type: telemetry.EventResolutionCompleted, ResolutionID: resolutionID,
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: telemetry.Client{Name: "skillhub"},
		Payload: payload,
	})
}

func stringPointer(value string) *string { return &value }
