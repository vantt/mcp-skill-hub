package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

func TestValidateWorkspaceApplicationContract(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	result, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil || result.Status != StatusOK || result.Error != nil {
		t.Fatalf("validate = %#v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil || result.Status != StatusError || result.Error == nil || result.Error.Code != ErrorWorkspaceInvalid {
		t.Fatalf("invalid validate = %#v, %v", result, err)
	}
}

func TestGetCurationDiffGroupsRelativeCanonicalPaths(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)
	changes := map[string]string{
		"skills/testing/example/SKILL.md": "# Example\n",
		"distill/comparisons/CMP-1.yaml":  "id: CMP-1\n",
		"config/local.yaml":               "enabled: true\n",
	}
	for relative, contents := range changes {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (WorkspaceService{}).GetCurationDiff(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Dirty || len(result.Groups) != 3 || len(result.SuggestedActions) != 1 {
		t.Fatalf("diff = %#v", result)
	}
	for _, group := range result.Groups {
		for _, file := range group.Files {
			if filepath.IsAbs(file.Path) || strings.Contains(file.Path, root) || strings.Contains(file.Path, "runtime/") {
				t.Fatalf("diff leaked local path: %#v", file)
			}
		}
	}
}

func TestParsePorcelainV2RenameClassifiesSourceAndDestination(t *testing.T) {
	t.Parallel()
	output := []byte("2 R. N... 100644 100644 100644 aaaaaaa bbbbbbb R100 sources/catalog/new.yaml\x00skills/core/old/SKILL.md\x00")
	files, err := parsePorcelain(output)
	if err != nil {
		t.Fatal(err)
	}
	want := []DiffFile{{Path: "skills/core/old/SKILL.md", Status: "D"}, {Path: "sources/catalog/new.yaml", Status: "A"}}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("rename effects = %#v, want %#v", files, want)
	}
	groups := groupDiffFiles(files)
	if len(groups) != 2 || groups[0].Kind != "active_skills" || groups[1].Kind != "source_learning" {
		t.Fatalf("rename groups = %#v", groups)
	}
}

func TestParsePorcelainRejectsAbsolutePath(t *testing.T) {
	t.Parallel()
	if _, err := parsePorcelain([]byte("? /tmp/secret\x00")); err == nil {
		t.Fatal("absolute Git path was accepted")
	}
}

func TestInspectGitDisablesConfiguredFSMonitor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)
	marker := filepath.Join(t.TempDir(), "fsmonitor-called")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf called > \"$SKILLHUB_FSMONITOR_MARKER\"\nprintf '\\0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", root, "config", "core.fsmonitor", hook)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("configure fsmonitor: %v: %s", err, output)
	}
	t.Setenv("SKILLHUB_FSMONITOR_MARKER", marker)
	if _, err := inspectGit(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("offline Git status invoked fsmonitor: %v", err)
	}
}
func TestValidateWorkspaceAddsNoRuleBeyondCanonicalValidate(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	// 1. Workspace is valid: both return 0 issues
	issues, err := canonical.Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 || res.Status != StatusOK {
		t.Fatalf("clean workspace mismatch: canonical=%d, app=%s", len(issues), res.Status)
	}

	// 2. Add an invalid skill with frontmatter mismatch
	skillDir := filepath.Join(root, "skills", "software", "test-skill")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte("schema_version: 1\nid: test-skill\nname: Test\nstatus: draft\ndescription: Test\nrouting: {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: WRONG\n---\n# Test\n"), 0o644)

	issues, err = canonical.Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err = (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != len(res.Items) {
		t.Fatalf("issue count mismatch: canonical=%d, app=%d (ValidateWorkspace added or dropped rules)", len(issues), len(res.Items))
	}
	if res.Status != StatusError {
		t.Fatalf("expected StatusError, got %s", res.Status)
	}
}

func TestWorkspaceServiceGetOperationDiff(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	opDir := filepath.Join(root, "history", "operations", "2026", "10")
	if err := os.MkdirAll(opDir, 0o755); err != nil {
		t.Fatal(err)
	}
	opReceipt := `id: op_test_123
changes:
  - path: skills/default/sample/SKILL.md
    before: ""
    after: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
    before_content: ""
    after_content: "# Sample\n"
    content_available: true
`
	if err := os.WriteFile(filepath.Join(opDir, "op_test_123.yaml"), []byte(opReceipt), 0o644); err != nil {
		t.Fatal(err)
	}
	diffRes, err := (WorkspaceService{}).GetOperationDiff(context.Background(), root, "op_test_123")
	if err != nil {
		t.Fatalf("GetOperationDiff failed: %v", err)
	}
	if diffRes.OperationID != "op_test_123" || len(diffRes.Changes) != 1 {
		t.Fatalf("unexpected diff result: %#v", diffRes)
	}
	if !diffRes.Changes[0].DiffAvailable {
		t.Fatalf("expected DiffAvailable true")
	}
}

func TestGetOperationDiff_CommittedMatchesGitDiff(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "default", "demo")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: demo\nstatus: active\nrouting:\n  triggers: [demo]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create and confirm a mutation
	filePath := "skills/default/demo/references/doc.txt"
	set := mutation.WriteSet{
		Command: "test_create",
		Changes: []mutation.Change{
			{Path: filePath, Contents: []byte("Line 1: initial content\nLine 2: more content\n")},
		},
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := mutation.ConfirmMutation(root, planned, mutation.Confirmation{
		ProposalID:          planned.ID,
		ProposalDigest:      planned.Digest,
		BaseCatalogSnapshot: planned.BaseCatalogSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Commit the mutation and receipt to Git
	cmdAdd := exec.Command("git", "-C", root, "add", "-A")
	if out, err := cmdAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (%s)", err, string(out))
	}
	cmdCommit := exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "apply test_create")
	if out, err := cmdCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v (%s)", err, string(out))
	}

	// Call GetOperationDiff
	wsSvc := WorkspaceService{}
	diffRes, err := wsSvc.GetOperationDiff(context.Background(), root, receipt.OperationID)
	if err != nil {
		t.Fatalf("GetOperationDiff: %v", err)
	}

	if diffRes.OperationID != receipt.OperationID || len(diffRes.Changes) != 1 {
		t.Fatalf("unexpected diffRes: %#v", diffRes)
	}
	ch := diffRes.Changes[0]
	if !ch.DiffAvailable {
		t.Fatal("expected DiffAvailable == true for committed operation")
	}
	if ch.After != "Line 1: initial content\nLine 2: more content\n" {
		t.Fatalf("unexpected after content: %q", ch.After)
	}

	// Verify that diff contains the added lines
	if !strings.Contains(ch.Diff, "+Line 1: initial content") {
		t.Fatalf("diff missing added line: %s", ch.Diff)
	}
}

func TestGetOperationDiff_PreservedAfterV5MigrationCommit(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "default", "demo")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: demo\nstatus: active\nrouting:\n  triggers: [demo]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Perform an initial mutation and commit it
	filePath := "skills/default/demo/references/doc.txt"
	set := mutation.WriteSet{
		Command: "create_doc",
		Changes: []mutation.Change{
			{Path: filePath, Contents: []byte("Line 1: initial\n")},
		},
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := mutation.ConfirmMutation(root, planned, mutation.Confirmation{
		ProposalID:          planned.ID,
		ProposalDigest:      planned.Digest,
		BaseCatalogSnapshot: planned.BaseCatalogSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmdAdd := exec.Command("git", "-C", root, "add", "-A")
	if out, err := cmdAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (%s)", err, string(out))
	}
	cmdCommit := exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "commit op 1")
	if out, err := cmdCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v (%s)", err, string(out))
	}

	// 2. Set schema-version to 4 and run v5 migration to rewrite receipts, then commit the migration
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	migService := MigrationService{}
	applied, err := migService.Migrate(context.Background(), root, 5, true)
	if err != nil || applied.Status != StatusApplied {
		t.Fatalf("v5 migration failed: %#v, %v", applied, err)
	}

	cmdAdd2 := exec.Command("git", "-C", root, "add", "-A")
	if out, err := cmdAdd2.CombinedOutput(); err != nil {
		t.Fatalf("git add after migration: %v (%s)", err, string(out))
	}
	cmdCommit2 := exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "commit v5 migration")
	if out, err := cmdCommit2.CombinedOutput(); err != nil {
		t.Fatalf("git commit after migration: %v (%s)", err, string(out))
	}

	// 3. Diff for original operation must still be found and accurate!
	wsSvc := WorkspaceService{}
	diffRes, err := wsSvc.GetOperationDiff(context.Background(), root, receipt.OperationID)
	if err != nil {
		t.Fatalf("GetOperationDiff: %v", err)
	}
	if len(diffRes.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(diffRes.Changes))
	}
	ch := diffRes.Changes[0]
	if !ch.DiffAvailable {
		t.Fatalf("expected DiffAvailable == true after v5 migration commit, got false (digest_only=%v)", ch.DigestOnlyMetadata)
	}
	if ch.After != "Line 1: initial\n" {
		t.Fatalf("unexpected After: %q", ch.After)
	}
	if !strings.Contains(ch.Diff, "+Line 1: initial") {
		t.Fatalf("unexpected diff: %s", ch.Diff)
	}
}

func TestGetOperationDiff_IndependentOfLaterEdits(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "default", "demo")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: demo\nstatus: active\nrouting:\n  triggers: [demo]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Perform OP-1 and commit
	filePath := "skills/default/demo/references/doc.txt"
	set1 := mutation.WriteSet{
		Command: "op1",
		Changes: []mutation.Change{
			{Path: filePath, Contents: []byte("Line 1: initial\n")},
		},
	}
	planned1, err := mutation.PlanMutation(root, set1)
	if err != nil {
		t.Fatal(err)
	}
	receipt1, err := mutation.ConfirmMutation(root, planned1, mutation.Confirmation{
		ProposalID:          planned1.ID,
		ProposalDigest:      planned1.Digest,
		BaseCatalogSnapshot: planned1.BaseCatalogSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	_ = exec.Command("git", "-C", root, "add", "-A").Run()
	_ = exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "commit op 1").Run()

	// 2. Perform OP-2 on the same file and commit
	set2 := mutation.WriteSet{
		Command: "op2",
		Changes: []mutation.Change{
			{Path: filePath, Contents: []byte("Line 1: initial\nLine 2: later edit\n")},
		},
	}
	planned2, err := mutation.PlanMutation(root, set2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mutation.ConfirmMutation(root, planned2, mutation.Confirmation{
		ProposalID:          planned2.ID,
		ProposalDigest:      planned2.Digest,
		BaseCatalogSnapshot: planned2.BaseCatalogSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	_ = exec.Command("git", "-C", root, "add", "-A").Run()
	_ = exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "commit op 2").Run()

	// 3. Diff for OP-1 must still reflect OP-1's content, NOT OP-2's content!
	wsSvc := WorkspaceService{}
	diffRes1, err := wsSvc.GetOperationDiff(context.Background(), root, receipt1.OperationID)
	if err != nil {
		t.Fatalf("GetOperationDiff OP-1: %v", err)
	}
	ch1 := diffRes1.Changes[0]
	if !ch1.DiffAvailable {
		t.Fatal("expected DiffAvailable == true for OP-1")
	}
	if ch1.After != "Line 1: initial\n" {
		t.Fatalf("OP-1 diff returned later commit's after content: %q", ch1.After)
	}
	if strings.Contains(ch1.Diff, "later edit") {
		t.Fatalf("OP-1 diff contains later edit text: %s", ch1.Diff)
	}
}

func TestGetOperationDiff_DigestMismatchYieldsDigestOnly(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "default", "demo")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: demo\nstatus: active\nrouting:\n  triggers: [demo]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Perform mutation recording after-digest for "Line 1: planned\n"
	filePath := "skills/default/demo/references/doc.txt"
	set := mutation.WriteSet{
		Command: "op_mismatch",
		Changes: []mutation.Change{
			{Path: filePath, Contents: []byte("Line 1: planned\n")},
		},
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := mutation.ConfirmMutation(root, planned, mutation.Confirmation{
		ProposalID:          planned.ID,
		ProposalDigest:      planned.Digest,
		BaseCatalogSnapshot: planned.BaseCatalogSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now modify the file to something unrelated before committing
	if err := os.WriteFile(filepath.Join(root, filePath), []byte("Line 1: completely different edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = exec.Command("git", "-C", root, "add", "-A").Run()
	_ = exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "commit with unrelated edit").Run()

	// GetOperationDiff must report digest_only=true because committed digest does not match receipt
	wsSvc := WorkspaceService{}
	diffRes, err := wsSvc.GetOperationDiff(context.Background(), root, receipt.OperationID)
	if err != nil {
		t.Fatalf("GetOperationDiff: %v", err)
	}
	ch := diffRes.Changes[0]
	if ch.DiffAvailable {
		t.Fatal("expected DiffAvailable == false when committed digest does not match receipt")
	}
	if !ch.DigestOnlyMetadata {
		t.Fatal("expected DigestOnlyMetadata == true when digests mismatch")
	}
}

func TestGetOperationDiff_UncommittedWorkingFileMismatchYieldsDigestOnly(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "default", "demo")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: demo\nstatus: active\nrouting:\n  triggers: [demo]\n  operations: [review]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Initial git commit of workspace baseline
	_ = exec.Command("git", "-C", root, "add", "-A").Run()
	_ = exec.Command("git", "-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "baseline").Run()

	// 2. Perform an uncommitted mutation
	filePath := "skills/default/demo/references/doc.txt"
	set := mutation.WriteSet{
		Command: "op_uncommitted",
		Changes: []mutation.Change{
			{Path: filePath, Contents: []byte("Line 1: operation content\n")},
		},
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := mutation.ConfirmMutation(root, planned, mutation.Confirmation{
		ProposalID:          planned.ID,
		ProposalDigest:      planned.Digest,
		BaseCatalogSnapshot: planned.BaseCatalogSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Edit the working file again before any commit
	if err := os.WriteFile(filepath.Join(root, filePath), []byte("Line 1: operation content\nExtra uncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. GetOperationDiff must report digest_only=true because working file != operation after-digest
	wsSvc := WorkspaceService{}
	diffRes, err := wsSvc.GetOperationDiff(context.Background(), root, receipt.OperationID)
	if err != nil {
		t.Fatalf("GetOperationDiff: %v", err)
	}
	ch := diffRes.Changes[0]
	if ch.DiffAvailable {
		t.Fatal("expected DiffAvailable == false when working file was edited after operation")
	}
	if !ch.DigestOnlyMetadata {
		t.Fatal("expected DigestOnlyMetadata == true when working file does not match after-digest")
	}
}
