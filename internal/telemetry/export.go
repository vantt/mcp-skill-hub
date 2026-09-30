package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Preview is an in-memory, sanitized JSONL export suitable for review.
type Preview struct {
	Version string
	JSONL   []byte
	Events  int
	Skipped int
}

// ExportResult describes an atomically written local export.
type ExportResult struct {
	Version string `json:"version"`
	Path    string `json:"path"`
	Events  int    `json:"events"`
	Skipped int    `json:"skipped"`
	Bytes   int64  `json:"bytes"`
}

func previewStore(ctx context.Context, config Config, limit int) (Preview, error) {
	if limit < 0 {
		return Preview{}, errors.New("preview limit cannot be negative")
	}
	if err := maintainStore(ctx, config); err != nil {
		return Preview{}, err
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return Preview{}, err
	}
	defer database.Close()
	query := `SELECT id,occurred_at,kind,payload_json FROM telemetry_events ORDER BY occurred_at,id`
	arguments := []any{}
	if limit > 0 {
		query += ` LIMIT ?`
		arguments = append(arguments, limit)
	}
	rows, err := database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return Preview{}, err
	}
	defer rows.Close()
	result := Preview{Version: ExportVersion}
	var output bytes.Buffer
	for rows.Next() {
		var id, occurredAt, kind, raw string
		if err := rows.Scan(&id, &occurredAt, &kind, &raw); err != nil {
			return Preview{}, err
		}
		var envelope storedEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil || validateStored(envelope, id, occurredAt, kind) != nil {
			result.Skipped++
			continue
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return Preview{}, err
		}
		output.Write(encoded)
		output.WriteByte('\n')
		result.Events++
	}
	if err := rows.Err(); err != nil {
		return Preview{}, err
	}
	result.JSONL = output.Bytes()
	return result, nil
}

func validateStored(envelope storedEnvelope, id, occurredAt, kind string) error {
	if envelope.Privacy.ContentMode != ContentModeNone || envelope.Privacy.RedactionVersion != RedactionVersion {
		return errors.New("unsupported privacy envelope")
	}
	parsed, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil {
		return err
	}
	candidate := Event{
		Version: envelope.Version, ID: envelope.ID, Type: envelope.Type, OccurredAt: parsed,
		SessionIDHash: envelope.SessionIDHash, RequestID: envelope.RequestID, ResolutionID: envelope.ResolutionID,
		CatalogSnapshot: envelope.CatalogSnapshot, PolicyRevision: envelope.PolicyRevision, Client: envelope.Client, Payload: envelope.Payload,
	}
	rebuilt, err := validateAndBuild(candidate, ContentModeNone, parsed, envelope.ID)
	if err != nil {
		return err
	}
	// Older rows were written with variable fractional precision, so compare instants.
	column, err := time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return err
	}
	if rebuilt.ID != id || rebuilt.Type != kind || !column.Equal(parsed) {
		return errors.New("stored envelope does not match indexed columns")
	}
	return nil
}

func exportStore(ctx context.Context, config Config, outputPath string) (ExportResult, error) {
	if outputPath == "" {
		return ExportResult{}, errors.New("export path is required")
	}
	if samePath(config.Path, outputPath) {
		return ExportResult{}, errors.New("export path cannot replace the telemetry database")
	}
	preview, err := previewStore(ctx, config, 0)
	if err != nil {
		return ExportResult{}, err
	}
	if err := writeAtomic(outputPath, preview.JSONL); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Version: ExportVersion, Path: outputPath, Events: preview.Events, Skipped: preview.Skipped, Bytes: int64(len(preview.JSONL))}, nil
}

func writeAtomic(path string, contents []byte) error {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("export target is a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".telemetry-export-*.jsonl")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(contents)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return fmt.Errorf("sync export directory: %w", err)
	}
	return nil
}

func samePath(left, right string) bool {
	absoluteLeft, leftErr := filepath.Abs(left)
	absoluteRight, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	ca := filepath.Clean(absoluteLeft)
	cb := filepath.Clean(absoluteRight)
	if realA, err := filepath.EvalSymlinks(ca); err == nil {
		ca = filepath.Clean(realA)
	}
	if realB, err := filepath.EvalSymlinks(cb); err == nil {
		cb = filepath.Clean(realB)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}
