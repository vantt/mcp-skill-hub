//go:build !race

package mcpserver

import "time"

// cacheHitBudget is the budget for a proxy cache hit. Shared CI runners are slow to start processes and
// sockets, so every platform gets the budget the race build uses; a hit that waits for the hub still misses
// it by a wide margin (the hub round trip in these tests is 300 ms).
var cacheHitBudget = 100 * time.Millisecond
