package mcpserver

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// An agent session calls the server in short bursts and then sits idle,
	// often for most of its life. The Go runtime keeps freed heap pages after a
	// burst; returning them once the server is quiet removes about a third of a
	// working process's private memory without touching request latency.
	memoryReleaseIdleAfter     = 30 * time.Second
	memoryReleaseCheckInterval = 5 * time.Second
)

// idleMemoryReleaser returns freed heap memory to the operating system once,
// after a period with no MCP request, and again after each later burst.
type idleMemoryReleaser struct {
	lastActivity atomic.Int64 // unix nanoseconds of the latest request start or end
	inFlight     atomic.Int64
	released     atomic.Bool
	release      func()
}

func (releaser *idleMemoryReleaser) noteActivity(at time.Time) {
	releaser.lastActivity.Store(at.UnixNano())
	releaser.released.Store(false)
}

func (releaser *idleMemoryReleaser) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		releaser.inFlight.Add(1)
		releaser.noteActivity(time.Now())
		defer func() {
			releaser.noteActivity(time.Now())
			releaser.inFlight.Add(-1)
		}()
		return next(ctx, method, req)
	}
}

// releaseIfIdle reports whether it released memory at now.
func (releaser *idleMemoryReleaser) releaseIfIdle(now time.Time, idleAfter time.Duration) bool {
	if releaser.inFlight.Load() > 0 || releaser.released.Load() {
		return false
	}
	if now.Sub(time.Unix(0, releaser.lastActivity.Load())) < idleAfter {
		return false
	}
	releaser.released.Store(true)
	releaser.release()
	return true
}

func (releaser *idleMemoryReleaser) run(ctx context.Context, interval, idleAfter time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			releaser.releaseIfIdle(now, idleAfter)
		}
	}
}
