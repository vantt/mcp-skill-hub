package mutation

import (
	"context"
	"errors"
	"fmt"
	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestPlanAndConfirmPinProposalAndCanonicalState(t *testing.T) {
	root := newWorkspace(t)
	set := WriteSet{OperationID: "OP-PLAN", Command: "create_source", IdempotencyKey: "create:SRC-PLAN", Changes: []Change{{Path: "sources/catalog/SRC-PLAN.yaml", Contents: []byte("id: SRC-PLAN\n")}}}
	proposal, err := PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ID == "" || proposal.Digest == "" || proposal.BaseCatalogSnapshot == "" || proposal.WriteSet.Changes[0].BeforeDigest != "" {
		t.Fatalf("proposal pins are incomplete: %#v", proposal)
	}
	confirmation := Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseCatalogSnapshot}
	tampered := proposal
	tampered.WriteSet.Changes[0].Contents = []byte("id: SRC-TAMPERED\n")
	if _, err := ConfirmMutation(root, tampered, confirmation); !errors.Is(err, ErrConflict) {
		t.Fatalf("tampered proposal error = %v, want conflict", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources/catalog/SRC-PLAN.yaml"), []byte("id: SRC-EXTERNAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmMutation(root, proposal, confirmation); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale proposal error = %v, want conflict", err)
	}
}

func TestIdempotentRetryReturnsFaithfulStructuredReceipt(t *testing.T) {
	root := newWorkspace(t)
	set := WriteSet{OperationID: "OP-FIRST", Command: "create_source", IdempotencyKey: "source:create:one", Changes: []Change{{Path: "sources/catalog/SRC-ONE.yaml", Contents: []byte("id: SRC-ONE\nname: 'id: OP-SECOND'\n")}}}
	first, err := Commit(root, set)
	if err != nil {
		t.Fatal(err)
	}
	retrySet := set
	retrySet.OperationID = "OP-SECOND"
	retry, err := Commit(root, retrySet)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retry, first) {
		t.Fatalf("retry receipt = %#v, want %#v", retry, first)
	}
	conflict := retrySet
	conflict.Changes[0].Contents = []byte("id: SRC-DIFFERENT\n")
	if _, err := Commit(root, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different idempotent request error = %v", err)
	}
}

func TestRollbackRestoresBeforeImagesForPartialReplacement(t *testing.T) {
	root := newWorkspace(t)
	original := []byte("id: SRC-EXISTING\nname: original\n")
	path := "sources/catalog/SRC-EXISTING.yaml"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), original, 0o644); err != nil {
		t.Fatal(err)
	}
	set := WriteSet{OperationID: "OP-RESTORE", Command: "replace_source", Changes: []Change{
		{Path: path, BeforeDigest: digest(original), Contents: []byte("id: SRC-EXISTING\nname: replacement\n")},
		{Path: "sources/catalog/SRC-NEW.yaml", Contents: []byte("id: SRC-NEW\n")},
	}}
	failure := errors.New("stop after first replacement")
	_, err := CommitWithOptions(root, set, Options{Fault: func(point FaultPoint) error {
		if point == FaultFirstCanonicalReplace {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	if err := RollBack(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("restored contents = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "sources/catalog/SRC-NEW.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new path remains after rollback: %v", err)
	}
}

func TestRollbackRestoresPartiallyAppliedTransaction(t *testing.T) {
	root := newWorkspace(t)
	set := threeFileWriteSet("OP-ROLLBACK")
	injected := errors.New("stop after first replacement")
	_, err := CommitWithOptions(root, set, Options{Fault: func(point FaultPoint) error {
		if point == FaultFirstCanonicalReplace {
			return injected
		}
		return nil
	}})
	if !errors.Is(err, injected) {
		t.Fatalf("commit error = %v", err)
	}
	if err := RollBack(root); err != nil {
		t.Fatal(err)
	}
	assertOldOrNewState(t, root, false)
}

func TestFaultInjectionAlwaysRecoversToOldOrNewState(t *testing.T) {
	tests := []struct {
		name       string
		point      FaultPoint
		occurrence int
	}{
		{"transaction directory", FaultTransactionCreated, 1},
		{"first staged file", FaultStagedFile, 1},
		{"last staged file", FaultStagedFile, 4},
		{"prepared manifest", FaultManifestPhaseUpdate, 1},
		{"before displacement", FaultBeforeTargetDisplace, 1},
		{"first replacement", FaultFirstCanonicalReplace, 1},
		{"middle replacement", FaultMiddleCanonicalReplace, 1},
		{"last replacement", FaultLastCanonicalReplace, 1},
		{"receipt", FaultReceiptWrite, 1},
		{"post validation", FaultCanonicalValidation, 1},
		{"canonical phase manifest", FaultManifestPhaseUpdate, 2},
		{"finalized manifest", FaultManifestPhaseUpdate, 3},
		{"journal cleanup", FaultJournalCleanup, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newWorkspace(t)
			set := threeFileWriteSet("OP-FAULT")
			seen := 0
			failure := fmt.Errorf("injected %s", test.point)
			_, err := CommitWithOptions(root, set, Options{Fault: func(point FaultPoint) error {
				if point == test.point {
					seen++
					if seen == test.occurrence {
						return failure
					}
				}
				return nil
			}})
			if !errors.Is(err, failure) {
				t.Fatalf("commit error = %v, injected point seen %d time(s)", err, seen)
			}
			if err := RollForward(root); err != nil {
				t.Fatal(err)
			}
			assertOldOrNewState(t, root, test.point != FaultTransactionCreated && test.point != FaultStagedFile)
			if pending, err := Pending(root); err != nil || len(pending) != 0 {
				t.Fatalf("pending = %#v, %v", pending, err)
			}
			if issues, err := canonical.Validate(root); err != nil || len(issues) != 0 {
				t.Fatalf("validation = %#v, %v", issues, err)
			}
		})
	}
}

func TestPlanAllocatesConfirmableUniqueOperationIDs(t *testing.T) {
	root := newWorkspace(t)
	set := WriteSet{Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-ALLOCATED.yaml", Contents: []byte("id: SRC-ALLOCATED\n")}}}
	first, err := PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	if !validOpaqueID(first.WriteSet.OperationID) || first.WriteSet.OperationID == second.WriteSet.OperationID {
		t.Fatalf("allocated operation IDs are not valid and unique: %q, %q", first.WriteSet.OperationID, second.WriteSet.OperationID)
	}
	result, err := ConfirmMutation(root, first, Confirmation{ProposalID: first.ID, ProposalDigest: first.Digest, BaseCatalogSnapshot: first.BaseCatalogSnapshot})
	if err != nil || result.OperationID != first.WriteSet.OperationID {
		t.Fatalf("confirm allocated proposal = %#v, %v", result, err)
	}
}

func TestLostResponseReplanReturnsOriginalResult(t *testing.T) {
	root := newWorkspace(t)
	payload := WriteSet{Command: "create_source", IdempotencyKey: "lost-response", Changes: []Change{{Path: "sources/catalog/SRC-LOST.yaml", Contents: []byte("id: SRC-LOST\n")}}}
	firstPlan, err := PlanMutation(root, payload)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ConfirmMutation(root, firstPlan, Confirmation{ProposalID: firstPlan.ID, ProposalDigest: firstPlan.Digest, BaseCatalogSnapshot: firstPlan.BaseCatalogSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	replanned, err := PlanMutation(root, payload)
	if err != nil {
		t.Fatal(err)
	}
	if replanned.WriteSet.OperationID == first.OperationID || replanned.Digest == firstPlan.Digest || replanned.WriteSet.Changes[0].BeforeDigest == "" {
		t.Fatal("test did not produce a fresh operation and changed planning pins")
	}
	retry, err := ConfirmMutation(root, replanned, Confirmation{ProposalID: replanned.ID, ProposalDigest: replanned.Digest, BaseCatalogSnapshot: replanned.BaseCatalogSnapshot})
	if err != nil || !reflect.DeepEqual(retry, first) {
		t.Fatalf("lost-response retry = %#v, %v; want %#v", retry, err, first)
	}
	changed := payload
	changed.Changes = []Change{{Path: "sources/catalog/SRC-LOST.yaml", Contents: []byte("id: SRC-OTHER\n")}}
	changedPlan, err := PlanMutation(root, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmMutation(root, changedPlan, Confirmation{ProposalID: changedPlan.ID, ProposalDigest: changedPlan.Digest, BaseCatalogSnapshot: changedPlan.BaseCatalogSnapshot}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed payload error = %v", err)
	}
}

func TestRecoveryContinuesAfterTargetWasDisplaced(t *testing.T) {
	root := newWorkspace(t)
	path := "sources/catalog/SRC-DISPLACED.yaml"
	before := []byte("id: SRC-DISPLACED\nname: old\n")
	after := []byte("id: SRC-DISPLACED\nname: new\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), before, 0o640); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("crash after displacement")
	_, err := CommitWithOptions(root, WriteSet{OperationID: "OP-DISPLACED", Command: "replace_source", Changes: []Change{{Path: path, BeforeDigest: digest(before), Contents: after}}}, Options{Fault: func(point FaultPoint) error {
		if point == FaultAfterTargetDisplace {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	recoveries, err := InspectRecovery(root)
	if err != nil || len(recoveries) != 1 || recoveries[0].Action != RecoveryRollForward || recoveries[0].Paths[0].State != PathBefore {
		t.Fatalf("displaced classification = %#v, %v", recoveries, err)
	}
	if err := RollForward(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil || !reflect.DeepEqual(got, after) {
		t.Fatalf("roll-forward result = %q, %v", got, err)
	}
}

func TestLostResponseAfterJournalCleanupReplanReturnsReceipt(t *testing.T) {
	root := newWorkspace(t)
	payload := WriteSet{Command: "create_source", IdempotencyKey: "lost-after-cleanup", Changes: []Change{{Path: "sources/catalog/SRC-CLEANUP.yaml", Contents: []byte("id: SRC-CLEANUP\n")}}}
	planned, err := PlanMutation(root, payload)
	if err != nil {
		t.Fatal(err)
	}
	lost := errors.New("response lost")
	_, err = CommitWithOptions(root, planned.WriteSet, Options{Fault: func(point FaultPoint) error {
		if point == FaultJournalCleanup {
			return lost
		}
		return nil
	}})
	if !errors.Is(err, lost) {
		t.Fatalf("commit error = %v", err)
	}
	if pending, err := Pending(root); err != nil || len(pending) != 0 {
		t.Fatalf("cleanup left pending transactions: %#v, %v", pending, err)
	}
	replanned, err := PlanMutation(root, payload)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ConfirmMutation(root, replanned, Confirmation{ProposalID: replanned.ID, ProposalDigest: replanned.Digest, BaseCatalogSnapshot: replanned.BaseCatalogSnapshot})
	if err != nil || receipt.OperationID != planned.WriteSet.OperationID {
		t.Fatalf("lost-response receipt = %#v, %v", receipt, err)
	}
}

func TestPostCanonicalFailureRetainsRecoverableWALAndRecoveryPublishes(t *testing.T) {
	root := newWorkspace(t)
	set := WriteSet{OperationID: "OP-PUBLISH-FAIL", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-PUBLISH.yaml", Contents: []byte("schema_version: 1\nid: SRC-PUBLISH\nadapter: git\n")}}}
	proposal, err := PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	publishFailure := errors.New("publish unavailable")
	_, err = ConfirmMutationWithOptions(root, proposal, Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseCatalogSnapshot}, Options{PostCanonical: func(string) (Publication, error) {
		return Publication{}, publishFailure
	}})
	if !errors.Is(err, publishFailure) {
		t.Fatalf("confirm error = %v", err)
	}
	recoveries, inspectErr := InspectRecovery(root)
	if inspectErr != nil || len(recoveries) != 1 || recoveries[0].Phase != "canonical_applied" {
		t.Fatalf("recovery = %#v, %v", recoveries, inspectErr)
	}
	var published string
	if err := RollForwardWithOptions(root, Options{PostCanonical: func(expected string) (Publication, error) {
		published = expected
		return Publication{CatalogSnapshot: expected, Generation: "gen-recovered"}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if published == "" {
		t.Fatal("recovery did not publish the canonical result")
	}
	if pending, err := Pending(root); err != nil || len(pending) != 0 {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
}

func TestPostCanonicalCallbackKeepsConcurrentMutationOutsideLifecycle(t *testing.T) {
	root := newWorkspace(t)
	first, err := PlanMutation(root, WriteSet{OperationID: "OP-FIRST-PUBLISH", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-FIRST.yaml", Contents: []byte("schema_version: 1\nid: SRC-FIRST\nadapter: git\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := ConfirmMutationWithOptions(root, first, Confirmation{ProposalID: first.ID, ProposalDigest: first.Digest, BaseCatalogSnapshot: first.BaseCatalogSnapshot}, Options{PostCanonical: func(expected string) (Publication, error) {
			close(entered)
			<-release
			return Publication{CatalogSnapshot: expected, Generation: "gen-first"}, nil
		}})
		firstDone <- err
	}()
	<-entered
	secondDone := make(chan error, 1)
	go func() {
		_, err := Commit(root, WriteSet{OperationID: "OP-SECOND-PUBLISH", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-SECOND.yaml", Contents: []byte("schema_version: 1\nid: SRC-SECOND\nadapter: git\n")}}})
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("concurrent mutation completed during publication: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestTransactionFilesRemainRestrictive(t *testing.T) {
	root := newWorkspace(t)
	failure := errors.New("inspect prepared WAL")
	_, err := CommitWithOptions(root, WriteSet{OperationID: "OP-PRIVATE-WAL", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-PRIVATE.yaml", Contents: []byte("id: SRC-PRIVATE\n")}}}, Options{Fault: func(point FaultPoint) error {
		if point == FaultManifestPhaseUpdate {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	txn := filepath.Join(root, ".skillhub", "transactions", "OP-PRIVATE-WAL")
	err = filepath.WalkDir(txn, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		want := os.FileMode(0o600)
		if entry.IsDir() {
			want = 0o700
		}
		if runtime.GOOS == "windows" {
			return nil
		}
		if info.Mode().Perm()&^want != 0 {
			return fmt.Errorf("transaction path %s mode %o is broader than %o", path, info.Mode().Perm(), want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLateExternalEditIsDisplacedRevalidatedAndRestored(t *testing.T) {
	root := newWorkspace(t)
	path := "sources/catalog/SRC-RACE.yaml"
	original := []byte("id: SRC-RACE\nname: original\n")
	external := []byte("id: SRC-RACE\nname: external\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), original, 0o750); err != nil {
		t.Fatal(err)
	}
	fired := false
	_, err := CommitWithOptions(root, WriteSet{OperationID: "OP-RACE", Command: "replace_source", Changes: []Change{{Path: path, BeforeDigest: digest(original), Contents: []byte("id: SRC-RACE\nname: desired\n")}}}, Options{Fault: func(point FaultPoint) error {
		if point == FaultBeforeTargetDisplace && !fired {
			fired = true
			return os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), external, 0o750)
		}
		return nil
	}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("late edit error = %v", err)
	}
	got, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if readErr != nil || !reflect.DeepEqual(got, external) {
		t.Fatalf("late external bytes were not restored: %q, %v", got, readErr)
	}
}

func TestParentSymlinkReplacementFailsClosed(t *testing.T) {
	root := newWorkspace(t)
	external := t.TempDir()
	parent := filepath.Join(root, "sources", "catalog")
	moved := filepath.Join(root, "sources", "catalog-original")
	fired := false
	_, err := CommitWithOptions(root, WriteSet{OperationID: "OP-PARENT-RACE", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-PARENT.yaml", Contents: []byte("id: SRC-PARENT\n")}}}, Options{Fault: func(point FaultPoint) error {
		if point != FaultBeforeTargetDisplace || fired {
			return nil
		}
		fired = true
		if err := os.Rename(parent, moved); err != nil {
			return err
		}
		return os.Symlink(external, parent)
	}})
	if err == nil {
		t.Fatal("parent replacement was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(external, "SRC-PARENT.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mutation escaped through replaced parent: %v", statErr)
	}
}

func TestConcurrentWritersSerializeWithoutLostUpdates(t *testing.T) {
	root := newWorkspace(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"A", "B"} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := Commit(root, WriteSet{OperationID: "OP-CONCURRENT-" + id, Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-CONCURRENT-" + id + ".yaml", Contents: []byte("id: SRC-CONCURRENT-" + id + "\n")}}})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"A", "B"} {
		if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "SRC-CONCURRENT-"+id+".yaml")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPendingTransactionBlocksNormalWritesAndClassifiesProgress(t *testing.T) {
	root := newWorkspace(t)
	failure := errors.New("interrupt first path")
	_, err := CommitWithOptions(root, threeFileWriteSet("OP-PENDING-BLOCK"), Options{Fault: func(point FaultPoint) error {
		if point == FaultFirstCanonicalReplace {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	recoveries, err := InspectRecovery(root)
	if err != nil || len(recoveries) != 1 || recoveries[0].Action != RecoveryRollForward {
		t.Fatalf("recovery classification = %#v, %v", recoveries, err)
	}
	states := map[PathState]int{}
	for _, path := range recoveries[0].Paths {
		states[path.State]++
	}
	if states[PathAfter] == 0 || states[PathBefore] == 0 || states[PathUnknown] != 0 {
		t.Fatalf("unexpected path states: %#v", states)
	}
	_, err = Commit(root, WriteSet{OperationID: "OP-BLOCKED", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-BLOCKED.yaml", Contents: []byte("id: SRC-BLOCKED\n")}}})
	if !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("pending write error = %v", err)
	}
}

func TestRollbackAfterReceiptFaultRestoresReplaceDeleteAndCreate(t *testing.T) {
	root := newWorkspace(t)
	deletePath := "sources/catalog/SRC-A-DELETE.yaml"
	replacePath := "sources/catalog/SRC-B-REPLACE.yaml"
	deleted := []byte("id: SRC-A-DELETE\n")
	replaced := []byte("id: SRC-B-REPLACE\nname: old\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(deletePath)), deleted, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(replacePath)), replaced, 0o750); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("after receipt")
	_, err := CommitWithOptions(root, WriteSet{OperationID: "OP-LATE-ROLLBACK", Command: "mixed", Changes: []Change{
		{Path: deletePath, BeforeDigest: digest(deleted), Delete: true},
		{Path: replacePath, BeforeDigest: digest(replaced), Contents: []byte("id: SRC-B-REPLACE\nname: new\n")},
		{Path: "sources/catalog/SRC-C-CREATE.yaml", Contents: []byte("id: SRC-C-CREATE\n")},
	}}, Options{Fault: func(point FaultPoint) error {
		if point == FaultReceiptWrite {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	if err := RollBack(root); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{deletePath: deleted, replacePath: replaced} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("rollback %s = %q, %v", path, got, err)
		}
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(replacePath))); err != nil || info.Mode().Perm() != 0o750 {
			t.Fatalf("replacement mode after rollback = %v, %v", info, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "sources/catalog/SRC-C-CREATE.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created path remains: %v", err)
	}
}

func TestRollbackRefusesUnknownPathWithoutOverwriting(t *testing.T) {
	root := newWorkspace(t)
	failure := errors.New("partial apply")
	set := threeFileWriteSet("OP-UNKNOWN-ROLLBACK")
	_, err := CommitWithOptions(root, set, Options{Fault: func(point FaultPoint) error {
		if point == FaultFirstCanonicalReplace {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	path := filepath.Join(root, "sources", "catalog", "SRC-A.yaml")
	external := []byte("id: SRC-EXTERNAL-UNKNOWN\n")
	if err := os.WriteFile(path, external, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RollBack(root); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("rollback error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(got, external) {
		t.Fatalf("unknown external path was overwritten: %q, %v", got, err)
	}
}

func TestReplacementPreservesPermissionsAndNewFilesUseCanonicalDefault(t *testing.T) {
	root := newWorkspace(t)
	path := "sources/catalog/SRC-MODE.yaml"
	before := []byte("id: SRC-MODE\nname: old\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), before, 0o750); err != nil {
		t.Fatal(err)
	}
	_, err := Commit(root, WriteSet{OperationID: "OP-MODE", Command: "mode", Changes: []Change{
		{Path: path, BeforeDigest: digest(before), Contents: []byte("id: SRC-MODE\nname: new\n")},
		{Path: "sources/catalog/SRC-NEW-MODE.yaml", Contents: []byte("id: SRC-NEW-MODE\n")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for file, want := range map[string]os.FileMode{path: 0o750, "sources/catalog/SRC-NEW-MODE.yaml": 0o644} {
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(file)))
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("mode %s = %v, %v; want %v", file, info, err, want)
			}
		}
	}
}

func TestSharedAndExclusiveLocksHaveBoundedWait(t *testing.T) {
	root := newWorkspace(t)
	first, err := AcquireSharedLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unlock()
	second, err := AcquireSharedLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Unlock()
	started := time.Now()
	if _, err := AcquireExclusiveLock(context.Background(), root, 40*time.Millisecond); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("exclusive lock error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("exclusive lock did not respect bounded wait")
	}
}

func newWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func threeFileWriteSet(operationID string) WriteSet {
	return WriteSet{OperationID: operationID, Command: "create_sources", Changes: []Change{
		{Path: "sources/catalog/SRC-A.yaml", Contents: []byte("id: SRC-A\n")},
		{Path: "sources/catalog/SRC-B.yaml", Contents: []byte("id: SRC-B\n")},
		{Path: "sources/catalog/SRC-C.yaml", Contents: []byte("id: SRC-C\n")},
	}}
}

func assertOldOrNewState(t *testing.T, root string, wantNew bool) {
	t.Helper()
	present := 0
	for _, name := range []string{"SRC-A.yaml", "SRC-B.yaml", "SRC-C.yaml"} {
		_, err := os.Stat(filepath.Join(root, "sources", "catalog", name))
		if err == nil {
			present++
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	want := 0
	if wantNew {
		want = 3
	}
	if present != want {
		t.Fatalf("mixed canonical state: %d/3 files present, want %d/3", present, want)
	}
}
