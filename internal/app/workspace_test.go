package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

func TestInitRequiresConfirmationBeforeMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	service := WorkspaceService{}
	preview, err := service.Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != StatusActionRequired {
		t.Fatalf("unexpected init preview status: %s", preview.Status)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("init preview wrote workspace: %v", err)
	}
	if _, err := service.Init(root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatalf("confirmed init did not initialize workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "catalog", "current.json")); err != nil {
		t.Fatalf("confirmed init did not publish a catalog generation: %v", err)
	}
}

func TestRemediationRejectsSkillhubSymlink(t *testing.T) {
	service := WorkspaceService{}
	for _, remediation := range []struct {
		name  string
		apply func(string) (Result, error)
	}{
		{name: "init", apply: func(path string) (Result, error) { return service.Init(path, true) }},
		{name: "doctor fix", apply: func(path string) (Result, error) { return service.DoctorFix(path, true) }},
	} {
		t.Run(remediation.name, func(t *testing.T) {
			root := t.TempDir()
			external := t.TempDir()
			if err := os.Symlink(external, filepath.Join(root, ".skillhub")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := remediation.apply(root); err == nil {
				t.Fatal("remediation accepted .skillhub symlink")
			}
			if _, err := os.Stat(filepath.Join(external, "schema-version")); !os.IsNotExist(err) {
				t.Fatalf("remediation wrote through .skillhub symlink: %v", err)
			}
		})
	}
}

func TestDoctorFixRebuildsMissingRuntime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	service := WorkspaceService{}
	if _, err := service.Init(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DoctorFix(root, true); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"runtime/catalog/current.json", "runtime/operational.db", "runtime/telemetry.db"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("doctor --fix did not recreate %s: %v", relative, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "history", "operations"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("runtime-only doctor repair created an operation receipt: %#v, %v", entries, err)
	}
}

func TestDoctorHealthyWorkspaceCreatesOnlyAdvisoryLockState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := (WorkspaceService{}).Doctor(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "locks", "workspace.lock")); err != nil {
		t.Fatalf("doctor did not hold the required shared workspace lock: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "history", "operations"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("read-only doctor created canonical operation state: %#v, %v", entries, err)
	}
}

func TestDoctorIsReadOnlyAndFixRequiresConfirmation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	service := WorkspaceService{}
	preview, err := service.Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != StatusActionRequired {
		t.Fatalf("unexpected doctor status: %s", preview.Status)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("doctor wrote workspace: %v", err)
	}
	refusal, err := service.DoctorFix(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if refusal.Status != StatusActionRequired {
		t.Fatalf("unexpected refusal status: %s", refusal.Status)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("doctor --fix without --yes wrote workspace: %v", err)
	}
	applied, err := service.DoctorFix(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != StatusApplied {
		t.Fatalf("unexpected applied status: %s", applied.Status)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatalf("fix did not initialize workspace: %v", err)
	}
}

func TestExistingWorkspaceDoctorFixUsesMutationReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	service := WorkspaceService{}
	if _, err := service.Init(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("outdated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DoctorFix(root, true); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(root, ".skillhub", "schema-version")); err != nil || string(contents) != "1\n" {
		t.Fatalf("schema remediation = %q, %v", contents, err)
	}
	var receipts int
	err := filepath.WalkDir(filepath.Join(root, "history", "operations"), func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			receipts++
		}
		return nil
	})
	if err != nil || receipts != 1 {
		t.Fatalf("doctor remediation receipts = %d, %v", receipts, err)
	}
}

func TestDoctorPreviewsRecoveryConflictWithoutOfferingFix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("interrupt after replacement")
	_, err := mutation.CommitWithOptions(root, mutation.WriteSet{
		OperationID: "OP-CONFLICT-PREVIEW",
		Command:     "create_source",
		Changes: []mutation.Change{{
			Path:     "sources/catalog/SRC-CONFLICT-PREVIEW.yaml",
			Contents: []byte("id: SRC-CONFLICT-PREVIEW\n"),
		}},
	}, mutation.Options{Fault: func(point mutation.FaultPoint) error {
		if point == mutation.FaultFirstCanonicalReplace {
			return failure
		}
		return nil
	}})
	if !errors.Is(err, failure) {
		t.Fatalf("commit error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources/catalog/SRC-CONFLICT-PREVIEW.yaml"), []byte("id: SRC-EXTERNAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (WorkspaceService{}).Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusRecoveryRequired || len(result.Items) != 1 || len(result.SuggestedActions) != 0 {
		t.Fatalf("conflicted recovery preview = %#v", result)
	}
}

func TestDoctorReportsAndFixesPendingRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("leave prepared transaction")
	_, err := mutation.CommitWithOptions(root, mutation.WriteSet{OperationID: "OP-PENDING", Command: "create_source", Changes: []mutation.Change{{Path: "sources/catalog/SRC-PENDING.yaml", Contents: []byte("schema_version: 1\nid: SRC-PENDING\nadapter: git\nlocator: https://example.invalid/repo\n")}}}, mutation.Options{Fault: func(point mutation.FaultPoint) error {
		if point == mutation.FaultManifestPhaseUpdate {
			return injected
		}
		return nil
	}})
	if !errors.Is(err, injected) {
		t.Fatalf("commit error = %v", err)
	}
	service := WorkspaceService{}
	result, err := service.Doctor(root)
	if err != nil || result.Status != StatusRecoveryRequired || result.Error == nil || result.Error.Code != ErrorRecoveryRequired {
		t.Fatalf("doctor recovery result = %#v, %v", result, err)
	}
	if _, err := service.DoctorFix(root, true); err != nil {
		t.Fatal(err)
	}
	if pending, err := mutation.Pending(root); err != nil || len(pending) != 0 {
		t.Fatalf("pending after fix = %#v, %v", pending, err)
	}
}
