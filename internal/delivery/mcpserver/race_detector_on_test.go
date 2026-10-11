//go:build race

package mcpserver

import "time"

// cacheHitBudget is the longest a proxy cache hit may take in a test. The
// plan's budget is 20 ms; the race detector slows the hit several times over,
// so instrumented runs allow more, still far below a round trip to the hub in
// the tests that check the hit does not wait for one.
const cacheHitBudget = 100 * time.Millisecond
