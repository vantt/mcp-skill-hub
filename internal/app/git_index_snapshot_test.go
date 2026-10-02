package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "initial commit")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, out)
	}
	return string(out)
}

func writeSkill(t *testing.T, root, id, fmName, body string) {
	t.Helper()
	dir := filepath.Join(root, "skills", "software", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := "schema_version: 1\nid: " + id + "\nname: " + strings.ToUpper(id) + "\nstatus: draft\ndescription: A test skill.\nrouting: {}\n"
	if err := os.WriteFile(filepath.Join(dir, "skill.meta.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + fmName + "\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStagedValidationOppositeResults(t *testing.T) {
	t.Parallel()
	root := setupTestGitRepo(t)
	svc := WorkspaceService{}
	ctx := context.Background()

	// Case 1: Staged is valid, worktree is invalid
	// Write valid skill and stage it
	writeSkill(t, root, "rr", "rr", "# Valid content")
	runGit(t, root, "add", "skills/software/rr")

	// Now modify worktree to be invalid (mismatched frontmatter name)
	worktreePath := filepath.Join(root, "skills", "software", "rr", "SKILL.md")
	if err := os.WriteFile(worktreePath, []byte("---\nname: INVALID_NAME\n---\n# Invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Staged validation should PASS
	stagedResult, err := svc.ValidateWorkspaceStaged(ctx, root)
	if err != nil {
		t.Fatalf("staged validation error: %v", err)
	}
	if stagedResult.Status != StatusOK {
		t.Fatalf("expected staged validation StatusOK, got %s: %#v", stagedResult.Status, stagedResult)
	}

	// Worktree validation should FAIL
	worktreeResult, err := svc.ValidateWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("worktree validation error: %v", err)
	}
	if worktreeResult.Status != StatusError {
		t.Fatalf("expected worktree validation StatusError, got %s: %#v", worktreeResult.Status, worktreeResult)
	}

	// Case 2: Staged is invalid, worktree is valid
	// Stage the invalid worktree
	runGit(t, root, "add", "skills/software/rr/SKILL.md")

	// Now fix the worktree to be valid
	if err := os.WriteFile(worktreePath, []byte("---\nname: rr\n---\n# Valid again\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Staged validation should now FAIL
	stagedResult2, err := svc.ValidateWorkspaceStaged(ctx, root)
	if err != nil {
		t.Fatalf("staged validation error: %v", err)
	}
	if stagedResult2.Status != StatusError {
		t.Fatalf("expected staged validation StatusError, got %s: %#v", stagedResult2.Status, stagedResult2)
	}

	// Worktree validation should now PASS
	worktreeResult2, err := svc.ValidateWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("worktree validation error: %v", err)
	}
	if worktreeResult2.Status != StatusOK {
		t.Fatalf("expected worktree validation StatusOK, got %s: %#v", worktreeResult2.Status, worktreeResult2)
	}
}

func TestStagedValidationNeverRunsGitFilters(t *testing.T) {
	t.Parallel()
	root := setupTestGitRepo(t)
	svc := WorkspaceService{}
	ctx := context.Background()

	// Sentinel file that the filter script will create if executed
	sentinelPath := filepath.Join(t.TempDir(), "filter_executed.sentinel")
	scriptPath := filepath.Join(t.TempDir(), "smudge.sh")
	scriptContent := "#!/bin/sh\ntouch \"" + sentinelPath + "\"\ncat\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	// Configure git filter in repo
	// Configure git filter in repo via .git/config and .git/info/attributes
	runGit(t, root, "config", "filter.sentinel.smudge", scriptPath)
	runGit(t, root, "config", "filter.sentinel.clean", scriptPath)
	infoDir := filepath.Join(root, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(infoDir, "attributes"), []byte("* filter=sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Remove sentinel if created during git add
	_ = os.Remove(sentinelPath)

	// Add valid skill and stage it
	writeSkill(t, root, "filt", "filt", "# Filter test")
	runGit(t, root, "add", "skills/software/filt")
	_ = os.Remove(sentinelPath)

	// Run staged validation
	result, err := svc.ValidateWorkspaceStaged(ctx, root)
	if err != nil {
		t.Fatalf("staged validation error: %v", err)
	}
	if result.Status != StatusOK {
		t.Fatalf("staged validation failed: %#v", result)
	}

	// Prove sentinel was NEVER created
	if _, err := os.Stat(sentinelPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("smudge filter was executed during staged validation! Sentinel exists: %s", sentinelPath)
	}
}

func TestStagedValidationPreservesIndexBytesAndGitStatus(t *testing.T) {
	t.Parallel()
	root := setupTestGitRepo(t)
	svc := WorkspaceService{}
	ctx := context.Background()

	writeSkill(t, root, "audit", "audit", "# Audit content")
	runGit(t, root, "add", "skills/software/audit")

	indexPath := filepath.Join(root, ".git", "index")
	initialBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	initialStatus := runGit(t, root, "status", "--porcelain=v2", "-z")

	// Run staged validation multiple times
	for i := range 3 {
		result, err := svc.ValidateWorkspaceStaged(ctx, root)
		if err != nil {
			t.Fatalf("pass %d failed: %v", i, err)
		}
		if result.Status != StatusOK {
			t.Fatalf("pass %d unexpected status: %s", i, result.Status)
		}

		afterBytes, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(initialBytes, afterBytes) {
			t.Fatalf("pass %d modified .git/index bytes!", i)
		}

		afterStatus := runGit(t, root, "status", "--porcelain=v2", "-z")
		if initialStatus != afterStatus {
			t.Fatalf("pass %d changed git status! Before:\n%q\nAfter:\n%q", i, initialStatus, afterStatus)
		}
	}
}

func TestGetGitPathSummary(t *testing.T) {
	t.Parallel()
	root := setupTestGitRepo(t)
	svc := WorkspaceService{}
	ctx := context.Background()
	// 1. Commit sk-b first so we can modify it unstaged later
	writeSkill(t, root, "sk-b", "sk-b", "# Skill B")
	runGit(t, root, "add", "skills/software/sk-b")
	runGit(t, root, "commit", "-m", "add sk-b")

	// 2. Unstaged modification to sk-b
	_ = os.WriteFile(filepath.Join(root, "skills", "software", "sk-b", "SKILL.md"), []byte("# Skill B edited\n"), 0o644)

	// 3. Staged file: sk-a
	writeSkill(t, root, "sk-a", "sk-a", "# Skill A")
	runGit(t, root, "add", "skills/software/sk-a")

	// 4. Untracked file: sk-c
	writeSkill(t, root, "sk-c", "sk-c", "# Skill C")

	summary, err := svc.GetGitPathSummary(ctx, root)
	if err != nil {
		t.Fatalf("GetGitPathSummary failed: %v", err)
	}

	if !summary.Configured {
		t.Fatal("expected summary.Configured to be true")
	}
	if !summary.Dirty {
		t.Fatal("expected summary.Dirty to be true")
	}

	hasStaged := false
	for _, f := range summary.Staged {
		if strings.Contains(f.Path, "skills/software/sk-a") {
			hasStaged = true
		}
	}
	if !hasStaged {
		t.Fatalf("expected skills/software/sk-a in staged, got: %#v", summary.Staged)
	}

	hasUnstaged := false
	for _, f := range summary.Unstaged {
		if f.Path == "skills/software/sk-b/SKILL.md" {
			hasUnstaged = true
		}
	}
	if !hasUnstaged {
		t.Fatalf("expected skills/software/sk-b/SKILL.md in unstaged, got: %#v", summary.Unstaged)
	}

	hasUntracked := false
	for _, f := range summary.Untracked {
		if strings.Contains(f.Path, "skills/software/sk-c") {
			hasUntracked = true
		}
	}
	if !hasUntracked {
		t.Fatalf("expected skills/software/sk-c in untracked, got: %#v", summary.Untracked)
	}
}

func TestReadGitIndexIdentity(t *testing.T) {
	t.Parallel()
	root := setupTestGitRepo(t)
	ctx := context.Background()

	id1, err := ReadGitIndexIdentity(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(id1) != 64 {
		t.Fatalf("expected 64-char SHA256, got: %s", id1)
	}

	// Change index
	writeSkill(t, root, "new", "new", "# New")
	runGit(t, root, "add", "skills/software/new")

	id2, err := ReadGitIndexIdentity(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Fatal("expected index identity to change after staging new file")
	}

	// Verify exact SHA256 of .git/index file
	raw, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(raw)
	if id2 != hex.EncodeToString(expected[:]) {
		t.Fatalf("identity mismatch: got %s, want %s", id2, hex.EncodeToString(expected[:]))
	}
}
