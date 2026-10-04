package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrFeedbackResolutionNotFound  = errors.New("feedback resolution not found")
	ErrFeedbackConflict            = errors.New("event_id was already used for different feedback")
	ErrFeedbackSkillNotRecommended = errors.New("selected_skill must be one of the resolution's recommended_skill_ids")
)

// Feedback reason codes are the complete content-free vocabulary accepted from
// hosts. They describe protocol outcomes without retaining user-authored text.
const (
	FeedbackReasonUserRejected          = "user_rejected"
	FeedbackReasonScopeMismatch         = "scope_mismatch"
	FeedbackReasonCapabilityUnavailable = "capability_unavailable"
	FeedbackReasonConstraintConflict    = "constraint_conflict"
	FeedbackReasonWorkflowCompleted     = "workflow_completed"
	FeedbackReasonWorkflowFailed        = "workflow_failed"
	FeedbackReasonAbandoned             = "abandoned"
	FeedbackReasonHostReport            = "host_report"
	// FeedbackReasonSetupFailed reports that a skill's script failed because a
	// dependency it needs was missing on the host.
	FeedbackReasonSetupFailed = "setup_failed"
)

var feedbackEventTypes = map[string]string{
	"activated": EventSkillActivated,
	"loaded":    EventSkillLoaded,
	"rejected":  EventActivationRejected,
	"used":      EventSkillUsed,
	"abandoned": EventSkillAbandoned,
	"completed": EventTaskOutcomeReported,
	"failed":    EventTaskOutcomeReported,
}

var feedbackUtilities = map[string]bool{"helpful": true, "harmful": true, "neutral": true}
var feedbackBases = map[string]bool{"user": true, "evaluator": true, "controlled-benchmark": true}
var feedbackReasonCodes = map[string]bool{
	FeedbackReasonUserRejected: true, FeedbackReasonScopeMismatch: true,
	FeedbackReasonCapabilityUnavailable: true, FeedbackReasonConstraintConflict: true,
	FeedbackReasonWorkflowCompleted: true, FeedbackReasonWorkflowFailed: true,
	FeedbackReasonAbandoned: true, FeedbackReasonHostReport: true, FeedbackReasonSetupFailed: true,
}

// IsFeedbackReasonCode reports whether value is in the public, content-free
// feedback reason vocabulary.
func IsFeedbackReasonCode(value string) bool { return feedbackReasonCodes[value] }

// Feedback is a bounded, content-free report about one retained resolution.
// Catalog, policy, and client identity are intentionally not accepted here;
// RecordFeedback derives them from the validated resolver envelope.
type Feedback struct {
	EventID      string
	ResolutionID string
	Outcome      string
	ReasonCode   string
	SkillID      string
	Utility      string
	Basis        string
}

// FeedbackResult reports whether an identical semantic report already existed.
type FeedbackResult struct {
	Deduplicated bool
	Events       int
}

func validateFeedback(feedback Feedback) error {
	for name, value := range map[string]string{
		"event_id": feedback.EventID, "resolution_id": feedback.ResolutionID,
	} {
		if !validToken(value) {
			return fmt.Errorf("%s must be a bounded opaque token", name)
		}
	}
	if _, ok := feedbackEventTypes[feedback.Outcome]; !ok {
		return errors.New("outcome must be activated, loaded, used, abandoned, rejected, completed, or failed")
	}
	if feedback.ReasonCode != "" && !feedbackReasonCodes[feedback.ReasonCode] {
		return errors.New("reason_code is not supported")
	}
	if feedback.SkillID != "" && !validToken(feedback.SkillID) {
		return errors.New("skill_id must be a bounded opaque token")
	}
	if (feedback.Utility == "") != (feedback.Basis == "") {
		return errors.New("utility and basis must be supplied together")
	}
	if feedback.Utility != "" && (!feedbackUtilities[feedback.Utility] || !feedbackBases[feedback.Basis]) {
		return errors.New("utility or basis is not supported")
	}
	if feedback.Utility != "" && !validToken(feedback.EventID+":utility") {
		return errors.New("event_id is too long to identify utility feedback")
	}
	return nil
}

func recordFeedbackStore(ctx context.Context, config Config, feedback Feedback) (FeedbackResult, error) {
	if err := validateFeedback(feedback); err != nil {
		return FeedbackResult{}, err
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return FeedbackResult{}, err
	}
	defer database.Close()

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return FeedbackResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()

	cutoff := formatStoredTime(config.Clock().Add(-config.Retention))
	if _, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_events WHERE occurred_at < ?`, cutoff); err != nil {
		return FeedbackResult{}, err
	}
	source, recommendedSkillIDs, err := feedbackResolutionSource(ctx, transaction, feedback.ResolutionID)
	if err != nil {
		return FeedbackResult{}, err
	}
	if feedback.SkillID != "" && !containsString(recommendedSkillIDs, feedback.SkillID) {
		return FeedbackResult{}, ErrFeedbackSkillNotRecommended
	}
	afterLoad, err := resolutionHasServerObservedLoad(ctx, transaction, feedback.ResolutionID)
	if err != nil {
		return FeedbackResult{}, err
	}
	events, err := buildFeedbackEvents(config, feedback, source, afterLoad)
	if err != nil {
		return FeedbackResult{}, err
	}

	deduplicated, err := storeFeedbackEvents(ctx, transaction, events)
	if err != nil {
		return FeedbackResult{}, err
	}
	if err := trimLogicalSize(ctx, transaction, config.MaxSizeBytes); err != nil {
		return FeedbackResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return FeedbackResult{}, err
	}
	committed = true
	if err := trimPhysicalSize(ctx, database, config.Path, config.MaxSizeBytes); err != nil {
		return FeedbackResult{}, err
	}
	return FeedbackResult{Deduplicated: deduplicated, Events: len(events)}, nil
}

func feedbackResolutionSource(ctx context.Context, transaction *sql.Tx, resolutionID string) (storedEnvelope, []string, error) {
	rows, err := transaction.QueryContext(ctx, `
SELECT id,occurred_at,kind,payload_json
FROM telemetry_events
WHERE resolution_id = ?
  AND kind IN ('resolution.recommended','resolution.completed','resolution.failed')
ORDER BY occurred_at,id`, resolutionID)
	if err != nil {
		return storedEnvelope{}, nil, err
	}
	defer rows.Close()

	var source storedEnvelope
	var recommendedSkillIDs []string
	found := false
	for rows.Next() {
		var id, occurredAt, kind, raw string
		if err := rows.Scan(&id, &occurredAt, &kind, &raw); err != nil {
			return storedEnvelope{}, nil, err
		}
		var candidate storedEnvelope
		if json.Unmarshal([]byte(raw), &candidate) != nil || validateStored(candidate, id, occurredAt, kind) != nil || candidate.ResolutionID != resolutionID {
			continue
		}
		candidateRecommended, ok := stringTokens(candidate.Payload["recommended_skill_ids"])
		if !ok {
			continue
		}
		if !found {
			source, recommendedSkillIDs, found = candidate, candidateRecommended, true
			continue
		}
		if source.CatalogSnapshot != candidate.CatalogSnapshot || source.PolicyRevision != candidate.PolicyRevision || source.Client != candidate.Client || !sameStrings(recommendedSkillIDs, candidateRecommended) {
			return storedEnvelope{}, nil, errors.New("validated resolver envelopes disagree on pinned identity or recommendations")
		}
	}
	if err := rows.Err(); err != nil {
		return storedEnvelope{}, nil, err
	}
	if !found {
		return storedEnvelope{}, nil, ErrFeedbackResolutionNotFound
	}
	return source, recommendedSkillIDs, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// resolutionHasServerObservedLoad reports whether the hub itself observed a
// skill load for the resolution before this feedback arrived.
func resolutionHasServerObservedLoad(ctx context.Context, transaction *sql.Tx, resolutionID string) (bool, error) {
	var found int
	err := transaction.QueryRowContext(ctx, `
SELECT 1 FROM telemetry_events
WHERE resolution_id = ?
  AND kind = 'skill.loaded'
  AND json_valid(payload_json)
  AND json_extract(payload_json,'$.payload.basis') = 'server-observed'
LIMIT 1`, resolutionID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func buildFeedbackEvents(config Config, feedback Feedback, source storedEnvelope, afterLoad bool) ([]storedEnvelope, error) {
	payload := map[string]any{"status": feedback.Outcome}
	if feedback.SkillID != "" {
		payload["skill_id"] = feedback.SkillID
	}
	if afterLoad {
		payload["after_load"] = true
	}
	if feedback.ReasonCode != "" {
		payload["reason_codes"] = []string{feedback.ReasonCode}
	}
	eventType := feedbackEventTypes[feedback.Outcome]
	occurredAt := config.Clock().UTC()
	primary, err := validateAndBuild(Event{
		ID: feedback.EventID, Type: eventType, OccurredAt: occurredAt,
		ResolutionID: feedback.ResolutionID, CatalogSnapshot: source.CatalogSnapshot,
		PolicyRevision: source.PolicyRevision, Client: source.Client, Payload: payload,
	}, config.ContentMode, occurredAt, feedback.EventID)
	if err != nil {
		return nil, err
	}
	events := []storedEnvelope{primary}
	if feedback.Utility == "" {
		return events, nil
	}
	utilityPayload := map[string]any{"utility": feedback.Utility, "basis": feedback.Basis}
	if feedback.SkillID != "" {
		utilityPayload["skill_id"] = feedback.SkillID
	}
	if afterLoad {
		utilityPayload["after_load"] = true
	}
	if feedback.ReasonCode != "" {
		utilityPayload["reason_codes"] = []string{feedback.ReasonCode}
	}
	utilityID := feedback.EventID + ":utility"
	utility, err := validateAndBuild(Event{
		ID: utilityID, Type: EventSkillUtilityReported, OccurredAt: occurredAt,
		ResolutionID: feedback.ResolutionID, CatalogSnapshot: source.CatalogSnapshot,
		PolicyRevision: source.PolicyRevision, Client: source.Client, Payload: utilityPayload,
	}, config.ContentMode, occurredAt, utilityID)
	if err != nil {
		return nil, err
	}
	return append(events, utility), nil
}

func storeFeedbackEvents(ctx context.Context, transaction *sql.Tx, events []storedEnvelope) (bool, error) {
	primaryExists, primaryMatches, err := feedbackEventState(ctx, transaction, events[0])
	if err != nil {
		return false, err
	}
	utilityID := events[0].ID + ":utility"
	if primaryExists {
		if !primaryMatches {
			return false, ErrFeedbackConflict
		}
		if len(events) == 1 {
			exists, _, err := feedbackEventByID(ctx, transaction, utilityID)
			if err != nil {
				return false, err
			}
			if exists {
				return false, ErrFeedbackConflict
			}
			return true, nil
		}
		exists, matches, err := feedbackEventState(ctx, transaction, events[1])
		if err != nil {
			return false, err
		}
		if !exists || !matches {
			return false, ErrFeedbackConflict
		}
		return true, nil
	}

	utilityExists, _, err := feedbackEventByID(ctx, transaction, utilityID)
	if err != nil {
		return false, err
	}
	if utilityExists {
		return false, ErrFeedbackConflict
	}
	for _, event := range events {
		if _, err := insertEvent(ctx, transaction, event, insertEventSQL); err != nil {
			return false, err
		}
	}
	return false, nil
}

func feedbackEventState(ctx context.Context, transaction *sql.Tx, expected storedEnvelope) (bool, bool, error) {
	exists, actual, err := feedbackEventByID(ctx, transaction, expected.ID)
	if err != nil || !exists {
		return exists, false, err
	}
	return true, sameFeedbackEvent(actual, expected), nil
}

func feedbackEventByID(ctx context.Context, transaction *sql.Tx, id string) (bool, storedEnvelope, error) {
	var occurredAt, kind, raw string
	err := transaction.QueryRowContext(ctx, `SELECT occurred_at,kind,payload_json FROM telemetry_events WHERE id = ?`, id).Scan(&occurredAt, &kind, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, storedEnvelope{}, nil
	}
	if err != nil {
		return false, storedEnvelope{}, err
	}
	var envelope storedEnvelope
	if json.Unmarshal([]byte(raw), &envelope) != nil || validateStored(envelope, id, occurredAt, kind) != nil {
		return true, storedEnvelope{}, nil
	}
	return true, envelope, nil
}

func sameFeedbackEvent(actual, expected storedEnvelope) bool {
	actualPayload, actualErr := json.Marshal(withoutServerDerivedFeedback(actual.Payload))
	expectedPayload, expectedErr := json.Marshal(withoutServerDerivedFeedback(expected.Payload))
	return actualErr == nil && expectedErr == nil && string(actualPayload) == string(expectedPayload) &&
		actual.Version == expected.Version && actual.ID == expected.ID && actual.Type == expected.Type &&
		actual.ResolutionID == expected.ResolutionID && actual.CatalogSnapshot == expected.CatalogSnapshot &&
		actual.PolicyRevision == expected.PolicyRevision && actual.Client == expected.Client && actual.Privacy == expected.Privacy
}

// withoutServerDerivedFeedback drops fields the hub derives at write time, so
// a host retry stays idempotent even when a server-observed load was recorded
// between the original report and the retry.
func withoutServerDerivedFeedback(payload map[string]any) map[string]any {
	if _, exists := payload["after_load"]; !exists {
		return payload
	}
	trimmed := make(map[string]any, len(payload))
	for key, value := range payload {
		if key != "after_load" {
			trimmed[key] = value
		}
	}
	return trimmed
}
