package migration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestMigrationV3ToV4_LeavesExistingDistillYAMLByteIdentical(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("3\n"), 0o644)

	skillDir := filepath.Join(root, "skills", "default", "test-audit")
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Audit\n\nAudit tests.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: test-audit\nstatus: active\nrouting:\n  triggers: [audit]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	testdataBytes, err := os.ReadFile(filepath.Join("testdata", "test-audit-distill.yaml"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	distillPath := filepath.Join(skillDir, ".meta", "distill.yaml")
	if err := os.WriteFile(distillPath, testdataBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := DefaultRegistry()
	proposal, err := reg.Preview(root, CurrentVersion)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	// Verify proposal has NO changes or diffs touching test-audit/.meta/distill.yaml
	distillRelPath := filepath.ToSlash(filepath.Join("skills", "default", "test-audit", ".meta", "distill.yaml"))
	for _, ch := range proposal.Mutation.WriteSet.Changes {
		if ch.Path == distillRelPath {
			t.Fatalf("expected no mutation change for existing distill.yaml, found %#v", ch)
		}
	}
	for _, d := range proposal.Changes {
		if d.Path == distillRelPath {
			t.Fatalf("expected no diff for existing distill.yaml, found %#v", d)
		}
	}

	_, err = mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}

	// Verify byte-identical content after migration
	afterBytes, err := os.ReadFile(distillPath)
	if err != nil {
		t.Fatalf("read after migration: %v", err)
	}
	if !bytes.Equal(testdataBytes, afterBytes) {
		t.Fatalf("distill.yaml was mutated! before len=%d, after len=%d", len(testdataBytes), len(afterBytes))
	}
}

func TestPrePhase3FixtureMigrationV3ToV4(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")

	// 1. Create a schema 3 workspace
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("3\n"), 0o644)

	// Create skill without distill.yaml
	skillDir := filepath.Join(root, "skills", "default", "sample-skill")
	_ = os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Sample Skill\n\nSample description.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: sample-skill\nstatus: active\nrouting:\n  triggers: [sample]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644)

	// 2. Add legacy distill/ and sources/ artifacts
	_ = os.MkdirAll(filepath.Join(root, "sources", "intake"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sources", "intake", "intake-1.yaml"), []byte("url: https://example.com/intake\n"), 0o644)

	_ = os.MkdirAll(filepath.Join(root, "sources", "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sources", "skills", "LINK-sample-skill--src-1.yaml"), []byte("skill_id: sample-skill\nsource_id: src-1\n"), 0o644)

	_ = os.MkdirAll(filepath.Join(root, "distill", "runs"), 0o755)
	runContent := `id: run-01
source_id: src-1
to_revision:
  kind: git-commit
  value: 1234567890123456789012345678901234567890
`
	_ = os.WriteFile(filepath.Join(root, "distill", "runs", "run-01.yaml"), []byte(runContent), 0o644)

	_ = os.MkdirAll(filepath.Join(root, "distill", "insights"), 0o755)
	insAccepted := `id: INS-sample-skill--accepted-item
skill_id: sample-skill
stable_key: accepted-item
status: accepted
recommendation: Adopt robust timeout pattern
rationale: Prevents hangs in distributed workers
`
	insRejected := `id: INS-sample-skill--rejected-item
skill_id: sample-skill
stable_key: rejected-item
status: rejected
recommendation: Naive sleep retry
rationale: Flaky pattern
decision_rationale: Rejected by curator because sleep creates flake
`
	_ = os.WriteFile(filepath.Join(root, "distill", "insights", "ins-01.yaml"), []byte(insAccepted), 0o644)
	_ = os.WriteFile(filepath.Join(root, "distill", "insights", "ins-02.yaml"), []byte(insRejected), 0o644)

	// 3. Run migration v3 to v4
	reg := DefaultRegistry()
	proposal, err := reg.Preview(root, CurrentVersion)
	if err != nil {
		t.Fatalf("registry preview failed: %v", err)
	}
	if proposal.SourceVersion != 3 || proposal.TargetVersion != 5 {
		t.Fatalf("unexpected versions: source=%d, target=%d", proposal.SourceVersion, proposal.TargetVersion)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("mutation confirm failed: %v", err)
	}
	if receipt.OperationID == "" {
		t.Fatal("expected non-empty operation ID")
	}

	// 4. Verify version marker on disk
	newVer, err := DetectVersion(root)
	if err != nil || newVer != 5 {
		t.Fatalf("expected detected version 5, got %d, %v", newVer, err)
	}

	// 5. Verify legacy directories deleted
	legacyPaths := []string{
		filepath.Join(root, "sources", "intake", "intake-1.yaml"),
		filepath.Join(root, "sources", "skills", "LINK-sample-skill--src-1.yaml"),
		filepath.Join(root, "distill", "runs", "run-01.yaml"),
		filepath.Join(root, "distill", "insights", "ins-01.yaml"),
		filepath.Join(root, "distill", "insights", "ins-02.yaml"),
	}
	for _, lp := range legacyPaths {
		if _, statErr := os.Stat(lp); !os.IsNotExist(statErr) {
			t.Errorf("legacy file was not deleted: %s", lp)
		}
	}

	// 6. Verify .meta/distill.yaml created for sample-skill in distill-lab format
	distillPath := filepath.Join(skillDir, ".meta", "distill.yaml")
	doc, err := distill.LoadDocument(distillPath)
	if err != nil {
		t.Fatalf("load migrated distill.yaml failed: %v", err)
	}

	// 7. Verify human decisions survive
	lessonMap := make(map[string]distill.Lesson)
	for _, l := range doc.Lessons {
		lessonMap[l.Key] = l
	}

	acc, ok := lessonMap["accepted-item"]
	if !ok || acc.Decision.State != "planned" {
		t.Fatalf("accepted insight did not survive as planned lesson: %#v", acc)
	}

	rej, ok := lessonMap["rejected-item"]
	if !ok || rej.Decision.State != "rejected" {
		t.Fatalf("rejected insight did not survive as rejected lesson: %#v", rej)
	}
	if rej.Decision.Reason != "Rejected by curator because sleep creates flake" {
		t.Fatalf("rejected insight lost decision rationale: %q", rej.Decision.Reason)
	}
}

func TestGarbledInsightFileFailsClosedV4Preview(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("3\n"), 0o644)

	skillDir := filepath.Join(root, "skills", "default", "sample-skill")
	_ = os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Sample Skill\n\nSample description.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 2\nid: sample-skill\nstatus: active\n"), 0o644)

	// Create garbled insight file
	insightsDir := filepath.Join(root, "distill", "insights")
	_ = os.MkdirAll(insightsDir, 0o755)
	badFile := filepath.Join(insightsDir, "garbled-insight.yaml")
	_ = os.WriteFile(badFile, []byte("[unterminated yaml: {oops\n"), 0o644)

	reg := DefaultRegistry()
	_, err := reg.Preview(root, CurrentVersion)
	if err == nil {
		t.Fatal("expected error previewing v4 migration with garbled insight file, got nil")
	}
	if !strings.Contains(err.Error(), "garbled-insight.yaml") {
		t.Fatalf("expected error to name garbled-insight.yaml, got %v", err)
	}

	// Verify workspace is unchanged
	ver, err := DetectVersion(root)
	if err != nil || ver != 3 {
		t.Fatalf("workspace version changed: ver=%d, err=%v", ver, err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, ".meta", "distill.yaml")); !os.IsNotExist(err) {
		t.Fatal("workspace was modified: distill.yaml was created despite preview failure")
	}
}
