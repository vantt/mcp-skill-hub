package resolver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	resolverPerformanceSkillCount = 256
	resolverHangLimit             = 2 * time.Second
)

var resolverP95Budget = 50 * time.Millisecond

func init() {
	if os.Getenv("CI") != "" {
		resolverP95Budget = 250 * time.Millisecond
	}
}

func TestResolverPerformanceBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("performance budgets are excluded from short tests")
	}
	if os.Getenv("SKILLHUB_PERF") != "1" {
		t.Skip("wall-clock performance budgets run only with SKILLHUB_PERF=1 on an otherwise idle machine")
	}
	if resolverPerformanceRaceEnabled() {
		t.Skip("wall-clock performance budgets are not stable under race instrumentation")
	}

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	writeResolverPerformanceSkills(t, root, resolverPerformanceSkillCount)
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	request := Request{
		SchemaVersion: SchemaVersion,
		RequestID:     "performance-request",
		Task:          Task{Description: "review performance target 137", Scope: "multi_step"},
		Operation:     "review",
	}

	durations := resolverMeasureDurations(t, 5, 30, func() error {
		handle, err := catalog.OpenCurrent(context.Background(), root)
		if err != nil {
			return err
		}
		defer handle.Close()
		sqliteCatalog, err := NewSQLiteCatalog(handle.DB, handle.Pointer.CatalogSnapshot)
		if err != nil {
			return err
		}
		policy, err := LoadPolicy(context.Background(), handle.DB)
		if err != nil {
			return err
		}
		engine, err := New(sqliteCatalog, policy, NewCache(4))
		if err != nil {
			return err
		}
		response, err := engine.Resolve(context.Background(), request)
		if err != nil {
			return err
		}
		if response.Status != StatusResolved || response.Primary == nil || response.Primary.ID != "performance-skill-137" {
			return fmt.Errorf("unexpected resolver result: status=%s primary=%v", response.Status, response.Primary)
		}
		return nil
	})
	p95 := resolverDurationPercentile(durations, 95)
	p99 := resolverDurationPercentile(durations, 99)
	t.Logf("resolver uncached resolve skills=%d n=%d p50=%s p95=%s p99=%s", resolverPerformanceSkillCount, len(durations), resolverDurationPercentile(durations, 50), p95, p99)
	if p95 >= resolverP95Budget {
		t.Fatalf("resolver p95 %s exceeds budget %s", p95, resolverP95Budget)
	}
	if p99 >= resolverHangLimit {
		t.Fatalf("resolver p99 %s exceeds hang ceiling %s", p99, resolverHangLimit)
	}
}

func writeResolverPerformanceSkills(t *testing.T, root string, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("performance-skill-%03d", index)
		metadata := fmt.Sprintf("schema_version: 1\nid: %s\nname: Performance Skill %03d\nstatus: active\ndescription: Review performance workspace skill %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review performance target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index)
		resolverWritePerformanceFile(t, root, fmt.Sprintf("skills/performance/%s/skill.meta.yaml", id), metadata)
		resolverWritePerformanceFile(t, root, fmt.Sprintf("skills/performance/%s/SKILL.md", id), fmt.Sprintf("# Performance Skill %03d\n\nReview target %03d with evidence.\n", index, index))
	}
}

func resolverWritePerformanceFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resolverMeasureDurations(t *testing.T, warmups, iterations int, operation func() error) []time.Duration {
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

// resolverDurationPercentile uses the deterministic nearest-rank definition.
func resolverDurationPercentile(samples []time.Duration, percentile int) time.Duration {
	ordered := append([]time.Duration(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := (percentile*len(ordered)+99)/100 - 1
	if index < 0 {
		index = 0
	}
	return ordered[index]
}

func resolverPerformanceRaceEnabled() bool {
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
