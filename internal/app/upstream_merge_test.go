package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
)

func TestUpstreamGitMerge(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	// 1. Clean non-overlapping edits (conflicts 0, output equals expected bytes)
	t.Run("clean non-overlapping edits", func(t *testing.T) {
		base := []byte("line1\nline2\nline3\nline4\nline5\n")
		local := []byte("line1\nline2-local\nline3\nline4\nline5\n")
		upstream := []byte("line1\nline2\nline3\nline4-upstream\nline5\n")
		expected := []byte("line1\nline2-local\nline3\nline4-upstream\nline5\n")

		merged, conflicts, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if conflicts != 0 {
			t.Fatalf("expected 0 conflicts, got %d", conflicts)
		}
		if !bytes.Equal(merged, expected) {
			t.Fatalf("expected %q, got %q", expected, merged)
		}
	})

	// 2. Same-line conflict (conflicts 1; output contains <<<<<<< local, ||||||| base, =======, >>>>>>> upstream)
	t.Run("same-line conflict", func(t *testing.T) {
		base := []byte("line1\nline2-base\nline3\n")
		local := []byte("line1\nline2-local\nline3\n")
		upstream := []byte("line1\nline2-upstream\nline3\n")

		merged, conflicts, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if conflicts != 1 {
			t.Fatalf("expected 1 conflict, got %d", conflicts)
		}
		mStr := string(merged)
		for _, marker := range []string{"<<<<<<< local", "||||||| base", "=======", ">>>>>>> upstream"} {
			if !strings.Contains(mStr, marker) {
				t.Fatalf("expected output to contain %q, got:\n%s", marker, mStr)
			}
		}
	})

	// 3. Two separate conflicts (conflicts 2)
	t.Run("two separate conflicts", func(t *testing.T) {
		base := []byte("line1-base\nsep\nline3-base\n")
		local := []byte("line1-local\nsep\nline3-local\n")
		upstream := []byte("line1-upstream\nsep\nline3-upstream\n")

		_, conflicts, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if conflicts != 2 {
			t.Fatalf("expected 2 conflicts, got %d", conflicts)
		}
	})

	// 4. Adjacent-line edits (compare count with direct git merge-file execution)
	t.Run("adjacent-line edits compared with git directly", func(t *testing.T) {
		base := []byte("line1\nline2\nline3\nline4\n")
		local := []byte("line1\nline2-local\nline3\nline4\n")
		upstream := []byte("line1\nline2\nline3-upstream\nline4\n")

		tmpDir := t.TempDir()
		bFile := filepath.Join(tmpDir, "b")
		lFile := filepath.Join(tmpDir, "l")
		uFile := filepath.Join(tmpDir, "u")
		_ = os.WriteFile(bFile, base, 0o600)
		_ = os.WriteFile(lFile, local, 0o600)
		_ = os.WriteFile(uFile, upstream, 0o600)

		cmd := exec.Command("git", "merge-file", "-p", "--diff3", "-L", "local", "-L", "base", "-L", "upstream", lFile, bFile, uFile)
		out, directErr := cmd.Output()
		var expectedConflicts int
		if directErr != nil {
			if exitErr, ok := directErr.(*exec.ExitError); ok {
				expectedConflicts = exitErr.ExitCode()
			}
		}

		merged, conflicts, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if conflicts != expectedConflicts {
			t.Fatalf("expected %d conflicts matching git, got %d", expectedConflicts, conflicts)
		}
		if !bytes.Equal(merged, out) {
			t.Fatalf("merged bytes mismatch with direct git")
		}
	})

	// 5. Identical edits on both sides (0 conflicts)
	t.Run("identical edits on both sides", func(t *testing.T) {
		base := []byte("line1\nline2\n")
		local := []byte("line1\nline2-identical\n")
		upstream := []byte("line1\nline2-identical\n")

		merged, conflicts, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if conflicts != 0 {
			t.Fatalf("expected 0 conflicts for identical edits, got %d", conflicts)
		}
		if !bytes.Equal(merged, local) {
			t.Fatalf("expected identical result %q, got %q", local, merged)
		}
	})

	// 6. Missing trailing newline on one side
	t.Run("missing trailing newline on one side", func(t *testing.T) {
		base := []byte("line1\nline2\n")
		local := []byte("line1\nline2")
		upstream := []byte("line1\nline2\n")

		merged, _, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if len(merged) == 0 {
			t.Fatalf("expected non-empty merged result")
		}
	})

	// 7. CRLF companion file with non-overlapping edits (clean, CRLF preserved)
	t.Run("crlf companion file preserved", func(t *testing.T) {
		base := []byte("line1\r\nline2\r\nline3\r\nline4\r\n")
		local := []byte("line1\r\nline2-local\r\nline3\r\nline4\r\n")
		upstream := []byte("line1\r\nline2\r\nline3\r\nline4-upstream\r\n")
		expected := []byte("line1\r\nline2-local\r\nline3\r\nline4-upstream\r\n")

		merged, conflicts, err := gitMergeFile(ctx, root, base, local, upstream)
		if err != nil {
			t.Fatalf("gitMergeFile failed: %v", err)
		}
		if conflicts != 0 {
			t.Fatalf("expected 0 conflicts, got %d", conflicts)
		}
		if !bytes.Equal(merged, expected) {
			t.Fatalf("expected CRLF merged %q, got %q", expected, merged)
		}
	})

	// 8. NUL-containing input rejected by mergeableText
	t.Run("nul containing input rejected", func(t *testing.T) {
		nulData := []byte("line1\x00line2")
		if mergeableText(nulData) {
			t.Fatalf("expected mergeableText to return false for NUL byte")
		}
		cleanData := []byte("valid utf-8 content")
		if !mergeableText(cleanData) {
			t.Fatalf("expected mergeableText to return true for valid text")
		}
	})

	// 9. Conflict markers detected by canonical.HasConflictMarker
	t.Run("canonical conflict marker detection", func(t *testing.T) {
		for _, marker := range []string{
			"<<<<<<< local",
			"=======",
			">>>>>>> upstream",
			"  <<<<<<< indented",
			"  ======= indented",
			"  >>>>>>> indented",
		} {
			if !canonical.HasConflictMarker(marker) {
				t.Fatalf("expected HasConflictMarker to be true for %q", marker)
			}
		}
		if canonical.HasConflictMarker("plain content\nwithout markers\n") {
			t.Fatalf("expected HasConflictMarker to be false for clean content")
		}
	})

	// 10. gitDiffNoIndex header rewriting and absent side
	t.Run("gitDiffNoIndex header rewriting", func(t *testing.T) {
		from := []byte("hello\n")
		to := []byte("hello\nworld\n")
		diff, err := gitDiffNoIndex(ctx, root, "skills/test/SKILL.md", from, to)
		if err != nil {
			t.Fatalf("gitDiffNoIndex failed: %v", err)
		}
		if !strings.Contains(diff, "--- a/skills/test/SKILL.md") {
			t.Fatalf("expected --- a/skills/test/SKILL.md in diff, got:\n%s", diff)
		}
		if !strings.Contains(diff, "+++ b/skills/test/SKILL.md") {
			t.Fatalf("expected +++ b/skills/test/SKILL.md in diff, got:\n%s", diff)
		}

		// Absent from side -> /dev/null
		diffAdd, err := gitDiffNoIndex(ctx, root, "new.txt", nil, to)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(diffAdd, "--- /dev/null") {
			t.Fatalf("expected --- /dev/null for absent from, got:\n%s", diffAdd)
		}
	})
}
