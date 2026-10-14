package mutation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const lockRetryDelay = 25 * time.Millisecond

var DefaultLockTimeout = 15 * time.Second

// WorkspaceLock wraps the cross-process advisory workspace lock.
type WorkspaceLock struct{ lock *flock.Flock }

// Unlock releases a shared or exclusive workspace lock.
func (lock *WorkspaceLock) Unlock() error {
	if lock == nil || lock.lock == nil {
		return nil
	}
	return lock.lock.Unlock()
}

// AcquireSharedLock waits for a bounded duration for a reader lock.
func AcquireSharedLock(ctx context.Context, root string, timeout time.Duration) (*WorkspaceLock, error) {
	return acquireLock(ctx, root, timeout, true)
}

// AcquireExclusiveLock waits for a bounded duration for the sole writer lock.
func AcquireExclusiveLock(ctx context.Context, root string, timeout time.Duration) (*WorkspaceLock, error) {
	return acquireLock(ctx, root, timeout, false)
}

// TryAcquireSharedLock takes a reader lock only when it is free right now.
// It never waits: when a writer holds the lock it returns ErrWorkspaceBusy at
// once, so a quick status read can tell the person to retry instead of
// blocking.
func TryAcquireSharedLock(root string) (*WorkspaceLock, error) {
	lock, path, err := openLock(root)
	if err != nil {
		return nil, err
	}
	ok, err := lock.TryRLock()
	if err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceBusy, root)
	}
	return checkedLock(lock, path)
}

func openLock(root string) (*flock.Flock, string, error) {
	return openNamedLock(root, "workspace.lock")
}

func openNamedLock(root, name string) (*flock.Flock, string, error) {
	if err := ensureSafeParents(root, "runtime/locks", 0o755); err != nil {
		return nil, "", err
	}
	path := filepath.Join(root, "runtime", "locks", name)
	return flock.New(path), path, nil
}

// CommitLockTimeout bounds the wait for another confirmed change to finish its
// Git commit, which may run a slow hook.
var CommitLockTimeout = 2 * time.Minute

// AcquireCommitLock serializes "write the change, then commit it" across
// processes. It is taken before the exclusive workspace lock and released after
// the Git commit, so a second change never sees the first one's files as
// uncommitted and two commits never race on Git's index lock. The hub's
// pre-commit hook never takes it.
func AcquireCommitLock(ctx context.Context, root string) (*WorkspaceLock, error) {
	return acquireNamedLock(ctx, root, "commit.lock", CommitLockTimeout, false)
}

func acquireLock(ctx context.Context, root string, timeout time.Duration, shared bool) (*WorkspaceLock, error) {
	return acquireNamedLock(ctx, root, "workspace.lock", timeout, shared)
}

func acquireNamedLock(ctx context.Context, root, name string, timeout time.Duration, shared bool) (*WorkspaceLock, error) {
	if timeout <= 0 {
		timeout = DefaultLockTimeout
	}
	lock, path, err := openNamedLock(root, name)
	if err != nil {
		return nil, err
	}
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var ok bool
	if shared {
		ok, err = lock.TryRLockContext(waitContext, lockRetryDelay)
	} else {
		ok, err = lock.TryLockContext(waitContext, lockRetryDelay)
	}
	if err != nil {
		if waitContext.Err() != nil {
			return nil, fmt.Errorf("%w after %s: %s", ErrWorkspaceBusy, timeout, root)
		}
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("%w after %s: %s", ErrWorkspaceBusy, timeout, root)
	}
	return checkedLock(lock, path)
}

func checkedLock(lock *flock.Flock, path string) (*WorkspaceLock, error) {
	// Ensure the lock file is not an unexpected filesystem object. flock opened
	// it already, but callers still receive a clear diagnosis.
	if info, statErr := os.Lstat(path); statErr != nil || !info.Mode().IsRegular() {
		_ = lock.Unlock()
		if statErr != nil {
			return nil, statErr
		}
		return nil, fmt.Errorf("unsafe workspace lock path: %s", path)
	}
	return &WorkspaceLock{lock: lock}, nil
}
