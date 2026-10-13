package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/userconfig"
)

func writeRoleFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func makeRoleHub(t *testing.T, dir string) string {
	t.Helper()
	writeRoleFile(t, filepath.Join(dir, ".skillhub", "schema-version"), "5\n")
	return dir
}

func localEntryJSON(t *testing.T, workspace string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{"args": []string{"mcp", "serve", "--workspace", workspace}}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func remoteEntryJSON(t *testing.T, hub string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"skillhub": map[string]any{"args": []string{"mcp", "proxy", "--hub", hub, "--token-file", "/t"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func remoteEntryTOML(hub string) string {
	return "[mcp_servers.skillhub]\nargs = [\"mcp\", \"proxy\", \"--hub\", \"" + hub + "\"]\n"
}

func hostRole(t *testing.T, report RoleReport, host hostintegration.Host) HostRole {
	t.Helper()
	for _, role := range report.Hosts {
		if role.Host == host {
			return role
		}
	}
	t.Fatalf("no section for %s in %+v", host, report)
	return HostRole{}
}

func TestDetectRole(t *testing.T) {
	t.Parallel()
	type fixture struct {
		cwd, home string
		cfg       userconfig.Config
	}
	cases := []struct {
		name  string
		setup func(t *testing.T) fixture
		kind  string
		check func(t *testing.T, f fixture, report RoleReport)
	}{
		{
			name: "hub root with its own connection",
			setup: func(t *testing.T) fixture {
				hub := makeRoleHub(t, t.TempDir())
				writeRoleFile(t, filepath.Join(hub, ".mcp.json"), localEntryJSON(t, hub))
				return fixture{cwd: hub, home: t.TempDir()}
			},
			kind: RoleHub,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if report.Hub == nil || report.Hub.Root != f.cwd {
					t.Fatalf("hub = %+v", report.Hub)
				}
				if claude := hostRole(t, report, hostintegration.HostClaude); claude.Effective == nil || !claude.Effective.Self {
					t.Fatalf("the hub's own entry is not marked as its self-connection: %+v", claude)
				}
			},
		},
		{
			name: "hub subfolder",
			setup: func(t *testing.T) fixture {
				hub := makeRoleHub(t, t.TempDir())
				writeRoleFile(t, filepath.Join(hub, ".mcp.json"), localEntryJSON(t, hub))
				sub := filepath.Join(hub, "skills", "core")
				if err := os.MkdirAll(sub, 0o755); err != nil {
					t.Fatal(err)
				}
				return fixture{cwd: sub, home: t.TempDir()}
			},
			kind: RoleHub,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if report.Hub == nil || report.Hub.Root != filepath.Dir(filepath.Dir(f.cwd)) || report.ProjectRoot != report.Hub.Root {
					t.Fatalf("hub = %+v, project root = %q", report.Hub, report.ProjectRoot)
				}
			},
		},
		{
			name: "hub whose project entry names another hub",
			setup: func(t *testing.T) fixture {
				hub := makeRoleHub(t, t.TempDir())
				other := makeRoleHub(t, t.TempDir())
				writeRoleFile(t, filepath.Join(hub, ".mcp.json"), localEntryJSON(t, other))
				return fixture{cwd: hub, home: t.TempDir()}
			},
			kind: RoleBoth,
		},
		{
			name: "hub plus a user-scope remote entry",
			setup: func(t *testing.T) fixture {
				hub := makeRoleHub(t, t.TempDir())
				writeRoleFile(t, filepath.Join(hub, ".mcp.json"), localEntryJSON(t, hub))
				home := t.TempDir()
				writeRoleFile(t, filepath.Join(home, ".codex", "config.toml"), remoteEntryTOML("http://100.64.0.2:7420"))
				return fixture{cwd: hub, home: home}
			},
			kind: RoleBoth,
		},
		{
			name: "project connected to a local hub",
			setup: func(t *testing.T) fixture {
				project := t.TempDir()
				writeRoleFile(t, filepath.Join(project, ".mcp.json"), localEntryJSON(t, makeRoleHub(t, t.TempDir())))
				return fixture{cwd: project, home: t.TempDir()}
			},
			kind: RoleClientLocal,
		},
		{
			name: "project connected to a remote hub",
			setup: func(t *testing.T) fixture {
				project := t.TempDir()
				writeRoleFile(t, filepath.Join(project, ".gemini", "settings.json"), remoteEntryJSON(t, "http://127.0.0.1:7420"))
				return fixture{cwd: project, home: t.TempDir()}
			},
			kind: RoleClientRemote,
		},
		{
			name: "subfolder of a remote project",
			setup: func(t *testing.T) fixture {
				project := t.TempDir()
				writeRoleFile(t, filepath.Join(project, ".mcp.json"), remoteEntryJSON(t, "http://127.0.0.1:7420"))
				sub := filepath.Join(project, "src", "pkg")
				if err := os.MkdirAll(sub, 0o755); err != nil {
					t.Fatal(err)
				}
				return fixture{cwd: sub, home: t.TempDir()}
			},
			kind: RoleClientRemote,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if report.ProjectRoot != filepath.Dir(filepath.Dir(f.cwd)) {
					t.Fatalf("project root = %q", report.ProjectRoot)
				}
			},
		},
		{
			name: "user scope only",
			setup: func(t *testing.T) fixture {
				home := t.TempDir()
				writeRoleFile(t, filepath.Join(home, ".claude.json"), localEntryJSON(t, makeRoleHub(t, t.TempDir())))
				return fixture{cwd: t.TempDir(), home: home}
			},
			kind: RoleClientLocal,
			check: func(t *testing.T, f fixture, report RoleReport) {
				claude := hostRole(t, report, hostintegration.HostClaude)
				if claude.Project != nil || claude.User == nil || claude.Effective != claude.User || !claude.User.Global {
					t.Fatalf("claude = %+v", claude)
				}
			},
		},
		{
			name: "project entry for one host and a user-scope remote for another",
			setup: func(t *testing.T) fixture {
				project := t.TempDir()
				writeRoleFile(t, filepath.Join(project, ".mcp.json"), localEntryJSON(t, makeRoleHub(t, t.TempDir())))
				home := t.TempDir()
				writeRoleFile(t, filepath.Join(home, ".codex", "config.toml"), remoteEntryTOML("http://100.64.0.2:7420"))
				return fixture{cwd: project, home: home}
			},
			kind: RoleClientLocal,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if codex := hostRole(t, report, hostintegration.HostCodex); codex.Effective == nil || !codex.Effective.Remote() {
					t.Fatalf("codex section = %+v, want its user-scope remote entry", codex)
				}
			},
		},
		{
			name: "a project entry wins over the user entry of the same host",
			setup: func(t *testing.T) fixture {
				project := t.TempDir()
				writeRoleFile(t, filepath.Join(project, ".mcp.json"), remoteEntryJSON(t, "http://127.0.0.1:7420"))
				home := t.TempDir()
				writeRoleFile(t, filepath.Join(home, ".claude.json"), localEntryJSON(t, makeRoleHub(t, t.TempDir())))
				return fixture{cwd: project, home: home}
			},
			kind: RoleClientRemote,
		},
		{
			name: "nothing, with a stored hub",
			setup: func(t *testing.T) fixture {
				return fixture{cwd: t.TempDir(), home: t.TempDir(), cfg: userconfig.Config{Hub: makeRoleHub(t, t.TempDir()), PreviousHubs: []string{"/old"}}}
			},
			kind: RoleNone,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if report.StoredHub != f.cfg.Hub || len(report.PreviousHubs) != 1 {
					t.Fatalf("stored hub = %q, previous = %v", report.StoredHub, report.PreviousHubs)
				}
			},
		},
		{
			name: "nothing, without a stored hub",
			setup: func(t *testing.T) fixture {
				return fixture{cwd: t.TempDir(), home: t.TempDir()}
			},
			kind: RoleNone,
		},
		{
			name: "entry that names a hub folder that is gone",
			setup: func(t *testing.T) fixture {
				project := t.TempDir()
				writeRoleFile(t, filepath.Join(project, ".mcp.json"), localEntryJSON(t, filepath.Join(t.TempDir(), "moved")))
				return fixture{cwd: project, home: t.TempDir()}
			},
			kind: RoleClientLocal,
			check: func(t *testing.T, f fixture, report RoleReport) {
				claude := hostRole(t, report, hostintegration.HostClaude)
				if claude.Effective == nil || IsHubRoot(claude.Effective.Workspace) {
					t.Fatalf("claude = %+v", claude)
				}
			},
		},
		{
			name: "stored hub that is gone",
			setup: func(t *testing.T) fixture {
				return fixture{cwd: t.TempDir(), home: t.TempDir(), cfg: userconfig.Config{Hub: filepath.Join(t.TempDir(), "gone")}}
			},
			kind: RoleNone,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if IsHubRoot(report.StoredHub) {
					t.Fatal("a missing stored hub reads as a hub")
				}
			},
		},
		{
			name: "home itself is a Git repository",
			setup: func(t *testing.T) fixture {
				home := t.TempDir()
				if err := os.MkdirAll(filepath.Join(home, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
				project := filepath.Join(home, "code", "app")
				if err := os.MkdirAll(project, 0o755); err != nil {
					t.Fatal(err)
				}
				return fixture{cwd: project, home: home}
			},
			kind: RoleNone,
		},
		{
			name: "hub outside home",
			setup: func(t *testing.T) fixture {
				hub := makeRoleHub(t, filepath.Join(t.TempDir(), "srv", "skill-hub"))
				return fixture{cwd: hub, home: t.TempDir()}
			},
			kind: RoleHub,
		},
		{
			name: "home is never scanned",
			setup: func(t *testing.T) fixture {
				home := makeRoleHub(t, t.TempDir())
				writeRoleFile(t, filepath.Join(home, ".mcp.json"), localEntryJSON(t, home))
				sub := filepath.Join(home, "notes")
				if err := os.MkdirAll(sub, 0o755); err != nil {
					t.Fatal(err)
				}
				return fixture{cwd: sub, home: home}
			},
			kind: RoleNone,
			check: func(t *testing.T, f fixture, report RoleReport) {
				if again := DetectRole(f.home, f.home, userconfig.Config{}); again.Kind() != RoleNone || again.Hub != nil {
					t.Fatalf("home as the current folder = %s, hub %+v", again.Kind(), again.Hub)
				}
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := test.setup(t)
			report := DetectRole(f.cwd, f.home, f.cfg)
			if got := report.Kind(); got != test.kind {
				t.Fatalf("Kind() = %s, want %s; report %+v", got, test.kind, report)
			}
			if test.check != nil {
				test.check(t, f, report)
			}
		})
	}
}

func TestDetectRoleIgnoresAProjectFileLinkingOutside(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	outside := filepath.Join(t.TempDir(), "entry.json")
	writeRoleFile(t, outside, localEntryJSON(t, makeRoleHub(t, t.TempDir())))
	if err := os.Symlink(outside, filepath.Join(project, ".mcp.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if kind := DetectRole(project, t.TempDir(), userconfig.Config{}).Kind(); kind != RoleNone {
		t.Fatalf("Kind() = %s, want none for a link that leaves the project", kind)
	}
}

func TestMovedHubFixNamesTheRightRepair(t *testing.T) {
	t.Parallel()
	stored := makeRoleHub(t, t.TempDir())
	missing := filepath.Join(t.TempDir(), "old-hub")
	cases := []struct {
		name   string
		report RoleReport
		want   string
	}{
		{"recorded earlier", RoleReport{StoredHub: stored, PreviousHubs: []string{missing}}, "skillhub doctor --fix"},
		{"never recorded", RoleReport{StoredHub: stored}, "skillhub connect"},
		{"stored hub gone", RoleReport{StoredHub: missing, PreviousHubs: []string{"/x"}}, "skillhub init"},
		{"no stored hub", RoleReport{}, NewHubCommand()},
	}
	for _, test := range cases {
		if got := MovedHubFix(test.report, missing); !strings.Contains(got, test.want) {
			t.Errorf("%s: %q does not name %q", test.name, got, test.want)
		}
	}
	if got := MovedHubFix(RoleReport{StoredHub: missing}, missing); strings.Contains(got, "connect "+missing) || !strings.Contains(got, "skillhub doctor --fix") || !strings.Contains(got, "skillhub init") {
		t.Errorf("the stored hub itself gone: %q", got)
	}
}
