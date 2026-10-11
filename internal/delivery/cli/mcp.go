package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver"
	"github.com/vantt/mcp-skill-hub/internal/delivery/web"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
)

// runMCP keeps `skillhub mcp serve` working, unchanged, for the host
// registrations that already launch it: it is the stdio form of
// `skillhub serve mcp` and accepts the same flags as before, so the network
// flags stay unknown there.
// `skillhub mcp proxy` is the stdio bridge to a remote hub.
func runMCP(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "proxy" {
		return runMCPProxy(ctx, args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "serve" {
		return writeInvalidRequest(stdout, stderr, false, "mcp requires the serve subcommand", "Run `skillhub mcp serve --workspace <path>`.")
	}
	return serveMCP(ctx, args[1:], false, stdout, stderr)
}

type mcpServeConfig struct {
	workspacePath       string
	profile             mcpserver.Profile
	profileSet          bool
	addr                string
	trustPrivateNetwork bool
	tokenFile           string
	allowHosts          []string
}

func parseMCPServeFlags(args []string, allowNetwork bool) (mcpServeConfig, string, string) {
	cfg := mcpServeConfig{profile: mcpserver.ProfileAll}
	value := func(index int) (string, bool) {
		if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
			return "", false
		}
		return args[index+1], true
	}
	for index := 0; index < len(args); index++ {
		if !allowNetwork && isNetworkFlag(args[index]) {
			return cfg, fmt.Sprintf("unknown MCP argument %q", args[index]), "Run `skillhub mcp serve --workspace <path>`."
		}
		switch args[index] {
		case "--profile":
			raw, ok := value(index)
			if !ok {
				return cfg, "--profile requires a value", "Run `skillhub mcp serve --profile runtime|curation|all`."
			}
			profile := mcpserver.Profile(raw)
			if profile != mcpserver.ProfileRuntime && profile != mcpserver.ProfileCuration && profile != mcpserver.ProfileAll {
				return cfg, fmt.Sprintf("invalid profile %q", raw), "Run `skillhub mcp serve --profile runtime|curation|all`."
			}
			cfg.profile, cfg.profileSet = profile, true
			index++
		case "--workspace":
			raw, ok := value(index)
			if !ok {
				return cfg, "--workspace requires a path", "Run `skillhub mcp serve --workspace <path>`."
			}
			cfg.workspacePath = raw
			index++
		case "--addr":
			raw, ok := value(index)
			if !ok {
				return cfg, "--addr requires an address", "Run `skillhub serve mcp --addr 127.0.0.1:7420`."
			}
			cfg.addr = raw
			index++
		case "--token-file":
			raw, ok := value(index)
			if !ok {
				return cfg, "--token-file requires a path", "Run `skillhub help serve`."
			}
			cfg.tokenFile = raw
			index++
		case "--allow-host":
			raw, ok := value(index)
			if !ok {
				return cfg, "--allow-host requires a host name", "Run `skillhub help serve`."
			}
			cfg.allowHosts = append(cfg.allowHosts, raw)
			index++
		case "--trust-private-network":
			cfg.trustPrivateNetwork = true
		default:
			return cfg, fmt.Sprintf("unknown MCP argument %q", args[index]), "Run `skillhub mcp serve --workspace <path>`."
		}
	}
	return cfg, "", ""
}

func isNetworkFlag(arg string) bool {
	switch arg {
	case "--addr", "--token-file", "--allow-host", "--trust-private-network":
		return true
	}
	return false
}

// runServeMCP handles `skillhub serve mcp`: stdio exactly as hosts launch it
// today, or the network listener when --addr is given.
func runServeMCP(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return serveMCP(ctx, args, true, stdout, stderr)
}

func serveMCP(ctx context.Context, args []string, allowNetwork bool, stdout, stderr io.Writer) int {
	cfg, reason, fix := parseMCPServeFlags(args, allowNetwork)
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, fix)
	}
	if cfg.addr != "" {
		return runMCPListener(ctx, cfg, stdout, stderr)
	}
	if cfg.trustPrivateNetwork || cfg.tokenFile != "" || len(cfg.allowHosts) > 0 {
		return writeInvalidRequest(stdout, stderr, false, "--trust-private-network, --token-file, and --allow-host only apply to the network listener", "Add --addr <host:port>, or drop those flags to serve an agent host over stdio.")
	}
	resolved, resErr := resolveWorkspace(cfg.workspacePath)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
	}
	if err := mcpserver.Serve(ctx, resolved, stderr, cfg.profile); err != nil {
		p := termui.New(stderr)
		p.Line("MCP server stopped because startup or transport validation failed; inspect workspace health with `skillhub doctor`.")
		return 1
	}
	return 0
}

func runMCPListener(ctx context.Context, cfg mcpServeConfig, stdout, stderr io.Writer) int {
	if cfg.profileSet && cfg.profile != mcpserver.ProfileRuntime {
		return writeInvalidRequest(stdout, stderr, false, fmt.Sprintf("--addr serves the runtime tools only, so --profile %s is not allowed", cfg.profile), "Drop --profile, or pass --profile runtime. Curation stays on this machine over stdio.")
	}
	if err := mcpserver.CheckListenAddress(cfg.addr, cfg.trustPrivateNetwork); err != nil {
		if errors.Is(err, mcpserver.ErrNonLoopbackAddress) {
			termui.New(stderr).Error(
				"The MCP listener will not start on "+cfg.addr+".",
				"That address can be reached from other machines and the listener speaks plain HTTP, so task text and skill content would cross the network unencrypted.",
				"Listen on 127.0.0.1 and reach it through an SSH tunnel, or, on a private network such as Tailscale, add --trust-private-network.",
			)
			return 2
		}
		if errors.Is(err, mcpserver.ErrWildcardAddress) {
			termui.New(stderr).Error(
				"The MCP listener will not start on "+cfg.addr+".",
				"That address listens on every network interface, including any that are not private, and --trust-private-network only vouches for one private network.",
				"Give the address of the private interface instead, for example your Tailscale address, as --addr <tailscale-ip>:7420.",
			)
			return 2
		}
		return writeInvalidRequest(stdout, stderr, false, err.Error(), "Run `skillhub serve mcp --addr 127.0.0.1:7420`.")
	}
	resolved, resErr := resolveWorkspace(cfg.workspacePath)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
	}
	tokenFile, code := ensureHubToken(resolved, cfg.tokenFile, stderr)
	if code != 0 {
		return code
	}
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		termui.New(stderr).Error(
			"The MCP listener could not listen on "+cfg.addr+".",
			err.Error(),
			"Stop the process that owns the port (for example `ss -ltnp`), or pass another --addr.",
		)
		return 1
	}
	defer listener.Close()

	err = mcpserver.ServeHTTP(ctx, listener, mcpserver.HTTPOptions{
		Workspace:           resolved,
		TokenFile:           tokenFile,
		TrustPrivateNetwork: cfg.trustPrivateNetwork,
		AllowHosts:          cfg.allowHosts,
		Diagnostics:         stderr,
		Ready: func(address net.Addr) {
			writeListenerBanner(stdout, listenerBanner{
				address:   address,
				workspace: resolved,
				tokenFile: tokenFile,
				trusted:   cfg.trustPrivateNetwork,
				hubURLs:   hubURLs(address),
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

// ensureHubToken resolves the token file (default: inside the workspace) and
// creates the token on first use. It never prints the token.
func ensureHubToken(workspaceRoot, explicit string, stderr io.Writer) (string, int) {
	path := hubtoken.DefaultPath(workspaceRoot)
	if explicit != "" {
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			termui.New(stderr).Error("The token file path is not usable.", err.Error(), "Pass an absolute --token-file path.")
			return "", 2
		}
		path = absolute
	}
	if _, err := hubtoken.Ensure(path); err != nil {
		writeHubTokenError(stderr, path, err)
		return "", 1
	}
	return path, 0
}

func writeHubTokenError(stderr io.Writer, path string, err error) {
	fix := "Check the file and its directory, or remove the file and run `skillhub token rotate --yes` to create a new token."
	if errors.Is(err, hubtoken.ErrInsecureMode) {
		fix = "Run `chmod 600 " + path + "`, or run `skillhub token rotate --yes` to replace it with a private file."
	}
	termui.New(stderr).Error("The hub token file "+path+" cannot be used.", err.Error(), fix)
}

type listenerBanner struct {
	address   net.Addr
	workspace string
	tokenFile string
	trusted   bool
	hubURLs   []string
}

// writeListenerBanner tells the user where the hub listens and the exact
// command to run on the other machine. It shows the token file path, never
// the token.
func writeListenerBanner(w io.Writer, banner listenerBanner) {
	p := termui.New(w)
	p.Line("Skill Hub MCP listener is running with the runtime tools only (skill_resolve, skill_get, skill_feedback).")
	if banner.trusted {
		p.Warning("Plain HTTP: --trust-private-network says this network is private (an SSH tunnel or a VPN such as Tailscale).")
		if tcp, ok := banner.address.(*net.TCPAddr); ok && tcp.IP.IsUnspecified() {
			p.Warning("This address listens on every network interface, including any that are not private. Prefer the address of the private interface.")
		}
	}
	p.Blank()
	transport := "loopback only; other machines reach it through an SSH tunnel"
	if banner.trusted {
		transport = "plain HTTP on a network you trust to be private"
	}
	p.Fields(
		termui.Field{Label: "Listening on", Value: banner.address.String()},
		termui.Field{Label: "Workspace", Value: banner.workspace},
		termui.Field{Label: "Token file", Value: banner.tokenFile},
		termui.Field{Label: "Transport", Value: transport},
	)
	if len(banner.hubURLs) > 1 {
		p.Heading("Hub URLs:")
		p.Bullets(banner.hubURLs...)
	}
	hub := banner.hubURLs[0]
	p.Heading("On machine A:")
	if !banner.trusted {
		host, port, _ := net.SplitHostPort(banner.address.String())
		p.Line("Open the tunnel (keep it running):")
		p.Command("ssh -N -L " + port + ":" + net.JoinHostPort(host, port) + " <user>@<this-machine>")
	}
	p.Line("Copy the token file to machine A (for example with scp) and pass its path there, then connect:")
	p.Command("skillhub connect --hub " + hub + " --token-file " + shellQuote(banner.tokenFile) + " --yes")
	if len(banner.hubURLs) > 1 {
		p.Line("Use another hub URL above if machine A reaches this machine by a different address.")
	}
	p.Blank()
	p.Line("The token itself is never printed; `skillhub token show` prints it on a terminal. Stop the listener with Ctrl-C.")
}

// hubURLs lists the base URLs at which the listener can be reached. A
// wildcard address expands to this machine's usable IPv4 addresses.
func hubURLs(address net.Addr) []string {
	tcp, ok := address.(*net.TCPAddr)
	if !ok {
		return []string{"http://" + address.String()}
	}
	port := strconv.Itoa(tcp.Port)
	if !tcp.IP.IsUnspecified() {
		return []string{"http://" + net.JoinHostPort(tcp.IP.String(), port)}
	}
	var urls []string
	if interfaces, err := web.DefaultInterfaces(); err == nil {
		for _, ip := range web.UsableIPv4(interfaces) {
			urls = append(urls, "http://"+net.JoinHostPort(ip.String(), port))
		}
	}
	return append(urls, "http://"+net.JoinHostPort("127.0.0.1", port))
}
