package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunContext(isolatedProbeContext(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// isolatedProbeContext makes doctor's probes refuse the default listener
// address, so a `skillhub serve` running on the test machine never changes
// a test's output. Probes of test servers on other ports work as usual.
func isolatedProbeContext() context.Context {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "127.0.0.1:7420" {
			return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
		}
		return dialer.DialContext(ctx, network, address)
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return app.WithProbeClient(context.Background(), client)
}

func initTestWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if code, _, stderr := runInitCLI(t, root); code != 0 {
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
	if code != 1 {
		t.Fatalf("doctor exit = %d, want 1 for a problem: %s", code, stderr)
	}
	command := "skillhub doctor --fix --workspace " + root
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
		for _, want := range []string{"Get started", "skillhub connect", "curate my Skill Hub", "Setup:", "Skills:", "Sources & learning:", "Serve:", "Maintenance:", "init", "doctor", "--version", "skillhub help --dev"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("%v help is missing %q:\n%s", args, want, stdout)
			}
		}
		for _, hidden := range []string{"resolve ", "resolution", "telemetry", "eval ", "mcp serve", "mcp proxy", "integrate", "--yes", "\n  check ", "skillhub web"} {
			if strings.Contains(stdout, hidden) {
				t.Fatalf("%v help shows %q:\n%s", args, hidden, stdout)
			}
		}
	}
}

func TestHelpDevListsTheDeveloperCommands(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runCLI(t, "help", "--dev")
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d stderr = %q", code, stderr)
	}
	for _, want := range []string{"resolve", "eval", "resolution", "telemetry"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help --dev is missing %q:\n%s", want, stdout)
		}
	}
}

func TestVersionFlagPrintsTheVersionLine(t *testing.T) {
	t.Parallel()
	code, flag, stderr := runCLI(t, "--version")
	if code != 0 || stderr != "" {
		t.Fatalf("--version exit = %d stderr = %q", code, stderr)
	}
	_, command, _ := runCLI(t, "version")
	if flag != command || !strings.Contains(flag, "skillhub version") {
		t.Fatalf("--version printed %q, version printed %q", flag, command)
	}
}

func TestRemovedCommandsAreNotSupported(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"integrate", "web", "check", "validate", "rebuild", "migrate", "diff"} {
		code, _, stderr := runCLI(t, command)
		if code != 2 || !strings.Contains(stderr, "is not supported") {
			t.Fatalf("%s exit = %d stderr = %q", command, code, stderr)
		}
		if code, _, stderr := runCLI(t, "help", command); code != 2 || !strings.Contains(stderr, "is not supported") {
			t.Fatalf("help %s exit = %d stderr = %q", command, code, stderr)
		}
	}
}

func TestMaintainGroupRunsTheMaintenanceCommands(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	for _, args := range [][]string{
		{"maintain", "validate", "--workspace", root},
		{"maintain", "validate", "--staged", "--workspace", root},
		{"maintain", "rebuild", "--workspace", root},
		{"maintain", "migrate", "--workspace", root},
		{"maintain", "diff", "--workspace", root},
	} {
		if code, stdout, stderr := runCLI(t, args...); code != 0 {
			t.Fatalf("%v exit = %d\n%s%s", args, code, stdout, stderr)
		}
	}
	for _, args := range [][]string{{"maintain"}, {"maintain", "check"}} {
		if code, _, stderr := runCLI(t, args...); code != 2 || !strings.Contains(stderr, "skillhub help maintain") {
			t.Fatalf("%v exit = %d stderr = %q", args, code, stderr)
		}
	}
	for _, args := range [][]string{{"maintain", "--help"}, {"maintain", "rebuild", "--help"}, {"help", "maintain"}} {
		if code, stdout, _ := runCLI(t, args...); code != 0 || !strings.HasPrefix(stdout, "Usage: skillhub maintain validate") {
			t.Fatalf("%v exit = %d stdout = %q", args, code, stdout)
		}
	}
	code, stdout, _ := runCLI(t, "help")
	if code != 0 || !strings.Contains(stdout, "\n  maintain  ") || regexp.MustCompile(`\n  (validate|rebuild|migrate|diff) `).MatchString(stdout) {
		t.Fatalf("global help:\n%s", stdout)
	}
}

func TestOnboardingCommandsRejectYes(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"init", "--yes"},
		{"connect", "--yes"},
		{"connect", "--remove", "--yes"},
		{"doctor", "--fix", "--yes"},
		{"cache", "clear", "--yes"},
		{"token", "rotate", "--token-file", filepath.Join(t.TempDir(), "token"), "--yes"},
	} {
		code, _, stderr := runCLI(t, args...)
		if code != 2 || !strings.Contains(stderr, `unknown argument "--yes"`) {
			t.Fatalf("%v exit = %d stderr = %q", args, code, stderr)
		}
	}
}

func TestCommandHelpPrintsThatCommandsUsage(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"init": "--verbose", "connect": "--global", "doctor": "--fix", "status": "--quiet",
		"skill": "list", "source": "capture", "mcp": "serve",
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
	code, stdout, stderr := runInitCLI(t, root)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	flat := strings.Join(strings.Fields(stdout), " ")
	for _, want := range []string{"Hub created at " + root, "Agent connection written: claude-code, codex-cli, gemini-cli.", "First commit", "Next:", shellquote.Chain("cd <your-project>", "skillhub connect")} {
		if !strings.Contains(flat, want) {
			t.Fatalf("init output is missing %q:\n%s", want, stdout)
		}
	}
	for _, unwanted := range []string{"sha256:", "row(s)", "Workspace directory does not exist", "generation", "git -C", "canonical", "digest"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("init output must not contain %q:\n%s", unwanted, stdout)
		}
	}
}

func TestInitVerboseAndJSONKeepGenerationDigestsAndCounts(t *testing.T) {
	t.Parallel()
	verboseRoot := filepath.Join(t.TempDir(), "workspace")
	code, stdout, stderr := runInitCLI(t, verboseRoot, "--verbose")
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
	code, stdout, stderr = runInitCLI(t, jsonRoot, "--json")
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

func TestInitDryRunDescribesWhatItWouldDo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	code, stdout, stderr := runInitCLI(t, root, "--dry-run")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Would change:") || !strings.Contains(stdout, "First commit of the files init writes") || strings.Contains(stdout, "does not exist") {
		t.Fatalf("dry-run wording:\n%s", stdout)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("init --dry-run wrote into the folder: %v, %v", entries, err)
	}
}

func TestConnectPreviewThenApplyWritesProjectFilesOnly(t *testing.T) {
	workspace := initTestWorkspace(t)
	project := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	code, stdout, stderr := runConnectCLI(t, project, workspace, "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run exit = %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Dry run") || !strings.Contains(stdout, "Would change:") || !strings.Contains(stdout, "$ skillhub connect "+shellquote.Arg(workspace)+"\n") || strings.Contains(stdout, "--yes\n") {
		t.Fatalf("dry-run output:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote files, stat error = %v", err)
	}

	code, stdout, stderr = runConnectCLI(t, project, workspace)
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

	before, _ := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	code, stdout, stderr = runConnectCLI(t, project, workspace)
	if code != 0 || !strings.Contains(stdout, "already current") {
		t.Fatalf("idempotent rerun exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	after, _ := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if !bytes.Equal(before, after) {
		t.Fatal("idempotent rerun changed CLAUDE.md")
	}
}

func TestConnectUsesTheStoredHubAndHonorsHostFilter(t *testing.T) {
	t.Setenv(userconfig.DirEnv, t.TempDir())
	workspace := initTestWorkspace(t)
	project := t.TempDir()

	code, _, stderr := runConnectCLI(t, project, "--host", "codex")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); err != nil {
		t.Fatalf("codex files missing: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(project, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := hostintegration.ReadRegistration(hostintegration.HostCodex, raw).Workspace; got != workspace {
		t.Fatalf("connect used hub %q, want the stored hub %q", got, workspace)
	}
	for _, path := range []string{"CLAUDE.md", "GEMINI.md", ".mcp.json"} {
		if _, err := os.Stat(filepath.Join(project, path)); !os.IsNotExist(err) {
			t.Fatalf("%s should not be written for --host codex, stat error = %v", path, err)
		}
	}
}

func TestConnectWithoutAStoredHubPrintsTheInitCommandAndWritesNothing(t *testing.T) {
	config := t.TempDir()
	t.Setenv(userconfig.DirEnv, config)
	project := t.TempDir()
	code, stdout, stderr := runConnectCLI(t, project)
	if flat := strings.Join(strings.Fields(stderr), " "); code != 2 || !strings.Contains(flat, "No hub is set up on this machine.") || !strings.Contains(flat, app.NewHubCommand()) {
		t.Fatalf("exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	for _, command := range regexp.MustCompile("`([^`]*skillhub [^`]*)`").FindAllStringSubmatch(stderr, -1) {
		for _, segment := range commandSegments(command[1]) {
			checkGuideCommand(t, 0, segment)
		}
	}
	if entries, err := os.ReadDir(project); err != nil || len(entries) != 0 {
		t.Fatalf("connect without a hub wrote into the project: %v, %v", entries, err)
	}

	gone := filepath.Join(t.TempDir(), "moved-hub")
	if err := userconfig.NewStore(config).SetHub(gone); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runConnectCLI(t, project)
	if flat := strings.Join(strings.Fields(stderr), " "); code != 2 || !strings.Contains(flat, "The hub stored at "+gone+" is gone.") || !strings.Contains(flat, "skillhub init") {
		t.Fatalf("stale stored hub exit = %d stderr = %q", code, stderr)
	}
	if entries, err := os.ReadDir(project); err != nil || len(entries) != 0 {
		t.Fatalf("connect with a missing hub wrote into the project: %v, %v", entries, err)
	}
}

func TestConnectInTheHubOnlyRefreshesItsOwnConnection(t *testing.T) {
	t.Parallel()
	workspace := initTestWorkspace(t)
	code, stdout, stderr := runConnectCLI(t, workspace)
	if code != 0 || !strings.Contains(stdout, "This is the hub; `skillhub init` already connected its agents") {
		t.Fatalf("exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
}

func TestConnectRejectsUninitializedWorkspaceAndBadFlags(t *testing.T) {
	t.Parallel()
	empty := t.TempDir()
	if code, _, stderr := runConnectCLI(t, t.TempDir(), empty); code != 2 || !strings.Contains(stderr, "skillhub init") {
		t.Fatalf("uninitialized workspace exit = %d stderr = %q", code, stderr)
	}
	for _, flag := range []string{"--project", "--workspace"} {
		if code, _, stderr := runConnectCLI(t, t.TempDir(), flag, empty); code != 2 || !strings.Contains(stderr, "unknown argument") {
			t.Fatalf("connect %s exit = %d stderr = %q", flag, code, stderr)
		}
	}
	if code, _, stderr := runConnectCLI(t, t.TempDir(), empty, empty); code != 2 || !strings.Contains(stderr, "at most one hub folder") {
		t.Fatalf("two hub folders exit = %d stderr = %q", code, stderr)
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
	code, _, stderr := runConnectCLI(t, project, workspace)
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

	code, stdout, stderr := runCLI(t, "connect", "-g", workspace, "--dry-run")
	if code != 0 || !strings.Contains(stdout, "Dry run") {
		t.Fatalf("preview exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".claude.json")); string(got) != existing {
		t.Fatal("global preview modified ~/.claude.json")
	}

	code, _, stderr = runCLI(t, "connect", "--global", workspace)
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

	code, stdout, _ = runCLI(t, "connect", "-g", workspace)
	if code != 0 || !strings.Contains(stdout, "already current") {
		t.Fatalf("idempotent rerun exit = %d stdout = %q", code, stdout)
	}
}

func TestConnectJSONUsesResultEnvelope(t *testing.T) {
	t.Parallel()
	workspace := initTestWorkspace(t)
	code, stdout, stderr := runConnectCLI(t, t.TempDir(), workspace, "--json", "--dry-run")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	var result app.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != app.ResultSchemaVersion || result.Status != app.StatusActionRequired || len(result.SuggestedActions) != 1 || !strings.HasPrefix(result.SuggestedActions[0].Command, "skillhub connect ") || strings.Contains(result.SuggestedActions[0].Command, "--dry-run") || strings.Contains(result.SuggestedActions[0].Command, "--yes") {
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
	if code, _, stderr := runCLI(t, "maintain", "validate"); code != 0 {
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

func TestFirstRunStatusAsksForNoCommit(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	code, stdout, stderr := runCLI(t, "status", "--workspace", root)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "Commit the new workspace") || strings.Contains(stdout, "Review uncommitted changes") || strings.Contains(stdout, "git -C") {
		t.Fatalf("init commits the hub, so status must not ask for a commit:\n%s", stdout)
	}
}

// TestDryRunWritesNothing runs every onboarding command with --dry-run and
// checks that the hub, the project, the user's home and the skill cache are
// byte for byte the same afterwards.
func TestDryRunWritesNothing(t *testing.T) {
	home, state := isolateClient(t)
	workspace := initTestWorkspace(t)
	project := t.TempDir()
	if code, _, stderr := runConnectCLI(t, project, workspace, "--host", "claude"); code != 0 {
		t.Fatalf("connect exit = %d: %s", code, stderr)
	}
	// Give doctor --fix something to repair.
	if err := os.Remove(filepath.Join(workspace, ".claude", "skills", "system-curator", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	cacheSkill(t, state, "http://127.0.0.1:7420", "alpha", "cached skill")
	snapshot := func() map[string]map[string]string {
		return map[string]map[string]string{"workspace": treeOf(t, workspace), "project": treeOf(t, project), "home": treeOf(t, home), "state": treeOf(t, state)}
	}
	before := snapshot()
	newHub := t.TempDir()
	for _, args := range [][]string{
		{"init", "--dry-run"},
		{"connect", workspace, "--dry-run"},
		{"connect", "-g", workspace, "--dry-run"},
		{"connect", "--remove", "--dry-run"},
		{"doctor", "--fix", "--workspace", workspace, "--dry-run"},
		{"cache", "clear", "--dry-run"},
		{"token", "rotate", "--workspace", workspace, "--dry-run"},
	} {
		var code int
		var stdout, stderr string
		switch args[0] {
		case "init":
			code, stdout, stderr = runInitCLI(t, newHub, args[1:]...)
			if entries, err := os.ReadDir(newHub); err != nil || len(entries) != 0 {
				t.Fatalf("init --dry-run wrote into the folder: %v, %v", entries, err)
			}
		case "connect":
			code, stdout, stderr = runConnectCLI(t, project, args[1:]...)
		default:
			code, stdout, stderr = runCLI(t, args...)
		}
		if code != 0 {
			t.Fatalf("%v exit = %d stdout = %q stderr = %q", args, code, stdout, stderr)
		}
		if after := snapshot(); !reflect.DeepEqual(before, after) {
			t.Fatalf("%v changed files:\nbefore: %v\nafter:  %v", args, before, after)
		}
	}
}
