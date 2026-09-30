package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, quiet, err := curationFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
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
	renderCurationHome(stdout, home, path)
	return 0
}

func renderCurationHome(writer io.Writer, home app.CurationHome, workspacePath string) {
	fmt.Fprintln(writer, home.Summary)
	if home.Workspace.Health == "valid" && home.Workspace.Index != "current" {
		fmt.Fprintln(writer, "Skill and source counts are unavailable until the search index is rebuilt.")
	} else if home.HomeSummary.ActiveSkills == 0 && home.HomeSummary.WatchingSources == 0 {
		fmt.Fprintln(writer, "No skills yet. Next: ask your agent 'create a skill for ...' or run `skillhub skill create ...`")
	} else {
		skillWord := "skills"
		if home.HomeSummary.ActiveSkills == 1 {
			skillWord = "skill"
		}
		sourceWord := "sources"
		if home.HomeSummary.WatchingSources == 1 {
			sourceWord = "source"
		}
		fmt.Fprintf(writer, "%d active %s; %d watched %s.\n", home.HomeSummary.ActiveSkills, skillWord, home.HomeSummary.WatchingSources, sourceWord)
	}
	gitState := "clean"
	if !home.Workspace.GitConfigured {
		gitState = "not configured"
	} else if home.Workspace.GitDirty {
		gitState = "has uncommitted changes"
	}
	fmt.Fprintf(writer, "Workspace %s; search index %s; Git %s.\n", home.Workspace.Health, home.Workspace.Index, gitState)
	if home.Error != nil {
		fmt.Fprintf(writer, "ERROR: %s\nWHY: %s\nFIX: %s\n", home.Error.Render.Error, home.Error.Render.Why, home.Error.Render.Fix)
	}
	if len(home.SuggestedActions) > 0 && home.SuggestedActions[0].Label != "" {
		label := home.SuggestedActions[0].Label
		if label == "Review uncommitted changes" {
			label = fmt.Sprintf("Review uncommitted changes: git -C %s add -A && git -C %s commit -m \"Update skills\"", workspacePath, workspacePath)
		}
		fmt.Fprintf(writer, "Recommended next: %s.\n", label)
	}
}

func runDiff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, err := workspaceFlag(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
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
		label := strings.ReplaceAll(group.Kind, "_", " ")
		fmt.Fprintf(stdout, "%s (%d):\n", label, group.Count)
		for _, file := range group.Files {
			statusWord := diffStatusWord(file.Status)
			fmt.Fprintf(stdout, "  %s  %s\n", statusWord, file.Path)
		}
	}
	fmt.Fprintf(stdout, "\nTo commit these changes, run:\n  git -C %s add -A && git -C %s commit -m \"...\"\n", path, path)
	return 0
}

func diffStatusWord(status string) string {
	switch status {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "deleted"
	case "??":
		return "untracked"
	case "R":
		return "renamed"
	case "U":
		return "conflicted"
	default:
		return status
	}
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
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return "", jsonOutput, quiet, resErr
	}
	return resolved, jsonOutput, quiet, nil
}
