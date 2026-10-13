//go:build !race

package cli

import "time"

// proxyHitBudget is the budget for a proxy cache hit. Shared CI runners are slow to start processes and
// sockets, so every platform gets the budget the race build uses; a hit that waits for the hub still
// misses it by a wide margin (the hub round trip in these tests is 250 ms).
var proxyHitBudget = 100 * time.Millisecond
