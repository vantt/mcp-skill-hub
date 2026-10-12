package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// writeSkillTree writes a tree shaped like a hub: category directories holding
// skill directories holding a few files, plus the excluded runtime directory.
func writeSkillTree(tb testing.TB, root string, skills int) {
	tb.Helper()
	files := []string{"skill.meta.yaml", "SKILL.md", "references/notes.md"}
	for index := range skills {
		for _, name := range files {
			path := filepath.Join(root, "skills", fmt.Sprintf("category-%02d", index%8), fmt.Sprintf("skill-%03d", index), filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				tb.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("content of "+name+"\n"), 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}
	runtimeFile := filepath.Join(root, "runtime", "catalog", "pointer.json")
	if err := os.MkdirAll(filepath.Dir(runtimeFile), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(runtimeFile, []byte("{}"), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// referenceRelativeFiles lists the canonical files by walking the tree with
// fs.WalkDir and the same exclusion rule, without sharing any code with
// RelativeFiles.
func referenceRelativeFiles(tb testing.TB, root string) []string {
	tb.Helper()
	var paths []string
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || isNonCanonicalPath(path) {
			if entry.IsDir() && path != "." {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func TestRelativeFilesListsTheSameFilesAsAnIndependentWalk(t *testing.T) {
	root := t.TempDir()
	writeSkillTree(t, root, 40)
	got, err := RelativeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := referenceRelativeFiles(t, root); !reflect.DeepEqual(got, want) || len(got) != 120 {
		t.Fatalf("RelativeFiles = %d paths, reference = %d paths\n%v\n%v", len(got), len(want), got, want)
	}
}

func BenchmarkRelativeFiles(b *testing.B) {
	for _, skills := range []int{32, 300} {
		b.Run(fmt.Sprintf("skills=%d", skills), func(b *testing.B) {
			root := b.TempDir()
			writeSkillTree(b, root, skills)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := RelativeFiles(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
