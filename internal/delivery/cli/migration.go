package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runMigrate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	workspacePath, target, yes, jsonOutput, err := migrationFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub migrate --workspace <path> [--to 1]` to preview, then add --yes after review.")
	}
	result, err := (app.MigrationService{}).Migrate(ctx, workspacePath, target, yes)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Run `skillhub doctor --workspace <path>` and review a fresh migration preview before retrying.")
	}
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(stdout, result.Summary)
	fmt.Fprintf(stdout, "Source schema version: %d\nTarget schema version: %d\n", result.SourceSchemaVersion, result.TargetSchemaVersion)
	if result.ProposalID != "" {
		fmt.Fprintf(stdout, "Proposal: %s\nProposal digest: %s\nBase version: %s\n", result.ProposalID, result.ProposalDigest, result.BaseCatalogSnapshot)
	}
	for _, change := range result.Changes {
		fmt.Fprint(stdout, change.Diff)
	}
	if result.Receipt != nil {
		fmt.Fprintf(stdout, "Operation: %s\n", result.Receipt.OperationID)
	}
	return 0
}

func migrationFlags(args []string) (workspacePath string, target int, yes, jsonOutput bool, err error) {
	target = 1
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--workspace":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", target, yes, jsonOutput, fmt.Errorf("--workspace requires a path")
			}
			index++
			workspacePath = args[index]
		case "--to":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", target, yes, jsonOutput, fmt.Errorf("--to requires a version")
			}
			index++
			value, parseErr := strconv.Atoi(args[index])
			if parseErr != nil || value < 1 {
				return "", target, yes, jsonOutput, fmt.Errorf("--to must be a positive canonical schema version")
			}
			target = value
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			return "", target, yes, jsonOutput, fmt.Errorf("unknown argument %q", args[index])
		}
	}
	resolved, resErr := resolveWorkspace(workspacePath)
	if resErr != nil {
		return "", target, yes, jsonOutput, resErr
	}
	return resolved, target, yes, jsonOutput, nil
}
