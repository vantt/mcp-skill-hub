package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrCurationSessionConflict = errors.New("event_id was already used for different curation session measurements")

const (
	CurationBasisHostReported        = "host-reported"
	CurationBasisControlledBenchmark = "controlled-benchmark"
)

var curationSessionStatuses = map[string]bool{
	"completed": true,
	"partial":   true,
	"failed":    true,
	"abandoned": true,
}

var curationSessionErrorCodes = map[string]bool{
	"capability_unavailable": true,
	"internal_error":         true,
	"operation_cancelled":    true,
	"partial_failure":        true,
	"recovery_required":      true,
	"source_unavailable":     true,
	"validation_failed":      true,
}

// CurationSession is a content-free observation recorded only when a curation
// session has actually ended. Pointer measurements distinguish observed zeroes
// from values the caller did not measure.
type CurationSession struct {
	EventID                  string
	Status                   string
	Basis                    string
	TurnsToNextAction        *int64
	UnnecessaryConfirmations *int64
	PromptsPerBatch          *int64
	BatchSize                *int64
	AutoFinalized            *bool
	RecoveryCompleted        *bool
	RoutineGitNoise          *int64
	DurationMS               *int64
	ErrorCode                *string
	CatalogSnapshot          string
	PolicyRevision           string
	Client                   Client
}

// CurationSessionResult reports whether the exact observed measurements had
// already been stored under the supplied event ID.
type CurationSessionResult struct {
	Deduplicated bool
}

func validateCurationSession(report CurationSession) error {
	if !validToken(report.EventID) {
		return errors.New("event_id must be a bounded opaque identifier")
	}
	if !curationSessionStatuses[report.Status] {
		return errors.New("status must be completed, partial, failed, or abandoned")
	}
	if report.Basis != CurationBasisHostReported && report.Basis != CurationBasisControlledBenchmark {
		return errors.New("basis must be host-reported or controlled-benchmark")
	}
	for name, value := range map[string]*int64{
		"turns_to_next_action":      report.TurnsToNextAction,
		"unnecessary_confirmations": report.UnnecessaryConfirmations,
		"prompts_per_batch":         report.PromptsPerBatch,
		"batch_size":                report.BatchSize,
		"routine_git_noise":         report.RoutineGitNoise,
	} {
		if value != nil && (*value < 0 || *value > 10_000) {
			return fmt.Errorf("%s must be between 0 and 10000", name)
		}
	}
	if report.DurationMS != nil && (*report.DurationMS < 0 || *report.DurationMS > 604_800_000) {
		return errors.New("duration_ms must be between 0 and 604800000")
	}
	if report.ErrorCode != nil && !curationSessionErrorCodes[*report.ErrorCode] {
		return errors.New("error_code is not allowlisted")
	}
	if report.Status == "completed" && report.ErrorCode != nil {
		return errors.New("completed status cannot include error_code")
	}
	return nil
}

func buildCurationSessionEvent(config Config, report CurationSession) (storedEnvelope, error) {
	if err := validateCurationSession(report); err != nil {
		return storedEnvelope{}, err
	}
	payload := map[string]any{"status": report.Status, "basis": report.Basis}
	for name, value := range map[string]*int64{
		"turns_to_next_action":      report.TurnsToNextAction,
		"unnecessary_confirmations": report.UnnecessaryConfirmations,
		"prompts_per_batch":         report.PromptsPerBatch,
		"batch_size":                report.BatchSize,
		"duration_ms":               report.DurationMS,
		"routine_git_noise":         report.RoutineGitNoise,
	} {
		if value != nil {
			payload[name] = *value
		}
	}
	for name, value := range map[string]*bool{
		"auto_finalized":     report.AutoFinalized,
		"recovery_completed": report.RecoveryCompleted,
	} {
		if value != nil {
			payload[name] = *value
		}
	}
	if report.ErrorCode != nil {
		payload["error_code"] = *report.ErrorCode
	}
	now := config.Clock().UTC()
	return validateAndBuild(Event{
		ID: report.EventID, Type: EventCurationSessionCompleted, OccurredAt: now,
		CatalogSnapshot: report.CatalogSnapshot, PolicyRevision: report.PolicyRevision,
		Client: report.Client, Payload: payload,
	}, config.ContentMode, now, report.EventID)
}

func recordCurationSessionStore(ctx context.Context, config Config, report CurationSession) (CurationSessionResult, error) {
	event, err := buildCurationSessionEvent(config, report)
	if err != nil {
		return CurationSessionResult{}, err
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return CurationSessionResult{}, err
	}
	defer database.Close()
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return CurationSessionResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()

	cutoff := formatStoredTime(config.Clock().Add(-config.Retention))
	if _, err := transaction.ExecContext(ctx, `DELETE FROM telemetry_events WHERE occurred_at < ?`, cutoff); err != nil {
		return CurationSessionResult{}, err
	}
	var occurredAt, kind, raw string
	err = transaction.QueryRowContext(ctx, `SELECT occurred_at,kind,payload_json FROM telemetry_events WHERE id = ?`, event.ID).Scan(&occurredAt, &kind, &raw)
	if err == nil {
		var existing storedEnvelope
		if json.Unmarshal([]byte(raw), &existing) != nil || validateStored(existing, event.ID, occurredAt, kind) != nil || existing.Type != EventCurationSessionCompleted || !sameCurationMeasurements(existing.Payload, event.Payload) {
			return CurationSessionResult{}, ErrCurationSessionConflict
		}
		if err := transaction.Commit(); err != nil {
			return CurationSessionResult{}, err
		}
		committed = true
		return CurationSessionResult{Deduplicated: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CurationSessionResult{}, err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return CurationSessionResult{}, err
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`, event.ID, event.OccurredAt, event.Type, nil, string(encoded)); err != nil {
		return CurationSessionResult{}, err
	}
	if err := trimLogicalSize(ctx, transaction, config.MaxSizeBytes); err != nil {
		return CurationSessionResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return CurationSessionResult{}, err
	}
	committed = true
	if err := trimPhysicalSize(ctx, database, config.Path, config.MaxSizeBytes); err != nil {
		return CurationSessionResult{}, err
	}
	return CurationSessionResult{}, nil
}

func sameCurationMeasurements(actual, expected map[string]any) bool {
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && string(actualJSON) == string(expectedJSON)
}
