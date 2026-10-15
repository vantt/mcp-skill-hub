package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newCommitRepo makes a Git repository with an identity and one commit, using a
// Git config that is isolated from the machine's.
func newCommitRepo(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.com")
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a\n")
	writeRepoFile(t, root, "skills/default/b/SKILL.md", "b\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "first")
	return root
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeRepoFile(t *testing.T, root, path, contents string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitPathsCommitsOnlyTheGivenPaths(t *testing.T) {
	root := newCommitRepo(t)
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	writeRepoFile(t, root, "skills/default/new/SKILL.md", "new\n")
	writeRepoFile(t, root, "skills/default/b/SKILL.md", "b edited by hand\n")
	writeRepoFile(t, root, "staged.txt", "staged\n")
	git(t, root, "add", "staged.txt")
	before := git(t, root, "rev-parse", "HEAD")

	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md", "skills/default/new/SKILL.md"}, nil, "Edit a\n\nSkillhub-Operation: OP-1", false)
	if outcome.Status != CommitCommitted || outcome.Hash == "" || outcome.Message != "Edit a" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if git(t, root, "rev-parse", "HEAD") == before || git(t, root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatal("expected exactly one new commit")
	}
	if files := git(t, root, "show", "--name-only", "--format=", "HEAD"); files != "skills/default/a/SKILL.md\nskills/default/new/SKILL.md" {
		t.Fatalf("commit holds %q", files)
	}
	status := git(t, root, "status", "--porcelain=v1")
	if !strings.Contains(status, "A  staged.txt") || !strings.Contains(status, "M skills/default/b/SKILL.md") {
		t.Fatalf("unrelated changes were disturbed:\n%s", status)
	}
	if !outcome.Reach.ThisHubNow || outcome.Reach.PushNeeded {
		t.Fatalf("reach without a remote = %+v", outcome.Reach)
	}
}

func TestCommitPathsCommitsDeletesAndReportsPushNeeded(t *testing.T) {
	root := newCommitRepo(t)
	git(t, root, "remote", "add", "origin", "https://example.invalid/hub.git")
	if err := os.Remove(filepath.Join(root, "skills/default/a/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Remove a", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if git(t, root, "show", "--name-status", "--format=", "HEAD") != "D\tskills/default/a/SKILL.md" {
		t.Fatal("the delete was not committed")
	}
	if !outcome.Reach.PushNeeded || outcome.Reach.Remote != "origin" {
		t.Fatalf("reach = %+v", outcome.Reach)
	}
}

func TestCommitPathsFailingHookLeavesFilesAndGivesManualCommand(t *testing.T) {
	root := newCommitRepo(t)
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'hook says no' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitFailed || !strings.Contains(outcome.Reason, "hook says no") {
		t.Fatalf("outcome = %+v", outcome)
	}
	if !strings.Contains(outcome.Reason, "staged") {
		t.Fatalf("the reason must say the files are left staged: %q", outcome.Reason)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "skills/default/a/SKILL.md")); string(data) != "a changed\n" {
		t.Fatal("the file must stay written")
	}
	want := "git -C " + root + " add -- skills/default/a/SKILL.md && git -C " + root + " commit -- skills/default/a/SKILL.md"
	if outcome.ManualCommand != want {
		t.Fatalf("manual command = %q, want %q", outcome.ManualCommand, want)
	}
}

func TestCommitPathsSigningWithoutTerminalTimesOut(t *testing.T) {
	previous := commitTimeout
	commitTimeout = 2 * time.Second
	t.Cleanup(func() { commitTimeout = previous })
	root := newCommitRepo(t)
	program := filepath.Join(t.TempDir(), "slow-gpg")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nexec sleep 300\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "config", "commit.gpgsign", "true")
	git(t, root, "config", "gpg.program", program)
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitFailed || !strings.Contains(outcome.Reason, "did not finish within 2 seconds") || !strings.Contains(outcome.Reason, "no terminal") {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestCommitPathsWithoutIdentityNamesTheSetting(t *testing.T) {
	root := newCommitRepo(t)
	git(t, root, "config", "--unset", "user.email")
	git(t, root, "config", "user.useConfigOnly", "true")
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitFailed || !strings.Contains(outcome.Reason, "user.email") {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestCommitPathsSkipsWhenThereIsNothingToCommitWith(t *testing.T) {
	plain := t.TempDir()
	writeRepoFile(t, plain, "skills/default/a/SKILL.md", "a\n")
	outcome := CommitPaths(context.Background(), plain, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false)
	if outcome.Status != CommitSkipped || outcome.Reason == "" {
		t.Fatalf("non-Git root: %+v", outcome)
	}

	root := newCommitRepo(t)
	nested := filepath.Join(root, "hub")
	writeRepoFile(t, nested, "skills/default/x/SKILL.md", "x\n")
	if outcome := CommitPaths(context.Background(), nested, []string{"skills/default/x/SKILL.md"}, nil, "Add x", false); outcome.Status != CommitSkipped {
		t.Fatalf("a hub inside another repository must not commit into it: %+v", outcome)
	}

	if outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md"}, nil, "Edit a", false); outcome.Status != CommitSkipped {
		t.Fatalf("unchanged files: %+v", outcome)
	}
}

func TestCommitPathsRecordsExecutablesWhenGitIgnoresFileModes(t *testing.T) {
	root := newCommitRepo(t)
	// core.filemode=false is how Git runs on Windows: the index never takes the
	// executable bit from the file system.
	git(t, root, "config", "core.filemode", "false")
	writeRepoFile(t, root, "skills/default/a/scripts/run.sh", "#!/bin/sh\n")
	writeRepoFile(t, root, "skills/default/a/notes.md", "notes\n")
	paths := []string{"skills/default/a/notes.md", "skills/default/a/scripts/run.sh"}
	outcome := CommitPaths(context.Background(), root, paths, map[string]bool{"skills/default/a/scripts/run.sh": true, "skills/default/a/gone.sh": true}, "Add a script", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	for path, want := range map[string]string{"skills/default/a/scripts/run.sh": "100755", "skills/default/a/notes.md": "100644"} {
		if mode, _, _ := strings.Cut(git(t, root, "ls-files", "-s", "--", path), " "); mode != want {
			t.Errorf("%s index mode = %s, want %s", path, mode, want)
		}
		if mode, _, _ := strings.Cut(git(t, root, "ls-tree", "HEAD", "--", path), " "); mode != want {
			t.Errorf("%s committed mode = %s, want %s", path, mode, want)
		}
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("the commit left changes behind: %q", status)
	}
}

func TestCommitPathsLeavesOtherStagedChangesAndSkipsPathsGoneSinceHead(t *testing.T) {
	root := newCommitRepo(t)
	writeRepoFile(t, root, "staged.txt", "staged\n")
	git(t, root, "add", "staged.txt")
	// A path staged after HEAD and removed again is not part of any commit.
	writeRepoFile(t, root, "skills/default/a/tmp.md", "tmp\n")
	git(t, root, "add", "skills/default/a/tmp.md")
	if err := os.Remove(filepath.Join(root, "skills", "default", "a", "tmp.md")); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	outcome := CommitPaths(context.Background(), root, []string{"skills/default/a/SKILL.md", "skills/default/a/tmp.md"}, nil, "Edit a", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if files := git(t, root, "show", "--name-only", "--format=", "HEAD"); files != "skills/default/a/SKILL.md" {
		t.Fatalf("commit holds %q", files)
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "A  staged.txt" {
		t.Fatalf("the other staged change was not left alone: %q", status)
	}
}

func TestDirtyPathsReportsModifiedStagedAndUntrackedUnderTheGivenPaths(t *testing.T) {
	root := newCommitRepo(t)
	writeRepoFile(t, root, "skills/default/a/SKILL.md", "a changed\n")
	writeRepoFile(t, root, "skills/default/b/notes/new.md", "new\n")
	git(t, root, "add", "skills/default/b/notes/new.md")
	writeRepoFile(t, root, "skills/default/c/SKILL.md", "c untracked\n")
	writeRepoFile(t, root, "elsewhere.txt", "other\n")

	got, err := DirtyPaths(context.Background(), root, []string{"skills/default/a", "skills/default/b", "skills/default/c/SKILL.md", "skills/default/never"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"skills/default/a/SKILL.md", "skills/default/b/notes/new.md", "skills/default/c/SKILL.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dirty = %v, want %v", got, want)
	}

	if got, err := DirtyPaths(context.Background(), t.TempDir(), []string{"skills"}); err != nil || got != nil {
		t.Fatalf("non-Git root = %v, %v", got, err)
	}
}

func TestCommitPathsTreatsGlobCharactersLiterally(t *testing.T) {
	root := newCommitRepo(t)
	writeRepoFile(t, root, "res/x1.md", "one\n")
	writeRepoFile(t, root, "res/x[12].md", "bracket\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "resources")
	writeRepoFile(t, root, "res/x1.md", "one edited by hand\n")
	writeRepoFile(t, root, "res/x[12].md", "bracket changed\n")

	dirty, err := DirtyPaths(context.Background(), root, []string{"res/x[12].md"})
	if err != nil || len(dirty) != 1 || dirty[0] != "res/x[12].md" {
		t.Fatalf("dirty = %v, err = %v", dirty, err)
	}
	outcome := CommitPaths(context.Background(), root, []string{"res/x[12].md"}, nil, "Edit bracket", false)
	if outcome.Status != CommitCommitted {
		t.Fatalf("outcome = %+v", outcome)
	}
	if files := git(t, root, "show", "--name-only", "--format=", "HEAD"); files != "res/x[12].md" {
		t.Fatalf("commit holds %q, want only the bracket path", files)
	}
	if status := git(t, root, "status", "--porcelain=v1"); status != "M res/x1.md" && status != " M res/x1.md" {
		t.Fatalf("the unrelated edit was not left alone: %q", status)
	}
}
