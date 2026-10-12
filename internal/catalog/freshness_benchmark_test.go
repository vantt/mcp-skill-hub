package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// BenchmarkInspectWhileLocked measures the per-request freshness check that
// every resolve performs before it serves the published generation.
func BenchmarkInspectWhileLocked(b *testing.B) {
	for _, size := range []struct{ skills, bodyBytes int }{{32, 0}, {300, 0}, {32, 64 << 10}} {
		skills := size.skills
		padding := strings.Repeat("Evidence line for a long skill body.\n", size.bodyBytes/37)
		b.Run(fmt.Sprintf("skills=%d/body=%dKiB", skills, size.bodyBytes>>10), func(b *testing.B) {
			root := filepath.Join(b.TempDir(), "workspace")
			if _, err := workspace.Apply(root); err != nil {
				b.Fatal(err)
			}
			for index := 0; index < skills; index++ {
				id := fmt.Sprintf("freshness-skill-%03d", index)
				writeCanonicalBenchmark(b, root, fmt.Sprintf("skills/freshness/%s/skill.meta.yaml", id), fmt.Sprintf("schema_version: 1\nid: %s\nname: Freshness Skill %03d\nstatus: active\ndescription: Review freshness workspace skill %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review freshness target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index))
				writeCanonicalBenchmark(b, root, fmt.Sprintf("skills/freshness/%s/SKILL.md", id), fmt.Sprintf("# Freshness Skill %03d\n\nReview target %03d with evidence.\n%s", index, index, padding))
			}
			ageCanonicalFiles(b, root, time.Minute)
			ctx := context.Background()
			built, err := BuildCatalogGeneration(ctx, root, BuildOptions{})
			if err != nil {
				b.Fatal(err)
			}
			past := time.Now().Add(-time.Minute)
			if err := os.Chtimes(generationPath(root, built.Pointer), past, past); err != nil {
				b.Fatal(err)
			}
			lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = lock.Unlock() }()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				status, err := InspectWhileLocked(ctx, root)
				if err != nil {
					b.Fatal(err)
				}
				if status.State != StateHealthy {
					b.Fatalf("state = %s, want healthy: %s", status.State, status.Detail)
				}
			}
		})
	}
}

// ageCanonicalFiles moves every canonical file's modification time into the past,
// as for a hub that was not edited in the last few seconds.
func ageCanonicalFiles(tb testing.TB, root string, age time.Duration) {
	tb.Helper()
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		tb.Fatal(err)
	}
	past := time.Now().Add(-age)
	for _, relative := range files {
		if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(relative)), past, past); err != nil {
			tb.Fatal(err)
		}
	}
}
