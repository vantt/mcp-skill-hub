package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

// gitForTest runs git in root without system or user configuration.
func gitForTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func systemSkillNotices(t *testing.T, root string) []Finding {
	t.Helper()
	var notices []Finding
	for _, finding := range newSystemSkillProvider().Check(t.Context(), RoleReport{Cwd: root, Hub: &HubRef{Root: root}}, DepthFull) {
		if finding.Code != distillLabNodeMissing {
			notices = append(notices, finding)
		}
	}
	return notices
}

func TestNewHubKeepsGeneratedSystemSkillsOutOfGit(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	ignore, err := os.ReadFile(filepath.Join(hub, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	folders := []string{".agents/skills/system-curator/", ".claude/skills/system-curator/", ".agents/skills/distill-lab/", ".claude/skills/distill-lab/"}
	for _, folder := range folders {
		if strings.Count(string(ignore), "/"+folder+"\n") != 1 {
			t.Fatalf(".gitignore lacks /%s once:\n%s", folder, ignore)
		}
		if _, err := os.Stat(filepath.Join(hub, filepath.FromSlash(folder), "SKILL.md")); err != nil {
			t.Fatalf("init did not write %s: %v", folder, err)
		}
	}
	committed := gitForTest(t, hub, "ls-tree", "-r", "--name-only", "HEAD")
	status := gitForTest(t, hub, "status", "--porcelain", "--untracked-files=all")
	for _, folder := range folders {
		if strings.Contains(committed, folder) || strings.Contains(status, folder) {
			t.Fatalf("%s is in Git:\ncommitted:\n%s\nstatus:\n%s", folder, committed, status)
		}
	}
	if notices := systemSkillNotices(t, hub); len(notices) != 0 {
		t.Fatalf("a new hub has notices: %+v", notices)
	}
}

func TestOldHubGetsTheSystemSkillIgnoreEntriesFromDoctorFix(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	ignorePath := filepath.Join(hub, ".gitignore")
	ignore, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(ignore), "\n") {
		if !strings.Contains(line, "/skills/") {
			kept = append(kept, line)
		}
	}
	if err := os.WriteFile(ignorePath, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	report := RoleReport{Cwd: hub, Hub: &HubRef{Root: hub}}
	sections := Run(t.Context(), report, DepthFull)
	missing := findingByCode(t, sections, "gitignore_entry_missing")
	for _, entry := range []string{"/.agents/skills/system-curator/", "/.claude/skills/distill-lab/"} {
		if !strings.Contains(missing.Message, entry) {
			t.Fatalf("missing entries = %q", missing.Message)
		}
	}
	ApplyFixes(t.Context(), sections)
	if finding := findingByCodeOrOK(Run(t.Context(), report, DepthFull), "gitignore_entry_missing"); finding.Severity != SeverityOK {
		t.Fatalf("after doctor --fix: %+v", finding)
	}
}

func TestTrackedSystemSkillFileIsANoticeThatDoesNotStopTheRepair(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	gitForTest(t, hub, "add", "-f", "--", ".claude/skills/system-curator/SKILL.md")
	gitForTest(t, hub, "commit", "-q", "-m", "track the curator wrapper")
	if err := os.RemoveAll(filepath.Join(hub, ".claude", "skills", "distill-lab")); err != nil {
		t.Fatal(err)
	}
	report := RoleReport{Cwd: hub, Hub: &HubRef{Root: hub}}
	sections := Run(t.Context(), report, DepthFull)
	tracked := findingByCode(t, sections, "system_skill_tracked")
	if tracked.Severity != SeverityNotice || tracked.Fix != nil || !strings.HasSuffix(tracked.Manual, " rm --cached -- .claude/skills/system-curator/SKILL.md") || !strings.HasPrefix(tracked.Manual, "git -C ") {
		t.Fatalf("tracked notice = %+v", tracked)
	}
	if count := len(slices.DeleteFunc(systemSkillNotices(t, hub), func(finding Finding) bool { return finding.Code != "system_skill_tracked" })); count != 1 {
		t.Fatalf("tracked notices = %d", count)
	}
	if slices.Contains(ProblemCodes(sections), "system_skill_tracked") {
		t.Fatal("a tracked system skill file is a problem")
	}
	ApplyFixes(t.Context(), sections)
	if _, err := os.Stat(filepath.Join(hub, ".claude", "skills", "distill-lab", "SKILL.md")); err != nil {
		t.Fatalf("doctor --fix skipped the host repair: %v", err)
	}
	if !strings.Contains(gitForTest(t, hub, "ls-files"), ".claude/skills/system-curator/SKILL.md") {
		t.Fatal("doctor --fix changed what Git tracks")
	}
}

func TestTrackedUserFileGetsNoUntrackCommand(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	notes := filepath.Join(hub, ".claude", "skills", "distill-lab", "notes.md")
	if err := os.WriteFile(notes, []byte("my notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitForTest(t, hub, "add", "-f", "--", ".claude/skills/distill-lab/notes.md")
	gitForTest(t, hub, "commit", "-q", "-m", "my notes")
	for _, section := range Run(t.Context(), RoleReport{Cwd: hub, Hub: &HubRef{Root: hub}}, DepthFull) {
		for _, finding := range section.Findings {
			if strings.Contains(finding.Manual, "rm --cached") && strings.Contains(finding.Manual, "notes.md") {
				t.Fatalf("untrack command for a user file: %+v", finding)
			}
		}
	}
}

func TestUntrackedUserFileInAnIgnoredSkillFolderIsNamed(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	script := filepath.Join(hub, ".claude", "skills", "distill-lab", "scripts", "distill.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("print('hand copy')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(hub, ".claude", "skills", "my-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mine, []byte("---\nname: my-skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notices := systemSkillNotices(t, hub)
	if len(notices) != 1 || notices[0].Code != "system_skill_unowned_ignored" || notices[0].Severity != SeverityNotice ||
		!strings.Contains(notices[0].Message, ".claude/skills/distill-lab/scripts/distill.py") || !strings.Contains(notices[0].Manual, "add -f -- .claude/skills/distill-lab/scripts/distill.py") {
		t.Fatalf("notices = %+v", notices)
	}
	if !strings.Contains(gitForTest(t, hub, "status", "--porcelain", "--untracked-files=all"), ".claude/skills/my-skill/SKILL.md") {
		t.Fatal("a user skill outside the system skill folders is ignored")
	}
}

func TestSystemSkillNoticesNeedAGitRepository(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "hub")
	if _, err := (WorkspaceService{}).Init(root, InitOptions{Store: userconfig.NewStore(t.TempDir())}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if notices := systemSkillNotices(t, root); len(notices) != 0 {
		t.Fatalf("notices without Git: %+v", notices)
	}
}
