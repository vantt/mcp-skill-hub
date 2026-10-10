package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
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
			p := termui.New(stderr)
			p.Error("Unable to write the command result.", err.Error(), "Check the output destination and retry.")
			return 1
		}
		return 0
	}
	if quiet {
		p := termui.New(stdout)
		p.Line(string(home.Status))
		return 0
	}
	renderCurationHome(stdout, home, path)
	return 0
}

func renderCurationHome(writer io.Writer, home app.CurationHome, workspacePath string) {
	p := termui.New(writer)
	var fields []termui.Field
	if workspacePath != "" {
		fields = append(fields, termui.Field{Label: "Workspace", Value: workspacePath})
	}
	fields = append(fields, termui.Field{Label: "Status", Value: home.Summary})

	gitState := "clean"
	if !home.Workspace.GitConfigured {
		gitState = "not configured"
	} else if home.Workspace.GitDirty {
		gitState = "has uncommitted changes"
	}
	healthVal := fmt.Sprintf("Workspace %s; search index %s; Git %s.", home.Workspace.Health, home.Workspace.Index, gitState)
	fields = append(fields, termui.Field{Label: "Health", Value: healthVal})

	if home.Workspace.Health != "valid" {
		if len(home.Items) > 0 {
			var issueLines []string
			for _, item := range home.Items {
				issueLines = append(issueLines, item.Summary)
			}
			p.Fields(fields...)
			p.Heading("Issues")
			p.Bullets(issueLines...)
			fields = nil
		}
	} else if home.Workspace.Index != "current" {
		fields = append(fields, termui.Field{Label: "Inventory", Value: "Skill and source counts are unavailable until the search index is rebuilt."})
	} else if home.CountsKnown && home.TotalSkills == 0 && home.HomeSummary.WatchingSources == 0 {
		inv := "No skills yet.\nNext: ask your agent 'create a skill for ...' or run:\n  skillhub skill create my-skill --collection core --name \"My Skill\" --description \"Skill description\""
		fields = append(fields, termui.Field{Label: "Inventory", Value: inv})
	} else if home.CountsKnown {
		inv := fmt.Sprintf("%d active, %d draft; %s.", home.HomeSummary.ActiveSkills, home.DraftSkills, termui.Plural(home.HomeSummary.WatchingSources, "watched source", "watched sources"))
		fields = append(fields, termui.Field{Label: "Inventory", Value: inv})
	}

	if len(fields) > 0 {
		p.Fields(fields...)
	}

	if home.Error != nil {
		p.Error(home.Error.Render.Error, home.Error.Render.Why, home.Error.Render.Fix)
	}

	if len(home.SuggestedActions) > 0 && home.SuggestedActions[0].Label != "" {
		p.Blank()
		action := home.SuggestedActions[0]
		if action.Label == "Review uncommitted changes" {
			cmd := fmt.Sprintf("git -C %s add -A && git -C %s commit -m \"Update skills\"", workspacePath, workspacePath)
			p.Next("Review uncommitted changes", cmd)
		} else {
			p.Next(action.Label, action.Command)
		}
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
			p := termui.New(stderr)
			p.Line(err.Error())
			return 1
		}
		return 0
	}
	p := termui.New(stdout)
	p.Line(result.Summary)
	for _, group := range result.Groups {
		label := strings.ReplaceAll(group.Kind, "_", " ")
		p.Line(fmt.Sprintf("%s (%d):", label, group.Count))
		for _, file := range group.Files {
			statusWord := diffStatusWord(file.Status)
			p.Line(fmt.Sprintf("  %s  %s", statusWord, file.Path))
		}
	}
	p.Blank()
	p.Line("To commit these changes, run:")
	p.Command(fmt.Sprintf("git -C %s add -A && git -C %s commit -m \"...\"", path, path))
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
