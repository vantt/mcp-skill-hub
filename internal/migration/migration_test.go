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
	proposal, err := DefaultRegistry().Preview(root, CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SourceVersion != 0 || proposal.TargetVersion != 5 || len(proposal.Changes) != 1 || !strings.Contains(proposal.Changes[0].Diff, "+5") {
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
	proposal, err := DefaultRegistry().Preview(root, CurrentVersion)
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
	proposal, err := DefaultRegistry().Preview(root, CurrentVersion)
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
	if !strings.Contains(text, "source_schema_version: 0\n") || !strings.Contains(text, "target_schema_version: 5\n") {
		t.Fatalf("receipt lacks structured versions:\n%s", text)
	}
	if _, err := DefaultRegistry().Preview(root, CurrentVersion); !errors.Is(err, ErrAlreadyCurrent) {
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
			proposal, err := DefaultRegistry().Preview(root, CurrentVersion)
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
				fresh, err := DefaultRegistry().Preview(root, CurrentVersion)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := mutation.ConfirmMutation(root, fresh.Mutation, mutation.Confirmation{ProposalID: fresh.ID, ProposalDigest: fresh.Digest, BaseCatalogSnapshot: fresh.BaseSnapshot}); err != nil {
					t.Fatal(err)
				}
			}
			if version, err = DetectVersion(root); err != nil || version != CurrentVersion {
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

func v2Workspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMigrationV2ToV3(t *testing.T) {
	root := v2Workspace(t)

	// Add a sample skill with skill.meta.yaml
	skillDir := filepath.Join(root, "skills", "core", "test-migrate")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "# Test Migrate\n\nMigrate test body.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	oldMeta := `schema_version: 1
id: test-migrate
name: Test Migrate
status: active
description: Migrate test description.
aliases: [migrate-test]
routing:
  operations: [review]
  triggers: [run migration]
  not_for: [none]
  min_scope: single_step
quality:
  reviewed: true
  content_reviewed_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
provenance:
  origin:
    kind: github
    repository: https://github.com/example/repo
    commit: 1111111111111111111111111111111111111111
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(oldMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	proposal, err := DefaultRegistry().Preview(root, CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SourceVersion != 2 || proposal.TargetVersion != 5 {
		t.Fatalf("proposal versions = %d -> %d", proposal.SourceVersion, proposal.TargetVersion)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID == "" {
		t.Fatal("missing operation ID")
	}
	version, err := DetectVersion(root)
	if err != nil || version != 5 {
		t.Fatalf("version = %d, %v", version, err)
	}

	// Verify skill.meta.yaml deleted and .meta/skill.yaml created
	if _, err := os.Stat(filepath.Join(skillDir, "skill.meta.yaml")); !os.IsNotExist(err) {
		t.Fatal("skill.meta.yaml was not deleted")
	}
	newMetaBytes, err := os.ReadFile(filepath.Join(skillDir, ".meta", "skill.yaml"))
	if err != nil {
		t.Fatalf("read .meta/skill.yaml: %v", err)
	}
	newMetaStr := string(newMetaBytes)
	if !strings.Contains(newMetaStr, "migrate-test") {
		t.Fatalf("expected aliases in routing, got: %s", newMetaStr)
	}
	if !strings.Contains(newMetaStr, "upstream") || !strings.Contains(newMetaStr, "https://github.com/example/repo") {
		t.Fatalf("expected upstream source, got: %s", newMetaStr)
	}

	// Idempotency: running migration again reports ErrAlreadyCurrent
	_, err = DefaultRegistry().Preview(root, CurrentVersion)
	if !errors.Is(err, ErrAlreadyCurrent) {
		t.Fatalf("expected ErrAlreadyCurrent, got %v", err)
	}
}

func TestMigrationV2ToV3WithExistingPhase0Meta(t *testing.T) {
	root := v2Workspace(t)

	skillDir := filepath.Join(root, "skills", "core", "test-audit")
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "# Test Audit\n\nAudit test body.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	oldMeta := `schema_version: 1
id: test-audit
name: Test Audit
status: active
routing:
  triggers: [audit]
  not_for: [none]
  min_scope: single_step
quality:
  reviewed: true
  content_reviewed_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
provenance:
  origin:
    kind: github
    repository: https://github.com/example/upstream-audit
    commit: 2222222222222222222222222222222222222222
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(oldMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	// Phase 0 already wrote .meta/skill.yaml with a learning source
	existingMeta := `schema_version: 1
id: test-audit
status: active
sources:
  - id: distill-lab-source
    roles: [learning]
    repository: https://github.com/example/learning-repo
    commit: 3333333333333333333333333333333333333333
`
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte(existingMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	proposal, err := DefaultRegistry().Preview(root, CurrentVersion)
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

	mergedBytes, err := os.ReadFile(filepath.Join(skillDir, ".meta", "skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	mergedStr := string(mergedBytes)
	// Must contain both learning and upstream sources
	if !strings.Contains(mergedStr, "distill-lab-source") || !strings.Contains(mergedStr, "learning") {
		t.Fatalf("expected learning source preserved, got: %s", mergedStr)
	}
	if !strings.Contains(mergedStr, "https://github.com/example/upstream-audit") || !strings.Contains(mergedStr, "upstream") {
		t.Fatalf("expected upstream source added, got: %s", mergedStr)
	}
	// Quality must be preserved
	if !strings.Contains(mergedStr, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef") {
		t.Fatalf("expected content_reviewed_digest preserved, got: %s", mergedStr)
	}
}

func TestMigrationRejectsNewerSchema(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DefaultRegistry().Preview(root, CurrentVersion); err == nil {
		t.Fatal("expected error previewing migration for newer schema 6")
	}
	plan, err := workspace.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	hasIncompatible := false
	for _, f := range plan.Findings {
		if f.ID == "canonical_schema_incompatible" {
			hasIncompatible = true
		}
	}
	if !hasIncompatible {
		t.Fatal("expected canonical_schema_incompatible finding for version 6")
	}
}
