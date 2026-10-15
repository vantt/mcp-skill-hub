package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkCommitPathsInALargeHub measures one auto-commit of two files in a
// hub that already tracks many files, the cost every confirmed change pays.
func BenchmarkCommitPathsInALargeHub(b *testing.B) {
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	b.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	b.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := b.TempDir()
	run := func(args ...string) {
		if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			b.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "Bench")
	run("config", "user.email", "bench@example.com")
	run("config", "gc.auto", "0")
	// 20,000 files of 8 KiB: hashing them all again would show in the result.
	padding := strings.Repeat("reference text\n", 550)
	for index := 0; index < 20000; index++ {
		write(fmt.Sprintf("skills/default/s%03d/references/f%05d.md", index%500, index), fmt.Sprintf("file %d\n%s", index, padding))
	}
	run("add", "-A")
	run("commit", "-q", "-m", "many files")
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		edited := "skills/default/s000/SKILL.md"
		script := fmt.Sprintf("skills/default/s000/scripts/run%d.sh", iteration)
		write(edited, fmt.Sprintf("edit %d\n", iteration))
		write(script, "#!/bin/sh\n")
		b.StartTimer()
		outcome := CommitPaths(context.Background(), root, []string{edited, script}, map[string]bool{script: true}, "Edit", false)
		if outcome.Status != CommitCommitted {
			b.Fatalf("outcome = %+v", outcome)
		}
	}
}
