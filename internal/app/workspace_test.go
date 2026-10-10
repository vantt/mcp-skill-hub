package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	workspacepkg "github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestInitRequiresConfirmationBeforeMutation(t *testing.T) {
	t.Parallel()
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

func TestDoctorHostIntegrationPreviewIsReadOnlyAndDependencyOrdered(t *testing.T) {
	t.Parallel()
	root := healthyWorkspaceWithoutHosts(t)
	result, err := (WorkspaceService{}).Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusActionRequired {
		t.Fatalf("host integration preview = %#v", result)
	}
	ranks := map[string]int{"permission-ownership": 0, "mcp-registration": 1, "host-permissions": 2, "native-skill": 3, "bootstrap-instructions": 4}
	lastRank := -1
	for _, item := range result.Items {
		parts := strings.SplitN(item.ID, "_", 3)
		if len(parts) != 3 {
			t.Fatalf("unexpected non-host finding: %#v", item)
		}
		rank, known := ranks[parts[2]]
		if !known || rank < lastRank || !strings.Contains(item.Summary, root) || !strings.Contains(item.Summary, "Managed diff preview:") {
			t.Fatalf("invalid or out-of-order preview item: %#v", item)
		}
		lastRank = rank
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("preview warnings = %#v, want 0", result.Warnings)
	}
	for _, relative := range hostArtifactPaths() {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("read-only doctor wrote %s: %v", relative, err)
		}
	}
}

func TestDoctorFixAppliesAllHostArtifactsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	root := healthyWorkspaceWithoutHosts(t)
	service := WorkspaceService{}
	applied, err := service.DoctorFix(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != StatusApplied {
		t.Fatalf("host apply status = %s", applied.Status)
	}
	before := make(map[string][]byte)
	for _, relative := range hostArtifactPaths() {
		path := filepath.Join(root, filepath.FromSlash(relative))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("host artifact %s: %v", relative, err)
		}
		before[relative] = content
	}
	for _, relative := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		if count := strings.Count(string(before[relative]), "<!-- skillhub:bootstrap:v1:start -->"); count != 1 {
			t.Fatalf("%s bootstrap block count = %d", relative, count)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(root, "history", "operations")); err != nil || len(entries) != 0 {
		t.Fatalf("host-only integration created canonical receipts: %#v, %v", entries, err)
	}

	if _, err := service.DoctorFix(root, true); err != nil {
		t.Fatal(err)
	}
	for relative, want := range before {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil || string(got) != string(want) {
			t.Fatalf("second fix changed %s: %v", relative, err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(root, "history", "operations")); err != nil || len(entries) != 0 {
		t.Fatalf("idempotent host fix created canonical receipts: %#v, %v", entries, err)
	}
}

func TestRemediationRejectsSkillhubSymlink(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestInitIsReadOnlyAndConfirmationInitializes(t *testing.T) {
	t.Parallel()
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
	applied, err := service.Init(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != StatusApplied {
		t.Fatalf("unexpected applied status: %s", applied.Status)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatalf("init did not initialize workspace: %v", err)
	}
}

func TestDoctorFixRejectsMissingWorkspace(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	service := WorkspaceService{}
	if _, err := service.Doctor(root); err == nil {
		t.Fatal("Doctor on missing path should fail")
	}
	if _, err := service.DoctorFix(root, true); err == nil {
		t.Fatal("DoctorFix on missing path should fail")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("DoctorFix created missing workspace: %v", err)
	}
}

func TestExistingWorkspaceDoctorFixNeverSilentlyChangesCanonicalVersion(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	service := WorkspaceService{}
	if _, err := service.Init(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("outdated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := service.DoctorFix(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusActionRequired || len(result.SuggestedActions) != 1 || !strings.Contains(result.SuggestedActions[0].Command, "skillhub migrate") {
		t.Fatalf("doctor result = %#v", result)
	}
	if contents, err := os.ReadFile(filepath.Join(root, ".skillhub", "schema-version")); err != nil || string(contents) != "outdated\n" {
		t.Fatalf("doctor changed schema marker = %q, %v", contents, err)
	}
	var receipts int
	err = filepath.WalkDir(filepath.Join(root, "history", "operations"), func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			receipts++
		}
		return nil
	})
	if err != nil || receipts != 0 {
		t.Fatalf("doctor created canonical receipts = %d, %v", receipts, err)
	}
}

func TestDoctorPreviewsRecoveryConflictWithoutOfferingFix(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func healthyWorkspaceWithoutHosts(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspacepkg.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (CatalogService{}).EnsureCatalog(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	return root
}

func hostArtifactPaths() []string {
	return []string{
		".mcp.json",
		".codex/config.toml",
		".gemini/settings.json",
		".claude/settings.local.json",
		".claude/skills/system-curator/SKILL.md",
		".agents/skills/system-curator/SKILL.md",
		".gemini/skills/system-curator/SKILL.md",
		"CLAUDE.md",
		"AGENTS.md",
		"GEMINI.md",
	}
}

func TestFreshCloneWithMissingLayoutDirectoriesGivesHealthyStatusAndValidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspacepkg.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (CatalogService{}).EnsureCatalog(ctx, root); err != nil {
		t.Fatal(err)
	}

	// Simulate fresh clone: delete all empty required canonical directories
	for _, dir := range workspacepkg.RequiredDirectories() {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(dir)))
	}

	// 1. validate must pass with 0 issues
	issues, err := canonical.Validate(root)
	if err != nil {
		t.Fatalf("validate error on clone: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues on clone-like workspace, got: %#v", issues)
	}

	// 2. status must report healthy valid workspace without statat errors
	home, err := (CurationService{}).GetCurationHome(ctx, root)
	if err != nil {
		t.Fatalf("status error on clone: %v", err)
	}
	if home.Workspace.Health != "valid" {
		t.Fatalf("expected workspace health 'valid', got %q", home.Workspace.Health)
	}

	// 3. doctor must run without errors
	result, err := (WorkspaceService{}).Doctor(root)
	if err != nil {
		t.Fatalf("doctor error on clone: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("doctor result error on clone: %#v", result.Error)
	}
}

func TestDoctorAndStatusReportHostIntegrationMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := healthyWorkspaceWithoutHosts(t)

	// 1. Doctor reports host integration missing as fixable with note to re-run skillhub integrate
	docResult, err := (WorkspaceService{}).Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if docResult.Status != StatusActionRequired {
		t.Fatalf("doctor status = %q, want action_required", docResult.Status)
	}
	if !strings.Contains(docResult.Summary, "host integration missing") || !strings.Contains(docResult.Summary, "re-run skillhub integrate after a pull that untracked those files") {
		t.Fatalf("doctor summary missing expected host integration text: %q", docResult.Summary)
	}
	hasFixableImpact := false
	for _, item := range docResult.Items {
		if strings.Contains(item.Impact, "Fixable") && strings.Contains(item.Impact, "re-run skillhub integrate after a pull that untracked those files") {
			hasFixableImpact = true
			break
		}
	}
	if !hasFixableImpact {
		t.Fatalf("doctor items missing fixable impact with integrate note: %#v", docResult.Items)
	}

	// 2. Status reports host_integration_missing action and finding with note to re-run skillhub integrate
	home, err := (CurationService{}).GetCurationHome(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if home.Status != StatusActionRequired {
		t.Fatalf("status = %q, want action_required", home.Status)
	}
	if !strings.Contains(home.Summary, "Host integration is missing; re-run skillhub integrate after a pull that untracked those files") {
		t.Fatalf("status summary missing expected note: %q", home.Summary)
	}
	hasStatusItem := false
	for _, item := range home.Items {
		if item.ID == "host_integration_missing" && strings.Contains(item.Impact, "re-run skillhub integrate after a pull that untracked those files") {
			hasStatusItem = true
			break
		}
	}
	if !hasStatusItem {
		t.Fatalf("status items missing host_integration_missing item: %#v", home.Items)
	}
}
