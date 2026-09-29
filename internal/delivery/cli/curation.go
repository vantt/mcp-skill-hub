package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, quiet, err := curationFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub status [--workspace <path>] [--json|--quiet]`.")
	}
	home, err := (app.CurationService{}).GetCurationHome(ctx, path)
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, home); err != nil {
			fmt.Fprintf(stderr, "ERROR: Unable to write the command result.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
			return 1
		}
		return 0
	}
	if quiet {
		fmt.Fprintln(stdout, home.Status)
		return 0
	}
	renderCurationHome(stdout, home)
	return 0
}

func renderCurationHome(writer io.Writer, home app.CurationHome) {
	fmt.Fprintln(writer, home.Summary)
	fmt.Fprintf(writer, "%d active skills; %d watched sources.\n", home.HomeSummary.ActiveSkills, home.HomeSummary.WatchingSources)
	gitState := "clean"
	if !home.Workspace.GitConfigured {
		gitState = "not configured"
	} else if home.Workspace.GitDirty {
		gitState = "has uncommitted changes"
	}
	fmt.Fprintf(writer, "Workspace %s; search index %s; Git %s.\n", home.Workspace.Health, home.Workspace.Index, gitState)
	var unsupported []string
	for _, category := range home.Categories {
		if category.Availability == app.AvailabilityNotConfigured {
			unsupported = append(unsupported, strings.ReplaceAll(category.Kind, "_", " ")+": 0 (not configured)")
		}
	}
	if len(unsupported) > 0 {
		fmt.Fprintln(writer, strings.Join(unsupported, "; ")+".")
	}
	if home.Error != nil {
		fmt.Fprintf(writer, "ERROR: %s\nWHY: %s\nFIX: %s\n", home.Error.Render.Error, home.Error.Render.Why, home.Error.Render.Fix)
	}
	fmt.Fprintf(writer, "Recommended next: %s.\n", home.SuggestedActions[0].Label)
}

func runDiff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, err := workspaceFlag(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub diff --workspace <path>`.")
	}
	result, err := (app.WorkspaceService{}).GetCurationDiff(ctx, path)
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(stdout, result.Summary)
	for _, group := range result.Groups {
		fmt.Fprintf(stdout, "%s (%d):\n", strings.ReplaceAll(group.Kind, "_", " "), group.Count)
		for _, file := range group.Files {
			fmt.Fprintf(stdout, "  %s %s\n", file.Status, strconv.Quote(file.Path))
		}
	}
	return 0
}

func curationFlags(args []string) (path string, jsonOutput, quiet bool, err error) {
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--quiet":
			quiet = true
		case "--workspace":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", jsonOutput, quiet, fmt.Errorf("--workspace requires a path")
			}
			path = args[index+1]
			index++
		default:
			return "", jsonOutput, quiet, fmt.Errorf("unknown argument %q", args[index])
		}
	}
	if jsonOutput && quiet {
		return "", jsonOutput, quiet, fmt.Errorf("--json and --quiet cannot be used together")
	}
	return path, jsonOutput, quiet, nil
}
