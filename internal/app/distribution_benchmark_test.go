package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// newDistributionBenchmarkWorkspace builds a hub of count active skills, each
// with an entrypoint and four reference files, and ages its files the way a hub
// nobody edited in the last minute looks.
func newDistributionBenchmarkWorkspace(b *testing.B, count int) string {
	b.Helper()
	root := filepath.Join(b.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		b.Fatal(err)
	}
	for index := range count {
		id := fmt.Sprintf("distribution-skill-%03d", index)
		writeResolveBenchmarkFile(b, root, fmt.Sprintf("skills/benchmark/%s/skill.meta.yaml", id), fmt.Sprintf("schema_version: 1\nid: %s\nname: Distribution Skill %03d\nstatus: active\ndescription: Review benchmark workspace skill %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review benchmark target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index))
		writeResolveBenchmarkFile(b, root, fmt.Sprintf("skills/benchmark/%s/SKILL.md", id), fmt.Sprintf("---\nname: %s\ndescription: Review benchmark workspace skill %03d.\n---\n\n# Distribution Skill %03d\n\n%s", id, index, index, strings.Repeat("Read the references first.\n", 40)))
		for part := range 4 {
			writeResolveBenchmarkFile(b, root, fmt.Sprintf("skills/benchmark/%s/references/part-%d.md", id, part), "# Part\n\n"+strings.Repeat("Reference text.\n", 200))
		}
	}
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		b.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	for _, relative := range files {
		if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(relative)), past, past); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		b.Fatal(err)
	}
	return root
}

// BenchmarkDistribution measures the service calls behind skills/list,
// skills/get and resources/read, including the catalog freshness check, with
// the generation unchanged between calls.
func BenchmarkDistribution(b *testing.B) {
	for _, count := range []int{32, 300} {
		b.Run(fmt.Sprintf("skills=%d", count), func(b *testing.B) {
			root := newDistributionBenchmarkWorkspace(b, count)
			ctx := context.Background()
			service := DistributionService{}
			entries, _, err := service.ListSkills(ctx, root)
			if err != nil {
				b.Fatal(err)
			}
			var target DistributedSkill
			for _, entry := range entries {
				if entry.SkillID == "distribution-skill-000" {
					target = entry
				}
			}
			if target.URI == "" {
				b.Fatal("benchmark skill is not listed")
			}
			b.Run("list", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, _, err := service.ListSkills(ctx, root); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("get", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := service.GetSkill(ctx, root, target.URI); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("read", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := service.ReadResource(ctx, root, target.URI); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("read_core", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := (SkillService{}).ReadSkillCore(ctx, root, "distribution-skill-000"); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("ensure", func(b *testing.B) {
				snapshots := NewSnapshotService()
				if _, err := snapshots.Ensure(ctx, root, "distribution-skill-000"); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := snapshots.Ensure(ctx, root, "distribution-skill-000"); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
