package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver"
)

func runMCP(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "serve" {
		return writeInvalidRequest(stdout, stderr, false, "mcp requires the serve subcommand", "Run `skillhub mcp serve --workspace <path>`.")
	}
	workspacePath := ""
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--workspace":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return writeInvalidRequest(stdout, stderr, false, "--workspace requires a path", "Run `skillhub mcp serve --workspace <path>`.")
			}
			workspacePath = args[index+1]
			index++
		default:
			return writeInvalidRequest(stdout, stderr, false, fmt.Sprintf("unknown MCP argument %q", args[index]), "Run `skillhub mcp serve --workspace <path>`.")
		}
	}
	resolved, resErr := resolveWorkspace(workspacePath)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
	}
	workspacePath = resolved
	if err := mcpserver.Serve(ctx, workspacePath, stderr); err != nil {
		p := termui.New(stderr)
		p.Line("MCP server stopped because startup or transport validation failed; inspect workspace health with `skillhub doctor`.")
		return 1
	}
	return 0
}
