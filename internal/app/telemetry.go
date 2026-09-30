package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// TelemetryService owns the application boundary for disposable workspace
// telemetry. Config.Path and Config.ContentMode are always replaced with the
// workspace-local, content-free defaults.
type TelemetryService struct {
	Config telemetry.Config
}

// TelemetryStoreError marks a failure of the disposable telemetry store itself
// (as opposed to an invalid workspace or bad request), so delivery adapters can
// point at telemetry recovery rather than workspace repair.
type TelemetryStoreError struct{ Err error }

func (e *TelemetryStoreError) Error() string { return e.Err.Error() }
func (e *TelemetryStoreError) Unwrap() error { return e.Err }

func storeFailure(err error) error {
	if err == nil {
		return nil
	}
	var existing *TelemetryStoreError
	if errors.As(err, &existing) {
		return err
	}
	return &TelemetryStoreError{Err: err}
}

// Open starts a recorder for runtime/telemetry.db. The caller must close the
// recorder so its bounded writer can flush and terminate.
func (service TelemetryService) Open(path string) (*telemetry.Recorder, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return nil, err
	}
	config := service.Config
	config.WorkspaceRoot = root
	config.Path = filepath.Join(root, "runtime", "telemetry.db")
	config.ContentMode = telemetry.ContentModeNone
	recorder, err := telemetry.Open(config)
	if err != nil {
		return nil, storeFailure(err)
	}
	return recorder, nil
}

// Health verifies the local store before returning the recorder counters.
func (service TelemetryService) Health(ctx context.Context, path string) (health telemetry.Health, resultErr error) {
	recorder, err := service.Open(path)
	if err != nil {
		return health, err
	}
	defer closeTelemetryRecorder(recorder, &resultErr)
	if err := recorder.Flush(ctx); err != nil {
		return health, storeFailure(err)
	}
	return recorder.Health(), nil
}

// Preview returns sanitized JSONL without creating an export artifact.
func (service TelemetryService) Preview(ctx context.Context, path string, limit int) (preview telemetry.Preview, resultErr error) {
	recorder, err := service.Open(path)
	if err != nil {
		return preview, err
	}
	defer closeTelemetryRecorder(recorder, &resultErr)
	preview, err = recorder.Preview(ctx, limit)
	return preview, storeFailure(err)
}

// PromotionDraft returns a sanitized, explicitly incomplete evaluation-case
// draft for human review. It never mutates canonical evaluation or policy files.
func (service TelemetryService) PromotionDraft(ctx context.Context, path, resolutionID string) (draft telemetry.PromotionDraft, resultErr error) {
	recorder, err := service.Open(path)
	if err != nil {
		return draft, err
	}
	defer closeTelemetryRecorder(recorder, &resultErr)
	return recorder.PromotionDraft(ctx, resolutionID)
}

// Export atomically writes a sanitized JSONL artifact.
func (service TelemetryService) Export(ctx context.Context, path, outputPath string) (result telemetry.ExportResult, resultErr error) {
	recorder, err := service.Open(path)
	if err != nil {
		return result, err
	}
	defer closeTelemetryRecorder(recorder, &resultErr)
	result, err = recorder.Export(ctx, outputPath)
	return result, storeFailure(err)
}

// Purge deletes all telemetry and recreates an empty disposable store.
func (service TelemetryService) Purge(ctx context.Context, path string) (resultErr error) {
	recorder, err := service.Open(path)
	if err != nil {
		return err
	}
	defer closeTelemetryRecorder(recorder, &resultErr)
	return recorder.Purge(ctx)
}

func closeTelemetryRecorder(recorder *telemetry.Recorder, resultErr *error) {
	// Cleanup must not inherit an already-canceled request context: Close owns
	// the recorder goroutine and must get a bounded chance to terminate it.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := recorder.Close(ctx); *resultErr == nil && err != nil {
		*resultErr = fmt.Errorf("close telemetry recorder: %w", err)
	}
}
