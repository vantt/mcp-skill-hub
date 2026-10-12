package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

func healthHub(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "hub")
	if _, err := (WorkspaceService{}).Init(root, InitOptions{Store: userconfig.NewStore(t.TempDir())}); err != nil {
		t.Fatal(err)
	}
	return root
}

// codesOf lists the codes of the findings with the given severity, in order.
func codesOf(sections []Section, severity Severity) []string {
	var codes []string
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == severity {
				codes = append(codes, finding.Code)
			}
		}
	}
	return codes
}

func findingByCode(t *testing.T, sections []Section, code string) Finding {
	t.Helper()
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Code == code && finding.Severity != SeverityOK {
				return finding
			}
		}
	}
	t.Fatalf("no finding %s in %+v", code, sections)
	return Finding{}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Skill Hub Test", "-c", "user.email=test@skillhub.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestFreshHubIsHealthyAtBothDepths(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	report := RoleReport{Cwd: root}.WithWorkspace(root)
	light := Run(t.Context(), report, DepthLight)
	full := Run(t.Context(), report, DepthFull)
	if len(light) != 1 || light[0].Kind != SectionHub || light[0].Root != root || light[0].Title != "Hub at "+root {
		t.Fatalf("sections = %+v", light)
	}
	if problems := append(codesOf(light, SeverityProblem), codesOf(full, SeverityProblem)...); len(problems) != 0 {
		t.Fatalf("a fresh hub has problems: %v\nlight: %+v\nfull: %+v", problems, light, full)
	}
	if slices.Contains(codesOf(light, SeverityOK), "hub_precommit_validate") || !slices.Contains(codesOf(full, SeverityOK), "hub_precommit_validate") {
		t.Fatalf("the pre-commit hook is a doctor-only check: light %v full %v", codesOf(light, SeverityOK), codesOf(full, SeverityOK))
	}
	next := StatusNext(report, light, HubInventory{Known: true})
	if next.Command != "skillhub skill create --workspace "+shellquote.Arg(root) {
		t.Fatalf("next for an empty hub = %+v", next)
	}
	if next := StatusNext(report, light, HubInventory{Known: true, Skills: 2}); next.Command != "skillhub skill list --workspace "+shellquote.Arg(root) {
		t.Fatalf("next for a hub with skills = %+v", next)
	}
}

func TestStatusChecksNeverWaitForABusyHub(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	lock, err := mutation.AcquireExclusiveLock(t.Context(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	report := RoleReport{Cwd: root, Hub: &HubRef{Root: root}}
	started := time.Now()
	sections := Run(t.Context(), report, DepthLight)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("status waited %s for the lock", elapsed)
	}
	busy := findingByCode(t, sections, "hub_busy")
	if busy.Severity != SeverityNotice || !strings.Contains(busy.Message, "run `skillhub status` again") {
		t.Fatalf("busy finding = %+v", busy)
	}
	if next := StatusNext(report, sections, HubInventory{}); next.Command != "skillhub status" {
		t.Fatalf("next while busy = %+v", next)
	}
}

func TestDoctorFixRepairsTheHubAndLeavesATrackedConnectionFileToThePerson(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	gitIn(t, root, "add", "-f", ".mcp.json")
	gitIn(t, root, "commit", "-q", "-m", "track a machine file")
	ignore := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore, []byte(strings.Replace(string(content), "/.codex/\n", "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(root, ".claude", "skills", "system-curator", "SKILL.md")
	if err := os.Remove(skill); err != nil {
		t.Fatal(err)
	}
	report := RoleReport{Cwd: root, Hub: &HubRef{Root: root}}
	before := Run(t.Context(), report, DepthFull)
	if got, want := codesOf(before, SeverityProblem), []string{"machine_file_tracked", "gitignore_entry_missing", "hub_index_stale"}; !slices.Equal(got, want) {
		t.Fatalf("problems = %v, want %v\n%+v", got, want, before)
	}
	tracked := findingByCode(t, before, "machine_file_tracked")
	if tracked.Fix != nil || tracked.Manual != "git -C "+shellquote.Arg(root)+" rm --cached -- .mcp.json" {
		t.Fatalf("tracked file finding = %+v", tracked)
	}
	light := Run(t.Context(), report, DepthLight)
	if !slices.Equal(codesOf(light, SeverityProblem), codesOf(before, SeverityProblem)) {
		t.Fatalf("status and doctor disagree: %v vs %v", codesOf(light, SeverityProblem), codesOf(before, SeverityProblem))
	}
	if next := StatusNext(report, light, HubInventory{Known: true}); next.Command != "skillhub doctor --fix" {
		t.Fatalf("next = %+v", next)
	}

	fixes := ApplyFixes(t.Context(), before)
	// The one hub repair also restores the agent connection file, which the
	// checks reach only once the layout is in order.
	if len(fixes.Fixed) != 1 || !slices.Equal(fixes.Fixed[0].Codes, []string{"gitignore_entry_missing", "hub_index_stale"}) {
		t.Fatalf("fixed = %+v", fixes.Fixed)
	}
	if !slices.Contains(fixes.Fixed[0].Changed, ignore) || !slices.Contains(fixes.Fixed[0].Changed, skill) {
		t.Fatalf("changed files = %v", fixes.Fixed[0].Changed)
	}
	if len(fixes.NotFixed) != 1 || fixes.NotFixed[0].Code != "machine_file_tracked" || fixes.NotFixed[0].Manual != tracked.Manual || !strings.Contains(fixes.NotFixed[0].Reason, "never changes what Git tracks") {
		t.Fatalf("not fixed = %+v", fixes.NotFixed)
	}
	after := Run(t.Context(), report, DepthFull)
	if got := codesOf(after, SeverityProblem); !slices.Equal(got, []string{"machine_file_tracked"}) {
		t.Fatalf("problems after the fix = %v", got)
	}
	if files := gitIn(t, root, "ls-files", "--", ".mcp.json"); strings.TrimSpace(files) != ".mcp.json" {
		t.Fatalf("doctor --fix changed what Git tracks: %q", files)
	}
}

func TestPrecommitHookRewriteChangesOnlyTheValidateCall(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	original := "#!/bin/sh\nset -e\nskillhub validate --staged\n/usr/local/bin/skillhub  validate --staged || exit 1\nskillhub maintain validate\nmyskillhub validate\necho done\n"
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}
	report := RoleReport{Cwd: root, Hub: &HubRef{Root: root}}
	if slices.Contains(codesOf(Run(t.Context(), report, DepthLight), SeverityProblem), "hub_precommit_validate") {
		t.Fatal("status reads the pre-commit hook; only doctor should")
	}
	sections := Run(t.Context(), report, DepthFull)
	finding := findingByCode(t, sections, "hub_precommit_validate")
	if finding.Severity != SeverityProblem || finding.Fix == nil {
		t.Fatalf("hook finding = %+v", finding)
	}
	fixes := ApplyFixes(t.Context(), sections)
	if len(fixes.Fixed) != 1 || !slices.Equal(fixes.Fixed[0].Changed, Changed{hook}) {
		t.Fatalf("fixes = %+v", fixes)
	}
	got, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	want := "#!/bin/sh\nset -e\nskillhub maintain validate --staged\n/usr/local/bin/skillhub  maintain validate --staged || exit 1\nskillhub maintain validate\nmyskillhub validate\necho done\n"
	if string(got) != want {
		t.Fatalf("hook after the fix:\n%s\nwant:\n%s", got, want)
	}
	if info, err := os.Stat(hook); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o755) {
		t.Fatalf("hook mode = %v, %v", info.Mode(), err)
	}
	if after := findingByCodeOrOK(Run(t.Context(), report, DepthFull), "hub_precommit_validate"); after.Severity != SeverityOK {
		t.Fatalf("hook after the fix = %+v", after)
	}
}

func findingByCodeOrOK(sections []Section, code string) Finding {
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Code == code {
				return finding
			}
		}
	}
	return Finding{}
}

// recordingHub is a test hub that records the Authorization header of every
// request. It answers like a Skill Hub listener: 401 with the skillhub realm
// without the token, the status body with it.
type recordingHub struct {
	server *httptest.Server
	token  string
	mutex  sync.Mutex
	auth   []string
	calls  int
}

func newRecordingHub(t *testing.T, token string) *recordingHub {
	t.Helper()
	hub := &recordingHub{token: token}
	hub.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hub.mutex.Lock()
		hub.calls++
		if value := request.Header.Get("Authorization"); value != "" {
			hub.auth = append(hub.auth, value)
		}
		hub.mutex.Unlock()
		if request.Header.Get("Authorization") != "Bearer "+token {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(writer).Encode(CurrentHubStatus())
	}))
	t.Cleanup(hub.server.Close)
	return hub
}

// client dials the test hub for every address, so a hub URL with a
// documentation address (not loopback) reaches it.
func (hub *recordingHub) client() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	address := hub.server.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (hub *recordingHub) seen() (calls int, auth []string) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	return hub.calls, slices.Clone(hub.auth)
}

func writeTokenFile(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub-token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func proxyEntryJSON(t *testing.T, hubURL, tokenFile string) string {
	t.Helper()
	binary, err := currentBinary()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{"type": "stdio", "command": binary, "args": []string{"mcp", "proxy", "--hub", hubURL, "--token-file", tokenFile}}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestTheHubTokenIsNeverSentToAHubNamedOnlyInProjectFiles(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("c4", 32)
	hub := newRecordingHub(t, token)
	const hubURL = "http://192.0.2.10:7420"
	tokenFile := writeTokenFile(t, token)
	project, home := t.TempDir(), t.TempDir()
	// A cloned project names the hub; its files are incomplete, so the
	// connection also needs a reconnect that doctor --fix must not run.
	writeRoleFile(t, filepath.Join(project, ".mcp.json"), proxyEntryJSON(t, hubURL, tokenFile))
	report := DetectRole(project, home, userconfig.Config{})
	if report.Kind() != RoleClientRemote {
		t.Fatalf("role = %s", report.Kind())
	}
	ctx := WithProbeClient(t.Context(), hub.client())
	light := Run(ctx, report, DepthLight)
	full := Run(ctx, report, DepthFull)
	fixes := ApplyFixes(ctx, full)
	if calls, auth := hub.seen(); calls == 0 || len(auth) != 0 {
		t.Fatalf("hub calls %d, Authorization headers %q: the token went to a hub only the project names", calls, auth)
	}
	reconnect := findingByCode(t, full, "remote_reconnect")
	if reconnect.Severity != SeverityProblem || reconnect.Fix != nil || reconnect.Manual != "skillhub connect --hub "+hubURL+" --token-file "+shellquote.Arg(tokenFile) {
		t.Fatalf("reconnect finding = %+v", reconnect)
	}
	if !slices.ContainsFunc(fixes.NotFixed, func(item NotFixedItem) bool {
		return item.Code == "remote_reconnect" && item.Reason == RemoteFixRefusal(hubURL, tokenFile)
	}) || len(fixes.Fixed) != 0 {
		t.Fatalf("doctor --fix did not refuse the reconnect: %+v", fixes)
	}
	if !slices.Equal(codesOf(light, SeverityProblem), []string{"remote_reconnect"}) || !slices.Equal(codesOf(full, SeverityProblem), []string{"remote_reconnect"}) {
		t.Fatalf("problems: light %v, full %v", codesOf(light, SeverityProblem), codesOf(full, SeverityProblem))
	}
	if next := StatusNext(report, light, HubInventory{}); next.Command != reconnect.Manual {
		t.Fatalf("next = %+v", next)
	}

	// The same hub named by the user scope is trusted: the token goes there.
	writeRoleFile(t, filepath.Join(home, ".claude.json"), proxyEntryJSON(t, hubURL, tokenFile))
	Run(ctx, DetectRole(project, home, userconfig.Config{}), DepthLight)
	if _, auth := hub.seen(); len(auth) == 0 || auth[0] != "Bearer "+token {
		t.Fatalf("a hub the user scope names did not get the token: %q", auth)
	}
}

func TestRemoteHubThatDoesNotAnswerCannotBeFixedHere(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	hubURL := "http://127.0.0.1:" + strconv.Itoa(port)
	tokenFile := writeTokenFile(t, strings.Repeat("ab", 32))
	project := filepath.Join(t.TempDir(), "project")
	sub := filepath.Join(project, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	binary, err := currentBinary()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := hostintegration.Plan(t.Context(), hostintegration.Request{Root: project, Scope: hostintegration.ScopeProject, Binary: binary, Hosts: []hostintegration.Host{hostintegration.HostClaude}, Hub: &hostintegration.RemoteHub{URL: hubURL, TokenFile: tokenFile}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hostintegration.Apply(t.Context(), plan, hostintegration.ApplyOptions{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	// Run from a subfolder: detection walks up to the project.
	report := DetectRole(sub, t.TempDir(), userconfig.Config{})
	full := Run(t.Context(), report, DepthFull)
	if got := codesOf(full, SeverityProblem); !slices.Equal(got, []string{"remote_reachable"}) {
		t.Fatalf("problems = %v\n%+v", got, full)
	}
	if !strings.HasPrefix(full[0].Title, "This project ("+project+") uses the hub at "+hubURL) {
		t.Fatalf("title = %q", full[0].Title)
	}
	reachable := findingByCode(t, full, "remote_reachable")
	if reachable.Fix != nil || reachable.Manual != "skillhub serve --addr 127.0.0.1:"+strconv.Itoa(port) || !strings.HasPrefix(reachable.Reason, "Skill Hub cannot start a hub on another machine.") {
		t.Fatalf("reachable finding = %+v", reachable)
	}
	fixes := ApplyFixes(t.Context(), full)
	if len(fixes.Fixed) != 0 || len(fixes.NotFixed) != 1 || fixes.NotFixed[0].Code != "remote_reachable" {
		t.Fatalf("fixes = %+v", fixes)
	}
	light := Run(t.Context(), report, DepthLight)
	if !slices.Equal(codesOf(light, SeverityProblem), []string{"remote_reachable"}) {
		t.Fatalf("status problems = %v", codesOf(light, SeverityProblem))
	}
	if next := StatusNext(report, light, HubInventory{}); next.Command != reachable.Manual || !next.Elsewhere || !strings.Contains(next.Label, "hub machine") {
		t.Fatalf("next = %+v", next)
	}
}

func connectProject(t *testing.T, hub string, hosts ...hostintegration.Host) string {
	t.Helper()
	project := t.TempDir()
	result, err := (ConnectService{Store: userconfig.NewStore(t.TempDir())}).Connect(t.Context(), ConnectRequest{Project: project, Workspace: hub, Hosts: hosts})
	if err != nil || result.Error != nil {
		t.Fatalf("connect: %+v %v", result.Error, err)
	}
	return project
}

func TestMovedHubIsRepointedOnlyToARecordedLocation(t *testing.T) {
	t.Parallel()
	original := healthHub(t)
	project := connectProject(t, original, hostintegration.HostClaude)
	moved := filepath.Join(t.TempDir(), "moved-hub")
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()

	notRecorded := DetectRole(project, home, userconfig.Config{Hub: moved})
	sections := Run(t.Context(), notRecorded, DepthFull)
	if len(sections) != 1 || sections[0].Kind != SectionClient {
		t.Fatalf("sections = %+v", sections)
	}
	missing := findingByCode(t, sections, "client_hub_missing")
	if missing.Fix != nil || !strings.HasPrefix(missing.Manual, "skillhub connect "+shellquote.Arg(moved)) {
		t.Fatalf("a folder this machine never recorded must not be re-pointed: %+v", missing)
	}

	storedGone := DetectRole(project, home, userconfig.Config{Hub: original})
	if gone := findingByCode(t, Run(t.Context(), storedGone, DepthLight), "client_hub_missing"); gone.Fix != nil || gone.Manual != shellquote.Chain("cd <new-location>", "skillhub init") {
		t.Fatalf("stored hub gone: %+v", gone)
	}

	recorded := DetectRole(project, home, userconfig.Config{Hub: moved, PreviousHubs: []string{original}})
	sections = Run(t.Context(), recorded, DepthFull)
	repoint := findingByCode(t, sections, "client_hub_missing")
	if repoint.Fix == nil || !strings.Contains(repoint.Fix.Summary, moved) || !strings.Contains(repoint.Fix.Summary, original) {
		t.Fatalf("recorded move: %+v", repoint)
	}
	fixes := ApplyFixes(t.Context(), sections)
	if len(fixes.Fixed) != 1 || !slices.Contains(fixes.Fixed[0].Changed, filepath.Join(project, ".mcp.json")) {
		t.Fatalf("fixes = %+v", fixes)
	}
	after := DetectRole(project, home, userconfig.Config{Hub: moved, PreviousHubs: []string{original}})
	if entry := hostRole(t, after, hostintegration.HostClaude).Project; entry == nil || entry.Workspace != moved {
		t.Fatalf("entry after the fix = %+v", entry)
	}
	// Now the project reaches the moved hub, whose own connection still
	// names the old folder; the next round repairs it.
	sections = Run(t.Context(), after, DepthFull)
	if problems := codesOf(sections, SeverityProblem); !slices.Equal(problems, []string{"hub_self_connection", "client_permissions_outdated"}) || sections[0].Root != moved {
		t.Fatalf("problems after re-pointing = %v", problems)
	}
	ApplyFixes(t.Context(), sections)
	if problems := codesOf(Run(t.Context(), after, DepthFull), SeverityProblem); len(problems) != 0 {
		t.Fatalf("problems after repairing the moved hub = %v", problems)
	}
	if codex := filepath.Join(project, ".codex", "config.toml"); fileExists(codex) {
		t.Fatal("re-pointing installed a host the project did not use")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestOutdatedPermissionRulesAreRewrittenKeepingTheUsersOwn(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	project := connectProject(t, hub, hostintegration.HostClaude)
	settings := filepath.Join(project, ".claude", "settings.local.json")
	var document map[string]any
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	permissions := document["permissions"].(map[string]any)
	var ask []any
	for _, rule := range permissions["ask"].([]any) {
		if rule != "Bash(skillhub *serve*)" {
			ask = append(ask, rule)
		}
	}
	permissions["ask"] = ask
	permissions["allow"] = append(permissions["allow"].([]any), "Bash(make test)")
	edited, _ := json.MarshalIndent(document, "", "  ")
	if err := os.WriteFile(settings, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	report := DetectRole(project, t.TempDir(), userconfig.Config{})
	sections := Run(t.Context(), report, DepthLight)
	outdated := findingByCode(t, sections, "client_permissions_outdated")
	if outdated.Severity != SeverityProblem || outdated.Fix == nil {
		t.Fatalf("permission finding = %+v", outdated)
	}
	if fixes := ApplyFixes(t.Context(), sections); len(fixes.Fixed) != 1 || len(fixes.NotFixed) != 0 {
		t.Fatalf("fixes = %+v", fixes)
	}
	after, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "Bash(skillhub *serve*)") || !strings.Contains(string(after), "Bash(make test)") {
		t.Fatalf("settings after the fix:\n%s", after)
	}
	if problems := codesOf(Run(t.Context(), report, DepthFull), SeverityProblem); len(problems) != 0 {
		t.Fatalf("problems after the fix = %v", problems)
	}
}

func TestExplicitWorkspaceKeepsOnlyTheConnectionsToThatHub(t *testing.T) {
	t.Parallel()
	first, second := healthHub(t), healthHub(t)
	project := connectProject(t, second, hostintegration.HostClaude)
	report := DetectRole(project, t.TempDir(), userconfig.Config{})
	if report.HealthHub() != second {
		t.Fatalf("health hub = %q", report.HealthHub())
	}
	narrowed := report.WithWorkspace(first)
	if narrowed.HealthHub() != first || narrowed.HealthRole() != RoleHub || len(localGroups(narrowed)) != 0 {
		t.Fatalf("narrowed report = %+v", narrowed)
	}
	if !strings.HasSuffix(narrowed.hubCommand("skillhub doctor"), " --workspace "+shellquote.Arg(first)) {
		t.Fatalf("hub command = %q", narrowed.hubCommand("skillhub doctor"))
	}
}

type fakeProvider struct {
	name     string
	root     string
	findings []Finding
	panics   bool
}

func (provider fakeProvider) Name() string { return provider.name }
func (provider fakeProvider) Applies(report RoleReport) bool {
	return report.HealthHub() == provider.root
}
func (provider fakeProvider) Check(context.Context, RoleReport, Depth) []Finding {
	if provider.panics {
		panic("broken check")
	}
	return provider.findings
}

func TestRegisteredProvidersAddToTheHubSection(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	RegisterProvider(fakeProvider{name: "test-node-" + root, root: root, findings: []Finding{{Code: "hub_node_missing", Label: "Node.js", Severity: SeverityNotice, Message: "Node.js is not installed."}}})
	RegisterProvider(fakeProvider{name: "test-broken-" + root, root: root, panics: true})
	for _, bad := range []func(){
		func() { RegisterProvider(nil) },
		func() { RegisterProvider(fakeProvider{name: "hub"}) },
		func() { RegisterProvider(fakeProvider{name: "test-node-" + root}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("RegisterProvider accepted a nil provider or a name in use")
				}
			}()
			bad()
		}()
	}
	sections := Run(t.Context(), RoleReport{Cwd: root, Hub: &HubRef{Root: root}}, DepthLight)
	if len(sections) != 1 {
		t.Fatalf("sections = %+v", sections)
	}
	if node := findingByCode(t, sections, "hub_node_missing"); node.Severity != SeverityNotice {
		t.Fatalf("registered finding = %+v", node)
	}
	broken := findingByCode(t, sections, "test-broken-"+root+"_check_failed")
	if broken.Severity != SeverityProblem || !strings.Contains(broken.Message, "broken check") {
		t.Fatalf("a panicking provider = %+v", broken)
	}
	other := healthHub(t)
	if codes := codesOf(Run(t.Context(), RoleReport{Cwd: other, Hub: &HubRef{Root: other}}, DepthLight), SeverityNotice); slices.Contains(codes, "hub_node_missing") {
		t.Fatal("a provider ran for a hub its Applies refused")
	}
}

func TestApplyFixesRunsEachFixOnceAndReportsWhatItCouldNotFix(t *testing.T) {
	t.Parallel()
	runs := 0
	shared := &FixAction{Summary: "repair the hub", Apply: func(context.Context) (Changed, error) {
		runs++
		return Changed{"/h/.gitignore"}, nil
	}}
	failing := &FixAction{Summary: "rewrite the hook", Apply: func(context.Context) (Changed, error) {
		return nil, errors.New("read-only file system")
	}}
	sections := []Section{{Kind: SectionHub, Findings: []Finding{
		{Code: "gitignore_entry_missing", Severity: SeverityProblem, Fix: shared},
		{Code: "hub_index_stale", Severity: SeverityProblem, Fix: shared},
		{Code: "hub_precommit_validate", Severity: SeverityProblem, Message: "hook", Fix: failing},
		{Code: "hub_schema", Severity: SeverityProblem, Message: "old schema", Manual: "skillhub maintain migrate", Reason: "needs a migration"},
		{Code: "hub_uncommitted", Severity: SeverityNotice, Manual: "git commit"},
		{Code: "hub_listener", Severity: SeverityOK, Fix: shared},
	}}}
	report := ApplyFixes(t.Context(), sections)
	if runs != 1 || len(report.Fixed) != 1 || !slices.Equal(report.Fixed[0].Codes, []string{"gitignore_entry_missing", "hub_index_stale"}) {
		t.Fatalf("runs %d, fixed %+v", runs, report.Fixed)
	}
	want := []NotFixedItem{
		{Code: "hub_precommit_validate", Message: "hook", Reason: "Could not rewrite the hook: read-only file system"},
		{Code: "hub_schema", Message: "old schema", Reason: "needs a migration", Manual: "skillhub maintain migrate"},
	}
	if !slices.Equal(report.NotFixed, want) {
		t.Fatalf("not fixed = %+v", report.NotFixed)
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"changed":["/h/.gitignore"]`) {
		t.Fatalf("JSON = %s, %v", encoded, err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if stopped := ApplyFixes(cancelled, sections); len(stopped.Fixed) != 0 || runs != 1 {
		t.Fatalf("a cancelled fix run changed something: %+v", stopped)
	}
}

func TestFindingJSONRoundTripsTheFixSummary(t *testing.T) {
	t.Parallel()
	finding := Finding{Code: "hub_index_stale", Label: "Search index", Severity: SeverityProblem, Message: "stale", Fix: &FixAction{Summary: "rebuild the search index"}}
	encoded, err := json.Marshal(finding)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"code":"hub_index_stale","label":"Search index","severity":"problem","message":"stale","fix":"rebuild the search index"}` {
		t.Fatalf("JSON = %s", encoded)
	}
	var decoded Finding
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Fix == nil || decoded.Fix.Summary != "rebuild the search index" || decoded.Fix.Apply != nil {
		t.Fatalf("decoded = %+v, %v", decoded, err)
	}
}

func TestStatusNextFollowsTheTableOrder(t *testing.T) {
	t.Parallel()
	fix := &FixAction{Summary: "repair"}
	hub := func(findings ...Finding) []Section {
		return []Section{{Kind: SectionHub, Root: "/h", Findings: findings}}
	}
	stored := makeRoleHub(t, t.TempDir())
	cases := []struct {
		name     string
		report   RoleReport
		sections []Section
		want     string
	}{
		{"none with a stored hub", RoleReport{StoredHub: stored}, nil, "skillhub connect"},
		{"none with a gone stored hub", RoleReport{StoredHub: filepath.Join(stored, "gone")}, nil, shellquote.Chain("cd <new-location>", "skillhub init")},
		{"none without a stored hub", RoleReport{}, nil, NewHubCommand()},
		{"busy first", RoleReport{}, hub(Finding{Code: "hub_busy", Severity: SeverityNotice}, Finding{Code: "x", Severity: SeverityProblem, Fix: fix}), "skillhub status"},
		{"fixable before schema", RoleReport{}, hub(Finding{Code: "hub_schema", Severity: SeverityProblem, Manual: "skillhub maintain migrate"}, Finding{Code: "x", Severity: SeverityProblem, Fix: fix}), "skillhub doctor --fix"},
		{"schema before content", RoleReport{}, hub(Finding{Code: "hub_content_invalid", Severity: SeverityProblem}, Finding{Code: "hub_schema", Severity: SeverityProblem, Manual: "skillhub maintain migrate"}), "skillhub maintain migrate"},
		{"content", RoleReport{}, hub(Finding{Code: "hub_content_invalid", Severity: SeverityProblem}), "skillhub doctor"},
		{"unreachable before reconnect", RoleReport{}, []Section{{Kind: SectionClient, Findings: []Finding{{Code: "remote_reconnect", Severity: SeverityProblem, Manual: "skillhub connect --hub u --token-file f"}, {Code: "remote_reachable", Severity: SeverityProblem, Manual: "skillhub serve"}}}}, "skillhub serve"},
		{"reconnect", RoleReport{}, []Section{{Kind: SectionClient, Findings: []Finding{{Code: "remote_reconnect", Severity: SeverityProblem, Manual: "skillhub connect --hub u --token-file f"}}}}, "skillhub connect --hub u --token-file f"},
		{"other problem with a command", RoleReport{}, hub(Finding{Code: "machine_file_tracked", Label: "Tracked", Severity: SeverityProblem, Message: "Git tracks x.", Manual: "git -C /h rm --cached -- x"}), "git -C /h rm --cached -- x"},
		{"uncommitted", RoleReport{}, hub(Finding{Code: "hub_uncommitted", Severity: SeverityNotice, Manual: "git -C /h add -- skills"}), "git -C /h add -- skills"},
		{"client-remote healthy", RoleReport{}, []Section{{Kind: SectionClient}}, "skillhub doctor"},
	}
	for _, test := range cases {
		if got := StatusNext(test.report, test.sections, HubInventory{}); got.Command != test.want {
			t.Errorf("%s: next = %+v, want %q", test.name, got, test.want)
		}
	}
}

func TestChangedPathsReadsTheRepairItems(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "h")
	result := NewResult(StatusApplied, "done")
	result.Items = []Item{
		{ID: "gitignore_entry_missing", Summary: ".gitignore: Required Git ignore entry is missing: /.codex/", Impact: "Remediated."},
		{ID: "workspace_missing", Summary: ".: Hub folders created.", Impact: "Remediated."},
		{ID: "generation", Summary: "gen-1", Impact: "Published immutable catalog generation."},
		{ID: "host_claude-code_native-skill", Summary: "claude-code (level) native-skill: " + filepath.Join(root, ".claude", "skills", "x", "SKILL.md"), Impact: "Remediated."},
		{ID: "host_codex-cli_mcp-registration", Summary: "codex-cli (level) mcp-registration: " + filepath.Join(root, ".codex", "config.toml"), Impact: "Written."},
		{ID: "rows_skills", Summary: "skills: 0 row(s)", Impact: "Verified projection row count."},
	}
	want := Changed{filepath.Join(root, ".gitignore"), filepath.Join(root, "runtime", "catalog"), filepath.Join(root, ".claude", "skills", "x", "SKILL.md"), filepath.Join(root, ".codex", "config.toml")}
	if got := changedPaths(root, result); !slices.Equal(got, want) {
		t.Fatalf("changed = %v, want %v", got, want)
	}
}
