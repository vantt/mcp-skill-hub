package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	scriptPath = "skills/default/a/scripts/run.sh"
	notesPath  = "skills/default/a/notes.md"
)

func writeHook(t *testing.T, root, body string) {
	t.Helper()
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func committedMode(t *testing.T, root, path string) string {
	t.Helper()
	mode, _, _ := strings.Cut(git(t, root, "ls-tree", "HEAD", "--", path), " ")
	return mode
}

// noScratchIndex fails when a scratch index of the commit is left in the Git dir.
func noScratchIndex(t *testing.T, root string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(root, ".git", "skillhub-commit-index*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Fatalf("scratch index left behind: %v", left)
	}
}

func TestCommitPathsHookSeesTheScratchIndexWithOnlyTheCommittedPaths(t *testing.T) {
	root := newCommitRepo(t)
	git(t, root, "config", "core.filemode", "false")
	writeRepoFile(t, root, "staged.txt", "staged\n")
	git(t, root, "add", "staged.txt")
	seen := t.TempDir()
	writeHook(t, root, "#!/bin/sh\ngit ls-files -s > \""+filepath.ToSlash(seen)+"/index\"\ngit diff --cached --name-only > \""+filepath.ToSlash(seen)+"/diff\"\n")
	writeRepoFile(t, root, scriptPath, "#!/bin/sh\n")
	writeRepoFile(t, root, notesPath, "notes\n")
	outcome := CommitPaths(context.Background(), root, []string{notesPath, scriptPath}, map[string]bool{scriptPath: true}, "Add a script", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	index, _ := os.ReadFile(filepath.Join(seen, "index"))
	if !strings.Contains(string(index), "100755 ") || !strings.Contains(string(index), scriptPath) {
		t.Fatalf("hook index lacks the executable script:\n%s", index)
	}
	for _, line := range strings.Split(string(index), "\n") {
		if strings.HasSuffix(line, "\t"+scriptPath) && !strings.HasPrefix(line, "100755 ") {
			t.Fatalf("hook sees %q", line)
		}
	}
	diff, _ := os.ReadFile(filepath.Join(seen, "diff"))
	if got := strings.Fields(string(diff)); strings.Join(got, ",") != notesPath+","+scriptPath {
		t.Fatalf("hook sees staged paths %v, want only the committed ones", got)
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "A  staged.txt" {
		t.Fatalf("status = %q", status)
	}
	noScratchIndex(t, root)
}

func TestCommitPathsLeavesAStagedThenEditedFileAlone(t *testing.T) {
	root := newCommitRepo(t)
	writeRepoFile(t, root, "skills/default/b/SKILL.md", "b staged\n")
	git(t, root, "add", "skills/default/b/SKILL.md")
	writeRepoFile(t, root, "skills/default/b/SKILL.md", "b staged, then edited\n")
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "MM skills/default/b/SKILL.md" {
		t.Fatalf("status = %q", status)
	}
	noScratchIndex(t, root)
}

func TestCommitPathsMakesTheFirstCommitOfAnUnbornHead(t *testing.T) {
	newCommitRepo(t) // isolates the Git config
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "core.filemode", "false")
	writeRepoFile(t, root, scriptPath, "#!/bin/sh\n")
	writeRepoFile(t, root, notesPath, "notes\n")
	outcome := CommitPaths(context.Background(), root, []string{notesPath, scriptPath}, map[string]bool{scriptPath: true}, "First", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if committedMode(t, root, scriptPath) != "100755" || committedMode(t, root, notesPath) != "100644" {
		t.Fatalf("tree = %s", git(t, root, "ls-tree", "-r", "HEAD"))
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("status = %q", status)
	}
	noScratchIndex(t, root)
}

func TestCommitPathsCommitsOnADetachedHead(t *testing.T) {
	root := newCommitRepo(t)
	before := git(t, root, "rev-parse", "HEAD")
	git(t, root, "checkout", "-q", "--detach")
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if parent := git(t, root, "rev-parse", "HEAD^"); parent != before {
		t.Fatalf("parent = %s, want %s", parent, before)
	}
	if branch := git(t, root, "rev-parse", "main"); branch != before {
		t.Fatal("a detached commit must not move the branch")
	}
	if err := exec.Command("git", "-C", root, "symbolic-ref", "-q", "HEAD").Run(); err == nil {
		t.Fatal("HEAD must stay detached")
	}
}

func TestCommitPathsRefusesAnIndexWithUnresolvedConflicts(t *testing.T) {
	root := newCommitRepo(t)
	blob := git(t, root, "rev-parse", "HEAD:skills/default/b/SKILL.md")
	command := exec.Command("git", "-C", root, "update-index", "--index-info")
	command.Stdin = strings.NewReader("0 " + strings.Repeat("0", len(blob)) + "\tconflict.txt\n100644 " + blob + " 1\tconflict.txt\n100644 " + blob + " 2\tconflict.txt\n100644 " + blob + " 3\tconflict.txt\n")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("make conflict: %v\n%s", err, output)
	}
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitFailed || !strings.Contains(outcome.Reason, "conflict") {
		t.Fatalf("outcome = %+v", outcome)
	}
	noScratchIndex(t, root)
}

// The files are written 0644 on every platform, as a Windows file system
// reports them, so the recorded modes can only come from the change.
func TestCommitPathsTakesModesFromTheChangeNotTheFileSystem(t *testing.T) {
	const (
		kept     = "skills/default/a/scripts/kept.sh"
		replaced = "skills/default/a/scripts/replaced.sh"
	)
	for _, fileMode := range []string{"true", "false"} {
		t.Run("core.filemode="+fileMode, func(t *testing.T) {
			root := newCommitRepo(t)
			writeRepoFile(t, root, kept, "#!/bin/sh\necho 1\n")
			writeRepoFile(t, root, replaced, "#!/bin/sh\necho old\n")
			git(t, root, "add", "--chmod=+x", "--", kept, replaced)
			git(t, root, "commit", "-q", "-m", "scripts")
			git(t, root, "config", "core.filemode", fileMode)

			writeRepoFile(t, root, scriptPath, "#!/bin/sh\n")           // new, marked executable
			writeRepoFile(t, root, notesPath, "notes\n")                // new, plain
			writeRepoFile(t, root, kept, "#!/bin/sh\necho 2\n")         // edited, mode untouched
			writeRepoFile(t, root, replaced, "echo plain, recreated\n") // deleted and written again plain
			paths := []string{notesPath, kept, replaced, scriptPath}
			modes := map[string]bool{scriptPath: true, notesPath: false, replaced: false}
			outcome := CommitPaths(context.Background(), root, paths, modes, "Write", false)
			if outcome.Status != CommitCommitted {
				t.Fatalf("outcome = %+v", outcome)
			}
			for path, want := range map[string]string{scriptPath: "100755", notesPath: "100644", kept: "100755", replaced: "100644"} {
				if mode := committedMode(t, root, path); mode != want {
					t.Errorf("%s committed as %s, want %s", path, mode, want)
				}
			}
		})
	}
}

func TestManualCommandAfterAFailedCommitKeepsTheExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the printed command is for PowerShell on Windows")
	}
	root := newCommitRepo(t)
	git(t, root, "config", "core.filemode", "false")
	writeRepoFile(t, root, "staged.txt", "staged\n")
	git(t, root, "add", "staged.txt")
	writeHook(t, root, "#!/bin/sh\necho 'hook says no' >&2\nexit 1\n")
	writeRepoFile(t, root, scriptPath, "#!/bin/sh\n")
	writeRepoFile(t, root, notesPath, "notes\n")
	before := git(t, root, "rev-parse", "HEAD")
	outcome := CommitPaths(context.Background(), root, []string{notesPath, scriptPath}, map[string]bool{scriptPath: true}, "Add a script", false)
	if outcome.Status != CommitFailed || outcome.ManualCommand == "" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if err := os.Remove(filepath.Join(root, ".git", "hooks", "pre-commit")); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", outcome.ManualCommand)
	// The command opens the editor for the message, as a plain `git commit` does.
	editor := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\necho 'Add a script' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command.Env = append(os.Environ(), "GIT_EDITOR="+editor)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("manual command %q: %v\n%s", outcome.ManualCommand, err, output)
	}
	if parent := git(t, root, "rev-parse", "HEAD^"); parent != before {
		t.Fatal("the manual command must make exactly one commit")
	}
	if committedMode(t, root, scriptPath) != "100755" || committedMode(t, root, notesPath) != "100644" {
		t.Fatalf("tree = %s", git(t, root, "ls-tree", "-r", "HEAD"))
	}
	if files := git(t, root, "show", "--name-only", "--format=", "HEAD"); files != notesPath+"\n"+scriptPath {
		t.Fatalf("commit holds %q", files)
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "A  staged.txt" {
		t.Fatalf("status = %q, want only the other staged change", status)
	}
	noScratchIndex(t, root)
}

func TestManualCommandForEachShell(t *testing.T) {
	t.Parallel()
	paths := []string{"skills/a b.md"}
	if got, want := manualCommandFor("linux", "/hub", paths, ""), "git -C /hub add -- 'skills/a b.md' && git -C /hub commit -- 'skills/a b.md'"; got != want {
		t.Errorf("plain = %q, want %q", got, want)
	}
	if got, want := manualCommandFor("linux", "/hub", paths, "/hub/.git/skillhub-commit-index-1"), "GIT_INDEX_FILE=/hub/.git/skillhub-commit-index-1 git -C /hub commit && rm -f /hub/.git/skillhub-commit-index-1"; got != want {
		t.Errorf("posix = %q, want %q", got, want)
	}
	want := `$env:GIT_INDEX_FILE = 'C:\hub\.git\skillhub-commit-index-1'; git -C C:\hub commit; $committed = $?; Remove-Item Env:GIT_INDEX_FILE; if ($committed) { Remove-Item -LiteralPath C:\hub\.git\skillhub-commit-index-1 }`
	if got := manualCommandFor("windows", `C:\hub`, paths, `C:\hub\.git\skillhub-commit-index-1`); got != want {
		t.Errorf("powershell = %q, want %q", got, want)
	}
}
