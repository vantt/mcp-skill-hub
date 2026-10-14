package source

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestGitListReportsExecutableBlobs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	writeCaptureFile(t, dir, "SKILL.md", "---\nname: x\n---\n", 0o644)
	writeCaptureFile(t, dir, "scripts/bump.sh", "#!/bin/sh\n", 0o755)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}

	adapter := GitRepositoryAdapter{CacheRoot: t.TempDir(), AllowFileProtocol: true}
	src := Source{Locator: Locator{Repository: fileURLFromPath(dir), Ref: "master"}}
	rev, err := adapter.CurrentRevision(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := adapter.List(context.Background(), src, rev, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, item := range resources {
		want := item.Path == "scripts/bump.sh"
		if item.Executable != want {
			t.Errorf("%s Executable = %v, want %v", item.Path, item.Executable, want)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("listed %d resources, want 2: %v", seen, resources)
	}
}
