package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseServeInstallService(t *testing.T) {
	t.Parallel()
	cfg, reason, _ := parseServeArgs([]string{"--install-service", "--addr", "127.0.0.1:7420"})
	if reason != "" || !cfg.installService || cfg.mode != serveModeMCP {
		t.Fatalf("got %+v, reason %q", cfg, reason)
	}
	if _, reason, _ := parseServeArgs([]string{"web", "--install-service"}); !strings.Contains(reason, "only applies to `skillhub serve`") {
		t.Errorf("serve web accepted --install-service: %q", reason)
	}
}

func TestSystemdQuote(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/usr/bin/skillhub": "/usr/bin/skillhub",
		"/home/a b/hub":     `"/home/a b/hub"`,
		"50%":               "50%%",
		"$HOME":             "$$HOME",
		`say "hi"`:          `"say \"hi\""`,
	}
	for in, want := range cases {
		if got := systemdQuote(in); got != want {
			t.Errorf("systemdQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderSystemdUnit(t *testing.T) {
	t.Parallel()
	unit := renderSystemdUnit(serviceSpec{
		executable: "/opt/skill hub/skillhub",
		workspace:  "/home/me/skill-hub",
		args:       []string{"serve", "--workspace", "/home/me/skill-hub", "--addr", "127.0.0.1:7420"},
	})
	for _, want := range []string{
		`ExecStart="/opt/skill hub/skillhub" serve --workspace /home/me/skill-hub --addr 127.0.0.1:7420`,
		"WorkingDirectory=/home/me/skill-hub\n",
		"Restart=on-failure\n",
		"WantedBy=default.target\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q:\n%s", want, unit)
		}
	}
}

func TestRenderLaunchdPlistEscapesValues(t *testing.T) {
	t.Parallel()
	plist := renderLaunchdPlist(serviceSpec{
		executable: "/usr/local/bin/skillhub",
		workspace:  "/Users/me/R&D hub",
		args:       []string{"serve", "--workspace", "/Users/me/R&D hub"},
		logFile:    "/Users/me/Library/Logs/skillhub-server.log",
	})
	for _, want := range []string{
		"<string>dev.skillhub.server</string>",
		"<string>/Users/me/R&amp;D hub</string>",
		"<key>SuccessfulExit</key>",
		"<string>/Users/me/Library/Logs/skillhub-server.log</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "R&D") {
		t.Errorf("plist holds an unescaped ampersand:\n%s", plist)
	}
}

func TestServiceUnitPath(t *testing.T) {
	t.Parallel()
	if got, _ := serviceUnitPath("linux", "/h", ""); got != "/h/.config/systemd/user/skillhub-server.service" {
		t.Errorf("linux default = %q", got)
	}
	if got, _ := serviceUnitPath("linux", "/h", "/x"); got != "/x/systemd/user/skillhub-server.service" {
		t.Errorf("linux XDG = %q", got)
	}
	if got, _ := serviceUnitPath("darwin", "/Users/me", ""); got != "/Users/me/Library/LaunchAgents/dev.skillhub.server.plist" {
		t.Errorf("darwin = %q", got)
	}
	if _, ok := serviceUnitPath("windows", "C:\\", ""); ok {
		t.Error("windows has no service path")
	}
}

// TestInstallServiceWritesUnitAndStartsIt runs the install end to end with a
// recording service runner, so no real service manager is touched.
func TestInstallServiceWritesUnitAndStartsIt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("covers the systemd path")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	hub := filepath.Join(home, "hub")
	initHub(t, hub)

	var calls []string
	oldRunner, oldExe := serviceRunner, serviceExecutable
	t.Cleanup(func() { serviceRunner, serviceExecutable = oldRunner, oldExe })
	serviceExecutable = func() (string, error) { return "/opt/skillhub", nil }
	serviceRunner = func(name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "loginctl" {
			return "no", nil
		}
		return "", nil
	}

	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"serve", "--workspace", hub, "--addr", "127.0.0.1:7420", "--install-service"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stdout %s, stderr %s", code, stdout.String(), stderr.String())
	}
	unit, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", systemdUnitName))
	if err != nil {
		t.Fatal(err)
	}
	if want := "ExecStart=/opt/skillhub serve --workspace " + hub + " --addr 127.0.0.1:7420\n"; !strings.Contains(string(unit), want) {
		t.Errorf("unit lacks %q:\n%s", want, unit)
	}
	if _, err := os.Stat(filepath.Join(hub, "runtime", "config", ".hub-token")); err != nil {
		t.Errorf("the hub token was not created: %v", err)
	}
	wantCalls := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable " + systemdUnitName,
		"systemctl --user restart " + systemdUnitName,
	}
	if strings.Join(calls[:3], "|") != strings.Join(wantCalls, "|") {
		t.Errorf("calls = %q", calls)
	}
	if !strings.Contains(stdout.String(), "loginctl enable-linger") {
		t.Errorf("no linger hint:\n%s", stdout.String())
	}
}

func TestInstallServiceRejectsBadAddrBeforeWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	hub := filepath.Join(home, "hub")
	initHub(t, hub)
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"serve", "--workspace", hub, "--addr", "bad", "--install-service"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, stderr %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); err == nil {
		t.Error("a service file was written for an invalid address")
	}
}
