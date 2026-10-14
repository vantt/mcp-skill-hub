//go:build race

package mcpserver

import "time"

// cacheHitBudget is the longest a proxy cache hit may take in a test. The
// plan's budget is 20 ms; the race detector slows the hit several times over,
// so instrumented runs allow more, still far below a round trip to the hub in
// the tests that check the hit does not wait for one: their hub round trip is
// 300 ms, so a hit that waited for the hub always takes longer than 250 ms, while
// a stalled shared runner (a macOS runner under the race detector took 160 ms)
// still passes.
const cacheHitBudget = 250 * time.Millisecond
