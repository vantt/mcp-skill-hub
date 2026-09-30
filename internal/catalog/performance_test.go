package catalog

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"testing"
	"time"
)

const (
	performanceSkillCount    = 256
	representativeBuildLimit = 10 * time.Second
	largeBuildLimit          = 30 * time.Second
)

var warmOpenP95Budget = 100 * time.Millisecond

func init() {
	if os.Getenv("CI") != "" {
		warmOpenP95Budget = 500 * time.Millisecond
	}
}

func TestCatalogPerformanceBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("performance budgets are excluded from short tests")
	}
	if os.Getenv("SKILLHUB_PERF") != "1" {
		t.Skip("wall-clock performance budgets run only with SKILLHUB_PERF=1 on an otherwise idle machine")
	}
	if performanceRaceEnabled() {
		t.Skip("wall-clock performance budgets are not stable under race instrumentation")
	}

	representativeRoot := newWorkspace(t)
	writePerformanceSkills(t, representativeRoot, 1)
	build(t, representativeRoot, BuildOptions{})
	representativeBuilds := measureDurations(t, 1, 5, func() error {
		_, err := BuildCatalogGeneration(context.Background(), representativeRoot, BuildOptions{})
		return err
	})
	representativeP95 := durationPercentile(representativeBuilds, 95)
	representativeP99 := durationPercentile(representativeBuilds, 99)
	t.Logf("catalog representative rebuild n=%d p50=%s p95=%s p99=%s", len(representativeBuilds), durationPercentile(representativeBuilds, 50), representativeP95, representativeP99)
	if representativeP95 >= representativeBuildLimit || representativeP99 >= representativeBuildLimit {
		t.Fatalf("representative catalog rebuild p95/p99 %s/%s exceeds hang ceiling %s", representativeP95, representativeP99, representativeBuildLimit)
	}

	largeRoot := newWorkspace(t)
	writePerformanceSkills(t, largeRoot, performanceSkillCount)
	build(t, largeRoot, BuildOptions{})

	openDurations := measureDurations(t, 3, 20, func() error {
		handle, err := OpenCurrent(context.Background(), largeRoot)
		if err != nil {
			return err
		}
		return handle.Close()
	})
	openP95 := durationPercentile(openDurations, 95)
	t.Logf("catalog warm open skills=%d n=%d p50=%s p95=%s p99=%s", performanceSkillCount, len(openDurations), durationPercentile(openDurations, 50), openP95, durationPercentile(openDurations, 99))
	if openP95 >= warmOpenP95Budget {
		t.Fatalf("warm catalog open p95 %s exceeds budget %s", openP95, warmOpenP95Budget)
	}

	largeBuilds := measureDurations(t, 1, 5, func() error {
		_, err := BuildCatalogGeneration(context.Background(), largeRoot, BuildOptions{})
		return err
	})
	largeP95 := durationPercentile(largeBuilds, 95)
	largeP99 := durationPercentile(largeBuilds, 99)
	t.Logf("catalog rebuild skills=%d n=%d p50=%s p95=%s p99=%s", performanceSkillCount, len(largeBuilds), durationPercentile(largeBuilds, 50), largeP95, largeP99)
	if largeP95 >= largeBuildLimit || largeP99 >= largeBuildLimit {
		t.Fatalf("%d-skill catalog rebuild p95/p99 %s/%s exceeds hang ceiling %s", performanceSkillCount, largeP95, largeP99, largeBuildLimit)
	}
}

func writePerformanceSkills(t *testing.T, root string, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("performance-skill-%03d", index)
		metadata := fmt.Sprintf("schema_version: 1\nid: %s\nname: Performance Skill %03d\nstatus: active\ndescription: Review performance workspace skill %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review performance target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index)
		writeCanonical(t, root, fmt.Sprintf("skills/performance/%s/skill.meta.yaml", id), metadata)
		writeCanonical(t, root, fmt.Sprintf("skills/performance/%s/SKILL.md", id), fmt.Sprintf("# Performance Skill %03d\n\nReview target %03d with evidence.\n", index, index))
	}
}

func measureDurations(t *testing.T, warmups, iterations int, operation func() error) []time.Duration {
	t.Helper()
	for index := 0; index < warmups; index++ {
		if err := operation(); err != nil {
			t.Fatalf("performance warm-up %d failed: %v", index+1, err)
		}
	}
	durations := make([]time.Duration, iterations)
	for index := range durations {
		started := time.Now()
		if err := operation(); err != nil {
			t.Fatalf("performance iteration %d failed: %v", index+1, err)
		}
		durations[index] = time.Since(started)
	}
	return durations
}

// durationPercentile uses the deterministic nearest-rank definition.
func durationPercentile(samples []time.Duration, percentile int) time.Duration {
	ordered := append([]time.Duration(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := (percentile*len(ordered)+99)/100 - 1
	if index < 0 {
		index = 0
	}
	return ordered[index]
}

func performanceRaceEnabled() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-race" && setting.Value == "true" {
			return true
		}
	}
	return false
}
