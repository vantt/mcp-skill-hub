package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/userconfig"
)

// doctorOnly are the checks only doctor runs; status leaves them out.
var doctorOnly = []string{"hub_precommit_validate", "hub_commit_index_leftover", "hub_listener", "remote_version", "remote_cache", "remote_last_fetch"}

func runStatusCLI(ctx context.Context, dir string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := runStatusIn(ctx, dir, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func runDoctorCLI(ctx context.Context, dir string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := runDoctorIn(ctx, dir, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func problemCodes(sections []app.Section) []string {
	var codes []string
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == app.SeverityProblem && !slices.Contains(codes, finding.Code) {
				codes = append(codes, finding.Code)
			}
		}
	}
	slices.Sort(codes)
	return codes
}

// nextCommand returns the command line after "Next:" in status output.
func nextCommand(t *testing.T, output string) string {
	t.Helper()
	lines := strings.Split(output, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "Next: ") {
			for _, after := range lines[index+1:] {
				if strings.HasPrefix(after, "  $ ") {
					return strings.TrimPrefix(after, "  $ ")
				}
			}
		}
	}
	t.Fatalf("no Next command in:\n%s", output)
	return ""
}

// healthMachine isolates the home folder and the user configuration of one
// fixture, as for a fresh machine.
func healthMachine(t *testing.T) (home, config string) {
	t.Helper()
	home, config = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(userconfig.DirEnv, config)
	t.Setenv(workspaceEnvVar, "")
	t.Setenv(hubTokenEnv, "")
	return home, config
}

func closedLoopbackURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + address
}

func anyTokenFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub-token")
	if err := os.WriteFile(path, []byte(strings.Repeat("ab", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustConnect(t *testing.T, project string, args ...string) {
	t.Helper()
	if code, stdout, stderr := runConnectCLI(t, project, args...); code != 0 {
		t.Fatalf("connect %v: exit %d\n%s%s", args, code, stdout, stderr)
	}
}

func moveHub(t *testing.T, from string) string {
	t.Helper()
	to := filepath.Join(t.TempDir(), "moved-hub")
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	return to
}

type roleFixture struct {
	name string
	// build returns the folder status and doctor run in.
	build    func(t *testing.T) string
	role     string
	problems []string
	// afterFix are the problems doctor --fix leaves; nil skips --fix.
	afterFix []string
	// fixOutput must appear in the doctor --fix output.
	fixOutput func(dir string) string
	// verify checks the files after doctor --fix.
	verify func(t *testing.T, dir string)
}

func roleFixtures() []roleFixture {
	return []roleFixture{
		{name: "hub fresh", role: app.RoleHub, build: func(t *testing.T) string {
			return initHub(t, filepath.Join(t.TempDir(), "hub"))
		}},
		{name: "hub dirty", role: app.RoleHub, build: func(t *testing.T) string {
			hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
			createDraftSkill(t, hub, "demo", "--not-for", "unrelated")
			return hub
		}},
		{name: "hub stale index", role: app.RoleHub, build: func(t *testing.T) string {
			hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
			createDraftSkill(t, hub, "demo", "--not-for", "unrelated")
			appendToFile(t, filepath.Join(hub, "skills", "software", "demo", "SKILL.md"), "\nhand edit\n")
			return hub
		}},
		{name: "hub missing gitignore entries", role: app.RoleHub, problems: []string{"gitignore_entry_missing"}, afterFix: []string{}, build: func(t *testing.T) string {
			hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
			replaceInFile(t, filepath.Join(hub, ".gitignore"), "/runtime/\n", "")
			return hub
		}},
		{name: "hub tracking a connection file", role: app.RoleHub, problems: []string{"hub_self_connection", "machine_file_tracked"}, afterFix: []string{"machine_file_tracked"},
			fixOutput: func(dir string) string { return "git -C " + shellquote.Arg(dir) + " rm --cached -- .mcp.json" },
			build: func(t *testing.T) string {
				hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
				gitOutput(t, hub, "add", "-f", ".mcp.json")
				gitOutput(t, hub, "-c", "user.name=T", "-c", "user.email=t@skillhub.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "track")
				if err := os.Remove(filepath.Join(hub, ".codex", "config.toml")); err != nil {
					t.Fatal(err)
				}
				return hub
			}},
		{name: "hub with a validate pre-commit hook", role: app.RoleHub, problems: []string{"hub_precommit_validate"}, afterFix: []string{}, build: func(t *testing.T) string {
			hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
			hook := filepath.Join(hub, ".git", "hooks", "pre-commit")
			if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nskillhub validate --staged\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			return hub
		}},
		{name: "hub whose folder is connected to another hub", role: app.RoleBoth, problems: []string{"gitignore_entry_missing"}, afterFix: []string{},
			verify: func(t *testing.T, dir string) {
				content, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
				if workspace := hostintegration.ReadRegistration(hostintegration.HostClaude, content).Workspace; err != nil || filepath.Base(workspace) != "other" {
					t.Fatalf("doctor --fix re-pointed the hub folder's Claude Code entry to %q: %v", workspace, err)
				}
			},
			build: func(t *testing.T) string {
				other := initHub(t, filepath.Join(t.TempDir(), "other"))
				hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
				mustConnect(t, hub, other, "--host", "claude")
				// Give doctor --fix a hub repair to run; it must leave the
				// Claude Code entry that now points at the other hub alone.
				replaceInFile(t, filepath.Join(hub, ".gitignore"), "/runtime/\n", "")
				return hub
			}},
		{name: "client-local healthy", role: app.RoleClientLocal, build: func(t *testing.T) string {
			hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
			project := t.TempDir()
			mustConnect(t, project, "--host", "claude")
			_ = hub
			return project
		}},
		{name: "client-local with a moved hub this machine recorded", role: app.RoleClientLocal, problems: []string{"client_hub_missing"}, afterFix: []string{}, build: func(t *testing.T) string {
			hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
			project := t.TempDir()
			mustConnect(t, project, "--host", "claude")
			initHub(t, moveHub(t, hub))
			return project
		}},
		{name: "client-local with a moved hub this machine never recorded", role: app.RoleClientLocal, problems: []string{"client_hub_missing"}, afterFix: []string{"client_hub_missing"},
			fixOutput: func(string) string { return "skillhub connect " },
			build: func(t *testing.T) string {
				hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
				project := t.TempDir()
				mustConnect(t, project, "--host", "claude")
				moved := moveHub(t, hub)
				fresh := t.TempDir()
				t.Setenv(userconfig.DirEnv, fresh)
				if err := userconfig.NewStore(fresh).SetHub(moved); err != nil {
					t.Fatal(err)
				}
				return project
			}},
		{name: "client-local whose stored hub moved", role: app.RoleClientLocal, problems: []string{"client_hub_missing"}, afterFix: []string{"client_hub_missing"},
			fixOutput: func(string) string { return shellquote.Chain("cd <new-location>", "skillhub init") },
			build: func(t *testing.T) string {
				hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
				project := t.TempDir()
				mustConnect(t, project, "--host", "claude")
				moveHub(t, hub)
				return project
			}},
		{name: "client-local with an old permission rule set", role: app.RoleClientLocal, problems: []string{"client_permissions_outdated"}, afterFix: []string{}, build: func(t *testing.T) string {
			initHub(t, filepath.Join(t.TempDir(), "hub"))
			project := t.TempDir()
			mustConnect(t, project, "--host", "claude")
			replaceInFile(t, filepath.Join(project, ".claude", "settings.local.json"), `"Bash(skillhub serve*)"`, `"Bash(make test)"`)
			return project
		}},
		{name: "client-remote down", role: app.RoleClientRemote, problems: []string{"remote_reachable"}, afterFix: []string{"remote_reachable"},
			fixOutput: func(string) string { return "Skill Hub cannot start a hub on another machine." },
			build: func(t *testing.T) string {
				project := t.TempDir()
				mustConnect(t, project, "--hub", closedLoopbackURL(t), "--token-file", anyTokenFile(t), "--host", "claude")
				return project
			}},
		{name: "client-remote in a subfolder", role: app.RoleClientRemote, problems: []string{"remote_reachable"}, build: func(t *testing.T) string {
			project := t.TempDir()
			mustConnect(t, project, "--hub", closedLoopbackURL(t), "--token-file", anyTokenFile(t), "--host", "claude")
			sub := filepath.Join(project, "src", "app")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			return sub
		}},
		{name: "mixed project", role: app.RoleClientLocal, problems: []string{"remote_reachable"}, build: func(t *testing.T) string {
			initHub(t, filepath.Join(t.TempDir(), "hub"))
			project := t.TempDir()
			mustConnect(t, project, "--host", "claude")
			mustConnect(t, project, "--hub", closedLoopbackURL(t), "--token-file", anyTokenFile(t), "--host", "codex")
			return project
		}},
		{name: "user scope only", role: app.RoleClientLocal, build: func(t *testing.T) string {
			initHub(t, filepath.Join(t.TempDir(), "hub"))
			mustConnect(t, t.TempDir(), "-g")
			return t.TempDir()
		}},
		{name: "none with a stored hub", role: app.RoleNone, build: func(t *testing.T) string {
			initHub(t, filepath.Join(t.TempDir(), "hub"))
			return t.TempDir()
		}},
		{name: "none without a stored hub", role: app.RoleNone, build: func(t *testing.T) string {
			return t.TempDir()
		}},
	}
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(content), old, replacement, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestStatusAndDoctorAgreeInEveryRole runs status and doctor in real role
// fixtures: both report the same problems, status always ends with a command
// whose skillhub parts the help knows, `status --json` keeps its schema, and
// doctor's exit code says whether a problem remains.
func TestStatusAndDoctorAgreeInEveryRole(t *testing.T) {
	ctx := isolatedProbeContext()
	for _, fixture := range roleFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			healthMachine(t)
			dir := fixture.build(t)
			report, resErr := healthReport(dir, "")
			if resErr != nil {
				t.Fatal(resErr)
			}
			if report.HealthRole() != fixture.role {
				t.Fatalf("role = %s, want %s", report.HealthRole(), fixture.role)
			}
			light, full := app.Run(ctx, report, app.DepthLight), app.Run(ctx, report, app.DepthFull)
			var fullProblems []string
			for _, code := range problemCodes(full) {
				if !slices.Contains(doctorOnly, code) {
					fullProblems = append(fullProblems, code)
				}
			}
			if !slices.Equal(problemCodes(light), fullProblems) {
				t.Fatalf("status problems %v, doctor problems %v", problemCodes(light), problemCodes(full))
			}
			want := slices.Clone(fixture.problems)
			slices.Sort(want)
			if got := problemCodes(full); !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("doctor problems %v, want %v\n%+v", got, want, full)
			}

			code, stdout, stderr := runStatusCLI(ctx, dir)
			output := stdout
			wantStatus := 0
			if fixture.role == app.RoleNone {
				output, wantStatus = stderr, 2
				if !strings.HasPrefix(stderr, noRoleMessage+"\n") {
					t.Fatalf("status in a folder without a role:\n%s", stderr)
				}
			}
			if code != wantStatus {
				t.Fatalf("status exit %d, want %d\n%s%s", code, wantStatus, stdout, stderr)
			}
			next := nextCommand(t, output)
			for _, segment := range commandSegments(next) {
				checkGuideCommand(t, 0, segment)
			}
			if strings.Contains(next, "--json") || strings.Contains(next, "workspace_") {
				t.Fatalf("next is not a command a person runs: %q", next)
			}

			code, stdout, _ = runStatusCLI(ctx, dir, "--json")
			if code != wantStatus {
				t.Fatalf("status --json exit %d", code)
			}
			validateJSONDocument(t, filepath.Join("..", "..", "..", "schemas", "curation-home-v1.schema.json"), []byte(stdout))
			var document statusJSON
			if err := json.Unmarshal([]byte(stdout), &document); err != nil || document.Role != fixture.role || document.SuggestedActions[0].Command != next {
				t.Fatalf("status --json role %q action %+v, want %q and %q: %v", document.Role, document.SuggestedActions, fixture.role, next, err)
			}

			code, stdout, stderr = runDoctorCLI(ctx, dir)
			wantDoctor := 0
			switch {
			case fixture.role == app.RoleNone:
				wantDoctor = 2
			case len(want) > 0:
				wantDoctor = 1
			}
			if code != wantDoctor {
				t.Fatalf("doctor exit %d, want %d\n%s%s", code, wantDoctor, stdout, stderr)
			}
			if wantDoctor == 0 && !strings.HasSuffix(stdout, "\nHealthy.\n") {
				t.Fatalf("healthy doctor output:\n%s", stdout)
			}

			if fixture.afterFix == nil {
				return
			}
			code, stdout, stderr = runDoctorCLI(ctx, dir, "--fix")
			if wantFix := min(len(fixture.afterFix), 1); code != wantFix {
				t.Fatalf("doctor --fix exit %d, want %d\n%s%s", code, wantFix, stdout, stderr)
			}
			if fixture.fixOutput != nil && !strings.Contains(stdout, fixture.fixOutput(dir)) {
				t.Fatalf("doctor --fix output lacks %q:\n%s", fixture.fixOutput(dir), stdout)
			}
			if fixture.verify != nil {
				fixture.verify(t, dir)
			}
			report, resErr = healthReport(dir, "")
			if resErr != nil {
				t.Fatal(resErr)
			}
			after := problemCodes(app.Run(ctx, report, app.DepthFull))
			wantAfter := slices.Clone(fixture.afterFix)
			slices.Sort(wantAfter)
			if !slices.Equal(after, wantAfter) && (len(after) != 0 || len(wantAfter) != 0) {
				t.Fatalf("problems after doctor --fix %v, want %v\n%s", after, wantAfter, stdout)
			}
		})
	}
}

func TestDoctorFixKeepsTheUsersOwnPermissionRules(t *testing.T) {
	healthMachine(t)
	initHub(t, filepath.Join(t.TempDir(), "hub"))
	project := t.TempDir()
	mustConnect(t, project, "--host", "claude")
	settings := filepath.Join(project, ".claude", "settings.local.json")
	replaceInFile(t, settings, `"Bash(skillhub serve*)"`, `"Bash(make test)"`)
	if code, stdout, stderr := runDoctorCLI(isolatedProbeContext(), project, "--fix"); code != 0 || !strings.Contains(stdout, "Fixed:") || !strings.Contains(stdout, settings) {
		t.Fatalf("doctor --fix exit %d\n%s%s", code, stdout, stderr)
	}
	content, err := os.ReadFile(settings)
	if err != nil || !strings.Contains(string(content), `"Bash(skillhub serve*)"`) || !strings.Contains(string(content), `"Bash(make test)"`) {
		t.Fatalf("settings after the fix: %s %v", content, err)
	}
}

// tokenRecorder is a test hub that records every Authorization header.
type tokenRecorder struct {
	mutex  sync.Mutex
	calls  int
	tokens []string
}

func (recorder *tokenRecorder) seen() (int, []string) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return recorder.calls, slices.Clone(recorder.tokens)
}

func TestStatusAndDoctorNeverSendTheTokenToAHubOnlyTheProjectNames(t *testing.T) {
	healthMachine(t)
	recorder := &tokenRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recorder.mutex.Lock()
		recorder.calls++
		if value := request.Header.Get("Authorization"); value != "" {
			recorder.tokens = append(recorder.tokens, value)
		}
		recorder.mutex.Unlock()
		writer.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	ctx := app.WithProbeClient(context.Background(), &http.Client{Transport: transport})

	const hubURL = "http://192.0.2.10:7420"
	tokenFile := anyTokenFile(t)
	project := t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{"type": "stdio", "command": binary, "args": []string{"mcp", "proxy", "--hub", hubURL, "--token-file", tokenFile}}}})
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), entry, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runStatusCLI(ctx, project); code != 0 || !strings.Contains(stdout, "skillhub connect --hub "+hubURL) {
		t.Fatalf("status exit %d\n%s%s", code, stdout, stderr)
	}
	if code, _, stderr := runDoctorCLI(ctx, project); code != 1 {
		t.Fatalf("doctor exit %d: %s", code, stderr)
	}
	code, stdout, stderr := runDoctorCLI(ctx, project, "--fix")
	if code != 1 || !strings.Contains(flatText(stdout), flatText(app.RemoteFixRefusal(hubURL, tokenFile))) {
		t.Fatalf("doctor --fix exit %d\n%s%s", code, stdout, stderr)
	}
	if calls, tokens := recorder.seen(); calls == 0 || len(tokens) != 0 {
		t.Fatalf("hub calls %d, Authorization headers %q", calls, tokens)
	}
	if strings.Contains(stdout, strings.Repeat("ab", 32)) {
		t.Fatal("doctor printed the token")
	}
}

func TestStatusAnswersAtOnceWhileTheHubIsBusy(t *testing.T) {
	healthMachine(t)
	hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		lock, err := mutation.AcquireExclusiveLock(context.Background(), hub, time.Second)
		if err != nil {
			t.Error(err)
			close(held)
			return
		}
		close(held)
		<-release
		_ = lock.Unlock()
	}()
	<-held
	defer close(release)
	started := time.Now()
	code, stdout, stderr := runStatusCLI(isolatedProbeContext(), hub)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("status took %s while the hub was busy", elapsed)
	}
	if code != 0 || !strings.Contains(stdout, "The hub is busy with another change") || nextCommand(t, stdout) != "skillhub status" {
		t.Fatalf("status while busy: exit %d\n%s%s", code, stdout, stderr)
	}
}

func TestDoctorJSONHasTheCombinedShape(t *testing.T) {
	healthMachine(t)
	hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
	replaceInFile(t, filepath.Join(hub, ".gitignore"), "/runtime/\n", "")
	code, stdout, _ := runDoctorCLI(isolatedProbeContext(), hub, "--json")
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &document); err != nil || code != 1 {
		t.Fatalf("doctor --json exit %d: %s %v", code, stdout, err)
	}
	keys := make([]string, 0, len(document))
	for key := range document {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"fixed", "not_fixed", "role", "sections"}) {
		t.Fatalf("doctor --json keys = %v", keys)
	}
	code, stdout, _ = runDoctorCLI(isolatedProbeContext(), hub, "--fix", "--json")
	var fixed doctorJSON
	if err := json.Unmarshal([]byte(stdout), &fixed); err != nil || code != 0 || len(fixed.Fixed) != 1 || !slices.Contains(fixed.Fixed[0].Codes, "gitignore_entry_missing") || !slices.Contains(fixed.Fixed[0].Changed, filepath.Join(hub, ".gitignore")) {
		t.Fatalf("doctor --fix --json exit %d: %s %v", code, stdout, err)
	}
}

func TestStatusNextCommandRepeatsAnExplicitHub(t *testing.T) {
	healthMachine(t)
	hub := initHub(t, filepath.Join(t.TempDir(), "hub"))
	code, stdout, _ := runStatusCLI(isolatedProbeContext(), t.TempDir(), "--workspace", hub)
	if code != 0 || !strings.HasSuffix(nextCommand(t, stdout), " --workspace "+shellquote.Arg(hub)) {
		t.Fatalf("status --workspace exit %d:\n%s", code, stdout)
	}
}
