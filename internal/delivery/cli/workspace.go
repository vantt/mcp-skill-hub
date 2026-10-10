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
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

func runInit(args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, yes, verbose, force, err := initFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub init [path]` to preview, then add `--yes` to apply.")
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
	p := termui.New(stdout)
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
		p.Line(fmt.Sprintf("Workspace created at %s.", workspacePath))
	} else {
		p.Line(fmt.Sprintf("Workspace ready at %s.", workspacePath))
	}
	if len(hosts) > 0 {
		p.Line(fmt.Sprintf("Agent connection for this workspace written: %s.", strings.Join(hosts, ", ")))
	} else {
		p.Line("Agent connection for this workspace is already in place.")
	}
	p.Blank()
	p.Line("Next:")
	p.Line(fmt.Sprintf("  1. Commit the workspace: git -C %s add -A && git -C %s commit -m \"Initialize Skill Hub workspace\"", workspacePath, workspacePath))
	p.Line(fmt.Sprintf("  2. Connect another project: cd <project> && skillhub connect --workspace %s --yes", workspacePath))
	p.Line(fmt.Sprintf("     (add -g instead to connect every project: skillhub connect -g --workspace %s --yes)", workspacePath))
	p.Line("  3. Open your agent there and ask: \"curate my Skill Hub\"")
}

func runValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if errors.Is(ctx.Err(), context.Canceled) {
		return writeWorkspaceResult(app.Result{}, context.Canceled, stdout, stderr, hasJSONFlag(args))
	}
	path, jsonOutput, staged, err := validateFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub validate [--staged] [--workspace <path>]`.")
	}
	result, err := executeValidate(ctx, path, staged)
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if writeErr := writeJSON(stdout, result); writeErr != nil {
			p := termui.New(stderr)
			p.Line(writeErr.Error())
			return 1
		}
		if result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	if result.Status == app.StatusError {
		p := termui.New(stderr)
		if result.Error != nil && len(result.Items) == 0 {
			p.Error(result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
			return 2
		}
		p.Line("Workspace validation failed:")
		for _, item := range result.Items {
			p.Line(fmt.Sprintf("- %s\n  FIX: %s", item.Summary, item.Impact))
		}
		return 2
	}
	p := termui.New(stdout)
	p.Line(result.Summary)
	for _, w := range result.Warnings {
		p.Line(fmt.Sprintf("WARN %s %s", w.Code, w.Summary))
	}
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
	code := writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
	if code != 0 || jsonOutput {
		return code
	}
	p := termui.New(stdout)
	for _, action := range result.SuggestedActions {
		p.Line("FIX: " + action.Command)
	}
	if p.Err() != nil {
		return 1
	}
	return 0
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
				return "", jsonOutput, yes, verbose, force, fmt.Errorf("init accepts at most one workspace path")
			}
			path = arg
		}
	}
	if path == "" {
		path = "."
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
				p := termui.New(stderr)
				p.Line(writeErr.Error())
				return 1
			}
		} else {
			p := termui.New(stderr)
			p.Error(cancelled.Error.Render.Error, cancelled.Error.Render.Why, cancelled.Error.Render.Fix)
		}
		return 130
	}
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if writeErr := writeJSON(stdout, result); writeErr != nil {
			p := termui.New(stderr)
			p.Error("Unable to write the command result.", "Output destination failed.", "Check the output destination and retry.")
			return 1
		}
		if result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	if result.Error != nil {
		p := termui.New(stderr)
		p.Error(result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
		return 2
	}
	p := termui.New(stdout)
	p.Line(result.Summary)
	var bullets []string
	for _, item := range result.Items {
		bullets = append(bullets, item.Summary)
	}
	p.Bullets(bullets...)
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
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
			p := termui.New(stderr)
			p.Line(err.Error())
			return 1
		}
		return 2
	}
	p := termui.New(stderr)
	p.Error(result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	return 2
}
