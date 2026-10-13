//go:build !race

package mcpserver

import (
	"runtime"
	"time"
)

// cacheHitBudget is the budget for a proxy cache hit. Windows and macOS runners are shared and slow
// to start processes and sockets, so they get the budget the race build uses; a hit that waits for the hub
// still misses it by a wide margin (the hub round trip in these tests is 300 ms).
var cacheHitBudget = func() time.Duration {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return 100 * time.Millisecond
	}
	return 20 * time.Millisecond
}()
