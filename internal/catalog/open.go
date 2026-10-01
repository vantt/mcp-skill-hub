package catalog

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

// Inspect reports catalog freshness without changing runtime or canonical state.
func Inspect(ctx context.Context, root string) (Status, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return Status{}, err
	}
	defer lock.Unlock()
	return InspectWhileLocked(ctx, root)
}

// EnsureFreshOrRebuild inspects catalog freshness. If the catalog is stale
// and canonical files pass validation, it automatically rebuilds the generation.
// If canonical validation fails, it returns a clear error describing the issue.
func EnsureFreshOrRebuild(ctx context.Context, root string) error {
	status, err := Inspect(ctx, root)
	if err != nil {
		return err
	}
	if status.State == StateHealthy {
		return nil
	}
	if status.State != StateStale {
		return catalogUnavailableError{state: status.State, detail: status.Detail}
	}
	// Check canonical validation before rebuilding
	issues, err := canonical.Validate(root)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return fmt.Errorf("search index is stale: canonical validation failed: %s: %s (run `skillhub validate` to inspect)", issues[0].Path, issues[0].Message)
	}
	_, err = BuildCatalogGeneration(ctx, root, BuildOptions{})
	return err
}

// InspectPublished reports pointer and generation health without reading canonical
// inputs. Callers use it while recovery is pending, when canonical files may be mid-update.
func InspectPublished(ctx context.Context, root string) (Status, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return Status{}, err
	}
	defer lock.Unlock()
	return inspectPublishedWhileLocked(ctx, root)
}

func inspectPublishedWhileLocked(ctx context.Context, root string) (Status, error) {
	if err := waitForContext(ctx); err != nil {
		return Status{}, err
	}
	pointer, err := readPointer(root)
	if errors.Is(err, os.ErrNotExist) {
		return Status{State: StateMissing, ServingMode: ServingUnavailable, Detail: "catalog generation pointer is missing"}, nil
	}
	if err != nil {
		return Status{State: StateCorrupt, ServingMode: ServingUnavailable, Detail: err.Error()}, nil
	}
	if pointer.DerivedSchemaVersion != DerivedSchemaVersion {
		return Status{State: StateIncompatible, ServingMode: ServingUnavailable, Pointer: &pointer, Generation: pointer.Generation, Detail: "derived schema version is incompatible"}, nil
	}
	if err := verifyPointerDatabase(ctx, root, pointer); err != nil {
		return Status{State: StateCorrupt, ServingMode: ServingUnavailable, Pointer: &pointer, Generation: pointer.Generation, Detail: err.Error()}, nil
	}
	return Status{State: StateUnknown, ServingMode: ServingCurrent, Pointer: &pointer, Generation: pointer.Generation, Detail: "canonical freshness was not inspected while workspace recovery is pending"}, nil
}

// InspectWhileLocked reports freshness while the caller holds a workspace lock.
func InspectWhileLocked(ctx context.Context, root string) (Status, error) {
	published, err := inspectPublishedWhileLocked(ctx, root)
	if err != nil || published.State != StateUnknown {
		return published, err
	}
	pointer := *published.Pointer
	snapshot, err := canonical.Scan(root)
	if err != nil {
		return Status{}, err
	}
	if snapshot.CatalogSnapshot != pointer.CatalogSnapshot || snapshot.ProjectionInputDigest != pointer.ProjectionInputDigest {
		issues, valErr := canonical.Validate(root)
		if valErr == nil && len(issues) > 0 {
			return Status{
				State:       StateStale,
				ServingMode: ServingFallback,
				Pointer:     &pointer,
				Generation:  pointer.Generation,
				Detail:      fmt.Sprintf("canonical validation failed: %s: %s", issues[0].Path, issues[0].Message),
				Warning:     fmt.Sprintf("canonical validation failed: %s: %s; serving published catalog generation %s as fallback", issues[0].Path, issues[0].Message, pointer.Generation),
			}, nil
		}
		return Status{
			State:       StateStale,
			ServingMode: ServingFallback,
			Pointer:     &pointer,
			Generation:  pointer.Generation,
			Detail:      "canonical inputs differ from the published generation",
			Warning:     fmt.Sprintf("canonical inputs differ from published catalog generation %s", pointer.Generation),
		}, nil
	}
	return Status{
		State:       StateHealthy,
		ServingMode: ServingCurrent,
		Pointer:     &pointer,
		Generation:  pointer.Generation,
	}, nil
}

// ErrSnapshotUnavailable reports that no retained, valid immutable generation
// exactly matches the requested catalog snapshot.
var ErrSnapshotUnavailable = errors.New("catalog_snapshot_unavailable")

// OpenSnapshot opens an exact retained catalog snapshot. Candidates are ordered
// by generation ID so equivalent generations are selected deterministically.
// It never substitutes the current or newest generation for the requested one.
func OpenSnapshot(ctx context.Context, root, catalogSnapshot string) (*Handle, error) {
	if catalogSnapshot == "" {
		return nil, fmt.Errorf("%w: catalog snapshot is required", ErrSnapshotUnavailable)
	}
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()
	pointer, err := findSnapshotGeneration(ctx, root, catalogSnapshot)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %v", ErrSnapshotUnavailable, err)
	}
	handle, err := openPinnedGenerationWithStatus(ctx, root, pointer, Status{
		State:       StateHealthy,
		ServingMode: ServingCurrent,
		Pointer:     &pointer,
		Generation:  pointer.Generation,
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: could not pin and open exact generation: %v", ErrSnapshotUnavailable, err)
	}
	return handle, nil
}

// ErrCatalogUnavailable reports that the current catalog is missing, stale,
// corrupt, or incompatible and must be rebuilt before it can be served.
var ErrCatalogUnavailable = errors.New("catalog_unavailable")

type catalogUnavailableError struct {
	state  State
	detail string
}

func (err catalogUnavailableError) Error() string {
	return fmt.Sprintf("catalog is %s: %s", err.state, err.detail)
}

func (err catalogUnavailableError) Is(target error) bool { return target == ErrCatalogUnavailable }

// OpenCurrent pins and opens the current immutable generation read-only.
func OpenCurrent(ctx context.Context, root string) (*Handle, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()
	return openCurrentWhileLocked(ctx, root)
}

// OpenCurrentLocked pins the current generation like OpenCurrent but keeps the
// shared workspace lock until Close. Callers that read canonical files described
// by the generation use it so no mutation can commit between the open and the read.
func OpenCurrentLocked(ctx context.Context, root string) (*Handle, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	handle, err := openCurrentWhileLocked(ctx, root)
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	handle.release = lock.Unlock
	return handle, nil
}

func openCurrentWhileLocked(ctx context.Context, root string) (*Handle, error) {
	status, err := InspectWhileLocked(ctx, root)
	if err != nil {
		return nil, err
	}
	if status.State != StateHealthy || status.Pointer == nil {
		return nil, catalogUnavailableError{state: status.State, detail: status.Detail}
	}
	return openPinnedGenerationWithStatus(ctx, root, *status.Pointer, status)
}

func openPinnedGeneration(ctx context.Context, root string, pointer Pointer) (*Handle, error) {
	status := Status{
		State:       StateHealthy,
		ServingMode: ServingCurrent,
		Pointer:     &pointer,
		Generation:  pointer.Generation,
	}
	return openPinnedGenerationWithStatus(ctx, root, pointer, status)
}

func openPinnedGenerationWithStatus(ctx context.Context, root string, pointer Pointer, status Status) (*Handle, error) {
	pinDirectory := filepath.Join(root, "runtime", "catalog", "pins", pointer.Generation)
	if err := ensureRuntimeDirectory(root, filepath.ToSlash(filepath.Join("runtime", "catalog", "pins", pointer.Generation))); err != nil {
		return nil, err
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	pinPath := filepath.Join(pinDirectory, hex.EncodeToString(random[:])+".pin")
	pin, err := os.OpenFile(pinPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	metadata, marshalErr := json.Marshal(struct {
		PID       int       `json:"pid"`
		CreatedAt time.Time `json:"created_at"`
	}{PID: os.Getpid(), CreatedAt: time.Now().UTC()})
	if marshalErr == nil {
		_, marshalErr = pin.Write(append(metadata, '\n'))
	}
	if marshalErr != nil {
		_ = pin.Close()
		removePin(pinPath)
		return nil, marshalErr
	}
	if err := pin.Close(); err != nil {
		removePin(pinPath)
		return nil, err
	}
	database, err := sql.Open("sqlite", sqliteDSN(generationPath(root, pointer), true))
	if err != nil {
		removePin(pinPath)
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		removePin(pinPath)
		return nil, err
	}
	return &Handle{DB: database, Pointer: pointer, Status: status, pinPath: pinPath}, nil
}

const maxOpenRetries = 3

// OpenWithFallback opens the current generation, or falls back to the published pointer
// if canonical files are invalid. If the catalog is stale and canonical files are valid,
// it rebuilds under publication locking and returns the fresh generation.
func OpenWithFallback(ctx context.Context, root string) (*Handle, error) {
	return openWithFallback(ctx, root, false)
}

// OpenWithFallbackLocked is like OpenWithFallback but retains the shared workspace lock
// until Handle.Close() is called. Callers that read canonical files described by the
// generation use it so no mutation can commit between the open and the read.
func OpenWithFallbackLocked(ctx context.Context, root string) (*Handle, error) {
	return openWithFallback(ctx, root, true)
}

// OpenServable is an alias for OpenWithFallback.
func OpenServable(ctx context.Context, root string) (*Handle, error) {
	return OpenWithFallback(ctx, root)
}

// OpenServableLocked is an alias for OpenWithFallbackLocked.
func OpenServableLocked(ctx context.Context, root string) (*Handle, error) {
	return OpenWithFallbackLocked(ctx, root)
}

func openWithFallback(ctx context.Context, root string, keepLock bool) (*Handle, error) {
	for range maxOpenRetries {
		if err := waitForContext(ctx); err != nil {
			return nil, err
		}
		lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
		if err != nil {
			return nil, err
		}

		published, err := inspectPublishedWhileLocked(ctx, root)
		if err != nil {
			_ = lock.Unlock()
			return nil, err
		}

		if published.State == StateMissing {
			issues, valErr := canonical.Validate(root)
			if valErr != nil || len(issues) > 0 {
				_ = lock.Unlock()
				detail := "catalog generation pointer is missing"
				if len(issues) > 0 {
					detail += " and canonical validation failed: " + issues[0].Message
				}
				return nil, catalogUnavailableError{state: StateMissing, detail: detail}
			}
			_ = lock.Unlock()
			if _, buildErr := BuildCatalogGeneration(ctx, root, BuildOptions{}); buildErr != nil {
				return nil, buildErr
			}
			continue
		}

		if published.State == StateCorrupt || published.State == StateIncompatible {
			issues, valErr := canonical.Validate(root)
			if valErr != nil || len(issues) > 0 {
				_ = lock.Unlock()
				return nil, catalogUnavailableError{state: published.State, detail: published.Detail}
			}
			_ = lock.Unlock()
			if _, buildErr := BuildCatalogGeneration(ctx, root, BuildOptions{}); buildErr != nil {
				return nil, buildErr
			}
			continue
		}

		pointer := *published.Pointer
		status, err := InspectWhileLocked(ctx, root)
		if err != nil {
			_ = lock.Unlock()
			return nil, err
		}

		if status.State == StateHealthy {
			handle, openErr := openPinnedGenerationWithStatus(ctx, root, pointer, status)
			if openErr != nil {
				_ = lock.Unlock()
				return nil, openErr
			}
			if keepLock {
				handle.release = lock.Unlock
			} else {
				_ = lock.Unlock()
			}
			return handle, nil
		}

		// Status is Stale. Check canonical validation.
		issues, valErr := canonical.Validate(root)
		if valErr != nil {
			_ = lock.Unlock()
			return nil, valErr
		}

		if len(issues) > 0 {
			// Canonical validation failed: fall back strictly to published pointer!
			status.ServingMode = ServingFallback
			status.Warning = fmt.Sprintf("canonical validation failed: %s: %s; serving published catalog generation %s as fallback", issues[0].Path, issues[0].Message, pointer.Generation)
			handle, openErr := openPinnedGenerationWithStatus(ctx, root, pointer, status)
			if openErr != nil {
				_ = lock.Unlock()
				return nil, openErr
			}
			if keepLock {
				handle.release = lock.Unlock
			} else {
				_ = lock.Unlock()
			}
			return handle, nil
		}

		// Canonical files are valid, but catalog is stale: rebuild under publication locking!
		_ = lock.Unlock()
		if _, buildErr := BuildCatalogGeneration(ctx, root, BuildOptions{}); buildErr != nil {
			return nil, buildErr
		}
		// Restart observation after rebuild.
	}
	return nil, fmt.Errorf("%w: exceeded %d retry attempts observing fresh catalog generation", ErrCatalogUnavailable, maxOpenRetries)
}

func removePin(path string) {
	_ = os.Remove(path)
}

func findSnapshotGeneration(ctx context.Context, root, catalogSnapshot string) (Pointer, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return Pointer{}, err
	}
	defer rootHandle.Close()
	for _, relative := range []string{"runtime", "runtime/catalog", "runtime/catalog/generations"} {
		info, err := rootHandle.Lstat(relative)
		if err != nil {
			return Pointer{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Pointer{}, fmt.Errorf("unsafe catalog runtime directory: %s", relative)
		}
	}
	directory, err := rootHandle.Open("runtime/catalog/generations")
	if err != nil {
		return Pointer{}, err
	}
	entries, readErr := directory.Readdir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return Pointer{}, readErr
	}
	if closeErr != nil {
		return Pointer{}, closeErr
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".db") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := waitForContext(ctx); err != nil {
			return Pointer{}, err
		}
		generation := strings.TrimSuffix(name, ".db")
		if !validGenerationID(generation) {
			continue
		}
		relative := "runtime/catalog/generations/" + name
		info, err := rootHandle.Lstat(relative)
		if err != nil {
			return Pointer{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Pointer{}, fmt.Errorf("unsafe catalog generation: %s", name)
		}
		pointer, err := readGenerationMetadata(ctx, root, generation)
		if err != nil {
			continue
		}
		if pointer.CatalogSnapshot == catalogSnapshot {
			return pointer, nil
		}
	}
	return Pointer{}, errors.New("exact catalog snapshot is not retained or valid")
}

func readGenerationMetadata(ctx context.Context, root, generation string) (Pointer, error) {
	pointer := Pointer{Generation: generation, Database: "generations/" + generation + ".db"}
	database, err := sql.Open("sqlite", sqliteDSN(generationPath(root, pointer), true))
	if err != nil {
		return Pointer{}, err
	}
	database.SetMaxOpenConns(1)
	var integrity string
	err = database.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity)
	if err == nil && integrity != "ok" {
		err = fmt.Errorf("generation quick check returned %q", integrity)
	}
	if err == nil {
		err = database.QueryRowContext(ctx, `SELECT canonical_schema_version,derived_schema_version,builder_version,catalog_snapshot,projection_input_digest FROM generation_metadata WHERE singleton=1`).Scan(
			&pointer.CanonicalSchemaVersion,
			&pointer.DerivedSchemaVersion,
			&pointer.BuilderVersion,
			&pointer.CatalogSnapshot,
			&pointer.ProjectionInputDigest,
		)
	}
	if closeErr := database.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Pointer{}, err
	}
	if pointer.CanonicalSchemaVersion <= 0 || pointer.DerivedSchemaVersion != DerivedSchemaVersion || pointer.BuilderVersion == "" || pointer.CatalogSnapshot == "" || pointer.ProjectionInputDigest == "" {
		return Pointer{}, errors.New("generation metadata is invalid or incompatible")
	}
	return pointer, nil
}

func verifyPointerDatabase(ctx context.Context, root string, pointer Pointer) error {
	path := generationPath(root, pointer)
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	info, err := rootHandle.Lstat("runtime/catalog/" + pointer.Database)
	_ = rootHandle.Close()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("generation path is not a regular file")
	}
	database, err := sql.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	var integrity string
	if err := database.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		if err != nil {
			return err
		}
		return fmt.Errorf("generation quick check returned %q", integrity)
	}
	var catalogDigest, projectionDigest, builder string
	var canonicalVersion, derivedVersion int
	if err := database.QueryRowContext(ctx, `SELECT canonical_schema_version,derived_schema_version,builder_version,catalog_snapshot,projection_input_digest FROM generation_metadata WHERE singleton=1`).Scan(&canonicalVersion, &derivedVersion, &builder, &catalogDigest, &projectionDigest); err != nil {
		return err
	}
	if canonicalVersion != pointer.CanonicalSchemaVersion || derivedVersion != pointer.DerivedSchemaVersion || builder != pointer.BuilderVersion || catalogDigest != pointer.CatalogSnapshot || projectionDigest != pointer.ProjectionInputDigest {
		return fmt.Errorf("generation metadata does not match pointer")
	}
	return nil
}
