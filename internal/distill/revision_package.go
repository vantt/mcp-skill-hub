package distill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/source"
)

// PackagedResource identifies exact bytes read from either pinned endpoint.
type PackagedResource struct {
	Revision RevisionIdentity `json:"revision"`
	Path     string           `json:"path"`
	Digest   string           `json:"digest"`
	Size     int64            `json:"size"`
	Side     string           `json:"side"`
}

// RevisionPackage is a verified immutable input bundle for an agent/caller.
type RevisionPackage struct {
	Version          int                `json:"version"`
	RunID            string             `json:"run_id"`
	SourceID         string             `json:"source_id"`
	FromRevision     *source.Revision   `json:"from_revision,omitempty"`
	ToRevision       source.Revision    `json:"to_revision"`
	ChangedResources []ChangedResource  `json:"changed_resources"`
	Resources        []PackagedResource `json:"resources"`
	CreatedAt        time.Time          `json:"created_at"`
	Digest           string             `json:"digest"`
}

// CreateRevisionPackage reads real adapter bytes at pinned revisions and publishes an immutable runtime bundle.
func CreateRevisionPackage(ctx context.Context, runtimeRoot string, adapter source.Adapter, src source.Source, runID string, from *source.Revision, to source.Revision, changed []ChangedResource, now time.Time) (RevisionPackage, error) {
	if !safeComponent(runID) || src.ID == "" || adapter == nil {
		return RevisionPackage{}, errors.New("valid run, source, and adapter are required")
	}
	if err := validatePackageScope(changed); err != nil {
		return RevisionPackage{}, err
	}
	stagingRoot := filepath.Join(runtimeRoot, "runtime", "distill", "packages")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return RevisionPackage{}, err
	}
	staging, err := os.MkdirTemp(stagingRoot, ".package-")
	if err != nil {
		return RevisionPackage{}, err
	}
	defer os.RemoveAll(staging)
	pkg := RevisionPackage{Version: 1, RunID: runID, SourceID: src.ID, FromRevision: from, ToRevision: to, ChangedResources: append([]ChangedResource(nil), changed...), CreatedAt: now.UTC()}
	for _, change := range changed {
		if err := ctx.Err(); err != nil {
			return RevisionPackage{}, err
		}
		revision, side := to, "to"
		if change.Status == "deleted" {
			if from == nil {
				return RevisionPackage{}, fmt.Errorf("deleted resource %q has no from revision", change.Path)
			}
			revision, side = *from, "from"
		}
		contents, err := adapter.Read(ctx, src, revision, change.Path)
		if err != nil {
			return RevisionPackage{}, fmt.Errorf("read %s at pinned %s revision: %w", change.Path, side, err)
		}
		digest := source.Digest(contents)
		relative := resourceFile(side, change.Path)
		stagingHandle, openErr := os.OpenRoot(staging)
		if openErr != nil {
			return RevisionPackage{}, openErr
		}
		if err = stagingHandle.MkdirAll(filepath.ToSlash(filepath.Dir(relative)), 0o700); err == nil {
			err = stagingHandle.WriteFile(relative, contents, 0o400)
		}
		closeErr := stagingHandle.Close()
		if err != nil {
			return RevisionPackage{}, err
		}
		if closeErr != nil {
			return RevisionPackage{}, closeErr
		}
		pkg.Resources = append(pkg.Resources, PackagedResource{Revision: IdentityOf(revision), Path: change.Path, Digest: digest, Size: int64(len(contents)), Side: side})
	}
	sort.Slice(pkg.ChangedResources, func(i, j int) bool { return pkg.ChangedResources[i].Path < pkg.ChangedResources[j].Path })
	sort.Slice(pkg.Resources, func(i, j int) bool {
		if pkg.Resources[i].Side == pkg.Resources[j].Side {
			return pkg.Resources[i].Path < pkg.Resources[j].Path
		}
		return pkg.Resources[i].Side < pkg.Resources[j].Side
	})
	pkg.Digest, err = packageDigest(pkg)
	if err != nil {
		return RevisionPackage{}, err
	}
	manifest, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return RevisionPackage{}, err
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), append(manifest, '\n'), 0o400); err != nil {
		return RevisionPackage{}, err
	}
	if err := makeReadOnly(staging); err != nil {
		return RevisionPackage{}, err
	}
	final := filepath.Join(stagingRoot, runID)
	if _, statErr := os.Lstat(final); statErr == nil {
		existing, loadErr := LoadRevisionPackage(runtimeRoot, runID)
		if loadErr != nil || existing.Digest != pkg.Digest {
			return RevisionPackage{}, errors.New("revision package identity collision")
		}
		return existing, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return RevisionPackage{}, statErr
	}
	if err := os.Rename(staging, final); err != nil {
		return RevisionPackage{}, err
	}
	return pkg, nil
}

// LoadRevisionPackage verifies manifest identity and every packaged byte before returning it.
func LoadRevisionPackage(runtimeRoot, runID string) (RevisionPackage, error) {
	if !safeComponent(runID) {
		return RevisionPackage{}, errors.New("invalid run ID")
	}
	root := filepath.Join(runtimeRoot, "runtime", "distill", "packages", runID)
	if info, err := os.Lstat(root); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err != nil {
			return RevisionPackage{}, err
		}
		return RevisionPackage{}, errors.New("unsafe revision package directory")
	}
	manifest, err := readPackageFile(root, "manifest.json")
	if err != nil {
		return RevisionPackage{}, err
	}
	var pkg RevisionPackage
	decoder := json.NewDecoder(strings.NewReader(string(manifest)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pkg); err != nil {
		return RevisionPackage{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return RevisionPackage{}, errors.New("revision package manifest must contain one JSON object")
	}
	if pkg.Version != 1 || pkg.RunID != runID || pkg.SourceID == "" {
		return RevisionPackage{}, errors.New("invalid revision package manifest")
	}
	if err := validatePackageScope(pkg.ChangedResources); err != nil {
		return RevisionPackage{}, err
	}
	if err := validateRevision(pkg.ToRevision); err != nil {
		return RevisionPackage{}, errors.New("invalid revision package target revision")
	}
	if pkg.FromRevision != nil {
		if err := validateRevision(*pkg.FromRevision); err != nil {
			return RevisionPackage{}, errors.New("invalid revision package base revision")
		}
	}
	seen := map[string]bool{}
	for _, item := range pkg.Resources {
		key := item.Side + "\x00" + item.Path
		if seen[key] || (item.Side != "from" && item.Side != "to") || !safeResourcePath(item.Path) || item.Size < 0 || item.Revision.Value == "" || item.Revision.Kind == "" || item.Revision.ContentDigest == "" {
			return RevisionPackage{}, errors.New("invalid revision package resource")
		}
		seen[key] = true
		expectedRevision := IdentityOf(pkg.ToRevision)
		if item.Side == "from" {
			if pkg.FromRevision == nil {
				return RevisionPackage{}, errors.New("from resource has no package from revision")
			}
			expectedRevision = IdentityOf(*pkg.FromRevision)
		}
		if item.Revision != expectedRevision {
			return RevisionPackage{}, errors.New("revision package resource identity mismatch")
		}
		contents, readErr := readPackageFile(root, resourceFile(item.Side, item.Path))
		if readErr != nil {
			return RevisionPackage{}, readErr
		}
		if int64(len(contents)) != item.Size || source.Digest(contents) != item.Digest {
			return RevisionPackage{}, fmt.Errorf("revision package resource %s failed digest validation", item.Path)
		}
	}
	expected, err := packageDigest(pkg)
	if err != nil {
		return RevisionPackage{}, err
	}
	if expected != pkg.Digest {
		return RevisionPackage{}, errors.New("revision package manifest digest mismatch")
	}
	return pkg, nil
}

// ReadEvidence returns verified bytes from the exact package side named by revision.
func ReadEvidence(runtimeRoot string, pkg RevisionPackage, revision RevisionIdentity, path string) ([]byte, error) {
	for _, item := range pkg.Resources {
		if item.Revision == revision && item.Path == path {
			root := filepath.Join(runtimeRoot, "runtime", "distill", "packages", pkg.RunID)
			contents, err := readPackageFile(root, resourceFile(item.Side, item.Path))
			if err != nil {
				return nil, err
			}
			if source.Digest(contents) != item.Digest {
				return nil, errors.New("packaged evidence digest mismatch")
			}
			return contents, nil
		}
	}
	return nil, os.ErrNotExist
}

func packageDigest(pkg RevisionPackage) (string, error) {
	copy := pkg
	copy.Digest = ""
	encoded, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
func resourceFile(side, path string) string { return "resources/" + side + "/" + path }
func safeResourcePath(value string) bool {
	if value == "" || value != filepath.ToSlash(filepath.Clean(filepath.FromSlash(value))) || filepath.IsAbs(filepath.FromSlash(value)) || strings.Contains(value, `\\`) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validatePackageScope(changed []ChangedResource) error {
	seen := map[string]bool{}
	for _, change := range changed {
		if !safeResourcePath(change.Path) || seen[change.Path] {
			return fmt.Errorf("unsafe or duplicate changed resource path %q", change.Path)
		}
		seen[change.Path] = true
		switch change.Status {
		case "added", "modified", "deleted":
		default:
			return fmt.Errorf("invalid changed resource status %q", change.Status)
		}
	}
	return nil
}
func readPackageFile(root, relative string) ([]byte, error) {
	if !safeResourcePath(relative) {
		return nil, errors.New("unsafe revision package path")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Lstat(filepath.ToSlash(relative))
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("unsafe revision package file")
	}
	return handle.ReadFile(filepath.ToSlash(relative))
}
func safeComponent(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func makeReadOnly(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return os.Chmod(path, 0o400)
	})
}
