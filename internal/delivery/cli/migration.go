package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
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
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		p.Line(result.Summary)
		var fields []termui.Field
		fields = append(fields,
			termui.Field{Label: "Source schema version", Value: fmt.Sprintf("%d", result.SourceSchemaVersion)},
			termui.Field{Label: "Target schema version", Value: fmt.Sprintf("%d", result.TargetSchemaVersion)},
		)
		if result.ProposalID != "" {
			fields = append(fields,
				termui.Field{Label: "Proposal", Value: result.ProposalID},
				termui.Field{Label: "Proposal digest", Value: result.ProposalDigest},
				termui.Field{Label: "Base version", Value: result.BaseCatalogSnapshot},
			)
		}
		if result.Receipt != nil {
			fields = append(fields, termui.Field{Label: "Operation", Value: result.Receipt.OperationID})
		}
		p.Fields(fields...)
		for _, change := range result.Changes {
			p.Raw(change.Diff)
		}
	})
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
