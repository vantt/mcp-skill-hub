package cli

import (
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
)

// mcpBanner is what the network MCP listener prints once it accepts
// requests. Every input is passed in, so tests can fix the interfaces, the
// home folder and the user name.
type mcpBanner struct {
	addresses  []net.Addr
	workspace  string
	tokenFile  string
	home       string
	user       string
	interfaces []hostpolicy.Interface
}

// writeMCPBanner tells the user, per address another machine can use, the
// two commands to run there: scp for the token file and skillhub connect.
// URLs carry each listener's real port. It shows the token file path, never
// the token.
func writeMCPBanner(w io.Writer, banner mcpBanner) {
	p := termui.New(w)
	p.Line("Skill Hub is serving skills to agents on other machines.")
	p.Line("It serves the runtime tools only (skill_resolve, skill_get, skill_feedback); curation stays on this machine.")
	p.Blank()
	p.Fields(
		termui.Field{Label: "Hub folder", Value: banner.workspace},
		termui.Field{Label: "Token file", Value: banner.tokenFile},
	)
	if warning := hostpolicy.ExposureWarning(banner.addresses, mcpDefaultPort); warning != "" {
		p.Raw("WARNING: " + warning + "\n")
	}
	login := banner.user
	if login == "" {
		login = "<user>"
	}
	remotePath, copyable := scpTokenPath(banner.tokenFile, banner.home)
	tokenLines := func(machine, hubURL string) {
		clientPath := hubtoken.ClientPath(hubURL)
		if copyable {
			p.Command(scpCommand(login, machine, remotePath, clientPath))
		} else {
			p.Line("Copy the token file to " + clientPath + " there by another means: scp needs a hub folder under your home folder whose path has no spaces or quotes.")
		}
		p.Command("skillhub connect --hub " + hubURL + " --token-file " + clientPath)
	}
	for _, address := range banner.addresses {
		tcp, ok := address.(*net.TCPAddr)
		if !ok {
			continue
		}
		port := strconv.Itoa(tcp.Port)
		reachable := hostpolicy.Reachable(address, banner.interfaces)
		if hostpolicy.IsLoopback(address) || (hostpolicy.IsWildcard(address) && len(reachable) == 0) {
			host := "127.0.0.1"
			if hostpolicy.IsLoopback(address) {
				host = tcp.IP.String()
			}
			hostPort := net.JoinHostPort(host, port)
			p.Heading("This machine only (" + hostPort + "); on the other machine, open an SSH tunnel and keep it running:")
			p.Command("ssh -N -L " + port + ":" + hostPort + " " + login + "@<this-machine>")
			p.Line("Then, in another terminal there:")
			tokenLines("<this-machine>", "http://"+hostPort)
		}
		for _, item := range reachable {
			machine := item.IP.String()
			if item.IP.To4() == nil {
				machine = "[" + machine + "]"
			}
			p.Heading(item.Label + " (" + item.IP.String() + "), on the other machine:")
			tokenLines(machine, "http://"+net.JoinHostPort(item.IP.String(), port))
		}
	}
	p.Blank()
	p.Line("The token itself is never printed. Stop the server with Ctrl-C.")
}

// scpCommand is the line a client machine runs to copy the hub's token file:
// -p keeps the file private (mode 0600), which the proxy requires.
func scpCommand(login, machine, remotePath, clientPath string) string {
	if login == "" {
		login = "<user>"
	}
	return "scp -p " + login + "@" + machine + ":" + remotePath + " " + clientPath
}

// scpPathPattern holds the characters a remote scp path may contain without
// quoting; legacy scp and SFTP mode quote remote paths differently, so a
// path that would need quotes is not printed.
var scpPathPattern = regexp.MustCompile(`^[A-Za-z0-9._+/-]+$`)

// scpTokenPath returns the token file path relative to the hub owner's home
// folder, the folder scp starts in, with forward slashes. ok is false when
// the file is outside home or the path needs quoting.
func scpTokenPath(tokenFile, home string) (string, bool) {
	if home == "" || tokenFile == "" {
		return "", false
	}
	homes := []string{home}
	if resolved, err := filepath.EvalSymlinks(home); err == nil && resolved != home {
		homes = append(homes, resolved)
	}
	for _, base := range homes {
		relative, err := filepath.Rel(base, tokenFile)
		if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		relative = filepath.ToSlash(relative)
		if scpPathPattern.MatchString(relative) && !strings.HasPrefix(relative, "-") {
			return relative, true
		}
		return "", false
	}
	return "", false
}

// userNamePattern holds the characters printed as the scp/ssh login name;
// anything else prints the <user> placeholder.
var userNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// currentUserName is this machine's login name for the printed scp and ssh
// lines, without a Windows domain, or "" when unknown.
func currentUserName() string {
	name := ""
	if current, err := user.Current(); err == nil {
		name = current.Username
	}
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if index := strings.LastIndex(name, `\`); index >= 0 {
		name = name[index+1:]
	}
	if !userNamePattern.MatchString(name) {
		return ""
	}
	return name
}
