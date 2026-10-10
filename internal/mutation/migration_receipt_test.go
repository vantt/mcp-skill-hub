package mutation

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestMigrationReceiptRoundTripPreservesStructuredVersions(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatal(err)
	}
	source, target := 0, 5
	set := WriteSet{
		OperationID: "OP-MIGRATION-RECEIPT", Command: "canonical_migration", IdempotencyKey: "migration:0:5",
		SourceSchemaVersion: &source, TargetSchemaVersion: &target,
		Changes: []Change{{Path: ".skillhub/schema-version", Contents: []byte("5\n")}},
	}
	set.RequestDigest = requestDigest(set)
	proposal, err := PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ConfirmMutation(root, proposal, Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseCatalogSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	if applied.SourceSchemaVersion == nil || applied.TargetSchemaVersion == nil || *applied.SourceSchemaVersion != 0 || *applied.TargetSchemaVersion != 5 {
		t.Fatalf("immediate receipt = %#v", applied)
	}
	retry, found, err := LookupOperation(root, set)
	if err != nil || !found || retry.SourceSchemaVersion == nil || retry.TargetSchemaVersion == nil || *retry.SourceSchemaVersion != 0 || *retry.TargetSchemaVersion != 5 {
		t.Fatalf("retry receipt = %#v, found=%v, err=%v", retry, found, err)
	}
	if retry.OperationID != applied.OperationID || !reflect.DeepEqual(retry.ChangedPaths, applied.ChangedPaths) || retry.CatalogSnapshot != applied.CatalogSnapshot || retry.GitDirty != applied.GitDirty {
		t.Fatalf("retry receipt lost applied evidence:\napplied=%#v\nretry=%#v", applied, retry)
	}
}

func TestMutationRejectsPartialSchemaVersionMetadata(t *testing.T) {
	t.Parallel()
	source := 0
	err := validWriteSet(WriteSet{OperationID: "OP-PARTIAL", Command: "migration", SourceSchemaVersion: &source, Changes: []Change{{Path: ".skillhub/schema-version", Contents: []byte("1\n")}}}, true)
	if err == nil {
		t.Fatal("partial migration version metadata was accepted")
	}
}
