package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nextStepHub is an initialised hub plus a runner that returns the exit code
// and both streams of one command.
type nextStepHub struct {
	t    *testing.T
	root string
}

func newNextStepHub(t *testing.T) nextStepHub {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := runInitTo(t, root, &stdout, &stderr); code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	return nextStepHub{t: t, root: root}
}

func (h nextStepHub) run(args ...string) (int, string, string) {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append(args, "--workspace", h.root), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func (h nextStepHub) mustRun(args ...string) string {
	h.t.Helper()
	code, stdout, stderr := h.run(args...)
	if code != 0 {
		h.t.Fatalf("%s = %d: stdout=%s stderr=%s", strings.Join(args, " "), code, stdout, stderr)
	}
	return stdout
}

// createReady creates a draft whose content and routing are complete.
func (h nextStepHub) createReady(id string) {
	h.t.Helper()
	content := filepath.Join(h.t.TempDir(), id+".md")
	if err := os.WriteFile(content, []byte("---\nname: "+id+"\ndescription: Cut a release\n---\n\n# Release\n\nCut a release and tag it.\n"), 0o600); err != nil {
		h.t.Fatal(err)
	}
	h.mustRun("skill", "create", id, "--collection", "software", "--name", "Release", "--description", "Cut a release",
		"--trigger", "cut a release", "--not-for", "deploying", "--min-scope", "single_step", "--content-file", content, "--yes")
}

// statusCommitCommand is the commit command `skillhub status` prints for the hub.
func (h nextStepHub) statusCommitCommand() string {
	h.t.Helper()
	for _, line := range strings.Split(h.mustRun("status"), "\n") {
		if command, ok := strings.CutPrefix(strings.TrimSpace(line), "$ "); ok && strings.Contains(command, " commit ") {
			return command
		}
	}
	h.t.Fatal("status printed no commit command")
	return ""
}

func TestSkillMutationHintsPrintTheCommitCommandStatusPrints(t *testing.T) {
	t.Parallel()
	hub := newNextStepHub(t)
	hub.createReady("team-release")
	activated := hub.mustRun("skill", "activate", "team-release", "--yes", "--no-commit")
	want := hub.statusCommitCommand()
	if !strings.Contains(activated, want) {
		t.Fatalf("activate output must carry the status command %q:\n%s", want, activated)
	}
	if strings.Contains(activated, "commit`") && !strings.Contains(activated, "add --") {
		t.Fatalf("activate hint commits without adding the new files:\n%s", activated)
	}
	deprecated := hub.mustRun("skill", "deprecate", "team-release", "--yes", "--no-commit")
	if want := hub.statusCommitCommand(); !strings.Contains(deprecated, want) {
		t.Fatalf("deprecate output must carry the status command %q:\n%s", want, deprecated)
	}
}

func TestDraftNextStepFollowsTheReviewNotAlwaysTheEditor(t *testing.T) {
	t.Parallel()
	hub := newNextStepHub(t)
	hub.createReady("team-release")
	edited := hub.mustRun("skill", "edit", "team-release", "--trigger", "cut a release now", "--yes")
	if !strings.Contains(edited, "skillhub skill activate team-release") || strings.Contains(edited, "--editor") {
		t.Fatalf("a complete draft must point at activation, not the editor:\n%s", edited)
	}

	hub.mustRun("skill", "create", "kafka-review", "--collection", "software", "--name", "Kafka Review", "--description", "Review Kafka consumers",
		"--trigger", "review kafka consumers", "--not-for", "brokers", "--min-scope", "single_step", "--yes")
	scaffold := hub.mustRun("skill", "edit", "kafka-review", "--trigger", "review kafka consumer lag", "--yes")
	if !strings.Contains(scaffold, "skill edit kafka-review --editor") {
		t.Fatalf("an untouched scaffold still points at the editor:\n%s", scaffold)
	}
}

func TestActivateFixForAnUntouchedScaffoldNamesTheContentFlags(t *testing.T) {
	t.Parallel()
	hub := newNextStepHub(t)
	hub.mustRun("skill", "create", "kafka-review", "--collection", "software", "--name", "Kafka Review", "--description", "Review Kafka consumers",
		"--trigger", "review kafka consumers", "--not-for", "brokers", "--min-scope", "single_step", "--yes")
	code, stdout, stderr := hub.run("skill", "activate", "kafka-review", "--yes")
	if code == 0 {
		t.Fatalf("activating an untouched scaffold must fail: %s", stdout)
	}
	for _, want := range []string{"--editor", "--content-file"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("FIX must name %s:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "kafka-review `") {
		t.Fatalf("FIX prints an edit command without a flag:\n%s", stderr)
	}
}

func TestSkillEditPreviewWarnsAboutLosingApprovalOfAThirdPartySkill(t *testing.T) {
	t.Parallel()
	hub := newNextStepHub(t)
	hub.createReady("team-release")
	metaPath := filepath.Join(hub.root, "skills", "software", "team-release", ".meta", "skill.yaml")
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	provenance := "\nprovenance:\n  created_by: skillhub\n  origin:\n    kind: github\n    repository: https://github.com/example/skills\n    commit: " + strings.Repeat("a", 40) + "\n"
	if err := os.WriteFile(metaPath, append(meta, provenance...), 0o600); err != nil {
		t.Fatal(err)
	}
	content := filepath.Join(t.TempDir(), "changed.md")
	if err := os.WriteFile(content, []byte("---\nname: team-release\ndescription: Cut a release\n---\n\n# Changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := hub.mustRun("skill", "edit", "team-release", "--content-file", content)
	if !strings.Contains(preview, "skillhub skill review team-release") {
		t.Fatalf("the preview must warn that the content needs approval again:\n%s", preview)
	}
}

func TestReviewCountsOnlyTheUsersFiles(t *testing.T) {
	t.Parallel()
	hub := newNextStepHub(t)
	hub.createReady("team-release")
	script := filepath.Join(hub.root, "skills", "software", "team-release", "scripts", "bump.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("echo bump\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	review := hub.mustRun("skill", "review", "team-release")
	if !strings.Contains(review, "2 files") {
		t.Fatalf("review must count SKILL.md and the script, not hub metadata:\n%s", review)
	}
}
