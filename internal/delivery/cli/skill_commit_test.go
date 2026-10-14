package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests work from a project directory and replace package-level terminal
// stand-ins, so none of them runs in parallel.

// checkedOutProject seeds a skill on a new hub, checks it out into a new
// project and returns the hub, the project and the project folder.
func checkedOutProject(t *testing.T, id string) (nextStepHub, string, string) {
	t.Helper()
	hub := newNextStepHub(t)
	hub.createReady(id)
	hub.mustRun("skill", "activate", id, "--yes")
	project := projectInDir(t)
	if code, stdout, stderr := runCLI(t, "skill", "checkout", id, "--workspace", hub.root); code != 0 {
		t.Fatalf("checkout exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	return hub, project, filepath.Join(project, ".claude", "skills", id)
}

// makeThirdParty marks a seeded skill as imported from a repository and
// commits that, so the hub stays clean.
func makeThirdParty(t *testing.T, hub nextStepHub, id string) {
	t.Helper()
	meta := filepath.Join(hub.root, "skills", "software", id, ".meta", "skill.yaml")
	appendToFile(t, meta, "sources:\n  - id: "+id+"-src\n    roles:\n      - upstream\n    kind: github\n    repo: github.com/anthropics/skills\n")
	gitOutput(t, hub.root, "add", "-A")
	gitOutput(t, hub.root, "commit", "-q", "-m", "mark "+id+" as third-party")
	if code, stdout, stderr := runCLI(t, "maintain", "rebuild", "--workspace", hub.root); code != 0 {
		t.Fatalf("rebuild exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

func TestSkillCheckoutPrintsWhatHappenedAndWhatToDoNext(t *testing.T) {
	hub := newNextStepHub(t)
	hub.createReady("review-checklist")
	project := projectInDir(t)
	before := hubHead(t, hub.root)

	code, stdout, stderr := runCLI(t, "skill", "checkout", "review-checklist", "--workspace", hub.root)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"Copied review-checklist (draft, 1 file) to .claude/skills/review-checklist/\n",
		"Added .claude/skills/review-checklist/ to .gitignore.\n",
		"The hub keeps serving the current version; agents in other projects are not affected.\n",
		"Try it: ask your agent to use /review-checklist.\n",
		"When ready: skillhub skill commit review-checklist\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "review-checklist", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if hubHead(t, hub.root) != before || !hubIsClean(t, hub.root) {
		t.Fatalf("checkout changed the hub")
	}
	code, _, stderr = runCLI(t, "skill", "checkout", "review-checklist", "--workspace", hub.root)
	if code != 2 || !strings.Contains(stderr, "Already checked out: edit it, then run `skillhub skill commit review-checklist`.") {
		t.Fatalf("second checkout: exit = %d stderr=%s", code, stderr)
	}
	code, stdout, _ = runCLI(t, "skill", "checkout", "nope", "--workspace", hub.root, "--json")
	if code == 0 || !strings.Contains(stdout, `"status":"error"`) {
		t.Fatalf("unknown skill: exit = %d stdout=%s", code, stdout)
	}
}

func TestSkillCommitAtATerminalShowsEveryFileAndAsksOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	hub, _, folder := checkedOutProject(t, "review-checklist")
	appendToFile(t, filepath.Join(folder, "SKILL.md"), "\nOne more line.\n")
	if err := os.MkdirAll(filepath.Join(folder, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "scripts", "lint-diff.sh"), []byte("#!/bin/sh\necho lint\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, ".env"), []byte("TOKEN=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := hubHead(t, hub.root)
	atTerminal(t, "y\n")
	code, stdout, stderr := runCLI(t, "skill", "commit", "review-checklist", "--workspace", hub.root)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"review-checklist: 2 files changed since checkout (the hub did not change)\n",
		"  M SKILL.md",
		"  A scripts/lint-diff.sh",
		"Left out: .env (looks like a secret)\n",
		"Routing lint:",
		"Show the diff: skillhub skill commit review-checklist",
		"Apply this change? [y/N] ",
		"Committed ",
		"Agents using this hub get it now",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Apply: ") || strings.Contains(stdout, "Nothing written yet.") {
		t.Fatalf("a preview with a prompt must not print the two-step hints:\n%s", stdout)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatalf("expected exactly one new hub commit:\n%s", stdout)
	}
	if subject := gitOutput(t, hub.root, "log", "-1", "--format=%s"); subject != "Commit review-checklist from a project (2 files)" {
		t.Fatalf("subject = %q", subject)
	}
	if !hubIsClean(t, hub.root) {
		t.Fatalf("the commit left files behind:\n%s", gitOutput(t, hub.root, "status", "--porcelain=v1"))
	}
	atTerminal(t, "y\n")
	code, stdout, _ = runCLI(t, "skill", "commit", "review-checklist", "--workspace", hub.root)
	if code != 0 || !strings.Contains(stdout, "Nothing to commit: review-checklist matches the hub.") || strings.Contains(stdout, "Apply this change?") {
		t.Fatalf("a second commit: exit = %d\n%s", code, stdout)
	}
}

func TestSkillCommitWithoutATerminalPreviewsThenAppliesWithYes(t *testing.T) {
	hub, _, folder := checkedOutProject(t, "review-checklist")
	appendToFile(t, filepath.Join(folder, "SKILL.md"), "\nOne more line.\n")
	before := hubHead(t, hub.root)

	code, stdout, stderr := runCLI(t, "skill", "commit", "review-checklist", "--workspace", hub.root)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{"Nothing written yet.\n", "Show the diff: skillhub skill commit review-checklist --workspace", " --diff\n", "Apply: skillhub skill commit review-checklist --workspace", " --yes\n"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("preview lacks %q:\n%s", want, stdout)
		}
	}
	if hubHead(t, hub.root) != before {
		t.Fatalf("a preview changed the hub")
	}
	code, stdout, _ = runCLI(t, "skill", "commit", "review-checklist", "--workspace", hub.root, "--diff")
	if code != 0 || !strings.Contains(stdout, "+One more line.") || strings.Contains(stdout, "Show the diff:") {
		t.Fatalf("--diff output:\n%s", stdout)
	}
	code, stdout, stderr = runCLI(t, "skill", "commit", "review-checklist", "--workspace", hub.root, "--yes")
	if code != 0 || !strings.Contains(stdout, "Committed ") {
		t.Fatalf("--yes exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatalf("expected one commit")
	}
}

func TestSkillCommitJSONPreviewCarriesFilesExcludedMergeRoutingAndCheckout(t *testing.T) {
	hub, project, folder := checkedOutProject(t, "review-checklist")
	appendToFile(t, filepath.Join(folder, "SKILL.md"), "\nOne more line.\n")
	if err := os.WriteFile(filepath.Join(folder, ".env"), []byte("TOKEN=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "skill", "commit", "review-checklist", "--workspace", hub.root, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var preview struct {
		Status string `json:"status"`
		Case   string `json:"case"`
		Files  []struct {
			Path, Change string
			Added        int
			Removed      int
			Size         int64
		} `json:"files"`
		Excluded []struct{ Path, Reason string } `json:"excluded"`
		Merge    []any                           `json:"merge"`
		Checkout struct {
			Path        string `json:"path"`
			BaseVersion string `json:"base_version"`
		} `json:"checkout"`
		RoutingLint []any `json:"routing_lint"`
	}
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatalf("%v:\n%s", err, stdout)
	}
	if preview.Case != "update" || len(preview.Files) != 1 || preview.Files[0].Path != "SKILL.md" || preview.Files[0].Change != "M" || preview.Files[0].Added != 2 ||
		len(preview.Excluded) != 1 || preview.Excluded[0].Path != ".env" || preview.Merge == nil || preview.Checkout.BaseVersion == "" || !strings.HasSuffix(preview.Checkout.Path, filepath.Join(".claude", "skills", "review-checklist")) {
		t.Fatalf("preview = %+v\n%s", preview, stdout)
	}
	if !strings.HasPrefix(preview.Checkout.Path, project) && !strings.HasSuffix(preview.Checkout.Path, "review-checklist") {
		t.Fatalf("checkout path = %s", preview.Checkout.Path)
	}
}

func TestSkillCommitThirdPartyApprovesInTheSameCommitAndNeverAppliesUnapproved(t *testing.T) {
	hub, _, folder := checkedOutProject(t, "pdf")
	makeThirdParty(t, hub, "pdf")
	appendToFile(t, filepath.Join(folder, "SKILL.md"), "\nOne more line.\n")

	// A person answers no to the approval: nothing is written, and the output says why.
	before := hubHead(t, hub.root)
	atTerminal(t, "y\nn\n")
	code, stdout, stderr := runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root)
	if code != 0 || !strings.Contains(stdout, "Approve this content for agents? It is third-party (github.com/anthropics/skills). [y/N] ") ||
		!strings.Contains(strings.Join(strings.Fields(stdout), " "), "Nothing written. Applying without approval would stop agents from getting pdf") {
		t.Fatalf("declined approval: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if hubHead(t, hub.root) != before || !hubIsClean(t, hub.root) {
		t.Fatalf("declining the approval changed the hub")
	}
	if !strings.Contains(strings.Join(strings.Fields(stdout), " "), "pdf is third-party (github.com/anthropics/skills). The new content needs your approval; without it agents could not use pdf.") {
		t.Fatalf("no trust line:\n%s", stdout)
	}

	// Without a terminal and without the digest the commit is refused.
	code, stdout, stderr = runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root, "--yes")
	if flat := strings.Join(strings.Fields(stderr), " "); code != 2 || !strings.Contains(flat, "applying new content without approval would stop agents from getting it") || !strings.Contains(flat, "--approve-content sha256:") {
		t.Fatalf("unapproved --yes: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if hubHead(t, hub.root) != before {
		t.Fatalf("a refused commit changed the hub")
	}
	code, _, stderr = runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root, "--yes", "--approve-content", "sha256:"+strings.Repeat("0", 64))
	if code != 2 || !strings.Contains(stderr, "does not match the new content") {
		t.Fatalf("wrong digest: exit = %d stderr=%s", code, stderr)
	}

	// At a terminal both answers land in one commit, and the skill is approved.
	atTerminal(t, "y\ny\n")
	code, stdout, stderr = runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root)
	if code != 0 || !strings.Contains(stdout, "Committed ") {
		t.Fatalf("approved commit: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatalf("content and approval must be one commit")
	}
	if subject := gitOutput(t, hub.root, "log", "-1", "--format=%s"); subject != "Commit pdf from a project (1 file), approve content" {
		t.Fatalf("subject = %q", subject)
	}
	code, stdout, stderr = runCLI(t, "skill", "review", "pdf", "--workspace", hub.root, "--json")
	if code != 0 || !strings.Contains(stdout, `"approved":true`) {
		t.Fatalf("review after the commit: exit = %d\n%s%s", code, stdout, stderr)
	}
}

func TestSkillCommitNewFolderAsksDraftThenActivateAndCommitsOnce(t *testing.T) {
	hub := newNextStepHub(t)
	project := projectInDir(t)
	if code, stdout, stderr := runCLI(t, "skill", "create", "team-release", "--description", "Cut a release for the team",
		"--trigger", "cut a release", "--not-for", "publish a package to npm", "--example", "cut release 1.4 for the api"); code != 0 {
		t.Fatalf("create exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	folder := filepath.Join(project, ".claude", "skills", "team-release")
	// The person writes the instructions and removes the scaffold comment.
	body := strings.ReplaceAll(readFileText(t, filepath.Join(folder, "SKILL.md")), "<!-- skillhub:scaffold -->\n", "")
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, ".env"), []byte("TOKEN=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := hubHead(t, hub.root)

	atTerminal(t, "n\n")
	code, stdout, _ := runCLI(t, "skill", "commit", "team-release", "--workspace", hub.root)
	if code != 0 || !strings.Contains(stdout, "Add team-release as a draft? [y/N] ") || !strings.Contains(stdout, "Nothing written.") || hubHead(t, hub.root) != before {
		t.Fatalf("declined: exit = %d\n%s", code, stdout)
	}

	atTerminal(t, "y\ny\n")
	code, stdout, stderr := runCLI(t, "skill", "commit", "team-release", "--workspace", hub.root)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"team-release is not on the hub: this adds a new skill (collection: default).\n",
		"Files (1,", "Left out: .env (looks like a secret)\n",
		"Routing, suggested from the description (edit with --edit-routing):\n", "trigger", `"cut a release"`, "not for", "min scope",
		"Add team-release as a draft? [y/N] ", "Activate team-release now? [y/N] ",
		"Committed ", `"Add team-release from a project, activate"`,
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout)
		}
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" || !hubIsClean(t, hub.root) {
		t.Fatalf("add and activate must be one commit and leave the hub clean")
	}
	meta := readFileText(t, filepath.Join(hub.root, "skills", "default", "team-release", ".meta", "skill.yaml"))
	if !strings.Contains(meta, "status: active") {
		t.Fatalf("not active:\n%s", meta)
	}
}

func TestSkillCommitNewFolderWithoutATerminalStopsAtDraft(t *testing.T) {
	hub := newNextStepHub(t)
	project := projectInDir(t)
	if code, _, stderr := runCLI(t, "skill", "create", "team-release", "--description", "Cut a release", "--trigger", "cut a release", "--not-for", "npm"); code != 0 {
		t.Fatalf("create: %s", stderr)
	}
	folder := filepath.Join(project, ".claude", "skills", "team-release")
	code, stdout, stderr := runCLI(t, "skill", "commit", folder, "--workspace", hub.root, "--yes", "--collection", "tools")
	if code != 0 || !strings.Contains(stdout, "Committed ") || !strings.Contains(stdout, "Next: it is a draft and agents do not get it yet") {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	meta := readFileText(t, filepath.Join(hub.root, "skills", "tools", "team-release", ".meta", "skill.yaml"))
	if !strings.Contains(meta, "status: draft") {
		t.Fatalf("a commit without a terminal must stop at draft:\n%s", meta)
	}
}

func TestSkillCommitEditRoutingOpensTheSuggestionsInTheEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	hub := newNextStepHub(t)
	project := projectInDir(t)
	if code, _, stderr := runCLI(t, "skill", "create", "team-release", "--description", "Cut a release", "--trigger", "cut a release", "--not-for", "npm"); code != 0 {
		t.Fatalf("create: %s", stderr)
	}
	_ = project
	editor := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\ncat > \"$1\" <<'EOF'\ntriggers:\n  - tag a release candidate\nnot_for:\n  - publishing to npm\nmin_scope: multi_step\nEOF\n"
	if err := os.WriteFile(editor, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editor)
	atTerminal(t, "y\n")
	code, stdout, stderr := runCLI(t, "skill", "commit", "team-release", "--workspace", hub.root, "--edit-routing")
	if code != 0 || !strings.Contains(stdout, `"tag a release candidate"`) {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	meta := readFileText(t, filepath.Join(hub.root, "skills", "default", "team-release", ".meta", "skill.yaml"))
	if !strings.Contains(meta, "tag a release candidate") || !strings.Contains(meta, "multi_step") || strings.Contains(meta, "- cut a release") {
		t.Fatalf("edited routing was not saved:\n%s", meta)
	}
	if code, _, stderr := runCLI(t, "skill", "commit", "team-release", "--workspace", hub.root, "--edit-routing", "--yes"); code != 2 || !strings.Contains(stderr, "--edit-routing") {
		t.Fatalf("--edit-routing needs a terminal: exit = %d stderr=%s", code, stderr)
	}
}

func TestSkillCommitMergeConflictIsResolvedWithAccept(t *testing.T) {
	hub, _, folder := checkedOutProject(t, "pdf")
	hubSkillMD := filepath.Join(hub.root, "skills", "software", "pdf", "SKILL.md")
	replaceInFile(t, hubSkillMD, "Cut a release and tag it.", "Hub wording.")
	gitOutput(t, hub.root, "add", "-A")
	gitOutput(t, hub.root, "commit", "-q", "-m", "hub edit")
	replaceInFile(t, filepath.Join(folder, "SKILL.md"), "Cut a release and tag it.", "Project wording.")

	code, stdout, stderr := runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root)
	if code != 0 || !strings.Contains(stdout, "pdf: 1 file changed in the project; the hub changed 1 file since checkout") ||
		!strings.Contains(stdout, "1 file needs a decision before this can be applied:") || !strings.Contains(stdout, "--accept SKILL.md=hub") || strings.Contains(stdout, "Apply this change?") {
		t.Fatalf("conflict preview: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	conflicts := filepath.Join(t.TempDir(), "conflicts")
	if code, _, stderr := runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root, "--write-conflicts", conflicts); code != 0 {
		t.Fatalf("write-conflicts: %s", stderr)
	}
	if text := readFileText(t, filepath.Join(conflicts, "SKILL.md")); !strings.Contains(text, "<<<<<<<") {
		t.Fatalf("conflict file lacks markers:\n%s", text)
	}
	before := hubHead(t, hub.root)
	code, stdout, stderr = runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root, "--yes")
	if code != 2 || !strings.Contains(stderr, "need a decision") || hubHead(t, hub.root) != before {
		t.Fatalf("--yes with a conflict: exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root, "--yes", "--accept", "SKILL.md=project")
	if code != 0 || !strings.Contains(stdout, "Committed ") {
		t.Fatalf("--accept: exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if got := readFileText(t, hubSkillMD); !strings.Contains(got, "Project wording.") {
		t.Fatalf("project choice not applied:\n%s", got)
	}
	if code, _, stderr := runCLI(t, "skill", "commit", "pdf", "--workspace", hub.root, "--accept", "SKILL.md=nobody"); code != 2 || !strings.Contains(stderr, "hub or <path>=project") && !strings.Contains(stderr, "=hub or") {
		t.Fatalf("bad --accept: exit = %d stderr=%s", code, stderr)
	}
}

func TestStatusInsideAProjectListsOpenCheckouts(t *testing.T) {
	hub, _, folder := checkedOutProject(t, "review-checklist")
	appendToFile(t, filepath.Join(folder, "SKILL.md"), "\nOne more line.\n")
	code, stdout, stderr := runCLI(t, "status", "--workspace", hub.root)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Open checkouts in this project\n  - review-checklist: 1 file changed since checkout · skillhub skill commit review-checklist\n") {
		t.Fatalf("status lacks the open checkout:\n%s", stdout)
	}
	code, jsonOut, _ := runCLI(t, "status", "--workspace", hub.root, "--json")
	if code != 0 || !strings.Contains(jsonOut, `"kind":"open_checkout"`) {
		t.Fatalf("status --json lacks open_checkout:\n%s", jsonOut)
	}
	_, thisFile, _, _ := runtime.Caller(0)
	validateJSONDocument(t, filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "schemas", "curation-home-v1.schema.json"), []byte(jsonOut))
}
