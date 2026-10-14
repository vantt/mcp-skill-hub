package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/delivery/mcpserver"
	"github.com/vantt/skillhub-mcp/internal/delivery/web"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/invite"
)

// mcpDefaultPort is the port `skillhub serve` listens on when --addr names
// none.
const mcpDefaultPort = 7420

const (
	serveModeMCP = "mcp"
	serveModeWeb = "web"
)

type serveConfig struct {
	mode           string
	workspacePath  string
	addrFlag       string
	tokenFile      string
	allowHosts     []string
	noOpen         bool
	dev            bool
	installService bool
	printConnect   bool
}

// parseServeArgs reads `skillhub serve [mcp|web] [flags]`. Without a mode
// word the mode is mcp. --token-file belongs to mcp; --no-open and --dev to
// web.
func parseServeArgs(args []string) (serveConfig, string, string) {
	cfg := serveConfig{mode: serveModeMCP}
	const fix = "Run `skillhub help serve`."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if args[0] != serveModeMCP && args[0] != serveModeWeb {
			return cfg, fmt.Sprintf("serve has no mode %q", args[0]), "Run `skillhub serve` for agents on other machines, or `skillhub serve web` for the web UI."
		}
		cfg.mode = args[0]
		args = args[1:]
	}
	value := func(index int) (string, bool) {
		if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
			return "", false
		}
		return args[index+1], true
	}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		switch flag {
		case "--workspace", "--addr", "--token-file", "--allow-host":
			if flag == "--token-file" && cfg.mode != serveModeMCP {
				return cfg, "--token-file only applies to `skillhub serve` (mcp); the web UI makes a new session token on every start", fix
			}
			raw, ok := value(index)
			if !ok {
				return cfg, flag + " requires a value", fix
			}
			switch flag {
			case "--workspace":
				cfg.workspacePath = raw
			case "--addr":
				if cfg.addrFlag != "" {
					return cfg, "--addr is given twice", "List several addresses in one --addr, separated by commas, for example --addr 127.0.0.1:7420,100.64.1.2:7420."
				}
				cfg.addrFlag = raw
			case "--token-file":
				cfg.tokenFile = raw
			case "--allow-host":
				cfg.allowHosts = append(cfg.allowHosts, raw)
			}
			index++
		case "--install-service":
			if cfg.mode != serveModeMCP {
				return cfg, "--install-service only applies to `skillhub serve` (mcp), not the web UI", fix
			}
			cfg.installService = true
		case "--print":
			if cfg.mode != serveModeMCP {
				return cfg, "--print only applies to `skillhub serve` (mcp), not the web UI", fix
			}
			cfg.printConnect = true
		case "--no-open", "--dev":
			if cfg.mode != serveModeWeb {
				return cfg, flag + " only applies to `skillhub serve web`", fix
			}
			if flag == "--no-open" {
				cfg.noOpen = true
			} else {
				cfg.dev = true
			}
		case "--profile":
			return cfg, "serve has no --profile: it serves the runtime tools (skill_resolve, skill_get, skill_feedback) to other machines, and curation stays on this machine", "Drop --profile. Agents on this machine get every tool through `skillhub connect`."
		default:
			return cfg, fmt.Sprintf("unknown argument %q", flag), fix
		}
	}
	if cfg.printConnect && cfg.installService {
		return cfg, "--print and --install-service cannot be combined", "Run `skillhub serve --print` to see the connect commands, or `skillhub serve --install-service` to keep the server running."
	}
	return cfg, "", ""
}

// listenAddressesFor applies the mode's default address and port to --addr.
// --dev listens on loopback only, because the Vite dev server in front of it
// is a development tool.
func listenAddressesFor(cfg serveConfig) ([]listenAddress, string, string) {
	defaultPort := mcpDefaultPort
	if cfg.mode == serveModeWeb {
		defaultPort = web.DefaultPort
	}
	raw := cfg.addrFlag
	if raw == "" {
		raw = "0.0.0.0"
		if cfg.dev {
			raw = "127.0.0.1"
		}
	}
	addresses, err := parseListenAddresses(raw, defaultPort)
	if err != nil {
		return nil, err.Error(), "Run `skillhub help serve`."
	}
	if cfg.dev {
		for _, address := range addresses {
			if address.wildcard() || !net.ParseIP(address.host).IsLoopback() {
				return nil, "--dev only listens on loopback", "Drop --addr, or pass a loopback address such as --addr 127.0.0.1:7421."
			}
		}
	}
	return addresses, "", ""
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, reason, fix := parseServeArgs(args)
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, fix)
	}
	addresses, reason, fix := listenAddressesFor(cfg)
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, fix)
	}
	resolved, resErr := resolveWorkspace(cfg.workspacePath)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
	}
	if cfg.installService {
		return runInstallService(cfg, resolved, stdout, stderr)
	}
	if cfg.printConnect {
		return runServePrint(cfg, addresses, resolved, stdout, stderr)
	}
	if cfg.mode == serveModeWeb {
		return runServeWeb(ctx, cfg, addresses, resolved, stdout, stderr)
	}
	return runServeMCP(ctx, cfg, addresses, resolved, stdout, stderr)
}

// bindOrReport binds every address or prints which one failed and why.
func bindOrReport(addresses []listenAddress, stderr io.Writer) ([]net.Listener, int) {
	listeners, failure := bindListeners(addresses)
	if failure != nil {
		termui.New(stderr).Error(
			"Could not listen on "+failure.address.String()+": "+failure.reason()+".",
			"Nothing was started: every address in --addr must be free before the server starts.",
			portOwnerHint(failure.address.port),
		)
		return nil, 1
	}
	return listeners, 0
}

// runServeMCP runs the network MCP listener: the runtime tools for agents on
// other machines, behind the hub token.
func runServeMCP(ctx context.Context, cfg serveConfig, addresses []listenAddress, workspaceRoot string, stdout, stderr io.Writer) int {
	tokenFile, code := ensureHubToken(workspaceRoot, cfg.tokenFile, stderr)
	if code != 0 {
		return code
	}
	listeners, code := bindOrReport(addresses, stderr)
	if code != 0 {
		return code
	}
	defer closeListeners(listeners)

	inviteStore, err := invite.NewFileStore(filepath.Join(workspaceRoot, "runtime", "config", "invites"))
	if err != nil {
		termui.New(stderr).Error(
			"Could not open the invite folder.",
			err.Error(),
			"Check the permissions of the workspace's runtime/config folder, then start it again.",
		)
		return 1
	}

	err = mcpserver.ServeHTTP(ctx, listeners, mcpserver.HTTPOptions{
		Workspace:   workspaceRoot,
		TokenFile:   tokenFile,
		AllowHosts:  cfg.allowHosts,
		Diagnostics: stderr,
		Invites:     invite.NewService(inviteStore, nil, nil),
		Ready: func(bound []net.Addr) {
			interfaces, _ := hostpolicy.SystemInterfaces()
			home, _ := os.UserHomeDir()
			writeMCPBanner(stdout, mcpBanner{
				addresses:  bound,
				workspace:  workspaceRoot,
				tokenFile:  tokenFile,
				home:       home,
				user:       currentUserName(),
				interfaces: interfaces,
			})
		},
	})
	if err != nil {
		termui.New(stderr).Error(
			"The MCP listener stopped.",
			err.Error(),
			"Check workspace health with `skillhub doctor`, then start it again.",
		)
		return 1
	}
	return 0
}

// runServePrint prints the commands another machine runs to connect to this
// hub, for the addresses a server started with the same flags would listen on.
// It starts nothing and creates no token file.
func runServePrint(cfg serveConfig, addresses []listenAddress, workspaceRoot string, stdout, stderr io.Writer) int {
	bound := make([]net.Addr, 0, len(addresses))
	for _, address := range addresses {
		if address.port == 0 {
			return writeInvalidRequest(stdout, stderr, false, "--print needs a fixed port, but "+address.String()+" asks the system for a free one when the server starts", "Give the port the server uses, for example --addr 0.0.0.0:7420.")
		}
		ip := net.IPv4zero
		if address.host != "" {
			parsed, err := netip.ParseAddr(address.host)
			if err != nil {
				return writeInvalidRequest(stdout, stderr, false, "cannot read the address "+address.String(), "Run `skillhub help serve`.")
			}
			ip = net.IP(parsed.AsSlice())
		} else if address.wildcard() {
			ip = net.IPv6unspecified
		}
		bound = append(bound, &net.TCPAddr{IP: ip, Port: address.port})
	}
	tokenFile := cfg.tokenFile
	if tokenFile == "" {
		tokenFile = hubtoken.DefaultPath(workspaceRoot)
	}
	_, statErr := os.Stat(tokenFile)
	interfaces, _ := hostpolicy.SystemInterfaces()
	home, _ := os.UserHomeDir()
	writeMCPBanner(stdout, mcpBanner{
		addresses:    bound,
		workspace:    workspaceRoot,
		tokenFile:    tokenFile,
		home:         home,
		user:         currentUserName(),
		interfaces:   interfaces,
		preview:      true,
		tokenMissing: statErr != nil,
	})
	return 0
}

// runServeWeb runs the web UI behind a new session token.
func runServeWeb(ctx context.Context, cfg serveConfig, addresses []listenAddress, workspaceRoot string, stdout, stderr io.Writer) int {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		termui.New(stderr).Error("Could not generate session token.", err.Error(), "Check system entropy source.")
		return 1
	}
	token := hex.EncodeToString(tokenBytes)

	listeners, code := bindOrReport(addresses, stderr)
	if code != 0 {
		return code
	}
	defer closeListeners(listeners)
	bound := listenerAddresses(listeners)

	srv, err := web.New(web.Options{
		Workspace:   workspaceRoot,
		Token:       token,
		ListenAddrs: bound,
		AllowHosts:  cfg.allowHosts,
		Dev:         cfg.dev,
	})
	if err != nil {
		termui.New(stderr).Error("Could not initialize web server.", err.Error(), "Check workspace health with `skillhub doctor`.")
		return 1
	}

	openURL := ""
	if cfg.dev {
		openURL = "http://127.0.0.1:5421/#token=" + token
		termui.New(stdout).Raw("Skill Hub web UI (dev): " + openURL + "\n")
	} else {
		interfaces, _ := hostpolicy.SystemInterfaces()
		openURL = web.WriteStartup(stdout, bound, token, interfaces)
	}
	if !cfg.noOpen && openURL != "" {
		openBrowser(openURL, stderr)
	}

	if err := srv.Serve(ctx, listeners); err != nil {
		termui.New(stderr).Error("Web server stopped with error.", err.Error(), "Check logs and workspace health.")
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
