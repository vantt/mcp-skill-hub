package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const (
	PromotionDraftVersion = "1"
	maxPromotionEvents    = 256
)

var (
	ErrPromotionNotFound  = errors.New("telemetry resolution not found")
	ErrPromotionAmbiguous = errors.New("telemetry resolution is ambiguous")
	ErrPromotionTooLarge  = errors.New("telemetry resolution exceeds promotion event limit")
)

// PromotionDraft is a sanitized, non-canonical starting point for human review.
// Observations describe runtime behavior only; they are never copied into the
// expected evaluation outcome.
type PromotionDraft struct {
	Version         string                 `json:"draft_version"`
	ResolutionID    string                 `json:"resolution_id"`
	CatalogSnapshot string                 `json:"catalog_snapshot"`
	PolicyRevision  string                 `json:"policy_revision"`
	Observed        PromotionObservation   `json:"observed"`
	SourceEventIDs  []string               `json:"source_event_ids"`
	Sanitization    PromotionSanitization  `json:"sanitization"`
	ReviewRequired  bool                   `json:"review_required"`
	CaseTemplate    EvaluationCaseTemplate `json:"evaluation_case_template"`
}

// PromotionObservation contains only explicitly allowlisted resolution output.
type PromotionObservation struct {
	Status      string   `json:"status,omitempty"`
	TopSkillID  string   `json:"top_skill_id,omitempty"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

type PromotionSanitization struct {
	RemovedFields                       []string `json:"removed_fields"`
	UnavailableFields                   []string `json:"unavailable_fields"`
	MalformedEventsSkipped              int      `json:"malformed_events_skipped"`
	UnsupportedContentModeEventsSkipped int      `json:"unsupported_content_mode_events_skipped"`
}

// EvaluationCaseTemplate is deliberately incomplete. A human must provide all
// RequiredHumanFields after checking that the supplied text is sanitized.
type EvaluationCaseTemplate struct {
	Incomplete          bool                         `json:"incomplete"`
	RequiredHumanFields []string                     `json:"required_human_fields"`
	ID                  string                       `json:"id"`
	SchemaVersion       int                          `json:"schema_version"`
	Request             EvaluationRequestTemplate    `json:"request"`
	Expected            EvaluationExpectedTemplate   `json:"expected"`
	Provenance          EvaluationProvenanceTemplate `json:"provenance"`
}

type EvaluationRequestTemplate struct {
	Task EvaluationTaskTemplate `json:"task"`
}

type EvaluationTaskTemplate struct {
	Description string   `json:"description"`
	Constraints []string `json:"constraints"`
}

type EvaluationExpectedTemplate struct {
	AcceptableStatuses []string `json:"acceptable_statuses"`
	AcceptablePrimary  []string `json:"acceptable_primary"`
	Rationale          string   `json:"rationale"`
}

type EvaluationProvenanceTemplate struct {
	Source     string   `json:"source"`
	ReviewedBy []string `json:"reviewed_by"`
	ReviewedAt string   `json:"reviewed_at"`
}

var requiredPromotionFields = []string{
	"evaluation_case_template.id",
	"evaluation_case_template.request.task.description",
	"evaluation_case_template.request.task.constraints",
	"evaluation_case_template.expected.acceptable_statuses",
	"evaluation_case_template.expected.acceptable_primary",
	"evaluation_case_template.expected.rationale",
	"evaluation_case_template.provenance.source",
	"evaluation_case_template.provenance.reviewed_by",
	"evaluation_case_template.provenance.reviewed_at",
}

// JSON returns a stable JSON representation with a trailing newline.
func (draft PromotionDraft) JSON() ([]byte, error) {
	encoded, err := json.Marshal(draft)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func promotionDraftStore(ctx context.Context, config Config, resolutionID string) (PromotionDraft, error) {
	if !validToken(resolutionID) {
		return PromotionDraft{}, errors.New("resolution_id must be a bounded opaque token")
	}
	if err := maintainStore(ctx, config); err != nil {
		return PromotionDraft{}, err
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return PromotionDraft{}, err
	}
	defer database.Close()

	rows, err := database.QueryContext(ctx, `
SELECT id,occurred_at,kind,payload_json
FROM telemetry_events
WHERE resolution_id = ?
ORDER BY occurred_at,id
LIMIT ?`, resolutionID, maxPromotionEvents+1)
	if err != nil {
		return PromotionDraft{}, err
	}
	defer rows.Close()

	type source struct {
		envelope storedEnvelope
		id       string
	}
	sources := make([]source, 0)
	malformed := 0
	unsupportedContentMode := 0
	matchedRows := 0
	for rows.Next() {
		matchedRows++
		if matchedRows > maxPromotionEvents {
			return PromotionDraft{}, ErrPromotionTooLarge
		}
		var id, occurredAt, kind, raw string
		if err := rows.Scan(&id, &occurredAt, &kind, &raw); err != nil {
			return PromotionDraft{}, err
		}
		var envelope storedEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			malformed++
			continue
		}
		if envelope.Privacy.ContentMode != ContentModeNone {
			unsupportedContentMode++
			continue
		}
		if validateStored(envelope, id, occurredAt, kind) != nil || envelope.ResolutionID != resolutionID {
			malformed++
			continue
		}
		sources = append(sources, source{envelope: envelope, id: id})
	}
	if err := rows.Err(); err != nil {
		return PromotionDraft{}, err
	}
	if len(sources) == 0 {
		return PromotionDraft{}, ErrPromotionNotFound
	}

	draft := newPromotionDraft(resolutionID)
	draft.Sanitization.MalformedEventsSkipped = malformed
	draft.Sanitization.UnsupportedContentModeEventsSkipped = unsupportedContentMode
	removed := make(map[string]struct{})
	reasons := make(map[string]struct{})
	var observedStatusPriority int
	var terminalEventType string
	for _, item := range sources {
		envelope := item.envelope
		if draft.CatalogSnapshot == "" {
			draft.CatalogSnapshot = envelope.CatalogSnapshot
			draft.PolicyRevision = envelope.PolicyRevision
		} else if draft.CatalogSnapshot != envelope.CatalogSnapshot || draft.PolicyRevision != envelope.PolicyRevision {
			return PromotionDraft{}, fmt.Errorf("%w: catalog snapshot or policy revision differs", ErrPromotionAmbiguous)
		}
		draft.SourceEventIDs = append(draft.SourceEventIDs, item.id)
		addRemovedEnvelopeFields(removed, envelope)
		if !isResolutionEvent(envelope.Type) {
			for key := range envelope.Payload {
				removed["payload."+key] = struct{}{}
			}
			continue
		}
		priority := statusPriority(envelope.Type)
		if priority == 3 {
			if terminalEventType != "" && terminalEventType != envelope.Type {
				return PromotionDraft{}, fmt.Errorf("%w: both completed and failed terminal events exist", ErrPromotionAmbiguous)
			}
			terminalEventType = envelope.Type
		}
		if status, ok := envelope.Payload["status"].(string); ok && priority >= observedStatusPriority {
			if priority == observedStatusPriority && draft.Observed.Status != "" && draft.Observed.Status != status {
				return PromotionDraft{}, fmt.Errorf("%w: observed status differs", ErrPromotionAmbiguous)
			}
			if priority > observedStatusPriority {
				draft.Observed.Status = ""
				observedStatusPriority = priority
			}
			draft.Observed.Status = status
		}
		if topSkill, ok := envelope.Payload["top_skill_id"].(string); ok {
			if draft.Observed.TopSkillID != "" && draft.Observed.TopSkillID != topSkill {
				return PromotionDraft{}, fmt.Errorf("%w: observed top skill differs", ErrPromotionAmbiguous)
			}
			draft.Observed.TopSkillID = topSkill
		}
		if values, ok := stringSlice(envelope.Payload["reason_codes"]); ok {
			for _, value := range values {
				reasons[value] = struct{}{}
			}
		}
		for key := range envelope.Payload {
			if key != "status" && key != "top_skill_id" && key != "reason_codes" {
				removed["payload."+key] = struct{}{}
			}
		}
	}

	sort.Strings(draft.SourceEventIDs)
	draft.Observed.ReasonCodes = sortedKeys(reasons)
	draft.Sanitization.RemovedFields = sortedKeys(removed)
	return draft, nil
}

func newPromotionDraft(resolutionID string) PromotionDraft {
	required := append([]string(nil), requiredPromotionFields...)
	return PromotionDraft{
		Version: PromotionDraftVersion, ResolutionID: resolutionID, ReviewRequired: true,
		SourceEventIDs: []string{},
		Sanitization:   PromotionSanitization{RemovedFields: []string{}, UnavailableFields: append([]string(nil), required...)},
		CaseTemplate: EvaluationCaseTemplate{
			Incomplete: true, RequiredHumanFields: required, SchemaVersion: 1,
			Request:    EvaluationRequestTemplate{Task: EvaluationTaskTemplate{Constraints: []string{}}},
			Expected:   EvaluationExpectedTemplate{AcceptableStatuses: []string{}, AcceptablePrimary: []string{}},
			Provenance: EvaluationProvenanceTemplate{ReviewedBy: []string{}},
		},
	}
}

func addRemovedEnvelopeFields(removed map[string]struct{}, envelope storedEnvelope) {
	removed["occurred_at"] = struct{}{}
	removed["event_type"] = struct{}{}
	removed["client"] = struct{}{}
	removed["privacy"] = struct{}{}
	if envelope.SessionIDHash != "" {
		removed["session_id_hash"] = struct{}{}
	}
	if envelope.RequestID != "" {
		removed["request_id"] = struct{}{}
	}
}

func isResolutionEvent(kind string) bool {
	switch kind {
	case EventResolutionStarted, EventResolutionRecommended, EventResolutionCompleted, EventResolutionFailed:
		return true
	default:
		return false
	}
}

func statusPriority(kind string) int {
	switch kind {
	case EventResolutionCompleted, EventResolutionFailed:
		return 3
	case EventResolutionRecommended:
		return 2
	case EventResolutionStarted:
		return 1
	default:
		return 0
	}
}

func stringSlice(value any) ([]string, bool) {
	switch items := value.(type) {
	case []string:
		return items, true
	case []any:
		result := make([]string, len(items))
		for index, item := range items {
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

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
