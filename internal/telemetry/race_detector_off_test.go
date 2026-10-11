//go:build !race

package telemetry

import "time"

// writerHelperPace spaces the events of one writer process: 200 events per
// second each, ten times one serve process under the fan-out benchmark.
const writerHelperPace = 5 * time.Millisecond
