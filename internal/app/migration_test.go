package app

import (
	"encoding/json"
	"errors"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
)

func TestMigrationServicePreviewThenApply(t *testing.T) {
	t.Parallel()
	root := newLegacyAppWorkspace(t)
	service := MigrationService{}
	preview, err := service.Migrate(t.Context(), root, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != StatusActionRequired || preview.SourceSchemaVersion != 0 || preview.TargetSchemaVersion != 2 || preview.ProposalDigest == "" || len(preview.Changes) != 1 {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); !os.IsNotExist(err) {
		t.Fatalf("preview changed canonical marker: %v", err)
	}

	applied, err := service.Migrate(t.Context(), root, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != StatusApplied || applied.Receipt == nil || applied.Receipt.SourceSchemaVersion != 0 || applied.Receipt.TargetSchemaVersion != 2 || applied.Receipt.Generation == "" {
		t.Fatalf("applied = %#v", applied)
	}
	if status, err := catalog.Inspect(t.Context(), root); err != nil || status.State != catalog.StateHealthy {
		t.Fatalf("catalog = %#v, %v", status, err)
	}

	currentPreview, err := service.Migrate(t.Context(), root, 2, false)
	if err != nil || currentPreview.Status != StatusReady || currentPreview.Receipt != nil {
		t.Fatalf("already-current preview = %#v, %v", currentPreview, err)
	}

	retry, err := service.Migrate(t.Context(), root, 2, true)
	if err != nil || retry.Status != StatusApplied || retry.Receipt == nil {
		t.Fatalf("idempotent retry = %#v, %v", retry, err)
	}
	if retry.SourceSchemaVersion != applied.SourceSchemaVersion || retry.TargetSchemaVersion != applied.TargetSchemaVersion ||
		retry.Receipt.OperationID != applied.Receipt.OperationID ||
		!reflect.DeepEqual(retry.Receipt.ChangedPaths, applied.Receipt.ChangedPaths) ||
		retry.Receipt.CatalogSnapshot != applied.Receipt.CatalogSnapshot || retry.Receipt.Generation != applied.Receipt.Generation ||
		retry.Receipt.GitDirty != applied.Receipt.GitDirty ||
		retry.Receipt.SourceSchemaVersion != applied.Receipt.SourceSchemaVersion || retry.Receipt.TargetSchemaVersion != applied.Receipt.TargetSchemaVersion {
		t.Fatalf("retry receipt lost applied evidence:\napplied=%#v\nretry=%#v", applied, retry)
	}
}

func TestDoctorDirectsLegacyWorkspaceToMigrationWithoutChangingIt(t *testing.T) {
	t.Parallel()
	root := newLegacyAppWorkspace(t)
	result, err := (WorkspaceService{}).DoctorFix(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusActionRequired || len(result.SuggestedActions) != 1 || !strings.Contains(result.SuggestedActions[0].Command, "skillhub migrate") {
		t.Fatalf("doctor result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); !os.IsNotExist(err) {
		t.Fatalf("doctor changed canonical marker: %v", err)
	}
}

func TestRebuildNeverUpgradesIncompatibleCanonicalVersion(t *testing.T) {
	t.Parallel()
	root := newLegacyAppWorkspace(t)
	if _, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root); err == nil {
		t.Fatal("rebuild accepted incompatible canonical version")
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); !os.IsNotExist(err) {
		t.Fatalf("rebuild changed canonical marker: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "history", "operations"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("rebuild created canonical receipts: %#v, %v", entries, err)
	}
}

func TestDerivedOnlyIncompatibilityAutoRebuildsWhenCanonicalIsValid(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	markerBefore, err := os.ReadFile(filepath.Join(root, ".skillhub", "schema-version"))
	if err != nil {
		t.Fatal(err)
	}
	pointerPath := filepath.Join(root, "runtime", "catalog", "current.json")
	contents, err := os.ReadFile(pointerPath)
	if err != nil {
		t.Fatal(err)
	}
	var pointer map[string]any
	if err := json.Unmarshal(contents, &pointer); err != nil {
		t.Fatal(err)
	}
	oldGeneration, _ := pointer["generation"].(string)
	pointer["derived_schema_version"] = float64(catalog.DerivedSchemaVersion + 100)
	contents, err = json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pointerPath, append(contents, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (CatalogService{}).EnsureCatalog(t.Context(), root)
	if err != nil || result.Status != StatusApplied {
		t.Fatalf("ensure = %#v, %v", result, err)
	}
	status, err := catalog.Inspect(t.Context(), root)
	if err != nil || status.State != catalog.StateHealthy || status.Pointer.Generation == oldGeneration {
		t.Fatalf("rebuilt status = %#v, %v", status, err)
	}
	markerAfter, err := os.ReadFile(filepath.Join(root, ".skillhub", "schema-version"))
	if err != nil || string(markerAfter) != string(markerBefore) {
		t.Fatalf("derived rebuild changed canonical marker: %q, %v", markerAfter, err)
	}
}

func newLegacyAppWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDoctorFixDoesNotWriteMarkerForLegacyCloneWithoutControlDirectory(t *testing.T) {
	t.Parallel()
	root := newLegacyAppWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "SRC-CLONE.yaml"), []byte("id: SRC-CLONE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Git does not track empty directories, so a fresh clone has no .skillhub at all.
	if err := os.RemoveAll(filepath.Join(root, ".skillhub")); err != nil {
		t.Fatal(err)
	}
	result, err := (WorkspaceService{}).DoctorFix(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusActionRequired || len(result.SuggestedActions) != 1 || !strings.Contains(result.SuggestedActions[0].Command, "skillhub migrate") {
		t.Fatalf("doctor result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); !os.IsNotExist(err) {
		t.Fatalf("doctor wrote the schema marker: %v", err)
	}
}

func TestMigrateReplayFailsWhenMarkerWasLostAfterApply(t *testing.T) {
	t.Parallel()
	root := newLegacyAppWorkspace(t)
	service := MigrationService{}
	if _, err := service.Migrate(t.Context(), root, 2, true); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".skillhub", "schema-version")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	result, err := service.Migrate(t.Context(), root, 2, true)
	if err == nil || !errors.Is(err, mutation.ErrConflict) {
		t.Fatalf("replay after marker loss = %#v, %v; want conflict", result, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("replay changed marker state: %v", statErr)
	}
}

func TestMigrationServiceV1ToV2(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".skillhub", "schema-version")
	if err := os.WriteFile(marker, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := MigrationService{}
	preview, err := service.Migrate(t.Context(), root, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceSchemaVersion != 1 || preview.TargetSchemaVersion != 2 {
		t.Fatalf("preview versions = %d to %d, want 1 to 2", preview.SourceSchemaVersion, preview.TargetSchemaVersion)
	}
	applied, err := service.Migrate(t.Context(), root, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != StatusApplied || applied.Receipt.TargetSchemaVersion != 2 {
		t.Fatalf("applied result = %#v", applied)
	}
	data, err := os.ReadFile(marker)
	if err != nil || strings.TrimSpace(string(data)) != "2" {
		t.Fatalf("marker after migration = %q, %v", string(data), err)
	}
}
