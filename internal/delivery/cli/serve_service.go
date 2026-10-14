package cli

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

const (
	systemdUnitName  = "skillhub-server.service"
	launchdLabel     = "dev.skillhub.server"
	launchdLogName   = "skillhub-server.log"
	serviceRestartIn = 10
)

// serviceSpec is everything a service definition needs to start the same
// `skillhub serve` the user would run by hand.
type serviceSpec struct {
	executable string
	workspace  string
	// args are the flags after `serve`: the hub, the addresses and the token
	// file, written out so the service does not depend on a working directory.
	args []string
	// logFile is where launchd sends output; systemd uses the journal.
	logFile string
}

// serviceRunner runs one external command and returns its combined output.
// Tests replace it so no service manager is touched.
var serviceRunner = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// serviceExecutable is the path of the running skillhub with symlinks
// resolved, so the service keeps working after a package manager moves the
// link. Tests replace it.
var serviceExecutable = func() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path, nil
}

// serviceArgs lists the `serve` flags the service starts with. Only the flags
// the user passed are written; the default address and token file stay
// defaults, so a later release can change them for everyone.
func serviceArgs(cfg serveConfig, workspace string) ([]string, error) {
	args := []string{"serve", "--workspace", workspace}
	if cfg.addrFlag != "" {
		args = append(args, "--addr", cfg.addrFlag)
	}
	if cfg.tokenFile != "" {
		absolute, err := filepath.Abs(cfg.tokenFile)
		if err != nil {
			return nil, err
		}
		args = append(args, "--token-file", absolute)
	}
	for _, host := range cfg.allowHosts {
		args = append(args, "--allow-host", host)
	}
	if cfg.strictPairing {
		args = append(args, "--strict-pairing")
	}
	return args, nil
}

// systemdQuote makes one word of a unit file ExecStart line. systemd splits
// the line on spaces, honours double quotes and backslashes, and expands %
// and $ itself, so each is protected.
func systemdQuote(value string) string {
	value = strings.NewReplacer("%", "%%", "$", "$$").Replace(value)
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '"' || r == '\'' || r == '\\' || r == ';'
	}) < 0 {
		return value
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func renderSystemdUnit(spec serviceSpec) string {
	words := []string{systemdQuote(spec.executable)}
	for _, arg := range spec.args {
		words = append(words, systemdQuote(arg))
	}
	var unit strings.Builder
	unit.WriteString("[Unit]\nDescription=Skill Hub server for other machines\nAfter=network-online.target\nWants=network-online.target\n\n")
	unit.WriteString("[Service]\n")
	fmt.Fprintf(&unit, "WorkingDirectory=%s\n", strings.ReplaceAll(spec.workspace, "%", "%%"))
	fmt.Fprintf(&unit, "ExecStart=%s\n", strings.Join(words, " "))
	fmt.Fprintf(&unit, "Restart=on-failure\nRestartSec=%d\n\n", serviceRestartIn)
	unit.WriteString("[Install]\nWantedBy=default.target\n")
	return unit.String()
}

func xmlText(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func renderLaunchdPlist(spec serviceSpec) string {
	var plist strings.Builder
	plist.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, word := range append([]string{spec.executable}, spec.args...) {
		fmt.Fprintf(&plist, "\t\t<string>%s</string>\n", xmlText(word))
	}
	fmt.Fprintf(&plist, `	</array>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>%d</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, xmlText(spec.workspace), serviceRestartIn, xmlText(spec.logFile), xmlText(spec.logFile))
	return plist.String()
}

// serviceUnitPath is where the service definition of goos for the user lives.
func serviceUnitPath(goos, home, configHome string) (string, bool) {
	switch goos {
	case "linux":
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "systemd", "user", systemdUnitName), true
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), true
	}
	return "", false
}

// runInstallService installs the network MCP listener as a user service that
// starts at login (launchd) or boot (systemd) and restarts after a failure.
func runInstallService(cfg serveConfig, workspaceRoot string, stdout, stderr io.Writer) int {
	p := termui.New(stderr)
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return writeInvalidRequest(stdout, stderr, false, "--install-service supports systemd on Linux and launchd on macOS", "Start `skillhub serve` yourself, or use a service manager of your system.")
	}
	// Check the flags the service will use before anything is written.
	if _, reason, fix := listenAddressesFor(cfg); reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, fix)
	}
	if _, code := ensureHubToken(workspaceRoot, cfg.tokenFile, stderr); code != 0 {
		return code
	}
	home, err := os.UserHomeDir()
	if err != nil {
		p.Error("The home folder cannot be found.", err.Error(), "Set HOME and run the command again.")
		return 1
	}
	executable, err := serviceExecutable()
	if err != nil {
		p.Error("The path of this skillhub cannot be found.", err.Error(), "Run the command again with skillhub started by its full path.")
		return 1
	}
	args, err := serviceArgs(cfg, workspaceRoot)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, false, "the token file path is not usable", "Pass an absolute --token-file path.")
	}
	spec := serviceSpec{
		executable: executable,
		workspace:  workspaceRoot,
		args:       args,
		logFile:    filepath.Join(home, "Library", "Logs", launchdLogName),
	}
	path, _ := serviceUnitPath(runtime.GOOS, home, os.Getenv("XDG_CONFIG_HOME"))
	content := renderSystemdUnit(spec)
	if runtime.GOOS == "darwin" {
		content = renderLaunchdPlist(spec)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		p.Error("The service folder cannot be created.", err.Error(), "Check the permissions of "+filepath.Dir(path)+".")
		return 1
	}
	if runtime.GOOS == "darwin" {
		if err := os.MkdirAll(filepath.Dir(spec.logFile), 0o755); err != nil {
			p.Error("The log folder cannot be created.", err.Error(), "Check the permissions of "+filepath.Dir(spec.logFile)+".")
			return 1
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		p.Error("The service file cannot be written.", err.Error(), "Check the permissions of "+path+".")
		return 1
	}
	out := termui.New(stdout)
	out.Line("Wrote " + path)

	if runtime.GOOS == "darwin" {
		return startLaunchdService(path, spec, stdout, stderr)
	}
	return startSystemdService(stdout, stderr)
}

func startSystemdService(stdout, stderr io.Writer) int {
	p := termui.New(stderr)
	out := termui.New(stdout)
	steps := [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", systemdUnitName},
		{"--user", "restart", systemdUnitName},
	}
	for _, step := range steps {
		if output, err := serviceRunner("systemctl", step...); err != nil {
			p.Error(
				"systemctl "+strings.Join(step, " ")+" failed.",
				firstNonEmpty(output, err.Error()),
				"The unit file is written. Fix the cause, then run `systemctl --user enable --now "+systemdUnitName+"`.",
			)
			return 1
		}
	}
	out.Line("Started " + systemdUnitName + ". It restarts after a failure and at boot.")
	out.Line("  Status:  systemctl --user status " + systemdUnitName)
	out.Line("  Logs:    journalctl --user -u " + systemdUnitName)
	out.Line("  Stop:    systemctl --user disable --now " + systemdUnitName)
	if linger, err := serviceRunner("loginctl", "show-user", currentUserName(), "--property=Linger", "--value"); err == nil && linger == "no" {
		out.Line("  To keep it running when you are logged out, run: loginctl enable-linger " + shellquote.Arg(currentUserName()))
	}
	return 0
}

func startLaunchdService(plistPath string, spec serviceSpec, stdout, stderr io.Writer) int {
	p := termui.New(stderr)
	out := termui.New(stdout)
	current, err := user.Current()
	if err != nil {
		p.Error("The current user cannot be found.", err.Error(), "Run the command from a normal login session.")
		return 1
	}
	domain := "gui/" + current.Uid
	// A service from an earlier install is unloaded first; it is fine when
	// there is none.
	_, _ = serviceRunner("launchctl", "bootout", domain+"/"+launchdLabel)
	if output, err := serviceRunner("launchctl", "bootstrap", domain, plistPath); err != nil {
		p.Error(
			"launchctl bootstrap failed.",
			firstNonEmpty(output, err.Error()),
			"The plist is written. Fix the cause, then run `launchctl bootstrap "+domain+" "+shellquote.Arg(plistPath)+"`.",
		)
		return 1
	}
	out.Line("Started " + launchdLabel + ". It starts at login and restarts after a failure.")
	out.Line("  Status:  launchctl print " + domain + "/" + launchdLabel)
	out.Line("  Logs:    " + spec.logFile)
	out.Line("  Stop:    launchctl bootout " + domain + "/" + launchdLabel)
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
