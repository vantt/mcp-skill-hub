package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withoutGitIdentity points Git at a global config that forbids guessing an
// identity, so the test behaves the same on a machine whose host name would
// let Git make one up.
func withoutGitIdentity(t *testing.T) {
	t.Helper()
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, []byte("[user]\n\tuseConfigOnly = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
}

const (
	setNameLine  = `git config --global user.name "<your name>"`
	setEmailLine = `git config --global user.email "<your email>"`
)

// Not parallel: it changes the Git configuration of the process.
func TestInitRefusesWithoutAGitIdentityAndWritesNothing(t *testing.T) {
	config := isolateInit(t)
	withoutGitIdentity(t)
	dir := filepath.Join(t.TempDir(), "hub")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runInitCLI(t, dir)
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{"ERROR:", "WHY:", "FIX:", "$ " + setNameLine, "$ " + setEmailLine, "skillhub init"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("init wrote into the folder: %v", listNames(t, dir))
	}
	if storedHub(t, config) != "" {
		t.Fatal("init stored a hub it did not create")
	}
}

// Not parallel: it changes the Git configuration of the process.
func TestDoctorReportsAMissingGitIdentity(t *testing.T) {
	isolateInit(t)
	hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
	if code, stdout, _ := runCLI(t, "doctor", "--workspace", hub); code != 0 || strings.Contains(stdout, "user.email") {
		t.Fatalf("doctor with an identity: exit %d\n%s", code, stdout)
	}
	withoutGitIdentity(t)
	code, stdout, stderr := runCLI(t, "doctor", "--workspace", hub)
	flat := strings.Join(strings.Fields(stdout), " ")
	if code != 1 || !strings.Contains(flat, "Git identity") || !strings.Contains(flat, setNameLine) || !strings.Contains(flat, setEmailLine) || strings.Contains(flat, "Healthy.") {
		t.Fatalf("doctor without an identity: exit %d\n%s%s", code, stdout, stderr)
	}
}

// A commit that Git refused leaves the change written but not committed; the
// next `skill commit` of that skill finishes it instead of calling the hub
// up to date. Not parallel: it changes the Git configuration and the working
// directory of the process.
func TestSkillCommitFinishesACommitGitRefusedEarlier(t *testing.T) {
	isolateInit(t)
	hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
	projectInDir(t)
	if code, stdout, stderr := runCLI(t, "skill", "create", "demo", "--workspace", hub, "--description", "Write release notes",
		"--trigger", "write release notes", "--not-for", "write code", "--example", "write the release notes for 2.0"); code != 0 {
		t.Fatalf("create: exit %d\n%s%s", code, stdout, stderr)
	}
	identity := os.Getenv("GIT_CONFIG_GLOBAL")
	withoutGitIdentity(t)
	code, stdout, stderr := runCLI(t, "skill", "commit", "demo", "--workspace", hub, "--yes")
	if code != 0 || !strings.Contains(stdout, "Not committed") {
		t.Fatalf("commit without an identity: exit %d\n%s%s", code, stdout, stderr)
	}
	if status := gitOutput(t, hub, "status", "--porcelain"); status == "" {
		t.Fatal("the refused commit left nothing to finish")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", identity)

	code, stdout, stderr = runCLI(t, "skill", "commit", "demo", "--workspace", hub)
	if code != 0 || strings.Contains(stdout, "matches the hub.") || !strings.Contains(stdout, "not committed to Git") || !strings.Contains(stdout, "--yes") {
		t.Fatalf("preview of the unfinished commit: exit %d\n%s%s", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI(t, "skill", "commit", "demo", "--workspace", hub, "--yes")
	if code != 0 || !strings.Contains(stdout, "Committed") {
		t.Fatalf("finishing the commit: exit %d\n%s%s", code, stdout, stderr)
	}
	if status := gitOutput(t, hub, "status", "--porcelain"); status != "" {
		t.Fatalf("the hub still has uncommitted files:\n%s", status)
	}
	if files := gitOutput(t, hub, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(files, "skills/") || !strings.Contains(files, "history/operations/") {
		t.Fatalf("the commit lacks the skill or its operation record:\n%s", files)
	}
	code, stdout, _ = runCLI(t, "skill", "commit", "demo", "--workspace", hub)
	if code != 0 || !strings.Contains(stdout, "Nothing to commit: demo matches the hub.") {
		t.Fatalf("commit after finishing: exit %d\n%s", code, stdout)
	}
}
