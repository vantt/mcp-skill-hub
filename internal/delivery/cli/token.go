package cli

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/invite"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// stdoutIsTerminal reports whether w is an interactive terminal. Tests
// replace it; the token is printed only when it returns true.
var stdoutIsTerminal = func(w io.Writer) bool {
	if out, ok := w.(commandOutput); ok {
		w = out.Writer
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

type tokenConfig struct {
	workspacePath string
	tokenFile     string
	addrFlag      string
	dryRun        bool
}

func runToken(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "show" && args[0] != "rotate" && args[0] != "invite") {
		return writeInvalidRequest(stdout, stderr, false, "token requires the show, rotate or invite subcommand", "Run `skillhub token show`, `skillhub token rotate` or `skillhub token invite`.")
	}
	cfg, reason := parseTokenFlags(args[1:], args[0])
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, "Run `skillhub help token`.")
	}
	if args[0] == "invite" {
		return runTokenInvite(cfg, stdout, stderr)
	}
	path := cfg.tokenFile
	if path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return writeInvalidRequest(stdout, stderr, false, "the token file path is not usable", "Pass an absolute --token-file path.")
		}
		path = absolute
	} else {
		resolved, resErr := resolveWorkspace(cfg.workspacePath)
		if resErr != nil {
			return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
		}
		path = hubtoken.DefaultPath(resolved)
	}
	if args[0] == "show" {
		return runTokenShow(path, stdout, stderr)
	}
	return runTokenRotate(path, cfg, stdout, stderr)
}

// parseTokenFlags reads the flags of one token subcommand: --dry-run belongs
// to rotate, --addr to invite, --token-file to show and rotate.
func parseTokenFlags(args []string, subcommand string) (tokenConfig, string) {
	var cfg tokenConfig
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--workspace", "--token-file", "--addr":
			if (args[index] == "--token-file" && subcommand == "invite") || (args[index] == "--addr" && subcommand != "invite") {
				return cfg, fmt.Sprintf("unknown argument %q", args[index])
			}
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				if args[index] == "--addr" {
					return cfg, "--addr requires a value"
				}
				return cfg, args[index] + " requires a path"
			}
			switch args[index] {
			case "--workspace":
				cfg.workspacePath = args[index+1]
			case "--addr":
				cfg.addrFlag = args[index+1]
			default:
				cfg.tokenFile = args[index+1]
			}
			index++
		case "--dry-run":
			if subcommand != "rotate" {
				return cfg, "unknown argument \"--dry-run\""
			}
			cfg.dryRun = true
		default:
			return cfg, fmt.Sprintf("unknown argument %q", args[index])
		}
	}
	return cfg, ""
}

// runTokenShow prints the token only to a terminal, so it never lands in a
// file, a pipe, a log, or an agent transcript.
func runTokenShow(path string, stdout, stderr io.Writer) int {
	if !stdoutIsTerminal(stdout) {
		p := termui.New(stderr)
		p.Error(
			"The hub token is printed only on a terminal.",
			"Standard output is redirected or captured, and the token must not end up in a file, a log, or an agent transcript.",
			"Copy the token file below with the scp line `skillhub serve` prints, or run `skillhub token show` directly in a terminal.",
		)
		p.Fields(termui.Field{Label: "Token file", Value: path})
		return 1
	}
	created, err := hubtoken.Ensure(path)
	if err != nil {
		writeHubTokenError(stderr, path, err)
		return 1
	}
	token, err := hubtoken.Read(path)
	if err != nil {
		writeHubTokenError(stderr, path, err)
		return 1
	}
	p := termui.New(stdout)
	if created {
		p.Line("No hub token existed, so a new one was created.")
		p.Blank()
	}
	p.Fields(
		termui.Field{Label: "Token file", Value: path},
		termui.Field{Label: "Token", Value: token},
	)
	p.Blank()
	p.Line("Anyone with this token can call the hub's runtime tools. Keep it private and prefer copying the token file.")
	if p.Err() != nil {
		return 1
	}
	return 0
}

// runTokenRotate replaces the token. It runs only on a terminal, so an agent
// that has no prompt for this command (Codex, Gemini CLI) cannot cut every
// connected machine off on its own; --dry-run works anywhere.
func runTokenRotate(path string, cfg tokenConfig, stdout, stderr io.Writer) int {
	p := termui.New(stdout)
	if cfg.dryRun {
		p.Line("Dry run: the hub token was not replaced.")
		p.Fields(termui.Field{Label: "Token file", Value: path})
		p.Line("Would change:")
		p.Bullets(
			"The token is replaced with a new random one.",
			"Every machine that connects to this hub stops working until it gets the new token file.",
			"A running listener uses the new token from its next request; it does not need a restart.",
		)
		p.Blank()
		p.Line("To replace the token, run in a terminal:")
		p.Command(tokenRotateCommand(cfg))
		return printerCode(p)
	}
	if !stdoutIsTerminal(stdout) {
		e := termui.New(stderr)
		e.Error(
			"The hub token is rotated only on a terminal.",
			"Rotating cuts off every machine that connects to this hub, so a person runs it, not a script or an agent.",
			"Run this command in a terminal:",
		)
		e.Command(tokenRotateCommand(cfg))
		return 1
	}
	if err := hubtoken.Rotate(path); err != nil {
		writeHubTokenError(stderr, path, err)
		return 1
	}
	p.Line("Hub token rotated.")
	p.Fields(termui.Field{Label: "Token file", Value: path})
	p.Blank()
	home, _ := os.UserHomeDir()
	if remotePath, ok := scpTokenPath(path, home); ok {
		p.Next("On each machine that connects to this hub, copy the new token file again, replacing the old one:", scpCommand(currentUserName(), "<this-machine>", remotePath, "~/.skillhub-token-<host>-<port>"))
	} else {
		p.Next("Copy the new token file to each machine that connects to this hub, replacing the old one.", "")
	}
	if p.Err() != nil {
		return 1
	}
	return 0
}

func tokenRotateCommand(cfg tokenConfig) string {
	command := "skillhub token rotate"
	if cfg.tokenFile != "" {
		command += " --token-file " + shellquote.Arg(cfg.tokenFile)
	} else if cfg.workspacePath != "" {
		command += " --workspace " + shellquote.Arg(cfg.workspacePath)
	}
	return command
}

// systemInterfaces lists this machine's network interfaces; tests replace it.
var systemInterfaces = hostpolicy.SystemInterfaces

// runTokenInvite issues a one-time invite and prints the connect line for each
// address the pairing route accepts. It runs only on a terminal, because the
// code lets whoever holds it fetch the hub token. It creates no hub token and
// starts nothing.
func runTokenInvite(cfg tokenConfig, stdout, stderr io.Writer) int {
	if !stdoutIsTerminal(stdout) {
		e := termui.New(stderr)
		e.Error(
			"An invite is created only on a terminal.",
			"The invite code lets whoever holds it fetch the hub token, so it must not end up in a file, a log, or an agent transcript.",
			"Run this command in a terminal:",
		)
		e.Command(tokenInviteCommand(cfg))
		return 1
	}
	addresses, reason, fix := listenAddressesFor(serveConfig{mode: serveModeMCP, addrFlag: cfg.addrFlag})
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, fix)
	}
	bound := make([]net.Addr, 0, len(addresses))
	for _, address := range addresses {
		if address.port == 0 {
			return writeInvalidRequest(stdout, stderr, false, "token invite needs a fixed port, but "+address.String()+" asks the system for a free one when the server starts", "Give the port the server uses, for example --addr 0.0.0.0:7420.")
		}
		ip := net.IPv6unspecified
		if address.host != "" {
			parsed, err := netip.ParseAddr(address.host)
			if err != nil {
				return writeInvalidRequest(stdout, stderr, false, "cannot read the address "+address.String(), "Run `skillhub help token`.")
			}
			ip = net.IP(parsed.AsSlice())
		}
		bound = append(bound, &net.TCPAddr{IP: ip, Port: address.port})
	}
	workspace, resErr := resolveWorkspace(cfg.workspacePath)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
	}
	var code invite.Code
	var expires time.Time
	store, err := invite.NewFileStore(filepath.Join(workspace, "runtime", "config", "invites"))
	if err == nil {
		code, expires, err = invite.NewService(store, nil, nil).Issue()
	}
	if err != nil {
		termui.New(stderr).Error(
			"Could not create the invite.",
			err.Error(),
			"Check that "+filepath.Join(workspace, "runtime", "config")+" is writable and private to your user account, then run the command again.",
		)
		return 1
	}

	interfaces, _ := systemInterfaces()
	login := currentUserName()
	if login == "" {
		login = "<user>"
	}
	p := termui.New(stdout)
	p.Line("Invite created. It works once and expires at " + expires.Local().Format("15:04") + " (" + strconv.Itoa(int(invite.Life.Minutes())) + " minutes).")
	p.Fields(termui.Field{Label: "Hub folder", Value: workspace})
	skipped := false
	for _, option := range connectTargets(bound, interfaces) {
		if option.kind == targetLAN {
			skipped = true
			continue
		}
		p.Blank()
		if option.kind == targetLoopback {
			p.Line("This machine only (" + net.JoinHostPort(option.host, strconv.Itoa(option.port)) + "); on the other machine, open an SSH tunnel and keep it running:")
			p.Command(sshTunnelCommand(option, login))
			p.Line("Then, in another terminal there:")
		} else {
			p.Line(option.label + " (" + option.host + "), on the other machine:")
		}
		p.Command("skillhub connect --hub " + option.hubURL + " --invite " + code.String())
	}
	p.Blank()
	if skipped {
		p.Line("Other addresses are not listed: an invite works only over Tailscale or an SSH tunnel. The `scp` way that `skillhub serve` prints still works there.")
	}
	p.Line("The code is not the hub token. Running this command again gives a new code.")
	p.Line("Run it with the same --workspace and --addr the server uses: the invite counts only for the `skillhub serve` that uses this hub folder. If the server is not running, start `skillhub serve`.")
	return printerCode(p)
}

func tokenInviteCommand(cfg tokenConfig) string {
	command := "skillhub token invite"
	if cfg.workspacePath != "" {
		command += " --workspace " + shellquote.Arg(cfg.workspacePath)
	}
	if cfg.addrFlag != "" {
		command += " --addr " + shellquote.Arg(cfg.addrFlag)
	}
	return command
}
