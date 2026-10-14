package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/invite"
)

const inviteTestToken = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

// fakeInviteHub stands in for a hub: it takes one invite once, answers the
// status route for the token it handed out, and counts every request.
type fakeInviteHub struct {
	URL      string
	Code     invite.Code
	Requests atomic.Int32

	mu       sync.Mutex
	consumed bool
}

func startFakeInviteHub(t *testing.T) *fakeInviteHub {
	t.Helper()
	code, err := invite.NewCode(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hub := &fakeInviteHub{Code: code}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.Requests.Add(1)
		switch r.URL.Path {
		case "/mcp/pair":
			var body struct {
				Invite string `json:"invite"`
			}
			hub.mu.Lock()
			defer hub.mu.Unlock()
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Invite != string(hub.Code) || hub.consumed {
				http.Error(w, "invite not valid", http.StatusForbidden)
				return
			}
			hub.consumed = true
			_ = json.NewEncoder(w).Encode(map[string]string{"token": inviteTestToken})
		case "/mcp/status":
			if r.Header.Get("Authorization") != "Bearer "+inviteTestToken {
				w.Header().Set("WWW-Authenticate", `Bearer realm="skillhub"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(app.CurrentHubStatus())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	hub.URL = server.URL
	return hub
}

// isolateInviteClient isolates the client machine and gives it a config
// directory that does not exist yet.
func isolateInviteClient(t *testing.T) string {
	t.Helper()
	isolateClient(t)
	configDir := filepath.Join(t.TempDir(), "config", "skillhub")
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", configDir)
	return configDir
}

func TestConnectInviteSavesTheTokenAndConnects(t *testing.T) {
	configDir := isolateInviteClient(t)
	hub := startFakeInviteHub(t)
	project := t.TempDir()
	want := hubtoken.ClientStorePath(configDir, hub.URL)

	code, stdout, stderr := runConnectCLI(t, project, "--hub", hub.URL, "--invite", strings.ToLower(hub.Code.Format()), "--host", "claude")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, stdout, stderr)
	}
	if got, err := hubtoken.Read(want); err != nil || got != inviteTestToken {
		t.Fatalf("saved token = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		for path, mode := range map[string]os.FileMode{want: 0o600, filepath.Dir(want): 0o700} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("%s: %v, %v; want mode %04o", path, info, err, mode)
			}
		}
	}
	if !strings.Contains(flatText(stdout), "Invite accepted. Hub token saved to "+want) {
		t.Fatalf("output lacks the saved path %s:\n%s", want, stdout)
	}
	if !strings.Contains(stdout, "reachable, token accepted") {
		t.Fatalf("the hub check did not run with the saved token:\n%s", stdout)
	}
	tree := treeOf(t, project)
	registration := hostintegration.ReadRegistration(hostintegration.HostClaude, []byte(tree[".mcp.json"]))
	if registration.HubURL != hub.URL || registration.TokenFile != want {
		t.Fatalf("registration = %+v, want %s and %s", registration, hub.URL, want)
	}
	everything := stdout + stderr
	for path, content := range tree {
		everything += content
		_ = path
	}
	for _, secret := range []string{inviteTestToken, string(hub.Code), hub.Code.Format()} {
		if strings.Contains(everything, secret) {
			t.Fatal("the output or a project file holds the token or the invite")
		}
	}
}

func TestConnectInviteUsedTwiceFailsAndKeepsTheSavedFile(t *testing.T) {
	configDir := isolateInviteClient(t)
	hub := startFakeInviteHub(t)
	if code, stdout, stderr := runConnectCLI(t, t.TempDir(), "--hub", hub.URL, "--invite", hub.Code.Format(), "--host", "claude"); code != 0 {
		t.Fatalf("first run: exit %d: %s%s", code, stdout, stderr)
	}
	saved := hubtoken.ClientStorePath(configDir, hub.URL)
	before, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	code, stdout, stderr := runConnectCLI(t, project, "--hub", hub.URL, "--invite", hub.Code.Format(), "--host", "claude")
	if code == 0 {
		t.Fatalf("second run succeeded: %s%s", stdout, stderr)
	}
	if text := flatText(stdout + stderr); !strings.Contains(text, "this invite is not valid") || !strings.Contains(text, "skillhub token invite") {
		t.Fatalf("second run did not explain:\n%s%s", stdout, stderr)
	}
	if strings.Contains(stdout+stderr, string(hub.Code)) || strings.Contains(stdout+stderr, hub.Code.Format()) {
		t.Fatal("the output holds the invite")
	}
	after, err := os.ReadFile(saved)
	if err != nil || string(after) != string(before) {
		t.Fatalf("saved file changed: %q, %v", after, err)
	}
	if len(treeOf(t, project)) != 0 {
		t.Fatal("a failed invite wrote project files")
	}
}

func TestConnectInviteWithTokenFileWritesThere(t *testing.T) {
	isolateInviteClient(t)
	hub := startFakeInviteHub(t)
	destination := filepath.Join(t.TempDir(), "sub", "token")
	code, stdout, stderr := runConnectCLI(t, t.TempDir(), "--hub", hub.URL, "--invite", hub.Code.Format(), "--token-file", destination, "--host", "claude")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, stdout, stderr)
	}
	if got, err := hubtoken.Read(destination); err != nil || got != inviteTestToken {
		t.Fatalf("token at %s = %q, %v", destination, got, err)
	}
}

func TestConnectInviteRefusals(t *testing.T) {
	isolateInviteClient(t)
	hub := startFakeInviteHub(t)
	code := hub.Code.Format()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no hub", []string{"--invite", code}, "--invite needs --hub"},
		{"dry run", []string{"--hub", hub.URL, "--invite", code, "--dry-run"}, "a dry run would use up the invite"},
		{"remove", []string{"--remove", "--invite", code}, "--remove cannot be combined"},
		{"hub folder", []string{"/w", "--hub", hub.URL, "--invite", code}, "A hub folder and --hub cannot be used together"},
		{"malformed code", []string{"--hub", hub.URL, "--invite", "nope"}, "The invite code is not valid"},
		{"code of the wrong length", []string{"--hub", hub.URL, "--invite", code + "A"}, "The invite code is not valid"},
		{"missing value", []string{"--hub", hub.URL, "--invite"}, "--invite requires a value"},
		{"LAN address", []string{"--hub", "http://192.168.1.20:7420", "--invite", code}, "an invite is only sent over Tailscale or to this machine (SSH tunnel); use the scp line instead"},
		{"public address", []string{"--hub", "https://8.8.8.8", "--invite", code}, "an invite is only sent over Tailscale or to this machine"},
		{"destination not writable", []string{"--hub", hub.URL, "--invite", code, "--token-file", filepath.Join(blocker, "sub", "token")}, "The invite is still unused"},
		{"destination is a folder", []string{"--hub", hub.URL, "--invite", code, "--token-file", t.TempDir()}, "The invite is still unused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			exit, stdout, stderr := runConnectCLI(t, project, tc.args...)
			if exit == 0 {
				t.Fatalf("exit 0: %s%s", stdout, stderr)
			}
			if text := flatText(stdout + stderr); !strings.Contains(text, tc.want) {
				t.Fatalf("output lacks %q:\n%s%s", tc.want, stdout, stderr)
			}
			if hub.Requests.Load() != 0 {
				t.Fatalf("the hub received %d requests", hub.Requests.Load())
			}
			if strings.Contains(stdout+stderr, string(hub.Code)) || strings.Contains(stdout+stderr, code) {
				t.Fatal("the output holds the invite")
			}
			if len(treeOf(t, project)) != 0 {
				t.Fatal("a refused invite wrote project files")
			}
		})
	}
}

func TestConnectInviteRefusesANameThatResolvesToALANAddress(t *testing.T) {
	isolateInviteClient(t)
	hub := startFakeInviteHub(t)
	resolved := []netip.Addr{netip.MustParseAddr("100.101.102.103"), netip.MustParseAddr("192.168.1.9")}
	previous := pairingLookup
	pairingLookup = func(context.Context, string) ([]netip.Addr, error) { return resolved, nil }
	t.Cleanup(func() { pairingLookup = previous })
	exit, stdout, stderr := runConnectCLI(t, t.TempDir(), "--hub", "http://hub.example.test:7420", "--invite", hub.Code.Format())
	if exit == 0 || !strings.Contains(flatText(stdout+stderr), "an invite is only sent over Tailscale or to this machine") {
		t.Fatalf("exit %d: %s%s", exit, stdout, stderr)
	}
	if hub.Requests.Load() != 0 {
		t.Fatal("the hub received a request")
	}
}

func TestCheckPairingHost(t *testing.T) {
	lookup := func(addresses ...string) func(context.Context, string) ([]netip.Addr, error) {
		return func(context.Context, string) ([]netip.Addr, error) {
			var out []netip.Addr
			for _, address := range addresses {
				out = append(out, netip.MustParseAddr(address))
			}
			return out, nil
		}
	}
	failing := func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("no such host") }
	cases := []struct {
		name   string
		url    string
		lookup func(context.Context, string) ([]netip.Addr, error)
		ok     bool
	}{
		{"loopback", "http://127.0.0.1:7420", nil, true},
		{"IPv6 loopback", "http://[::1]:7420", nil, true},
		{"mapped loopback", "http://[::ffff:127.0.0.1]:7420", nil, true},
		{"Tailscale", "http://100.64.0.1:7420", nil, true},
		{"localhost", "http://LocalHost:7420", nil, true},
		{"LAN", "http://10.0.0.5:7420", nil, false},
		{"LAN IPv6", "http://[fd00::1]:7420", nil, false},
		{"name on Tailscale", "http://box.tail.test", lookup("100.100.1.1"), true},
		{"name on both", "http://box.tail.test", lookup("100.100.1.1", "192.168.0.2"), false},
		{"name on LAN", "http://box.lan.test", lookup("192.168.0.2"), false},
		{"name not found", "http://nowhere.test", failing, false},
		{"name without addresses", "http://empty.test", lookup(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.lookup != nil {
				previous := pairingLookup
				pairingLookup = tc.lookup
				t.Cleanup(func() { pairingLookup = previous })
			}
			if err := checkPairingHost(context.Background(), tc.url); (err == nil) != tc.ok {
				t.Fatalf("checkPairingHost(%s) = %v, want ok=%v", tc.url, err, tc.ok)
			}
		})
	}
}

func TestNoteSavedTokenNamesTheFileAndTheRerunCommand(t *testing.T) {
	flags := connectFlags{hub: "http://100.64.0.1:7420", global: true}
	result := app.ErrorResult(app.NewInvalidRequestError("why", "do this"))
	result, _ = noteSavedToken(result, nil, flags, "/home/me/my token")
	fix := result.Error.Render.Fix
	for _, want := range []string{"hub token is saved at /home/me/my token", "skillhub connect --hub http://100.64.0.1:7420 --token-file '/home/me/my token' --global", "do this"} {
		if !strings.Contains(fix, want) {
			t.Fatalf("fix lacks %q: %s", want, fix)
		}
	}
	_, err := noteSavedToken(app.Result{}, errors.New("disk full"), flags, "/t")
	if err == nil || !strings.Contains(err.Error(), "saved at /t") || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
}
