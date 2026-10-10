package migration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestMigrationV4ToV5_StripsBodiesFromLegacyReceipt(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("4\n"), 0o644)

	// Copy the live hub's 208 KB receipt fixture
	liveReceiptPath := "/home/vantt/skill-hub/history/operations/2026/10/OP-c011d730fa8122171101881543a63bc8.yaml"
	receiptData, err := os.ReadFile(liveReceiptPath)
	if err != nil {
		t.Skipf("live hub 208 KB receipt fixture not found at %s: %v", liveReceiptPath, err)
	}

	beforeSize := len(receiptData)
	if beforeSize < 100<<10 {
		t.Fatalf("expected large receipt before migration (>100 KB), got %d bytes", beforeSize)
	}

	targetDir := filepath.Join(root, "history", "operations", "2026", "10")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(targetDir, "OP-c011d730fa8122171101881543a63bc8.yaml")
	if err := os.WriteFile(targetFile, receiptData, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := DefaultRegistry()
	proposal, err := reg.Preview(root, CurrentVersion)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if proposal.SourceVersion != 4 || proposal.TargetVersion != 5 {
		t.Fatalf("unexpected versions: source=%d, target=%d", proposal.SourceVersion, proposal.TargetVersion)
	}

	_, err = mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}

	// Verify new schema version is 5
	newVer, err := DetectVersion(root)
	if err != nil || newVer != 5 {
		t.Fatalf("expected schema version 5, got %d, %v", newVer, err)
	}

	// Read migrated receipt
	afterData, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read migrated receipt: %v", err)
	}
	afterSize := len(afterData)

	// Verify dramatic shrinkage (>90% reduction, under 10 KB)
	if afterSize > 10<<10 {
		t.Fatalf("expected stripped receipt < 10 KB, got %d bytes (was %d bytes)", afterSize, beforeSize)
	}

	// Unmarshal and assert body fields are completely absent
	var doc struct {
		ID      string           `yaml:"id"`
		Changes []map[string]any `yaml:"changes"`
	}
	if err := yaml.Unmarshal(afterData, &doc); err != nil {
		t.Fatalf("unmarshal migrated receipt: %v", err)
	}
	if doc.ID != "OP-c011d730fa8122171101881543a63bc8" {
		t.Fatalf("expected id OP-c011d730fa8122171101881543a63bc8, got %q", doc.ID)
	}
	if len(doc.Changes) == 0 {
		t.Fatal("expected non-empty changes")
	}
	for i, ch := range doc.Changes {
		if _, ok := ch["before_content"]; ok {
			t.Fatalf("change[%d] still has before_content", i)
		}
		if _, ok := ch["after_content"]; ok {
			t.Fatalf("change[%d] still has after_content", i)
		}
		if _, ok := ch["content_available"]; ok {
			t.Fatalf("change[%d] still has content_available", i)
		}
		if _, ok := ch["path"]; !ok {
			t.Fatalf("change[%d] missing path", i)
		}
	}
}

func TestMigrationV4ToV5_IdempotentWhenAlreadyStripped(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("4\n"), 0o644)

	// A receipt with no body fields
	opDir := filepath.Join(root, "history", "operations", "2026", "10")
	if err := os.MkdirAll(opDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cleanReceipt := `schema_version: 1
id: OP-clean
status: applied
changes:
  - path: a.txt
    before: sha256:1111111111111111111111111111111111111111111111111111111111111111
    after: sha256:2222222222222222222222222222222222222222222222222222222222222222
`
	cleanFile := filepath.Join(opDir, "OP-clean.yaml")
	if err := os.WriteFile(cleanFile, []byte(cleanReceipt), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := DefaultRegistry()
	proposal, err := reg.Preview(root, CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Should only touch .skillhub/schema-version, NOT OP-clean.yaml
	for _, ch := range proposal.Mutation.WriteSet.Changes {
		if strings.Contains(ch.Path, "OP-clean") {
			t.Fatalf("already clean receipt should not be mutated: %#v", ch)
		}
	}

	_, err = mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(cleanFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(cleanReceipt), after) {
		t.Fatal("clean receipt was modified")
	}
}
