package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const filesystemMaxTimeout = DefaultTimeout

var filesystemCacheGate = func() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}()

// FilesystemAdapter captures bounded immutable snapshots of a subtree beneath
// an explicitly configured root. Revisions refer to cached bytes rather than a
// moving live directory.
type FilesystemAdapter struct {
	Root        string
	CacheRoot   string
	MaxFiles    int
	MaxBytes    int64
	MaxFileSize int64
	Timeout     time.Duration
	Now         func() time.Time
}

type filesystemManifest struct {
	Version   int        `json:"version"`
	Digest    string     `json:"digest"`
	CreatedAt time.Time  `json:"created_at"`
	Resources []Resource `json:"resources"`
}

func (adapter FilesystemAdapter) defaults() FilesystemAdapter {
	if adapter.MaxFiles <= 0 {
		adapter.MaxFiles = DefaultMaxFiles
	}
	if adapter.MaxBytes <= 0 {
		adapter.MaxBytes = DefaultMaxBytes
	}
	if adapter.MaxFileSize <= 0 {
		adapter.MaxFileSize = DefaultMaxFileSize
	}
	if adapter.Timeout <= 0 || adapter.Timeout > filesystemMaxTimeout {
		adapter.Timeout = filesystemMaxTimeout
	}
	if adapter.Now == nil {
		adapter.Now = time.Now
	}
	if adapter.CacheRoot == "" && adapter.Root != "" {
		adapter.CacheRoot = filepath.Join(adapter.Root, "runtime", "sources", "filesystem")
	}
	return adapter
}

func (adapter FilesystemAdapter) Identify(ctx context.Context, locator Locator) (Identity, error) {
	adapter = adapter.defaults()
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()
	root, err := adapter.resolve(locator.Path)
	if err != nil {
		return Identity{}, err
	}
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return Identity{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Identity{}, fmt.Errorf("%w: filesystem source must be a directory", ErrInvalidLocator)
	}
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	return Identity{Name: filepath.Base(root), Canonical: filepath.ToSlash(locator.Path), Path: "."}, nil
}

func (adapter FilesystemAdapter) CurrentRevision(ctx context.Context, source Source) (Revision, error) {
	adapter = adapter.withLimits(source.Limits)
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()

	if source.Locator.SnapshotDigest != "" {
		rev := Revision{
			Kind:          "filesystem-snapshot",
			Value:         source.Locator.SnapshotDigest,
			ContentDigest: source.Locator.SnapshotDigest,
			ObservedAt:    adapter.Now().UTC(),
		}
		manifest, err := adapter.loadManifest(ctx, rev)
		if err != nil {
			return Revision{}, err
		}
		return Revision{
			Kind:          "filesystem-snapshot",
			Value:         manifest.Digest,
			ContentDigest: manifest.Digest,
			ObservedAt:    manifest.CreatedAt,
		}, nil
	}

	manifest, err := adapter.capture(ctx, source.Locator.Path)
	if err != nil {
		return Revision{}, err
	}
	if err := ctx.Err(); err != nil {
		return Revision{}, err
	}
	return Revision{Kind: "filesystem-snapshot", Value: manifest.Digest, ContentDigest: manifest.Digest, ObservedAt: adapter.Now().UTC()}, nil
}

func (adapter FilesystemAdapter) Diff(ctx context.Context, source Source, from, to Revision) (ChangeSet, error) {
	adapter = adapter.withLimits(source.Limits)
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return ChangeSet{}, err
	}
	if from.ContentDigest == to.ContentDigest {
		return ChangeSet{From: from, To: to, Changes: []Change{}}, nil
	}
	before, err := adapter.loadManifest(ctx, from)
	if err != nil {
		return ChangeSet{}, err
	}
	after, err := adapter.loadManifest(ctx, to)
	if err != nil {
		return ChangeSet{}, err
	}
	beforeMap := make(map[string]int64, len(before.Resources))
	afterMap := make(map[string]int64, len(after.Resources))
	for _, item := range before.Resources {
		beforeMap[item.Path] = item.Size
	}
	for _, item := range after.Resources {
		afterMap[item.Path] = item.Size
	}
	paths := make(map[string]struct{}, len(beforeMap)+len(afterMap))
	for name := range beforeMap {
		paths[name] = struct{}{}
	}
	for name := range afterMap {
		paths[name] = struct{}{}
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	changes := make([]Change, 0)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return ChangeSet{}, err
		}
		_, oldOK := beforeMap[name]
		_, newOK := afterMap[name]
		status := ""
		switch {
		case !oldOK:
			status = "added"
		case !newOK:
			status = "deleted"
		default:
			oldBytes, oldErr := adapter.readSnapshotFile(from.ContentDigest, name)
			if oldErr != nil {
				return ChangeSet{}, oldErr
			}
			newBytes, newErr := adapter.readSnapshotFile(to.ContentDigest, name)
			if newErr != nil {
				return ChangeSet{}, newErr
			}
			if Digest(oldBytes) != Digest(newBytes) {
				status = "modified"
			}
		}
		if status != "" {
			changes = append(changes, Change{Path: name, Status: status})
		}
	}
	if err := ctx.Err(); err != nil {
		return ChangeSet{}, err
	}
	return ChangeSet{From: from, To: to, Changes: changes}, nil
}

func (adapter FilesystemAdapter) Read(ctx context.Context, source Source, revision Revision, resourcePath string) ([]byte, error) {
	adapter = adapter.withLimits(source.Limits)
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()
	if !safeResourcePath(resourcePath) {
		return nil, ErrInvalidLocator
	}
	manifest, err := adapter.loadManifest(ctx, revision)
	if err != nil {
		return nil, err
	}
	found := false
	for _, item := range manifest.Resources {
		if item.Path == resourcePath {
			if item.Size > adapter.MaxFileSize {
				return nil, &LimitExceededError{Limit: "file_size", Actual: item.Size, Max: adapter.MaxFileSize, Path: item.Path}
			}
			found = true
			break
		}
	}
	if !found {
		return nil, os.ErrNotExist
	}
	contents, err := adapter.readSnapshotFile(revision.ContentDigest, resourcePath)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return contents, nil
}

func (adapter FilesystemAdapter) List(ctx context.Context, source Source, revision Revision, scope Scope) ([]Resource, error) {
	adapter = adapter.withLimits(source.Limits)
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()
	if scope.Prefix != "" && !safeResourcePath(scope.Prefix) {
		return nil, ErrInvalidLocator
	}
	manifest, err := adapter.loadManifest(ctx, revision)
	if err != nil {
		return nil, err
	}
	resources := make([]Resource, 0, len(manifest.Resources))
	var total int64
	for _, item := range manifest.Resources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if scope.Prefix != "" && item.Path != scope.Prefix && !strings.HasPrefix(item.Path, strings.TrimSuffix(scope.Prefix, "/")+"/") {
			continue
		}
		total += item.Size
		if len(resources) >= adapter.MaxFiles {
			return nil, &LimitExceededError{Limit: "files", Actual: int64(len(resources) + 1), Max: int64(adapter.MaxFiles)}
		}
		if item.Size > adapter.MaxFileSize {
			return nil, &LimitExceededError{Limit: "file_size", Actual: item.Size, Max: adapter.MaxFileSize, Path: item.Path}
		}
		if total > adapter.MaxBytes {
			return nil, &LimitExceededError{Limit: "bytes", Actual: total, Max: adapter.MaxBytes}
		}
		resources = append(resources, item)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return resources, nil
}

func (adapter FilesystemAdapter) withLimits(limits Limits) FilesystemAdapter {
	adapter = adapter.defaults()
	if limits.MaxFiles > 0 && limits.MaxFiles < adapter.MaxFiles {
		adapter.MaxFiles = limits.MaxFiles
	}
	if limits.MaxBytes > 0 && limits.MaxBytes < adapter.MaxBytes {
		adapter.MaxBytes = limits.MaxBytes
	}
	if limits.MaxFileBytes > 0 && limits.MaxFileBytes < adapter.MaxFileSize {
		adapter.MaxFileSize = limits.MaxFileBytes
	}
	const maxDurationSeconds = int64((time.Duration(1<<63 - 1)) / time.Second)
	if seconds := int64(limits.TimeoutSeconds); seconds > 0 && seconds <= maxDurationSeconds {
		configured := time.Duration(seconds) * time.Second
		if configured < adapter.Timeout {
			adapter.Timeout = configured
		}
	}
	return adapter
}

func (adapter FilesystemAdapter) resolve(relative string) (string, error) {
	if adapter.Root == "" || !safeResourcePath(relative) {
		return "", ErrInvalidLocator
	}
	base, err := filepath.Abs(adapter.Root)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(base); statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if statErr != nil {
			return "", statErr
		}
		return "", ErrInvalidLocator
	}
	result := filepath.Join(base, filepath.FromSlash(relative))
	rel, err := filepath.Rel(base, result)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidLocator
	}
	current := base
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe source symlink: %s", component)
		}
	}
	return result, nil
}

// CaptureAuthorizedRoot captures a bounded immutable snapshot of an authorized directory
// into the workspace-private cache root and returns the manifest.
func (adapter FilesystemAdapter) CaptureAuthorizedRoot(ctx context.Context, authRoot AuthorizedLocalRoot, subpath string) (filesystemManifest, error) {
	adapter = adapter.defaults()
	root := authRoot.Path()
	if root == "" {
		return filesystemManifest{}, ErrInvalidLocator
	}
	if subpath != "" && subpath != "." {
		if !safeResourcePath(subpath) {
			return filesystemManifest{}, ErrInvalidLocator
		}
		root = filepath.Join(root, filepath.FromSlash(subpath))
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			return filesystemManifest{}, ErrInvalidLocator
		}
	}
	return adapter.captureDirectory(ctx, root)
}

func (adapter FilesystemAdapter) capture(ctx context.Context, relative string) (filesystemManifest, error) {
	root, err := adapter.resolve(relative)
	if err != nil {
		return filesystemManifest{}, err
	}
	return adapter.captureDirectory(ctx, root)
}

func (adapter FilesystemAdapter) captureDirectory(ctx context.Context, root string) (filesystemManifest, error) {
	cacheRoot, err := filepath.Abs(adapter.CacheRoot)
	if err != nil {
		return filesystemManifest{}, err
	}
	sourceRoot, err := filepath.Abs(root)
	if err != nil {
		return filesystemManifest{}, err
	}
	if rel, relErr := filepath.Rel(sourceRoot, cacheRoot); relErr == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return filesystemManifest{}, fmt.Errorf("%w: filesystem cache must be outside the source subtree", ErrInvalidLocator)
	}
	if rel, relErr := filepath.Rel(cacheRoot, sourceRoot); relErr == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return filesystemManifest{}, fmt.Errorf("%w: source root cannot be inside the filesystem cache", ErrInvalidLocator)
	}
	select {
	case <-ctx.Done():
		return filesystemManifest{}, ctx.Err()
	case <-filesystemCacheGate:
	}
	defer func() { filesystemCacheGate <- struct{}{} }()
	if err := ensureCacheDirectory(cacheRoot); err != nil {
		return filesystemManifest{}, err
	}
	staging, err := os.MkdirTemp(cacheRoot, ".capture-")
	if err != nil {
		return filesystemManifest{}, err
	}
	defer os.RemoveAll(staging)
	if err := os.Mkdir(filepath.Join(staging, "files"), 0o700); err != nil {
		return filesystemManifest{}, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return filesystemManifest{}, err
	}
	defer handle.Close()
	resources := make([]Resource, 0)
	hash := sha256.New()
	var total int64
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == root {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := handle.Lstat(rel)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: unsafe source symlink: %s", ErrUnsafeFile, rel)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: unsupported source object: %s", ErrUnsafeFile, rel)
		}
		if len(resources) >= adapter.MaxFiles {
			return &LimitExceededError{Limit: "files", Actual: int64(len(resources) + 1), Max: int64(adapter.MaxFiles)}
		}
		if info.Size() > adapter.MaxFileSize {
			return &LimitExceededError{Limit: "file_size", Actual: info.Size(), Max: adapter.MaxFileSize, Path: rel}
		}
		contents, err := readRootRegularFile(root, rel, adapter.MaxFileSize)
		if err != nil {
			return err
		}
		total += int64(len(contents))
		if total > adapter.MaxBytes {
			return &LimitExceededError{Limit: "bytes", Actual: total, Max: adapter.MaxBytes}
		}
		if err := writeSnapshotFile(staging, rel, contents); err != nil {
			return err
		}
		resources = append(resources, Resource{Path: rel, Size: int64(len(contents))})
		fmt.Fprintf(hash, "%s\x00%d\x00", rel, len(contents))
		hash.Write(contents)
		hash.Write([]byte{'\n'})
		return nil
	})
	if err != nil {
		return filesystemManifest{}, err
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })
	// Walk order is platform-defined; recompute the identity in sorted order.
	hash.Reset()
	for _, item := range resources {
		if err := ctx.Err(); err != nil {
			return filesystemManifest{}, err
		}
		contents, readErr := os.ReadFile(filepath.Join(staging, "files", filepath.FromSlash(item.Path)))
		if readErr != nil {
			return filesystemManifest{}, readErr
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", item.Path, len(contents))
		hash.Write(contents)
		hash.Write([]byte{'\n'})
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	manifest := filesystemManifest{Version: 1, Digest: digest, CreatedAt: adapter.Now().UTC(), Resources: resources}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return filesystemManifest{}, err
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), append(manifestBytes, '\n'), 0o600); err != nil {
		return filesystemManifest{}, err
	}
	final := adapter.snapshotPath(digest)
	if _, err := os.Lstat(final); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(staging, final); err != nil {
			return filesystemManifest{}, err
		}
	} else if err != nil {
		return filesystemManifest{}, err
	}
	if err := adapter.pruneCache(ctx, final); err != nil {
		return filesystemManifest{}, err
	}
	return manifest, nil
}

func ensureCacheDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("unsafe filesystem snapshot cache")
	}
	return os.Chmod(path, 0o700)
}

func writeSnapshotFile(staging, relative string, contents []byte) error {
	if !safeResourcePath(relative) {
		return ErrInvalidLocator
	}
	path := filepath.Join(staging, "files", filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o600)
}

func readRootRegularFile(rootPath, relative string, maximum int64) ([]byte, error) {
	file, err := openNoFollow(rootPath, relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || opened.Size() > maximum {
		return nil, &LimitExceededError{Limit: "file_size", Actual: opened.Size(), Max: maximum, Path: relative}
	}
	contents, err := boundedRead(file, maximum)
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) != opened.Size() {
		return nil, ErrRevisionMismatch
	}
	return contents, nil
}

func (adapter FilesystemAdapter) snapshotPath(digest string) string {
	return filepath.Join(adapter.CacheRoot, strings.TrimPrefix(digest, "sha256:"))
}

func (adapter FilesystemAdapter) loadManifest(ctx context.Context, revision Revision) (filesystemManifest, error) {
	if err := ctx.Err(); err != nil {
		return filesystemManifest{}, err
	}
	if revision.Kind != "filesystem-snapshot" || revision.Value != revision.ContentDigest || !validDigest(revision.ContentDigest) {
		return filesystemManifest{}, ErrRevisionMismatch
	}
	path := adapter.snapshotPath(revision.ContentDigest)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return filesystemManifest{}, ErrHistoryUnavailable
	}
	if err != nil {
		return filesystemManifest{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return filesystemManifest{}, ErrHistoryUnavailable
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return filesystemManifest{}, ErrHistoryUnavailable
	}
	defer root.Close()
	data, err := readRootRegularFile(path, "manifest.json", 4<<20)
	if err != nil {
		return filesystemManifest{}, ErrHistoryUnavailable
	}
	var manifest filesystemManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.Version != 1 || manifest.Digest != revision.ContentDigest {
		return filesystemManifest{}, ErrHistoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return filesystemManifest{}, err
	}
	if len(manifest.Resources) > adapter.MaxFiles {
		return filesystemManifest{}, &LimitExceededError{Limit: "files", Actual: int64(len(manifest.Resources)), Max: int64(adapter.MaxFiles)}
	}
	var total int64
	for _, item := range manifest.Resources {
		if err := ctx.Err(); err != nil {
			return filesystemManifest{}, err
		}
		if !safeResourcePath(item.Path) || item.Size < 0 || item.Size > adapter.MaxFileSize {
			return filesystemManifest{}, ErrHistoryUnavailable
		}
		total += item.Size
		if total > adapter.MaxBytes {
			return filesystemManifest{}, &LimitExceededError{Limit: "bytes", Actual: total, Max: adapter.MaxBytes}
		}
	}
	return manifest, nil
}

func (adapter FilesystemAdapter) readSnapshotFile(digest, relative string) ([]byte, error) {
	path := adapter.snapshotPath(digest)
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrHistoryUnavailable
	}
	defer root.Close()
	contents, err := readRootRegularFile(path, "files/"+relative, adapter.MaxFileSize)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrHistoryUnavailable
		}
		return nil, err
	}
	return contents, nil
}

func (adapter FilesystemAdapter) pruneCache(ctx context.Context, keep string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(adapter.CacheRoot)
	if err != nil {
		return err
	}
	type cached struct {
		path     string
		modified time.Time
		size     int64
	}
	items := make([]cached, 0)
	var total int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".capture-") {
			continue
		}
		path := filepath.Join(adapter.CacheRoot, entry.Name())
		var size int64
		if err := filepath.WalkDir(path, func(_ string, child fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if child.Type()&os.ModeSymlink != 0 {
				return errors.New("unsafe filesystem snapshot cache entry")
			}
			if child.IsDir() {
				return nil
			}
			info, err := child.Info()
			if err != nil {
				return err
			}
			size += info.Size()
			return nil
		}); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		items = append(items, cached{path: path, modified: info.ModTime(), size: size})
		total += size
	}
	limit := adapter.MaxBytes * 4
	if limit < adapter.MaxBytes {
		limit = adapter.MaxBytes
	}
	sort.Slice(items, func(i, j int) bool { return items[i].modified.Before(items[j].modified) })
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if total <= limit {
			break
		}
		if item.path == keep {
			continue
		}
		if err := os.RemoveAll(item.path); err != nil {
			return err
		}
		total -= item.size
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && strings.ToLower(value) == value
}
