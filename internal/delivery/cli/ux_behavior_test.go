package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createDraftSkill(t *testing.T, root, id string, extra ...string) {
	t.Helper()
	contentFile := filepath.Join(t.TempDir(), id+"-content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: "+id+"\ndescription: Demo skill.\n---\n\n# Demo\n\nReal procedural instructions to replace untouched scaffold.\n"), 0o600)
	args := append([]string{"skill", "create", "--workspace", root, "--id", id, "--collection", "software", "--name", "Demo", "--description", "Demo skill.", "--trigger", "demo it", "--min-scope", "multi_step", "--content-file", contentFile, "--yes"}, extra...)
	if code, stdout, stderr := runCLI(t, args...); code != 0 {
		t.Fatalf("create exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSkillApplyReportsResultingStateWithoutPreviewText(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "demo-content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: demo\ndescription: Demo skill.\n---\n\n# Demo\n\nReal procedural instructions to replace untouched scaffold.\n"), 0o600)
	code, stdout, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "demo", "--collection", "software", "--name", "Demo", "--description", "Demo skill.", "--trigger", "demo it", "--not-for", "unrelated", "--min-scope", "multi_step", "--content-file", contentFile, "--yes")
	if code != 0 {
		t.Fatalf("create exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Draft skill demo saved.") {
		t.Fatalf("create output lacks the draft result line:\n%s", stdout)
	}
	for _, unwanted := range []string{"No files changed", "confirm these exact pins", "active locally", "published"} {
		if strings.Contains(strings.ToLower(stdout), strings.ToLower(unwanted)) {
			t.Fatalf("applied output must not contain %q:\n%s", unwanted, stdout)
		}
	}
	code, stdout, stderr = runCLI(t, "skill", "edit", "demo", "--workspace", root, "--description", "Changed.", "--yes")
	if code != 0 || !strings.Contains(stdout, "Draft skill demo saved.") {
		t.Fatalf("edit exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI(t, "skill", "activate", "demo", "--workspace", root, "--yes")
	if code != 0 || !strings.Contains(stdout, "Skill demo is now active.") || strings.Contains(stdout, "No files changed") {
		t.Fatalf("activate exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, _ = runCLI(t, "skill", "edit", "demo", "--workspace", root, "--description", "Changed again.", "--yes")
	if code != 0 || !strings.Contains(stdout, "Skill demo updated; it is active.") {
		t.Fatalf("active edit exit = %d stdout=%s", code, stdout)
	}
	code, stdout, _ = runCLI(t, "skill", "deprecate", "demo", "--workspace", root, "--yes")
	if code != 0 || !strings.Contains(stdout, "Skill demo is now deprecated.") {
		t.Fatalf("deprecate exit = %d stdout=%s", code, stdout)
	}
}

func TestSkillPreviewStillExplainsHowToApply(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, _ := runCLI(t, "skill", "create", "--workspace", root, "--id", "demo", "--collection", "software", "--name", "Demo", "--description", "Demo skill.")
	if code != 0 || !strings.Contains(stdout, "No files changed") {
		t.Fatalf("preview exit = %d stdout=%s", code, stdout)
	}
}

func TestSourceHelpDoesNotPromiseCaptureConfirmation(t *testing.T) {
	t.Parallel()
	code, stdout, _ := runCLI(t, "source", "--help")
	if code != 0 || strings.Contains(stdout, "preview; --yes applies") || !strings.Contains(stdout, "applies immediately") {
		t.Fatalf("source help = %d:\n%s", code, stdout)
	}
	root := initTestWorkspace(t)
	code, _, stderr := runCLI(t, "source", "capture", "https://github.com/owner/repo.git", "--reason", "why", "--workspace", root, "--yes")
	if code == 0 || !strings.Contains(stderr, `unknown argument "--yes"`) {
		t.Fatalf("capture must keep rejecting --yes: exit = %d stderr=%s", code, stderr)
	}
}

func TestSkillEditReadsContentFileFromOutsideWorkspace(t *testing.T) {
	root := initTestWorkspace(t)
	createDraftSkill(t, root, "demo", "--not-for", "unrelated")
	outside := filepath.Join(t.TempDir(), "edited.md")
	if err := os.WriteFile(outside, []byte("# Demo\n\nWritten outside the workspace.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "skill", "edit", "demo", "--workspace", root, "--content-file", outside, "--yes")
	if code != 0 {
		t.Fatalf("absolute content file exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	written, err := os.ReadFile(filepath.Join(root, "skills", "software", "demo", "SKILL.md"))
	if err != nil || !strings.Contains(string(written), "Written outside the workspace.") {
		t.Fatalf("canonical SKILL.md = %q, %v", written, err)
	}

	relativeDir := t.TempDir()
	t.Chdir(relativeDir)
	if err := os.WriteFile("relative.md", []byte("# Demo\n\nWritten from a relative path.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "skill", "edit", "demo", "--workspace", root, "--content-file", "relative.md", "--yes"); code != 0 {
		t.Fatalf("relative content file exit = %d stderr=%s", code, stderr)
	}
}

func TestReadContentFileRefusesUnsafeInputs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	if err := os.WriteFile(target, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readContentFile(link); err == nil {
		t.Fatal("symbolic link content file was accepted")
	}
	if _, err := readContentFile(dir); err == nil {
		t.Fatal("directory content file was accepted")
	}
	if _, err := readContentFile(filepath.Join(dir, "missing.md")); err == nil {
		t.Fatal("missing content file was accepted")
	}
	large := filepath.Join(dir, "large.md")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxContentFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := readContentFile(large); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversized content file error = %v", err)
	}
}

func TestSkillShowReadsDraftAndReportsState(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	createDraftSkill(t, root, "demo", "--not-for", "unrelated")
	code, stdout, stderr := runCLI(t, "skill", "show", "demo", "--workspace", root)
	if code != 0 || !strings.Contains(stdout, "State: draft") {
		t.Fatalf("show draft exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if code, _, stderr := runCLI(t, "skill", "show", "missing", "--workspace", root); code == 0 || !strings.Contains(stderr, "skill not found") {
		t.Fatalf("show missing exit = %d stderr=%s", code, stderr)
	}
}

func TestSkillActivateWithoutNotForNamesTheExactFix(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	createDraftSkill(t, root, "demo")
	code, _, stderr := runCLI(t, "skill", "activate", "demo", "--workspace", root, "--yes")
	if code == 0 || !strings.Contains(stderr, "routing_review_rationale") {
		t.Fatalf("activate must keep rejecting an empty not_for: exit = %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, `skillhub skill edit demo --not-for "<when not to use>"`) {
		t.Fatalf("fix does not name the edit command:\n%s", stderr)
	}
	// Following the hint fixes it without losing the other routing fields.
	if code, _, stderr := runCLI(t, "skill", "edit", "demo", "--workspace", root, "--not-for", "unrelated work", "--yes"); code != 0 {
		t.Fatalf("edit --not-for exit = %d stderr=%s", code, stderr)
	}
	if code, stdout, stderr := runCLI(t, "skill", "activate", "demo", "--workspace", root, "--yes"); code != 0 || !strings.Contains(stdout, "now active") {
		t.Fatalf("activate after fix exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestStatusOnlyCallsWorkspaceNewWhenNothingHasBeenAdded(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, _ := runCLI(t, "status", "--workspace", root)
	if code != 0 || !strings.Contains(stdout, "Workspace is new") {
		t.Fatalf("fresh uncommitted workspace status = %d:\n%s", code, stdout)
	}
	createDraftSkill(t, root, "demo", "--not-for", "unrelated")
	code, stdout, _ = runCLI(t, "status", "--workspace", root)
	if code != 0 || strings.Contains(stdout, "Workspace is new") || strings.Contains(stdout, "connect") {
		t.Fatalf("workspace with a draft must not be reported as new:\n%s", stdout)
	}
}

func TestStatusReportsStaleIndexAndRebuildAfterHandEdit(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	createDraftSkill(t, root, "demo", "--not-for", "unrelated")
	entry := filepath.Join(root, "skills", "software", "demo", "SKILL.md")
	file, err := os.OpenFile(entry, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nhand edit\n"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	code, stdout, _ := runCLI(t, "status", "--workspace", root)
	if code != 0 || !strings.Contains(stdout, "stale") || !strings.Contains(stdout, "skillhub rebuild") || strings.Contains(stdout, "0 active skills") {
		t.Fatalf("stale status = %d:\n%s", code, stdout)
	}
}

func TestPerCommandHelpHasRealUsage(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"check": {"--all-due", "Example"}, "distill": {"prepare", "submit", "Example"}, "inbox": {"Example"},
		"insight": {"decide", "confirm", "Example"}, "resolve": {"--request", "Example"}, "resolution": {"replay", "--manifest"},
		"validate": {"Example"}, "rebuild": {"Example"}, "migrate": {"--to", "--yes"}, "diff": {"Example"},
		"telemetry": {"purge --yes", "export"}, "eval": {"manifest", "promote"}, "version": {"--json"},
	}
	for command, fragments := range want {
		for _, args := range [][]string{{command, "--help"}, {"help", command}} {
			code, stdout, stderr := runCLI(t, args...)
			if code != 0 || stderr != "" || strings.Contains(stdout, "Run `skillhub help`") || !strings.HasPrefix(stdout, "Usage: skillhub "+command) {
				t.Fatalf("%v exit = %d stderr=%q stdout=%q", args, code, stderr, stdout)
			}
			for _, fragment := range fragments {
				if !strings.Contains(stdout, fragment) {
					t.Fatalf("%v help is missing %q:\n%s", args, fragment, stdout)
				}
			}
		}
	}
}

func TestTelemetryPurgeWithoutYesShowsExactCommand(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, _, stderr := runCLI(t, "telemetry", "purge", "--workspace", root)
	if code != 2 || !strings.Contains(stderr, "FIX: Run `skillhub telemetry purge --workspace <path> --yes`.") {
		t.Fatalf("purge without --yes exit = %d stderr=%s", code, stderr)
	}
}

func TestTelemetryHealthIsCleanAfterPurgingACorruptStore(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime", "telemetry.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "telemetry", "health", "--workspace", root); code != 2 || !strings.Contains(stderr, "purge") {
		t.Fatalf("corrupt health exit = %d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "telemetry", "purge", "--workspace", root, "--yes"); code != 0 {
		t.Fatalf("purge exit = %d stderr=%s", code, stderr)
	}
	for range 2 {
		code, stdout, stderr := runCLI(t, "telemetry", "health", "--workspace", root)
		if code != 0 || !strings.Contains(stdout, "healthy") || !strings.Contains(stdout, "0 errors") {
			t.Fatalf("health after purge exit = %d stdout=%s stderr=%s", code, stdout, stderr)
		}
	}
}

func TestSkillCreateStarterTemplateAndNextStep(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "starter-skill", "--collection", "core", "--name", "Starter Skill", "--description", "Starter description.", "--yes")
	if code != 0 {
		t.Fatalf("create exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Next:") || !strings.Contains(stdout, "skill activate starter-skill --yes") {
		t.Fatalf("missing Next step in create output:\n%s", stdout)
	}
	if strings.Contains(stdout, "Catalog snapshot:") || strings.Contains(stdout, "Generation:") {
		t.Fatalf("unwanted internals in non-verbose create output:\n%s", stdout)
	}
	content, err := os.ReadFile(filepath.Join(root, "skills", "core", "starter-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, heading := range []string{"## When to use", "## Steps", "## Examples"} {
		if !strings.Contains(string(content), heading) {
			t.Fatalf("starter template missing %q:\n%s", heading, content)
		}
	}
}

func TestSkillShowDisplaysRoutingAndFilePath(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "show-test", "--collection", "core", "--name", "Show Test", "--description", "Show description.", "--trigger", "review code", "--not-for", "write prose", "--min-scope", "single_step", "--yes")
	if code != 0 {
		t.Fatalf("create exit = %d: %s", code, stderr)
	}
	code, stdout, stderr := runCLI(t, "skill", "show", "--workspace", root, "show-test")
	if code != 0 {
		t.Fatalf("show exit = %d: %s", code, stderr)
	}
	for _, want := range []string{"Triggers: review code", "Not for: write prose", "Min scope: single_step", "File: skills/core/show-test/SKILL.md"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("skill show missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "digest-pinned") || strings.Contains(stdout, "Catalog snapshot:") {
		t.Fatalf("unwanted internals in default skill show:\n%s", stdout)
	}
}

func TestSkillActivateMissingRequirementsCombinedCommand(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "missing-reqs", "--collection", "core", "--name", "Missing Reqs", "--description", "Missing description.", "--yes")
	if code != 0 {
		t.Fatalf("create exit = %d: %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "skill", "activate", "--workspace", root, "missing-reqs", "--yes")
	if code != 2 {
		t.Fatalf("expected activate to fail with exit 2, got %d", code)
	}
	if !strings.Contains(stderr, "--trigger") || !strings.Contains(stderr, "--not-for") || !strings.Contains(stderr, "--min-scope") {
		t.Fatalf("activate error does not combine all missing flags in one command:\n%s", stderr)
	}
}

func TestInitRefusesNonEmptyDirectoryUnlessForce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "core", "existing")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "init", dir, "--yes")
	if code != 2 || !strings.Contains(stderr, "Directory is not empty") || !strings.Contains(stderr, "--force") {
		t.Fatalf("init without --force exit = %d stderr = %s", code, stderr)
	}
	code, stdout, stderr := runCLI(t, "init", dir, "--force", "--yes")
	if code != 0 {
		t.Fatalf("init with --force exit = %d stdout = %s stderr = %s", code, stdout, stderr)
	}
}

func TestDiffGroupsDraftSkills(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "draft-diff", "--collection", "core", "--name", "Draft Diff", "--description", "Draft diff description.", "--yes")
	if code != 0 {
		t.Fatalf("create exit = %d: %s", code, stderr)
	}
	code, stdout, stderr := runCLI(t, "diff", "--workspace", root)
	if code != 0 {
		t.Fatalf("diff exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "draft skills") {
		t.Fatalf("diff did not group draft files under 'draft skills':\n%s", stdout)
	}
	if !strings.Contains(stdout, "untracked") && !strings.Contains(stdout, "added") {
		t.Fatalf("diff did not use status words:\n%s", stdout)
	}
	if !strings.Contains(stdout, "git -C") || !strings.Contains(stdout, "commit") {
		t.Fatalf("diff did not include commit command:\n%s", stdout)
	}
}

func TestSkillPreviewShowsExactConfirmCommand(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "preview-test", "--collection", "core", "--name", "Preview Test", "--description", "Preview description.")
	if code != 0 {
		t.Fatalf("create preview exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "skillhub skill confirm --proposal") || !strings.Contains(stdout, "--base-version") {
		t.Fatalf("preview missing exact confirm command:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Base version:") {
		t.Fatalf("preview should use 'Base version', got:\n%s", stdout)
	}
}
