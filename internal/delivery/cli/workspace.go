package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runInit(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, yes, err := initFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub init <path>` to preview, then add `--yes` to apply.")
	}
	result, err := (app.WorkspaceService{}).Init(path, yes)
	return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
}

func runValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, err := workspaceFlag(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub validate --workspace <path>`.")
	}
	result, err := (app.WorkspaceService{}).ValidateWorkspace(ctx, path)
	return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, fix, yes, err := doctorFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub doctor --workspace <path>` or add `--fix --yes` after preview.")
	}
	var result app.Result
	if fix {
		result, err = (app.WorkspaceService{}).DoctorFix(path, yes)
	} else {
		result, err = (app.WorkspaceService{}).Doctor(path)
	}
	return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
}

func initFlags(args []string) (path string, jsonOutput, yes bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--yes":
			yes = true
		default:
			if strings.HasPrefix(arg, "-") || path != "" {
				return "", jsonOutput, yes, fmt.Errorf("init requires exactly one workspace path")
			}
			path = arg
		}
	}
	if path == "" {
		return "", jsonOutput, yes, fmt.Errorf("init requires exactly one workspace path")
	}
	return path, jsonOutput, yes, nil
}

func workspaceFlag(args []string) (path string, jsonOutput bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--workspace":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", jsonOutput, fmt.Errorf("--workspace requires a path")
			}
			path = args[i+1]
			i++
		default:
			return "", jsonOutput, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if path == "" {
		return "", jsonOutput, fmt.Errorf("--workspace is required for non-interactive use")
	}
	return path, jsonOutput, nil
}

func doctorFlags(args []string) (path string, jsonOutput, fix, yes bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--fix":
			fix = true
		case "--yes":
			yes = true
		case "--workspace":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", jsonOutput, fix, yes, fmt.Errorf("--workspace requires a path")
			}
			path = args[i+1]
			i++
		default:
			return "", jsonOutput, fix, yes, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if yes && !fix {
		return "", jsonOutput, fix, yes, fmt.Errorf("--yes requires --fix")
	}
	if path == "" {
		return "", jsonOutput, fix, yes, fmt.Errorf("--workspace is required for non-interactive use")
	}
	return path, jsonOutput, fix, yes, nil
}

func writeWorkspaceResult(result app.Result, err error, stdout, stderr io.Writer, jsonOutput bool) int {
	if errors.Is(err, context.Canceled) {
		cancelled := app.ErrorResult(app.NewOperationCancelledError())
		if jsonOutput {
			if writeErr := writeJSON(stdout, cancelled); writeErr != nil {
				fmt.Fprintln(stderr, writeErr)
				return 1
			}
		} else {
			fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", cancelled.Error.Render.Error, cancelled.Error.Render.Why, cancelled.Error.Render.Fix)
		}
		return 130
	}
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, "ERROR: Unable to write the command result.\nWHY: Output destination failed.\nFIX: Check the output destination and retry.")
			return 1
		}
		if result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	if result.Error != nil {
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
		return 2
	}
	fmt.Fprintln(stdout, result.Summary)
	for _, item := range result.Items {
		fmt.Fprintf(stdout, "- %s\n", item.Summary)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "WARNING: %s\n", warning.Summary)
	}
	return 0
}

func writeInvalidWorkspace(stdout, stderr io.Writer, jsonOutput bool, cause error) int {
	result := app.ErrorResult(app.NewWorkspaceInvalidError(cause.Error()))
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 2
	}
	fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	return 2
}
