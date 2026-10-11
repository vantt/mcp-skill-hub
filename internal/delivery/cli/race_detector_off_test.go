//go:build !race

package cli

import "time"

// proxyHitBudget is the plan's budget for a cache hit through the proxy.
const proxyHitBudget = 20 * time.Millisecond
