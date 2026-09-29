package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FindGenerationForOperation locates the earliest retained immutable generation
// that contains an applied operation and its exact result snapshot. It is used
// only to replay a faithful idempotent result; it never rebuilds or publishes.
func FindGenerationForOperation(ctx context.Context, root, operationID, catalogSnapshot string) (string, error) {
	if operationID == "" || catalogSnapshot == "" {
		return "", errors.New("operation ID and catalog snapshot are required")
	}
	directory := filepath.Join(root, "runtime", "catalog", "generations")
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rootHandle.Close()
	for _, relative := range []string{"runtime", "runtime/catalog", "runtime/catalog/generations"} {
		info, err := rootHandle.Lstat(relative)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("unsafe catalog runtime directory: %s", relative)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}
	type candidate struct {
		generation string
		modified   time.Time
	}
	var selected *candidate
	for _, entry := range entries {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", fmt.Errorf("unsafe catalog generation: %s", entry.Name())
		}
		path := filepath.Join(directory, entry.Name())
		database, err := sql.Open("sqlite", sqliteDSN(path, true))
		if err != nil {
			return "", err
		}
		database.SetMaxOpenConns(1)
		var found int
		queryErr := database.QueryRowContext(ctx, `SELECT 1 FROM generation_metadata gm JOIN operations op ON op.id=? WHERE gm.singleton=1 AND gm.catalog_snapshot=? AND op.result_catalog_snapshot=? AND op.status='applied' LIMIT 1`, operationID, catalogSnapshot, catalogSnapshot).Scan(&found)
		closeErr := database.Close()
		if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
			return "", queryErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if found == 1 {
			current := candidate{generation: strings.TrimSuffix(entry.Name(), ".db"), modified: info.ModTime()}
			if selected == nil || current.modified.Before(selected.modified) || (current.modified.Equal(selected.modified) && current.generation < selected.generation) {
				selected = &current
			}
		}
	}
	if selected == nil {
		return "", os.ErrNotExist
	}
	return selected.generation, nil
}
