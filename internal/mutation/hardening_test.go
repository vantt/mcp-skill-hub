package mutation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func FuzzMutationPathConfinement(f *testing.F) {
	for _, path := range []string{
		"sources/catalog/fuzz.yaml",
		"../sentinel.txt",
		"sources/catalog/../../sentinel.txt",
		"/tmp/skillhub-fuzz-escape",
		`sources\catalog\escape.yaml`,
		".skillhub/transactions/escape",
	} {
		f.Add(path)
	}

	f.Fuzz(func(t *testing.T, path string) {
		if len(path) > 512 {
			t.Skip()
		}
		parent := t.TempDir()
		root := filepath.Join(parent, "workspace")
		if _, err := workspace.Apply(root); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(parent, "sentinel.txt")
		const original = "outside canonical authority\n"
		if err := os.WriteFile(sentinel, []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = Commit(root, WriteSet{
			OperationID: "OP-FUZZ-PATH",
			Command:     "fuzz_path",
			Changes:     []Change{{Path: path, Contents: []byte("id: fuzz-path\n")}},
		})
		got, err := os.ReadFile(sentinel)
		if err != nil {
			t.Fatalf("outside sentinel unavailable after path %q: %v", path, err)
		}
		if string(got) != original {
			t.Fatalf("mutation escaped workspace for path %q: %q", path, got)
		}
	})
}

func TestEveryMutationFaultRecoversCanonicalAuthority(t *testing.T) {
	points := []FaultPoint{
		FaultTransactionCreated,
		FaultStagedFile,
		FaultManifestPhaseUpdate,
		FaultBeforeTargetDisplace,
		FaultAfterTargetDisplace,
		FaultFirstCanonicalReplace,
		FaultMiddleCanonicalReplace,
		FaultLastCanonicalReplace,
		FaultReceiptWrite,
		FaultCanonicalValidation,
		FaultJournalCleanup,
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			root := newWorkspace(t)
			set := hardeningReplacementSet(t, root, "OP-HARDEN-"+string(point))
			injected := errors.New("injected " + string(point))
			seen := 0
			_, err := CommitWithOptions(root, set, Options{Fault: func(actual FaultPoint) error {
				if actual == point {
					seen++
					if seen == 1 {
						return injected
					}
				}
				return nil
			}})
			if !errors.Is(err, injected) || seen == 0 {
				t.Fatalf("fault %s was not observed: seen=%d err=%v", point, seen, err)
			}
			if err := RollForward(root); err != nil {
				t.Fatalf("recover %s: %v", point, err)
			}
			wantNew := point != FaultTransactionCreated && point != FaultStagedFile
			assertHardeningReplacementState(t, root, wantNew)
			if pending, err := Pending(root); err != nil || len(pending) != 0 {
				t.Fatalf("pending after %s = %#v, %v", point, pending, err)
			}
			if issues, err := canonical.Validate(root); err != nil || len(issues) != 0 {
				t.Fatalf("canonical authority after %s = %#v, %v", point, issues, err)
			}
		})
	}
}

func TestMutationDiskFullDuringStagingPreservesAuthority(t *testing.T) {
	root := newWorkspace(t)
	set := WriteSet{OperationID: "OP-DISK-FULL", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-DISK-FULL.yaml", Contents: []byte("id: SRC-DISK-FULL\n")}}}
	_, err := CommitWithOptions(root, set, Options{Fault: func(point FaultPoint) error {
		if point == FaultStagedFile {
			return syscall.ENOSPC
		}
		return nil
	}})
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("disk-full error = %v", err)
	}
	if err := RollForward(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "SRC-DISK-FULL.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed staged write changed canonical authority: %v", err)
	}
	if issues, err := canonical.Validate(root); err != nil || len(issues) != 0 {
		t.Fatalf("canonical validation = %#v, %v", issues, err)
	}
}

func TestMutationReadOnlyWorkspaceFailsClosedWhereSupported(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permission enforcement is not reliable on this platform or as root")
	}
	root := newWorkspace(t)
	control := filepath.Join(root, ".skillhub")
	info, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(control, 0o500); err != nil {
		t.Skipf("read-only permission setup unsupported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(control, info.Mode().Perm()) })
	_, err = Commit(root, WriteSet{OperationID: "OP-READ-ONLY", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-READ-ONLY.yaml", Contents: []byte("id: SRC-READ-ONLY\n")}}})
	if err == nil {
		t.Fatal("read-only transaction directory accepted a mutation")
	}
	if _, statErr := os.Stat(filepath.Join(root, "sources", "catalog", "SRC-READ-ONLY.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("permission failure changed canonical authority: %v", statErr)
	}
}

func hardeningReplacementSet(t *testing.T, root, operationID string) WriteSet {
	t.Helper()
	changes := make([]Change, 0, 3)
	for _, id := range []string{"A", "B", "C"} {
		path := "sources/catalog/SRC-HARDEN-" + id + ".yaml"
		before := []byte("id: SRC-HARDEN-" + id + "\nname: old\n")
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), before, 0o640); err != nil {
			t.Fatal(err)
		}
		changes = append(changes, Change{Path: path, BeforeDigest: digest(before), Contents: []byte("id: SRC-HARDEN-" + id + "\nname: new\n")})
	}
	return WriteSet{OperationID: operationID, Command: "replace_sources", Changes: changes}
}

func assertHardeningReplacementState(t *testing.T, root string, wantNew bool) {
	t.Helper()
	name := "old"
	if wantNew {
		name = "new"
	}
	for _, id := range []string{"A", "B", "C"} {
		path := filepath.Join(root, "sources", "catalog", "SRC-HARDEN-"+id+".yaml")
		got, err := os.ReadFile(path)
		want := "id: SRC-HARDEN-" + id + "\nname: " + name + "\n"
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestCommitRejectsMutationWhoseReceiptWouldExceedAggregateLimit(t *testing.T) {
	root := newWorkspace(t)
	path := "sources/catalog/SRC-NEAR-LIMIT.yaml"
	before := []byte("id: SRC-NEAR-LIMIT\nname: old\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), before, 0o640); err != nil {
		t.Fatal(err)
	}
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	var used int64
	for _, relative := range files {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		used += info.Size()
	}
	// Leave less room than the operation receipt needs, but enough for the edit itself.
	remaining := workspace.MaxCanonicalBytesV1 - used - 16
	for index := 0; remaining > 0; index++ {
		size := min(remaining, workspace.MaxCanonicalFileBytesV1)
		pad, err := os.Create(filepath.Join(root, "sources", "intake", fmt.Sprintf("pad-%02d.bin", index)))
		if err != nil {
			t.Fatal(err)
		}
		if err := pad.Truncate(size); err != nil {
			t.Fatal(err)
		}
		if err := pad.Close(); err != nil {
			t.Fatal(err)
		}
		remaining -= size
	}
	_, err = Commit(root, WriteSet{OperationID: "OP-NEAR-LIMIT", Command: "replace_source", Changes: []Change{
		{Path: path, BeforeDigest: digest(before), Contents: []byte("id: SRC-NEAR-LIMIT\nname: new\n")},
	}})
	if err == nil || !strings.Contains(err.Error(), "aggregate limit") {
		t.Fatalf("Commit error = %v; want aggregate limit rejection before any write", err)
	}
	pending, pendingErr := pendingUnlocked(root)
	if pendingErr != nil || len(pending) != 0 {
		t.Fatalf("pending transactions = %v, %v; want none", pending, pendingErr)
	}
	if got, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); readErr != nil || string(got) != string(before) {
		t.Fatalf("source after rejected commit = %q, %v", got, readErr)
	}
}
