package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// recordCurationTelemetry pins events to the catalog and recommendation policy
// that are current after the domain operation. Telemetry is observational: a
// missing, stale, or panicking sink must never affect the domain result.
func recordCurationTelemetry(ctx context.Context, sink TelemetrySink, root string, events ...telemetry.Event) {
	if sink == nil || len(events) == 0 {
		return
	}
	defer func() { _ = recover() }()

	var pins telemetryPins
	var pinsKnown, pinsTried bool
	for _, event := range events {
		event.Version = telemetry.EventVersion
		if event.CatalogSnapshot != "" && event.PolicyRevision != "" {
			if event.Client.Name == "" {
				event.Client = telemetry.Client{Name: "skillhub"}
			}
			safeRecordTelemetry(sink, event)
			continue
		}
		if !pinsTried {
			pinsTried = true
			pins, pinsKnown = currentTelemetryPins(ctx, root)
		}
		if pinsKnown {
			event.CatalogSnapshot = pins.CatalogSnapshot
			event.PolicyRevision = pins.PolicyRevision
			if event.Client.Name == "" {
				event.Client = telemetry.Client{Name: "skillhub"}
			}
			safeRecordTelemetry(sink, event)
			continue
		}
		if event.Type == telemetry.EventSkillLoaded {
			if event.CatalogSnapshot == "" {
				event.CatalogSnapshot = "unknown"
			}
			if event.PolicyRevision == "" {
				event.PolicyRevision = "unknown"
			}
			if event.Client.Name == "" {
				event.Client = telemetry.Client{Name: "skillhub"}
			}
			safeRecordTelemetry(sink, event)
		}
	}
}

// telemetryPins is the catalog snapshot and recommendation policy revision an
// event is attributed to. Both are fixed for one published generation.
type telemetryPins struct {
	Generation      string
	CatalogSnapshot string
	PolicyRevision  string
}

var (
	telemetryPinsMu    sync.Mutex
	telemetryPinsCache = map[string]telemetryPins{}
)

// currentTelemetryPins returns the pins of the current generation. The
// published pointer names the generation, so a request reads one small file
// and opens the catalog only the first time a generation is seen.
func currentTelemetryPins(ctx context.Context, root string) (telemetryPins, bool) {
	pointer, err := catalog.CurrentPointer(root)
	if err == nil {
		telemetryPinsMu.Lock()
		cached, found := telemetryPinsCache[root]
		telemetryPinsMu.Unlock()
		if found && cached.Generation == pointer.Generation {
			return cached, true
		}
	}
	handle, err := catalog.OpenCurrent(ctx, root)
	if err != nil {
		handle, err = catalog.OpenWithFallback(ctx, root)
	}
	if err != nil {
		return telemetryPins{}, false
	}
	defer handle.Close()
	policy, err := resolverpkg.LoadPolicy(ctx, handle.DB)
	if err != nil || policy.Revision == "" {
		return telemetryPins{}, false
	}
	pins := telemetryPins{Generation: handle.Pointer.Generation, CatalogSnapshot: handle.Pointer.CatalogSnapshot, PolicyRevision: policy.Revision}
	telemetryPinsMu.Lock()
	telemetryPinsCache[root] = pins
	telemetryPinsMu.Unlock()
	return pins, true
}

func curationTelemetryEvent(eventType string, payload map[string]any) telemetry.Event {
	return telemetry.Event{Version: telemetry.EventVersion, Type: eventType, Client: telemetry.Client{Name: "skillhub"}, Payload: payload}
}

// RecordCatalogChange records catalog.changed and index.rebuilt telemetry events
// from the app layer after a generation change.
func RecordCatalogChange(ctx context.Context, sink TelemetrySink, root string, changeKind string, entityCount int64, durationMS int64) {
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()

	evt1 := curationTelemetryEvent(telemetry.EventCatalogChanged, map[string]any{
		"change_kind":  changeKind,
		"entity_count": entityCount,
		"duration_ms":  durationMS,
	})
	evt2 := curationTelemetryEvent(telemetry.EventIndexRebuilt, map[string]any{
		"status":       "completed",
		"entity_count": entityCount,
		"duration_ms":  durationMS,
	})
	recordCurationTelemetry(ctx, sink, root, evt1, evt2)
}

// CurationSessionInput contains only explicitly observed, content-free values.
// Nil measurements were unavailable and are omitted rather than guessed.
type CurationSessionInput struct {
	SchemaVersion            string  `json:"schema_version"`
	EventID                  string  `json:"event_id"`
	Status                   string  `json:"status"`
	Basis                    string  `json:"basis"`
	TurnsToNextAction        *int64  `json:"turns_to_next_action,omitempty"`
	UnnecessaryConfirmations *int64  `json:"unnecessary_confirmations,omitempty"`
	PromptsPerBatch          *int64  `json:"prompts_per_batch,omitempty"`
	BatchSize                *int64  `json:"batch_size,omitempty"`
	AutoFinalized            *bool   `json:"auto_finalized,omitempty"`
	RecoveryCompleted        *bool   `json:"recovery_completed,omitempty"`
	RoutineGitNoise          *int64  `json:"routine_git_noise,omitempty"`
	DurationMS               *int64  `json:"duration_ms,omitempty"`
	ErrorCode                *string `json:"error_code,omitempty"`
}

type CurationSessionResult struct {
	Result
	EventID          string `json:"event_id"`
	Deduplicated     bool   `json:"deduplicated"`
	CanonicalMutated bool   `json:"canonical_mutated"`
	PolicyMutated    bool   `json:"policy_mutated"`
}

// CurationTelemetryService records completed-session UX observations. The
// recorder is disposable derived state; this service never invokes a canonical
// mutation or policy write path.
type CurationTelemetryService struct {
	Recorder  *telemetry.Recorder
	Telemetry TelemetryService
}

func (service CurationTelemetryService) RecordSession(ctx context.Context, path string, input CurationSessionInput) (result CurationSessionResult, resultErr error) {
	input = normalizeCurationSession(input)
	if err := validateCurationSessionInput(input); err != nil {
		return CurationSessionResult{}, err
	}

	handle, err := catalog.OpenCurrent(ctx, path)
	if err != nil {
		return CurationSessionResult{}, fmt.Errorf("derive current catalog snapshot: %w", err)
	}
	policy, policyErr := resolverpkg.LoadPolicy(ctx, handle.DB)
	closeErr := handle.Close()
	if policyErr != nil {
		return CurationSessionResult{}, fmt.Errorf("derive current policy revision: %w", policyErr)
	}
	if closeErr != nil {
		return CurationSessionResult{}, fmt.Errorf("close current catalog: %w", closeErr)
	}
	if handle.Pointer.CatalogSnapshot == "" || policy.Revision == "" {
		return CurationSessionResult{}, errors.New("current catalog snapshot and policy revision are required")
	}

	recorder := service.Recorder
	if recorder == nil {
		recorder, err = service.Telemetry.Open(path)
		if err != nil {
			return CurationSessionResult{}, err
		}
		defer closeTelemetryRecorder(recorder, &resultErr)
	}
	report := telemetry.CurationSession{
		EventID: input.EventID, Status: input.Status, Basis: input.Basis,
		TurnsToNextAction: input.TurnsToNextAction, UnnecessaryConfirmations: input.UnnecessaryConfirmations,
		PromptsPerBatch: input.PromptsPerBatch, BatchSize: input.BatchSize,
		AutoFinalized: input.AutoFinalized, RecoveryCompleted: input.RecoveryCompleted,
		RoutineGitNoise: input.RoutineGitNoise, DurationMS: input.DurationMS, ErrorCode: input.ErrorCode,
		CatalogSnapshot: handle.Pointer.CatalogSnapshot, PolicyRevision: policy.Revision,
		Client: telemetry.Client{Name: "skillhub-mcp", Version: telemetry.EventVersion},
	}
	stored, err := recorder.RecordCurationSession(ctx, report)
	if err != nil {
		return CurationSessionResult{}, err
	}
	summary := "Curation session measurements were recorded as local runtime telemetry."
	if stored.Deduplicated {
		summary = "The existing curation session event was returned without a duplicate write."
	}
	return CurationSessionResult{
		Result: NewResult(StatusOK, summary), EventID: input.EventID,
		Deduplicated: stored.Deduplicated, CanonicalMutated: false, PolicyMutated: false,
	}, nil
}

func normalizeCurationSession(input CurationSessionInput) CurationSessionInput {
	input.SchemaVersion = strings.TrimSpace(input.SchemaVersion)
	input.EventID = strings.TrimSpace(input.EventID)
	input.Status = strings.TrimSpace(input.Status)
	input.Basis = strings.TrimSpace(input.Basis)
	if input.ErrorCode != nil {
		value := strings.TrimSpace(*input.ErrorCode)
		input.ErrorCode = &value
	}
	return input
}

func validateCurationSessionInput(input CurationSessionInput) error {
	if input.SchemaVersion != telemetry.EventVersion {
		return errors.New("unsupported schema_version; use 1")
	}
	if input.EventID == "" || len(input.EventID) > 128 {
		return errors.New("event_id must contain 1..128 characters")
	}
	if input.Status == "" {
		return errors.New("status is required and must be explicitly observed")
	}
	if input.Basis == "" {
		return errors.New("basis is required and must identify host-reported or controlled-benchmark measurements")
	}
	return nil
}
