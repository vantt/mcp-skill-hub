package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// BenchmarkSkillResolve measures one in-process skill_resolve, including the
// catalog freshness check, against a published catalog that is current.
func BenchmarkSkillResolve(b *testing.B) {
	for _, skills := range []int{32, 300} {
		b.Run(fmt.Sprintf("skills=%d", skills), func(b *testing.B) {
			root := filepath.Join(b.TempDir(), "workspace")
			if _, err := workspace.Apply(root); err != nil {
				b.Fatal(err)
			}
			for index := 0; index < skills; index++ {
				id := fmt.Sprintf("resolve-skill-%03d", index)
				writeResolveBenchmarkFile(b, root, fmt.Sprintf("skills/benchmark/%s/skill.meta.yaml", id), fmt.Sprintf("schema_version: 1\nid: %s\nname: Resolve Skill %03d\nstatus: active\ndescription: Review benchmark workspace skill %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review benchmark target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index))
				writeResolveBenchmarkFile(b, root, fmt.Sprintf("skills/benchmark/%s/SKILL.md", id), fmt.Sprintf("---\nname: %s\ndescription: Review benchmark workspace skill %03d.\n---\n\n# Resolve Skill %03d\n\nReview target %03d with evidence.\n", id, index, index, index))
			}
			// A hub that was not edited in the last few seconds.
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
			service := ResolverService{Cache: resolverpkg.NewCache(8)}
			b.ReportAllocs()
			b.ResetTimer()
			index := 0
			for b.Loop() {
				target := index % skills
				index++
				response, err := service.Resolve(context.Background(), root, resolverpkg.Request{
					SchemaVersion: resolverpkg.SchemaVersion,
					RequestID:     fmt.Sprintf("benchmark-%d", index),
					Task:          resolverpkg.Task{Description: fmt.Sprintf("review benchmark target %03d", target), Scope: "multi_step"},
					Operation:     "review",
				})
				if err != nil {
					b.Fatal(err)
				}
				if response.Status != resolverpkg.StatusResolved || response.Primary == nil || response.Primary.ID != fmt.Sprintf("resolve-skill-%03d", target) {
					b.Fatalf("unexpected resolution: status=%s primary=%v", response.Status, response.Primary)
				}
			}
		})
	}
}

func writeResolveBenchmarkFile(tb testing.TB, root, relative, contents string) {
	tb.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		tb.Fatal(err)
	}
}
