package migration

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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

	// Generate a synthetic large legacy receipt (>100 KB) in the test
	var legacyReceipt strings.Builder
	legacyReceipt.WriteString(`schema_version: 1
id: OP-large-legacy-test
kind: skill_active
occurred_at: "2026-10-08T12:00:00Z"
idempotency_key: test-idemp
request_digest: sha256:1111111111111111111111111111111111111111111111111111111111111111
proposal:
    id: PROP-test
    digest: sha256:2222222222222222222222222222222222222222222222222222222222222222
catalog_snapshot: sha256:3333333333333333333333333333333333333333333333333333333333333333
changes:
    - path: skills/default/sample/SKILL.md
      before: sha256:4444444444444444444444444444444444444444444444444444444444444444
      after: sha256:5555555555555555555555555555555555555555555555555555555555555555
      content_available: true
      before_content: |
`)
	for range 1500 {
		legacyReceipt.WriteString("        # Large before content line padding for test fixture over 100 KB\n")
	}
	legacyReceipt.WriteString("      after_content: |\n")
	for range 1500 {
		legacyReceipt.WriteString("        # Large after content line padding for test fixture over 100 KB\n")
	}

	receiptData := []byte(legacyReceipt.String())
	beforeSize := len(receiptData)
	if beforeSize < 100<<10 {
		t.Fatalf("expected large receipt before migration (>100 KB), got %d bytes", beforeSize)
	}

	targetDir := filepath.Join(root, "history", "operations", "2026", "10")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(targetDir, "OP-large-legacy-test.yaml")
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
	if doc.ID != "OP-large-legacy-test" {
		t.Fatalf("expected id OP-large-legacy-test, got %q", doc.ID)
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

func TestMigrationV4ToV5_PreservesReceiptKeyOrderAndIndentation(t *testing.T) {
	t.Parallel()

	inputYAML := `schema_version: 1
id: OP-02e11071f24379a54a8a1c4b15a08066
kind: skill_active
occurred_at: "2026-10-02T07:19:21.834866866Z"
idempotency_key: skill_active:test-audit:055d5d484329c75b79380f39e25c2ac21d9c2038681d49cd833ecd603029b590
request_digest: sha256:055d5d484329c75b79380f39e25c2ac21d9c2038681d49cd833ecd603029b590
proposal:
    id: PROP-8ab833ebd498e44c332e
    digest: sha256:8ab833ebd498e44c332ecd493d787e77969c27aecd20d8d403657cbc663bd91d
base_catalog_snapshot: sha256:2c390be63f37c2a74935aa0a4dfd05545f4ea2099fc5fae00c08dd0ba28babb4
result_catalog_snapshot: sha256:0e737c998a0c0e506b7b501516bf69253b2fe471bfa2d1219af46d5a25e697e1
changes:
    - path: skills/default/test-audit/skill.meta.yaml
      before: sha256:17cf1d2e33a96fd293416469728af8a2efd1bca17ca596f9c5ebdb1de82abd9f
      after: sha256:1fa500cd35c572f1c532b4ec366c2ce58f1242163fca53e5dc84a04e1e12f187
      content_available: true
      before_content: |
        line 1
      after_content: |
        line 2
`
	outputYAML, stripped, err := stripReceiptBodiesNode([]byte(inputYAML))
	if err != nil {
		t.Fatalf("stripReceiptBodiesNode: %v", err)
	}
	if !stripped {
		t.Fatal("expected stripped == true")
	}

	// 1. Verify key order via yaml.Node
	var doc yaml.Node
	if err := yaml.Unmarshal(outputYAML, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	mapping := doc.Content[0]
	var topKeys []string
	for i := 0; i < len(mapping.Content); i += 2 {
		topKeys = append(topKeys, mapping.Content[i].Value)
	}
	expectedTopKeys := []string{
		"schema_version",
		"id",
		"kind",
		"occurred_at",
		"idempotency_key",
		"request_digest",
		"proposal",
		"base_catalog_snapshot",
		"result_catalog_snapshot",
		"changes",
	}
	if !reflect.DeepEqual(topKeys, expectedTopKeys) {
		t.Fatalf("top-level key order changed:\ngot:  %v\nwant: %v", topKeys, expectedTopKeys)
	}

	// Verify proposal keys: id must precede digest
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "proposal" {
			propMap := mapping.Content[i+1]
			var propKeys []string
			for j := 0; j < len(propMap.Content); j += 2 {
				propKeys = append(propKeys, propMap.Content[j].Value)
			}
			if len(propKeys) >= 2 && (propKeys[0] != "id" || propKeys[1] != "digest") {
				t.Fatalf("proposal key order changed: got %v, want [id digest]", propKeys)
			}
		}
		if mapping.Content[i].Value == "changes" {
			chSeq := mapping.Content[i+1]
			chMap := chSeq.Content[0]
			var chKeys []string
			for j := 0; j < len(chMap.Content); j += 2 {
				chKeys = append(chKeys, chMap.Content[j].Value)
			}
			expectedChKeys := []string{"path", "before", "after"}
			if !reflect.DeepEqual(chKeys, expectedChKeys) {
				t.Fatalf("change item key order changed:\ngot:  %v\nwant: %v", chKeys, expectedChKeys)
			}
		}
	}

	// 2. Verify 4-space indentation for proposal and changes
	outStr := string(outputYAML)
	if !strings.Contains(outStr, "proposal:\n    id:") {
		t.Fatalf("expected 4-space indentation for proposal, got:\n%s", outStr)
	}
	if strings.Contains(outStr, "before_content") || strings.Contains(outStr, "after_content") || strings.Contains(outStr, "content_available") {
		t.Fatalf("stripped receipt still contains body fields:\n%s", outStr)
	}
}
