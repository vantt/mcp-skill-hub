//go:build race

package telemetry

import "time"

// writerHelperPace spaces the events of one writer process. The race detector
// slows SQLite many times over, so instrumented writers produce more slowly to
// keep measuring lost events rather than a saturated queue.
const writerHelperPace = 20 * time.Millisecond
