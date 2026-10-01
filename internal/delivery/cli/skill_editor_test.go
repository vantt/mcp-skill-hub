package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillCreatePositionalIDAndConflict(t *testing.T) {
	root := initTestWorkspace(t)

	// 1. Conflicting positional ID and --id fails immediately
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "create", "pos-id", "--id", "flag-id", "--workspace", root}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 on conflicting IDs, got %d", code)
	}
	if !strings.Contains(stderr.String(), "conflicts with --id") {
		t.Errorf("expected conflict error, got: %s", stderr.String())
	}

	// 2. Consistent positional ID and --id works
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: matching-id\ndescription: Test matching.\n---\n\n# Match\n\nInstructions.\n"), 0o600)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"skill", "create", "matching-id", "--id", "matching-id",
		"--collection", "core", "--name", "Match", "--description", "Test matching.",
		"--trigger", "match it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--workspace", root, "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("matching IDs failed (exit %d): %s", code, stderr.String())
	}

	// 3. Positional ID alone works
	contentFile2 := filepath.Join(t.TempDir(), "content2.md")
	_ = os.WriteFile(contentFile2, []byte("---\nname: pos-only\ndescription: Test positional only.\n---\n\n# Pos Only\n\nInstructions.\n"), 0o600)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"skill", "create", "pos-only",
		"--collection", "core", "--name", "Pos Only", "--description", "Test positional only.",
		"--trigger", "pos it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile2, "--workspace", root, "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("positional-only ID failed (exit %d): %s", code, stderr.String())
	}
}

func TestSkillEditorRecoveryLifecycleBUG03AndBUG12(t *testing.T) {
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: edit-target\ndescription: Target for editor.\n---\n\n# Target\n\nOriginal instructions.\n"), 0o600)

	code, _, stderr := runCLI(t, "skill", "create", "edit-target",
		"--collection", "core", "--name", "Target", "--description", "Target for editor.",
		"--trigger", "target it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--workspace", root, "--yes")
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}

	// Prepare an editor script that modifies the file to valid content
	scriptDir := t.TempDir()
	goodScript := filepath.Join(scriptDir, "editor-good.sh")
	_ = os.WriteFile(goodScript, []byte("#!/bin/sh\ncat << 'EOF' > \"$1\"\n---\nname: edit-target\ndescription: Target for editor.\n---\n\n# Target\n\nUpdated valuable instructions via editor.\nEOF\n"), 0o755)

	// 1. Run skill edit --editor without --yes: preview test (BUG-12)
	t.Setenv("VISUAL", goodScript)
	var stdout bytes.Buffer
	var errBuf bytes.Buffer
	code = Run([]string{"skill", "edit", "edit-target", "--editor", "--workspace", root}, &stdout, &errBuf)
	if code != 0 {
		t.Fatalf("editor preview exit %d: %s", code, errBuf.String())
	}

	// Verify BUG-12: Diff must be visible, confirm command must be short form, and NO "or re-run with --yes" advice
	outStr := stdout.String()
	if !strings.Contains(outStr, "Updated valuable instructions via editor.") {
		t.Errorf("preview missing entrypoint diff: %s", outStr)
	}
	if !strings.Contains(outStr, "skillhub skill confirm") {
		t.Errorf("preview missing confirm command: %s", outStr)
	}
	if strings.Contains(outStr, "re-run with --yes") {
		t.Errorf("preview must not advise re-running with --yes (BUG-12): %s", outStr)
	}

	// Extract proposal ID from preview output
	var proposalID string
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- Proposal:") {
			proposalID = strings.TrimSpace(strings.TrimPrefix(line, "- Proposal:"))
			break
		}
	}
	if proposalID == "" {
		t.Fatalf("could not find proposal ID in preview: %s", outStr)
	}

	// 2. Test BUG-03: Preview failure preserves recovery artifact
	badScript := filepath.Join(scriptDir, "editor-bad.sh")
	_ = os.WriteFile(badScript, []byte("#!/bin/sh\ncat << 'EOF' > \"$1\"\n---\nname: mismatched-name\ndescription: Target for editor.\n---\n\n# Bad\n\nVALUABLE RECOVERY DATA.\nEOF\n"), 0o755)
	t.Setenv("VISUAL", badScript)

	stdout.Reset()
	errBuf.Reset()
	code = Run([]string{"skill", "edit", "edit-target", "--editor", "--workspace", root}, &stdout, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2 on bad frontmatter, got %d", code)
	}

	// Check that recovery artifact was saved and mentioned in stderr
	if !strings.Contains(errBuf.String(), "runtime/edits/REC-") {
		t.Errorf("stderr missing recovery artifact path: %s", errBuf.String())
	}

	// Verify the recovery file exists on disk with mode 0600
	editsDir := filepath.Join(root, "runtime", "edits")
	entries, err := os.ReadDir(editsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("recovery files not found in runtime/edits: %v", err)
	}
	foundValuable := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			data, _ := os.ReadFile(filepath.Join(editsDir, entry.Name()))
			if strings.Contains(string(data), "VALUABLE RECOVERY DATA") {
				foundValuable = true
				break
			}
		}
	}
	if !foundValuable {
		t.Error("valuable editor content was not retained in recovery storage (BUG-03)")
	}

	// 3. Confirming proposal applies edit and removes recovery artifact
	stdout.Reset()
	errBuf.Reset()
	code = Run([]string{"skill", "confirm", proposalID, "--workspace", root}, &stdout, &errBuf)
	if code != 0 {
		t.Fatalf("short confirm failed exit %d: %s", code, errBuf.String())
	}
	if !strings.Contains(stdout.String(), "saved") && !strings.Contains(stdout.String(), "updated") {
		t.Errorf("confirm output unexpected: %s", stdout.String())
	}

	// Verify recovery artifact associated with proposal was cleaned up
	leftover, _ := os.ReadDir(editsDir)
	for _, entry := range leftover {
		if strings.Contains(entry.Name(), proposalID) {
			t.Errorf("proposal recovery artifact was not deleted on confirm: %s", entry.Name())
		}
	}
}

func TestSkillMutationCommitHintUsesActualWorkspaceBUG14(t *testing.T) {
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: hint-skill\ndescription: Test commit hint.\n---\n\n# Hint\n\nReal procedural instructions to replace untouched scaffold.\n"), 0o600)

	code, stdout, stderr := runCLI(t, "skill", "create", "hint-skill",
		"--collection", "core", "--name", "Hint", "--description", "Test commit hint.",
		"--trigger", "hint it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--workspace", root, "--yes")
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}

	// Activate and check commit hint
	code, stdout, stderr = runCLI(t, "skill", "activate", "hint-skill", "--workspace", root, "--yes")
	if code != 0 {
		t.Fatalf("activate failed (exit %d): %s", code, stderr)
	}

	// BUG-14: Must NOT contain literal "<ws>"
	if strings.Contains(stdout, "<ws>") {
		t.Errorf("output still contains literal '<ws>' (BUG-14):\n%s", stdout)
	}
	expectedHint := "git -C " + root + " commit"
	if !strings.Contains(stdout, expectedHint) {
		t.Errorf("output missing expected commit hint %q:\n%s", expectedHint, stdout)
	}
}
