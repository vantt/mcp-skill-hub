package mutation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// committedWorkspace is a hub whose Git history holds everything written so far,
// with a Git config isolated from the machine's.
func committedWorkspace(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := newWorkspace(t)
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	commitEverything(t, root, "baseline")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func commitEverything(t *testing.T, root, message string) {
	t.Helper()
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "--allow-empty", "-m", message)
}

func commitCount(t *testing.T, root string) string {
	return runGit(t, root, "rev-list", "--count", "HEAD")
}

func writeSourceFile(t *testing.T, root, name, contents string) string {
	t.Helper()
	path := "sources/catalog/" + name + ".yaml"
	if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func confirmCtx(policy workspace.CommitPolicy) context.Context {
	return workspace.WithCommitPolicy(context.Background(), policy)
}

func TestConfirmedMutationBecomesOneScopedCommit(t *testing.T) {
	root := committedWorkspace(t)
	writeSourceFile(t, root, "SRC-UNRELATED", "id: SRC-UNRELATED\n")
	before := runGit(t, root, "rev-parse", "HEAD")

	receipt, err := CommitWithOptions(root, WriteSet{OperationID: "OP-COMMIT", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}},
		Options{Context: confirmCtx(workspace.CommitPolicy{}), CommitMessage: "Add SRC-1"})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Commit.Status != workspace.CommitCommitted || receipt.Commit.Hash == "" || receipt.Commit.Message != "Add SRC-1" {
		t.Fatalf("commit outcome = %+v", receipt.Commit)
	}
	if runGit(t, root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatal("expected exactly one new commit")
	}
	if body := runGit(t, root, "log", "-1", "--format=%B"); !strings.Contains(body, "Skillhub-Operation: OP-COMMIT") {
		t.Fatalf("commit message lacks the operation trailer:\n%s", body)
	}
	files := strings.Fields(runGit(t, root, "show", "--name-only", "--format=", "HEAD"))
	if len(files) != len(receipt.ChangedPaths) {
		t.Fatalf("commit holds %v, receipt lists %v", files, receipt.ChangedPaths)
	}
	if status := runGit(t, root, "status", "--porcelain=v1"); !strings.Contains(status, "SRC-UNRELATED") || strings.Contains(status, "SRC-1") {
		t.Fatalf("only the unrelated file may stay untracked:\n%s", status)
	}
}

func TestMutationWithoutAPolicyOnlyWritesFiles(t *testing.T) {
	root := committedWorkspace(t)
	before := commitCount(t, root)
	receipt, err := Commit(root, WriteSet{OperationID: "OP-PLAIN", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Commit.Status != "" || commitCount(t, root) != before {
		t.Fatalf("a context without a policy must not commit: %+v", receipt.Commit)
	}
}

func TestMutationRefusesFilesWithUncommittedEditsBeforeWriting(t *testing.T) {
	root := committedWorkspace(t)
	path := writeSourceFile(t, root, "SRC-EDITED", "id: SRC-EDITED\n")
	commitEverything(t, root, "add edited")
	edited := []byte("id: SRC-EDITED\nname: hand edit\n")
	if err := os.WriteFile(filepath.Join(root, path), edited, 0o644); err != nil {
		t.Fatal(err)
	}
	set := WriteSet{OperationID: "OP-REFUSED", Command: "edit_source", Changes: []Change{{Path: path, BeforeDigest: digest(edited), Contents: []byte("id: SRC-EDITED\nname: tool edit\n")}}}

	_, err := CommitWithOptions(root, set, Options{Context: confirmCtx(workspace.CommitPolicy{})})
	var refused *UncommittedEditsError
	if !errors.As(err, &refused) || len(refused.Paths) != 1 || refused.Paths[0] != path {
		t.Fatalf("err = %v, want the uncommitted edits refusal for %s", err, path)
	}
	if data, _ := os.ReadFile(filepath.Join(root, path)); string(data) != string(edited) {
		t.Fatal("a refused change must not write")
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "history", "operations", "*", "*", "OP-REFUSED.yaml")); len(matches) != 0 {
		t.Fatalf("a refused change must not write a receipt: %v", matches)
	}

	before := commitCount(t, root)
	receipt, err := CommitWithOptions(root, set, Options{Context: confirmCtx(workspace.CommitPolicy{Disabled: true})})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Commit.Status != "" || commitCount(t, root) != before {
		t.Fatalf("a disabled commit writes without committing: %+v", receipt.Commit)
	}
}

func TestReplayedOperationDoesNotCommitAgain(t *testing.T) {
	root := committedWorkspace(t)
	set := WriteSet{OperationID: "OP-REPLAY", IdempotencyKey: "replay-key", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}}
	options := Options{Context: confirmCtx(workspace.CommitPolicy{}), CommitMessage: "Add SRC-1"}
	if _, err := CommitWithOptions(root, set, options); err != nil {
		t.Fatal(err)
	}
	before := commitCount(t, root)
	replayed, err := CommitWithOptions(root, set, options)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Commit.Status != "" || commitCount(t, root) != before {
		t.Fatalf("a replay must not commit again: %+v", replayed.Commit)
	}
}

func TestDeferredPolicyReportsDeferredAndDoesNotCommit(t *testing.T) {
	root := committedWorkspace(t)
	before := commitCount(t, root)
	receipt, err := CommitWithOptions(root, WriteSet{OperationID: "OP-DEFER", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}},
		Options{Context: confirmCtx(workspace.CommitPolicy{Defer: true})})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Commit.Status != workspace.CommitDeferred || commitCount(t, root) != before {
		t.Fatalf("deferred outcome = %+v", receipt.Commit)
	}
}

func TestCommitRunsAfterTheWorkspaceLockIsReleased(t *testing.T) {
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock is not installed")
	}
	root := committedWorkspace(t)
	lockPath := filepath.Join(root, "runtime", "locks", "workspace.lock")
	hook := "#!/bin/sh\nflock -n " + lockPath + " true || { echo 'workspace lock is still held' >&2; exit 1; }\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "hooks", "pre-commit"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt, err := CommitWithOptions(root, WriteSet{OperationID: "OP-HOOK", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}},
		Options{Context: confirmCtx(workspace.CommitPolicy{}), CommitMessage: "Add SRC-1"})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Commit.Status != workspace.CommitCommitted {
		t.Fatalf("a hook that needs the lock must find it free: %+v", receipt.Commit)
	}
}
