package mutation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/workspace"
)

func TestConcurrentConfirmsEachBecomeTheirOwnCommit(t *testing.T) {
	root := committedWorkspace(t)
	// A slow hook widens the window in which two commits would collide.
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nsleep 0.2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := strconv.Atoi(commitCount(t, root))
	if err != nil {
		t.Fatal(err)
	}
	const confirms = 4
	receipts := make([]Receipt, confirms)
	errs := make([]error, confirms)
	var wg sync.WaitGroup
	for index := 0; index < confirms; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("SRC-%d", index)
			set := WriteSet{OperationID: "OP-" + name, Command: "create_source", Changes: []Change{{Path: "sources/catalog/" + name + ".yaml", Contents: []byte("id: " + name + "\n")}}}
			receipts[index], errs[index] = CommitWithOptions(root, set, Options{Context: confirmCtx(workspace.CommitPolicy{}), CommitMessage: "Add " + name})
		}()
	}
	wg.Wait()
	for index := range receipts {
		if errs[index] != nil || receipts[index].Commit.Status != workspace.CommitCommitted {
			t.Fatalf("confirm %d: err = %v, commit = %+v", index, errs[index], receipts[index].Commit)
		}
	}
	if got := commitCount(t, root); got != strconv.Itoa(before+confirms) {
		t.Fatalf("commits = %s, want %d", got, before+confirms)
	}
	if status := runGit(t, root, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("files were left behind:\n%s", status)
	}
}
