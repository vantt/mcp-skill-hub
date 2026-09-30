package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runInit(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, yes, verbose, force, err := initFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub init <path>` to preview, then add `--yes` to apply.")
	}
	result, err := (app.WorkspaceService{}).Init(path, yes, force)
	compact := err == nil && !jsonOutput && !verbose && result.Error == nil &&
		(result.Status == app.StatusApplied || result.Status == app.StatusReady)
	if !compact {
		return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
	}
	absolute, absErr := filepath.Abs(path)
	if absErr != nil {
		absolute = path
	}
	renderInitSummary(stdout, absolute, result)
	return 0
}

// renderInitSummary prints the short first-run summary. Generation IDs, digests,
// and row counts stay available through --verbose and --json.
func renderInitSummary(stdout io.Writer, workspacePath string, result app.Result) {
	created := false
	var hosts []string
	for _, item := range result.Items {
		if item.ID == "workspace_missing" {
			created = true
		}
		// Host item IDs are host_<host>_<change-kind>; neither part contains "_".
		if parts := strings.Split(item.ID, "_"); len(parts) == 3 && parts[0] == "host" && !slices.Contains(hosts, parts[1]) {
			hosts = append(hosts, parts[1])
		}
	}
	if created {
		fmt.Fprintf(stdout, "Workspace created at %s.\n", workspacePath)
	} else {
		fmt.Fprintf(stdout, "Workspace ready at %s.\n", workspacePath)
	}
	if len(hosts) > 0 {
		fmt.Fprintf(stdout, "Agent connection for this workspace written: %s.\n", strings.Join(hosts, ", "))
	} else {
		fmt.Fprintln(stdout, "Agent connection for this workspace is already in place.")
	}
	fmt.Fprintf(stdout, "\nNext:\n")
	fmt.Fprintf(stdout, "  1. Commit the workspace: git -C %s add -A && git -C %s commit -m \"Initialize Skill Hub workspace\"\n", workspacePath, workspacePath)
	fmt.Fprintf(stdout, "  2. Connect your project: cd <your project> && skillhub connect --workspace %s --yes\n", workspacePath)
	fmt.Fprintf(stdout, "     (add -g instead to connect every project: skillhub connect -g --workspace %s --yes)\n", workspacePath)
	fmt.Fprintln(stdout, "  3. Open your agent there and ask: \"curate my Skill Hub\"")
}

func runValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if errors.Is(ctx.Err(), context.Canceled) {
		return writeWorkspaceResult(app.Result{}, context.Canceled, stdout, stderr, hasJSONFlag(args))
	}
	path, jsonOutput, err := workspaceFlag(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub validate --workspace <path>`.")
	}
	result, err := (app.WorkspaceService{}).ValidateWorkspace(ctx, path)
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if writeErr := writeJSON(stdout, result); writeErr != nil {
			fmt.Fprintln(stderr, writeErr)
			return 1
		}
		if result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	if result.Status == app.StatusError {
		fmt.Fprintln(stderr, "Workspace validation failed:")
		for _, item := range result.Items {
			fmt.Fprintf(stderr, "- %s\n  FIX: %s\n", item.Summary, item.Impact)
		}
		return 2
	}
	fmt.Fprintln(stdout, result.Summary)
	return 0
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, fix, yes, err := doctorFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
		}
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

func initFlags(args []string) (path string, jsonOutput, yes, verbose, force bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--yes":
			yes = true
		case "--verbose":
			verbose = true
		case "--force":
			force = true
		default:
			if strings.HasPrefix(arg, "-") || path != "" {
				return "", jsonOutput, yes, verbose, force, fmt.Errorf("init requires exactly one workspace path")
			}
			path = arg
		}
	}
	if path == "" {
		return "", jsonOutput, yes, verbose, force, fmt.Errorf("init requires exactly one workspace path")
	}
	return path, jsonOutput, yes, verbose, force, nil
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
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return "", jsonOutput, resErr
	}
	return resolved, jsonOutput, nil
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
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return "", jsonOutput, fix, yes, resErr
	}
	return resolved, jsonOutput, fix, yes, nil
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
	var resErr *WorkspaceResolutionError
	if errors.As(cause, &resErr) {
		return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
	}
	why := cause.Error()
	fix := "Run `skillhub doctor --workspace <path>` to inspect safe remediation, then use `--fix --yes` only after review."
	if strings.Contains(why, "canonical validation failed") {
		fix = "Correct the content errors in the affected file (or use `skillhub skill edit <id>`), then run `skillhub validate`."
	}
	result := app.ErrorResult(&app.Error{
		Code: app.ErrorWorkspaceInvalid,
		Render: app.ErrorRender{
			Error: "The workspace is invalid.",
			Why:   why,
			Fix:   fix,
		},
	})
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
