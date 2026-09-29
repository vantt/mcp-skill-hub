package skill

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
)

// Resource describes a digest-pinned skill file in one catalog snapshot.
type Resource struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
}

// Manifest is the immutable resource view returned before content loading.
type Manifest struct {
	SkillID         string     `json:"skill_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	CatalogSnapshot string     `json:"catalog_snapshot"`
	Resources       []Resource `json:"resources"`
}

// GetManifest returns only active skills by default because draft, deprecated,
// and archived skills are curation state rather than distributable procedures.
func GetManifest(ctx context.Context, root, id string) (Manifest, error) {
	if !idPattern.MatchString(id) {
		return Manifest{}, errors.New("skill id must be a lowercase kebab-case identifier")
	}
	handle, err := catalog.OpenCurrent(ctx, root)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrSnapshotExpired, err)
	}
	defer handle.Close()
	manifest := Manifest{SkillID: id, CatalogSnapshot: handle.Pointer.CatalogSnapshot, Resources: []Resource{}}
	if err := handle.DB.QueryRowContext(ctx, `SELECT name,description,status FROM skills WHERE id=?`, id).Scan(&manifest.Name, &manifest.Description, &manifest.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Manifest{}, ErrNotFound
		}
		return Manifest{}, err
	}
	if manifest.Status != "active" {
		return Manifest{}, ErrNotFound
	}
	rows, err := handle.DB.QueryContext(ctx, `SELECT path,kind,digest,size_bytes FROM resources WHERE skill_id=? ORDER BY path`, id)
	if err != nil {
		return Manifest{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Resource
		if err := rows.Scan(&item.Path, &item.Kind, &item.Digest, &item.SizeBytes); err != nil {
			return Manifest{}, err
		}
		manifest.Resources = append(manifest.Resources, item)
	}
	if err := rows.Err(); err != nil {
		return Manifest{}, err
	}
	sort.Slice(manifest.Resources, func(i, j int) bool { return manifest.Resources[i].Path < manifest.Resources[j].Path })
	return manifest, nil
}

// ReadResource serves canonical bytes only when the current generation, path,
// and digest all match the caller's manifest pins. Old bytes are never guessed.
func ReadResource(ctx context.Context, root, snapshot, path, expectedDigest string) ([]byte, error) {
	if snapshot == "" || expectedDigest == "" {
		return nil, errors.New("catalog snapshot and resource digest pins are required")
	}
	if !safeResourcePath(path) {
		return nil, errors.New("resource path is invalid")
	}
	handle, err := catalog.OpenCurrent(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSnapshotExpired, err)
	}
	defer handle.Close()
	if handle.Pointer.CatalogSnapshot != snapshot {
		return nil, ErrSnapshotExpired
	}
	var catalogDigest string
	if err := handle.DB.QueryRowContext(ctx, `SELECT digest FROM resources WHERE path=?`, path).Scan(&catalogDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSnapshotExpired
		}
		return nil, err
	}
	if catalogDigest != expectedDigest {
		return nil, ErrSnapshotExpired
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootHandle.Close()
	info, err := rootHandle.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, ErrSnapshotExpired
	}
	contents, err := rootHandle.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(contents)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != expectedDigest {
		return nil, ErrResourceDigestMismatch
	}
	return contents, nil
}

func safeResourcePath(path string) bool {
	if path == "" || filepath.IsAbs(filepath.FromSlash(path)) || strings.Contains(path, `\`) || !strings.HasPrefix(path, "skills/") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean == path && !strings.Contains(path, "../")
}
