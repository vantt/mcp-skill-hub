// Package catalog builds and opens immutable SQLite projections of canonical workspaces.
package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

// ErrCanonicalChanged rejects publication when canonical bytes no longer match the immutable build input.
var ErrCanonicalChanged = errors.New("canonical_changed_during_rebuild")

// FaultPoint identifies a durable catalog publication boundary for deterministic tests.
type FaultPoint string

const (
	FaultDatabaseClose      FaultPoint = "database_close"
	FaultGenerationSync     FaultPoint = "generation_sync"
	FaultGenerationsDirSync FaultPoint = "generations_directory_sync"
	FaultPointerTempSync    FaultPoint = "pointer_temp_sync"
	FaultPointerRename      FaultPoint = "pointer_rename"
	FaultPointerDirSync     FaultPoint = "pointer_directory_sync"
)

const (
	// DerivedSchemaVersion changes whenever the disposable SQLite schema changes.
	DerivedSchemaVersion  = 4
	defaultBuilderVersion = "dev"
)

// Pointer is the atomically published identity of the current immutable generation.
type Pointer struct {
	Generation             string `json:"generation"`
	Database               string `json:"database"`
	CatalogSnapshot        string `json:"catalog_snapshot"`
	ProjectionInputDigest  string `json:"projection_input_digest"`
	CanonicalSchemaVersion int    `json:"canonical_schema_version"`
	DerivedSchemaVersion   int    `json:"derived_schema_version"`
	BuilderVersion         string `json:"builder_version"`
}

// PublishFreshness reports whether canonical bytes still match immediately after publish.
type PublishFreshness string

const (
	FreshnessCurrent       PublishFreshness = "current"
	FreshnessStale         PublishFreshness = "stale"
	FreshnessIndeterminate PublishFreshness = "indeterminate"
)

// BuildResult describes a published generation. Warnings never invalidate catalog behavior.
type BuildResult struct {
	Pointer         Pointer
	RowCounts       map[string]int64
	Freshness       PublishFreshness
	FreshnessDetail string
	Warnings        []string
}

// BuildOptions provides deterministic test hooks. Production callers use zero values.
type BuildOptions struct {
	BuilderVersion      string
	BeforeVerify        func(databasePath string) error
	BeforePublish       func() error
	AfterPointerPublish func()
	Progress            func(ProgressEvent)
	Fault               func(FaultPoint) error
	GCMinimumAge        time.Duration
	// ExpectedCatalogSnapshot pins transaction-coupled publication to the
	// canonical result calculated before any files were replaced.
	ExpectedCatalogSnapshot string
}

// ProgressEvent reports a durable rebuild stage without exposing local paths.
type ProgressEvent struct {
	Stage     string
	Completed int
	Total     int
	Message   string
}

func (options BuildOptions) report(stage string, completed, total int, message string) {
	if options.Progress != nil {
		options.Progress(ProgressEvent{Stage: stage, Completed: completed, Total: total, Message: message})
	}
}

func (options BuildOptions) fail(point FaultPoint) error {
	if options.Fault == nil {
		return nil
	}
	return options.Fault(point)
}

// State describes whether the derived catalog can represent current canonical bytes.
type State string

const (
	StateHealthy      State = "healthy"
	StateMissing      State = "missing"
	StateStale        State = "stale"
	StateCorrupt      State = "corrupt"
	StateIncompatible State = "incompatible"
	StateUnknown      State = "unknown"
)

// ServingMode describes whether a generation is served as current, fallback, or unavailable.
type ServingMode string

const (
	ServingCurrent     ServingMode = "current"
	ServingFallback    ServingMode = "fallback"
	ServingUnavailable ServingMode = "unavailable"
)

// Status is the read-only stale/corruption detector result. Unknown means the
// published generation is readable but canonical freshness was intentionally not inspected.
type Status struct {
	State       State       `json:"state"`
	ServingMode ServingMode `json:"serving_mode,omitempty"`
	Pointer     *Pointer    `json:"pointer,omitempty"`
	Generation  string      `json:"generation,omitempty"`
	Detail      string      `json:"detail,omitempty"`
	Warning     string      `json:"warning,omitempty"`
}

// Handle pins an immutable generation until Close is called.
type Handle struct {
	DB      *sql.DB
	Pointer Pointer
	Status  Status
	pinPath string
	release func() error
}

// Close releases the database and then its GC pin.
func (handle *Handle) Close() error {
	if handle == nil {
		return nil
	}
	var closeErr error
	if handle.DB != nil {
		closeErr = handle.DB.Close()
	}
	if handle.pinPath != "" {
		if err := os.Remove(handle.pinPath); err != nil && !os.IsNotExist(err) && closeErr == nil {
			closeErr = err
		}
	}
	if handle.release != nil {
		if err := handle.release(); err != nil && closeErr == nil {
			closeErr = err
		}
		handle.release = nil
	}
	return closeErr
}

func readPointer(root string) (Pointer, error) {
	var pointer Pointer
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return pointer, err
	}
	defer rootHandle.Close()
	const relative = "runtime/catalog/current.json"
	info, err := rootHandle.Lstat(relative)
	if err != nil {
		return pointer, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return pointer, fmt.Errorf("catalog pointer is not a regular file")
	}
	contents, err := rootHandle.ReadFile(relative)
	if err != nil {
		return pointer, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pointer); err != nil {
		return pointer, fmt.Errorf("parse catalog pointer: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return pointer, fmt.Errorf("catalog pointer has trailing data")
	}
	if !validGenerationID(pointer.Generation) || pointer.Database != "generations/"+pointer.Generation+".db" ||
		pointer.CatalogSnapshot == "" || pointer.ProjectionInputDigest == "" || pointer.CanonicalSchemaVersion <= 0 ||
		pointer.DerivedSchemaVersion <= 0 || pointer.BuilderVersion == "" {
		return Pointer{}, fmt.Errorf("catalog pointer is invalid")
	}
	return pointer, nil
}

func validGenerationID(value string) bool {
	if len(value) != len("gen-")+32 || !strings.HasPrefix(value, "gen-") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "gen-"))
	return err == nil && strings.ToLower(value) == value
}

func generationPath(root string, pointer Pointer) string {
	return filepath.Join(root, "runtime", "catalog", filepath.FromSlash(pointer.Database))
}

func waitForContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

const defaultGCMinimumAge = 24 * time.Hour

// CollectGarbage removes inactive, unpinned generations only after the caller's grace period.
func CollectGarbage(root string, minimumAge time.Duration) (resultErr error) {
	lock, err := mutation.AcquireExclusiveLock(context.Background(), root, mutation.DefaultLockTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); resultErr == nil && unlockErr != nil {
			resultErr = unlockErr
		}
	}()
	pointer, err := readPointer(root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	active := pointer.Generation
	generationDir := filepath.Join(root, "runtime", "catalog", "generations")
	entries, err := os.ReadDir(generationDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		generation := entry.Name()[:len(entry.Name())-len(".db")]
		if generation == active {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) < minimumAge {
			continue
		}
		pinDir := filepath.Join(root, "runtime", "catalog", "pins", generation)
		pins, err := os.ReadDir(pinDir)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		livePin := false
		for _, pin := range pins {
			if pin.IsDir() {
				livePin = true
				continue
			}
			pinPath := filepath.Join(pinDir, pin.Name())
			pinInfo, infoErr := pin.Info()
			if infoErr != nil {
				return infoErr
			}
			if time.Since(pinInfo.ModTime()) < minimumAge || !reclaimAbandonedPin(pinPath) {
				livePin = true
			}
		}
		if livePin {
			continue
		}
		_ = os.Remove(pinDir)
		if err := os.Remove(filepath.Join(generationDir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	pruneEmptyPinDirectories(filepath.Join(root, "runtime", "catalog", "pins"))
	return nil
}

// pruneEmptyPinDirectories removes pin directories that hold no pins. Readers
// never remove them because opens only hold a shared lock; the caller must hold
// the exclusive lock so no reader is between creating the directory and its pin.
func pruneEmptyPinDirectories(pinsDir string) {
	entries, err := os.ReadDir(pinsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// os.Remove fails on non-empty directories, which is the intent.
			_ = os.Remove(filepath.Join(pinsDir, entry.Name()))
		}
	}
}
