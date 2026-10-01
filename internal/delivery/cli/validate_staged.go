package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

// validateFlags parses flags for the validate command.
// Incompatible or unknown arguments fail before any workspace or Git access.
func validateFlags(args []string) (path string, jsonOutput, staged bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--staged":
			staged = true
		case "--workspace":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", jsonOutput, staged, fmt.Errorf("--workspace requires a path")
			}
			path = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", jsonOutput, staged, fmt.Errorf("unknown argument %q", args[i])
			}
			return "", jsonOutput, staged, fmt.Errorf("validate accepts no positional arguments, got %q", args[i])
		}
	}
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return "", jsonOutput, staged, resErr
	}
	return resolved, jsonOutput, staged, nil
}

// executeValidate runs either staged or working-tree validation through the app WorkspaceService.
func executeValidate(ctx context.Context, path string, staged bool) (app.Result, error) {
	if staged {
		return (app.WorkspaceService{}).ValidateStaged(ctx, path)
	}
	return (app.WorkspaceService{}).ValidateWorkspace(ctx, path)
}
