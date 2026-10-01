package migration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestPreviewIsReadOnlyAndPinsLegacyMarkerDiff(t *testing.T) {
	root := legacyWorkspace(t)
	proposal, err := DefaultRegistry().Preview(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SourceVersion != 0 || proposal.TargetVersion != 1 || len(proposal.Changes) != 1 || !strings.Contains(proposal.Changes[0].Diff, "+1") {
		t.Fatalf("proposal = %#v", proposal)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview wrote marker: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "history", "operations")); err != nil || len(entries) != 0 {
		t.Fatalf("preview wrote receipt: %#v, %v", entries, err)
	}
}

func TestConfirmationRejectsStaleMigrationProposal(t *testing.T) {
	root := legacyWorkspace(t)
	proposal, err := DefaultRegistry().Preview(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot})
	if !errors.Is(err, mutation.ErrConflict) {
		t.Fatalf("confirmation error = %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(root, ".skillhub", "schema-version")); err != nil || string(contents) != "0\n" {
		t.Fatalf("stale confirmation changed marker: %q, %v", contents, err)
	}
}

func TestMigrationReceiptVersionsAndIdempotence(t *testing.T) {
	root := legacyWorkspace(t)
	proposal, err := DefaultRegistry().Preview(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID == "" {
		t.Fatal("missing operation ID")
	}
	path := findReceipt(t, root)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "source_schema_version: 0\n") || !strings.Contains(text, "target_schema_version: 1\n") {
		t.Fatalf("receipt lacks structured versions:\n%s", text)
	}
	if _, err := DefaultRegistry().Preview(root, 1); !errors.Is(err, ErrAlreadyCurrent) {
		t.Fatalf("second preview error = %v", err)
	}
}

func TestMigrationRecoversAtEveryApplicableMutationFault(t *testing.T) {
	points := []mutation.FaultPoint{
		mutation.FaultTransactionCreated,
		mutation.FaultStagedFile,
		mutation.FaultManifestPhaseUpdate,
		mutation.FaultFirstCanonicalReplace,
		mutation.FaultReceiptWrite,
		mutation.FaultCanonicalValidation,
		mutation.FaultJournalCleanup,
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			root := legacyWorkspace(t)
			proposal, err := DefaultRegistry().Preview(root, 1)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected migration crash")
			_, err = mutation.ConfirmMutationWithOptions(root, proposal.Mutation, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot}, mutation.Options{Fault: func(actual mutation.FaultPoint) error {
				if actual == point {
					return injected
				}
				return nil
			}})
			if !errors.Is(err, injected) {
				t.Fatalf("fault %s was not reached: %v", point, err)
			}
			if err := mutation.RollForward(root); err != nil {
				t.Fatal(err)
			}
			version, err := DetectVersion(root)
			if err != nil {
				t.Fatal(err)
			}
			if version == 0 {
				fresh, err := DefaultRegistry().Preview(root, 1)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := mutation.ConfirmMutation(root, fresh.Mutation, mutation.Confirmation{ProposalID: fresh.ID, ProposalDigest: fresh.Digest, BaseCatalogSnapshot: fresh.BaseSnapshot}); err != nil {
					t.Fatal(err)
				}
			}
			if version, err = DetectVersion(root); err != nil || version != 1 {
				t.Fatalf("recovered version = %d, %v", version, err)
			}
			if pending, err := mutation.Pending(root); err != nil || len(pending) != 0 {
				t.Fatalf("pending = %#v, %v", pending, err)
			}
		})
	}
}

func TestLegacyMigrationRejectsOtherInvalidLayout(t *testing.T) {
	root := legacyWorkspace(t)
	if err := os.RemoveAll(filepath.Join(root, "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DefaultRegistry().Preview(root, 1); err == nil || !strings.Contains(err.Error(), "not otherwise v1-compatible") {
		t.Fatalf("invalid legacy layout error = %v", err)
	}
}

func legacyWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatal(err)
	}
	return root
}

func findReceipt(t *testing.T, root string) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(filepath.Join(root, "history", "operations"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			found = path
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("find receipt = %q, %v", found, err)
	}
	return found
}

func TestDetectVersionRejectsSymlinkedMarkerWithoutEchoingContents(t *testing.T) {
	root := legacyWorkspace(t)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("TOPSECRET-API-KEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := DetectVersion(root); err == nil || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("DetectVersion error = %v", err)
	}
}

func TestDetectVersionInvalidMarkerErrorOmitsMarkerValue(t *testing.T) {
	root := legacyWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("TOPSECRET-VALUE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DetectVersion(root); err == nil || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("DetectVersion error = %v", err)
	}
}
