package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/delivery/web"
)

type serveConfig struct {
	workspacePath string
	addrFlag      string
	loopbackOnly  bool
	allowHosts    []string
	noOpen        bool
	dev           bool
}

func parseServeFlags(args []string) (serveConfig, string, string) {
	var cfg serveConfig
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return cfg, "--workspace requires a path", "Run `skillhub help serve`."
			}
			cfg.workspacePath = args[i+1]
			i++
		case "--addr":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return cfg, "--addr requires an address", "Run `skillhub help serve`."
			}
			cfg.addrFlag = args[i+1]
			i++
		case "--loopback-only":
			cfg.loopbackOnly = true
		case "--allow-host":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return cfg, "--allow-host requires a host", "Run `skillhub help serve`."
			}
			cfg.allowHosts = append(cfg.allowHosts, args[i+1])
			i++
		case "--no-open":
			cfg.noOpen = true
		case "--dev":
			cfg.dev = true
		default:
			return cfg, fmt.Sprintf("unknown argument %q", args[i]), "Run `skillhub help serve`."
		}
	}
	return cfg, "", ""
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "web" {
		return writeInvalidRequest(stdout, stderr, false, "serve requires the web subcommand", "Run `skillhub serve web`.")
	}

	cfg, parseErr, fix := parseServeFlags(args)
	if parseErr != "" {
		return writeInvalidRequest(stdout, stderr, false, parseErr, fix)
	}

	explicit := cfg.addrFlag
	if explicit == "" {
		explicit = os.Getenv("SKILLHUB_WEB_ADDR")
	}

	if cfg.dev {
		if explicit != "" {
			h, _, err := net.SplitHostPort(explicit)
			if err != nil || (h != "127.0.0.1" && h != "localhost") {
				return writeInvalidRequest(stdout, stderr, false, "--dev only listens on loopback", "Run `skillhub help serve`.")
			}
		} else {
			cfg.loopbackOnly = true
		}
	}

	resolved, resErr := resolveWorkspace(cfg.workspacePath)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		p := termui.New(stderr)
		p.Error("Could not generate session token.", err.Error(), "Check system entropy source.")
		return 1
	}
	token := hex.EncodeToString(tokenBytes)

	ifaces, _ := web.DefaultInterfaces()
	addr, err := web.ChooseListenAddr(explicit, cfg.loopbackOnly, 7421, ifaces)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, false, err.Error(), "Run `skillhub help serve`.")
	}

	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		p := termui.New(stderr)
		p.Error("The web server could not listen on "+addr+".", err.Error(), "Stop the process that owns the port (for example `ss -ltnp | grep 7421`), or pass --addr.")
		return 1
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	srv, err := web.New(web.Options{
		Workspace:  resolved,
		Token:      token,
		ListenPort: port,
		AllowHosts: cfg.allowHosts,
		Dev:        cfg.dev,
	})
	if err != nil {
		p := termui.New(stderr)
		p.Error("Could not initialize web server.", err.Error(), "Check workspace health with `skillhub doctor`.")
		return 1
	}

	boundHost, _, _ := net.SplitHostPort(addr)
	if cfg.dev {
		termui.New(stdout).Raw(fmt.Sprintf("Skill Hub web UI (dev): http://127.0.0.1:5421/#token=%s\n", token))
	} else {
		web.WriteStartup(stdout, boundHost, port, token, ifaces)
	}

	if !cfg.noOpen {
		openURL := fmt.Sprintf("http://127.0.0.1:%d/#token=%s", port, token)
		if boundHost != "0.0.0.0" && boundHost != "127.0.0.1" && boundHost != "localhost" && boundHost != "" {
			openURL = fmt.Sprintf("http://%s:%d/#token=%s", boundHost, port, token)
		}
		openBrowser(openURL, stderr)
	}

	if err := srv.Serve(ctx, ln); err != nil {
		p := termui.New(stderr)
		p.Error("Web server stopped with error.", err.Error(), "Check logs and workspace health.")
		return 1
	}
	return 0
}

func openBrowser(targetURL string, stderr io.Writer) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	if err := cmd.Start(); err != nil {
		termui.New(stderr).Warning(fmt.Sprintf("Could not open browser: %v", err))
	}
}
