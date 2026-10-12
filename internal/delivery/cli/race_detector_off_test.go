//go:build !race

package cli

import (
	"runtime"
	"time"
)

// proxyHitBudget is the budget for a proxy cache hit. Windows runners are shared and slow to start
// processes and sockets, so they get the budget the race build uses; a hit that waits for the hub
// still misses it by a wide margin (the hub round trip in these tests is 250 ms).
var proxyHitBudget = func() time.Duration {
	if runtime.GOOS == "windows" {
		return 100 * time.Millisecond
	}
	return 20 * time.Millisecond
}()
