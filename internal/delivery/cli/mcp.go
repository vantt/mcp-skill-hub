package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/delivery/mcpserver"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
)

// runMCP handles the hidden `skillhub mcp` commands. `skillhub mcp serve` is
// the stdio server that agent host registrations launch; it keeps its name
// and flags so existing registrations work, and has no network flags
// (`skillhub serve` is the network listener). `skillhub mcp proxy` is the
// stdio bridge to a remote hub.
func runMCP(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "proxy" {
		return runMCPProxy(ctx, args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "serve" {
		return writeInvalidRequest(stdout, stderr, false, "mcp requires the serve subcommand", "Run `skillhub mcp serve --workspace <path>`.")
	}
	cfg, reason, fix := parseMCPServeFlags(args[1:])
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, fix)
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

type mcpServeConfig struct {
	workspacePath string
	profile       mcpserver.Profile
}

func parseMCPServeFlags(args []string) (mcpServeConfig, string, string) {
	cfg := mcpServeConfig{profile: mcpserver.ProfileAll}
	value := func(index int) (string, bool) {
		if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
			return "", false
		}
		return args[index+1], true
	}
	for index := 0; index < len(args); index++ {
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
			cfg.profile = profile
			index++
		case "--workspace":
			raw, ok := value(index)
			if !ok {
				return cfg, "--workspace requires a path", "Run `skillhub mcp serve --workspace <path>`."
			}
			cfg.workspacePath = raw
			index++
		case "--addr", "--token-file", "--allow-host":
			return cfg, fmt.Sprintf("unknown MCP argument %q", args[index]), "`skillhub mcp serve` is the stdio server agents start; run `skillhub serve` for agents on other machines."
		default:
			return cfg, fmt.Sprintf("unknown MCP argument %q", args[index]), "Run `skillhub mcp serve --workspace <path>`."
		}
	}
	return cfg, "", ""
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
	fix := "Check the file and its directory, or remove the file and run `skillhub token rotate` to create a new token."
	if errors.Is(err, hubtoken.ErrInsecureMode) {
		fix = "Run `chmod 600 " + path + "`, or run `skillhub token rotate` to replace it with a private file."
	}
	termui.New(stderr).Error("The hub token file "+path+" cannot be used.", err.Error(), fix)
}
