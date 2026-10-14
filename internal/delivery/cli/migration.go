package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
)

func runMigrate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	workspacePath, target, yes, jsonOutput, noCommit, err := migrationFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub maintain migrate --workspace <path> [--to <version>]` to preview, then add --yes after review.")
	}
	retry := "Run `skillhub doctor` in the hub and review a fresh `skillhub maintain migrate` preview before retrying."
	migrate := func(apply bool, expected string) (app.MigrationResult, error) {
		return (app.MigrationService{ExpectedDigest: expected}).Migrate(commitContext(ctx, noCommit, stdout), workspacePath, target, apply)
	}
	result, err := migrate(yes, "")
	if err != nil {
		return writeMigrationError(stdout, stderr, jsonOutput, err, retry)
	}
	if !yes && result.ProposalID != "" {
		apply, code := confirmInPlace(stdout, jsonOutput, true, func(interactive bool) int {
			return writeMigrationResult(stdout, stderr, jsonOutput, result, rerunCommand("skillhub maintain migrate", args), interactive)
		})
		if !apply {
			return code
		}
		if result, err = migrate(true, result.ProposalDigest); err != nil {
			return writeMigrationError(stdout, stderr, jsonOutput, err, retry)
		}
	}
	return writeMigrationResult(stdout, stderr, jsonOutput, result, "", false)
}

// writeMigrationResult prints a migration preview or its applied result. A
// preview with rerun set ends with the hint lines for applying it.
func writeMigrationResult(stdout, stderr io.Writer, jsonOutput bool, result app.MigrationResult, rerun string, interactive bool) int {
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		p.Line(result.Summary)
		if result.Receipt != nil {
			writeCommitLines(p, result.Receipt.Commit)
		}
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
		if rerun != "" && result.Receipt == nil {
			previewTail(p, rerun, true, interactive)
		}
	})
}

func migrationFlags(args []string) (workspacePath string, target int, yes, jsonOutput, noCommit bool, err error) {
	target = 0
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--workspace":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", target, yes, jsonOutput, noCommit, fmt.Errorf("--workspace requires a path")
			}
			index++
			workspacePath = args[index]
		case "--to":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", target, yes, jsonOutput, noCommit, fmt.Errorf("--to requires a version")
			}
			index++
			value, parseErr := strconv.Atoi(args[index])
			if parseErr != nil || value < 1 {
				return "", target, yes, jsonOutput, noCommit, fmt.Errorf("--to must be a positive canonical schema version")
			}
			target = value
		case "--yes":
			yes = true
		case "--no-commit":
			noCommit = true
		case "--json":
			jsonOutput = true
		default:
			return "", target, yes, jsonOutput, noCommit, fmt.Errorf("unknown argument %q", args[index])
		}
	}
	resolved, resErr := resolveWorkspace(workspacePath)
	if resErr != nil {
		return "", target, yes, jsonOutput, noCommit, resErr
	}
	return resolved, target, yes, jsonOutput, noCommit, nil
}

func writeMigrationError(stdout, stderr io.Writer, jsonOutput bool, err error, retry string) int {
	if refusal := app.UncommittedEditsError(err); refusal != nil {
		writeStructuredSkillError(stdout, stderr, jsonOutput, refusal)
		return 2
	}
	return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), retry)
}
