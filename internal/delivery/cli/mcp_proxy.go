package cli

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

// hubTokenEnv names the environment variable that may carry the hub token on
// the client machine when no --token-file is given.
const hubTokenEnv = "SKILLHUB_HUB_TOKEN"

type mcpProxyConfig struct {
	hubURL    string
	tokenFile string
}

func parseMCPProxyFlags(args []string) (mcpProxyConfig, string) {
	var cfg mcpProxyConfig
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--hub", "--token-file":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return cfg, args[index] + " requires a value"
			}
			if args[index] == "--hub" {
				cfg.hubURL = args[index+1]
			} else {
				cfg.tokenFile = args[index+1]
			}
			index++
		default:
			return cfg, "unknown argument \"" + args[index] + "\""
		}
	}
	if cfg.hubURL == "" {
		return cfg, "mcp proxy requires --hub <url>"
	}
	return cfg, ""
}

// runMCPProxy runs the stdio proxy that `skillhub connect --hub` registers for
// an agent host. Standard output carries the MCP protocol, so every message
// for the user goes to stderr. The token is never printed.
func runMCPProxy(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, reason := parseMCPProxyFlags(args)
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, "Run `skillhub mcp proxy --hub <url> --token-file <path>`; `skillhub connect --hub` writes this for your agent host.")
	}
	hubURL, err := mcpserver.NormalizeHubURL(cfg.hubURL)
	if err != nil {
		termui.New(stderr).Error("The hub URL cannot be used.", err.Error(), "Pass the base URL the hub printed when it started, for example --hub http://127.0.0.1:7420.")
		return 2
	}
	token, code := proxyToken(cfg.tokenFile, stderr)
	if code != 0 {
		return code
	}
	stateDir, err := skillruntime.ClientStateDir()
	if err != nil {
		termui.New(stderr).Error("This machine has no directory for cached skills.", err.Error(), "Set XDG_STATE_HOME to a writable directory and start again.")
		return 1
	}
	proxy, err := mcpserver.NewProxy(ctx, mcpserver.ProxyOptions{
		HubURL:   hubURL,
		Token:    token,
		StateDir: stateDir,
		Warn:     lockedWarning(stderr),
	})
	if err != nil {
		termui.New(stderr).Error("The Skill Hub proxy could not start.", err.Error(), "Check that "+skillruntime.RemoteCacheDir(stateDir)+" is writable, then start again.")
		return 1
	}
	writeProxyBanner(stderr, hubURL, proxy.CacheDir())
	if err := proxy.Run(ctx, &mcp.StdioTransport{MaxLineLength: mcpserver.MaxFrameBytes}); err != nil {
		termui.New(stderr).Error("The Skill Hub proxy stopped.", err.Error(), "Restart the agent session; run `skillhub help mcp` for the proxy's options.")
		return 1
	}
	return 0
}

// lockedWarning writes the proxy's warnings, which may come from concurrent
// tool calls, one whole message at a time.
func lockedWarning(stderr io.Writer) func(string) {
	var mu sync.Mutex
	printer := termui.New(stderr)
	return func(message string) {
		mu.Lock()
		defer mu.Unlock()
		printer.Warning(message)
	}
}

// writeProxyBanner is the one start-up line, on stderr.
func writeProxyBanner(stderr io.Writer, hubURL, cacheDir string) {
	termui.New(stderr).Fields(
		termui.Field{Label: "Skill Hub proxy", Value: "forwarding skill_resolve, skill_get and skill_feedback to " + hubURL},
		termui.Field{Label: "Skill cache", Value: cacheDir},
	)
}

// proxyToken reads the token from --token-file, or else from
// SKILLHUB_HUB_TOKEN. Neither the token nor its length is ever printed.
func proxyToken(tokenFile string, stderr io.Writer) (string, int) {
	if tokenFile != "" {
		path, err := filepath.Abs(tokenFile)
		if err != nil {
			termui.New(stderr).Error("The token file path is not usable.", err.Error(), "Pass an absolute --token-file path.")
			return "", 2
		}
		token, err := hubtoken.Read(path)
		if err != nil {
			fix := "Copy the token file from the hub machine again (it is printed in the hub's start-up message) and pass its path with --token-file."
			switch {
			case errors.Is(err, hubtoken.ErrInsecureMode):
				fix = "Run `chmod 600 " + shellQuote(path) + "`: the token must be readable by you only."
			case errors.Is(err, fs.ErrNotExist):
				fix = "Copy the token file from the hub machine (for example with scp) to " + path + ", or pass the right path with --token-file."
			}
			termui.New(stderr).Error("The hub token file "+path+" cannot be used.", err.Error(), fix)
			return "", 2
		}
		return token, 0
	}
	token := strings.TrimSpace(os.Getenv(hubTokenEnv))
	if token == "" {
		termui.New(stderr).Error(
			"No hub token was given.",
			"Every request to a remote hub needs the hub's shared token.",
			"Pass --token-file <path> with a copy of the hub's token file, or set "+hubTokenEnv+".",
		)
		return "", 2
	}
	if !headerSafeToken(token) {
		termui.New(stderr).Error(
			hubTokenEnv+" does not hold a usable token.",
			"A token is one line of letters, digits and the characters - . _ ~ + / =.",
			"Set "+hubTokenEnv+" to the content of the hub's token file, or pass --token-file <path>.",
		)
		return "", 2
	}
	return token, 0
}

func headerSafeToken(token string) bool {
	if len(token) < 16 || len(token) > 1024 {
		return false
	}
	for _, character := range token {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case strings.ContainsRune("-._~+/=", character):
		default:
			return false
		}
	}
	return true
}
