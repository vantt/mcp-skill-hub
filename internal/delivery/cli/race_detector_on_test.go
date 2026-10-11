//go:build race

package cli

import "time"

// proxyHitBudget is the longest a cache hit through a proxy process may take
// in a test. The plan's budget is 20 ms; the race detector slows both
// processes several times over, so instrumented runs allow more, still below
// the round trip to the hub that the tests add.
const proxyHitBudget = 100 * time.Millisecond
