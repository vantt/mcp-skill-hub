package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runRebuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, err := workspaceFlag(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub rebuild --workspace <path>`.")
	}
	var progress app.ProgressSink
	if !jsonOutput {
		progress = func(event app.ProgressEvent) {
			fmt.Fprintf(stderr, "[%d/%d] %s\n", event.Completed, event.Total, event.Message)
		}
	}
	result, err := (app.CatalogService{}).BuildCatalogGenerationWithProgress(ctx, path, progress)
	return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
}
