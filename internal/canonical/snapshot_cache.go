package canonical

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// racyWindow is how long before its hash a file must have last been modified for
// its stat stamp alone to prove the contents are unchanged. A write that lands in
// the same timestamp tick as the hashed version, on a filesystem with a coarse
// clock (one or two seconds on some), or with a modest clock skew between a
// network file server and this host, keeps the modification time unchanged. Such
// a "racy" file is rehashed on every scan until its modification time falls
// outside the window, the same rule Git applies to racily clean index entries.
const racyWindow = 5 * time.Second

// maxCachedRoots bounds the process-wide cache; a server normally scans one workspace.
const maxCachedRoots = 16

// fileStamp is the cheap identity of one canonical file. Any write, replacement,
// rename onto the path, or metadata restore changes at least one field, except a
// write that leaves the modification time unchanged, which the racy window covers.
type fileStamp struct {
	size       int64
	mode       fs.FileMode
	modTime    int64  // nanoseconds since the Unix epoch
	changeTime int64  // status change time in nanoseconds; 0 where the platform has none
	device     uint64 // 0 where the platform has none
	inode      uint64 // 0 where the platform has none
}

// stampedDigest records the content digest observed for a stamp and the wall
// clock reading taken before that stamp and those contents were read.
type stampedDigest struct {
	stamp    fileStamp
	digest   string
	hashedAt int64
}

// trusted reports whether an unchanged stamp proves the digest is still current:
// the file was last modified clearly before its contents were hashed, so any later
// write must have moved its modification or change time.
func (entry stampedDigest) trusted() bool {
	return entry.stamp.modTime < entry.hashedAt-int64(racyWindow)
}

// cachedTree is one workspace's digests and the snapshot they produced. It is
// replaced as a whole and never mutated, so readers need no lock once loaded.
type cachedTree struct {
	entries  map[string]stampedDigest
	snapshot Snapshot
}

// reusable returns the cached digest for a path when its stamp proves it current.
func (tree *cachedTree) reusable(path string, stamp fileStamp) (stampedDigest, bool) {
	if tree == nil {
		return stampedDigest{}, false
	}
	entry, ok := tree.entries[path]
	if !ok || entry.stamp != stamp || !entry.trusted() {
		return stampedDigest{}, false
	}
	return entry, true
}

// snapshotCache remembers content digests per workspace so repeated scans read
// only files whose stamp changed or is too recent to trust.
type snapshotCache struct {
	mu    sync.Mutex
	roots map[string]*cachedTree
	now   func() time.Time
}

var processSnapshotCache = &snapshotCache{now: time.Now}

// ScanCached returns exactly the Snapshot that Scan returns for the same tree. It
// lists the canonical files on every call, then reuses a digest computed by an
// earlier call in this process when the file's size, mode, modification time,
// change time and file identity are all unchanged and the file was not modified
// within racyWindow of being hashed. Every other file is read and hashed twice
// around a second listing, as Scan does. With no recent changes the cost depends
// on the number of files, not on their contents.
//
// A snapshot built only from reused digests describes the tree at the start of
// the call: each file kept its hashed contents from its last modification, before
// that instant, until it was stamped, after it.
func ScanCached(root string) (Snapshot, error) {
	return processSnapshotCache.scan(root, nil)
}

func (cache *snapshotCache) scan(root string, beforeRecheck func()) (Snapshot, error) {
	key, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve canonical workspace: %w", err)
	}
	startedAt := cache.now().UnixNano()
	paths, stamps, err := stampTree(root)
	if err != nil {
		return Snapshot{}, err
	}
	previous := cache.load(key)
	files := make([]FileDigest, len(paths))
	var pending []int
	for index, path := range paths {
		if entry, ok := previous.reusable(path, stamps[index]); ok {
			files[index] = FileDigest{Path: path, Digest: entry.digest}
			continue
		}
		pending = append(pending, index)
	}
	// With nothing pending every listed path is a cached one, so equal counts mean
	// the path set is unchanged too and the cached snapshot is current.
	if previous != nil && len(pending) == 0 && len(previous.entries) == len(paths) {
		snapshot := previous.snapshot
		snapshot.Files = files
		return snapshot, nil
	}
	// A file was added, removed, renamed or must be read: observe the tree twice,
	// like Scan does, before trusting what was read.
	first, err := hashPending(root, paths, pending)
	if err != nil {
		return Snapshot{}, err
	}
	if beforeRecheck != nil {
		beforeRecheck()
	}
	recheckPaths, recheckStamps, err := stampTree(root)
	if err != nil {
		return Snapshot{}, err
	}
	if !sameStampedTree(paths, stamps, recheckPaths, recheckStamps) {
		return Snapshot{}, errChangedDuringScan
	}
	second, err := hashPending(root, paths, pending)
	if err != nil {
		return Snapshot{}, err
	}
	entries := make(map[string]stampedDigest, len(paths))
	for index, path := range paths {
		if entry, ok := previous.reusable(path, stamps[index]); ok {
			entries[path] = entry
		}
	}
	for position, index := range pending {
		if first[position] != second[position] {
			return Snapshot{}, errChangedDuringScan
		}
		files[index] = FileDigest{Path: paths[index], Digest: first[position]}
		entries[paths[index]] = stampedDigest{stamp: stamps[index], digest: first[position], hashedAt: startedAt}
	}
	snapshot := newSnapshot(files)
	cached := snapshot
	cached.Files = nil // callers own the returned slice; the cache keeps only the digests
	cache.store(key, &cachedTree{entries: entries, snapshot: cached})
	return snapshot, nil
}

func (cache *snapshotCache) load(key string) *cachedTree {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.roots[key]
}

func (cache *snapshotCache) store(key string, tree *cachedTree) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.roots == nil {
		cache.roots = make(map[string]*cachedTree)
	}
	if _, exists := cache.roots[key]; !exists && len(cache.roots) >= maxCachedRoots {
		clear(cache.roots)
	}
	cache.roots[key] = tree
}

// stampTree lists canonical files with the same rules and limits as Scan and
// stamps each one without reading its contents.
func stampTree(root string) ([]string, []fileStamp, error) {
	paths, err := workspace.RelativeFiles(root)
	if err != nil {
		return nil, nil, err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, fmt.Errorf("open canonical workspace: %w", err)
	}
	defer rootHandle.Close()
	stamps := make([]fileStamp, len(paths))
	for index, relative := range paths {
		info, err := rootHandle.Lstat(relative)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect canonical input %s: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("canonical input %s is not a regular file", relative)
		}
		if info.Size() > workspace.MaxCanonicalFileBytesV1 {
			return nil, nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, workspace.MaxCanonicalFileBytesV1)
		}
		stamp := fileStamp{size: info.Size(), mode: info.Mode(), modTime: info.ModTime().UnixNano()}
		if err := addPlatformStamp(&stamp, rootHandle, relative, info); err != nil {
			return nil, nil, fmt.Errorf("inspect canonical input %s: %w", relative, err)
		}
		stamps[index] = stamp
	}
	return paths, stamps, nil
}

func hashPending(root string, paths []string, pending []int) ([]string, error) {
	if len(pending) == 0 {
		return nil, nil
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open canonical workspace: %w", err)
	}
	defer rootHandle.Close()
	digests := make([]string, len(pending))
	for position, index := range pending {
		contents, err := readCanonicalFile(rootHandle, paths[index])
		if err != nil {
			return nil, err
		}
		digests[position] = contentDigest(contents)
	}
	return digests, nil
}

func sameStampedTree(paths []string, stamps []fileStamp, otherPaths []string, otherStamps []fileStamp) bool {
	if len(paths) != len(otherPaths) {
		return false
	}
	for index := range paths {
		if paths[index] != otherPaths[index] || stamps[index] != otherStamps[index] {
			return false
		}
	}
	return true
}
