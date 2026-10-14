package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

func guidedCreate(t *testing.T, root, answers string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runGuidedSkillCreate(context.Background(), app.SkillService{}, skillFlags{workspace: root}, strings.NewReader(answers), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func readSkillFiles(t *testing.T, root, collection, id string) (string, string) {
	t.Helper()
	dir := filepath.Join(root, "skills", collection, id)
	entry, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := os.ReadFile(filepath.Join(dir, ".meta", "skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(entry), string(meta)
}

func TestGuidedSkillCreateWritesWhatTheFlagFormWrites(t *testing.T) {
	t.Parallel()
	guidedRoot := initTestWorkspace(t)
	code, stdout, stderr := guidedCreate(t, guidedRoot, "Commit style\n\nWrite commit messages in our style\nwrite a commit message\ndraft a commit\n\nwrite release notes\n\n")
	if code != 0 {
		t.Fatalf("guided create exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	file := filepath.Join(guidedRoot, "skills", "default", "commit-style", "SKILL.md")
	for _, want := range []string{"commit-style", "Draft skill", file, "skillhub skill activate commit-style --yes", "[commit-style]"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output missing %q:\n%s", want, stdout)
		}
	}

	flagRoot := initTestWorkspace(t)
	code, _, stderr = runCLI(t, "skill", "create", "--workspace", flagRoot, "--id", "commit-style", "--collection", "default", "--name", "Commit style",
		"--description", "Write commit messages in our style", "--operation", "other", "--trigger", "write a commit message", "--trigger", "draft a commit",
		"--not-for", "write release notes", "--min-scope", "single_step", "--yes")
	if code != 0 {
		t.Fatalf("flag create exit = %d: %s", code, stderr)
	}
	guidedEntry, guidedMeta := readSkillFiles(t, guidedRoot, "default", "commit-style")
	flagEntry, flagMeta := readSkillFiles(t, flagRoot, "default", "commit-style")
	if guidedEntry != flagEntry || guidedMeta != flagMeta {
		t.Fatalf("guided and flag forms differ:\n%s\n---\n%s\n=====\n%s\n---\n%s", guidedEntry, guidedMeta, flagEntry, flagMeta)
	}
}

func TestGuidedSkillCreateRetriesAnInvalidOrDuplicateID(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	if code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "taken", "--collection", "core", "--name", "Taken", "--description", "Taken.", "--yes"); code != 0 {
		t.Fatalf("setup create exit = %d: %s", code, stderr)
	}
	code, stdout, stderr := guidedCreate(t, root, "Taken\n\nBad Id!\ntaken\nfresh-one\nDescription.\nwhen asked\n\nnever\n\n")
	if code != 0 {
		t.Fatalf("guided create exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "lowercase letters, digits and hyphens") || !strings.Contains(stdout, `"taken" already exists`) {
		t.Fatalf("expected retry messages:\n%s", stdout)
	}
	readSkillFiles(t, root, "core", "fresh-one")
}

func TestGuidedSkillCreateEmptyRequiredAnswerWritesNothing(t *testing.T) {
	t.Parallel()
	for name, answers := range map[string]string{
		"name":        "\n",
		"description": "Some name\n\n\n",
		"trigger":     "Some name\n\nA description\n\n",
		"not for":     "Some name\n\nA description\nwhen asked\n\n\n",
	} {
		root := initTestWorkspace(t)
		code, _, stderr := guidedCreate(t, root, answers)
		if code != 2 || !strings.Contains(stderr, "Nothing was written") {
			t.Fatalf("%s: exit = %d stderr=%s", name, code, stderr)
		}
		if _, err := os.Stat(filepath.Join(root, "skills", "default")); err == nil {
			t.Fatalf("%s: a skill was written", name)
		}
	}
}

func TestGuidedSkillCreateStopsOnEndOfInputAndInterrupt(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	if code, _, stderr := guidedCreate(t, root, "Some name\n"); code != 130 || !strings.Contains(stderr, "Nothing was written") {
		t.Fatalf("end of input: exit = %d stderr=%s", code, stderr)
	}

	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := runGuidedSkillCreate(ctx, app.SkillService{}, skillFlags{workspace: root}, reader, &stdout, &stderr); code != 130 {
		t.Fatalf("interrupt: exit = %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "default")); err == nil {
		t.Fatal("a skill was written")
	}
}

func TestSkillCreateWithoutFlagsOutsideATerminalNamesTheMissingFlags(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, stderr := runCLI(t, "skill", "create", "--workspace", root)
	if code != 2 || !strings.Contains(stderr, "create requires --id, --collection, --name, and --description") || strings.Contains(stdout, "Name of the skill") {
		t.Fatalf("exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "default")); err == nil {
		t.Fatal("a skill was written")
	}
}

// Not parallel: it replaces the terminal check and the answer source for the whole package.
func TestSkillCreateInATerminalAsksQuestions(t *testing.T) {
	root := initTestWorkspace(t)
	savedInteractive, savedInput := skillCreateInteractive, skillCreateInput
	t.Cleanup(func() { skillCreateInteractive, skillCreateInput = savedInteractive, savedInput })
	skillCreateInteractive = func(io.Writer) bool { return true }
	skillCreateInput = strings.NewReader("Review checklist\n\nReview a change\nreview my code\n\nwrite prose\n\n")
	code, stdout, stderr := runCLI(t, "skill", "create", "--workspace", root)
	if code != 0 {
		t.Fatalf("exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	readSkillFiles(t, root, "default", "review-checklist")
	if !strings.Contains(stdout, "skillhub skill activate review-checklist --yes") {
		t.Fatalf("missing next command:\n%s", stdout)
	}
}

func TestGuidedSkillCreateOnAHubWithSeveralCollectionsUsesTheDefaultCollection(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	for _, collection := range []string{"software", "operations"} {
		if code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "seed-"+collection, "--collection", collection, "--name", "Seed", "--description", "Seed.", "--yes"); code != 0 {
			t.Fatalf("setup create exit = %d: %s", code, stderr)
		}
	}
	code, stdout, stderr := guidedCreate(t, root, "Fresh one\n\nDescription.\nwhen asked\n\nnever\n\n")
	if code != 0 {
		t.Fatalf("guided create exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	readSkillFiles(t, root, "default", "fresh-one")
}
