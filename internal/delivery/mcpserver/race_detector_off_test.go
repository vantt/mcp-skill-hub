//go:build !race

package mcpserver

import "time"

// cacheHitBudget is the plan's budget for a proxy cache hit.
const cacheHitBudget = 20 * time.Millisecond
