package mutation

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const executableTestPath = "sources/catalog/SRC-EXEC.yaml"

func executableTestSet(operationID string, executable bool) WriteSet {
	return WriteSet{OperationID: operationID, Command: "create_source", Changes: []Change{{
		Path: executableTestPath, Contents: []byte("schema_version: 1\nid: SRC-EXEC\nadapter: git\n"), Executable: executable,
	}}}
}

func TestExecutableChangeCreatesExecutableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	root := newWorkspace(t)
	if _, err := Commit(root, executableTestSet("OP-EXEC", true)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, executableTestPath))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestPlainChangeStaysNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	root := newWorkspace(t)
	if _, err := Commit(root, executableTestSet("OP-PLAIN", false)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, executableTestPath))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestRecoveryPublishesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	root := newWorkspace(t)
	injected := errors.New("stop before publish")
	_, err := CommitWithOptions(root, executableTestSet("OP-EXEC-RECOVER", true), Options{Fault: func(point FaultPoint) error {
		if point == FaultBeforeTargetDisplace {
			return injected
		}
		return nil
	}})
	if !errors.Is(err, injected) {
		t.Fatalf("commit error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, executableTestPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("file published before the fault point: %v", statErr)
	}
	if err := RollForward(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, executableTestPath))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("recovered mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestExecutableMarkerChangesProposalDigestOnlyWhenSet(t *testing.T) {
	t.Parallel()
	plain := proposalDigest(executableTestSet("OP", false))
	if plain != "sha256:e817233525cf7d4014a2481b95869068f61ee32e022f04171e4db772f3e7c5d6" {
		t.Fatalf("digest of a non-executable write set changed: %s", plain)
	}
	if executable := proposalDigest(executableTestSet("OP", true)); executable == plain {
		t.Fatal("an executable change must change the proposal digest")
	}
}

func TestRecoveryAfterDisplacementPublishesExecutableReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	root := newWorkspace(t)
	target := filepath.Join(root, filepath.FromSlash(executableTestPath))
	before := []byte("schema_version: 1\nid: SRC-EXEC\nadapter: git\nname: old\n")
	if err := os.WriteFile(target, before, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	set := executableTestSet("OP-EXEC-REPLACE", true)
	set.Changes[0].BeforeDigest = digest(before)
	crash := errors.New("crash after displacement")
	_, err := CommitWithOptions(root, set, Options{Fault: func(point FaultPoint) error {
		if point == FaultAfterTargetDisplace {
			return crash
		}
		return nil
	}})
	if !errors.Is(err, crash) {
		t.Fatalf("commit error = %v", err)
	}
	if err := RollForward(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("recovered replacement mode = %v, want 0755", info.Mode().Perm())
	}
}
