package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
)

func runInit(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	dir, err := os.Getwd()
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "The current folder cannot be determined: "+err.Error(), "Run `skillhub init` from the folder that becomes the hub.")
	}
	return runInitIn(ctx, dir, args, stdout, stderr)
}

// runInitIn runs init for dir, which stands for the current folder. Tests
// call it directly so they never change the process working directory.
func runInitIn(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) int {
	if errors.Is(ctx.Err(), context.Canceled) {
		return writeWorkspaceResult(app.Result{}, context.Canceled, stdout, stderr, hasJSONFlag(args))
	}
	source, jsonOutput, dryRun, verbose, err := initFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub init` in an empty folder, or `skillhub init <git-url>` to start from a repository; add `--dry-run` to see what it would do.")
	}
	var options app.InitOptions
	options.DryRun = dryRun
	if source != "" {
		classified, problem := app.ClassifyInitSource(source)
		if problem != nil {
			return writeWorkspaceResult(app.ErrorResult(problem), nil, stdout, stderr, jsonOutput)
		}
		options.Source = classified
	}
	result, err := (app.WorkspaceService{}).Init(dir, options)
	report, _ := result.Details[app.InitDetailsKey].(*app.InitReport)
	code := 0
	if report != nil && report.Remote != nil && report.Remote.PushError != "" {
		code = 1
	}
	switch {
	case err == nil && result.Error != nil && !jsonOutput:
		return writeInitError(stderr, result)
	case dryRun:
		return writeDryRunResult(result, err, stdout, stderr, jsonOutput)
	case err != nil || jsonOutput || report == nil:
		written := writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
		if err == nil && result.Error != nil && result.Error.Code == app.ErrorGitFailed && written == 2 {
			// A Git failure is a runtime failure, not a refused request.
			return 1
		}
		if written != 0 {
			return written
		}
		if err == nil && result.Error == nil && result.Status != app.StatusApplied {
			// The folder is a hub, but init stopped before it could store it.
			p := termui.New(stdout)
			for _, action := range result.SuggestedActions {
				p.Line("FIX: " + action.Label + ":")
				p.Command(action.Command)
			}
			return 2
		}
		return code
	case verbose:
		if written := writeWorkspaceResult(result, nil, stdout, stderr, false); written != 0 {
			return written
		}
	}
	p := termui.New(stdout)
	renderInitSummary(p, result, report)
	if p.Err() != nil {
		return 1
	}
	return code
}

// writeInitError prints a refusal or Git failure of init: what happened, Git's
// own message kept as it is, and the one command to run instead.
func writeInitError(stderr io.Writer, result app.Result) int {
	p := termui.New(stderr)
	render := result.Error.Render
	if result.Error.Code == app.ErrorGitFailed {
		p.Line("ERROR: " + render.Error)
		for _, line := range strings.Split(strings.TrimRight(render.Why, "\n"), "\n") {
			p.Raw("  " + line + "\n")
		}
		p.Line(render.Fix)
		return 1
	}
	p.Error(render.Error, render.Why, render.Fix)
	for _, action := range result.SuggestedActions {
		p.Command(action.Command)
	}
	return 2
}

// renderInitSummary prints what init changed and the one next command.
// Generation IDs and row counts stay available through --verbose and --json.
func renderInitSummary(p *termui.Printer, result app.Result, report *app.InitReport) {
	// Lines that hold a path or URL are not wrapped, so they can be copied whole.
	p.Raw(result.Summary + "\n")
	p.Line(connectionSummaryLine(result))
	if remote := report.Remote; remote != nil && remote.Fetched {
		p.Raw(fmt.Sprintf("Fetched the hub from %s (branch %s); origin is set.\n", remote.URL, remote.Branch))
	}
	if report.Commit != "" {
		p.Line(fmt.Sprintf("First commit %s: %q.", shortCommit(report.Commit), "Initialize Skill Hub hub"))
		p.Line("Git hooks and commit signing were skipped for this first commit.")
		if report.FallbackIdentity {
			p.Line("Git has no user name or email set, so this commit is by Skill Hub <skillhub@localhost>.")
		}
	}
	if remote := report.Remote; remote != nil && !remote.Fetched {
		switch {
		case remote.PushError != "":
			p.Blank()
			p.Raw("The hub is ready here, but the push to " + remote.URL + " failed:\n")
			for _, line := range strings.Split(remote.PushError, "\n") {
				p.Raw("  " + line + "\n")
			}
			p.Line("Fix what Git reports, then run the same `skillhub init` command again to push.")
		case remote.Pushed != "":
			p.Raw(fmt.Sprintf("Pushed %s to %s (branch %s); origin is set.\n", shortCommit(remote.Pushed), remote.URL, remote.Branch))
		default:
			p.Raw(fmt.Sprintf("Connected to %s (branch %s); origin is set.\n", remote.URL, remote.Branch))
		}
	}
	if len(report.GitignoreAdded) > 0 {
		p.Line(".gitignore: added " + strings.Join(report.GitignoreAdded, " ") + " (not committed; commit .gitignore when you are ready).")
	}
	for _, path := range report.TrackedMachineFiles {
		p.Line("Git tracks " + path + ", which holds paths of this machine. Stop tracking it (the file stays on disk):")
		p.Command("git rm --cached " + shellquote.Arg(path))
	}
	for _, warning := range result.Warnings {
		// Only what init found about this hub; routine notes of the steps it
		// ran stay in --verbose and --json.
		if warning.Code == "git_ssh_command" || warning.Code == "hub_tracks_agent_config" {
			p.Warning(warning.Summary)
		}
	}
	if report.Stored {
		p.Line("This is now the hub of this machine; `skillhub connect` uses it.")
	} else if report.StoreError != "" {
		p.Warning("The hub path could not be recorded for `skillhub connect`: " + report.StoreError)
	}
	p.Blank()
	p.Next("connect a project to this hub:", report.Next)
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// connectionSummaryLine names the agents whose connection files a result
// wrote, in one wording shared by init and connect:
// "Agent connection written: claude-code, codex-cli, gemini-cli."
func connectionSummaryLine(result app.Result) string {
	var hosts []string
	for _, item := range result.Items {
		// The shared Agent Skills folder belongs to no agent.
		if host, _, _, ok := parseHostItemID(item.ID); ok && hostItemWritten(item) && host != app.SharedSkillsItemKey && !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return "Agent connection already in place."
	}
	return "Agent connection written: " + strings.Join(hosts, ", ") + "."
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
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub maintain validate [--staged] [--workspace <path>]`.")
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

// initFlags parses init's flags. init takes no folder: it acts on the
// current folder; its one optional argument is a Git repository.
func initFlags(args []string) (source string, jsonOutput, dryRun, verbose bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--dry-run":
			dryRun = true
		case "--verbose":
			verbose = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", jsonOutput, dryRun, verbose, fmt.Errorf("unknown argument %q", arg)
			}
			if source != "" {
				return "", jsonOutput, dryRun, verbose, fmt.Errorf("init accepts at most one Git URL")
			}
			source = arg
		}
	}
	return source, jsonOutput, dryRun, verbose, nil
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

// writeDryRunResult prints what a --dry-run would change under a "Would
// change:" heading. Errors and --json output are the same as without
// --dry-run.
func writeDryRunResult(result app.Result, err error, stdout, stderr io.Writer, jsonOutput bool) int {
	if err != nil || jsonOutput || result.Error != nil {
		return writeWorkspaceResult(result, err, stdout, stderr, jsonOutput)
	}
	p := termui.New(stdout)
	p.Line(result.Summary)
	var bullets []string
	for _, item := range result.Items {
		bullets = append(bullets, item.Summary)
	}
	if len(bullets) > 0 {
		p.Line("Would change:")
		p.Bullets(bullets...)
	}
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	return printerCode(p)
}

func writeInvalidWorkspace(stdout, stderr io.Writer, jsonOutput bool, cause error) int {
	var resErr *WorkspaceResolutionError
	if errors.As(cause, &resErr) {
		return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
	}
	why := cause.Error()
	fix := "Run `skillhub doctor` in the hub to see what needs repair, then `skillhub doctor --fix` to repair what it can."
	if strings.Contains(why, "canonical validation failed") {
		fix = "Correct the content errors in the affected file (or use `skillhub skill edit <id>`), then run `skillhub maintain validate`."
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
