package catalog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestEveryCatalogFaultPreservesCanonicalAuthorityAndRecoverablePointer(t *testing.T) {
	t.Parallel()
	points := []FaultPoint{FaultDatabaseClose, FaultGenerationSync, FaultGenerationsDirSync, FaultPointerTempSync, FaultPointerRename, FaultPointerDirSync}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			root := newWorkspace(t)
			initial := build(t, root, BuildOptions{BuilderVersion: "hardening"})
			before, err := canonical.Scan(root)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + string(point))
			result, buildErr := BuildCatalogGeneration(t.Context(), root, BuildOptions{BuilderVersion: "hardening", Fault: func(actual FaultPoint) error {
				if actual == point {
					return injected
				}
				return nil
			}})
			if point == FaultPointerDirSync {
				if buildErr != nil || result.Freshness != FreshnessIndeterminate {
					t.Fatalf("post-publish fault result = %#v, %v", result, buildErr)
				}
			} else if !errors.Is(buildErr, injected) {
				t.Fatalf("fault %s error = %v", point, buildErr)
			}
			after, err := canonical.Scan(root)
			if err != nil || before.CatalogSnapshot != after.CatalogSnapshot || before.ProjectionInputDigest != after.ProjectionInputDigest {
				t.Fatalf("catalog fault changed canonical authority: before=%#v after=%#v err=%v", before, after, err)
			}
			pointer, err := readPointer(root)
			if err != nil {
				t.Fatalf("pointer is not recoverable: %v", err)
			}
			if point != FaultPointerDirSync && pointer.Generation != initial.Pointer.Generation {
				t.Fatalf("pre-publication fault replaced pointer: got %s want %s", pointer.Generation, initial.Pointer.Generation)
			}
			handle, err := OpenCurrent(t.Context(), root)
			if err != nil {
				t.Fatalf("open after %s: %v", point, err)
			}
			if err := handle.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCatalogDiskFullWriteFailurePreservesPreviousGeneration(t *testing.T) {
	t.Parallel()
	root := newWorkspace(t)
	initial := build(t, root, BuildOptions{})
	before, err := canonical.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildCatalogGeneration(t.Context(), root, BuildOptions{Fault: func(point FaultPoint) error {
		if point == FaultGenerationSync {
			return syscall.ENOSPC
		}
		return nil
	}})
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("disk-full error = %v", err)
	}
	pointer, err := readPointer(root)
	if err != nil || pointer.Generation != initial.Pointer.Generation {
		t.Fatalf("pointer after disk-full = %#v, %v", pointer, err)
	}
	after, err := canonical.Scan(root)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("disk-full changed canonical authority: before=%#v after=%#v err=%v", before, after, err)
	}
}

func TestCatalogReadOnlyGenerationDirectoryFailsClosedWhereSupported(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permission enforcement is not reliable on this platform or as root")
	}
	root := newWorkspace(t)
	initial := build(t, root, BuildOptions{})
	directory := filepath.Join(root, "runtime", "catalog", "generations")
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Skipf("read-only permission setup unsupported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, info.Mode().Perm()) })
	if _, err := BuildCatalogGeneration(t.Context(), root, BuildOptions{}); err == nil {
		t.Fatal("read-only generation directory accepted a rebuild")
	}
	pointer, err := readPointer(root)
	if err != nil || pointer.Generation != initial.Pointer.Generation {
		t.Fatalf("read-only failure replaced pointer: %#v, %v", pointer, err)
	}
}

func TestCatalogGarbageCollectionHandlesClockSkew(t *testing.T) {
	t.Parallel()
	root := newWorkspace(t)
	first := build(t, root, BuildOptions{})
	writeCanonical(t, root, "sources/catalog/clock.yaml", "schema_version: 1\nid: clock\nadapter: git\nlocator: https://example.invalid/clock\n")
	build(t, root, BuildOptions{})
	oldGeneration := generationPath(root, first.Pointer)
	future := time.Now().Add(48 * time.Hour)
	if err := os.Chtimes(oldGeneration, future, future); err != nil {
		t.Skipf("filesystem timestamps do not support future skew: %v", err)
	}
	if err := CollectGarbage(root, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldGeneration); err != nil {
		t.Fatalf("future-skewed generation was collected: %v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldGeneration, past, past); err != nil {
		t.Fatal(err)
	}
	if err := CollectGarbage(root, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldGeneration); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired generation survived corrected clock: %v", err)
	}
	if issues, err := canonical.Validate(root); err != nil || len(issues) != 0 {
		t.Fatalf("clock skew affected canonical authority: %#v, %v", issues, err)
	}
}

func BenchmarkCatalogStartupAndOpen(b *testing.B) {
	root := filepath.Join(b.TempDir(), "workspace")
	if _, err := workspaceForBenchmark(root); err != nil {
		b.Fatal(err)
	}
	addCatalogBenchmarkSkills(b, root, 8)
	workspaceBytes := catalogFixtureBytes(b, root)
	b.ReportMetric(float64(workspaceBytes), "workspace_bytes")

	b.Run("cold_startup", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(workspaceBytes), "workspace_bytes")
		for range b.N {
			b.StopTimer()
			if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{BuilderVersion: "benchmark"}); err != nil {
				b.Fatal(err)
			}
		}
	})
	if _, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{BuilderVersion: "benchmark"}); err != nil {
		b.Fatal(err)
	}
	b.Run("open_current", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(workspaceBytes), "workspace_bytes")
		for range b.N {
			handle, err := OpenCurrent(context.Background(), root)
			if err != nil {
				b.Fatal(err)
			}
			if err := handle.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDeterministicFullRebuild(b *testing.B) {
	for _, fixture := range []struct {
		name   string
		skills int
	}{{"representative", 24}, {"large", 256}} {
		b.Run(fixture.name, func(b *testing.B) {
			b.StopTimer()
			root := filepath.Join(b.TempDir(), "workspace")
			if _, err := workspaceForBenchmark(root); err != nil {
				b.Fatal(err)
			}
			addCatalogBenchmarkSkills(b, root, fixture.skills)
			workspaceBytes := catalogFixtureBytes(b, root)
			b.ReportAllocs()
			b.ReportMetric(float64(workspaceBytes), "workspace_bytes")
			b.ReportMetric(float64(fixture.skills), "skills")
			var wantCatalog, wantProjection string
			for range b.N {
				b.StopTimer()
				if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				result, err := BuildCatalogGeneration(context.Background(), root, BuildOptions{BuilderVersion: "benchmark"})
				if err != nil {
					b.Fatal(err)
				}
				if wantCatalog == "" {
					wantCatalog, wantProjection = result.Pointer.CatalogSnapshot, result.Pointer.ProjectionInputDigest
				} else if result.Pointer.CatalogSnapshot != wantCatalog || result.Pointer.ProjectionInputDigest != wantProjection {
					b.Fatal("full rebuild was not deterministic")
				}
			}
		})
	}
}

func workspaceForBenchmark(root string) (string, error) {
	if _, err := workspace.Apply(root); err != nil {
		return "", err
	}
	return root, nil
}

func addCatalogBenchmarkSkills(tb testing.TB, root string, count int) {
	tb.Helper()
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("benchmark-skill-%04d", index)
		metadata := fmt.Sprintf("schema_version: 1\nid: %s\nname: Benchmark Skill %04d\nstatus: active\ndescription: Review benchmark workload %04d.\nrouting:\n  operations: [review]\n  triggers: [review benchmark workload %04d]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index)
		writeCanonicalBenchmark(tb, root, "skills/benchmark/"+id+"/skill.meta.yaml", metadata)
		writeCanonicalBenchmark(tb, root, "skills/benchmark/"+id+"/SKILL.md", "# "+id+"\n\nReview evidence and run tests.\n")
	}
}

func writeCanonicalBenchmark(tb testing.TB, root, relative, contents string) {
	tb.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func catalogFixtureBytes(tb testing.TB, root string) int64 {
	tb.Helper()
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "runtime") {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	return total
}
