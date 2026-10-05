package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	EventVersion     = "1"
	ExportVersion    = "1"
	RedactionVersion = "redact-v1"
	ContentModeNone  = "none"

	EventResolutionStarted      = "resolution.started"
	EventResolutionRecommended  = "resolution.recommended"
	EventResolutionCompleted    = "resolution.completed"
	EventResolutionFailed       = "resolution.failed"
	EventClarificationRequested = "clarification.requested"
	EventClarificationAnswered  = "clarification.answered"
	EventSkillActivated         = "skill.activated"
	EventActivationApproved     = "activation.approved"
	EventActivationRejected     = "activation.rejected"
	EventSkillLoaded            = "skill.loaded"
	EventSkillUsed              = "skill.used"
	EventSkillCompleted         = "skill.completed"
	EventSkillAbandoned         = "skill.abandoned"
	EventTaskCompleted          = "task.completed"
	EventTaskOutcomeReported    = "task.outcome_reported"
	EventSkillUtilityReported   = "skill.utility_reported"
	EventSkillDoctorChecked     = "skill.doctor_checked"
	EventTranscriptToolObserved = "transcript.tool_observed"

	EventSourceCandidateCaptured  = "source_candidate.captured"
	EventSourceCandidateTriaged   = "source_candidate.triaged"
	EventSourceChecked            = "source.checked"
	EventCurationSessionCompleted = "curation.session_completed"

	EventDistillRunPrepared           = "distill_run.prepared"
	EventDistillRunSubmitted          = "distill_run.submitted"
	EventDistillRunFinalized          = "distill_run.finalized"
	EventDistillRunFailed             = "distill_run.failed"
	EventObservationCreated           = "observation.created"
	EventObservationUpdated           = "observation.updated"
	EventObservationTombstoned        = "observation.tombstoned"
	EventCoverageGapRecorded          = "coverage_gap.recorded"
	EventComparisonUpdated            = "comparison.updated"
	EventInsightProposed              = "insight.proposed"
	EventInsightReopened              = "insight.reopened"
	EventIncorporationOutcomeRecorded = "incorporation.outcome_recorded"

	EventIndexRebuilt           = "index.rebuilt"
	EventCatalogChanged         = "catalog.changed"
	EventEvaluationRunCompleted = "evaluation.run_completed"
)

// Client identifies the calling integration without recording request content.
type Client struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Event is the input accepted by Recorder.Record. Empty version, ID and time are
// filled by the recorder. Payload keys and value shapes are event-type specific.
type Event struct {
	Version         string         `json:"event_version,omitempty"`
	ID              string         `json:"event_id,omitempty"`
	Type            string         `json:"event_type"`
	OccurredAt      time.Time      `json:"occurred_at,omitempty"`
	SessionIDHash   string         `json:"session_id_hash,omitempty"`
	RequestID       string         `json:"request_id,omitempty"`
	ResolutionID    string         `json:"resolution_id,omitempty"`
	CatalogSnapshot string         `json:"catalog_snapshot"`
	PolicyRevision  string         `json:"policy_revision"`
	Client          Client         `json:"client"`
	Payload         map[string]any `json:"payload,omitempty"`
}

type privacyEnvelope struct {
	ContentMode      string `json:"content_mode"`
	RedactionVersion string `json:"redaction_version"`
}

type storedEnvelope struct {
	Version         string          `json:"event_version"`
	ID              string          `json:"event_id"`
	Type            string          `json:"event_type"`
	OccurredAt      string          `json:"occurred_at"`
	SessionIDHash   string          `json:"session_id_hash,omitempty"`
	RequestID       string          `json:"request_id,omitempty"`
	ResolutionID    string          `json:"resolution_id,omitempty"`
	CatalogSnapshot string          `json:"catalog_snapshot"`
	PolicyRevision  string          `json:"policy_revision"`
	Client          Client          `json:"client"`
	Privacy         privacyEnvelope `json:"privacy"`
	Payload         map[string]any  `json:"payload"`
}

type valueKind uint8

const (
	kindToken valueKind = iota
	kindTokens
	kindCount
	kindMillis
	kindBool
	kindStageMillis
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@+\-]{0,255}$`)

var commonRouting = fields(
	"status", kindToken, "operation", kindToken, "artifact_kind", kindToken,
	"constraint_count", kindCount, "fact_keys", kindTokens, "candidate_count", kindCount,
	"top_skill_id", kindToken, "skill_id", kindToken, "confidence_band", kindToken,
	"reason_codes", kindTokens, "recommended_skill_ids", kindTokens, "channels", kindTokens, "stage_ms", kindStageMillis,
	"duration_ms", kindMillis, "error_code", kindToken, "basis", kindToken, "setup_state", kindToken,
)

// Closed vocabularies for fields that become rollup metric names. Enforcing
// them keeps rollup cardinality bounded and the Go allowlist aligned with the
// published event schema.
var (
	setupStates              = map[string]bool{"ready": true, "setup_required": true, "unsupported_platform": true, "review_required": true, "unknown": true}
	skillResourceKinds       = map[string]bool{"entrypoint": true, "reference": true, "script": true, "asset": true, "resource": true}
	skillLoadSurfaces        = map[string]bool{"skill_get": true, "skills_get": true, "resources_read": true}
	activationAttributes     = map[string]bool{"recommended": true, "supporting": true, "override": true, "after_no_skill": true, "after_needs_context": true, "unsolicited": true}
	doctorStatuses           = map[string]bool{"ready": true, "setup_required": true, "unsupported_platform": true, "failed": true}
	contentReviewReasonCodes = map[string]bool{"content_review_required": true, "content_review_stale": true}
)

const (
	// LoadBasisServerObserved marks a skill load the hub observed itself, as
	// opposed to a host-reported feedback outcome.
	LoadBasisServerObserved = "server-observed"
	TranscriptSourceClaude  = "claude-code"
	TranscriptBasis         = "transcript"
)

var eventPayloads = map[string]map[string]valueKind{
	EventResolutionStarted:      commonRouting,
	EventResolutionRecommended:  commonRouting,
	EventResolutionCompleted:    commonRouting,
	EventResolutionFailed:       commonRouting,
	EventClarificationRequested: fields("field", kindToken, "reason_codes", kindTokens, "duration_ms", kindMillis),
	EventClarificationAnswered:  fields("field", kindToken, "answer_kind", kindToken, "duration_ms", kindMillis),
	EventSkillActivated:         skillFields(), EventActivationApproved: skillFields(), EventActivationRejected: skillFields(),
	EventSkillLoaded: skillFields(), EventSkillUsed: skillFields(), EventSkillCompleted: skillFields(), EventSkillAbandoned: skillFields(),
	EventTaskCompleted:          fields("status", kindToken, "duration_ms", kindMillis, "skill_count", kindCount),
	EventTaskOutcomeReported:    fields("status", kindToken, "skill_id", kindToken, "utility", kindToken, "basis", kindToken, "reason_codes", kindTokens, "duration_ms", kindMillis, "after_load", kindBool),
	EventSkillUtilityReported:   fields("skill_id", kindToken, "utility", kindToken, "basis", kindToken, "reason_codes", kindTokens, "after_load", kindBool),
	EventSkillDoctorChecked:     fields("skill_id", kindToken, "status", kindToken, "reason_codes", kindTokens, "duration_ms", kindMillis),
	EventTranscriptToolObserved: fields("tool", kindToken, "skill_id", kindToken, "source", kindToken, "basis", kindToken, "resolved_before", kindBool),

	EventSourceCandidateCaptured: curationFields(), EventSourceCandidateTriaged: curationFields(), EventSourceChecked: curationFields(),
	EventCurationSessionCompleted: fields("status", kindToken, "basis", kindToken, "turns_to_next_action", kindCount, "unnecessary_confirmations", kindCount, "prompts_per_batch", kindCount, "batch_size", kindCount, "auto_finalized", kindBool, "recovery_completed", kindBool, "routine_git_noise", kindCount, "duration_ms", kindMillis, "error_code", kindToken),

	EventDistillRunPrepared: distillFields(), EventDistillRunSubmitted: distillFields(), EventDistillRunFinalized: distillFields(), EventDistillRunFailed: distillFields(),
	EventObservationCreated: entityFields(), EventObservationUpdated: entityFields(), EventObservationTombstoned: entityFields(),
	EventCoverageGapRecorded: fields("run_id", kindToken, "resource_id", kindToken, "classification", kindToken, "reason_code", kindToken, "duration_ms", kindMillis),
	EventComparisonUpdated:   entityFields(), EventInsightProposed: entityFields(), EventInsightReopened: entityFields(), EventIncorporationOutcomeRecorded: fields("incorporation_id", kindToken, "insight_id", kindToken, "outcome", kindToken, "basis", kindToken, "duration_ms", kindMillis),

	EventIndexRebuilt:           fields("status", kindToken, "entity_count", kindCount, "duration_ms", kindMillis, "error_code", kindToken),
	EventCatalogChanged:         fields("change_kind", kindToken, "entity_count", kindCount, "duration_ms", kindMillis),
	EventEvaluationRunCompleted: fields("run_id", kindToken, "suite_id", kindToken, "variant", kindToken, "status", kindToken, "case_count", kindCount, "acceptable_count", kindCount, "no_skill_count", kindCount, "seed", kindCount, "model_id", kindToken, "duration_ms", kindMillis, "error_code", kindToken),
}

func fields(values ...any) map[string]valueKind {
	result := make(map[string]valueKind, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		result[values[i].(string)] = values[i+1].(valueKind)
	}
	return result
}

func skillFields() map[string]valueKind {
	return fields("skill_id", kindToken, "status", kindToken, "reason_codes", kindTokens, "basis", kindToken, "duration_ms", kindMillis, "error_code", kindToken,
		"resource_kind", kindToken, "surface", kindToken, "attribution", kindToken, "first_activation", kindBool, "after_load", kindBool)
}

func curationFields() map[string]valueKind {
	return fields("candidate_id", kindToken, "source_id", kindToken, "status", kindToken, "triage", kindToken, "reason_codes", kindTokens, "duration_ms", kindMillis, "error_code", kindToken)
}

func distillFields() map[string]valueKind {
	return fields("run_id", kindToken, "status", kindToken, "observation_count", kindCount, "coverage_gap_count", kindCount, "resource_count", kindCount, "cursor_advanced", kindBool, "auto_finalized", kindBool, "retry_count", kindCount, "duration_ms", kindMillis, "error_code", kindToken)
}

func entityFields() map[string]valueKind {
	return fields("entity_id", kindToken, "run_id", kindToken, "status", kindToken, "reason_codes", kindTokens, "duration_ms", kindMillis)
}

// storedTimeLayout is fixed width (nanoseconds, UTC) so that lexical SQLite
// comparison of stored timestamps matches chronological order.
const storedTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatStoredTime(value time.Time) string { return value.UTC().Format(storedTimeLayout) }

func validateAndBuild(event Event, contentMode string, now time.Time, id string) (storedEnvelope, error) {
	if event.Version == "" {
		event.Version = EventVersion
	}
	if event.Version != EventVersion {
		return storedEnvelope{}, errors.New("unsupported event_version")
	}
	allowed, ok := eventPayloads[event.Type]
	if !ok {
		return storedEnvelope{}, fmt.Errorf("unsupported event_type %q", event.Type)
	}
	if event.ID == "" {
		event.ID = id
	}
	if !validToken(event.ID) {
		return storedEnvelope{}, errors.New("event_id must be a bounded opaque identifier")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	if event.OccurredAt.Location() != time.UTC {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	for name, value := range map[string]string{
		"session_id_hash": event.SessionIDHash, "request_id": event.RequestID, "resolution_id": event.ResolutionID,
		"catalog_snapshot": event.CatalogSnapshot, "policy_revision": event.PolicyRevision,
		"client.name": event.Client.Name, "client.version": event.Client.Version,
	} {
		if value != "" && !validToken(value) {
			return storedEnvelope{}, fmt.Errorf("%s must be a bounded opaque token", name)
		}
	}
	if event.CatalogSnapshot == "" || event.PolicyRevision == "" {
		return storedEnvelope{}, errors.New("catalog_snapshot and policy_revision are required")
	}
	if event.Client.Name == "" {
		return storedEnvelope{}, errors.New("client.name is required")
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	if err := validatePayload(event.Payload, allowed); err != nil {
		return storedEnvelope{}, err
	}
	if err := validateEventSemantics(event.Type, event.Payload); err != nil {
		return storedEnvelope{}, err
	}
	return storedEnvelope{
		Version: event.Version, ID: event.ID, Type: event.Type, OccurredAt: formatStoredTime(event.OccurredAt),
		SessionIDHash: event.SessionIDHash, RequestID: event.RequestID, ResolutionID: event.ResolutionID,
		CatalogSnapshot: event.CatalogSnapshot, PolicyRevision: event.PolicyRevision, Client: event.Client,
		Privacy: privacyEnvelope{ContentMode: contentMode, RedactionVersion: RedactionVersion}, Payload: event.Payload,
	}, nil
}

func validateEventSemantics(eventType string, payload map[string]any) error {
	if err := validateEnumFields(payload, map[string]map[string]bool{
		"setup_state": setupStates, "resource_kind": skillResourceKinds, "surface": skillLoadSurfaces, "attribution": activationAttributes,
	}); err != nil {
		return err
	}
	if eventType == EventSkillLoaded && payload["basis"] == LoadBasisServerObserved {
		if _, ok := payload["skill_id"].(string); !ok {
			return errors.New("server-observed skill load requires skill_id")
		}
		if _, ok := payload["resource_kind"].(string); !ok {
			return errors.New("server-observed skill load requires resource_kind")
		}
		if statusRaw, exists := payload["status"]; exists {
			status, ok := statusRaw.(string)
			if !ok || status != "review_required" {
				return errors.New("server-observed skill load status must be review_required when present")
			}
			if first, _ := payload["first_activation"].(bool); first {
				return errors.New("server-observed blocked load cannot have first_activation")
			}
		}
		if values, exists := payload["reason_codes"]; exists {
			reasons, ok := stringTokens(values)
			if !ok {
				return errors.New("server-observed skill load reason_codes must be an array")
			}
			for _, reason := range reasons {
				if !contentReviewReasonCodes[reason] {
					return errors.New("server-observed skill load reason_code is not supported")
				}
			}
		}
		if first, _ := payload["first_activation"].(bool); first {
			if _, ok := payload["attribution"].(string); !ok {
				return errors.New("first activation requires attribution")
			}
		}
	}
	if eventType == EventSkillDoctorChecked {
		status, ok := payload["status"].(string)
		if !ok || !doctorStatuses[status] {
			return errors.New("doctor check requires a supported status")
		}
		if _, ok := payload["skill_id"].(string); !ok {
			return errors.New("doctor check requires skill_id")
		}
	}
	if eventType == EventTranscriptToolObserved {
		if _, ok := payload["tool"].(string); !ok {
			return errors.New("transcript observation requires tool")
		}
		if payload["source"] != TranscriptSourceClaude || payload["basis"] != TranscriptBasis {
			return errors.New("transcript observation requires a supported source and transcript basis")
		}
	}
	if eventType == EventResolutionRecommended || eventType == EventResolutionCompleted || eventType == EventResolutionFailed {
		if rawRecommended, exists := payload["recommended_skill_ids"]; exists {
			recommended, ok := stringTokens(rawRecommended)
			if !ok {
				return errors.New("resolution recommended_skill_ids must be an array")
			}
			seen := make(map[string]struct{}, len(recommended))
			for _, skillID := range recommended {
				if _, duplicate := seen[skillID]; duplicate {
					return errors.New("resolution recommended_skill_ids must be unique")
				}
				seen[skillID] = struct{}{}
			}
			for _, field := range []string{"top_skill_id", "skill_id"} {
				if skillID, present := payload[field]; present {
					value, valid := skillID.(string)
					if !valid {
						return fmt.Errorf("resolution %s must be a skill ID", field)
					}
					if _, included := seen[value]; !included {
						return fmt.Errorf("resolution %s must appear in recommended_skill_ids", field)
					}
				}
			}
		}
	}
	if feedbackEventType(eventType) && (eventType != EventSkillLoaded || payload["basis"] != LoadBasisServerObserved) {
		if values, exists := payload["reason_codes"]; exists {
			reasons, ok := stringTokens(values)
			if !ok {
				return errors.New("feedback reason_codes must be an array")
			}
			for _, reason := range reasons {
				if !feedbackReasonCodes[reason] {
					return errors.New("feedback reason_code is not supported")
				}
			}
		}
	}
	if eventType == EventCurationSessionCompleted {
		status, statusOK := payload["status"].(string)
		basis, basisOK := payload["basis"].(string)
		if !statusOK || !basisOK || !curationSessionStatuses[status] || (basis != CurationBasisHostReported && basis != CurationBasisControlledBenchmark) {
			return errors.New("curation session requires a supported observed status and measurement basis")
		}
		for _, field := range []string{"turns_to_next_action", "unnecessary_confirmations", "prompts_per_batch", "batch_size", "routine_git_noise"} {
			if value, exists := payload[field]; exists {
				number, ok := jsonNumber(value)
				if !ok || number < 0 || number > 10_000 {
					return fmt.Errorf("curation session %s must be between 0 and 10000", field)
				}
			}
		}
		if value, exists := payload["duration_ms"]; exists {
			number, ok := jsonNumber(value)
			if !ok || number < 0 || number > 604_800_000 {
				return errors.New("curation session duration_ms must be between 0 and 604800000")
			}
		}
		if errorCode, exists := payload["error_code"]; exists {
			code, ok := errorCode.(string)
			if !ok || !curationSessionErrorCodes[code] || status == "completed" {
				return errors.New("curation session error_code is not valid for the observed status")
			}
		}
	}
	if eventType == EventSkillUtilityReported {
		utility, utilityOK := payload["utility"].(string)
		basis, basisOK := payload["basis"].(string)
		if !utilityOK || !basisOK || !feedbackUtilities[utility] || !feedbackBases[basis] {
			return errors.New("skill utility requires a supported utility and explicit basis")
		}
	}
	if eventType == EventTaskOutcomeReported {
		if _, ok := payload["status"].(string); !ok {
			return errors.New("task outcome requires status")
		}
		utility, hasUtility := payload["utility"]
		basis, hasBasis := payload["basis"]
		if hasUtility != hasBasis {
			return errors.New("task outcome utility and basis must be supplied together")
		}
		if hasUtility {
			utilityValue, utilityOK := utility.(string)
			basisValue, basisOK := basis.(string)
			if !utilityOK || !basisOK || !feedbackUtilities[utilityValue] || !feedbackBases[basisValue] {
				return errors.New("task outcome utility or basis is not supported")
			}
		}
	}
	return nil
}

// validateEnumFields checks closed-vocabulary fields when they are present.
// Shape validation already guaranteed they are tokens.
func validateEnumFields(payload map[string]any, enums map[string]map[string]bool) error {
	for field, allowed := range enums {
		if value, exists := payload[field]; exists {
			text, _ := value.(string)
			if !allowed[text] {
				return fmt.Errorf("payload.%s is not a supported value", field)
			}
		}
	}
	return nil
}

func feedbackEventType(eventType string) bool {
	switch eventType {
	case EventSkillActivated, EventActivationRejected, EventSkillLoaded, EventSkillUsed,
		EventSkillAbandoned, EventTaskOutcomeReported, EventSkillUtilityReported:
		return true
	default:
		return false
	}
}

func stringTokens(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		result := make([]string, len(values))
		for index, item := range values {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[index] = text
		}
		return result, true
	default:
		return nil, false
	}
}

func validatePayload(payload map[string]any, allowed map[string]valueKind) error {
	for key, value := range payload {
		kind, ok := allowed[key]
		if !ok {
			return fmt.Errorf("payload field %q is not allowlisted for this event_type", key)
		}
		if err := validateValue(key, value, kind); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(key string, value any, kind valueKind) error {
	switch kind {
	case kindToken:
		text, ok := value.(string)
		if !ok || !validToken(text) {
			return fmt.Errorf("payload.%s must be a bounded token", key)
		}
	case kindTokens:
		items, ok := value.([]string)
		if !ok {
			if generic, yes := value.([]any); yes {
				items = make([]string, len(generic))
				for i, item := range generic {
					var itemOK bool
					items[i], itemOK = item.(string)
					if !itemOK {
						return fmt.Errorf("payload.%s must contain tokens", key)
					}
				}
			} else {
				return fmt.Errorf("payload.%s must be a token array", key)
			}
		}
		if len(items) > 64 {
			return fmt.Errorf("payload.%s exceeds 64 entries", key)
		}
		for _, item := range items {
			if !validToken(item) {
				return fmt.Errorf("payload.%s must contain bounded tokens", key)
			}
		}
	case kindCount, kindMillis:
		number, ok := jsonNumber(value)
		if !ok || number < 0 || number > 1_000_000_000_000 {
			return fmt.Errorf("payload.%s must be a bounded non-negative integer", key)
		}
	case kindBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("payload.%s must be boolean", key)
		}
	case kindStageMillis:
		object, ok := value.(map[string]int64)
		if !ok {
			generic, yes := value.(map[string]any)
			if !yes {
				return fmt.Errorf("payload.%s must be an integer map", key)
			}
			object = make(map[string]int64, len(generic))
			for name, item := range generic {
				number, valid := jsonNumber(item)
				if !valid {
					return fmt.Errorf("payload.%s.%s must be an integer", key, name)
				}
				object[name] = number
			}
		}
		if len(object) > 32 {
			return fmt.Errorf("payload.%s exceeds 32 entries", key)
		}
		for name, number := range object {
			if !validToken(name) || number < 0 || number > 1_000_000_000_000 {
				return fmt.Errorf("payload.%s contains an invalid stage", key)
			}
		}
	}
	return nil
}

func jsonNumber(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), true
	case int8:
		return int64(number), true
	case int16:
		return int64(number), true
	case int32:
		return int64(number), true
	case int64:
		return number, true
	case uint:
		if uint64(number) <= uint64(^uint64(0)>>1) {
			return int64(number), true
		}
	case uint8:
		return int64(number), true
	case uint16:
		return int64(number), true
	case uint32:
		return int64(number), true
	case uint64:
		if number <= uint64(^uint64(0)>>1) {
			return int64(number), true
		}
	case float64:
		converted := int64(number)
		return converted, float64(converted) == number
	case json.Number:
		converted, err := number.Int64()
		return converted, err == nil
	}
	return 0, false
}

func validToken(value string) bool {
	return tokenPattern.MatchString(strings.TrimSpace(value)) && value == strings.TrimSpace(value)
}
