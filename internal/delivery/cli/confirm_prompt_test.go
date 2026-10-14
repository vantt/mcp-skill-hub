package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests replace package-level terminal stand-ins, so none of them runs
// in parallel.

// atTerminal makes the next commands behave as if a person typed answer.
func atTerminal(t *testing.T, answer string) {
	t.Helper()
	savedInteractive, savedInput := confirmInteractive, confirmInput
	t.Cleanup(func() { confirmInteractive, confirmInput = savedInteractive, savedInput })
	confirmInteractive = func(io.Writer) bool { return true }
	confirmInput = strings.NewReader(answer)
}

func hubHead(t *testing.T, root string) string {
	t.Helper()
	return gitOutput(t, root, "rev-parse", "HEAD")
}

func hubIsClean(t *testing.T, root string) bool {
	t.Helper()
	return gitOutput(t, root, "status", "--porcelain=v1") == ""
}

// confirmCase is one hub-changing command with the hub state it starts from.
type confirmCase struct {
	name  string
	setup func(t *testing.T, hub nextStepHub)
	args  func(hub nextStepHub) []string
	// subject is the Git subject the applied change gets.
	subject string
}

func confirmCases() []confirmCase {
	ready := func(t *testing.T, hub nextStepHub) { hub.createReady("tbl-skill") }
	active := func(t *testing.T, hub nextStepHub) {
		hub.createReady("tbl-skill")
		hub.mustRun("skill", "activate", "tbl-skill", "--yes")
	}
	deprecated := func(t *testing.T, hub nextStepHub) {
		active(t, hub)
		hub.mustRun("skill", "deprecate", "tbl-skill", "--yes")
	}
	return []confirmCase{
		{
			name: "commit",
			setup: func(t *testing.T, hub nextStepHub) {
				hub.createReady("tbl-skill")
				project := projectInDir(t)
				if code, stdout, stderr := runCLI(t, "skill", "checkout", "tbl-skill", "--workspace", hub.root); code != 0 {
					t.Fatalf("checkout exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
				}
				appendToFile(t, filepath.Join(project, ".claude", "skills", "tbl-skill", "SKILL.md"), "\nOne more line.\n")
			},
			args:    func(nextStepHub) []string { return []string{"skill", "commit", "tbl-skill"} },
			subject: "Commit tbl-skill from a project (1 file)",
		},
		{
			name: "edit", setup: ready,
			args: func(nextStepHub) []string {
				return []string{"skill", "edit", "tbl-skill", "--trigger", "cut a release now"}
			},
			subject: "Edit tbl-skill",
		},
		{
			name: "activate", setup: ready,
			args:    func(nextStepHub) []string { return []string{"skill", "activate", "tbl-skill"} },
			subject: "Activate tbl-skill",
		},
		{
			name: "deprecate", setup: active,
			args:    func(nextStepHub) []string { return []string{"skill", "deprecate", "tbl-skill"} },
			subject: "Deprecate tbl-skill",
		},
		{
			name: "archive", setup: deprecated,
			args:    func(nextStepHub) []string { return []string{"skill", "archive", "tbl-skill"} },
			subject: "Archive tbl-skill",
		},
		{
			name: "add",
			setup: func(t *testing.T, hub nextStepHub) {
				dir := filepath.Join(t.TempDir(), "tbl-added")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				content := "---\nname: tbl-added\ndescription: Added from a folder.\n---\n\n# Added\n\nInstructions.\n"
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Setenv("TBL_ADD_DIR", dir)
			},
			args:    func(nextStepHub) []string { return []string{"skill", "add", os.Getenv("TBL_ADD_DIR")} },
			subject: "Add tbl-added",
		},
		{
			name: "migrate",
			setup: func(t *testing.T, hub nextStepHub) {
				if err := os.Remove(filepath.Join(hub.root, ".skillhub", "schema-version")); err != nil {
					t.Fatal(err)
				}
				gitOutput(t, hub.root, "add", "-A")
				gitOutput(t, hub.root, "commit", "-q", "-m", "hub without a schema marker")
			},
			args:    func(nextStepHub) []string { return []string{"maintain", "migrate", "--to", "5"} },
			subject: "Migrate the hub to schema version 5",
		},
	}
}

func TestConfirmPromptYesAppliesOneCommit(t *testing.T) {
	for _, tc := range confirmCases() {
		t.Run(tc.name, func(t *testing.T) {
			hub := newNextStepHub(t)
			if tc.setup != nil {
				tc.setup(t, hub)
			}
			before := hubHead(t, hub.root)
			atTerminal(t, "y\n")
			code, stdout, stderr := hub.run(tc.args(hub)...)
			if code != 0 {
				t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			if !strings.Contains(stdout, "Apply this change? [y/N] ") {
				t.Fatalf("no prompt:\n%s", stdout)
			}
			if strings.Contains(stdout, "Apply: ") || strings.Contains(stdout, "Nothing written yet.") {
				t.Fatalf("a preview with a prompt must not print the two-step hints:\n%s", stdout)
			}
			if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" {
				t.Fatalf("expected exactly one new commit; output:\n%s", stdout)
			}
			if subject := gitOutput(t, hub.root, "log", "-1", "--format=%s"); subject != tc.subject {
				t.Fatalf("commit subject = %q, want %q", subject, tc.subject)
			}
			if !strings.Contains(stdout, "Committed ") || !strings.Contains(stdout, "Agents using this hub get it now") {
				t.Fatalf("no commit line:\n%s", stdout)
			}
			if !hubIsClean(t, hub.root) {
				t.Fatalf("the commit left files behind:\n%s", gitOutput(t, hub.root, "status", "--porcelain=v1"))
			}
		})
	}
}

func TestConfirmPromptNoOrEmptyWritesNothing(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "maybe\n", ""} {
		for _, tc := range confirmCases() {
			t.Run(tc.name+"/"+strings.TrimSpace(answer), func(t *testing.T) {
				hub := newNextStepHub(t)
				if tc.setup != nil {
					tc.setup(t, hub)
				}
				head := hubHead(t, hub.root)
				atTerminal(t, answer)
				code, stdout, stderr := hub.run(tc.args(hub)...)
				if code != 0 {
					t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
				}
				if !strings.Contains(stdout, "Nothing written.") {
					t.Fatalf("missing the nothing written line:\n%s", stdout)
				}
				if hubHead(t, hub.root) != head || !hubIsClean(t, hub.root) {
					t.Fatalf("an unconfirmed change touched the hub:\n%s", gitOutput(t, hub.root, "status", "--porcelain=v1"))
				}
			})
		}
	}
}

func TestPreviewWithoutTerminalPrintsHintsAndWritesNothing(t *testing.T) {
	for _, tc := range confirmCases() {
		t.Run(tc.name, func(t *testing.T) {
			hub := newNextStepHub(t)
			if tc.setup != nil {
				tc.setup(t, hub)
			}
			head := hubHead(t, hub.root)
			code, stdout, stderr := hub.run(tc.args(hub)...)
			if code != 0 {
				t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			if strings.Contains(stdout, "Apply this change?") {
				t.Fatalf("no terminal, so no prompt:\n%s", stdout)
			}
			for _, want := range []string{"Nothing written yet.\n", " --yes\n"} {
				if !strings.Contains(stdout, want) {
					t.Fatalf("missing %q:\n%s", want, stdout)
				}
			}
			lines := strings.Split(stdout, "\n")
			var apply string
			for _, line := range lines {
				if strings.HasPrefix(line, "Apply: ") {
					apply = line
				}
			}
			if !strings.HasPrefix(apply, "Apply: skillhub ") || !strings.HasSuffix(apply, " --yes") {
				t.Fatalf("apply line = %q\n%s", apply, stdout)
			}
			if hubHead(t, hub.root) != head || !hubIsClean(t, hub.root) {
				t.Fatalf("a preview touched the hub:\n%s", gitOutput(t, hub.root, "status", "--porcelain=v1"))
			}
		})
	}
}

func TestJSONNeverPrompts(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("tbl-skill")
	head := hubHead(t, hub.root)
	atTerminal(t, "y\n")
	code, stdout, stderr := hub.run("skill", "activate", "tbl-skill", "--json")
	if code != 0 || strings.Contains(stdout, "Apply this change?") {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if hubHead(t, hub.root) != head {
		t.Fatal("--json applied a change without --yes")
	}
}

func TestYesNeverPromptsAndCommits(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("tbl-skill")
	before := hubHead(t, hub.root)
	atTerminal(t, "n\n")
	code, stdout, stderr := hub.run("skill", "activate", "tbl-skill", "--yes")
	if code != 0 || strings.Contains(stdout, "Apply this change?") {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatal("--yes did not commit")
	}
}

func TestNoCommitWritesWithoutCommitting(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("tbl-skill")
	head := hubHead(t, hub.root)
	code, stdout, stderr := hub.run("skill", "activate", "tbl-skill", "--yes", "--no-commit")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if hubHead(t, hub.root) != head || hubIsClean(t, hub.root) {
		t.Fatal("--no-commit must leave the written files uncommitted")
	}
	if strings.Contains(stdout, "Committed ") || !strings.Contains(stdout, "Commit with") {
		t.Fatalf("without a commit the output keeps the manual commit hint:\n%s", stdout)
	}
}

func TestNoCommitIsRefusedByCommandsThatDoNotChangeTheHub(t *testing.T) {
	hub := newNextStepHub(t)
	for _, args := range [][]string{{"skill", "list", "--no-commit"}, {"skill", "show", "x", "--no-commit"}} {
		code, _, stderr := hub.run(args...)
		if code != 2 || !strings.Contains(stderr, "--no-commit is available only for commands that change the hub") {
			t.Fatalf("%v: exit = %d stderr=%s", args, code, stderr)
		}
	}
}

func TestDiffPrintsTheUnifiedDiffAndFullDiffIsGone(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("tbl-skill")
	head := hubHead(t, hub.root)
	code, stdout, stderr := hub.run("skill", "activate", "tbl-skill", "--diff")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "--- a/") || !strings.Contains(stdout, "+++ b/") || !strings.Contains(stdout, "@@") {
		t.Fatalf("--diff printed no unified diff:\n%s", stdout)
	}
	if strings.Contains(stdout, "Show the diff:") {
		t.Fatalf("the diff hint is pointless once the diff is shown:\n%s", stdout)
	}
	if hubHead(t, hub.root) != head || !hubIsClean(t, hub.root) {
		t.Fatal("--diff must not write")
	}

	code, _, stderr = hub.run("skill", "activate", "tbl-skill", "--full-diff")
	if code != 2 || !strings.Contains(stderr, `unknown argument "--full-diff"`) {
		t.Fatalf("--full-diff: exit = %d stderr=%s", code, stderr)
	}
}

func TestChangeToFilesWithUncommittedEditsIsRefusedBeforeWriting(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("tbl-skill")
	skillFile := filepath.Join(hub.root, "skills", "software", "tbl-skill", ".meta", "skill.yaml")
	original, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	edited := append([]byte("# edited by hand\n"), original...)
	if err := os.WriteFile(skillFile, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	head := hubHead(t, hub.root)

	code, stdout, stderr := hub.run("skill", "activate", "tbl-skill", "--yes")
	if code != 2 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{"has edits that were not committed", "WHY: Skill Hub commits each change with only its own files", "FIX: Commit them (git -C ", "--no-commit"} {
		if !strings.Contains(flatText(stderr), want) {
			t.Fatalf("missing %q:\n%s", want, stderr)
		}
	}
	if hubHead(t, hub.root) != head {
		t.Fatal("a refused change must not commit")
	}
	if got, _ := os.ReadFile(skillFile); !bytes.Equal(got, edited) {
		t.Fatal("a refused change must not write")
	}

	code, stdout, stderr = hub.run("skill", "activate", "tbl-skill", "--yes", "--no-commit")
	if code != 0 {
		t.Fatalf("--no-commit must still apply: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

func TestFailedCommitKeepsFilesAndPrintsTheManualCommand(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("tbl-skill")
	hook := filepath.Join(hub.root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'hook says no' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	head := hubHead(t, hub.root)
	code, stdout, stderr := hub.run("skill", "activate", "tbl-skill", "--yes")
	if code != 0 {
		t.Fatalf("an applied change whose commit failed still succeeds: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{"is now active", "Not committed: hook says no", "Finish it with: git -C "} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("missing %q:\n%s", want, stdout)
		}
	}
	if hubHead(t, hub.root) != head || hubIsClean(t, hub.root) {
		t.Fatal("the files must stay written and uncommitted")
	}
}

func TestSourceCaptureCommitsAndHonoursNoCommit(t *testing.T) {
	hub := newNextStepHub(t)
	before := hubHead(t, hub.root)
	code, stdout, stderr := hub.run("source", "capture", "https://github.com/owner/repo.git", "--reason", "worth a look")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" || !hubIsClean(t, hub.root) {
		t.Fatalf("capture must be one commit that holds all its files:\n%s", gitOutput(t, hub.root, "status", "--porcelain=v1"))
	}
	if subject := gitOutput(t, hub.root, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Capture SRC-") && !strings.HasPrefix(subject, "Capture ") {
		t.Fatalf("subject = %q", subject)
	}

	head := hubHead(t, hub.root)
	code, stdout, stderr = hub.run("source", "capture", "https://github.com/owner/other.git", "--reason", "another", "--no-commit")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if hubHead(t, hub.root) != head || hubIsClean(t, hub.root) {
		t.Fatal("--no-commit must leave the capture uncommitted")
	}
}

// readerFunc lets a test act at the moment the person answers.
type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestMigrateYesRefusesAProposalThatChangedSincePreview(t *testing.T) {
	hub := newNextStepHub(t)
	marker := filepath.Join(hub.root, ".skillhub", "schema-version")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, hub.root, "add", "-A")
	gitOutput(t, hub.root, "commit", "-q", "-m", "hub without a schema marker")
	atTerminal(t, "")
	answered := false
	confirmInput = readerFunc(func(p []byte) (int, error) {
		if answered {
			return 0, io.EOF
		}
		answered = true
		// The hub changes while the preview is on screen.
		if err := os.WriteFile(marker, []byte("4\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitOutput(t, hub.root, "add", "-A")
		gitOutput(t, hub.root, "commit", "-q", "-m", "someone else changed the marker")
		return copy(p, "y\n"), nil
	})
	code, stdout, _ := hub.run("maintain", "migrate", "--to", "5")
	if code == 0 {
		t.Fatalf("a changed proposal was applied:\n%s", stdout)
	}
	if subject := gitOutput(t, hub.root, "log", "-1", "--format=%s"); subject != "someone else changed the marker" {
		t.Fatalf("a commit was made for a proposal the person never saw (%q):\n%s", subject, stdout)
	}
	if got, _ := os.ReadFile(marker); string(got) != "4\n" {
		t.Fatalf("marker = %q, want it untouched", got)
	}
}
