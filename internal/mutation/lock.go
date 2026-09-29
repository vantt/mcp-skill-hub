package mutation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	DefaultLockTimeout = 5 * time.Second
	lockRetryDelay     = 25 * time.Millisecond
)

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

func acquireLock(ctx context.Context, root string, timeout time.Duration, shared bool) (*WorkspaceLock, error) {
	if timeout <= 0 {
		timeout = DefaultLockTimeout
	}
	lockDir := filepath.Join(root, "runtime", "locks")
	if err := ensureSafeParents(root, "runtime/locks", 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(lockDir, "workspace.lock")
	lock := flock.New(path)
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var ok bool
	var err error
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
