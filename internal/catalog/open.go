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
		return Status{State: StateMissing, Detail: "catalog generation pointer is missing"}, nil
	}
	if err != nil {
		return Status{State: StateCorrupt, Detail: err.Error()}, nil
	}
	if pointer.DerivedSchemaVersion != DerivedSchemaVersion {
		return Status{State: StateIncompatible, Pointer: &pointer, Detail: "derived schema version is incompatible"}, nil
	}
	if err := verifyPointerDatabase(ctx, root, pointer); err != nil {
		return Status{State: StateCorrupt, Pointer: &pointer, Detail: err.Error()}, nil
	}
	return Status{State: StateUnknown, Pointer: &pointer, Detail: "canonical freshness was not inspected while workspace recovery is pending"}, nil
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
		return Status{State: StateStale, Pointer: &pointer, Detail: "canonical inputs differ from the published generation"}, nil
	}
	return Status{State: StateHealthy, Pointer: &pointer}, nil
}

// OpenCurrent pins and opens the current immutable generation read-only.
func OpenCurrent(ctx context.Context, root string) (*Handle, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()
	status, err := InspectWhileLocked(ctx, root)
	if err != nil {
		return nil, err
	}
	if status.State != StateHealthy || status.Pointer == nil {
		return nil, fmt.Errorf("catalog is %s: %s", status.State, status.Detail)
	}
	pointer := *status.Pointer
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
		_ = os.Remove(pinPath)
		return nil, marshalErr
	}
	if err := pin.Close(); err != nil {
		_ = os.Remove(pinPath)
		return nil, err
	}
	database, err := sql.Open("sqlite", sqliteDSN(generationPath(root, pointer), true))
	if err != nil {
		_ = os.Remove(pinPath)
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		_ = os.Remove(pinPath)
		return nil, err
	}
	return &Handle{DB: database, Pointer: pointer, pinPath: pinPath}, nil
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
