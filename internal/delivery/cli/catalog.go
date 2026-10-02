package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

func runRebuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, verbose, err := rebuildFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub rebuild --workspace <path>`.")
	}
	var progress app.ProgressSink
	if verbose && !jsonOutput {
		progress = func(event app.ProgressEvent) {
			p := termui.New(stderr)
			p.Line(fmt.Sprintf("[%d/%d] %s", event.Completed, event.Total, event.Message))
		}
	}
	result, err := (app.CatalogService{}).BuildCatalogGenerationWithProgress(ctx, path, progress)
	if err != nil || jsonOutput || result.Error != nil {
		return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
	}
	if !verbose {
		p := termui.New(stdout)
		p.Line("Search index rebuilt.")
		return 0
	}
	return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
}

func rebuildFlags(args []string) (path string, jsonOutput, verbose bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--verbose":
			verbose = true
		case "--workspace":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", jsonOutput, verbose, fmt.Errorf("--workspace requires a path")
			}
			path = args[i+1]
			i++
		default:
			return "", jsonOutput, verbose, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return "", jsonOutput, verbose, resErr
	}
	return resolved, jsonOutput, verbose, nil
}
