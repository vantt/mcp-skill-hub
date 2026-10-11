package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

const testHubToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// fakeHub answers like a hub listener: the bearer token gate, then the
// status endpoint unless legacy is set (a hub that predates it answers 404
// there before checking the token).
func fakeHub(t *testing.T, legacy bool, status HubStatus) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if legacy && request.URL.Path == HubStatusPath {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+testHubToken {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if request.URL.Path == HubStatusPath {
			_ = json.NewEncoder(writer).Encode(status)
			return
		}
		http.Error(writer, "Bad Request: missing session ID", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func stoppedHubURL(t *testing.T) string {
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

func TestProbeHubTellsTheFailuresApart(t *testing.T) {
	current := CurrentHubStatus()
	newer := HubStatus{Service: "skillhub", Version: "v9.0.0", RemoteAPI: RemoteAPILevel + 1}
	ctx := context.Background()
	tests := []struct {
		name    string
		url     string
		token   string
		outcome string
		legacy  bool
	}{
		{"current hub", fakeHub(t, false, current), testHubToken, ProbeAccepted, false},
		{"wrong token", fakeHub(t, false, current), strings.Repeat("f", 64), ProbeRejected, false},
		{"no token", fakeHub(t, false, current), "", ProbeNoToken, false},
		{"legacy hub", fakeHub(t, true, current), testHubToken, ProbeAccepted, true},
		{"legacy hub, wrong token", fakeHub(t, true, current), strings.Repeat("f", 64), ProbeRejected, false},
		{"stopped hub", stoppedHubURL(t), testHubToken, ProbeUnreachable, false},
		{"newer hub", fakeHub(t, false, newer), testHubToken, ProbeAccepted, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := ProbeHub(ctx, nil, tc.url, tc.token)
			if probe.Outcome != tc.outcome || probe.Legacy != tc.legacy {
				t.Fatalf("probe = %+v, want %s legacy=%v", probe, tc.outcome, tc.legacy)
			}
			encoded, _ := json.Marshal(probe)
			if tc.token != "" && strings.Contains(string(encoded), tc.token) {
				t.Fatal("the probe result holds the token")
			}
		})
	}
	if probe := ProbeHub(ctx, nil, stoppedHubURL(t), ""); probe.Reason != "connection refused" {
		t.Fatalf("reason for a closed port = %q", probe.Reason)
	}
}

func TestProbeHubRecognizesOtherServersAndGateAnswers(t *testing.T) {
	answers := map[string]struct {
		status  int
		body    string
		outcome string
	}{
		"web server":     {http.StatusNotFound, "not found", ProbeNotAHub},
		"host refused":   {http.StatusForbidden, "Forbidden: the Host header is not a name of this hub; add it with --allow-host", ProbeHostRefused},
		"throttled":      {http.StatusTooManyRequests, "too many failed authentication attempts; wait a minute", ProbeThrottled},
		"rate limited":   {http.StatusTooManyRequests, "too many requests; slow down", ProbeRateLimited},
		"shutting down":  {http.StatusServiceUnavailable, "the hub is shutting down", ProbeHubError},
		"other JSON 200": {http.StatusOK, `{"service":"other"}`, ProbeNotAHub},
	}
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(answer.status)
				_, _ = writer.Write([]byte(answer.body))
			}))
			defer server.Close()
			if probe := ProbeHub(context.Background(), nil, server.URL, testHubToken); probe.Outcome != answer.outcome {
				t.Fatalf("probe = %+v, want %s", probe, answer.outcome)
			}
		})
	}
}

func TestProbeHubDoesNotTakeACatchAllServerForALegacyHub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == HubStatusPath {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte("<html>app</html>"))
	}))
	defer server.Close()
	if probe := ProbeHub(context.Background(), nil, server.URL, testHubToken); probe.Outcome != ProbeNotAHub || probe.Legacy {
		t.Fatalf("catch-all server = %+v", probe)
	}
}

func TestProbeHubNeverEchoesAnErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "upstream failed for "+request.Header.Get("Authorization"), http.StatusBadGateway)
	}))
	defer server.Close()
	probe := ProbeHub(context.Background(), nil, server.URL, testHubToken)
	if probe.Outcome != ProbeHubError || probe.Reason != "Bad Gateway" {
		t.Fatalf("probe = %+v", probe)
	}
}

// redirectingClient sends every request to server, whatever host the URL
// names, so a test can use a hub URL that is not on loopback.
func redirectingClient(server *httptest.Server) *http.Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestDoctorSendsTheTokenOnlyToHubsTheUserScopeNames(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(HubTokenEnv, "")
	received := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			received++
		}
		if request.Header.Get("Authorization") != "Bearer "+testHubToken {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(writer).Encode(CurrentHubStatus())
	}))
	defer server.Close()
	tokenFile := filepath.Join(t.TempDir(), "hub-token")
	if err := os.WriteFile(tokenFile, []byte(testHubToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const hub = "http://hub.example.net:7420"
	service := RemoteDoctorService{Client: redirectingClient(server)}
	project := RemoteEntry{Host: hostintegration.HostClaude, Root: "/p", HubURL: hub, TokenFile: tokenFile}

	reports := service.Check(context.Background(), []RemoteEntry{project})
	if received != 0 {
		t.Fatalf("the token was sent %d times to a hub only the project names", received)
	}
	checks := map[string]RemoteCheck{}
	for _, check := range reports[0].Checks {
		checks[check.ID] = check
	}
	if checks["hub_reachable"].Status != CheckOK || checks["token_accepted"].Status != CheckSkipped ||
		!strings.Contains(checks["token_accepted"].Detail, "only this project's files name this hub") ||
		!strings.Contains(checks["token_accepted"].Detail, "`skillhub connect --hub "+hub+" --token-file "+shellquote.Arg(tokenFile)+" --dry-run`, which sends the token because you run it and writes nothing") || !reports[0].Healthy() {
		t.Fatalf("checks of a project-only hub: %+v", reports[0].Checks)
	}

	global := RemoteEntry{Host: hostintegration.HostCodex, Global: true, Root: "/h", HubURL: hub, TokenFile: tokenFile}
	reports = service.Check(context.Background(), []RemoteEntry{project, global})
	if received == 0 || len(reports) != 1 {
		t.Fatalf("the token was not sent to a hub the user scope names: %d, %+v", received, reports)
	}
	for _, check := range reports[0].Checks {
		if (check.ID == "token_accepted" || check.ID == "hub_version") && check.Status != CheckOK {
			t.Fatalf("checks of a user-scope hub: %+v", reports[0].Checks)
		}
	}
}

func TestDoctorRefusesAHubURLItCannotUse(t *testing.T) {
	reports := (RemoteDoctorService{}).Check(context.Background(), []RemoteEntry{{Host: hostintegration.HostClaude, Root: "/p", HubURL: "http://h:1/other"}})
	if len(reports[0].Checks) != 1 || reports[0].Checks[0].Status != CheckFailed || !strings.Contains(reports[0].Checks[0].Detail, "has the path /other") {
		t.Fatalf("checks: %+v", reports[0].Checks)
	}
}

func TestProbeHubFollowsNoRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+HubStatusPath, http.StatusFound)
	}))
	defer redirect.Close()
	if probe := ProbeHub(context.Background(), nil, redirect.URL, testHubToken); probe.Outcome != ProbeNotAHub || reached {
		t.Fatalf("redirect followed: %+v reached=%v", probe, reached)
	}
}

func TestNormalizeConnectHubURL(t *testing.T) {
	for raw, want := range map[string]string{
		"http://127.0.0.1:7420":         "http://127.0.0.1:7420",
		"http://127.0.0.1:7420/":        "http://127.0.0.1:7420",
		" HTTP://machine-b:7420 ":       "http://machine-b:7420",
		"https://hub.tailnet.ts.net:74": "https://hub.tailnet.ts.net:74",
		"http://[::1]:7420":             "http://[::1]:7420",
	} {
		if got, problem := NormalizeConnectHubURL(raw); problem != nil || got != want {
			t.Errorf("%q = %q, %v", raw, got, problem)
		}
	}
	for raw, why := range map[string]string{
		"127.0.0.1:7420":                 "must start with http:// or https://",
		"ftp://127.0.0.1:7420":           "must start with http:// or https://",
		"http://":                        "has no host",
		"http://127.0.0.1:7420/mcp":      "has the path /mcp",
		"http://user:pw@127.0.0.1:7420":  "user name or password",
		"http://127.0.0.1:7420/?token=x": "query or fragment",
	} {
		_, problem := NormalizeConnectHubURL(raw)
		if problem == nil || !strings.Contains(problem.Render.Why, why) || problem.Render.Error != "The hub URL cannot be used." {
			t.Errorf("%q: %+v", raw, problem)
		}
		if problem != nil && strings.Contains(problem.Render.Why+problem.Render.Fix, "pw@") && !strings.Contains(raw, "pw@") {
			t.Errorf("%q leaks", raw)
		}
	}
	if _, problem := NormalizeConnectHubURL("http://127.0.0.1:7420/mcp"); problem.Render.Fix != "Pass --hub http://127.0.0.1:7420." {
		t.Fatalf("fix = %q", problem.Render.Fix)
	}
}

func TestReadHubTokenDescribesEachProblem(t *testing.T) {
	directory := t.TempDir()
	good := filepath.Join(directory, "good")
	if err := os.WriteFile(good, []byte(testHubToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if token := ReadHubToken("http://h:1", good); token.Problem != nil || token.Value != testHubToken || token.Source != "file" {
		t.Fatalf("good file: %+v", token.Problem)
	}
	missing := ReadHubToken("http://h:1", filepath.Join(directory, "missing"))
	if missing.Problem == nil || !strings.Contains(missing.Problem.Detail, "does not exist") || !strings.Contains(missing.Problem.Fix, "scp") {
		t.Fatalf("missing file: %+v", missing.Problem)
	}
	invalid := filepath.Join(directory, "invalid")
	if err := os.WriteFile(invalid, []byte("not a token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if token := ReadHubToken("http://h:1", invalid); token.Problem == nil || !strings.Contains(token.Problem.Detail, "does not hold a hub token") {
		t.Fatalf("invalid file: %+v", token.Problem)
	}
	if runtime.GOOS != "windows" {
		open := filepath.Join(directory, "open")
		if err := os.WriteFile(open, []byte(testHubToken+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if token := ReadHubToken("http://h:1", open); token.Problem == nil || token.Problem.Fix != "Run `chmod 600 "+open+"`." {
			t.Fatalf("group readable file: %+v", token.Problem)
		}
	}
	t.Setenv(HubTokenEnv, "")
	if token := ReadHubToken("http://h:1", ""); token.Problem == nil || !strings.Contains(token.Problem.Detail, HubTokenEnv+" is not set") {
		t.Fatalf("no token at all: %+v", token.Problem)
	}
	t.Setenv(HubTokenEnv, testHubToken)
	if token := ReadHubToken("http://h:1", ""); token.Problem != nil || token.Source != "environment" {
		t.Fatalf("environment token: %+v", token)
	}
}

func TestCacheCheckReportsAnUnwritableCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directories have no Unix write bit")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes into read-only directories")
	}
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	checks := cacheChecks("http://127.0.0.1:7420")
	if checks[0].Status != CheckOK || checks[1].Detail != "never: no skill was fetched from this hub yet" {
		t.Fatalf("writable cache: %+v", checks)
	}
	if err := os.Chmod(state, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(state, 0o700) })
	checks = cacheChecks("http://127.0.0.1:7420")
	if checks[0].Status != CheckFailed || !strings.Contains(checks[0].Detail, "is not writable") || !strings.Contains(checks[0].Fix, "chmod u+w "+state) {
		t.Fatalf("read-only cache: %+v", checks[0])
	}
}

func TestFindRemoteEntriesReadsProjectAndUserScopes(t *testing.T) {
	project, home := t.TempDir(), t.TempDir()
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(project, ".mcp.json"), `{"mcpServers":{"skillhub":{"args":["mcp","proxy","--hub","http://127.0.0.1:7420","--token-file","/t"]}}}`)
	write(filepath.Join(project, ".gemini", "settings.json"), `{"mcpServers":{"skillhub":{"args":["mcp","serve","--workspace","/w"]}}}`)
	write(filepath.Join(home, ".codex", "config.toml"), "[mcp_servers.skillhub]\nargs = [\"mcp\", \"proxy\", \"--hub\", \"http://100.64.0.2:7420\"]\n")
	entries := FindRemoteEntries(DetectRole(project, home, userconfig.Config{}))
	if len(entries) != 2 || entries[0].Host != hostintegration.HostClaude || entries[0].Global || entries[0].TokenFile != "/t" ||
		entries[1].Host != hostintegration.HostCodex || !entries[1].Global || entries[1].HubURL != "http://100.64.0.2:7420" {
		t.Fatalf("entries = %+v", entries)
	}
	// The home directory itself is never scanned as a project, so its Codex
	// file, which is also the user file, is reported once, as the user scope.
	if entries := FindRemoteEntries(DetectRole(home, home, userconfig.Config{})); len(entries) != 1 || !entries[0].Global {
		t.Fatalf("home as project: %+v", entries)
	}
	// A subfolder of the project finds the project entries by walking up.
	sub := filepath.Join(project, "src", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if entries := FindRemoteEntries(DetectRole(sub, home, userconfig.Config{})); len(entries) != 2 || entries[0].Root != project {
		t.Fatalf("subfolder: %+v", entries)
	}
}

func TestRemoteDoctorGroupsHostsPerHubAndScope(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	hub := stoppedHubURL(t)
	entries := []RemoteEntry{
		{Host: hostintegration.HostClaude, Root: "/p", HubURL: hub},
		{Host: hostintegration.HostCodex, Root: "/p", HubURL: hub},
		{Host: hostintegration.HostGemini, Global: true, Root: "/h", HubURL: hub},
	}
	t.Setenv(HubTokenEnv, testHubToken)
	reports := (RemoteDoctorService{}).Check(context.Background(), entries)
	if len(reports) != 1 || strings.Join(reports[0].UsedBy, "; ") != "Claude Code, Codex (project /p); Gemini CLI (your user account)" {
		t.Fatalf("reports = %+v", reports)
	}
	result := RemoteDoctorResult(reports)
	if result.Status != StatusActionRequired || len(result.Items) != 1 || result.Items[0].ID != "remote_hub_reachable" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRemoteFixRefusalNamesTheConnectCommand(t *testing.T) {
	t.Parallel()
	got := RemoteFixRefusal("http://192.168.1.20:7420", "")
	want := "doctor --fix does not reconnect to a remote hub named in this project's files; if you trust http://192.168.1.20:7420, run: `skillhub connect --hub http://192.168.1.20:7420 --token-file ~/.skillhub-token-192.168.1.20-7420`"
	if got != want {
		t.Fatalf("RemoteFixRefusal without a token file =\n%s\nwant\n%s", got, want)
	}
	tokenFile := filepath.Join(t.TempDir(), "my token")
	if got := RemoteFixRefusal("http://192.168.1.20:7420", tokenFile); !strings.HasSuffix(got, "--token-file "+shellquote.Arg(tokenFile)+"`") {
		t.Fatalf("RemoteFixRefusal with a token file = %s", got)
	}
}

func TestRemoteFixTextsNameTheCurrentServeCommand(t *testing.T) {
	t.Parallel()
	texts := []string{
		tokenCopyFix("/home/a/.skillhub-token-hub-7420"),
		unreachableFix("http://127.0.0.1:7420", "127.0.0.1:7420"),
		unreachableFix("http://100.101.102.103:7420", "100.101.102.103:7420"),
		reachableCheck("http://hub.example:7420", HubProbe{Outcome: ProbeHostRefused}).Fix,
		reachableCheck("http://hub.example:7420", HubProbe{Outcome: ProbeNotAHub}).Fix,
		reachableCheck("http://hub.example:7420", HubProbe{Outcome: ProbeHubError}).Fix,
	}
	for _, text := range texts {
		if strings.Contains(text, "--trust-private-network") || strings.Contains(text, "serve mcp") || !strings.Contains(text, "`skillhub serve") {
			t.Errorf("fix text does not name `skillhub serve` plainly: %s", text)
		}
	}
}
