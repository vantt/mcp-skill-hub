package catalog

import (
	"context"
	"database/sql"
	"sort"
	"sync/atomic"

	"github.com/vantt/skillhub-mcp/internal/canonical"
	"github.com/vantt/skillhub-mcp/internal/mutation"
)

// View is the current generation observed under the shared workspace lock,
// which it keeps until Close, so no mutation can commit between the
// observation and the reads that follow. Unlike a Handle it does not open the
// database: a request that finds what it needs in the generation's memo never
// touches SQLite or writes a pin file. DB opens and pins the generation on the
// first request that does need it. A View belongs to one request.
type View struct {
	Pointer Pointer
	Status  Status

	root    string
	files   []canonical.FileDigest // sorted by path
	memo    *GenerationMemo
	handle  *Handle
	release func() error
}

// ObserveWithFallbackLocked observes the current generation like
// OpenWithFallbackLocked: it rebuilds a stale catalog when the canonical files
// are valid and serves the published generation as fallback when they are not.
func ObserveWithFallbackLocked(ctx context.Context, root string) (*View, error) {
	return observeWithFallback(ctx, root)
}

// ObserveCurrentLocked observes the current generation like OpenCurrentLocked:
// the catalog must be healthy.
func ObserveCurrentLocked(ctx context.Context, root string) (*View, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	status, snapshot, err := healthyCurrentWhileLocked(ctx, root)
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	return newView(root, status, snapshot, lock.Unlock), nil
}

// ObservePublishedLocked observes the published generation as it is, without
// asking whether the canonical files have moved on since it was built. It never
// rebuilds. A reader that wants to know what agents are served now uses it: an
// index that is only behind file edits is still the one in use. It fails when
// the pointer is missing, the generation is corrupt or its schema is
// incompatible. The view carries no canonical digests.
func ObservePublishedLocked(ctx context.Context, root string) (*View, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	status, err := inspectPublishedWhileLocked(ctx, root)
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	if status.State != StateUnknown || status.Pointer == nil {
		_ = lock.Unlock()
		return nil, catalogUnavailableError{state: status.State, detail: status.Detail}
	}
	return newView(root, status, canonical.Snapshot{}, lock.Unlock), nil
}

func newView(root string, status Status, snapshot canonical.Snapshot, release func() error) *View {
	return &View{Pointer: *status.Pointer, Status: status, root: root, memo: memoFor(root, *status.Pointer), files: snapshot.Files, release: release}
}

// CanonicalDigest returns the content digest of a canonical file as the
// freshness check observed it under the lock this view holds: a file whose
// size, times and identity are unchanged and settled keeps its earlier digest,
// every other file was read and hashed. It reports false for a path that is
// not a canonical file.
func (view *View) CanonicalDigest(path string) (string, bool) {
	index := sort.Search(len(view.files), func(i int) bool { return view.files[i].Path >= path })
	if index < len(view.files) && view.files[index].Path == path {
		return view.files[index].Digest, true
	}
	return "", false
}

// DB opens the observed generation read-only and pins it until Close. The
// database is opened once per view.
func (view *View) DB(ctx context.Context) (*sql.DB, error) {
	if view.handle == nil {
		handle, err := openPinnedGenerationWithStatus(ctx, view.root, view.Pointer, view.Status)
		if err != nil {
			return nil, err
		}
		view.handle = handle
	}
	return view.handle.DB, nil
}

// Close releases the database and its pin, if DB opened them, and then the
// workspace lock.
func (view *View) Close() error {
	if view == nil {
		return nil
	}
	closeErr := view.handle.Close()
	view.handle = nil
	if view.release != nil {
		if err := view.release(); err != nil && closeErr == nil {
			closeErr = err
		}
		view.release = nil
	}
	return closeErr
}

// pinnedOpens counts generation databases opened by this process.
var pinnedOpens atomic.Int64

// PinnedOpens reports how many generation databases this process has opened and
// pinned. A request served from the generation memo does not add to it.
func PinnedOpens() int64 { return pinnedOpens.Load() }
