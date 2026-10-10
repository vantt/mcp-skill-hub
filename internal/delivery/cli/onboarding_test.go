package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func initTestWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if code, _, stderr := runCLI(t, "init", root, "--yes"); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr)
	}
	return root
}

func TestDoctorTextReportsNativeCuratorFixCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := initTestWorkspace(t)
	path := filepath.Join(root, ".claude", "skills", "system-curator", "SKILL.md")
	stale := []byte("Previous native curator instructions.\n")
	if err := os.WriteFile(path, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "doctor", "--workspace", root)
	if code != 0 {
		t.Fatalf("doctor exit = %d: %s", code, stderr)
	}
	command := "skillhub doctor --fix --workspace " + root + " --yes"
	text := strings.Join(strings.Fields(stdout), " ")
	if !strings.Contains(text, path) || !strings.Contains(text, command) {
		t.Fatalf("doctor drift report omitted path or fix command %q:\n%s", command, stdout)
	}
	content, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(content, stale) {
		t.Fatalf("doctor drift report changed native curator: %v", err)
	}
}

func TestHelpEntryPointsPrintUsageAndExitZero(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{}, {"help"}, {"--help"}, {"-h"}} {
		code, stdout, stderr := runCLI(t, args...)
		if code != 0 || stderr != "" {
			t.Fatalf("%v exit = %d stderr = %q", args, code, stderr)
		}
		for _, want := range []string{"Get started", "skillhub connect", "curate my Skill Hub", "Setup:", "Skills:", "Sources & learning:", "Agent:", "Maintenance:", "init", "doctor", "resolve", "mcp serve", "telemetry"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("%v help is missing %q:\n%s", args, want, stdout)
			}
		}
	}
}

func TestCommandHelpPrintsThatCommandsUsage(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"init": "--verbose", "connect": "--global", "doctor": "--fix", "status": "--quiet",
		"skill": "list", "source": "capture", "mcp": "serve", "check": "Check sources",
	}
	for command, want := range cases {
		for _, args := range [][]string{{command, "--help"}, {command, "-h"}, {"help", command}} {
			code, stdout, stderr := runCLI(t, args...)
			if code != 0 || stderr != "" || !strings.Contains(stdout, want) {
				t.Fatalf("%v exit = %d stderr = %q stdout = %q, want %q", args, code, stderr, stdout, want)
			}
		}
	}
}

func TestUnknownCommandPointsToHelp(t *testing.T) {
	t.Parallel()
	code, _, stderr := runCLI(t, "frobnicate")
	if code != 2 || !strings.Contains(stderr, "skillhub help") || strings.Contains(stderr, "skillhub version") {
		t.Fatalf("exit = %d stderr = %q", code, stderr)
	}
}

func TestInitDefaultOutputIsShortSummaryWithoutDigests(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	code, stdout, stderr := runCLI(t, "init", root, "--yes")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	for _, want := range []string{"Workspace created at " + root, "claude-code", "codex-cli", "gemini-cli", "Next:", "git -C", root, "skillhub connect", "-g", `curate my Skill Hub`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("init output is missing %q:\n%s", want, stdout)
		}
	}
	for _, unwanted := range []string{"sha256:", "row(s)", "Workspace directory does not exist", "generation"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("init output must not contain %q:\n%s", unwanted, stdout)
		}
	}
}

func TestInitVerboseAndJSONKeepGenerationDigestsAndCounts(t *testing.T) {
	t.Parallel()
	verboseRoot := filepath.Join(t.TempDir(), "workspace")
	code, stdout, stderr := runCLI(t, "init", verboseRoot, "--yes", "--verbose")
	if code != 0 {
		t.Fatalf("verbose exit = %d: %s", code, stderr)
	}
	for _, want := range []string{"sha256:", "row(s)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("verbose init is missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Workspace directory does not exist") {
		t.Fatalf("applied init still reports the missing directory:\n%s", stdout)
	}

	jsonRoot := filepath.Join(t.TempDir(), "workspace")
	code, stdout, stderr = runCLI(t, "init", jsonRoot, "--yes", "--json")
	if code != 0 {
		t.Fatalf("json exit = %d: %s", code, stderr)
	}
	var result app.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != app.StatusApplied || len(result.Items) < 5 {
		t.Fatalf("json init result = %#v", result)
	}
}

func TestInitPreviewDescribesPlannedDirectoryCreation(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	code, stdout, stderr := runCLI(t, "init", root)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Workspace directory will be created.") || strings.Contains(stdout, "does not exist") {
		t.Fatalf("preview wording:\n%s", stdout)
	}
}

func TestConnectPreviewThenApplyWritesProjectFilesOnly(t *testing.T) {
	workspace := initTestWorkspace(t)
	project := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	code, stdout, stderr := runCLI(t, "connect", "--project", project, "--workspace", workspace)
	if code != 0 {
		t.Fatalf("preview exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Preview only") || !strings.Contains(stdout, "--yes") {
		t.Fatalf("preview output:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote files, stat error = %v", err)
	}

	code, stdout, stderr = runCLI(t, "connect", "--project", project, "--workspace", workspace, "--yes")
	if code != 0 || !strings.Contains(stdout, "written") {
		t.Fatalf("apply exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	registration, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	workspaceJSON, _ := json.Marshal(workspace)
	escapedWorkspace := string(workspaceJSON[1 : len(workspaceJSON)-1])
	for _, want := range []string{`"skillhub"`, `"mcp"`} {
		if !strings.Contains(string(registration), want) {
			t.Fatalf(".mcp.json is missing %q:\n%s", want, registration)
		}
	}
	if !strings.Contains(string(registration), workspace) && !strings.Contains(string(registration), escapedWorkspace) {
		t.Fatalf(".mcp.json is missing workspace %q:\n%s", workspace, registration)
	}
	var files []string
	if err := filepath.WalkDir(project, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(project, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	wantFiles := []string{
		".agents/skills/system-curator/SKILL.md",
		".claude/settings.local.json",
		".claude/skills/system-curator/SKILL.md",
		".codex/config.toml",
		".gemini/settings.json",
		".gemini/skills/system-curator/SKILL.md",
		".mcp.json",
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
	}
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("connect file set = %v, want %v (no curator sidecars)", files, wantFiles)
	}

	before, _ := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	code, stdout, stderr = runCLI(t, "connect", "--project", project, "--workspace", workspace, "--yes")
	if code != 0 || !strings.Contains(stdout, "already current") {
		t.Fatalf("idempotent rerun exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	after, _ := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if !bytes.Equal(before, after) {
		t.Fatal("idempotent rerun changed CLAUDE.md")
	}
}

func TestConnectDefaultsToCurrentDirectoryAndHonorsHostFilter(t *testing.T) {
	workspace := initTestWorkspace(t)
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv(workspaceEnvVar, workspace)

	code, _, stderr := runCLI(t, "connect", "--host", "codex", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); err != nil {
		t.Fatalf("codex files missing: %v", err)
	}
	for _, path := range []string{"CLAUDE.md", "GEMINI.md", ".mcp.json"} {
		if _, err := os.Stat(filepath.Join(project, path)); !os.IsNotExist(err) {
			t.Fatalf("%s should not be written for --host codex, stat error = %v", path, err)
		}
	}
}

func TestConnectWithoutWorkspaceExplainsHowToProvideOne(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")
	t.Chdir(t.TempDir())
	code, _, stderr := runCLI(t, "connect")
	if code != 2 || !strings.Contains(stderr, "--workspace") || !strings.Contains(stderr, "SKILLHUB_WORKSPACE") {
		t.Fatalf("exit = %d stderr = %q", code, stderr)
	}
}

func TestConnectRejectsUninitializedWorkspaceAndBadFlags(t *testing.T) {
	t.Parallel()
	empty := t.TempDir()
	if code, _, stderr := runCLI(t, "connect", "--project", t.TempDir(), "--workspace", empty); code != 2 || !strings.Contains(stderr, "skillhub init") {
		t.Fatalf("uninitialized workspace exit = %d stderr = %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "connect", "-g", "--project", empty); code != 2 || !strings.Contains(stderr, "cannot be combined") {
		t.Fatalf("global+project exit = %d stderr = %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "connect", "--host", "vim"); code != 2 || !strings.Contains(stderr, "unsupported host") {
		t.Fatalf("bad host exit = %d stderr = %q", code, stderr)
	}
}

func TestConnectRefusesConflictingManagedBlockWithoutWriting(t *testing.T) {
	t.Parallel()
	workspace := initTestWorkspace(t)
	project := t.TempDir()
	broken := "<!-- skillhub:bootstrap:v1:start -->\nunterminated\n"
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "connect", "--project", project, "--workspace", workspace, "--yes")
	if code != 2 || !strings.Contains(stderr, "Nothing was written") {
		t.Fatalf("exit = %d stderr = %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("conflict refusal still wrote files, stat error = %v", err)
	}
}

func TestConnectGlobalWritesUserScopeAndPreservesClaudeConfig(t *testing.T) {
	workspace := initTestWorkspace(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	existing := "{\n  \"numStartups\": 7,\n  \"projects\": {\"/p\": {\"allowedTools\": []}},\n  \"mcpServers\": {\"keep\": {\"command\": \"keep\"}}\n}\n"
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, "connect", "-g", "--workspace", workspace)
	if code != 0 || !strings.Contains(stdout, "Preview only") {
		t.Fatalf("preview exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".claude.json")); string(got) != existing {
		t.Fatal("global preview modified ~/.claude.json")
	}

	code, _, stderr = runCLI(t, "connect", "--global", "--workspace", workspace, "--yes")
	if code != 0 {
		t.Fatalf("apply exit = %d: %s", code, stderr)
	}
	got, _ := os.ReadFile(filepath.Join(home, ".claude.json"))
	workspaceJSON, _ := json.Marshal(workspace)
	escapedWorkspace := string(workspaceJSON[1 : len(workspaceJSON)-1])
	for _, want := range []string{`"numStartups": 7`, `"allowedTools": []`, `"keep": {"command": "keep"}`, `"skillhub"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("~/.claude.json is missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(string(got), workspace) && !strings.Contains(string(got), escapedWorkspace) {
		t.Fatalf("~/.claude.json is missing workspace %q:\n%s", workspace, got)
	}
	for _, path := range []string{".claude/CLAUDE.md", ".claude/skills/system-curator/SKILL.md", ".codex/config.toml", ".codex/AGENTS.md", ".agents/skills/system-curator/SKILL.md", ".gemini/settings.json", ".gemini/GEMINI.md", ".gemini/skills/system-curator/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(path))); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}

	code, stdout, _ = runCLI(t, "connect", "-g", "--workspace", workspace, "--yes")
	if code != 0 || !strings.Contains(stdout, "already current") {
		t.Fatalf("idempotent rerun exit = %d stdout = %q", code, stdout)
	}
}

func TestConnectJSONUsesResultEnvelope(t *testing.T) {
	t.Parallel()
	workspace := initTestWorkspace(t)
	code, stdout, stderr := runCLI(t, "connect", "--project", t.TempDir(), "--workspace", workspace, "--json")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	var result app.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != app.ResultSchemaVersion || result.Status != app.StatusActionRequired || len(result.SuggestedActions) != 1 || !strings.Contains(result.SuggestedActions[0].Command, "--yes") {
		t.Fatalf("result = %#v", result)
	}
}

func TestWorkspaceEnvironmentPrecedence(t *testing.T) {
	first := initTestWorkspace(t)
	second := initTestWorkspace(t)
	for _, root := range []string{first, second} {
		if code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "only-in-"+filepath.Base(filepath.Dir(root)), "--collection", "software", "--name", "Env "+filepath.Base(filepath.Dir(root)), "--description", "env precedence", "--trigger", "env", "--not-for", "other", "--min-scope", "single_step", "--yes"); code != 0 {
			t.Fatalf("create exit = %d: %s", code, stderr)
		}
	}
	t.Chdir(t.TempDir())

	t.Setenv(workspaceEnvVar, first)
	code, fromEnv, stderr := runCLI(t, "skill", "list", "--json")
	if code != 0 {
		t.Fatalf("env list exit = %d: %s", code, stderr)
	}
	code, fromFlag, stderr := runCLI(t, "skill", "list", "--workspace", second, "--json")
	if code != 0 {
		t.Fatalf("flag list exit = %d: %s", code, stderr)
	}
	if fromEnv == fromFlag {
		t.Fatal("--workspace did not override SKILLHUB_WORKSPACE")
	}
	if !strings.Contains(fromEnv, filepath.Base(filepath.Dir(first))) || strings.Contains(fromEnv, filepath.Base(filepath.Dir(second))) {
		t.Fatalf("env list served the wrong workspace:\n%s", fromEnv)
	}
	if code, _, stderr := runCLI(t, "status", "--quiet"); code != 0 {
		t.Fatalf("status through env exit = %d: %s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "validate"); code != 0 {
		t.Fatalf("validate through env exit = %d: %s", code, stderr)
	}

	// Discovery from inside a workspace loses to the environment variable.
	t.Chdir(second)
	code, inside, stderr := runCLI(t, "skill", "list", "--json")
	if code != 0 || inside != fromEnv {
		t.Fatalf("env must win over discovery: exit = %d stderr = %q", code, stderr)
	}
	t.Setenv(workspaceEnvVar, "")
	code, inside, stderr = runCLI(t, "skill", "list", "--json")
	if code != 0 || inside != fromFlag {
		t.Fatalf("discovery from the workspace directory failed: exit = %d stderr = %q", code, stderr)
	}
}

func TestSkillListShowsIDStateCollectionNameAndFiltersByState(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	if code, _, stderr := runCLI(t, "skill", "create", "--workspace", root, "--id", "list-me", "--collection", "software", "--name", "List Me", "--description", "listing", "--trigger", "list", "--not-for", "other", "--min-scope", "single_step", "--yes"); code != 0 {
		t.Fatalf("create exit = %d: %s", code, stderr)
	}

	code, stdout, stderr := runCLI(t, "skill", "list", "--workspace", root)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	for _, want := range []string{"ID", "STATE", "COLLECTION", "NAME", "list-me", "draft", "software", "List Me"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("list is missing %q:\n%s", want, stdout)
		}
	}
	if code, stdout, _ := runCLI(t, "skill", "list", "--workspace", root, "--state", "active"); code != 0 || !strings.Contains(stdout, "No skills found.") {
		t.Fatalf("active filter exit = %d stdout = %q", code, stdout)
	}

	code, stdout, stderr = runCLI(t, "skill", "list", "--workspace", root, "--state", "draft", "--json")
	if code != 0 {
		t.Fatalf("json exit = %d: %s", code, stderr)
	}
	var result app.SkillListResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 || result.Skills[0].ID != "list-me" || result.Skills[0].State != "draft" || result.Skills[0].Collection != "software" || result.Skills[0].Name != "List Me" {
		t.Fatalf("json skills = %#v", result.Skills)
	}
	if code, _, stderr := runCLI(t, "skill", "list", "--workspace", root, "--state", "bogus"); code != 2 || !strings.Contains(stderr, "unknown skill state") {
		t.Fatalf("bad state exit = %d stderr = %q", code, stderr)
	}
}

func TestFirstRunStatusRecommendsCommitThenConnect(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, stderr := runCLI(t, "status", "--workspace", root)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if got := strings.Count(stdout, "Next: Commit the new workspace"); got != 1 {
		t.Fatalf("want exactly one recommendation, got %d:\n%s", got, stdout)
	}
	if !strings.Contains(stdout, "Next: Commit the new workspace") || !strings.Contains(stdout, "skillhub connect") || strings.Contains(stdout, "Next: Review uncommitted changes") {
		t.Fatalf("first-run guidance missing:\n%s", stdout)
	}
}
