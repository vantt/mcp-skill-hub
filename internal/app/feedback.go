package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

var feedbackOutcomes = map[string]bool{
	"activated": true, "loaded": true, "used": true, "abandoned": true,
	"rejected": true, "completed": true, "failed": true,
}
var feedbackUtilities = map[string]bool{"helpful": true, "harmful": true, "neutral": true}
var feedbackBases = map[string]bool{"user": true, "evaluator": true, "controlled-benchmark": true}

// FeedbackInput contains only bounded runtime telemetry fields. It deliberately
// excludes task text, paths, source content, and routing-policy mutations.
type FeedbackInput struct {
	SchemaVersion string  `json:"schema_version"`
	ResolutionID  string  `json:"resolution_id"`
	EventID       string  `json:"event_id"`
	Outcome       string  `json:"outcome"`
	ReasonCode    *string `json:"reason_code,omitempty"`
	SelectedSkill string  `json:"selected_skill,omitempty"`
	Utility       *string `json:"utility,omitempty"`
	Basis         *string `json:"basis,omitempty"`
}

type FeedbackResult struct {
	Result
	EventID       string `json:"event_id"`
	Deduplicated  bool   `json:"deduplicated"`
	PolicyMutated bool   `json:"policy_mutated"`
}

// FeedbackService records disposable telemetry through the Phase 14 recorder.
// MCP injects its process recorder; other callers receive an owned, bounded
// recorder lifecycle for the duration of the explicit command.
type FeedbackService struct {
	Recorder  *telemetry.Recorder
	Telemetry TelemetryService
}

func (service FeedbackService) Record(ctx context.Context, path string, input FeedbackInput) (result FeedbackResult, resultErr error) {
	input = normalizeFeedback(input)
	if err := validateFeedback(input); err != nil {
		return FeedbackResult{}, err
	}
	recorder := service.Recorder
	if recorder == nil {
		var err error
		recorder, err = service.Telemetry.Open(path)
		if err != nil {
			return FeedbackResult{}, err
		}
		defer closeTelemetryRecorder(recorder, &resultErr)
	}

	caller := CallerFromContext(ctx)
	feedback := telemetry.Feedback{
		EventID: input.EventID, ResolutionID: input.ResolutionID, Outcome: input.Outcome,
		SkillID: input.SelectedSkill, SessionIDHash: caller.SessionHash,
	}
	if input.ReasonCode != nil {
		feedback.ReasonCode = *input.ReasonCode
	}
	if input.Utility != nil {
		feedback.Utility, feedback.Basis = *input.Utility, *input.Basis
	}
	stored, err := recorder.RecordFeedback(ctx, feedback)
	if err != nil {
		return FeedbackResult{}, err
	}
	return feedbackResult(input.EventID, stored.Deduplicated), nil
}

func normalizeFeedback(input FeedbackInput) FeedbackInput {
	input.SchemaVersion = strings.TrimSpace(input.SchemaVersion)
	input.ResolutionID = strings.TrimSpace(input.ResolutionID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.Outcome = strings.TrimSpace(input.Outcome)
	input.SelectedSkill = strings.TrimSpace(input.SelectedSkill)
	trim := func(value *string) *string {
		if value == nil {
			return nil
		}
		normalized := strings.TrimSpace(*value)
		return &normalized
	}
	input.ReasonCode, input.Utility, input.Basis = trim(input.ReasonCode), trim(input.Utility), trim(input.Basis)
	return input
}

func validateFeedback(input FeedbackInput) error {
	if input.SchemaVersion != "1" {
		return errors.New("unsupported schema_version; use 1")
	}
	for name, value := range map[string]string{"resolution_id": input.ResolutionID, "event_id": input.EventID} {
		if value == "" || len(value) > 128 {
			return fmt.Errorf("%s must contain 1..128 characters", name)
		}
	}
	if !feedbackOutcomes[input.Outcome] {
		return errors.New("outcome must be activated, loaded, used, abandoned, rejected, completed, or failed")
	}
	if len(input.SelectedSkill) > 128 {
		return errors.New("selected_skill exceeds 128 characters")
	}
	if input.ReasonCode != nil && !telemetry.IsFeedbackReasonCode(*input.ReasonCode) {
		return errors.New("reason_code must be user_rejected, scope_mismatch, capability_unavailable, constraint_conflict, workflow_completed, workflow_failed, abandoned, host_report, or setup_failed")
	}
	if (input.Utility == nil) != (input.Basis == nil) {
		return errors.New("utility and basis must be supplied together")
	}
	if input.Utility != nil && (!feedbackUtilities[*input.Utility] || !feedbackBases[*input.Basis]) {
		return errors.New("utility or basis is not supported")
	}
	return nil
}

func feedbackResult(eventID string, duplicate bool) FeedbackResult {
	summary := "Skill feedback was recorded as local runtime telemetry."
	if duplicate {
		summary = "The existing skill feedback event was returned without a duplicate write."
	}
	return FeedbackResult{
		Result: NewResult(StatusOK, summary), EventID: eventID,
		Deduplicated: duplicate, PolicyMutated: false,
	}
}
